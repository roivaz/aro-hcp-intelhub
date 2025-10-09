package logging

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// Logger provides a thin wrapper around logr.Logger with convenience helpers.
type Logger struct {
	log logr.Logger
}

// New returns a Logger based on the provided logr.Logger. When the base logger
// is uninitialized it falls back to the module default.
func New(base logr.Logger) Logger {
	if base.GetSink() == nil {
		base = DefaultLogger()
	}
	return Logger{log: base}
}

// DefaultLogger returns the module's default structured logger.
func DefaultLogger() logr.Logger {
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		zapLogger = zap.NewNop()
	}
	return zapr.NewLogger(zapLogger)
}

// WithValues returns a new Logger with additional key-value pairs attached.
func (l Logger) WithValues(keysAndValues ...any) Logger {
	return Logger{log: l.log.WithValues(keysAndValues...)}
}

// WithName scopes the logger with the supplied name.
func (l Logger) WithName(name string) Logger {
	return Logger{log: l.log.WithName(name)}
}

// Info logs an informational message.
func (l Logger) Info(msg string, keysAndValues ...any) {
	l.log.Info(msg, keysAndValues...)
}

// Debug logs a verbose message when V(1) is enabled on the underlying logger.
func (l Logger) Debug(msg string, keysAndValues ...any) {
	if l.log.V(1).Enabled() {
		l.log.V(1).Info(msg, keysAndValues...)
	}
}

// Error logs an error message.
func (l Logger) Error(err error, msg string, keysAndValues ...any) {
	l.log.Error(err, msg, keysAndValues...)
}

// Logr exposes the underlying logr.Logger.
func (l Logger) Logr() logr.Logger {
	return l.log
}
