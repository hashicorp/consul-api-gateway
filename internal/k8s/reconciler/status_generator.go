//+build generate

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/yaml"
)

//go:generate sh -c "go run status_generator.go && go fmt zz_generated_status.go"

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
	Name           string
	Description    string
	Message        string
	Support        string
	StatusOverride statusOverride `yaml:"status"`
	IsString       bool           `yaml:"is_string"`
}

type conditionType struct {
	Name        string
	Description string
	Required    bool
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

func init() {
	file, err := os.OpenFile("statuses.yaml", os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		panic(err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(file, int(stat.Size()))
	err = decoder.Decode(&statuses)
	if err != nil {
		panic(err)
	}
	for i, status := range statuses {
		statuses[i] = status.normalize()
	}

	generator = template.Must(template.New("templated").Funcs(template.FuncMap{
		"writeComment": writeComment,
	}).Parse(generatorTemplate))
}

var (
	generator *template.Template
	statuses  []status
)

const generatorTemplate = `package reconciler

// GENERATED, DO NOT EDIT DIRECTLY

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

{{ range $status := $ }}{{ range $conditionType := $status.Types -}}
{{- if (ne $conditionType.Description "") }}{{ writeComment (print $status.Kind $conditionType.Name "Status") $conditionType.Description $conditionType.Support }}{{ end }}
type {{ $status.Kind }}{{ $conditionType.Name }}Status struct {
	{{- range $error := $conditionType.Errors }}
	{{ if (ne $error.Description "") }}{{ writeComment "" $error.Description $error.Support }}{{ end }}
	{{ $error.Name }} {{ if $error.IsString }}string{{else}}error{{end}}{{ end }}
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
	if s.{{ $error.Name }} != nil {
		return meta.Condition{
			Type:               {{ $status.Kind }}Condition{{ $conditionType.Name }},
			Status:             meta.Condition{{ if $error.StatusOverride.Override }}{{ if $error.StatusOverride.Value }}True{{else}}False{{end}}{{else}}{{ if $conditionType.Invert }}True{{ else }}False{{ end }}{{end}},
			Reason:             {{ $status.Kind }}ConditionReason{{ $error.Name }},
			Message:            {{ if $error.IsString }}s.{{ $error.Name }}{{else}}s.{{ $error.Name }}.Error(){{end}},
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}
	{{ end }}
	return meta.Condition{
		Type:               {{ $status.Kind }}Condition{{ $conditionType.Name }},
		Status:             meta.Condition{{ if $conditionType.Base.StatusOverride.Override }}{{ if $conditionType.Base.StatusOverride.Value }}True{{else}}False{{end}}{{else}}{{ if $conditionType.Invert }}False{{ else }}True{{ end }}{{end}},
		Reason:             {{ $status.Kind }}ConditionReason{{ $conditionType.Base.Name }},
		Message:            "{{ $conditionType.Base.Message }}",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

{{ writeComment "" (print "HasError returns whether any of the " $status.Kind $conditionType.Name "Status errors are set.") }}
func (s {{ $status.Kind}}{{ $conditionType.Name }}Status) HasError() bool {
	return {{ range $index, $error := $conditionType.Errors }}{{ if (ne $index 0) }} || {{ end }}s.{{$error.Name}} != {{ if $error.IsString }}""{{else}}nil{{end}}{{end}}
}
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
	if {{ range $index, $conditionType := $status.Types }}{{if $conditionType.Required}}{{ if (ne $index 0) }} || {{ end }}s.{{ $conditionType.Name }}.HasError(){{end}}{{end}} {
		return false
	}
	return true
}
{{- end}}
{{ end }}
`

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
	if err := generator.Execute(&buffer, statuses); err != nil {
		panic(err)
	}
	if err := os.WriteFile("zz_generated_status.go", buffer.Bytes(), 0644); err != nil {
		panic(err)
	}
}
