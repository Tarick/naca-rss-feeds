package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tarick/naca-rss-feeds/internal/entity"
	"github.com/asaskevich/govalidator"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otLog "github.com/opentracing/opentracing-go/log"

	"github.com/gofrs/uuid"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
)

// Handler provides http handlers
type Handler struct {
	logger     Logger
	repository FeedsRepository
	producer   RSSFeedsUpdateProducer
	tracer     opentracing.Tracer
}

// RSSFeedsUpdateProducer provides methods to call update (refresh news from) RSS Feed via messaging subsystem
type RSSFeedsUpdateProducer interface {
	SendUpdateOne(context.Context, uuid.UUID) error
	SendUpdateAll(context.Context) error
}

// FeedsRepository defines repository methods used to manage feeds
type FeedsRepository interface {
	Create(context.Context, *entity.Feed) error
	Update(context.Context, *entity.Feed) error
	Delete(context.Context, uuid.UUID) error
	GetAll(context.Context) ([]entity.Feed, error)
	GetByPublicationUUID(context.Context, uuid.UUID) (*entity.Feed, error)
	Healthcheck(context.Context) error
}

// NewHandler creates http handler
func NewHandler(logger Logger, tracer opentracing.Tracer, feedRepository FeedsRepository, messageProducer RSSFeedsUpdateProducer) *Handler {
	return &Handler{
		logger:     logger,
		repository: feedRepository,
		producer:   messageProducer,
		tracer:     tracer,
	}
}

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
func (h *Handler) feedCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := h.setupTracingSpan(r, "retrieve-feed-middleware")
		defer span.Finish()
		var err error

		feedPublicationUUIDParam := chi.URLParam(r, "publication_uuid")
		feedPublicationUUID, err := uuid.FromString(feedPublicationUUIDParam)
		if err != nil {
			ext.HTTPStatusCode.Set(span, http.StatusBadRequest)
			span.LogFields(
				otLog.Error(err),
			)
			ErrInvalidRequest(fmt.Errorf("Wrong UUID format: %w", err)).Render(w, r)
			return
		}
		span.SetTag("feed.PublicationUUID", feedPublicationUUID.String())
		dbFeed, err := h.repository.GetByPublicationUUID(ctx, feedPublicationUUID)
		if err != nil {
			ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
			ErrInternal(err).Render(w, r)
			return
		}
		// empty result
		if dbFeed == nil {
			ext.HTTPStatusCode.Set(span, http.StatusNotFound)
			ErrNotFound.Render(w, r)
			return
		}
		span.LogKV("event", "got feed from repository")
		ctx = context.WithValue(ctx, "feed", dbFeed)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FeedRequest defines update/create request for single feed
type FeedRequest struct {
	// in: body
	Body FeedRequestBody
}

// FeedRequestBody defines data of request body
type FeedRequestBody struct {
	// swagger:allOf
	*entity.Feed
}

var isLanguageCode = validation.NewStringRuleWithError(
	govalidator.IsISO693Alpha2,
	validation.NewError("validation_is_language_code_2_letter", "must be a valid two-letter ISO693Alpha2 language code"))

