package consumer

import (
	"github.com/nsqio/go-nsq"
)

// MessageConsumerConfig defines NSQ publish configuration
type MessageConsumerConfig struct {
	NSQLookup string `mapstructure:"nsqlookup"`
	Topic     string `mapstructure:"topic"`
	Channel   string `mapstructure:"channel"`
	Prefetch  int    `mapstructure:"prefetch"`
	Workers   int    `mapstructure:"workers"`
	Attempts  uint16 `mapstructure:"attempts"`
}

type MessageProcessor interface {
	Process([]byte) error
}
type messageHandler struct {
	processor MessageProcessor
	logger    Logger
}

// HandleMessage implements the Handler interface.
func (h *messageHandler) HandleMessage(m *nsq.Message) error {
	if len(m.Body) == 0 {
		// Returning nil will automatically send a FIN command to NSQ to mark the message as processed.
		return nil
	}

	h.logger.Debug("Message body received: ", string(m.Body))
	err := h.processor.Process(m.Body)
	if err != nil {
		h.logger.Error("Failure processing message ", string(m.Body), ": ", err)
		// Returning a non-nil error will automatically send a REQ command to NSQ to re-queue a message.
		//TODO: handle errors that should not cause a reschedule
		return err
	}
	return nil
}

type MessageConsumer struct {
	consumer       *nsq.Consumer
	nsqLookupdHost string
	logger         Logger
	handler        *messageHandler
}

func (c *MessageConsumer) Start() error {
	// Use nsqlookupd to discover nsqd instances.
	// Could be a load balanced service, so use single connection.
	// It peridically calls nsqlookupd to refresh.
	return c.consumer.ConnectToNSQLookupd(c.nsqLookupdHost)
}
func (c *MessageConsumer) Stop() {
	c.consumer.Stop()
}

func New(config *MessageConsumerConfig, processor MessageProcessor, logger Logger) (*MessageConsumer, error) {
	NSQConsumerConfig := nsq.NewConfig()
	NSQConsumerConfig.MaxInFlight = config.Prefetch
	NSQConsumerConfig.MaxAttempts = config.Attempts
	consumer, err := nsq.NewConsumer(config.Topic, config.Channel, NSQConsumerConfig)
	if err != nil {
		return nil, err
	}
	// consumer.SetLogger(log, )
	handler := &messageHandler{
		processor,
		logger,
	}
	consumer.AddConcurrentHandlers(handler, config.Workers)

	return &MessageConsumer{consumer: consumer, nsqLookupdHost: config.NSQLookup, handler: handler, logger: logger}, nil
}
