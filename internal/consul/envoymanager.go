package consul

import (
	"context"
	"errors"
	"os/exec"

	"github.com/hashicorp/go-hclog"
)

type EnvoyManager struct {
	logger  hclog.Logger
	command string
}

func NewEnvoyManager(logger hclog.Logger, command string) *EnvoyManager {
	// fmt.Sprintf("while true; do printf 'HTTP/1.1 200 OK\n\nOK' | nc -l %s; done", port)
	return &EnvoyManager{
		logger:  logger,
		command: command,
	}
}

func (m *EnvoyManager) Run(ctx context.Context) error {
	err := exec.CommandContext(ctx, "sh", "-c", m.command).Run()
	if errors.Is(err, context.Canceled) {
		// we intentionally canceled the context, just return
		return nil
	}
	return nil
}
