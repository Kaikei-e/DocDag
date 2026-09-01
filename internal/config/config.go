// Package config defines the DocDag configuration schema, the built-in presets
// and the discovery/merge rules that produce an effective configuration.
package config

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

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

// Frontmatter keys the engine reads on every document whatever its kind
// declares: the two it renders, and the two that carry identity.
const (
	KeyTitle = "title"
	KeyDate  = "date"
	KeyID    = "id"
	KeyKind  = "kind"
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

// KindSpec declares one document kind: the directory its documents live in,
// the shape of their identifiers, the status vocabulary they answer to and
// whether their frontmatter is a closed set of keys. A kind without an id
// pattern keeps the digit-run identity a single-kind corpus has, and one
// without a status vocabulary inherits the top-level one.
type KindSpec struct {
	Dir          string   `yaml:"dir"`
	ID           string   `yaml:"id,omitempty"`
	StatusValues []string `yaml:"status_values,omitempty"`
	Closed       bool     `yaml:"closed,omitempty"`
}

// EdgeSpec declares one typed constraint edge and the frontmatter key that
// carries its references.
type EdgeSpec struct {
	Name      string `yaml:"name"`
	Key       string `yaml:"key"`
	Acyclic   bool   `yaml:"acyclic"`
	Direction string `yaml:"direction"`
	// From and To are the kinds the edge's endpoints may have, empty meaning
	// any. They constrain the edge as the graph holds it, so a reverse edge's
	// From is the kind of the document its key names, not of the one that
	// wrote the key down.
	From []string `yaml:"from,omitempty"`
	To   []string `yaml:"to,omitempty"`
	// Inverse is the frontmatter key the edge's target must mirror the edge
	// under. It declares no edges of its own.
	Inverse string `yaml:"inverse,omitempty"`
	// Degree bounds on this edge type. Zero means unbounded.
	MaxInbound  int `yaml:"max_inbound,omitempty"`
	MaxOutbound int `yaml:"max_outbound,omitempty"`
	MinOutbound int `yaml:"min_outbound,omitempty"`
	// Attrs declares the attributes an entry under Key may carry. An edge that
	// declares none takes plain references only, which is what every edge took
	// before attributes existed.
	Attrs map[string]EdgeAttrSpec `yaml:"attrs,omitempty"`
}

// EdgeRefKey is the key an attributed edge entry names its target under:
// `{ref: 0001, reason: conflict}`. It is reserved, so an edge cannot declare an
// attribute of that name and shadow the reference itself.
const EdgeRefKey = "ref"

// Edge attribute value types. A declaration that names none reads a string, and
// one_of is a closed string vocabulary, so it implies the same.
const (
	AttrTypeString = "string"
	AttrTypeNumber = "number"
	AttrTypeDate   = "date"
)

// AttrDateLayout is the one date an edge attribute accepts: ISO 8601 calendar
// dates, the spelling frontmatter already writes dates in.
const AttrDateLayout = "2006-01-02"

// EdgeAttrSpec declares one attribute of an edge: whether every entry has to
// carry it, the shape of its value, and the closed vocabulary it comes from.
type EdgeAttrSpec struct {
	Required bool     `yaml:"required,omitempty"`
	Type     string   `yaml:"type,omitempty"`
	OneOf    []string `yaml:"one_of,omitempty"`
}

// ValueType reports the type an attribute value is read as, string being what a
// declaration without a type asks for.
func (a EdgeAttrSpec) ValueType() string {
	if a.Type == "" {
		return AttrTypeString
	}
	return a.Type
}

// Accepts reports whether a value satisfies the attribute. The one_of
// comparison is exact and case-sensitive, unlike a status value: a status
// vocabulary describes prose a person writes by hand, while an edge attribute
// is a closed machine vocabulary that a preset revision renames wholesale, and
// two spellings of one word would make that revision ambiguous.
func (a EdgeAttrSpec) Accepts(value string) bool {
	if len(a.OneOf) > 0 {
		return slices.Contains(a.OneOf, value)
	}
	switch a.ValueType() {
	case AttrTypeNumber:
		_, err := strconv.ParseFloat(value, 64)
		return err == nil
	case AttrTypeDate:
		_, err := time.Parse(AttrDateLayout, value)
		return err == nil
	}
	return true
}

// Wants describes what the attribute accepts, as the phrase a finding about a
// rejected value ends with.
func (a EdgeAttrSpec) Wants() string {
	if len(a.OneOf) > 0 {
		return "one of: " + strings.Join(a.OneOf, ", ")
	}
	switch a.ValueType() {
	case AttrTypeNumber:
		return "a number"
	case AttrTypeDate:
		return "a date as YYYY-MM-DD"
	}
	return "a string"
}

// AttrNames returns the attributes an edge declares, sorted, so a message that
// enumerates the vocabulary reads the same on every run.
func (s EdgeSpec) AttrNames() []string {
	return slices.Sorted(maps.Keys(s.Attrs))
}

// Attr returns the declaration of one edge attribute.
func (s EdgeSpec) Attr(name string) (EdgeAttrSpec, bool) {
	spec, ok := s.Attrs[name]
	return spec, ok
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

// EdgeCondition is one degree clause: the edge type it reads and how many edges
// of it the clause wants. It is written either as the edge name alone, which
// asks for one edge or more, or as a mapping carrying min and max.
type EdgeCondition struct {
	Edge string `yaml:"edge,omitempty"`
	// Min and Max are pointers because absence is not zero: an omitted min asks
	// for one edge, and an omitted max is unbounded, so a written "max: 0" has
	// to be distinguishable from a missing one.
	Min *int `yaml:"min,omitempty"`
	Max *int `yaml:"max,omitempty"`
}

// UnmarshalYAML accepts both spellings of an edge clause: the edge name alone,
// which is sugar for one edge or more, and the mapping form with thresholds.
func (e *EdgeCondition) UnmarshalYAML(src []byte) error {
	var name string
	if err := yaml.Unmarshal(src, &name); err == nil {
		*e = EdgeCondition{Edge: name}
		return nil
	}
	// The alias sheds the method set, so decoding the mapping form does not
	// call this unmarshaler again.
	type mapping EdgeCondition
	var full mapping
	if err := yaml.Unmarshal(src, &full); err != nil {
		return err
	}
	*e = EdgeCondition(full)
	return nil
}

// set reports whether an edge clause was written at all, which a clause naming
// no edge type still was: an edge-less threshold is a configuration error, not
// an absent clause.
func (e EdgeCondition) set() bool {
	return e.Edge != "" || e.Min != nil || e.Max != nil
}

// bounds resolves the degree window a clause asks for: at least atLeast edges,
// and at most atMost unless atMost is zero, which is unbounded.
func (e EdgeCondition) bounds() (atLeast, atMost int) {
	atLeast = 1
	if e.Min != nil {
		atLeast = *e.Min
	}
	if e.Max != nil {
		atMost = *e.Max
	}
	return atLeast, atMost
}

// ViaCondition is the one-hop clause: it holds when at least one neighbour
// across the named edge type satisfies every attribute condition. It carries
// attributes alone by construction, so the vocabulary reaches exactly one hop
// and stays inside the bisimulation-invariant fragment the ADR argues for.
type ViaCondition struct {
	Edge string                   `yaml:"edge,omitempty"`
	Attr map[string]AttrCondition `yaml:"attr,omitempty"`
}

// Condition is the fixed, tiny rule vocabulary. Every populated field is ANDed:
// AnyOf holds if any member holds, and Not holds if its condition does not.
type Condition struct {
	Inbound     EdgeCondition            `yaml:"inbound,omitempty"`
	NotInbound  string                   `yaml:"not_inbound,omitempty"`
	Outbound    EdgeCondition            `yaml:"outbound,omitempty"`
	NotOutbound string                   `yaml:"not_outbound,omitempty"`
	Via         *ViaCondition            `yaml:"via,omitempty"`
	ViaInbound  *ViaCondition            `yaml:"via_inbound,omitempty"`
	Attr        map[string]AttrCondition `yaml:"attr,omitempty"`
	AnyOf       []Condition              `yaml:"any_of,omitempty"`
	Not         *Condition               `yaml:"not,omitempty"`
}

// Empty reports whether a condition constrains nothing, which is how a
// projection says it wrote down no when block at all.
func (c Condition) Empty() bool {
	return !c.Inbound.set() && !c.Outbound.set() &&
		c.NotInbound == "" && c.NotOutbound == "" &&
		c.Via == nil && c.ViaInbound == nil &&
		len(c.Attr) == 0 && c.AnyOf == nil && c.Not == nil
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
// direction it is read in, the degree window it asks for, and whether its
// absence is what the rule wants.
type EdgeClause struct {
	Edge    string
	Inbound bool
	Negate  bool
	// Min is the smallest degree that satisfies the clause and Max the largest,
	// zero meaning unbounded. A negated clause carries neither: absence is not a
	// threshold, and the vocabulary has no negated one.
	Min int
	Max int
}

// Holds reports whether a degree satisfies the clause.
func (c EdgeClause) Holds(degree int) bool {
	if c.Negate {
		return degree == 0
	}
	return degree >= c.Min && (c.Max == 0 || degree <= c.Max)
}

// EdgeClauses enumerates the populated edge clauses of a condition, so the
// validator and the matcher never disagree about the vocabulary. A threshold
// written without an edge type names nothing to count and is left out; the
// validator reports it against the condition itself.
func (c Condition) EdgeClauses() []EdgeClause {
	inboundMin, inboundMax := c.Inbound.bounds()
	outboundMin, outboundMax := c.Outbound.bounds()
	all := []EdgeClause{
		{Edge: c.Inbound.Edge, Inbound: true, Min: inboundMin, Max: inboundMax},
		{Edge: c.NotInbound, Inbound: true, Negate: true},
		{Edge: c.Outbound.Edge, Min: outboundMin, Max: outboundMax},
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

// ViaClause is one populated one-hop clause: the edge type it crosses, the
// direction it crosses it in, and what the neighbour has to look like.
type ViaClause struct {
	Edge    string
	Inbound bool
	Attr    map[string]AttrCondition
}

// Key names the vocabulary word a one-hop clause was written under.
func (c ViaClause) Key() string {
	if c.Inbound {
		return "via_inbound"
	}
	return "via"
}

// ViaClauses enumerates the populated one-hop clauses of a condition. They are
// kept apart from the edge clauses because a one-hop clause is not an existence
// question: the neighbour has to look a particular way.
func (c Condition) ViaClauses() []ViaClause {
	clauses := make([]ViaClause, 0, 2)
	if c.Via != nil {
		clauses = append(clauses, ViaClause{Edge: c.Via.Edge, Attr: c.Via.Attr})
	}
	if c.ViaInbound != nil {
		clauses = append(clauses, ViaClause{Edge: c.ViaInbound.Edge, Inbound: true, Attr: c.ViaInbound.Attr})
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

// ProjectionSpec declares one derived boolean attribute. A projection holds
// where its when block holds, or where any of its alternatives does; exactly
// one of the two is written. The result is readable as an attribute named after
// the projection, from rules, from other projections and from a listing.
type ProjectionSpec struct {
	Name  string          `yaml:"name"`
	When  Condition       `yaml:"when,omitempty"`
	AnyOf []ProjectionAlt `yaml:"any_of,omitempty"`
}

// ProjectionAlt is one alternative of a projection. It carries a when block of
// its own rather than a bare condition, so an alternative reads the way the
// projection it belongs to does.
type ProjectionAlt struct {
	When Condition `yaml:"when"`
}

// Whens returns the conditions a projection holds under: its own when block, or
// the when block of each alternative.
func (p ProjectionSpec) Whens() []Condition {
	if len(p.AnyOf) > 0 {
		out := make([]Condition, 0, len(p.AnyOf))
		for _, alt := range p.AnyOf {
			out = append(out, alt.When)
		}
		return out
	}
	return []Condition{p.When}
}

// Conditions returns every condition a projection evaluates, nested ones
// included, so validation never misses a clause the matcher reaches.
func (p ProjectionSpec) Conditions() []Condition {
	all := []Condition{}
	for _, when := range p.Whens() {
		all = append(all, when.Conditions()...)
	}
	return all
}

// AttrKeys returns every attribute key a projection reads, sorted, the one-hop
// clauses included. A key naming another projection is a dependency: it has to
// be evaluated first.
func (p ProjectionSpec) AttrKeys() []string {
	keys := make(map[string]bool)
	for _, cond := range p.Conditions() {
		for key := range cond.Attr {
			keys[key] = true
		}
		for _, clause := range cond.ViaClauses() {
			for key := range clause.Attr {
				keys[key] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(keys))
}

// Config is the effective configuration. A preset is nothing more than a
// built-in Config value, so every field is expressible in docdag.yaml.
type Config struct {
	Preset  string `yaml:"preset,omitempty"`
	Dir     string `yaml:"dir,omitempty"`
	IDWidth int    `yaml:"id_width,omitempty"`
	// Kinds declares the document kinds of a multi-kind corpus, keyed by kind
	// name. A configuration that declares none is single-kind: Dir, IDWidth and
	// StatusValues describe that one kind, exactly as they always have.
	Kinds        map[string]KindSpec `yaml:"kinds,omitempty"`
	StatusField  string              `yaml:"status_field,omitempty"`
	StatusValues []string            `yaml:"status_values,omitempty"`
	Edges        []EdgeSpec          `yaml:"edges,omitempty"`
	DerivedEdges []DerivedEdgeSpec   `yaml:"derived_edges,omitempty"`
	Rules        []Rule              `yaml:"rules,omitempty"`
	Projections  []ProjectionSpec    `yaml:"projections,omitempty"`
	// Binding names the projection that defines the set of documents in force.
	// It is a preset's answer to "what is current", not a hard-coded one.
	Binding      string         `yaml:"binding,omitempty"`
	Template     string         `yaml:"template,omitempty"`
	Filename     string         `yaml:"filename,omitempty"`
	References   ReferencesSpec `yaml:"references,omitempty"`
	AcyclicUnion bool           `yaml:"acyclic_union,omitempty"`
	// Structural raises the severity of a built-in check. Lowering one is a
	// configuration error: the checks are the contract, not a preference.
	Structural map[string]model.Severity `yaml:"structural,omitempty"`
}

// DefaultFilename is the name template a created document is written under.
// `{id}` is the padded identifier and `{slug}` the kebab-cased title.
const DefaultFilename = "{id}-{slug}.md"

// FilenameTemplate returns the configured document name template, or the
// default when the corpus configures none.
func (c Config) FilenameTemplate() string {
	if c.Filename == "" {
		return DefaultFilename
	}
	return c.Filename
}

// validateFilename holds the name template to what discovery can find again:
// a document is identified by the identifier in its name, and it lives in the
// documents directory rather than below it.
func (c Config) validateFilename() error {
	if c.Filename == "" {
		return nil
	}
	if !strings.Contains(c.Filename, "{id}") {
		return fmt.Errorf("filename %q carries no {id}: %w", c.Filename, model.ErrInvalidConfig)
	}
	if strings.ContainsAny(c.Filename, `/\`) {
		return fmt.Errorf("filename %q carries a path separator: %w", c.Filename, model.ErrInvalidConfig)
	}
	return nil
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
	model.RuleEdgeAttrUnknown:        model.SeverityError,
	model.RuleEdgeAttrMissing:        model.SeverityError,
	model.RuleEdgeAttrInvalid:        model.SeverityError,
	model.RuleIDMismatch:             model.SeverityError,
	model.RuleKindMismatch:           model.SeverityError,
	model.RuleUnknownField:           model.SeverityError,
	model.RuleEdgeKindMismatch:       model.SeverityError,
}

// Severity reports the severity a structural check speaks at, after whatever
// escalation the configuration applies.
func (c Config) Severity(rule string) model.Severity {
	if raised, ok := c.Structural[rule]; ok {
		return raised
	}
	return structuralSeverities[rule]
}

// Multikind reports whether the configuration declares document kinds. A
// configuration that declares none is the single-kind corpus DocDag has always
// managed, and every kind-aware code path falls back on that behaviour.
func (c Config) Multikind() bool { return len(c.Kinds) > 0 }

// Kind returns the spec of one declared kind.
func (c Config) Kind(name string) (KindSpec, bool) {
	spec, ok := c.Kinds[name]
	return spec, ok
}

// KindNames returns the declared kinds sorted by name. Sorted rather than
// written order because a map has no order to keep, and every answer derived
// from the kinds — which one resolves a reference, which directory is read
// first — has to be the same on every run.
func (c Config) KindNames() []string { return slices.Sorted(maps.Keys(c.Kinds)) }

// KindStatusValues returns the status vocabulary a kind answers to: its own
// when it declares one, the top-level vocabulary otherwise. A kind that says
// nothing about status is not a kind without status.
func (c Config) KindStatusValues(name string) []string {
	if spec, ok := c.Kinds[name]; ok && len(spec.StatusValues) > 0 {
		return spec.StatusValues
	}
	return c.StatusValues
}

// KnownFrontmatterKeys returns the frontmatter keys a closed kind accepts,
// sorted: the keys the engine itself reads, the status field, and every key the
// configured edges declare or mirror or a derived edge reads. It is built from
// the configuration rather than written down, so a corpus that renames a key
// does not also have to widen the set; a later revision adds the keys a kind
// declares under fields: by extending this one place.
func (c Config) KnownFrontmatterKeys() []string {
	keys := map[string]bool{
		KeyTitle:            true,
		KeyDate:             true,
		KeyID:               true,
		KeyKind:             true,
		c.EffectiveStatus(): true,
	}
	for _, spec := range c.Edges {
		keys[spec.Key] = true
		if spec.Inverse != "" {
			keys[spec.Inverse] = true
		}
	}
	for _, spec := range c.DerivedEdges {
		keys[spec.Field] = true
	}
	return slices.Sorted(maps.Keys(keys))
}

// EffectiveStatus names the frontmatter key statuses are read from, which is
// the default wherever a configuration renames nothing.
func (c Config) EffectiveStatus() string {
	if c.StatusField == "" {
		return DefaultStatusField
	}
	return c.StatusField
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

// Projection returns the spec of one declared projection.
func (c Config) Projection(name string) (ProjectionSpec, bool) {
	for _, spec := range c.Projections {
		if spec.Name == name {
			return spec, true
		}
	}
	return ProjectionSpec{}, false
}

// ProjectionNames returns every declared projection name in declaration order.
func (c Config) ProjectionNames() []string {
	names := make([]string, 0, len(c.Projections))
	for _, spec := range c.Projections {
		names = append(names, spec.Name)
	}
	return names
}

// BindingProjection returns the projection that defines the binding set, and
// whether the configuration resolves one at all. A configuration that declares
// no projections resolves none, and the caller falls back on the built-in
// definition rather than reporting every document as current.
func (c Config) BindingProjection() (ProjectionSpec, bool) {
	if c.Binding == "" {
		return ProjectionSpec{}, false
	}
	return c.Projection(c.Binding)
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
	if err := c.validateKinds(); err != nil {
		return err
	}
	if err := c.validateEdges(); err != nil {
		return err
	}
	if err := c.validateRules(); err != nil {
		return err
	}
	if err := c.validateProjections(); err != nil {
		return err
	}
	if err := c.validateFilename(); err != nil {
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

// validateKinds holds the declared kinds to what the parser can read: a named
// kind with a directory of its own and, where it declares one, an identifier
// pattern that compiles. Two kinds sharing a directory would make the kind of a
// document — and therefore its identity — depend on which one was looked at
// first, so the directories have to be distinct.
func (c Config) validateKinds() error {
	if !c.Multikind() {
		return nil
	}
	// A corpus whose kinds declare their own directories has nothing for a
	// top-level dir to describe, and one written anyway — in the file or as
	// --dir — names a directory no kind would claim the documents of.
	if c.Dir != "" {
		return fmt.Errorf("dir %q describes nothing beside kinds, which declare their own directories: %w",
			c.Dir, model.ErrInvalidConfig)
	}
	dirs := make(map[string]string, len(c.Kinds))
	for _, name := range c.KindNames() {
		spec := c.Kinds[name]
		switch {
		case name == "":
			return fmt.Errorf("kind without a name: %w", model.ErrInvalidConfig)
		case spec.Dir == "":
			return fmt.Errorf("kind %q without a dir: %w", name, model.ErrInvalidConfig)
		}
		dir := path.Clean(filepath.ToSlash(spec.Dir))
		if owner, taken := dirs[dir]; taken {
			return fmt.Errorf("kind %q: dir %q already holds kind %q: %w", name, spec.Dir, owner, model.ErrInvalidConfig)
		}
		dirs[dir] = name
		if spec.ID == "" {
			continue
		}
		if _, err := IDPattern(spec.ID); err != nil {
			return fmt.Errorf("kind %q: id %q: %v: %w", name, spec.ID, err, model.ErrInvalidConfig)
		}
	}
	return nil
}

// validateEdgeKinds holds an edge's endpoint constraints to the declared
// vocabulary: a kind nobody declares constrains nothing, and a constraint on a
// corpus without kinds could never be violated, so both are typos rather than
// opinions.
func (c Config) validateEdgeKinds(spec EdgeSpec) error {
	for _, side := range []struct {
		key   string
		kinds []string
	}{{"from", spec.From}, {"to", spec.To}} {
		for _, name := range side.kinds {
			if !c.Multikind() {
				return fmt.Errorf("edge %q: %s names kind %q, which the configuration does not declare: %w",
					spec.Name, side.key, name, model.ErrInvalidConfig)
			}
			if _, ok := c.Kind(name); !ok {
				return fmt.Errorf("edge %q: %s names undeclared kind %q, declare it under kinds: %w",
					spec.Name, side.key, name, model.ErrInvalidConfig)
			}
		}
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
		if err := validEdgeAttrs(spec); err != nil {
			return err
		}
		if err := c.validateEdgeKinds(spec); err != nil {
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
		subject := fmt.Sprintf("rule %q", rule.Name)
		for _, cond := range rule.When.Conditions() {
			if err := c.validateCondition(subject, cond); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateProjections holds the projections to the shape the evaluator can
// answer: named, distinct, written one way, over declared edges, and acyclic.
func (c Config) validateProjections() error {
	declared := make(map[string]bool, len(c.Projections))
	for _, spec := range c.Projections {
		switch {
		case spec.Name == "":
			return fmt.Errorf("projection without a name: %w", model.ErrInvalidConfig)
		case declared[spec.Name]:
			return fmt.Errorf("projection %q is declared twice: %w", spec.Name, model.ErrInvalidConfig)
		}
		declared[spec.Name] = true
		subject := fmt.Sprintf("projection %q", spec.Name)
		switch {
		case !spec.When.Empty() && len(spec.AnyOf) > 0:
			return fmt.Errorf("%s: when and any_of are alternatives, write one: %w", subject, model.ErrInvalidConfig)
		case spec.When.Empty() && len(spec.AnyOf) == 0:
			return fmt.Errorf("%s: needs a when block or any_of alternatives: %w", subject, model.ErrInvalidConfig)
		}
		for _, alt := range spec.AnyOf {
			if alt.When.Empty() {
				return fmt.Errorf("%s: alternative without a when block: %w", subject, model.ErrInvalidConfig)
			}
		}
		for _, cond := range spec.Conditions() {
			if err := c.validateCondition(subject, cond); err != nil {
				return err
			}
		}
	}
	if err := c.validateProjectionCycles(); err != nil {
		return err
	}
	// A binding that names nothing is how a configuration without projections
	// asks for the built-in definition; one that names a projection nobody
	// declared is a typo the evaluator cannot answer.
	if len(c.Projections) > 0 && c.Binding != "" && !declared[c.Binding] {
		return fmt.Errorf("binding %q is not a declared projection: %w", c.Binding, model.ErrInvalidConfig)
	}
	return nil
}

// validateProjectionCycles rejects projections that read each other in a loop.
// The dependency graph is over the specs, not the documents, so it is as small
// as the configuration and a plain depth-first walk over it terminates.
func (c Config) validateProjectionCycles() error {
	const (
		white = iota
		gray
		black
	)
	color := make(map[string]int, len(c.Projections))
	var path []string
	var visit func(spec ProjectionSpec) error
	visit = func(spec ProjectionSpec) error {
		color[spec.Name] = gray
		path = append(path, spec.Name)
		for _, key := range spec.AttrKeys() {
			next, ok := c.Projection(key)
			if !ok {
				continue
			}
			switch color[key] {
			case gray:
				// The walk may have reached the loop from outside it, so the
				// message names the loop rather than the way in.
				loop := path[slices.Index(path, key):]
				return fmt.Errorf("projection %q: reference cycle %s -> %s: %w",
					spec.Name, strings.Join(loop, " -> "), key, model.ErrInvalidConfig)
			case white:
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		path = path[:len(path)-1]
		color[spec.Name] = black
		return nil
	}
	for _, spec := range c.Projections {
		if color[spec.Name] != white {
			continue
		}
		if err := visit(spec); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) validateCondition(subject string, cond Condition) error {
	if cond.AnyOf != nil && len(cond.AnyOf) == 0 {
		return fmt.Errorf("%s: any_of without alternatives: %w", subject, model.ErrInvalidConfig)
	}
	if err := c.validateEdgeClauses(subject, cond); err != nil {
		return err
	}
	if err := c.validateViaClauses(subject, cond); err != nil {
		return err
	}
	return validateAttrs(subject, "", cond.Attr)
}

func (c Config) validateEdgeClauses(subject string, cond Condition) error {
	for _, clause := range cond.EdgeClauses() {
		if _, ok := c.Edge(model.EdgeType(clause.Edge)); !ok {
			return fmt.Errorf("%s: undeclared edge type %q, declare it under edges or replace rules: %w", subject, clause.Edge, model.ErrInvalidConfig)
		}
	}
	for _, written := range []struct {
		key    string
		clause EdgeCondition
	}{{"inbound", cond.Inbound}, {"outbound", cond.Outbound}} {
		if err := validateEdgeBounds(subject, written.key, written.clause); err != nil {
			return err
		}
	}
	return nil
}

// validateEdgeBounds holds a degree threshold to a window a document can land
// in: at least one edge, and at most no fewer than it asks for at least.
func validateEdgeBounds(subject, key string, clause EdgeCondition) error {
	if !clause.set() {
		return nil
	}
	if clause.Edge == "" {
		return fmt.Errorf("%s: %s without an edge type: %w", subject, key, model.ErrInvalidConfig)
	}
	atLeast, atMost := clause.bounds()
	switch {
	case atLeast < 1:
		return fmt.Errorf("%s: %s min %d is below 1, absence is not_%s: %w", subject, key, atLeast, key, model.ErrInvalidConfig)
	case clause.Max != nil && atMost < 1:
		return fmt.Errorf("%s: %s max %d is below 1, absence is not_%s: %w", subject, key, atMost, key, model.ErrInvalidConfig)
	case atMost > 0 && atLeast > atMost:
		return fmt.Errorf("%s: %s min %d is above max %d: %w", subject, key, atLeast, atMost, model.ErrInvalidConfig)
	}
	return nil
}

func (c Config) validateViaClauses(subject string, cond Condition) error {
	for _, clause := range cond.ViaClauses() {
		if clause.Edge == "" {
			return fmt.Errorf("%s: %s without an edge type: %w", subject, clause.Key(), model.ErrInvalidConfig)
		}
		if _, ok := c.Edge(model.EdgeType(clause.Edge)); !ok {
			return fmt.Errorf("%s: %s names undeclared edge type %q, declare it under edges or replace rules: %w",
				subject, clause.Key(), clause.Edge, model.ErrInvalidConfig)
		}
		if err := validateAttrs(subject, clause.Key()+" ", clause.Attr); err != nil {
			return err
		}
	}
	return nil
}

func validateAttrs(subject, where string, attrs map[string]AttrCondition) error {
	for _, key := range slices.Sorted(maps.Keys(attrs)) {
		if attrs[key].Operands() != 1 {
			return fmt.Errorf("%s: %sattribute %q needs exactly one of eq, not, contains, not_contains and subset_of: %w",
				subject, where, key, model.ErrInvalidConfig)
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

// validEdgeAttrs holds an attribute declaration to a vocabulary the checker can
// answer: a name of its own that is not the reserved reference key, one of the
// three value types, and one_of only where the values are strings.
func validEdgeAttrs(spec EdgeSpec) error {
	for _, name := range spec.AttrNames() {
		attr := spec.Attrs[name]
		switch {
		case name == "":
			return fmt.Errorf("edge %q: attribute without a name: %w", spec.Name, model.ErrInvalidConfig)
		case name == EdgeRefKey:
			return fmt.Errorf("edge %q: attribute %q is reserved for the reference itself: %w",
				spec.Name, name, model.ErrInvalidConfig)
		case attr.Type != "" && attr.Type != AttrTypeString && attr.Type != AttrTypeNumber && attr.Type != AttrTypeDate:
			return fmt.Errorf("edge %q: attribute %q: unknown type %q, want %s, %s or %s: %w",
				spec.Name, name, attr.Type, AttrTypeString, AttrTypeNumber, AttrTypeDate, model.ErrInvalidConfig)
		case len(attr.OneOf) > 0 && attr.ValueType() != AttrTypeString:
			return fmt.Errorf("edge %q: attribute %q: one_of reads a %s, not a %s: %w",
				spec.Name, name, AttrTypeString, attr.ValueType(), model.ErrInvalidConfig)
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
