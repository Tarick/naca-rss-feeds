package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tarick/naca-rss-feeds/internal/entity"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/gofrs/uuid"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
)

// FeedResponse defines Feed response with Body and any additional headers
// swagger:response
type FeedResponse struct {
	// in: body
	Body FeedResponseBody
}

// FeedResponseBody is returned on successfull operations to get, create or delete feed.
type FeedResponseBody struct {
	// swagger:allOf
	*entity.Feed
}

// Render converts FeedResponseBody to json and sends it to client
func (fp *FeedResponse) Render(w http.ResponseWriter, r *http.Request) {
	// Pre-processing before a response is marshalled and sent across the wire
	// Any instructions here
	render.JSON(w, r, fp.Body)
}

// NewFeedResponse creates new response struct body for feed
func NewFeedResponse(f *entity.Feed) *FeedResponse {
	return &FeedResponse{Body: FeedResponseBody{
		Feed: f,
	}}
}

// Used as middleware to load an feed object from the URL parameters passed through as the request.
// If not found - 404
func (s *Server) feedCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		feedPublicationUUIDParam := chi.URLParam(r, "publication_uuid")
		if feedPublicationUUIDParam == "" {
			ErrNotFound.Render(w, r)
			return
		}
		feedPublicationUUID, err := uuid.FromString(feedPublicationUUIDParam)
		if err != nil {
			ErrInvalidRequest(fmt.Errorf("Wrong UUID format: %w", err)).Render(w, r)
			return
		}
		dbFeed, err := s.repository.GetByPublicationUUID(feedPublicationUUID)
		if err != nil {
			ErrNotFound.Render(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), "feed", dbFeed)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FeedRequest defines update/create request for single feed
type FeedRequest struct {
	Body FeedRequestBody
}

// FeedRequestBody defines data of request body
type FeedRequestBody struct {
	*entity.Feed
}

// Validate request body
func (b FeedRequestBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.URL, validation.Required, validation.Length(5, 100), is.URL),
		validation.Field(&b.PublicationUUID, validation.Required, is.UUID, validation.By(checkUUIDNotNil)),
	)
}

// Bind implements Bind interface for chi Bind to map request body to request body struct
func (b *FeedRequestBody) Bind(r *http.Request) error {
	return b.Validate()
}

// validation helper to check UUID
func checkUUIDNotNil(value interface{}) error {
	u, _ := value.(uuid.UUID)
	if u == uuid.Nil {
		return errors.New("uuid is nil")
	}
	return nil
}

// Response with single feed
func (s *Server) getFeed(w http.ResponseWriter, r *http.Request) {
	dbFeed := r.Context().Value("feed").(*entity.Feed)
	NewFeedResponse(dbFeed).Render(w, r)
}

func (s *Server) createFeed(w http.ResponseWriter, r *http.Request) {
	body := new(FeedRequestBody)
	// data := new(FeedRequest)
	if err := render.Bind(r, body); err != nil {
		ErrInvalidRequest(err).Render(w, r)
		return
	}
	f := &entity.Feed{
		PublicationUUID: body.PublicationUUID,
		URL:             body.URL,
	}
	// TODO: create validator on record, that already exist
	if err := s.repository.Create(f); err != nil {
		ErrInternal(err).Render(w, r)
		return
	}
	// return 201 on create
	render.Status(r, http.StatusCreated)
	NewFeedResponse(f).Render(w, r)
}

func (s *Server) updateFeed(w http.ResponseWriter, r *http.Request) {
	dbFeed := r.Context().Value("feed").(*entity.Feed)

	body := new(FeedRequestBody)
	body.URL = dbFeed.URL
	body.PublicationUUID = dbFeed.PublicationUUID
	if err := render.Bind(r, body); err != nil {
		ErrInvalidRequest(err).Render(w, r)
		return
	}
	dbFeed.URL = body.URL
	dbFeed.PublicationUUID = body.PublicationUUID
	if err := s.repository.Update(dbFeed); err != nil {
		ErrInternal(err).Render(w, r)
		return
	}
	NewFeedResponse(dbFeed).Render(w, r)
}

func (s *Server) deleteFeed(w http.ResponseWriter, r *http.Request) {
	dbFeed := r.Context().Value("feed").(*entity.Feed)
	if err := s.repository.Delete(dbFeed.PublicationUUID); err != nil {
		ErrInternal(err).Render(w, r)
		return
	}
	render.NoContent(w, r)
}

func (s *Server) refreshFeed(w http.ResponseWriter, r *http.Request) {
	dbFeed := r.Context().Value("feed").(*entity.Feed)
	err := s.producer.SendUpdateOne(dbFeed.PublicationUUID)
	if err != nil {
		ErrInternal(err).Render(w, r)
		return
	}
	s.logger.Debug("Sent refresh for one feed message: ", dbFeed)
	render.NoContent(w, r)
}

func (s *Server) refreshAllFeeds(w http.ResponseWriter, r *http.Request) {
	if err := s.producer.SendUpdateAll(); err != nil {
		ErrInternal(err).Render(w, r)
		return
	}
	s.logger.Debug("Sent refresh message for all feeds")
	render.NoContent(w, r)
}

// Returns feeds entries
// TODO: filtering
func (s *Server) getFeeds(w http.ResponseWriter, r *http.Request) {
	dbFeeds, err := s.repository.GetAll()
	if err != nil {
		s.logger.Error("Failure reading feeds from database: ", err)
		ErrInternal(fmt.Errorf("Failure reading feeds from database")).Render(w, r)
		return
	}
	feedsResponse := make([]FeedResponseBody, len(dbFeeds), len(dbFeeds))
	for i := 0; i < len(dbFeeds); i++ {
		feedsResponse[i] = NewFeedResponse(&dbFeeds[i]).Body
	}
	render.JSON(w, r, feedsResponse)
}
