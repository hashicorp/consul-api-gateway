//go:build generate
// +build generate

package main

/*
This file generates boilerplate error structures and k8s object statuses to
facillitate status construction
*/

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/yaml"
)

//go:generate sh -c "go run generator.go && go fmt zz_generated_status.go zz_generated_status_test.go zz_generated_errors.go zz_generated_errors_test.go"

type customError struct {
	Name  string
	Types []string
}

type status struct {
	Kind        string
	Description string
	Validation  bool
	Types       []conditionType
}

func (s status) normalize() status {
	for i, conditionType := range s.Types {
		s.Types[i] = conditionType.normalize()
	}
	return s
}

type statusOverride struct {
	Override bool
	Value    bool
}

type reasonType struct {
	Name        string
	Description string
	Message     string
	Support     string
	Status      statusOverride
	String      bool
}

type conditionType struct {
	Name        string
	Description string
	Required    bool
	Ignore      bool
	Invert      bool
	Base        reasonType
	Support     string
	Errors      []reasonType
}

func (c conditionType) normalize() conditionType {
	if c.Support == "" {
		c.Support = "spec"
	}
	if c.Base.Name == "" {
		c.Base.Name = c.Name
	}
	if c.Base.Message == "" {
		c.Base.Message = c.Base.Name
	}
	if c.Base.Support == "" {
		c.Base.Support = c.Support
	}
	for i, err := range c.Errors {
		if err.Support == "" {
			err.Support = c.Support
		}
		c.Errors[i] = err
	}
	return c
}

func mustDecodeYAML(name string, into interface{}) {
	file, err := os.OpenFile(path.Join("config", name), os.O_RDONLY, 0644)
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
	mustDecodeYAML("statuses.yaml", &statuses)
	mustDecodeYAML("errors.yaml", &errors)

	for i, status := range statuses {
		statuses[i] = status.normalize()
	}

	errorGenerator = template.Must(template.New("errors").Parse(errorTemplate))
	errorTestGenerator = template.Must(template.New("errorTests").Parse(errorTestsTemplate))
	statusGenerator = template.Must(template.New("statuses").Funcs(template.FuncMap{
		"writeComment": writeComment,
		"required":     required,
	}).Parse(statusTemplate))
	statusTestGenerator = template.Must(template.New("statusTests").Funcs(template.FuncMap{
		"required": required,
	}).Parse(statusTestsTemplate))
}

var (
	errorGenerator      *template.Template
	errorTestGenerator  *template.Template
	statusGenerator     *template.Template
	statusTestGenerator *template.Template
	statuses            []status
	errors              []customError
)

