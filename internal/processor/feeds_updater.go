package processor

import (
	"context"
	"encoding/json"

	"github.com/gofrs/uuid"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otLog "github.com/opentracing/opentracing-go/log"
)

// MessageProducer is used to publish messages
type MessageProducer interface {
	Publish([]byte) error
}

// NewFeedsUpdateProducer returns producer to publish feeds update messages
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
