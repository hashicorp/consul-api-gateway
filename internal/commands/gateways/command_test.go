package gateways

import (
	"os"
	"path"
	"strings"
	"testing"

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

type fixture struct {
	name       string
	exitCode   int
	inputPath  string
	outputPath string
}

func getFixtures(t *testing.T, directory string) []fixture {
	t.Helper()

	files, err := os.ReadDir(path.Join("fixtures", "put"))
	require.NoError(t, err)

	fixtures := []fixture{}
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

			fixtures = append(fixtures, fixture{
				name:       name,
				exitCode:   exitCode,
				inputPath:  path.Join("fixtures", directory, file.Name()),
				outputPath: path.Join("fixtures", directory, name+"-output"),
			})
		}
	}
	return fixtures
}
