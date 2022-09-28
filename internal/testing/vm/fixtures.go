package vm

import (
	"context"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	generate bool
)

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

type Fixture struct {
	Name       string
	ExitCode   int
	InputPath  string
	OutputPath string
}

type FixturesConfig struct {
	Command func(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command
	Args    func(fixture Fixture) []string
	Setup   func(controller *Controller)
}

func TestFixtures(t *testing.T, config FixturesConfig) {
	t.Helper()

	controller := TestController(t)

	if config.Setup != nil {
		config.Setup(controller)
	}

	for _, fixture := range getFixtures(t) {
		fixture := fixture

		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			args := []string{}
			if config.Args != nil {
				args = config.Args(fixture)
			}

			controller.RunCLI(t, CLITest{
				Command:    config.Command,
				ExitStatus: fixture.ExitCode,
				Args:       args,
				OutputCheck: func(t *testing.T, output string) {
					if generate {
						require.NoError(t, os.WriteFile(fixture.OutputPath, []byte(output), 0644))
					}
					data, err := os.ReadFile(fixture.OutputPath)
					require.NoError(t, err)

					expected := string(data)
					assert.Equal(t, expected, output)
				},
			})
		})
	}
}

func getFixtures(t *testing.T) []Fixture {
	t.Helper()

	files, err := os.ReadDir(path.Join("fixtures"))
	require.NoError(t, err)

	fixtures := []Fixture{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name(), ".json") {
			name := strings.TrimSuffix(file.Name(), ".json")

			exitCode := 1
			if strings.Contains(name, "success") {
				exitCode = 0
			}

			fixtures = append(fixtures, Fixture{
				Name:       name,
				ExitCode:   exitCode,
				InputPath:  path.Join("fixtures", file.Name()),
				OutputPath: path.Join("fixtures", name+"-output"),
			})
		}
	}
	return fixtures
}
