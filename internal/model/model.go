// Package model holds the vocabulary shared by every DocDag layer: document
// identity, the typed constraint graph, and validation findings.
package model

import (
	"fmt"
	"maps"
	"slices"
)

// ID is a normalized, opaque document identifier. Presets decide how a raw
// reference or filename token becomes an ID; the engine only compares them.
type ID string

// String returns the identifier as written.
func (id ID) String() string { return string(id) }

// EdgeType names a typed constraint edge, such as "supersedes".
type EdgeType string

// String returns the edge type as written.
func (t EdgeType) String() string { return string(t) }

// Origin records how an edge entered the graph.
type Origin string

// Edge origins. Reference edges are never subject to graph invariants.
const (
	OriginStructured Origin = "structured"
	OriginDerived    Origin = "derived"
	OriginReference  Origin = "reference"
)

// Severity ranks a finding.
type Severity string

// Finding severities. Info is what `docdag lint` says a fact in: it never
// fails a build and never raises an exit code, so a check that reports one is
// telling the reader something about the corpus rather than about a mistake.
// No structural check speaks at it — `structural:` cannot lower a check to
// info, because the checks are the contract.
const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
)

// Rank orders severities for deterministic output, errors first. Info sorts
// last, where an unrecognized severity already sorted: both are things a
// report ends with, and a finding is ordered by its position after that.
func (s Severity) Rank() int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarn:
		return 1
	}
	return 2
}

// Built-in structural check names and the preset rule names.
const (
	RuleCycle                  = "cycle"
	RuleDanglingRef            = "dangling_ref"
	RuleIDCollision            = "id_collision"
	RuleInvalidFrontmatter     = "invalid_frontmatter"
	RuleMissingFrontmatter     = "missing_frontmatter"
	RuleUnknownStatus          = "unknown_status"
	RuleDerivedConflict        = "derived_conflict"
	RuleUnstructuredSupersedes = "unstructured_supersedes"
	RuleStatusDrift            = "status_drift"
	RuleSupersededOrphan       = "superseded_orphan"
	RuleInvalidRef             = "invalid_ref"
	RuleDanglingReference      = "dangling_reference"
	RuleEmptyEdge              = "empty_edge"
	RuleInverseMismatch        = "inverse_mismatch"
	RuleCardinality            = "cardinality"
	RuleEdgeAttrUnknown        = "edge_attr_unknown"
	RuleEdgeAttrMissing        = "edge_attr_missing"
	RuleEdgeAttrInvalid        = "edge_attr_invalid"
	RuleIDMismatch             = "id_mismatch"
	RuleKindMismatch           = "kind_mismatch"
	RuleUnknownField           = "unknown_field"
	RuleEdgeKindMismatch       = "edge_kind_mismatch"
	RuleDeprecatedField        = "deprecated_field"
	RuleUnknownFieldValue      = "unknown_field_value"
	RuleMissingField           = "missing_field"
	RuleStaleTarget            = "stale_target"
	RulePathMismatch           = "path_mismatch"
	RuleModalityConflict       = "modality_conflict"
	RuleExceptsStrict          = "excepts_strict"
	RuleImmutableViolation     = "immutable_violation"
	RuleOrphanMust             = "orphan_must"
	RuleOrphanTest             = "orphan_test"
	RuleStalePremise           = "stale_premise"
	RuleDeviationPressure      = "deviation_pressure"
	RuleNoCounterexample       = "no_counterexample"
	RuleMayWithoutInterop      = "may_without_interop"
	RuleInteropNotMust         = "interop_not_must"
)

