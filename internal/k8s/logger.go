package k8s

import (
	"github.com/go-logr/logr"

	"github.com/hashicorp/go-hclog"
)

func fromHCLogger(log hclog.Logger) logr.Logger {
	return logr.New(&logger{log})
}

// logger is a LogSink that wraps hclog
type logger struct {
	hclog.Logger
}

// Verify that it actually implements the interface
var _ logr.LogSink = logger{}

func (l logger) Init(logr.RuntimeInfo) {
}

func (l logger) Enabled(_ int) bool {
	return true
}

func (l logger) Info(_ int, msg string, keysAndValues ...interface{}) {
	keysAndValues = append([]interface{}{"info", msg}, keysAndValues...)
	l.Logger.Info(msg, keysAndValues...)
}

func (l logger) Error(err error, msg string, keysAndValues ...interface{}) {
	keysAndValues = append([]interface{}{"error", err}, keysAndValues...)
	l.Logger.Error(msg, keysAndValues...)
}

func (l logger) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &logger{l.With(keysAndValues...)}
}

func (l logger) WithName(name string) logr.LogSink {
	return &logger{l.Named(name)}
}
