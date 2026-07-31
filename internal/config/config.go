// Package config defines the DocDag configuration schema, the built-in presets
// and the discovery/merge rules that produce an effective configuration.
package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/Kaikei-e/DocDag/internal/model"
)

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
func (c Config) Edge(name model.EdgeType) (EdgeSpec, bool) {
	for _, spec := range c.Edges {
		if spec.Name == name.String() {
			return spec, true
		}
	}
	return EdgeSpec{}, false
}

// EdgeTypes returns every declared edge type in declaration order.
func (c Config) EdgeTypes() []model.EdgeType {
	types := make([]model.EdgeType, 0, len(c.Edges))
	for _, spec := range c.Edges {
		types = append(types, model.EdgeType(spec.Name))
	}
	return types
}

// AcyclicEdgeTypes returns the edge types that must stay acyclic.
func (c Config) AcyclicEdgeTypes() []model.EdgeType {
	types := make([]model.EdgeType, 0, len(c.Edges))
	for _, spec := range c.Edges {
		if spec.Acyclic {
			types = append(types, model.EdgeType(spec.Name))
		}
	}
	return types
}

// Validate reports schema problems such as an unknown direction, a rule with an
// unknown severity or a derived edge pointing at an undeclared edge type.
func (c Config) Validate() error {
	if c.IDWidth < 0 {
		return fmt.Errorf("id_width %d is negative: %w", c.IDWidth, model.ErrInvalidConfig)
	}
	if err := c.validateEdges(); err != nil {
		return err
	}
	if err := c.validateRules(); err != nil {
		return err
	}
	return c.validateDerivedEdges()
}

func (c Config) validateEdges() error {
	declared := make(map[string]bool, len(c.Edges))
	for _, spec := range c.Edges {
		switch {
		case spec.Name == "":
			return fmt.Errorf("edge without a name: %w", model.ErrInvalidConfig)
		case spec.Key == "":
			return fmt.Errorf("edge %q without a frontmatter key: %w", spec.Name, model.ErrInvalidConfig)
		case declared[spec.Name]:
			return fmt.Errorf("edge %q is declared twice: %w", spec.Name, model.ErrInvalidConfig)
		}
		if err := validDirection(spec.Direction); err != nil {
			return fmt.Errorf("edge %q: %w", spec.Name, err)
		}
		declared[spec.Name] = true
	}
	return nil
}

func (c Config) validateRules() error {
	for _, rule := range c.Rules {
		if rule.Name == "" {
			return fmt.Errorf("rule without a name: %w", model.ErrInvalidConfig)
		}
		if rule.Severity != model.SeverityError && rule.Severity != model.SeverityWarn {
			return fmt.Errorf("rule %q: unknown severity %q: %w", rule.Name, rule.Severity, model.ErrInvalidConfig)
		}
		for _, edge := range []string{rule.When.Inbound, rule.When.NotInbound, rule.When.Outbound, rule.When.NotOutbound} {
			if edge == "" {
				continue
			}
			if _, ok := c.Edge(model.EdgeType(edge)); !ok {
				return fmt.Errorf("rule %q: undeclared edge type %q, declare it under edges or replace rules: %w", rule.Name, edge, model.ErrInvalidConfig)
			}
		}
		for _, key := range slices.Sorted(maps.Keys(rule.When.Attr)) {
			cond := rule.When.Attr[key]
			if (cond.Eq == nil) == (cond.Not == nil) {
				return fmt.Errorf("rule %q: attribute %q needs exactly one of eq and not: %w", rule.Name, key, model.ErrInvalidConfig)
			}
		}
	}
	return nil
}

func (c Config) validateDerivedEdges() error {
	for _, spec := range c.DerivedEdges {
		if spec.Field == "" {
			return fmt.Errorf("derived edge without a field: %w", model.ErrInvalidConfig)
		}
		if _, ok := c.Edge(model.EdgeType(spec.Edge)); !ok {
			return fmt.Errorf("derived edge on %q: undeclared edge type %q: %w", spec.Field, spec.Edge, model.ErrInvalidConfig)
		}
		compiled, err := regexp.Compile(spec.Pattern)
		if err != nil {
			return fmt.Errorf("derived edge on %q: pattern %q: %v: %w", spec.Field, spec.Pattern, err, model.ErrInvalidConfig)
		}
		if compiled.NumSubexp() < 1 {
			return fmt.Errorf("derived edge on %q: pattern %q captures no reference: %w", spec.Field, spec.Pattern, model.ErrInvalidConfig)
		}
		if err := validDirection(spec.Direction); err != nil {
			return fmt.Errorf("derived edge on %q: %w", spec.Field, err)
		}
	}
	return nil
}

func validDirection(direction string) error {
	if direction != DirectionForward && direction != DirectionReverse {
		return fmt.Errorf("unknown direction %q: %w", direction, model.ErrInvalidConfig)
	}
	return nil
}
