package httproutes

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

type PutCommand struct {
	*common.ClientCLI
}

func NewPutCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command {
	return &PutCommand{
		ClientCLI: common.NewClientCLI(ctx, putHelp, putSynopsis, ui, logOutput, "delete"),
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
		return c.Error("reading route definition file", err)
	}

	route := v1.HTTPRoute{}
	if err := json.Unmarshal(data, &route); err != nil {
		return c.Error("parsing route definition file", err)
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
	putSynopsis = "Creates or updates an HTTPRoute"
	putHelp     = `
Usage: consul-api-gateway http-routes put [options] FILE

  Creates or updates an HTTPRoute based off of the payload specified in FILE.

  Additional flags and more advanced use cases are detailed below.
`
)
