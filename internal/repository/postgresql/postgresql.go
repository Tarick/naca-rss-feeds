package postgresql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tarick/naca-rss-feeds/internal/entity"

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

type FeedsRepositoryImpl struct {
	pool *pgxpool.Pool
}

func NewZapLogger(logger *zap.Logger) *zapadapter.Logger {
	return zapadapter.NewLogger(logger)
}

// New creates database pool configuration
func New(databaseConfig *Config, logger pgx.Logger) (*FeedsRepositoryImpl, error) {
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
	return &FeedsRepositoryImpl{pool: pool}, nil
}

func (repository *FeedsRepositoryImpl) Create(ctx context.Context, f *entity.Feed) error {
	_, err := repository.pool.Exec(ctx, "insert into feeds (publication_uuid, url, language_code) values ($1, $2, $3)", f.PublicationUUID, f.URL, f.LanguageCode)
	return err
}

func (feedRepo *FeedsRepositoryImpl) Update(ctx context.Context, f *entity.Feed) error {
	_, err := feedRepo.pool.Exec(ctx, "update feeds set url=$1, language_code=$2 where publication_uuid=$3", f.URL, f.LanguageCode, f.PublicationUUID)
	return err
}

func (feedRepo *FeedsRepositoryImpl) Delete(ctx context.Context, publicationUUID uuid.UUID) error {
	result, err := feedRepo.pool.Exec(ctx, "delete from feeds where publication_uuid=$1", publicationUUID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New(fmt.Sprint("feeds delete from db execution didn't delete record for UUID ", publicationUUID))
	}
	return err
}

func (feedRepo *FeedsRepositoryImpl) GetByPublicationUUID(ctx context.Context, publicationUUID uuid.UUID) (*entity.Feed, error) {
	f := &entity.Feed{}
	err := feedRepo.pool.QueryRow(ctx, "select publication_uuid, url, language_code from feeds where publication_uuid=$1", publicationUUID).Scan(&f.PublicationUUID, &f.URL, &f.LanguageCode)
	if err != nil && err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}
func (feedRepo *FeedsRepositoryImpl) GetFeedHTTPMetadataByPublicationUUID(ctx context.Context, publicationUUID uuid.UUID) (*entity.FeedHTTPMetadata, error) {
	m := &entity.FeedHTTPMetadata{}
	err := feedRepo.pool.QueryRow(ctx, "SELECT publication_uuid, COALESCE(etag, 'noetag'), COALESCE(last_modified,$2) FROM feeds WHERE publication_uuid=$1",
		publicationUUID, time.Time{}).
		Scan(&m.PublicationUUID, &m.ETag, &m.LastModified)
	if err != nil && err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}
func (feedRepo *FeedsRepositoryImpl) SaveFeedHTTPMetadata(ctx context.Context, m *entity.FeedHTTPMetadata) error {
	_, err := feedRepo.pool.Exec(ctx, "update feeds set etag=$1, last_modified=$2 where publication_uuid=$3", m.ETag, m.LastModified, m.PublicationUUID)
	return err
}

func (feedRepo *FeedsRepositoryImpl) GetAll(ctx context.Context) ([]entity.Feed, error) {
	rows, err := feedRepo.pool.Query(ctx, "select publication_uuid, url, language_code from feeds")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feeds := []entity.Feed{}
	for rows.Next() {
		f := entity.Feed{}
		if err := rows.Scan(&f.PublicationUUID, &f.URL, &f.LanguageCode); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	if rows.Err() != nil {
		return nil, err
	}
	return feeds, nil
}

func (feedRepo *FeedsRepositoryImpl) SaveProcessedItem(ctx context.Context, i *entity.ProcessedItem) error {
	_, err := feedRepo.pool.Exec(ctx,
		"INSERT INTO processed_items (guid, feeds_publication_uuid, pubDate) VALUES ($1, $2, $3) ON CONFLICT (guid) DO UPDATE SET pubDate=EXCLUDED.pubDate",
		i.GUID, i.PublicationUUID, i.PublicationDate)
	return err
}

func (feedRepo *FeedsRepositoryImpl) ProcessedItemExists(ctx context.Context, i *entity.ProcessedItem) (bool, error) {
	var exists bool
	row := feedRepo.pool.QueryRow(ctx, "select exists (select 1 from processed_items where (guid=$1 AND feeds_publication_uuid=$2 AND pubDate=$3))",
		i.GUID, i.PublicationUUID, i.PublicationDate)
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	if exists == true {
		return true, nil
	}
	return false, nil
}
