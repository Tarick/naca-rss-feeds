package messaging

import (
	"context"
	"encoding/json"

	"github.com/gofrs/uuid"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otLog "github.com/opentracing/opentracing-go/log"
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
	Type     MessageType       `json:"type,int"`
	Metadata map[string]string `json:"metadata,string"`
	// Headers interface{}
	Msg interface{}
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

func NewFeedsUpdateProducer(producer MessageProducer, tracer opentracing.Tracer) *rssFeedsUpdateProducer {
	return &rssFeedsUpdateProducer{producer, tracer}
}

type rssFeedsUpdateProducer struct {
	producer MessageProducer
	tracer   opentracing.Tracer
}

func (p *rssFeedsUpdateProducer) setupTracingSpan(ctx context.Context, name string) (opentracing.Span, context.Context) {
	span, ctx := opentracing.StartSpanFromContextWithTracer(ctx, p.tracer, name)
	ext.Component.Set(span, "rssFeedsUpdateProducer")
	return span, ctx
}

func (p *rssFeedsUpdateProducer) SendUpdateOne(ctx context.Context, feedPublicationUUID uuid.UUID) error {
	span, ctx := p.setupTracingSpan(ctx, "send-update-one-feed")
	defer span.Finish()
	carrier := opentracing.TextMapCarrier{}
	err := span.Tracer().Inject(span.Context(), opentracing.TextMap, carrier)
	if err != nil {
		return err
	}
	span.SetTag("feed.PublicationUUID", feedPublicationUUID.String())
	message := NewFeedsUpdateOneMessage(feedPublicationUUID)
	message.Metadata = carrier
	msgbytes, err := json.Marshal(message)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return err
	}
	span.LogKV("event", "sent update one feed message")
	return p.producer.Publish(msgbytes)
}

func (p *rssFeedsUpdateProducer) SendUpdateAll(ctx context.Context) error {
	span, ctx := p.setupTracingSpan(ctx, "send-update-all-feeds")
	defer span.Finish()
	carrier := opentracing.TextMapCarrier{}
	err := span.Tracer().Inject(span.Context(), opentracing.TextMap, carrier)
	if err != nil {
		return err
	}
	message := NewFeedsUpdateAllMessage()
	message.Metadata = carrier
	msgbytes, err := json.Marshal(message)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return err
	}
	err = p.producer.Publish(msgbytes)
	if err != nil {
		span.LogFields(
			otLog.Error(err),
		)
		return err
	}
	span.LogKV("event", "sent update all feeds message")
	return err
}
