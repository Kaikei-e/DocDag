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
	StatusWithdrawn  = "withdrawn"
)

// ReferencesSpec configures reference-layer validation, which is off unless a
// configuration asks for it.
type ReferencesSpec struct {
	Dangling string   `yaml:"dangling,omitempty"`
	Pattern  string   `yaml:"pattern,omitempty"`
	Scan     []string `yaml:"scan,omitempty"`
}

// Reference-layer validation modes and the scannable regions.
const (
	ReferencesOff   = "off"
	ScanBody        = "body"
	ScanFrontmatter = "frontmatter"
)

// ReferenceSeverity reports the severity reference-layer findings carry, and
// whether the reference layer is validated at all.
func (c Config) ReferenceSeverity() (model.Severity, bool) {
	switch c.References.Dangling {
	case string(model.SeverityWarn):
		return model.SeverityWarn, true
	case string(model.SeverityError):
		return model.SeverityError, true
	}
	return "", false
}

// Scans reports whether a region of a document feeds the reference layer.
func (c Config) Scans(region string) bool {
	if len(c.References.Scan) == 0 {
		return region == ScanBody
	}
	return slices.Contains(c.References.Scan, region)
}

// EdgeSpec declares one typed constraint edge and the frontmatter key that
// carries its references.
type EdgeSpec struct {
	Name      string `yaml:"name"`
	Key       string `yaml:"key"`
	Acyclic   bool   `yaml:"acyclic"`
	Direction string `yaml:"direction"`
	// Inverse is the frontmatter key the edge's target must mirror the edge
	// under. It declares no edges of its own.
	Inverse string `yaml:"inverse,omitempty"`
	// Degree bounds on this edge type. Zero means unbounded.
	MaxInbound  int `yaml:"max_inbound,omitempty"`
	MaxOutbound int `yaml:"max_outbound,omitempty"`
	MinOutbound int `yaml:"min_outbound,omitempty"`
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

// AttrCondition matches one frontmatter attribute. Exactly one operand is set.
// Eq and Not read a scalar; the rest read the value as a list, a scalar being a
// one-element list.
type AttrCondition struct {
	Eq          *string  `yaml:"eq,omitempty"`
	Not         *string  `yaml:"not,omitempty"`
	Contains    *string  `yaml:"contains,omitempty"`
	NotContains *string  `yaml:"not_contains,omitempty"`
	SubsetOf    []string `yaml:"subset_of,omitempty"`
}

// Operands counts the operands an attribute condition sets, which must be one.
func (a AttrCondition) Operands() int {
	set := 0
	for _, operand := range []*string{a.Eq, a.Not, a.Contains, a.NotContains} {
		if operand != nil {
			set++
		}
	}
	if a.SubsetOf != nil {
		set++
	}
	return set
}

// Condition is the fixed, tiny rule vocabulary. Every populated field is ANDed:
// AnyOf holds if any member holds, and Not holds if its condition does not.
type Condition struct {
	Inbound     string                   `yaml:"inbound,omitempty"`
	NotInbound  string                   `yaml:"not_inbound,omitempty"`
	Outbound    string                   `yaml:"outbound,omitempty"`
	NotOutbound string                   `yaml:"not_outbound,omitempty"`
	Attr        map[string]AttrCondition `yaml:"attr,omitempty"`
	AnyOf       []Condition              `yaml:"any_of,omitempty"`
	Not         *Condition               `yaml:"not,omitempty"`
}

// Conditions returns this condition and every condition nested inside it, so
// validation and location lookup never miss a clause the matcher evaluates.
func (c Condition) Conditions() []Condition {
	all := []Condition{c}
	for _, alternative := range c.AnyOf {
		all = append(all, alternative.Conditions()...)
	}
	if c.Not != nil {
		all = append(all, c.Not.Conditions()...)
	}
	return all
}

// EdgeClause is one edge requirement of a condition: an edge type, the
// direction it is read in, and whether its absence is what the rule wants.
type EdgeClause struct {
	Edge    string
	Inbound bool
	Negate  bool
}

// EdgeClauses enumerates the populated edge clauses of a condition, so the
// validator and the matcher never disagree about the vocabulary.
func (c Condition) EdgeClauses() []EdgeClause {
	all := []EdgeClause{
		{Edge: c.Inbound, Inbound: true},
		{Edge: c.NotInbound, Inbound: true, Negate: true},
		{Edge: c.Outbound},
		{Edge: c.NotOutbound, Negate: true},
	}
	clauses := make([]EdgeClause, 0, len(all))
	for _, clause := range all {
		if clause.Edge != "" {
			clauses = append(clauses, clause)
		}
	}
	if len(clauses) == 0 {
		return nil
	}
	return clauses
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
	References   ReferencesSpec    `yaml:"references,omitempty"`
	AcyclicUnion bool              `yaml:"acyclic_union,omitempty"`
	// Structural raises the severity of a built-in check. Lowering one is a
	// configuration error: the checks are the contract, not a preference.
	Structural map[string]model.Severity `yaml:"structural,omitempty"`
}

// structuralSeverities is the severity every built-in check reports at, and
// therefore the set of names a structural escalation may address.
var structuralSeverities = map[string]model.Severity{
	model.RuleCycle:                  model.SeverityError,
	model.RuleDanglingRef:            model.SeverityError,
	model.RuleIDCollision:            model.SeverityError,
	model.RuleInvalidFrontmatter:     model.SeverityError,
	model.RuleMissingFrontmatter:     model.SeverityWarn,
	model.RuleUnknownStatus:          model.SeverityError,
	model.RuleDerivedConflict:        model.SeverityError,
	model.RuleUnstructuredSupersedes: model.SeverityWarn,
	model.RuleInvalidRef:             model.SeverityError,
	model.RuleEmptyEdge:              model.SeverityError,
	model.RuleInverseMismatch:        model.SeverityError,
	model.RuleCardinality:            model.SeverityError,
}

// Severity reports the severity a structural check speaks at, after whatever
// escalation the configuration applies.
func (c Config) Severity(rule string) model.Severity {
	if raised, ok := c.Structural[rule]; ok {
		return raised
	}
	return structuralSeverities[rule]
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
	if err := c.validateReferences(); err != nil {
		return err
	}
	if err := c.validateStructural(); err != nil {
		return err
	}
	return c.validateDerivedEdges()
}

func (c Config) validateStructural() error {
	for _, rule := range slices.Sorted(maps.Keys(c.Structural)) {
		want := c.Structural[rule]
		base, known := structuralSeverities[rule]
		switch {
		case !known:
			return fmt.Errorf("structural: %q is not a structural check: %w", rule, model.ErrInvalidConfig)
		case want != model.SeverityError && want != model.SeverityWarn:
			return fmt.Errorf("structural %q: unknown severity %q: %w", rule, want, model.ErrInvalidConfig)
		case want.Rank() > base.Rank():
			return fmt.Errorf("structural %q: %q cannot be lowered to %q: %w", rule, base, want, model.ErrInvalidConfig)
		}
	}
	return nil
}

func (c Config) validateReferences() error {
	if _, on := c.ReferenceSeverity(); !on && c.References.Dangling != "" && c.References.Dangling != ReferencesOff {
		return fmt.Errorf("references: unknown dangling mode %q, want %s, %s or %s: %w",
			c.References.Dangling, ReferencesOff, model.SeverityWarn, model.SeverityError, model.ErrInvalidConfig)
	}
	for _, region := range c.References.Scan {
		if region != ScanBody && region != ScanFrontmatter {
			return fmt.Errorf("references: unknown scan region %q, want %s or %s: %w",
				region, ScanBody, ScanFrontmatter, model.ErrInvalidConfig)
		}
	}
	if c.References.Pattern == "" {
		return nil
	}
	if _, err := regexp.Compile(c.References.Pattern); err != nil {
		return fmt.Errorf("references: pattern %q: %v: %w", c.References.Pattern, err, model.ErrInvalidConfig)
	}
	return nil
}

func (c Config) validateEdges() error {
	declared := make(map[string]bool, len(c.Edges))
	keys := make(map[string]bool, len(c.Edges))
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
		if err := validBounds(spec); err != nil {
			return err
		}
		declared[spec.Name] = true
		keys[spec.Key] = true
	}
	return c.validateInverseKeys(keys)
}

