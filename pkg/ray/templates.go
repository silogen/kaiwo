package ray

import (
	_ "embed"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

//go:embed rayjob.yaml
var RayJobYAML []byte

//go:embed rayservice.yaml
var RayServiceYAML []byte

// RayJob YAML file as an AST
func GetRayJobYAML() (*ast.File, error) {
	file, err := parser.ParseBytes(RayJobYAML, 0)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// RayService YAML file as an AST
func GetRayServiceYAML() (*ast.File, error) {
	file, err := parser.ParseBytes(RayServiceYAML, 0)
	if err != nil {
		return nil, err
	}
	return file, nil
}
