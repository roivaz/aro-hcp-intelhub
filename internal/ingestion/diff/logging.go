package diff

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

type logger struct {
	log logr.Logger
}

func newLogger(base logr.Logger) logger {
	if base.GetSink() == nil {
		base = defaultLogger()
	}
	return logger{log: base}
}

func defaultLogger() logr.Logger {
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		zapLogger = zap.NewNop()
	}
	return zapr.NewLogger(zapLogger)
}

func (l logger) Info(msg string, keysAndValues ...any) {
	l.log.Info(msg, keysAndValues...)
}

func (l logger) Debug(msg string, keysAndValues ...any) {
	if l.log.V(1).Enabled() {
		l.log.V(1).Info(msg, keysAndValues...)
	}
}

func (l logger) Error(err error, msg string, keysAndValues ...any) {
	l.log.Error(err, msg, keysAndValues...)
}
