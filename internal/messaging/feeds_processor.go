package messaging

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tarick/naca-rss-feeds/internal/entity"
	"github.com/Tarick/naca-rss-feeds/internal/logger"

	"github.com/gofrs/uuid"

	"github.com/mmcdole/gofeed"
)

// ErrNotModified is used for Etag and Last-Modified handling
var ErrNotModified = errors.New("not modified")

// RssFeed is extended feed with etag and lastmodified
type RssFeed struct {
	*gofeed.Feed

	ETag         string
	LastModified time.Time
}

// RSSFeedsUpdateProducer provides methods to call update (refresh news from) RSS Feed via messaging subsystem
type RSSFeedsUpdateProducer interface {
	SendUpdateOne(uuid.UUID) error
	SendUpdateAll() error
}

// FeedsRepository defines repository methods
type FeedsRepository interface {
	GetAll() ([]entity.Feed, error)
	GetByPublicationUUID(uuid.UUID) (*entity.Feed, error)
	GetFeedHTTPMetadataByPublicationUUID(uuid.UUID) (*entity.FeedHTTPMetadata, error)
	SaveFeedHTTPMetadata(*entity.FeedHTTPMetadata) error
	SaveProcessedItem(*entity.ProcessedItem) error
	ProcessedItemExists(*entity.ProcessedItem) (bool, error)
}

// Handler for consumer
type rssFeedsProcessor struct {
	repository          FeedsRepository
	feedsUpdater        RSSFeedsUpdateProducer
	logger              logger.Logger
	GMTTimeZoneLocation *time.Location
}

// NewRSSFeedsProcessor creates processor for messaging feeds operations
func NewRSSFeedsProcessor(repository FeedsRepository, feedsUpdateProducer RSSFeedsUpdateProducer, logger logger.Logger) *rssFeedsProcessor {
	GMTTimeZoneLocation, err := time.LoadLocation("GMT")
	if err != nil {
		panic(err)
	}
	return &rssFeedsProcessor{
		repository,
		feedsUpdateProducer,
		logger,
		GMTTimeZoneLocation,
	}
}

// Process is a gateway for message consumption - handles incoming data and calls related handlers
// It uses json.RawMessage to delay the unmarshalling of message content - Type is unmarshalled first.
// TODO: currently only FeedsUpdateMsg types, we'll need more in the future.
func (p *rssFeedsProcessor) Process(data []byte) error {
	var msg json.RawMessage
	message := MessageEnvelope{Msg: &msg}
	if err := json.Unmarshal(data, &message); err != nil {
		return err
	}
	switch message.Type {
	case FeedsUpdateOne:
		var msgContent FeedsUpdateOneMsg
		if err := json.Unmarshal(msg, &msgContent); err != nil {
			p.logger.Error("Failure unmarshalling FeedsUpdateOneMsg content: ", err)
			return err
		}
		return p.refreshFeed(msgContent.PublicationUUID)
	case FeedsUpdateAll:
		// No body here, just refresh
		return p.refreshAllFeeds()
	default:
		p.logger.Error("Undefined message type: ", message.Type)
		// TODO: implement common errors
		return fmt.Errorf("Undefined message type: %v", message.Type)
	}
}

