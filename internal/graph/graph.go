// Package graph builds the two-layer document graph and answers every question
// asked of it: invariants, reachability, resolution and degree statistics.
package graph

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// Frontmatter keys the engine recognizes beyond the configured status field.
const (
	attrTitle = "title"
	attrDate  = "date"
)

type edgeKey struct {
	from model.ID
	to   model.ID
	t    model.EdgeType
}

// Build assembles the typed constraint layer and the untyped reference layer
// from parsed documents, recording structural findings it observes on the way.
func Build(docs []*parse.Document, cfg config.Config) (*model.Graph, error) {
	g := model.NewGraph()
	normalizer := cfg.Normalizer()
	findings := CheckDocuments(docs, cfg)

	for _, doc := range docs {
		g.Nodes[doc.ID] = buildNode(doc, cfg)
	}

	origins := make(map[edgeKey]model.Origin)
	for _, doc := range docs {
		for _, spec := range cfg.Edges {
			t := model.EdgeType(spec.Name)
			refs, invalid := parse.Refs(doc.Frontmatter, spec.Key)
			for _, entry := range invalid {
				findings = append(findings, unresolvableRef(doc.ID, t, entry))
			}
			for _, ref := range refs {
				target, ok := normalizer.Normalize(ref)
				if !ok {
					findings = append(findings, unresolvableRef(doc.ID, t, ref))
					continue
				}
				recordEdge(origins, doc.ID, target, t, spec.Direction, model.OriginStructured)
			}
		}
		for _, derived := range parse.Derived(doc, cfg) {
			t := model.EdgeType(derived.Spec.Edge)
			target, ok := normalizer.Normalize(derived.Target)
			if !ok {
				findings = append(findings, unresolvableRef(doc.ID, t, derived.Target))
				continue
			}
			recordEdge(origins, doc.ID, target, t, derived.Spec.Direction, model.OriginDerived)
		}
	}
	g.Edges = sortedEdges(origins)

	refs := make(map[edgeKey]bool)
	for _, doc := range docs {
		for _, link := range parse.Links(doc.Body) {
			target, ok := normalizer.Normalize(link.Target)
			if !ok {
				continue
			}
			if _, known := g.Nodes[target]; !known {
				continue
			}
			refs[edgeKey{from: doc.ID, to: target}] = true
		}
	}
	g.RefEdges = sortedReferenceEdges(refs)

	SortFindings(findings)
	g.Findings = findings
	return g, nil
}

func buildNode(doc *parse.Document, cfg config.Config) *model.Node {
	n := &model.Node{
		ID:    doc.ID,
		Path:  doc.Path,
		Attrs: make(map[string]any, len(doc.Frontmatter)),
	}
	for key, value := range doc.Frontmatter {
		n.Attrs[key] = value
	}
	n.Title, _ = parse.Attr(doc.Frontmatter, attrTitle)
	n.Date, _ = parse.Attr(doc.Frontmatter, attrDate)

	field := statusField(cfg)
	raw, ok := parse.Attr(doc.Frontmatter, field)
	if !ok {
		return n
	}
	status, _ := canonicalStatus(cfg, raw)
	n.Status = status
	// Rules read the attribute and the checks read the field, so a projected
	// MADR "superseded by 0003" status has to land on both.
	n.Attrs[field] = status
	return n
}

func recordEdge(origins map[edgeKey]model.Origin, doc, target model.ID, t model.EdgeType, direction string, origin model.Origin) {
	from, to := doc, target
	if direction == config.DirectionReverse {
		from, to = target, doc
	}
	k := edgeKey{from: from, to: to, t: t}
	if previous, ok := origins[k]; ok && previous == model.OriginStructured {
		return
	}
	origins[k] = origin
}

func unresolvableRef(id model.ID, t model.EdgeType, ref string) model.Finding {
	return model.Finding{
		Severity: model.SeverityError,
		Rule:     model.RuleDanglingRef,
		ID:       id,
		Detail:   fmt.Sprintf("%s reference %q does not name a document", t, ref),
	}
}

func sortedEdges(origins map[edgeKey]model.Origin) []model.Edge {
	edges := make([]model.Edge, 0, len(origins))
	for k, origin := range origins {
		edges = append(edges, model.Edge{From: k.from, To: k.to, Type: k.t, Origin: origin})
	}
	slices.SortFunc(edges, compareEdges)
	return edges
}

