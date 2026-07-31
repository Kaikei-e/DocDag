// Package config defines the DocDag configuration schema, the built-in presets
// and the discovery/merge rules that produce an effective configuration.
package config

import "github.com/Kaikei-e/DocDag/internal/model"

// Preset names.
const (
	PresetADR = "adr"
)

// Defaults applied when neither flags nor docdag.yaml say otherwise.
const (
	DefaultIDWidth     = 4
	DefaultStatusField = "status"
	DefaultConfigFile  = "docdag.yaml"
)

// Edge directions. Forward means the containing document is the edge source;
// reverse means the referenced document is the source.
const (
	DirectionForward = "forward"
	DirectionReverse = "reverse"
)

// Edge types of the ADR preset.
const (
	EdgeSupersedes model.EdgeType = "supersedes"
	EdgeDependsOn  model.EdgeType = "depends-on"
)

// Status vocabulary of the ADR preset.
const (
	StatusProposed   = "proposed"
	StatusAccepted   = "accepted"
	StatusRejected   = "rejected"
	StatusDeprecated = "deprecated"
	StatusSuperseded = "superseded"
)

// EdgeSpec declares one typed constraint edge and the frontmatter key that
// carries its references.
type EdgeSpec struct {
	Name      string `yaml:"name"`
	Key       string `yaml:"key"`
	Acyclic   bool   `yaml:"acyclic"`
	Direction string `yaml:"direction"`
}

// DerivedEdgeSpec declares an edge inferred from a frontmatter field value.
type DerivedEdgeSpec struct {
	Field string `yaml:"field"`
	// Pattern is a Go regular expression whose first capture group is the
	// referenced document.
	Pattern   string `yaml:"pattern"`
	Edge      string `yaml:"edge"`
	Direction string `yaml:"direction"`
}

// AttrCondition matches one frontmatter attribute. Exactly one of Eq or Not is
// set.
type AttrCondition struct {
	Eq  *string `yaml:"eq,omitempty"`
	Not *string `yaml:"not,omitempty"`
}

// Condition is the fixed, tiny rule vocabulary. Every populated field is ANDed.
type Condition struct {
	Inbound     string                   `yaml:"inbound,omitempty"`
	NotInbound  string                   `yaml:"not_inbound,omitempty"`
	Outbound    string                   `yaml:"outbound,omitempty"`
	NotOutbound string                   `yaml:"not_outbound,omitempty"`
	Attr        map[string]AttrCondition `yaml:"attr,omitempty"`
}

// Rule is one declarative implication evaluated per node.
type Rule struct {
	Name     string         `yaml:"name"`
	Severity model.Severity `yaml:"severity"`
	When     Condition      `yaml:"when"`
	Message  string         `yaml:"message"`
}

// Config is the effective configuration. A preset is nothing more than a
// built-in Config value, so every field is expressible in docdag.yaml.
type Config struct {
	Preset       string            `yaml:"preset,omitempty"`
	Dir          string            `yaml:"dir,omitempty"`
	IDWidth      int               `yaml:"id_width,omitempty"`
	StatusField  string            `yaml:"status_field,omitempty"`
	StatusValues []string          `yaml:"status_values,omitempty"`
	Edges        []EdgeSpec        `yaml:"edges,omitempty"`
	DerivedEdges []DerivedEdgeSpec `yaml:"derived_edges,omitempty"`
	Rules        []Rule            `yaml:"rules,omitempty"`
	Template     string            `yaml:"template,omitempty"`
}

// Edge returns the spec of one typed edge.
func (c Config) Edge(name model.EdgeType) (EdgeSpec, bool) { return EdgeSpec{}, false }

// EdgeTypes returns every declared edge type in declaration order.
func (c Config) EdgeTypes() []model.EdgeType { return nil }

// AcyclicEdgeTypes returns the edge types that must stay acyclic.
func (c Config) AcyclicEdgeTypes() []model.EdgeType { return nil }

// Validate reports schema problems such as an unknown direction, a rule with an
// unknown severity or a derived edge pointing at an undeclared edge type.
func (c Config) Validate() error { return model.ErrNotImplemented }
