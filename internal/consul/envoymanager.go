package consul

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-hclog"
)

type EnvoyManager struct {
	logger hclog.Logger
	ports  []int
}

func NewEnvoyManager(logger hclog.Logger, ports []int) *EnvoyManager {
	// fmt.Sprintf("while true; do printf 'HTTP/1.1 200 OK\n\nOK' | nc -l %s; done", port)
	return &EnvoyManager{
		logger: logger,
		ports:  ports,
	}
}

func (m *EnvoyManager) Run(ctx context.Context) error {
	cmd := m.command()
	err := exec.CommandContext(ctx, "sh", "-c", cmd).Run()
	if errors.Is(err, context.Canceled) {
		// we intentionally canceled the context, just return
		return nil
	}
	return nil
}

func (m *EnvoyManager) command() string {
	// replace all this junk with something that actually uses envoy
	template := "sh -c \"while true; do printf 'HTTP/1.1 200 OK\nConnection: close\nContent-Length: 3\n\nOK\n' | nc -l %d; done\" &"
	commands := []string{}
	for _, port := range m.ports {
		commands = append(commands, fmt.Sprintf(template, port))
	}
	commands = append(commands, "wait $(jobs -p)")
	return strings.Join(commands, "\n")
}
