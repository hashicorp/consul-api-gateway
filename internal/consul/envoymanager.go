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
	ports  []NamedPort
}

func NewEnvoyManager(logger hclog.Logger, ports []NamedPort) *EnvoyManager {
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
	template := "sh -c \"while true; do printf 'HTTP/1.1 200 OK\nConnection: close\nContent-Length: %d\n\n%s\n' | nc -l %d; done\" &"
	commands := []string{}
	for _, port := range m.ports {
		message := fmt.Sprintf("Protocol: %s, Name: %s, Port: %d", port.Protocol, port.Name, port.Port)
		commands = append(commands, fmt.Sprintf(template, len(message)+1, message, port.Port))
	}
	commands = append(commands, "wait $(jobs -p)")
	return strings.Join(commands, "\n")
}
