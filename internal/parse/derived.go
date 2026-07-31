package parse

import "github.com/Kaikei-e/DocDag/internal/config"

// DerivedEdge is an edge inferred from a frontmatter field value instead of a
// declared edge key, such as the MADR status string "superseded by 0003".
type DerivedEdge struct {
	Spec   config.DerivedEdgeSpec
	Field  string
	Value  string
	Target string
}

// MatchDerived applies one derived-edge pattern to a field value and returns
// the captured reference.
func MatchDerived(value string, spec config.DerivedEdgeSpec) (string, bool) { return "", false }

// Derived applies every configured derived-edge pattern to a document.
func Derived(doc *Document, cfg config.Config) []DerivedEdge { return nil }
