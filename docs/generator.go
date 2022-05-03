//go:build generate
// +build generate

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

var (
	docTpl *template.Template
)

func init() {
	docTpl = template.Must(template.New("doc").Parse(docTemplate))
}

type DocItem struct {
	Description string     `yaml:"description"`
	Fields      []DocField `yaml:"fields"`
}

type DocField struct {
	Name        string     `yaml:"name"`
	Type        string     `yaml:"type"`
	Description string     `yaml:"description"`
	Enum        []string   `yaml:"enum"`
	Fields      []DocField `yaml:"fields"`
	Items       DocItem    `yaml:"items"`
}

func RenderField(field DocField) string {
	docString := fmt.Sprintf("- `%s` - (type: `%s`): %s", field.Name, field.Type, strings.Replace(field.Description, "\n", "", -1))

	fields := []string{}
	for _, field := range field.Fields {
		fields = append(fields, "\t"+RenderField(field))
	}
	for _, field := range field.Items.Fields {
		fields = append(fields, "\t"+RenderField(field))
	}
	return strings.Join(append([]string{docString}, fields...), "\n")
}

const docTemplate = `
## {{ .Kind }} (version: {{ .Version }}, scope: {{ .Scope }})

{{ .Description }}

### Fields
`

type Doc struct {
	Kind        string     `yaml:"string"`
	Description string     `yaml:"description"`
	Version     string     `yaml:"version"`
	Scope       string     `yaml:"scope"`
	Fields      []DocField `yaml:"fields"`
}

func RenderDoc(doc Doc) (string, error) {
	var docString bytes.Buffer
	if err := docTpl.Execute(&docString, doc); err != nil {
		return "", err
	}
	fields := []string{}
	for _, field := range doc.Fields {
		fields = append(fields, RenderField(field))
	}
	return docString.String() + strings.Join(fields, "\n"), nil
}

//go:generate sh -c "go run generator.go -file gateway.yaml -out gateway.md"
//go:generate sh -c "go run generator.go -file gatewayclass.yaml -out gatewayclass.md"
//go:generate sh -c "go run generator.go -file gatewayclassconfig.yaml -out gatewayclassconfig.md"
//go:generate sh -c "go run generator.go -file http-route.yaml -out http-route.md"
//go:generate sh -c "go run generator.go -file meshservice.yaml -out meshservice.md"
//go:generate sh -c "go run generator.go -file referencepolicy.yaml -out referencepolicy.md"
//go:generate sh -c "go run generator.go -file tcp-route.yaml -out tcp-route.md"

func main() {
	var file string
	var out string

	flag.StringVar(&file, "file", "", "name of the file to use for generating docs")
	flag.StringVar(&out, "out", "", "name of the output file for generated docs")

	flag.Parse()

	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)

	doc := Doc{}
	if err := decoder.Decode(&doc); err != nil {
		log.Fatal(err)
	}

	docString, err := RenderDoc(doc)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(out, []byte(docString), 0644); err != nil {
		log.Fatal(err)
	}
}