func sortedReferenceEdges(refs map[edgeKey]bool) []model.Edge {
	edges := make([]model.Edge, 0, len(refs))
	for k := range refs {
		edges = append(edges, model.Edge{From: k.from, To: k.to, Origin: model.OriginReference})
	}
	slices.SortFunc(edges, compareEdges)
	return edges
}

func compareEdges(a, b model.Edge) int {
	if c := strings.Compare(string(a.From), string(b.From)); c != 0 {
		return c
	}
	if c := strings.Compare(string(a.Type), string(b.Type)); c != 0 {
		return c
	}
	return strings.Compare(string(a.To), string(b.To))
}

// Adjacency returns from-to adjacency over the given edge types, or over every
// typed edge when none are given. Neighbour lists are sorted.
func Adjacency(g *model.Graph, types ...model.EdgeType) map[model.ID][]model.ID {
	return typedNeighbors(g, false, types)
}

// Reverse returns to-from adjacency over the given edge types, or over every
// typed edge when none are given. Neighbour lists are sorted.
func Reverse(g *model.Graph, types ...model.EdgeType) map[model.ID][]model.ID {
	return typedNeighbors(g, true, types)
}

func typedNeighbors(g *model.Graph, reverse bool, types []model.EdgeType) map[model.ID][]model.ID {
	adj := make(map[model.ID][]model.ID, len(g.Nodes))
	for id := range g.Nodes {
		adj[id] = nil
	}
	for _, e := range g.Edges {
		if e.Origin == model.OriginReference || !matchesType(e.Type, types) {
			continue
		}
		from, to := e.From, e.To
		if reverse {
			from, to = to, from
		}
		adj[from] = append(adj[from], to)
	}
	return sortNeighbors(adj)
}

// ReferenceAdjacency returns adjacency over the reference layer only.
func ReferenceAdjacency(g *model.Graph) map[model.ID][]model.ID {
	adj := make(map[model.ID][]model.ID, len(g.Nodes))
	for id := range g.Nodes {
		adj[id] = nil
	}
	for _, e := range g.RefEdges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	return sortNeighbors(adj)
}

// retainKnown drops neighbours the corpus does not hold, in place. A walk over
// the result can only ever name documents that exist.
func retainKnown(g *model.Graph, adj map[model.ID][]model.ID) map[model.ID][]model.ID {
	for id, neighbors := range adj {
		known := make([]model.ID, 0, len(neighbors))
		for _, next := range neighbors {
			if _, ok := g.Nodes[next]; ok {
				known = append(known, next)
			}
		}
		adj[id] = known
	}
	return adj
}

func sortNeighbors(adj map[model.ID][]model.ID) map[model.ID][]model.ID {
	for id, list := range adj {
		if len(list) < 2 {
			continue
		}
		slices.Sort(list)
		adj[id] = slices.Compact(list)
	}
	return adj
}

func matchesType(t model.EdgeType, types []model.EdgeType) bool {
	return len(types) == 0 || slices.Contains(types, t)
}

func statusField(cfg config.Config) string {
	if cfg.StatusField == "" {
		return config.DefaultStatusField
	}
	return cfg.StatusField
}

// canonicalStatus collapses a status onto the configured vocabulary: a MADR
// "superseded by 0003" string becomes "superseded". Only a value a configured
// derived-edge pattern claims may collapse, so prose that merely opens with a
// vocabulary word stays unknown. A value the vocabulary does not cover comes
// back unchanged and unknown.
func canonicalStatus(cfg config.Config, raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	for _, known := range cfg.StatusValues {
		if strings.EqualFold(value, known) {
			return value, true
		}
	}
	if !derivesEdge(cfg, value) {
		return value, false
	}
	for _, known := range cfg.StatusValues {
		if len(value) <= len(known) || !strings.EqualFold(value[:len(known)], known) {
			continue
		}
		if separator := value[len(known)]; separator == ' ' || separator == '-' {
			return known, true
		}
	}
	return value, false
}

// derivesEdge reports whether a status value produces a derived edge, which is
// what earns it the right to project onto the vocabulary word it opens with.
func derivesEdge(cfg config.Config, value string) bool {
	field := statusField(cfg)
	for _, spec := range cfg.DerivedEdges {
		if spec.Field != field {
			continue
		}
		if _, ok := parse.MatchDerived(value, spec); ok {
			return true
		}
	}
	return false
}
