package messaging

import (
	"encoding/json"

	"github.com/gofrs/uuid"
)

const (
	FeedsUpdateOne MessageType = iota
	FeedsUpdateAll
)

// MessageType defines types of messages
//go:generate stringer -type=MessageType
type MessageType uint

// MessageEnvelope defines shared fields for MQ message with message type as action key and Msg as actual message body content
type MessageEnvelope struct {
	Type MessageType `json:"type,int"`
	Msg  interface{}
}

type FeedsUpdateOneMsg struct {
	PublicationUUID uuid.UUID `json:"publication_uuid,string"`
}
type FeedsUpdateAllMsg struct {
}

// NewFeedsUpdateOneMsg returns message with action to update one feed
func NewFeedsUpdateOneMessage(publicationUUID uuid.UUID) *MessageEnvelope {
	return &MessageEnvelope{
		Type: FeedsUpdateOne,
		Msg:  FeedsUpdateOneMsg{PublicationUUID: publicationUUID},
	}
}

// NewFeedsUpdateAllMsg returns message with action to update all feeds
func NewFeedsUpdateAllMessage() *MessageEnvelope {
	return &MessageEnvelope{
		Type: FeedsUpdateAll,
		Msg:  FeedsUpdateAllMsg{},
	}
}

type MessageProducer interface {
	Publish([]byte) error
}

type rssFeedsUpdateProducer struct {
	producer MessageProducer
}

func NewFeedsUpdateProducer(producer MessageProducer) *rssFeedsUpdateProducer {
	return &rssFeedsUpdateProducer{producer}
}
func (p *rssFeedsUpdateProducer) SendUpdateOne(feedPublicationUUID uuid.UUID) error {
	msgbytes, err := json.Marshal(NewFeedsUpdateOneMessage(feedPublicationUUID))
	if err != nil {
		return err
	}
	return p.producer.Publish(msgbytes)
}

func (p *rssFeedsUpdateProducer) SendUpdateAll() error {
	msgbytes, err := json.Marshal(NewFeedsUpdateAllMessage())
	if err != nil {
		return err
	}
	return p.producer.Publish(msgbytes)
}
