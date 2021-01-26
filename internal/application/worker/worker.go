package worker

import (
	"os"
	"os/signal"
	"syscall"
)

type MessageConsumer interface {
	Start() error
	Stop()
}

type Worker struct {
	consumer MessageConsumer
	logger   Logger
}

func New(consumer MessageConsumer, logger Logger) *Worker {
	return &Worker{consumer: consumer, logger: logger}
}

// Start launches worker
func (w *Worker) Start() error {
	// TODO: error handling
	if err := w.consumer.Start(); err != nil {
		w.logger.Error("Failure starting consumer: ", err)
		return err
	}
	w.logger.Info("Started consumer")
	// Kill signal handling
	done := make(chan struct{})
	signalChan := make(chan os.Signal, 1)
	go func() {
		<-signalChan
		close(done)
	}()
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	w.logger.Info("Started worker, terminate with 'kill <pid>'")
	<-done
	// Block, wait for signal above, make it stop if terminating
	return w.Stop()
}

func (w *Worker) Stop() error {
	w.consumer.Stop()
	w.logger.Info("Stopped consumer")
	return nil
}
