//go:build generate
// +build generate

package main

import (
	"flag"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

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

type Doc struct {
	Kind        string     `yaml:"string"`
	Description string     `yaml:"description"`
	Version     string     `yaml:"version"`
	Scope       string     `yaml:"scope"`
	Fields      []DocField `yaml:"fields"`
}

//go:generate sh -c "go run generator.go -file gateway.yaml"
//go:generate sh -c "go run generator.go -file gatewayclass.yaml"
//go:generate sh -c "go run generator.go -file gatewayclassconfig.yaml"
//go:generate sh -c "go run generator.go -file http-route.yaml"
//go:generate sh -c "go run generator.go -file meshservice.yaml"
//go:generate sh -c "go run generator.go -file referencepolicy.yaml"
//go:generate sh -c "go run generator.go -file tcp-route.yaml"

func main() {
	var file string

	flag.StringVar(&file, "file", "", "name of the file to use for generating docs")

	flag.Parse()

	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)

	doc := &Doc{}
	if err := decoder.Decode(doc); err != nil {
		log.Fatal(err)
	}
	log.Println(doc)
}
