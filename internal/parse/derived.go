package parse

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Kaikei-e/DocDag/config"
)

// patterns caches compiled derived-edge patterns: a corpus applies the same
// handful of patterns once per document, and compiling dominates matching.
var patterns sync.Map

// pattern compiles a derived-edge pattern once per process.
func pattern(expr string) (*regexp.Regexp, bool) {
	if cached, ok := patterns.Load(expr); ok {
		compiled, ok := cached.(*regexp.Regexp)
		return compiled, ok
	}
	compiled, err := regexp.Compile(expr)
	if err != nil {
		patterns.Store(expr, nil)
		return nil, false
	}
	patterns.Store(expr, compiled)
	return compiled, true
}

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
	compiled, ok := pattern(spec.Pattern)
	if !ok {
		return "", false
	}
	match := compiled.FindStringSubmatch(value)
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
