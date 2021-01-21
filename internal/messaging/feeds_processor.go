package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tarick/naca-rss-feeds/internal/entity"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otLog "github.com/opentracing/opentracing-go/log"

	"github.com/gofrs/uuid"

	"github.com/mmcdole/gofeed"
)

type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}

// ErrNotModified is used for Etag and Last-Modified handling
var ErrNotModified = errors.New("not modified")

// RSSFeed is extended feed with etag and lastmodified
type RSSFeed struct {
	*gofeed.Feed

	ETag         string
	LastModified time.Time
}

// RSSFeedsUpdateProducer provides methods to call update (refresh news from) RSS Feed via messaging subsystem
type RSSFeedsUpdateProducer interface {
	SendUpdateOne(context.Context, uuid.UUID) error
	SendUpdateAll(context.Context) error
}

// FeedsRepository defines repository methods
type FeedsRepository interface {
	GetAll(context.Context) ([]entity.Feed, error)
	GetByPublicationUUID(context.Context, uuid.UUID) (*entity.Feed, error)
	GetFeedHTTPMetadataByPublicationUUID(context.Context, uuid.UUID) (*entity.FeedHTTPMetadata, error)
	SaveFeedHTTPMetadata(context.Context, *entity.FeedHTTPMetadata) error
	SaveProcessedItem(context.Context, *entity.ProcessedItem) error
	ProcessedItemExists(context.Context, *entity.ProcessedItem) (bool, error)
}

type ItemPublisherClient interface {
	PublishNewItem(
		publicationUUID uuid.UUID,
		title string,
		description string,
		content string,
		url string,
		languageCode string,
		publishedDate time.Time,
	) error
}

// Handler for consumer
type rssFeedsProcessor struct {
	repository          FeedsRepository
	feedsUpdater        RSSFeedsUpdateProducer
	itemPublisher       ItemPublisherClient
	logger              Logger
	tracer              opentracing.Tracer
	GMTTimeZoneLocation *time.Location
}

// NewRSSFeedsProcessor creates processor for messaging feeds operations
func NewRSSFeedsProcessor(repository FeedsRepository, feedsUpdateProducer RSSFeedsUpdateProducer, itemPublisherClient ItemPublisherClient, logger Logger, tracer opentracing.Tracer) *rssFeedsProcessor {
	GMTTimeZoneLocation, err := time.LoadLocation("GMT")
	if err != nil {
		panic(err)
	}
	return &rssFeedsProcessor{
		repository,
		feedsUpdateProducer,
		itemPublisherClient,
		logger,
		tracer,
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
	// Setup tracing span
	messageSpanContext, err := p.tracer.Extract(opentracing.TextMap, opentracing.TextMapCarrier(message.Metadata))
	if err != nil {
		p.logger.Debug("No tracing information in message metadata: ", err)
	}
	span := p.tracer.StartSpan("process-message", opentracing.FollowsFrom(messageSpanContext))
	defer span.Finish()
	ext.Component.Set(span, "rssFeedsProcessor")
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	switch message.Type {
	case FeedsUpdateOne:
		var msgContent FeedsUpdateOneMsg
		if err := json.Unmarshal(msg, &msgContent); err != nil {
			p.logger.Error("Failure unmarshalling FeedsUpdateOneMsg content: ", err)
			span.LogFields(
				otLog.Error(err),
			)
			return err
		}
		return p.refreshFeed(ctx, msgContent.PublicationUUID)
	case FeedsUpdateAll:
		// No body here, just refresh
		return p.refreshAllFeeds(ctx)
	default:
		p.logger.Error("Undefined message type: ", message.Type)
		span.LogFields(
			otLog.Error(fmt.Errorf("Underfined message type: %s", message.Type)),
		)
		// TODO: implement common errors
		return fmt.Errorf("Undefined message type: %v", message.Type)
	}
}

// refreshFeed refreshes single feed
func (p *rssFeedsProcessor) refreshFeed(ctx context.Context, publicationUUID uuid.UUID) error {
	span, ctx := p.setupTracingSpan(ctx, "refresh-feed")
	defer span.Finish()
	span.SetTag("feed.publicationUUID", publicationUUID)

	dbFeed, err := p.repository.GetByPublicationUUID(ctx, publicationUUID)
	if err != nil {
		return fmt.Errorf("couldn't get feed item from repository, %w", err)
	}
	if dbFeed == nil {
		span.LogKV("event", "no feed to refresh")
		return fmt.Errorf("repository doesn't have items with this publication uuid %v", publicationUUID)
	}
	dbFeedMetadata, err := p.repository.GetFeedHTTPMetadataByPublicationUUID(ctx, publicationUUID)
	if err != nil {
		return fmt.Errorf("couldn't get feed HTTP metadata from repository, %w", err)
	}
	if dbFeedMetadata == nil {
		return fmt.Errorf("repository doesn't have HTTP metadata items with this publication uuid %v", publicationUUID)
	}
	p.logger.Debug(fmt.Sprintf("Got feed item from db, %v, with metadata %v", dbFeed, dbFeedMetadata))
	feed, err := p.readFeedFromURL(ctx, dbFeed.URL, dbFeedMetadata.ETag, dbFeedMetadata.LastModified)
	if err == ErrNotModified {
		p.logger.Debug("Feed ", dbFeed.URL, " skipped: ", err)
		span.LogKV("event", "feed update skipped as not modified")
		return nil
	}
	if err != nil {
		return err
	}
	p.logger.Info("Feed ", dbFeed.URL, " returned ", len(feed.Items), " items")
	for _, item := range feed.Items {
		var itemPublished *time.Time
		if item.PublishedParsed == nil {
			if item.UpdatedParsed != nil {
				itemPublished = item.UpdatedParsed
			} else {
				p.logger.Error("Item ", item.GUID, " doesn't have set Published or Updated fields, skipping")
				span.LogFields(
					otLog.Error(err),
				)
				continue
			}
		} else {
			itemPublished = item.PublishedParsed
		}
		processedItem := &entity.ProcessedItem{
			GUID:            item.GUID,
			PublicationUUID: dbFeed.PublicationUUID,
			PublicationDate: *itemPublished,
		}
		exists, err := p.repository.ProcessedItemExists(ctx, processedItem)
		if err != nil {
			p.logger.Error("Couldn't process item with GUID ", processedItem.GUID, "error: ", err)
			span.LogFields(
				otLog.Error(err),
			)
			continue
		}
		// Skip if such feed (GUID and PubDate) already exist in db as processed item
		// If Pubdate is different - item will be updated.
		// If Pubdate is missing - Update date will be used, otherwise skipped.
		if exists {
			p.logger.Debug("Item ", item.GUID, "with publish date ", item.Published, " already exist, skipping processing")
			span.LogKV("event", "item already exists, skipping processing")
			continue
		}
		// Publish new item to Items service
		err = p.itemPublisher.PublishNewItem(
			publicationUUID,
			item.Title,
			item.Description,
			item.Content,
			item.Link,
			dbFeed.LanguageCode,
			itemPublished.In(time.UTC))

		if err != nil {
			p.logger.Error("failed to publish new item ", item.GUID, " of publication ", dbFeed.PublicationUUID, " with error ", err)
			span.LogFields(
				otLog.Error(err),
			)
			continue
		}
		p.logger.Info("Pushed item ", item.GUID, " to process")
		span.LogKV("event", "pushed item to process")
		if err := p.repository.SaveProcessedItem(ctx, processedItem); err != nil {
			p.logger.Error("Failure saving new processed item: ", err)
			continue
		}
	}
	// Update Feed
	dbFeedMetadata.ETag = feed.ETag
	dbFeedMetadata.LastModified = feed.LastModified
	if err = p.repository.SaveFeedHTTPMetadata(ctx, dbFeedMetadata); err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return fmt.Errorf("couldn't save feed HTTP metadata, %w", err)
	}
	span.LogKV("event", "saved feed http metadata")
	p.logger.Info("Successfully updated feed ", dbFeed.PublicationUUID)
	return nil
}

