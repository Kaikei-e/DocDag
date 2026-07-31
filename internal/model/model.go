// Package model holds the vocabulary shared by every DocDag layer: document
// identity, the typed constraint graph, and validation findings.
package model

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
func (s Severity) Rank() int { return 0 }

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
)

// Node is one managed document.
type Node struct {
	ID     ID             `json:"id"`
	Path   string         `json:"path"`
	Title  string         `json:"title"`
	Status string         `json:"status"`
	Date   string         `json:"date"`
	Attrs  map[string]any `json:"-"`
}

// Attr reports the scalar frontmatter value stored under key.
func (n *Node) Attr(key string) (string, bool) { return "", false }

// Edge is one directed relation between two nodes. Reference-layer edges carry
// an empty Type and OriginReference.
type Edge struct {
	From   ID       `json:"from"`
	To     ID       `json:"to"`
	Type   EdgeType `json:"type"`
	Origin Origin   `json:"origin"`
}

// Finding is one validation result.
type Finding struct {
	Severity Severity `json:"severity"`
	Rule     string   `json:"rule"`
	ID       ID       `json:"id"`
	Detail   string   `json:"detail"`
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
func NewGraph() *Graph { return nil }

// Node looks up a node by identifier.
func (g *Graph) Node(id ID) (*Node, bool) { return nil, false }

// NodeIDs returns every node identifier in ascending order.
func (g *Graph) NodeIDs() []ID { return nil }

// EdgesOfType returns the typed edges of one type, or every typed edge when t
// is empty.
func (g *Graph) EdgesOfType(t EdgeType) []Edge { return nil }
