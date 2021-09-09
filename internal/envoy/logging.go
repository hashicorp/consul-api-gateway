package envoy

import (
	"fmt"

	"github.com/envoyproxy/go-control-plane/pkg/log"
	"github.com/hashicorp/go-hclog"
)

func logFunc(log func(msg string, args ...interface{})) func(msg string, args ...interface{}) {
	return func(msg string, args ...interface{}) {
		log(fmt.Sprintf(msg, args...))
	}
}
func wrapEnvoyLogger(logger hclog.Logger) log.Logger {
	return log.LoggerFuncs{
		DebugFunc: logFunc(logger.Debug),
		InfoFunc:  logFunc(logger.Info),
		WarnFunc:  logFunc(logger.Warn),
		ErrorFunc: logFunc(logger.Error),
	}
}