// validateInverseKeys keeps an inverse key out of every other role: a key that
// also declares edges would make one relation two contradictory things.
func (c Config) validateInverseKeys(keys map[string]bool) error {
	inverses := make(map[string]bool, len(c.Edges))
	for _, spec := range c.Edges {
		switch {
		case spec.Inverse == "":
			continue
		case keys[spec.Inverse]:
			return fmt.Errorf("edge %q: inverse key %q already declares edges: %w", spec.Name, spec.Inverse, model.ErrInvalidConfig)
		case inverses[spec.Inverse]:
			return fmt.Errorf("edge %q: inverse key %q is already the inverse of another edge: %w", spec.Name, spec.Inverse, model.ErrInvalidConfig)
		}
		inverses[spec.Inverse] = true
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
		for _, cond := range rule.When.Conditions() {
			if err := c.validateCondition(rule.Name, cond); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c Config) validateCondition(rule string, cond Condition) error {
	if cond.AnyOf != nil && len(cond.AnyOf) == 0 {
		return fmt.Errorf("rule %q: any_of without alternatives: %w", rule, model.ErrInvalidConfig)
	}
	for _, clause := range cond.EdgeClauses() {
		if _, ok := c.Edge(model.EdgeType(clause.Edge)); !ok {
			return fmt.Errorf("rule %q: undeclared edge type %q, declare it under edges or replace rules: %w", rule, clause.Edge, model.ErrInvalidConfig)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(cond.Attr)) {
		if cond.Attr[key].Operands() != 1 {
			return fmt.Errorf("rule %q: attribute %q needs exactly one of eq, not, contains, not_contains and subset_of: %w",
				rule, key, model.ErrInvalidConfig)
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

func validBounds(spec EdgeSpec) error {
	bounds := []struct {
		name  string
		value int
	}{
		{"max_inbound", spec.MaxInbound},
		{"max_outbound", spec.MaxOutbound},
		{"min_outbound", spec.MinOutbound},
	}
	for _, bound := range bounds {
		if bound.value < 0 {
			return fmt.Errorf("edge %q: %s %d is negative: %w", spec.Name, bound.name, bound.value, model.ErrInvalidConfig)
		}
	}
	if spec.MaxOutbound > 0 && spec.MinOutbound > spec.MaxOutbound {
		return fmt.Errorf("edge %q: min_outbound %d is above max_outbound %d: %w",
			spec.Name, spec.MinOutbound, spec.MaxOutbound, model.ErrInvalidConfig)
	}
	return nil
}

func validDirection(direction string) error {
	if direction != DirectionForward && direction != DirectionReverse {
		return fmt.Errorf("unknown direction %q: %w", direction, model.ErrInvalidConfig)
	}
	return nil
}
