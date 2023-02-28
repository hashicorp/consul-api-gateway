// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build generate
// +build generate

package main

/*
This file generates boilerplate error structures
*/

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/yaml"
)

//go:generate sh -c "go run generator.go && go fmt zz_generated_errors.go zz_generated_errors_test.go"

type customError struct {
	Name  string
	Types []string
}

func mustDecodeYAML(name string, into interface{}) {
	file, err := os.OpenFile(name, os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		panic(err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(file, int(stat.Size()))
	err = decoder.Decode(into)
	if err != nil {
		panic(err)
	}
}

func init() {
	mustDecodeYAML("errors.yaml", &errors)
	errorGenerator = template.Must(template.New("errors").Parse(errorTemplate))
	errorTestGenerator = template.Must(template.New("errorTests").Parse(errorTestsTemplate))
}

var (
	errorGenerator     *template.Template
	errorTestGenerator *template.Template
	errors             []customError
)

const (
	errorTemplate = `// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package errors

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

{{ range $error := $ -}}
type {{ $error.Name }}ErrorType string

const (
	{{- range $index, $value := $error.Types }}
	{{ $error.Name }}ErrorType{{ $value }} {{ $error.Name }}ErrorType = "{{ $value }}Error"{{end}}
)
	
type {{ $error.Name }}Error struct {
	inner string
	errorType {{ $error.Name }}ErrorType
}

{{ range $index, $value := $error.Types -}}
func New{{ $error.Name }}Error{{ $value }}(inner string) {{$error.Name}}Error {
	return {{ $error.Name }}Error{inner, {{ $error.Name }}ErrorType{{ $value }}}
}
{{end}}
	
func (r {{ $error.Name }}Error) Error() string {
	return r.inner
}
	
func (r {{ $error.Name }}Error) Kind() {{ $error.Name }}ErrorType {
	return r.errorType
}	
{{end}}
`
	errorTestsTemplate = `// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package errors

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

import (
	"testing"

	"github.com/stretchr/testify/require"
)
{{ range $error := $ }}

func Test{{ $error.Name }}ErrorType(t *testing.T) {
	t.Parallel()

	expected := "expected"

	{{ range $value := $error.Types -}}
	require.Equal(t, expected, New{{ $error.Name }}Error{{ $value }}(expected).Error())
	require.Equal(t, {{ $error.Name }}ErrorType{{ $value }}, New{{ $error.Name }}Error{{ $value }}(expected).Kind())
{{end}}}
{{end}}
`
)

const (
	lineLength = 77
)

func wrapLine(line string) []string {
	if len(line) <= lineLength {
		return []string{line}
	}
	tokens := strings.Split(line, " ")
	lines := []string{}
	currentLine := ""
	for _, token := range tokens {
		appendedLength := len(token)
		if currentLine != "" {
			appendedLength++
		}
		newLength := appendedLength + len(currentLine)
		if newLength > lineLength {
			lines = append(lines, currentLine)
			currentLine = ""
		}
		if currentLine == "" {
			currentLine = token
			continue
		}
		currentLine = currentLine + " " + token
	}
	return append(lines, currentLine)
}

func writeComment(name, comment string, support ...string) string {
	comment = strings.TrimSpace(comment)
	lines := strings.Split(comment, "\n")
	wrappedLines := []string{}
	for i, line := range lines {
		if i == 0 && name != "" {
			line = name + " - " + line
		}
		if i != 0 {
			wrappedLines = append(wrappedLines, "")
		}
		wrappedLines = append(wrappedLines, wrapLine(line)...)
	}
	if len(support) != 0 {
		wrappedLines = append(wrappedLines, "", fmt.Sprintf("[%s]", strings.Join(support, ", ")))
	}
	for i, line := range wrappedLines {
		wrappedLines[i] = "// " + line
	}
	return strings.Join(wrappedLines, "\n")
}

func main() {
	var buffer bytes.Buffer

	if err := errorGenerator.Execute(&buffer, errors); err != nil {
		panic(err)
	}
	if err := os.WriteFile("zz_generated_errors.go", buffer.Bytes(), 0644); err != nil {
		panic(err)
	}

	buffer.Reset()

	if err := errorTestGenerator.Execute(&buffer, errors); err != nil {
		panic(err)
	}
	if err := os.WriteFile("zz_generated_errors_test.go", buffer.Bytes(), 0644); err != nil {
		panic(err)
	}
}