// refreshFeed refreshes single feed
func (p *rssFeedsProcessor) refreshFeed(publicationUUID uuid.UUID) error {
	dbFeed, err := p.repository.GetByPublicationUUID(publicationUUID)
	if err != nil {
		return fmt.Errorf("couldn't get feed item from repository, %w", err)
	}
	if dbFeed == nil {
		return fmt.Errorf("repository doesn't have items with this publication uuid %v", publicationUUID)
	}
	dbFeedMetadata, err := p.repository.GetFeedHTTPMetadataByPublicationUUID(publicationUUID)
	if err != nil {
		return fmt.Errorf("couldn't get feed HTTP metadata from repository, %w", err)
	}
	if dbFeedMetadata == nil {
		return fmt.Errorf("repository doesn't have HTTP metadata items with this publication uuid %v", publicationUUID)
	}
	p.logger.Debug(fmt.Sprintf("Got feed item from db, %v, with metadata %v", dbFeed, dbFeedMetadata))
	feed, err := p.readFeedFromURL(dbFeed.URL, dbFeedMetadata.ETag, dbFeedMetadata.LastModified)
	if err == ErrNotModified {
		p.logger.Debug("Feed ", dbFeed.URL, " skipped: ", err)
		return nil
	}
	if err != nil {
		return err
	}
	p.logger.Info("Feed ", dbFeed.URL, " returned ", len(feed.Items), " items")
	for _, item := range feed.Items {
		// Skip if such feed (GUID and PubDate) already exist in db as processed item
		// If Pubdate is different - item will be updated.
		processedItem := &entity.ProcessedItem{
			GUID:            item.GUID,
			PublicationUUID: dbFeed.PublicationUUID,
			PublicationDate: *item.PublishedParsed,
		}
		exists, err := p.repository.ProcessedItemExists(processedItem)
		if err != nil {
			p.logger.Error("Couldn't process item with GUID ", processedItem.GUID, "error: ", err)
			continue
		}
		if exists {
			p.logger.Debug("Item ", item.GUID, "with publish date ", item.Published, " already exist, skipping processing")
			continue
		}
		// Otherwise process
		// newItem := &PublishItem{
		// 	Author:      item.Author.Name,
		// 	Title:       item.Title,
		// 	Description: item.Description,
		// 	Content:     item.Content,
		// 	GUID:        item.GUID,
		// 	Link:        item.Link,
		// 	Published:   *item.PublishedParsed,
		// 	Updated:     *item.UpdatedParsed,
		// }
		// publishedMQItem, err := json.Marshal(*newItem)
		// // if err != nil {
		// // 	log.Error(fmt.Sprint("Failure converting item to JSON: ", err))
		// // 	continue
		// // }
		// // if err = producer.Publish(publishConfig.Topic, publishedMQItem); err != nil {
		// // 	log.Error(fmt.Sprint("Failure pushing item to process: ", err))
		// // 	continue
		// // }
		p.logger.Info("Pushed item ", item.GUID, " to process")
		if err := p.repository.SaveProcessedItem(processedItem); err != nil {
			p.logger.Error("Failure saving new processed item: ", err)
			continue
		}
	}
	// Update Feed
	dbFeedMetadata.ETag = feed.ETag
	dbFeedMetadata.LastModified = feed.LastModified
	if err = p.repository.SaveFeedHTTPMetadata(dbFeedMetadata); err != nil {
		return fmt.Errorf("couldn't save feed HTTP metadata, %w", err)
	}
	p.logger.Info("Successfully updated feed ", dbFeed.PublicationUUID)
	return nil
}

// readFeedFromURL fetches feed from url and returns parsed feed
// Uses Etag and Last-Modified to verify if feed didn't change
func (p *rssFeedsProcessor) readFeedFromURL(url string, etag string, lastModified time.Time) (feed *RssFeed, err error) {
	var client = http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Gofeed/1.0")

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
		p.logger.Debug("Set etag for feed retrieval: ", req.Header.Get("If-None-Match"))
	}

	req.Header.Set("If-Modified-Since", lastModified.In(p.GMTTimeZoneLocation).Format(time.RFC1123))
	p.logger.Debug("Set If-Modified-Since header for feed retrieval: ", req.Header.Get("If-Modified-Since"))

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	if resp != nil {
		defer func() {
			ce := resp.Body.Close()
			if ce != nil {
				err = ce
			}
		}()
	}
	p.logger.Debug("Got HTTP response: ", resp.StatusCode)
	if resp.StatusCode == http.StatusNotModified {
		return nil, ErrNotModified
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, gofeed.HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	feed = &RssFeed{}

	feedBody, err := gofeed.NewParser().Parse(resp.Body)
	if err != nil {
		return nil, err
	}
	feed.Feed = feedBody

	if eTag := resp.Header.Get("Etag"); eTag != "" {
		p.logger.Debug("ETag from feed request: ", eTag)
		feed.ETag = eTag
	}

	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		p.logger.Debug("Last-Modifed from feed request: ", lastModified)
		parsed, err := time.ParseInLocation(time.RFC1123, lastModified, p.GMTTimeZoneLocation)
		if err == nil {
			feed.LastModified = parsed
		}
	}

	return feed, err
}

// Refresh all feeds.
// Gets all feeds ids from db and pushes per-feed messages to process.
func (p *rssFeedsProcessor) refreshAllFeeds() error {
	dbFeeds, err := p.repository.GetAll()
	if err != nil {
		return fmt.Errorf("couldn't get feeds from repository, %w", err)
	}
	if len(dbFeeds) == 0 {
		return fmt.Errorf("couldn't get feeds records ids, empty set returned")
	}
	p.logger.Debug("Got ", len(dbFeeds), " feeds to refresh from db")
	for _, dbFeed := range dbFeeds {
		if err := p.feedsUpdater.SendUpdateOne(dbFeed.PublicationUUID); err != nil {
			p.logger.Error("Failure publishing feed refresh for PublicationUUID", dbFeed.PublicationUUID, ": ", err)
			continue
		}
		p.logger.Debug("Published feed refresh for PublicationUUID", dbFeed.PublicationUUID)

	}
	return nil
}

// type PublishItem struct {
// 	Title       string    `json:"title"`
// 	Description string    `json:"description"`
// 	Content     string    `json:"content"`
// 	Link        string    `json:"link"`
// 	Updated     time.Time `json:"updated"`
// 	Published   time.Time `json:"published"`
// 	Author      string    `json:"author"`
// 	GUID        string    `json:"guid"`
// }
