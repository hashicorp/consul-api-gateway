package log

import (
	"github.com/go-logr/logr"

	"github.com/hashicorp/go-hclog"
)

func FromHCLogger(log hclog.Logger) logr.Logger {
	return &logger{log}
}

type logger struct {
	hclog.Logger
}

func (l *logger) Enabled() bool {
	return true
}

func (l *logger) Error(err error, msg string, keys ...interface{}) {
	l.Logger.Error(msg, "error", err, keys)
}

func (l *logger) V(level int) logr.Logger {
	panic("implement me")
}

func (l *logger) WithValues(keysAndValues ...interface{}) logr.Logger {
	return &logger{l.With(keysAndValues)}
}

func (l *logger) WithName(name string) logr.Logger {
	return &logger{l.Named(name)}
}