// Node is one managed document. Line and KeyLines are the frontmatter
// positions the parser recorded, so a finding can point at the key it is about.
// Kind names the document kind the corpus declares it under, and is empty on a
// single-kind corpus, which is every corpus that declares no kinds at all.
type Node struct {
	ID       ID             `json:"id"`
	Path     string         `json:"path"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Date     string         `json:"date"`
	Kind     string         `json:"kind,omitempty"`
	Attrs    map[string]any `json:"-"`
	Line     int            `json:"-"`
	KeyLines map[string]int `json:"-"`
}

// Location returns where a finding about this document belongs: the line of
// the first named frontmatter key that the document carries, or the opening
// delimiter when it carries none of them.
func (n *Node) Location(keys ...string) Location {
	return Locate(n.Path, n.Line, n.KeyLines, keys...)
}

// Locate resolves a finding position inside a file: the line of the first
// named key the file carries, or fallback when it carries none of them.
func Locate(path string, fallback int, lines map[string]int, keys ...string) Location {
	loc := Location{Path: path, Line: fallback}
	for _, key := range keys {
		if line, ok := lines[key]; ok {
			loc.Line = line
			break
		}
	}
	return loc
}

// Attr reports the scalar frontmatter value stored under key. A list or mapping
// value is not scalar and reports absent, so rules never match on it.
func (n *Node) Attr(key string) (string, bool) {
	raw, ok := n.Attrs[key]
	if !ok || raw == nil {
		return "", false
	}
	return scalar(raw)
}

// AttrList reports the frontmatter value under key as a list of strings. A
// scalar is a one-element list, so a rule reads both shapes the same way; an
// item that is not a scalar has no string form and is dropped.
func (n *Node) AttrList(key string) ([]string, bool) {
	raw, ok := n.Attrs[key]
	if !ok || raw == nil {
		return nil, false
	}
	items, isList := raw.([]any)
	if !isList {
		value, ok := scalar(raw)
		if !ok {
			return nil, false
		}
		return []string{value}, true
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := scalar(item); ok {
			values = append(values, value)
		}
	}
	return values, true
}

func scalar(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return value, true
	case bool, int, int64, uint64, float64:
		return fmt.Sprint(value), true
	}
	return "", false
}

// Edge is one directed relation between two nodes. Reference-layer edges carry
// an empty Type and OriginReference. Attrs holds the attributes the edge was
// declared with, normalized to the canonical string form of the value written
// down, and is empty for an edge whose spec declares none — which every derived
// and every reference edge is.
type Edge struct {
	From   ID                `json:"from"`
	To     ID                `json:"to"`
	Type   EdgeType          `json:"type"`
	Origin Origin            `json:"origin"`
	Attrs  map[string]string `json:"attrs,omitempty"`
}

// Equal compares two edges by value. Attributes make an Edge uncomparable with
// ==, so every comparison goes through this method, and an absent attribute set
// equals an empty one: an edge carrying no attributes was written the same way
// whichever of the two the builder happened to record.
func (e Edge) Equal(other Edge) bool {
	return e.From == other.From && e.To == other.To &&
		e.Type == other.Type && e.Origin == other.Origin &&
		maps.Equal(e.Attrs, other.Attrs)
}

// Attr reports the value of one edge attribute.
func (e Edge) Attr(key string) (string, bool) {
	value, ok := e.Attrs[key]
	return value, ok
}

// Location is a position in a file. Line and Column are 1-based; zero means
// the position is unknown.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// Finding is one validation result. Location is where the reader should look;
// Related names the other files the finding involves.
//
// Suppressed marks a finding the corpus has already answered — today only a
// modality_conflict a recorded exception defeats. It is reported rather than
// dropped at the source, so `validate --show-suppressed` can show what the
// exception is holding down, and its Detail says which edge does the holding.
// A suppressed finding is out of the summary counts and therefore out of the
// exit code: it is a record of a decision, not an open failure.
type Finding struct {
	Severity   Severity   `json:"severity"`
	Rule       string     `json:"rule"`
	ID         ID         `json:"id"`
	Detail     string     `json:"detail"`
	Location   Location   `json:"location"`
	Related    []Location `json:"related,omitempty"`
	Fix        string     `json:"fix,omitempty"`
	Suppressed bool       `json:"suppressed,omitempty"`
}

// Summary is the aggregate reported alongside validation findings.
type Summary struct {
	Documents int `json:"documents"`
	Edges     int `json:"edges"`
	Errors    int `json:"errors"`
	Warnings  int `json:"warnings"`
	Cycles    int `json:"cycles"`
}

// Graph is the two-layer document graph: typed constraint edges plus the
// untyped reference layer extracted from document bodies.
type Graph struct {
	Nodes    map[ID]*Node
	Edges    []Edge
	RefEdges []Edge
	Findings []Finding
}

// NewGraph returns an empty graph ready for population.
func NewGraph() *Graph {
	return &Graph{Nodes: make(map[ID]*Node)}
}

// Node looks up a node by identifier.
func (g *Graph) Node(id ID) (*Node, bool) {
	n, ok := g.Nodes[id]
	return n, ok
}

// NodeIDs returns every node identifier in ascending order.
func (g *Graph) NodeIDs() []ID {
	ids := make([]ID, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// EdgesOfType returns the typed edges of one type, or every typed edge when t
// is empty. Edges keep the order they hold on the graph.
func (g *Graph) EdgesOfType(t EdgeType) []Edge {
	if t == "" {
		return slices.Clone(g.Edges)
	}
	edges := make([]Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if e.Type == t {
			edges = append(edges, e)
		}
	}
	return edges
}
