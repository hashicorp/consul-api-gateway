package gateways

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	commonCLI "github.com/hashicorp/consul-api-gateway/internal/cli"
	"github.com/mitchellh/cli"
)

type PutCommand struct {
	*commonCLI.ClientCLI
}

func NewPutCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command {
	return &PutCommand{
		ClientCLI: commonCLI.NewClientCLI(ctx, putHelp, putSynopsis, ui, logOutput, "delete"),
	}
}

func (c *PutCommand) Run(args []string) int {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	file := c.Flags.Arg(0)
	if file == "" {
		return c.Error("parsing arguments", errors.New("a file parameter must be provided"))
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return c.Error("reading gateway definition file", err)
	}

	gateway := v1.Gateway{}
	if err := json.Unmarshal(data, &gateway); err != nil {
		return c.Error("parsing gateway definition file", err)
	}

	client, err := c.CreateClient()
	if err != nil {
		return c.Error("creating the client", err)
	}

	if _, err := client.V1().CreateGateway(c.Context(), gateway); err != nil {
		return c.Error("sending the request", err)
	}

	return c.Success("Successfully created Gateway")
}

const (
	putSynopsis = "Creates or updates a Gateway"
	putHelp     = `
Usage: consul-api-gateway gateways put [options] FILE

  Creates or updates a Gateway based off of the payload specified in FILE.

  Additional flags and more advanced use cases are detailed below.
`
)