// Validate request body
func (b FeedRequestBody) Validate() error {
	return validation.ValidateStruct(&b,
		validation.Field(&b.PublicationUUID, validation.Required, is.UUID, validation.By(checkUUIDNotNil)),
		validation.Field(&b.URL, validation.Required, validation.Length(5, 100), is.URL),
		validation.Field(&b.LanguageCode, validation.Required, validation.Length(2, 2), isLanguageCode),
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
func (h *Handler) getFeed(w http.ResponseWriter, r *http.Request) {
	span, _ := h.setupTracingSpan(r, "get-feed")
	defer span.Finish()
	dbFeed := r.Context().Value("feed").(*entity.Feed)
	ext.HTTPStatusCode.Set(span, http.StatusOK)
	span.LogKV("event", "got feed")
	NewFeedResponse(dbFeed).Render(w, r)
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	if err := h.repository.Healthcheck(r.Context()); err != nil {
		h.logger.Error("Healthcheck: repository check failed with: ", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Repository is unailable"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("."))
}

func (h *Handler) createFeed(w http.ResponseWriter, r *http.Request) {
	span, ctx := h.setupTracingSpan(r, "create-feed")
	defer span.Finish()
	body := new(FeedRequestBody)
	// data := new(FeedRequest)
	if err := render.Bind(r, body); err != nil {
		h.logger.Error("Failure accepting input for updating feed", body, " with error: ", err)
		ext.HTTPStatusCode.Set(span, http.StatusBadRequest)
		span.LogFields(
			otLog.Error(err),
		)
		ErrInvalidRequest(err).Render(w, r)
		return
	}
	f := &entity.Feed{
		PublicationUUID: body.PublicationUUID,
		URL:             body.URL,
		LanguageCode:    body.LanguageCode,
	}
	// TODO: create validator on record, that already exist
	if err := h.repository.Create(ctx, f); err != nil {
		ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
		ErrInternal(err).Render(w, r)
		return
	}
	// return 201 on create
	ext.HTTPStatusCode.Set(span, http.StatusCreated)
	span.LogKV("event", "created feed")
	render.Status(r, http.StatusCreated)
	NewFeedResponse(f).Render(w, r)
}

func (h *Handler) updateFeed(w http.ResponseWriter, r *http.Request) {
	span, ctx := h.setupTracingSpan(r, "update-feed")
	defer span.Finish()
	dbFeed := r.Context().Value("feed").(*entity.Feed)

	body := new(FeedRequestBody)
	body.URL = dbFeed.URL
	body.LanguageCode = dbFeed.LanguageCode
	body.PublicationUUID = dbFeed.PublicationUUID
	h.logger.Debug("Updating feed: ", body)
	if err := render.Bind(r, body); err != nil {
		h.logger.Error("Failure accepting input for updating feed", body, " with error: ", err)
		ErrInvalidRequest(err).Render(w, r)
		ext.HTTPStatusCode.Set(span, http.StatusBadRequest)
		span.LogFields(
			otLog.Error(err),
		)
		return
	}
	dbFeed.URL = body.URL
	dbFeed.LanguageCode = body.LanguageCode
	dbFeed.PublicationUUID = body.PublicationUUID
	if err := h.repository.Update(ctx, dbFeed); err != nil {
		h.logger.Error("Failure updating feed in repository", dbFeed, " with error: ", err)
		ErrInternal(err).Render(w, r)
		return
	}
	h.logger.Debug("Updated feed: ", dbFeed)
	span.LogKV("event", "updated feed")
	ext.HTTPStatusCode.Set(span, http.StatusOK)
	render.Status(r, http.StatusOK)
	NewFeedResponse(dbFeed).Render(w, r)
}

func (h *Handler) deleteFeed(w http.ResponseWriter, r *http.Request) {
	span, ctx := h.setupTracingSpan(r, "serve-delete-feed")
	defer span.Finish()
	dbFeed := r.Context().Value("feed").(*entity.Feed)

	if err := h.repository.Delete(ctx, dbFeed.PublicationUUID); err != nil {
		h.logger.Error("Failure deleting feed", dbFeed, " with error: ", err)
		ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
		ErrInternal(err).Render(w, r)
		return
	}
	span.LogKV("event", "deleted feed")
	ext.HTTPStatusCode.Set(span, http.StatusNoContent)
	render.NoContent(w, r)
}

func (h *Handler) refreshFeed(w http.ResponseWriter, r *http.Request) {
	span, ctx := h.setupTracingSpan(r, "serve-refresh-feed")
	defer span.Finish()

	dbFeed := r.Context().Value("feed").(*entity.Feed)
	h.logger.Debug("Sending message to update feed: ", dbFeed)
	err := h.producer.SendUpdateOne(ctx, dbFeed.PublicationUUID)
	if err != nil {
		h.logger.Error("Failure sending message to refresh one feed: ", err)
		ErrInternal(err).Render(w, r)
		ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
		span.LogFields(
			otLog.Error(err),
		)
		return
	}
	h.logger.Debug("Sent message to refresh one feed: ", dbFeed)
	span.LogKV("event", "sent refresh for one feed")
	ext.HTTPStatusCode.Set(span, http.StatusNoContent)
	render.NoContent(w, r)
}

func (h *Handler) refreshAllFeeds(w http.ResponseWriter, r *http.Request) {
	span, ctx := h.setupTracingSpan(r, "serve-refresh-all-feeds")
	defer span.Finish()
	h.logger.Debug("Sending refresh for all feeds")
	if err := h.producer.SendUpdateAll(ctx); err != nil {
		ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
		span.LogFields(
			otLog.Error(err),
		)
		ErrInternal(err).Render(w, r)
		return
	}
	h.logger.Debug("Sent refresh message for all feeds")
	span.LogKV("event", "sent refresh for all feeds")
	ext.HTTPStatusCode.Set(span, http.StatusNoContent)
	render.NoContent(w, r)
}

// Returns feeds entries
// TODO: filtering
func (h *Handler) getFeeds(w http.ResponseWriter, r *http.Request) {
	span, ctx := h.setupTracingSpan(r, "serve-get-all-feeds")
	defer span.Finish()

	dbFeeds, err := h.repository.GetAll(ctx)
	span.LogKV("event", "got feeds from repository")
	if err != nil {
		h.logger.Error("Failure reading feeds from database: ", err)
		ErrInternal(fmt.Errorf("Failure reading feeds from database")).Render(w, r)
		ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
		return
	}
	feedsResponse := make([]FeedResponseBody, len(dbFeeds), len(dbFeeds))
	for i := 0; i < len(dbFeeds); i++ {
		feedsResponse[i] = NewFeedResponse(&dbFeeds[i]).Body
	}
	span.LogKV("event", "populated response feeds slice")
	span.LogFields(
		otLog.Int("feedsNumber", len(dbFeeds)),
	)
	// ext.HTTPStatusCode.Set(span, http.StatusOK)
	// FIXME: convert to encoder, record span status code only after everything is sent
	render.JSON(w, r, feedsResponse)
}

func (h *Handler) setupTracingSpan(r *http.Request, name string) (opentracing.Span, context.Context) {
	// we ignore error since if there are missing headers it will start new trace
	spanContext, _ := h.tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
	span := h.tracer.StartSpan(name, ext.RPCServerOption(spanContext))
	ctx := opentracing.ContextWithSpan(r.Context(), span)
	ext.Component.Set(span, "httpServer-chi")
	ext.HTTPMethod.Set(span, r.Method)
	ext.HTTPUrl.Set(span, r.URL.String())
	return span, ctx
}
