package grpc

import (
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
)

// taken from https://github.com/grpc/grpc-go/blob/41e044e1c82fcf6a5801d6cbd7ecf952505eecb1/grpclog/loggerv2.go#L77-L86
const (
	// infoLog indicates Info severity.
	infoLog int = iota
	// warningLog indicates Warning severity.
	warningLog
	// errorLog indicates Error severity.
	errorLog
	// fatalLog indicates Fatal severity.
	fatalLog
)

// Logger implements google.golang.org/grpc/grpclog.LoggerV2 using hclog
type Logger struct {
	logger hclog.Logger
	exit   func()
}

// NewHCLogLogger returns a GRPC-compatible logger that wraps an hclog.Logger
func NewHCLogLogger(logger hclog.Logger) *Logger {
	return &Logger{logger: logger, exit: exit}
}

// Info logs to INFO log. Arguments are handled in the manner of fmt.Print.
func (l *Logger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Infoln logs to INFO log. Arguments are handled in the manner of fmt.Println.
func (l *Logger) Infoln(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Infof logs to INFO log. Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Warning logs to WARNING log. Arguments are handled in the manner of fmt.Print.
func (l *Logger) Warning(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Warningln logs to WARNING log. Arguments are handled in the manner of fmt.Println.
func (l *Logger) Warningln(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Warningf logs to WARNING log. Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Warningf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

// Error logs to ERROR log. Arguments are handled in the manner of fmt.Print.
func (l *Logger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// Errorln logs to ERROR log. Arguments are handled in the manner of fmt.Println.
func (l *Logger) Errorln(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// Errorf logs to ERROR log. Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

// Fatal logs to ERROR log. Arguments are handled in the manner of fmt.Print.
// gRPC ensures that all Fatal logs will exit with os.Exit(1).
// Implementations may also call os.Exit() with a non-zero exit code.
func (l *Logger) Fatal(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
	l.exit()
}

// Fatalln logs to ERROR log. Arguments are handled in the manner of fmt.Println.
// gRPC ensures that all Fatal logs will exit with os.Exit(1).
// Implementations may also call os.Exit() with a non-zero exit code.
func (l *Logger) Fatalln(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
	l.exit()
}

// Fatalf logs to ERROR log. Arguments are handled in the manner of fmt.Printf.
// gRPC ensures that all Fatal logs will exit with os.Exit(1).
// Implementations may also call os.Exit() with a non-zero exit code.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
	l.exit()
}

// V reports whether verbosity level l is at least the requested verbose level.
func (l *Logger) V(level int) bool {
	// we could do some math to make the verbosity levels match up, but
	// this makes things more explicit
	switch level {
	case infoLog:
		return l.logger.IsInfo() || l.logger.IsDebug() || l.logger.IsTrace()
	case warningLog:
		return l.logger.IsWarn() || l.logger.IsInfo() || l.logger.IsDebug() || l.logger.IsTrace()
	case errorLog:
		return l.logger.IsError() || l.logger.IsWarn() || l.logger.IsInfo() || l.logger.IsDebug() || l.logger.IsTrace()
	case fatalLog:
		// we don't have a fatal level in hclog
		return false
	default:
		return false
	}
}

func exit() {
	os.Exit(1)
}
