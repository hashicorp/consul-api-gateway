package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/consul-api-gateway/internal/api"
	"github.com/hashicorp/go-hclog"
	"github.com/kr/text"
	"github.com/mitchellh/cli"
)

type CommonCLI struct {
	UI       cli.Ui
	output   io.Writer
	ctx      context.Context
	help     string
	synopsis string

	// Logging
	flagLogLevel string
	flagLogJSON  bool

	Flags *flag.FlagSet
}

func NewCommonCLI(ctx context.Context, help, synopsis string, ui cli.Ui, logOutput io.Writer, name string) *CommonCLI {
	cli := &CommonCLI{UI: ui, synopsis: synopsis, output: logOutput, ctx: ctx, Flags: flag.NewFlagSet(name, flag.ContinueOnError)}
	cli.init()

	cli.help = FlagUsage(help, cli.Flags)

	return cli
}

func (c *CommonCLI) init() {
	c.Flags.StringVar(&c.flagLogLevel, "log-level", "info",
		`Log verbosity level. Supported values (in order of detail) are "trace", "debug", "info", "warn", and "error".`)
	c.Flags.BoolVar(&c.flagLogJSON, "log-json", false,
		"Enable or disable JSON output format for logging.")

	c.Flags.SetOutput(c.output)
}

func (c *CommonCLI) Context() context.Context {
	return c.ctx
}

func (c *CommonCLI) LogLevel() string {
	return c.flagLogLevel
}

func (c *CommonCLI) Output() io.Writer {
	return c.output
}

func (c *CommonCLI) Logger(name string) hclog.Logger {
	return CreateLogger(c.output, c.flagLogLevel, c.flagLogJSON, name)
}

func (c *CommonCLI) Parse(args []string) error {
	return c.Flags.Parse(args)
}

func (c *CommonCLI) Error(message string, err error) int {
	c.UI.Error("There was an error " + message + ":\n\t" + err.Error())
	return 1
}

func (c *CommonCLI) Success(message string) int {
	c.UI.Output(message)
	return 0
}

func (c *CommonCLI) Synopsis() string {
	return c.synopsis
}

func (c *CommonCLI) Help() string {
	return c.help
}

type ClientCLI struct {
	*CommonCLI

	flagAddress    string // Server address for requests
	flagPort       uint   // Server port for requests
	flagToken      string // Token for requests
	flagScheme     string // Server scheme for API
	flagCAFile     string // Server TLS CA file for TLS verification
	flagSkipVerify bool   // Skip certificate verification for client
}

func NewClientCLI(ctx context.Context, help, synopsis string, ui cli.Ui, logOutput io.Writer, name string) *ClientCLI {
	cli := &ClientCLI{
		CommonCLI: NewCommonCLI(ctx, help, synopsis, ui, logOutput, name),
	}
	cli.init()
	cli.help = FlagUsage(help, cli.Flags)

	return cli
}

func (c *ClientCLI) init() {
	c.Flags.StringVar(&c.flagToken, "consul-token", "", "Token to use for client.")
	c.Flags.StringVar(&c.flagAddress, "gateway-controller-address", "localhost", "Server address to use for client.")
	c.Flags.UintVar(&c.flagPort, "gateway-controller-port", 5605, "Server port to use for client.")
	c.Flags.StringVar(&c.flagScheme, "gateway-controller-scheme", "http", "Server scheme to use for client.")
	c.Flags.StringVar(&c.flagCAFile, "gateway-controller-ca-file", "", "Path to CA file for verifying server TLS certificate.")
	c.Flags.BoolVar(&c.flagSkipVerify, "gateway-controller-skip-verify", false, "Skip certificate verification for TLS connection.")
}

func (c *ClientCLI) CreateClient() (*api.Client, error) {
	var tlsConfig *api.TLSConfiguration
	if c.flagScheme == "https" {
		tlsConfig = &api.TLSConfiguration{
			CAFile:           c.flagCAFile,
			SkipVerification: c.flagSkipVerify,
		}
	}

	return api.CreateClient(api.ClientConfig{
		Address:          c.flagAddress,
		Port:             c.flagPort,
		Token:            GetConsulTokenOr(c.flagToken),
		TLSConfiguration: tlsConfig,
	})
}

type ClientCLIWithNamespace struct {
	*ClientCLI

	flagNamespace string // Namespace to pass in client requests
}

func NewClientCLIWithNamespace(ctx context.Context, help, synopsis string, ui cli.Ui, logOutput io.Writer, name string) *ClientCLIWithNamespace {
	cli := &ClientCLIWithNamespace{ClientCLI: NewClientCLI(ctx, help, synopsis, ui, logOutput, name)}
	cli.Flags.StringVar(&cli.flagNamespace, "namespace", "", "Namespace to pass in client requests.")
	cli.help = FlagUsage(help, cli.Flags)

	return cli
}

func (c *ClientCLIWithNamespace) Namespace() string {
	if c.flagNamespace == "" {
		return "default"
	}
	return c.flagNamespace
}

func LogAndDie(logger hclog.Logger, message string, err error) int {
	logger.Error("error "+message, "error", err)
	return 1
}

func LogSuccess(logger hclog.Logger, message string) int {
	logger.Info(message)
	return 0
}

func FlagUsage(usage string, flags *flag.FlagSet) string {
	out := new(bytes.Buffer)
	out.WriteString(strings.TrimSpace(usage))
	out.WriteString("\n")
	out.WriteString("\n")

	printTitle(out, "Command Options")
	flags.VisitAll(func(f *flag.Flag) {
		printFlag(out, f)
	})

	return strings.TrimRight(out.String(), "\n")
}

// printTitle prints a consistently-formatted title to the given writer.
func printTitle(w io.Writer, s string) {
	fmt.Fprintf(w, "%s\n\n", s)
}

// printFlag prints a single flag to the given writer.
func printFlag(w io.Writer, f *flag.Flag) {
	example, _ := flag.UnquoteUsage(f)
	if example != "" {
		fmt.Fprintf(w, "  -%s=<%s>\n", f.Name, example)
	} else {
		fmt.Fprintf(w, "  -%s\n", f.Name)
	}

	indented := wrapAtLength(f.Usage, 5)
	fmt.Fprintf(w, "%s\n\n", indented)
}

// contains returns true if the given flag is contained in the given flag
// set or false otherwise.
func contains(fs *flag.FlagSet, f *flag.Flag) bool {
	if fs == nil {
		return false
	}

	var in bool
	fs.VisitAll(func(hf *flag.Flag) {
		in = in || f.Name == hf.Name
	})
	return in
}

// maxLineLength is the maximum width of any line.
const maxLineLength int = 72

// wrapAtLength wraps the given text at the maxLineLength, taking into account
// any provided left padding.
func wrapAtLength(s string, pad int) string {
	wrapped := text.Wrap(s, maxLineLength-pad)
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", pad) + line
	}
	return strings.Join(lines, "\n")
}
