package entity

import (
	"fmt"
	"time"

	"github.com/gofrs/uuid"
)

// Feed defines minimal feed type
// swagger:model
type Feed struct {
	// PublicationUUID that owns this feed (since publication uuid is one to one mapping, no need for other ID as DB serial key)
	PublicationUUID uuid.UUID `json:"publication_uuid"`
	// URL of the feed
	// TODO: separate type, validation (value object)
	URL          string `json:"url"`
	LanguageCode string `json:"language_code"`
}

func (f *Feed) String() string {
	return fmt.Sprintf("PublicationUUID: %v, URL: %s, Language: %s", f.PublicationUUID, f.URL, f.LanguageCode)
}

// FeeFeedHTTPMetadata is used during feed retrieval and parsing
type FeedHTTPMetadata struct {
	PublicationUUID uuid.UUID `json:"publication_uuid"`
	LastModified    time.Time `json:"last_modified"`
	ETag            string    `json:"etag"`
}

func (f *FeedHTTPMetadata) String() string {
	return fmt.Sprintf("LastModified: %v, ETag: %s", f.LastModified, f.ETag)
}