const (
	errorTemplate = `package reconciler

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

{{ range $error := $ -}}
type {{ $error.Name }}ErrorType int

const (
	{{- range $index, $value := $error.Types }}
	{{ $error.Name }}ErrorType{{ $value }}{{ if (eq $index 0) }} {{ $error.Name }}ErrorType = iota{{end}}{{end}}
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
	errorTestsTemplate = `package reconciler

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
	statusTestsTemplate = `package reconciler

// GENERATED from statuses.yaml, DO NOT EDIT DIRECTLY

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

{{ range $status := $ }}{{ range $conditionType := $status.Types -}}
func Test{{ $status.Kind }}{{ $conditionType.Name }}Status(t *testing.T) {
	t.Parallel()

	var status {{ $status.Kind }}{{ $conditionType.Name }}Status

	expected := errors.New("expected")

	status = {{ $status.Kind }}{{ $conditionType.Name }}Status{}
	require.Equal(t, "{{ $conditionType.Base.Message }}", status.Condition(0).Message)
	require.Equal(t, {{ $status.Kind }}ConditionReason{{ $conditionType.Base.Name }}, status.Condition(0).Reason)
	{{ if not $conditionType.Ignore }}require.False(t, status.HasError()){{end}}

	{{ range $error := $conditionType.Errors }}
	status = {{ $status.Kind }}{{ $conditionType.Name }}Status{ {{ $error.Name }}: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, {{ $status.Kind }}ConditionReason{{ $error.Name }}, status.Condition(0).Reason)
	{{ if not $conditionType.Ignore }}require.True(t, status.HasError()){{end}}
{{ end }}}

{{end}}

func Test{{ $status.Kind }}Status(t *testing.T) {
	t.Parallel()

	status := {{ $status.Kind }}Status{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	{{ range $index, $conditionType := $status.Types }}
	conditionType = {{ $status.Kind }}Condition{{ $conditionType.Name }}
	reason = {{ $status.Kind }}ConditionReason{{ $conditionType.Base.Name }} 
	require.Equal(t, conditionType, conditions[{{ $index }}].Type)
	require.Equal(t, reason, conditions[{{ $index }}].Reason)
	{{end}}
	{{- if $status.Validation }}

	require.True(t, status.Valid())

	validationError := errors.New("error")

	{{ range $conditionType := (required $status.Types) }}
	{{ range $error := $conditionType.Errors }}
	status = {{ $status.Kind }}Status{}
	status.{{ $conditionType.Name }}.{{$error.Name}} = validationError
	require.False(t, status.Valid())
{{end}}{{end}}{{end}}}

{{end}}
`
	statusTemplate = `package reconciler

// GENERATED from statuses.yaml, DO NOT EDIT DIRECTLY

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

{{ range $status := $ }}{{ range $conditionType := $status.Types -}}
{{- if (ne $conditionType.Description "") }}{{ writeComment (print $status.Kind $conditionType.Name "Status") $conditionType.Description $conditionType.Support }}{{ end }}
type {{ $status.Kind }}{{ $conditionType.Name }}Status struct {
	{{- range $error := $conditionType.Errors }}
	{{ if (ne $error.Description "") }}{{ writeComment "" $error.Description $error.Support }}{{ end }}
	{{ $error.Name }} {{ if $error.String }}string{{else}}error{{end}}{{ end }}
}

const (
	{{ if (ne $conditionType.Description "") }}{{ writeComment (print $status.Kind "Condition" $conditionType.Name) $conditionType.Description $conditionType.Support }}{{ end }}
	{{ $status.Kind }}Condition{{ $conditionType.Name }} = "{{ $conditionType.Name }}"
	{{ if (ne $conditionType.Base.Description "") }}{{ writeComment (print $status.Kind "ConditionReason" $conditionType.Base.Name) $conditionType.Base.Description $conditionType.Base.Support }}{{ end }}
	{{ $status.Kind }}ConditionReason{{ $conditionType.Base.Name }} = "{{ $conditionType.Base.Name }}"
	{{- range $error := $conditionType.Errors }}
	{{ if (ne $error.Description "") }}{{ writeComment (print $status.Kind "ConditionReason" $error.Name) $error.Description $error.Support }}{{ end }}
	{{ $status.Kind }}ConditionReason{{ $error.Name }} = "{{ $error.Name }}"{{ end }}
)

{{ writeComment "" (print "Condition returns the status condition of the " $status.Kind $conditionType.Name "Status based off of the underlying errors that are set.") }}
func (s {{ $status.Kind}}{{ $conditionType.Name }}Status) Condition(generation int64) meta.Condition {
	{{- range $error := $conditionType.Errors }}
	if s.{{ $error.Name }} != {{ if $error.String }}""{{else}}nil{{end}} {
		return meta.Condition{
			Type:               {{ $status.Kind }}Condition{{ $conditionType.Name }},
			Status:             meta.Condition{{ if $error.Status.Override }}{{ if $error.Status.Value }}True{{else}}False{{end}}{{else}}{{ if $conditionType.Invert }}True{{ else }}False{{ end }}{{end}},
			Reason:             {{ $status.Kind }}ConditionReason{{ $error.Name }},
			Message:            {{ if $error.String }}s.{{ $error.Name }}{{else}}s.{{ $error.Name }}.Error(){{end}},
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}
	{{ end }}
	return meta.Condition{
		Type:               {{ $status.Kind }}Condition{{ $conditionType.Name }},
		Status:             meta.Condition{{ if $conditionType.Base.Status.Override }}{{ if $conditionType.Base.Status.Value }}True{{else}}False{{end}}{{else}}{{ if $conditionType.Invert }}False{{ else }}True{{ end }}{{end}},
		Reason:             {{ $status.Kind }}ConditionReason{{ $conditionType.Base.Name }},
		Message:            "{{ $conditionType.Base.Message }}",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

{{ if not $conditionType.Ignore -}}
{{ writeComment "" (print "HasError returns whether any of the " $status.Kind $conditionType.Name "Status errors are set.") }}
func (s {{ $status.Kind}}{{ $conditionType.Name }}Status) HasError() bool {
	return {{ range $index, $error := $conditionType.Errors }}{{ if (ne $index 0) }} || {{ end }}s.{{$error.Name}} != {{ if $error.String }}""{{else}}nil{{end}}{{end}}
}
{{ end }}
{{ end }}
{{- if (ne $status.Description "") }}{{ writeComment (print $status.Kind "Status") $status.Description }}{{ end }}
type {{ $status.Kind}}Status struct {
	{{- range $conditionType := $status.Types }}
	{{ if (ne $conditionType.Description "") }}{{ writeComment "" $conditionType.Description }}{{ end }}
	{{ $conditionType.Name }} {{ $status.Kind}}{{ $conditionType.Name }}Status{{ end }}
}

{{ writeComment "" (print "Conditions returns the aggregated status conditions of the " $status.Kind "Status.") }}
func (s {{ $status.Kind}}Status) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		{{- range $conditionType := $status.Types }}
		s.{{ $conditionType.Name }}.Condition(generation),{{ end }}
	}
}

{{ if $status.Validation -}}
{{ writeComment "" (print "Valid returns whether all of the required conditions for the " $status.Kind "Status are satisfied.") }}
func (s {{ $status.Kind}}Status) Valid() bool {
	if {{ range $index, $conditionType := (required $status.Types) }}{{ if (ne $index 0) }} || {{ end }}s.{{ $conditionType.Name }}.HasError(){{end}} {
		return false
	}
	return true
}
{{- end}}
{{ end }}
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

func required(conditions []conditionType) []conditionType {
	filtered := []conditionType{}
	for _, condition := range conditions {
		if condition.Required {
			filtered = append(filtered, condition)
		}
	}
	return filtered
}

func main() {
	var buffer bytes.Buffer
	if err := statusGenerator.Execute(&buffer, statuses); err != nil {
		panic(err)
	}
	if err := os.WriteFile("zz_generated_status.go", buffer.Bytes(), 0644); err != nil {
		panic(err)
	}

	buffer.Reset()

	if err := statusTestGenerator.Execute(&buffer, statuses); err != nil {
		panic(err)
	}
	if err := os.WriteFile("zz_generated_status_test.go", buffer.Bytes(), 0644); err != nil {
		panic(err)
	}

	buffer.Reset()

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
