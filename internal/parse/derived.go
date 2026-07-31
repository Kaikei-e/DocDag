package parse

import (
	"regexp"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
)

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
func MatchDerived(value string, spec config.DerivedEdgeSpec) (string, bool) {
	if value == "" || spec.Pattern == "" {
		return "", false
	}
	pattern, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return "", false
	}
	match := pattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return "", false
	}
	ref := strings.TrimSpace(match[1])
	if ref == "" {
		return "", false
	}
	return ref, true
}

// Derived applies every configured derived-edge pattern to a document.
func Derived(doc *Document, cfg config.Config) []DerivedEdge {
	if doc == nil || len(doc.Frontmatter) == 0 {
		return nil
	}
	var derived []DerivedEdge
	for _, spec := range cfg.DerivedEdges {
		value, ok := Attr(doc.Frontmatter, spec.Field)
		if !ok {
			continue
		}
		target, ok := MatchDerived(value, spec)
		if !ok {
			continue
		}
		derived = append(derived, DerivedEdge{Spec: spec, Field: spec.Field, Value: value, Target: target})
	}
	return derived
}
