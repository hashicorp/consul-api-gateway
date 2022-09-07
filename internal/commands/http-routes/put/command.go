package put

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/mitchellh/cli"
)

type Command struct {
	*common.ClientCLI
}

func New(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	return &Command{
		ClientCLI: common.NewClientCLI(ctx, help, synopsis, ui, logOutput, "delete"),
	}
}

func (c *Command) Run(args []string) int {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	file := c.Flags.Arg(0)
	if file == "" {
		return c.Error("parsing arguments", errors.New("a file parameter must be provided"))
	}
	route := v1.HTTPRoute{}
	data, err := os.ReadFile(file)
	if err != nil {
		return c.Error("reading route definition file", err)
	}
	if err := json.Unmarshal(data, &route); err != nil {
		return c.Error("unmarshaling route definition file", err)
	}

	client, err := c.CreateClient()
	if err != nil {
		return c.Error("creating the client", err)
	}
	if _, err := client.V1().CreateHTTPRoute(c.Context(), route); err != nil {
		return c.Error("sending the request", err)
	}
	return c.Success("Successfully created route")
}

const (
	synopsis = "Creates or updates an HTTPRoute"
	help     = `
Usage: consul-api-gateway http-routes put [options] FILE

  Creates or updates an HTTPRoute based off of the payload specified in FILE.

  Additional flags and more advanced use cases are detailed below.
`
)