// readFeedFromURL fetches feed from url and returns parsed feed
// Uses Etag and Last-Modified to verify if feed didn't change
func (p *rssFeedsProcessor) readFeedFromURL(ctx context.Context, url string, etag string, lastModified time.Time) (feed *RSSFeed, err error) {
	span, ctx := p.setupTracingSpan(ctx, "read-feed-from-url")
	defer span.Finish()
	span.SetTag("feed.url", url)

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
	span.LogKV("event", "queried feed remote endpoint")

	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
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
	ext.HTTPStatusCode.Set(span, uint16(resp.StatusCode))

	if resp.StatusCode == http.StatusNotModified {
		return nil, ErrNotModified
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, gofeed.HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	feed = &RSSFeed{}

	feedBody, err := gofeed.NewParser().Parse(resp.Body)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
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
	span.LogKV("event", "parsed feed")
	return feed, err
}

// Refresh all feeds.
// Gets all feeds ids from db and pushes per-feed messages to process.
func (p *rssFeedsProcessor) refreshAllFeeds(ctx context.Context) error {
	span, ctx := p.setupTracingSpan(ctx, "refresh-all-feeds")
	defer span.Finish()

	dbFeeds, err := p.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("couldn't get feeds from repository, %w", err)
	}
	if len(dbFeeds) == 0 {
		span.LogKV("error", "no feeds returned")
		return fmt.Errorf("couldn't get feeds records ids, empty set returned")
	}
	p.logger.Debug("Got ", len(dbFeeds), " feeds to refresh from db")
	// FIXME: go parallel
	for _, dbFeed := range dbFeeds {
		if err := p.feedsUpdater.SendUpdateOne(ctx, dbFeed.PublicationUUID); err != nil {
			p.logger.Error("Failure publishing feed refresh for PublicationUUID", dbFeed.PublicationUUID, ": ", err)
			continue
		}
		p.logger.Debug("Published feed refresh for PublicationUUID", dbFeed.PublicationUUID)

	}
	span.LogKV("event", "finished sending feeds update")
	return nil
}

func (p *rssFeedsProcessor) setupTracingSpan(ctx context.Context, name string) (opentracing.Span, context.Context) {
	span, ctx := opentracing.StartSpanFromContextWithTracer(ctx, p.tracer, name)
	ext.Component.Set(span, "rssFeedsProcessor")
	return span, ctx
}
