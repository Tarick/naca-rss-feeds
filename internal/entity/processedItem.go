package entity

import (
	"fmt"
	"time"

	"github.com/gofrs/uuid"
)

// ProcessedItem defines already processed items from the feed
type ProcessedItem struct {
	// PublicationUUID that owns this feed (since publication uuid is one to one mapping, no need for int ID as DB serial key)
	PublicationUUID uuid.UUID `json:"publication_uuid"`
	GUID            string    `json:"guid"`
	PublicationDate time.Time `json:"publication_date"`
}

func (i *ProcessedItem) String() string {
	return fmt.Sprintf("PublicationUUID: %v, GUID: %s, Publication Date: %v", i.PublicationUUID, i.GUID, i.PublicationDate)
}
