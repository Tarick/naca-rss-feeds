package processor

import "github.com/gofrs/uuid"

const (
	// Enumeration type to specify Type in messages in order to efficiently unmarshal variable params messages
	FeedsUpdateOne MessageType = iota
	FeedsUpdateAll
)

// MessageType defines types of messages
//go:generate stringer -type=MessageType
type MessageType uint

// MessageEnvelope defines shared fields for message with message type as action key, any metadata (e.g. opentracing) and Msg as actual message body content
type MessageEnvelope struct {
	Type     MessageType       `json:"type,int"`
	Metadata map[string]string `json:"metadata,string"`
	Msg      interface{}
}

// FeedsUpdateOneMsg is used to trigger update for one feed using its publicationUUID
type FeedsUpdateOneMsg struct {
	PublicationUUID uuid.UUID `json:"publication_uuid,string"`
}

// FeedsUpdateAllMsg is used to trigger update of all feeds
type FeedsUpdateAllMsg struct {
}

// NewFeedsUpdateOneMessage returns message envelope with action to update one feed
func NewFeedsUpdateOneMessage(publicationUUID uuid.UUID) *MessageEnvelope {
	return &MessageEnvelope{
		Type: FeedsUpdateOne,
		Msg:  FeedsUpdateOneMsg{PublicationUUID: publicationUUID},
	}
}

// NewFeedsUpdateAllMessage returns message with action to update all feeds
func NewFeedsUpdateAllMessage() *MessageEnvelope {
	return &MessageEnvelope{
		Type: FeedsUpdateAll,
		Msg:  FeedsUpdateAllMsg{},
	}
}
