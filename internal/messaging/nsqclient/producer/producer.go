package producer

import (
	"github.com/nsqio/go-nsq"
)

// MessageProducerConfig defines NSQ publish configuration
type MessageProducerConfig struct {
	Host  string `mapstructure:"host"`
	Topic string `mapstructure:"topic"`
}
type messageProducer struct {
	producer *nsq.Producer
	topic    string
}

func (p *messageProducer) Stop() {
	p.producer.Stop()
}

func (p *messageProducer) Publish(body []byte) error {
	return p.producer.Publish(p.topic, body)
}

// New returns producer if infra is ok.
func New(config *MessageProducerConfig) (*messageProducer, error) {
	msgProducer := &messageProducer{
		topic: config.Topic,
	}

	producer, err := nsq.NewProducer(config.Host, nsq.NewConfig())
	if err != nil {
		return nil, err
	}
	if err := producer.Ping(); err != nil {
		return nil, err
	}
	msgProducer.producer = producer
	return msgProducer, nil
}
