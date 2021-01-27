package tracing

import (
	"go.uber.org/zap"
)

// zapLogger is zap logger implementation of jaeger.Logger
// logger delegates all calls to the underlying zap.Logger
type zapLogger struct {
	logger *zap.SugaredLogger
}

// Info logs an info msg with fields
func (l zapLogger) Infof(msg string, args ...interface{}) {
	l.logger.Info(msg, args)
}

// Error logs an error msg with fields
func (l zapLogger) Error(msg string) {
	l.logger.Error(msg)
}

// Info logs an info msg with fields
func (l zapLogger) Debugf(msg string, args ...interface{}) {
	l.logger.Debug(msg, args)
}

func NewZapLogger(logger *zap.SugaredLogger) *zapLogger {
	return &zapLogger{logger: logger}
}
