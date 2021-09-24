package grpc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/go-hclog"
)

func TestLogging(t *testing.T) {
	var buffer bytes.Buffer
	log := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	logger := NewHCLogLogger(log)
	logger.exit = func() {}

	logger.Info("info")
	require.Contains(t, buffer.String(), "[INFO]")
	require.Contains(t, buffer.String(), "info")
	buffer.Reset()

	logger.Infof("infof")
	require.Contains(t, buffer.String(), "[INFO]")
	require.Contains(t, buffer.String(), "infof")
	buffer.Reset()

	logger.Infoln("infoln")
	require.Contains(t, buffer.String(), "[INFO]")
	require.Contains(t, buffer.String(), "infoln")
	buffer.Reset()

	logger.Warning("warning")
	require.Contains(t, buffer.String(), "[WARN]")
	require.Contains(t, buffer.String(), "warning")
	buffer.Reset()

	logger.Warningf("warningf")
	require.Contains(t, buffer.String(), "[WARN]")
	require.Contains(t, buffer.String(), "warningf")
	buffer.Reset()

	logger.Warningln("warningln")
	require.Contains(t, buffer.String(), "[WARN]")
	require.Contains(t, buffer.String(), "warningln")
	buffer.Reset()

	logger.Error("error")
	require.Contains(t, buffer.String(), "[ERROR]")
	require.Contains(t, buffer.String(), "error")
	buffer.Reset()

	logger.Errorf("errorf")
	require.Contains(t, buffer.String(), "[ERROR]")
	require.Contains(t, buffer.String(), "errorf")
	buffer.Reset()

	logger.Errorln("errorln")
	require.Contains(t, buffer.String(), "[ERROR]")
	require.Contains(t, buffer.String(), "errorln")
	buffer.Reset()

	logger.Fatal("fatal")
	require.Contains(t, buffer.String(), "[ERROR]")
	require.Contains(t, buffer.String(), "fatal")
	buffer.Reset()

	logger.Fatalf("fatalf")
	require.Contains(t, buffer.String(), "[ERROR]")
	require.Contains(t, buffer.String(), "fatalf")
	buffer.Reset()

	logger.Fatalln("fatalln")
	require.Contains(t, buffer.String(), "[ERROR]")
	require.Contains(t, buffer.String(), "fatalln")
	buffer.Reset()

	log.SetLevel(hclog.Error)
	require.False(t, logger.V(4))
	require.False(t, logger.V(3))
	require.True(t, logger.V(2))
	require.False(t, logger.V(1))
	require.False(t, logger.V(0))

	log.SetLevel(hclog.Warn)
	require.False(t, logger.V(4))
	require.False(t, logger.V(3))
	require.True(t, logger.V(2))
	require.True(t, logger.V(1))
	require.False(t, logger.V(0))

	log.SetLevel(hclog.Info)
	require.False(t, logger.V(4))
	require.False(t, logger.V(3))
	require.True(t, logger.V(2))
	require.True(t, logger.V(1))
	require.True(t, logger.V(0))

	log.SetLevel(hclog.Debug)
	require.False(t, logger.V(4))
	require.False(t, logger.V(3))
	require.True(t, logger.V(2))
	require.True(t, logger.V(1))
	require.True(t, logger.V(0))

	log.SetLevel(hclog.Trace)
	require.False(t, logger.V(4))
	require.False(t, logger.V(3))
	require.True(t, logger.V(2))
	require.True(t, logger.V(1))
	require.True(t, logger.V(0))
}
