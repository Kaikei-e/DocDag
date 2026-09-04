package lint

import (
	"os"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// The sections of a configuration file a lint finding is located in.
const (
	SectionRules           = "rules"
	SectionProjections     = "projections"
	SectionEdges           = "edges"
	SectionPathConstraints = "path_constraints"
	SectionKinds           = "kinds"
	SectionStatusValues    = "status_values"
	SectionBinding         = "binding"
)

// PresetPath is the path a finding about a preset-bundled rule is reported at.
// There is no file to open, and a finding with no path at all would read as one
// nobody could place; a virtual path says where the rule came from instead.
func PresetPath(preset string) string {
	if preset == "" {
		preset = config.PresetADR
	}
	return "<preset:" + preset + ">"
}

// Locator answers where in docdag.yaml a rule, a projection, an edge or a path
// constraint was written down. A name the file does not hold came from the
// preset, and is located at the preset's virtual path: the merge replaces a
// section wholesale, so a rule absent from the file is one the file never wrote.
type Locator struct {
	path     string
	preset   string
	names    map[string]int
	sections map[string]int
}

// NewLocator reads the configuration file and records the line every named
// entry was written on. A file that does not exist, does not read or does not
// parse yields a locator that places every finding at the preset: lint is not
// the layer that reports a broken configuration file.
func NewLocator(path, preset string) Locator {
	loc := Locator{
		path:     path,
		preset:   preset,
		names:    map[string]int{},
		sections: map[string]int{},
	}
	if path == "" {
		return loc
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return loc
	}
	file, err := parser.ParseBytes(src, 0)
	if err != nil {
		return loc
	}
	for _, doc := range file.Docs {
		for _, entry := range mappingValues(doc.Body) {
			loc.record(entry)
		}
	}
	return loc
}

// Path is the configuration file the locator reads, empty where the corpus
// configures nothing and every rule comes from the preset.
func (l Locator) Path() string { return l.path }

// record notes one top-level key: the line it is written on, and the line of
// every named entry under it. A sequence of mappings is named by each entry's
// own name key — the shape rules, projections, edges and path constraints all
// share — and a mapping of mappings by its keys, which is how kinds are written.
func (l Locator) record(entry *ast.MappingValueNode) {
	section, line, ok := keyLine(entry)
	if !ok {
		return
	}
	l.sections[section] = line
	switch value := entry.Value.(type) {
	case *ast.SequenceNode:
		for _, item := range value.Values {
			for _, field := range mappingValues(item) {
				name, at, ok := keyLine(field)
				if !ok || name != "name" {
					continue
				}
				if written := scalarValue(field.Value); written != "" {
					l.names[section+"/"+written] = at
				}
			}
		}
	default:
		for _, field := range mappingValues(entry.Value) {
			if name, at, ok := keyLine(field); ok {
				l.names[section+"/"+name] = at
			}
		}
	}
}

// Locate reports where a named entry of a section was written: its own line in
// the configuration file, the section's line where the file writes the section
// but not that entry, and the preset otherwise.
func (l Locator) Locate(section, name string) model.Location {
	if line, ok := l.names[section+"/"+name]; ok {
		return model.Location{Path: l.path, Line: line}
	}
	return l.Section(section)
}

// Section reports where a whole section was written, and the preset where the
// file writes none.
func (l Locator) Section(section string) model.Location {
	if line, ok := l.sections[section]; ok {
		return model.Location{Path: l.path, Line: line}
	}
	return model.Location{Path: PresetPath(l.preset)}
}

// mappingValues reads a node as the key-value pairs it holds. A mapping of one
// pair parses as the pair itself rather than as a mapping, so both shapes are
// read here and nowhere else.
func mappingValues(node ast.Node) []*ast.MappingValueNode {
	switch body := node.(type) {
	case *ast.MappingNode:
		return body.Values
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{body}
	}
	return nil
}

func keyLine(value *ast.MappingValueNode) (string, int, bool) {
	if value == nil || value.Key == nil {
		return "", 0, false
	}
	token := value.Key.GetToken()
	if token == nil || token.Position == nil {
		return "", 0, false
	}
	return token.Value, token.Position.Line, true
}

// scalarValue reads a node as the string it was written as, and reports the
// empty string for anything that is not a scalar.
func scalarValue(node ast.Node) string {
	if node == nil {
		return ""
	}
	if token := node.GetToken(); token != nil {
		return token.Value
	}
	return ""
}
