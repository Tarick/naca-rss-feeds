package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tarick/naca-rss-feeds/internal/entity"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/go-chi/stampede"
	"github.com/gofrs/uuid"
)

// Server defines HTTP application
type Server struct {
	httpServer *http.Server
	logger     Logger
	repository FeedsRepository
	producer   RSSFeedsUpdateProducer
}

// RSSFeedsUpdateProducer provides methods to call update (refresh news from) RSS Feed via messaging subsystem
type RSSFeedsUpdateProducer interface {
	SendUpdateOne(uuid.UUID) error
	SendUpdateAll() error
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

// Config defines webserver configuration
type Config struct {
	Address        string `mapstructure:"address"`
	RequestTimeout int    `mapstructure:"request_timeout"`
}

// New creates new server configuration and configurates middleware
// TODO: move routes to handler file
func New(serverConfig Config, logger Logger, feedRepository FeedsRepository, messageProducer RSSFeedsUpdateProducer) *Server {
	r := chi.NewRouter()
	s := &Server{
		httpServer: &http.Server{Addr: serverConfig.Address, Handler: r},
		logger:     logger,
		repository: feedRepository,
		producer:   messageProducer,
	}
	r.Use(middleware.RequestID)
	r.Use(middlewareLogger(logger))
	// Basic CORS to allow API calls from browsers (Swagger-UI)
	// for more ideas, see: https://developer.github.com/v3/#cross-origin-resource-sharing
	r.Use(cors.Handler(cors.Options{
		// AllowedOrigins: []string{"https://foo.com"},
		// Use this to allow specific origin hosts
		AllowedOrigins: []string{"*"},
		// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))
	r.Use(middleware.AllowContentType("application/json"))
	r.Use(middleware.Recoverer)
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Use(middleware.Timeout(time.Duration(serverConfig.RequestTimeout) * time.Second))
	// Healthcheck could be moved back to middleware in case of auth meddling
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if err := s.repository.Healthcheck(r.Context()); err != nil {
			s.logger.Error("Healthcheck: repository check failed with: ", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Repository is unailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("."))
	},
	)
	// Create a route along /doc that will serve contents from
	// the ./swaggerui directory.
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "swaggerui"))
	FileServer(r, "/doc", filesDir)
	r.Route("/feeds", func(r chi.Router) {
		// Set 1 second caching and requests coalescing to avoid requests stampede. Beware of any user specific responses.
		cached := stampede.Handler(512, 1*time.Second)

		// swagger:operation GET /feeds getFeeds
		// Returns all feeds registered in db
		// ---
		// responses:
		//   '200':
		//     description: list all feeds
		//     schema:
		//       type: array
		//       items:
		//         $ref: "#/definitions/FeedResponseBody"
		r.With(cached).Get("/", s.getFeeds)

		// swagger:operation  POST /feeds createFeed
		// Creates feed using supplied params from body
		// ---
		// parameters:
		//  - $ref: "#/definitions/Feed"
		// responses:
		//    '201':
		//      $ref: "#/responses/FeedResponse"
		//    default:
		//      $ref: "#/responses/ErrResponse"
		r.Post("/", s.createFeed)

		r.Route("/{publication_uuid}", func(r chi.Router) {
			r.Use(s.feedCtx) // handle publication_uuid

			// swagger:operation GET /feeds/{publication_uuid} getFeed
			// Gets single feed using its publication_uuid as parameter
			// ---
			// parameters:
			//  - name: publication_uuid
			//    in: path
			//    description: feed publication_uuid to get
			//    required: true
			//    type: string
			// responses:
			//    '200':
			//      $ref: "#/responses/FeedResponse"
			//    default:
			//      $ref: "#/responses/ErrResponse"
			r.Get("/", s.getFeed)

			// swagger:operation PUT /feeds/{publication_uuid} updateFeed
			// Modifies feed using supplied params from body
			// ---
			// parameters:
			//  - name: publication_uuid
			//    in: path
			//    description: Feed publication_uuid to update
			//    required: true
			//    type: string
			//  - $ref: "#/definitions/Feed"
			// responses:
			//    '200':
			//      $ref: "#/responses/FeedResponse"
			//    default:
			//      $ref: "#/responses/ErrResponse"
			r.Put("/", s.updateFeed)

			// swagger:operation DELETE /feeds/{publication_uuid} deleteFeed
			// Deletes feed using its publication_uuid
			// ---
			// parameters:
			//  - name: publication_uuid
			//    in: path
			//    description: Feed publication_uuid to update
			//    required: true
			//    type: string
			// responses:
			//  '204':
			//    description: Send success
			//  default:
			//    $ref: "#/responses/ErrResponse"
			r.Delete("/", s.deleteFeed)
		})
	})
	r.Route("/refreshFeeds", func(r chi.Router) {
		// Set 60 second caching and requests coalescing to avoid requests stampede for all feeds refresh
		cachedAll := stampede.Handler(512, 10*time.Second)
		// Set 10 second caching and requests coalescing to avoid requests stampede for one feed refresh
		cachedOne := stampede.Handler(512, 10*time.Second)
		// swagger:operation PUT /refreshFeeds refreshFeeds
		// Triggers refresh (pull of content) for all feeds
		// ---
		// responses:
		//    '204':
		//      description: Send success
		//    default:
		//      description: Error payload
		//      schema:
		//        $ref: "#/responses/ErrResponse"
		r.With(cachedAll).Put("/", s.refreshAllFeeds)
		// swagger:operation PUT /refreshFeeds/{publication_uuid} refreshFeed
		// Triggers refresh (pull of content) for single feeds
		// ---
		// parameters:
		//  - name: publication_uuid
		//    in: path
		//    description: Feed publication_uuid to update
		//    required: true
		//    type: string
		// responses:
		//    '204':
		//      description: Send success
		//    default:
		//      $ref: "#/responses/ErrResponse"
		r.Route("/{publication_uuid}", func(r chi.Router) {
			r.Use(s.feedCtx)                          // handle publication_uuid
			r.With(cachedOne).Put("/", s.refreshFeed) // PUT /refreshFeeds/sfsd-fds-fsd-fsd
		})
	})
	return s
}

// StartAndServe configures routers and starts http server
func (s *Server) StartAndServe() {
	s.logger.Info("Server is ready to serve on ", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Fatal(fmt.Sprint("Server startup failed: ", err))
	}
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem. Used for Swagger-UI and swagger.json files.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
