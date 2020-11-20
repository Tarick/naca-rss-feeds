package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Tarick/naca-rss-feeds/internal/application/server"
	"github.com/Tarick/naca-rss-feeds/internal/entity"

	"github.com/gofrs/uuid"
)

const feedsCRUDPath string = "/feeds"

// TODO: WithTimeout?
// New creates RSS Feeds API http client
func New(baseURL string) (*client, error) {
	url, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &client{
		baseURL: url,
		httpClient: &http.Client{
			Timeout: time.Minute,
		}}, nil
}

// TODO: add logger
type client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func (c *client) GetRSSFeedByPublicationUUID(ctx context.Context, publicationUUID uuid.UUID) (entity.Feed, error) {
	rel := &url.URL{Path: fmt.Sprintf("%s/%s", feedsCRUDPath, publicationUUID)}
	u := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return entity.Feed{}, err
	}
	req = req.WithContext(ctx)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return entity.Feed{}, err
	}
	if res != nil {
		defer func() {
			ce := res.Body.Close()
			if ce != nil {
				err = ce
			}
		}()
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes server.ErrResponseBody
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return entity.Feed{}, errors.New(errRes.ErrorText)
		}

		return entity.Feed{}, fmt.Errorf("unknown error, status code: %d", res.StatusCode)
	}
	feed := entity.Feed{}
	if err = json.NewDecoder(res.Body).Decode(&feed); err != nil {
		return entity.Feed{}, err
	}
	return feed, nil
}

func (c *client) GetAllRSSFeeds(ctx context.Context) ([]entity.Feed, error) {
	rel := &url.URL{Path: feedsCRUDPath}
	u := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes server.ErrResponseBody
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return nil, errors.New(errRes.ErrorText)
		}

		return nil, fmt.Errorf("unknown error, status code: %d", res.StatusCode)
	}
	feeds := []entity.Feed{}
	if err = json.NewDecoder(res.Body).Decode(&feeds); err != nil {
		return []entity.Feed{}, err
	}
	return feeds, nil
}

func (c *client) UpdateRSSFeed(ctx context.Context, publicationUUID uuid.UUID, feedURL string) error {
	feed := &entity.Feed{
		PublicationUUID: publicationUUID,
		URL:             feedURL,
	}
	body, err := json.Marshal(feed)
	if err != nil {
		return err
	}
	rel := &url.URL{Path: fmt.Sprintf("%s/%s", feedsCRUDPath, publicationUUID)}
	u := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequest("PUT", u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes server.ErrResponseBody
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return errors.New(errRes.ErrorText)
		}
		return fmt.Errorf("unknown error, status code: %d, message: %v", res.StatusCode, res.Status)
	}
	return nil
}

func (c *client) CreateRSSFeed(ctx context.Context, publicationUUID uuid.UUID, feedURL string) error {
	feed := &entity.Feed{
		PublicationUUID: publicationUUID,
		URL:             feedURL,
	}
	body, err := json.Marshal(feed)
	if err != nil {
		return err
	}
	rel := &url.URL{Path: feedsCRUDPath}
	u := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequest("POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusCreated {
		return nil
	}
	// handle error
	var errRes server.ErrResponseBody
	if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
		return errors.New(errRes.ErrorText)
	}
	return fmt.Errorf("unknown error, status code: %d, message: %v", res.StatusCode, res.Status)
}

func (c *client) DeleteRSSFeed(ctx context.Context, publicationUUID uuid.UUID) error {
	rel := &url.URL{Path: fmt.Sprintf("%s/%s", feedsCRUDPath, publicationUUID)}
	u := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequest("DELETE", u.String(), nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	// handle error
	var errRes server.ErrResponseBody
	if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
		return errors.New(errRes.ErrorText)
	}
	return fmt.Errorf("unknown error, status code: %d, message: %v", res.StatusCode, res.Status)
}
