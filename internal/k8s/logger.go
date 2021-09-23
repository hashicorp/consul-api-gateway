package k8s

import (
	"github.com/go-logr/logr"

	"github.com/hashicorp/go-hclog"
)

func fromHCLogger(log hclog.Logger) logr.Logger {
	return &logger{log}
}

type logger struct {
	hclog.Logger
}

func (l *logger) Enabled() bool {
	return true
}

func (l *logger) Error(err error, msg string, keysAndValues ...interface{}) {
	keysAndValues = append([]interface{}{"error", err}, keysAndValues...)
	l.Logger.Error(msg, keysAndValues...)
}

func (l *logger) V(_ int) logr.Logger {
	return l
}

func (l *logger) WithValues(keysAndValues ...interface{}) logr.Logger {
	return &logger{l.With(keysAndValues...)}
}

func (l *logger) WithName(name string) logr.Logger {
	return &logger{l.Named(name)}
}
