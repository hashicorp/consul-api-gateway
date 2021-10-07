//+build generate

package main

import (
	"bytes"
	"os"
	"text/template"
)

//go:generate sh -c "go run status_generator.go && go fmt zz_generated_status.go"

type status struct {
	Prefix         string
	ConditionTypes []conditionType
}

type conditionType struct {
	Name     string
	Required bool
	Invert   bool
	Value    string
	Base     string
	Errors   []string
}

// prefix -> conditionType -> reason
var statuses = []status{{
	Prefix: "Listener",
	ConditionTypes: []conditionType{{
		// TODO: RouteConflict
		"Conflicted", true, true, "ListenerConditionConflicted", "NoConflicts", []string{"HostnameConflict", "ProtocolConflict", "RouteConflict"},
	}, {
		// TODO: PortUnavailable, UnsupportedExtension
		"Detached", true, true, "ListenerConditionDetached", "Attached", []string{"PortUnvailable", "UnsupportedExtension", "UnsupportedProtocol", "UnsupportedAddress"},
	}, {
		// TODO: Pending
		"Ready", false, false, "ListenerConditionReady", "Ready", []string{"Invalid", "Pending"},
	}, {
		// TODO: RefNotPermitted
		"ResolvedRefs", true, false, "ListenerConditionResolvedRefs", "ResolvedRefs", []string{"InvalidCertificateRef", "InvalidRouteKinds", "RefNotPermitted"},
	}},
}}

func init() {
	generator = template.Must(template.New("templated").Parse(generatorTemplate))
}

var generator *template.Template

const generatorTemplate = `package reconciler

// GENERATED, DO NOT EDIT DIRECTLY

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

{{ range $status := $ }}{{ range $conditionType := $status.ConditionTypes -}}
type {{ $status.Prefix }}{{ $conditionType.Name }}Status struct {
	{{- range $error := $conditionType.Errors }}
	{{ $error }} error{{ end }}
}

const (
	ConditionReason{{ $conditionType.Base }} = "{{ $conditionType.Base }}"
	{{- range $error := $conditionType.Errors }}
	ConditionReason{{ $error }} = "{{ $error }}"{{ end }}
)

func (s {{ $status.Prefix}}{{ $conditionType.Name }}Status) Condition(generation int64) meta.Condition {
	{{- range $error := $conditionType.Errors }}
	if s.{{ $error }} != nil {
		return meta.Condition{
			Type:               string(gw.{{ $conditionType.Value }}),
			Status:             meta.Condition{{ if $conditionType.Invert }}True{{ else }}False{{ end }},
			Reason:             ConditionReason{{ $error }},
			Message:            s.{{ $error }}.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}
	{{ end }}
	return meta.Condition{
		Type:               string(gw.{{ $conditionType.Value }}),
		Status:             meta.Condition{{ if $conditionType.Invert }}False{{ else }}True{{ end }},
		Reason:             ConditionReason{{ $conditionType.Base }},
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

func (s {{ $status.Prefix}}{{ $conditionType.Name }}Status) HasError() bool {
	return {{ range $index, $error := $conditionType.Errors }}{{ if (ne $index 0) }} || {{ end }}s.{{$error}} != nil{{end}}
}
{{ end }}
type {{ $status.Prefix}}Status struct {
	{{- range $conditionType := $status.ConditionTypes }}
	{{ $conditionType.Name }} {{ $status.Prefix}}{{ $conditionType.Name }}Status{{ end }}
}

func (s {{ $status.Prefix}}Status) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		{{- range $conditionType := $status.ConditionTypes }}
		s.{{ $conditionType.Name }}.Condition(generation),{{ end }}
	}
}

func (s {{ $status.Prefix}}Status) Valid() bool {
	if {{ range $index, $conditionType := $status.ConditionTypes }}{{if $conditionType.Required}}{{ if (ne $index 0) }} || {{ end }}s.{{ $conditionType.Name }}.HasError(){{end}}{{end}} {
		return false
	}
	return true
}
{{ end }}
`

func main() {
	var buffer bytes.Buffer
	if err := generator.Execute(&buffer, statuses); err != nil {
		panic(err)
	}
	if err := os.WriteFile("zz_generated_status.go", buffer.Bytes(), 0644); err != nil {
		panic(err)
	}
}
