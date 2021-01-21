package postgresql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tarick/naca-rss-feeds/internal/entity"
	opentracing "github.com/opentracing/opentracing-go"
	otLog "github.com/opentracing/opentracing-go/log"

	"go.uber.org/zap"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/log/zapadapter"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Config defines database configuration, usable for Viper
type Config struct {
	Name           string `mapstructure:"name"`
	Hostname       string `mapstructure:"hostname"`
	Port           string `mapstructure:"port"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	SSLMode        string `mapstructure:"sslmode"`
	LogLevel       string `mapstructure:"log_level"`
	MinConnections int32  `mapstructure:"min_connections"`
	MaxConnections int32  `mapstructure:"max_connections"`
}

type Repository struct {
	pool   *pgxpool.Pool
	tracer opentracing.Tracer
}

func NewZapLogger(logger *zap.Logger) *zapadapter.Logger {
	return zapadapter.NewLogger(logger)
}

// New creates database pool configuration
func New(databaseConfig *Config, logger pgx.Logger, tracer opentracing.Tracer) (*Repository, error) {
	postgresDataSource := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		databaseConfig.Username,
		databaseConfig.Password,
		databaseConfig.Hostname,
		databaseConfig.Name,
		databaseConfig.SSLMode)
	poolConfig, err := pgxpool.ParseConfig(postgresDataSource)
	if err != nil {
		return nil, err
	}
	poolConfig.ConnConfig.Logger = logger
	logLevelMapping := map[string]pgx.LogLevel{
		"trace": pgx.LogLevelTrace,
		"debug": pgx.LogLevelDebug,
		"info":  pgx.LogLevelInfo,
		"warn":  pgx.LogLevelWarn,
		"error": pgx.LogLevelError,
	}
	poolConfig.ConnConfig.LogLevel = logLevelMapping[databaseConfig.LogLevel]
	poolConfig.MaxConns = databaseConfig.MaxConnections
	poolConfig.MinConns = databaseConfig.MinConnections

	pool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, err
	}
	return &Repository{pool: pool, tracer: tracer}, nil
}

func (repository *Repository) Create(ctx context.Context, f *entity.Feed) error {
	query := "insert into feeds (publication_uuid, url, language_code) values ($1, $2, $3)"
	span, ctx := repository.setupTracingSpan(ctx, "get-feed-http-metadata", query)
	defer span.Finish()
	_, err := repository.pool.Exec(ctx, query, f.PublicationUUID, f.URL, f.LanguageCode)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
	} else {
		span.LogKV("event", "created feed")
	}
	return err
}

func (repository *Repository) Update(ctx context.Context, f *entity.Feed) error {
	query := "update feeds set url=$1, language_code=$2 where publication_uuid=$3"
	span, ctx := repository.setupTracingSpan(ctx, "update-feed", query)
	defer span.Finish()
	_, err := repository.pool.Exec(ctx, query, f.URL, f.LanguageCode, f.PublicationUUID)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
	} else {
		span.LogKV("event", "updated feed")
	}
	return err
}

func (repository *Repository) Delete(ctx context.Context, publicationUUID uuid.UUID) error {
	query := "delete from feeds where publication_uuid=$1"
	span, ctx := repository.setupTracingSpan(ctx, "delete-feed", query)
	defer span.Finish()
	span.LogKV("publicationUUID", publicationUUID)
	result, err := repository.pool.Exec(ctx, query, publicationUUID)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return err
	}
	if result.RowsAffected() != 1 {
		span.LogKV("event", "didn't find the feed to delete")
		return errors.New(fmt.Sprint("feeds delete from db execution didn't delete record for UUID ", publicationUUID))
	}

	span.LogKV("event", "delete feed")
	return err
}

func (repository *Repository) GetByPublicationUUID(ctx context.Context, publicationUUID uuid.UUID) (*entity.Feed, error) {
	query := "select publication_uuid, url, language_code from feeds where publication_uuid=$1"
	span, ctx := repository.setupTracingSpan(ctx, "get-feed-by-publicationUUID", query)
	defer span.Finish()

	f := &entity.Feed{}
	err := repository.pool.QueryRow(ctx, query, publicationUUID).Scan(&f.PublicationUUID, &f.URL, &f.LanguageCode)
	if err != nil && err == pgx.ErrNoRows {
		span.LogKV("event", "feed not found")
		return nil, nil
	}
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)

		return nil, err
	}
	span.LogKV("event", "got feed")
	return f, nil
}
func (repository *Repository) GetFeedHTTPMetadataByPublicationUUID(ctx context.Context, publicationUUID uuid.UUID) (*entity.FeedHTTPMetadata, error) {
	query := "SELECT publication_uuid, COALESCE(etag, 'noetag'), COALESCE(last_modified,$2) FROM feeds WHERE publication_uuid=$1"
	span, ctx := repository.setupTracingSpan(ctx, "get-feed-http-metadata", query)
	defer span.Finish()
	m := &entity.FeedHTTPMetadata{}
	err := repository.pool.QueryRow(ctx, query, publicationUUID, time.Time{}).Scan(&m.PublicationUUID, &m.ETag, &m.LastModified)
	if err != nil && err == pgx.ErrNoRows {
		span.LogFields(
			otLog.Error(err),
		)
		return nil, nil
	}
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return nil, err
	}
	span.LogKV("event", "got feed http metadata")
	return m, nil
}
func (repository *Repository) SaveFeedHTTPMetadata(ctx context.Context, m *entity.FeedHTTPMetadata) error {
	query := "update feeds set etag=$1, last_modified=$2 where publication_uuid=$3"
	span, ctx := repository.setupTracingSpan(ctx, "save-feed-http-metadata", query)
	defer span.Finish()
	_, err := repository.pool.Exec(ctx, query, m.ETag, m.LastModified, m.PublicationUUID)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
	} else {
		span.LogKV("event", "saved feed http metadata")
	}
	return err
}

func (repository *Repository) GetAll(ctx context.Context) ([]entity.Feed, error) {
	query := "select publication_uuid, url, language_code from feeds"
	span, ctx := repository.setupTracingSpan(ctx, "repository-feeds-get-all", query)
	defer span.Finish()
	rows, err := repository.pool.Query(ctx, query)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return nil, err
	}
	span.LogKV("event", "query DB for all feeds")
	defer rows.Close()

	feeds := []entity.Feed{}
	for rows.Next() {
		f := entity.Feed{}
		if err := rows.Scan(&f.PublicationUUID, &f.URL, &f.LanguageCode); err != nil {
			span.LogFields(
				otLog.Error(err),
			)
			return nil, err
		}
		feeds = append(feeds, f)
	}
	if rows.Err() != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return nil, err
	}
	span.LogKV("items number", len(feeds))

	return feeds, nil
}

func (repository *Repository) SaveProcessedItem(ctx context.Context, i *entity.ProcessedItem) error {
	query := "INSERT INTO processed_items (guid, feeds_publication_uuid, pubDate) VALUES ($1, $2, $3) ON CONFLICT (guid) DO UPDATE SET pubDate=EXCLUDED.pubDate"
	span, ctx := repository.setupTracingSpan(ctx, "save-processed-item", query)
	defer span.Finish()
	_, err := repository.pool.Exec(ctx, query, i.GUID, i.PublicationUUID, i.PublicationDate)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
	} else {
		span.LogKV("event", "saved processed item")
	}
	return err
}

func (repository *Repository) ProcessedItemExists(ctx context.Context, i *entity.ProcessedItem) (bool, error) {
	var exists bool
	query := "select exists (select 1 from processed_items where (guid=$1 AND feeds_publication_uuid=$2 AND pubDate=$3))"
	span, ctx := repository.setupTracingSpan(ctx, "check-processed-item-exists", query)
	defer span.Finish()
	row := repository.pool.QueryRow(ctx, query, i.GUID, i.PublicationUUID, i.PublicationDate)
	if err := row.Scan(&exists); err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return false, err
	}
	if exists == true {
		span.LogKV("event", "processed item already exists")
		return true, nil
	}
	span.LogKV("event", "processed item doesn't exist")
	return false, nil
}

// Healthcheck is needed for application healtchecks
func (repository *Repository) Healthcheck(ctx context.Context) error {
	var exists bool
	query := "select exists (select 1 from feeds limit 1)"
	row := repository.pool.QueryRow(ctx, query)
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	return fmt.Errorf("failure checking access to 'feeds' table")
}
func (repository *Repository) setupTracingSpan(ctx context.Context, name string, query string) (opentracing.Span, context.Context) {
	span, ctx := opentracing.StartSpanFromContextWithTracer(ctx, repository.tracer, name)
	span.SetTag("component", "repository")
	span.SetTag("db.type", "sql")
	span.SetTag("db.query", query)
	return span, ctx
}
