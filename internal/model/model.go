// Package model holds the vocabulary shared by every DocDag layer: document
// identity, the typed constraint graph, and validation findings.
package model

import (
	"fmt"
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

// Finding severities.
const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
)

// Rank orders severities for deterministic output, errors first.
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
)

// Node is one managed document. Line and KeyLines are the frontmatter
// positions the parser recorded, so a finding can point at the key it is about.
type Node struct {
	ID       ID             `json:"id"`
	Path     string         `json:"path"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Date     string         `json:"date"`
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
	switch value := raw.(type) {
	case string:
		return value, true
	case bool, int, int64, uint64, float64:
		return fmt.Sprint(value), true
	}
	return "", false
}

// Edge is one directed relation between two nodes. Reference-layer edges carry
// an empty Type and OriginReference.
type Edge struct {
	From   ID       `json:"from"`
	To     ID       `json:"to"`
	Type   EdgeType `json:"type"`
	Origin Origin   `json:"origin"`
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
type Finding struct {
	Severity Severity   `json:"severity"`
	Rule     string     `json:"rule"`
	ID       ID         `json:"id"`
	Detail   string     `json:"detail"`
	Location Location   `json:"location"`
	Related  []Location `json:"related,omitempty"`
	Fix      string     `json:"fix,omitempty"`
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
