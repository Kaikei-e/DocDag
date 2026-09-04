// Package graph builds the two-layer document graph and answers every question
// asked of it: invariants, reachability, resolution and degree statistics.
package graph

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// Frontmatter keys the engine recognizes beyond the configured status field.
const (
	attrTitle = config.KeyTitle
	attrDate  = config.KeyDate
	attrKind  = config.KeyKind
)

type edgeKey struct {
	from model.ID
	to   model.ID
	t    model.EdgeType
}

// refKey identifies one reference-layer link as a reader sees it: what it
// names, how it was written and where.
type refKey struct {
	kind   parse.LinkKind
	target string
	line   int
}

// Build assembles the typed constraint layer and the untyped reference layer
// from parsed documents, recording structural findings it observes on the way.
func Build(docs []*parse.Document, cfg config.Config) *model.Graph {
	g := model.NewGraph()
	normalizer := cfg.Normalizer()
	findings := CheckDocuments(docs, cfg)
	docs = identified(docs)

	for _, doc := range docs {
		g.Nodes[doc.ID] = buildNode(doc, cfg)
	}

	// The kinds an edge may point at resolve its references first, so two kinds
	// whose patterns overlap never make an edge ambiguous. One normalizer per
	// edge type is enough: which kinds an edge reaches is a property of the
	// configuration, not of the document that wrote the reference down.
	targets := make(map[string]config.IDNormalizer, len(cfg.Edges))
	for _, spec := range cfg.Edges {
		targets[spec.Name] = cfg.EdgeNormalizer(spec)
	}

	records := make(map[edgeKey]edgeRecord)
	for _, doc := range docs {
		for _, spec := range cfg.Edges {
			t := model.EdgeType(spec.Name)
			entries, invalid := edgeEntries(doc, spec)
			if declaresNothing(doc, spec.Key, len(entries), len(invalid)) {
				findings = append(findings, emptyEdge(cfg, doc, spec.Key))
			}
			if spec.Inverse != "" {
				// An inverse key mirrors edges rather than declaring them, so it
				// takes plain references whatever the edge's attributes are.
				mirrored, bad := parse.Refs(doc.Frontmatter, spec.Inverse)
				if declaresNothing(doc, spec.Inverse, len(mirrored), len(bad)) {
					findings = append(findings, emptyEdge(cfg, doc, spec.Inverse))
				}
			}
			for _, entry := range invalid {
				findings = append(findings, unresolvableRef(cfg, doc, spec.Key, t, entry))
			}
			for _, entry := range entries {
				// Attributes describe what a document wrote down, so they are
				// checked whatever the reference beside them resolves to: a
				// missing reason is worth reporting on an entry whose target is
				// itself a finding.
				attrs, attrFindings := edgeAttrs(cfg, doc, spec, entry)
				findings = append(findings, attrFindings...)
				if !cfg.IDShaped(entry.Ref) {
					findings = append(findings, invalidRef(cfg, doc, spec.Key, t, entry.Ref))
					continue
				}
				target, ok := targets[spec.Name].Normalize(entry.Ref)
				if !ok {
					findings = append(findings, unresolvableRef(cfg, doc, spec.Key, t, entry.Ref))
					continue
				}
				recordEdge(records, doc.ID, target, t, spec.Direction, model.OriginStructured, attrs)
			}
		}
		for _, derived := range parse.Derived(doc, cfg) {
			t := model.EdgeType(derived.Spec.Edge)
			target, ok := normalizer.Normalize(derived.Target)
			if !ok {
				findings = append(findings, unresolvableRef(cfg, doc, derived.Field, t, derived.Target))
				continue
			}
			// A derived edge comes from a field value rather than an entry, so
			// there is nowhere to write an attribute down: it carries none.
			recordEdge(records, doc.ID, target, t, derived.Spec.Direction, model.OriginDerived, nil)
		}
	}
	g.Edges = sortedEdges(records)

	severity, validated := cfg.ReferenceSeverity()
	refs := make(map[edgeKey]bool)
	for _, doc := range docs {
		// One link written twice on a line is one thing to fix, and a second
		// byte-identical finding only costs a reader time.
		reported := make(map[refKey]bool)
		for _, link := range referenceLinks(doc, cfg) {
			ref, ok := referenceTarget(cfg, link)
			if !ok {
				continue
			}
			target, ok := normalizer.Normalize(ref)
			if !ok {
				continue
			}
			if _, known := g.Nodes[target]; !known {
				key := refKey{kind: link.Kind, target: ref, line: link.Line}
				if validated && !reported[key] {
					reported[key] = true
					findings = append(findings, danglingReference(doc, link, ref, severity))
				}
				continue
			}
			refs[edgeKey{from: doc.ID, to: target}] = true
		}
	}
	g.RefEdges = sortedReferenceEdges(refs)

	SortFindings(findings)
	g.Findings = findings
	return g
}

// identified returns the documents that carry an identity. A document without
// one is reported by CheckDocuments and left out of the graph: it has no key to
// be stored under, and storing it under the empty identifier would let a
// reference to nothing resolve.
func identified(docs []*parse.Document) []*parse.Document {
	out := make([]*parse.Document, 0, len(docs))
	for _, doc := range docs {
		if doc.ID != "" {
			out = append(out, doc)
		}
	}
	return out
}

func buildNode(doc *parse.Document, cfg config.Config) *model.Node {
	n := &model.Node{
		ID:       doc.ID,
		Path:     doc.Path,
		Kind:     doc.Kind,
		Attrs:    make(map[string]any, len(doc.Frontmatter)),
		Line:     doc.FrontmatterLine,
		KeyLines: doc.KeyLines,
	}
	for key, value := range doc.Frontmatter {
		n.Attrs[key] = value
	}
	n.Title, _ = parse.Attr(doc.Frontmatter, attrTitle)
	n.Date, _ = parse.Attr(doc.Frontmatter, attrDate)
	// A document's kind is the directory's answer, not the frontmatter's: the
	// directory chose the identity rules the document was read under, and a
	// frontmatter key that disagrees is the kind_mismatch finding rather than a
	// second opinion rules could read.
	if doc.Kind != "" {
		n.Attrs[attrKind] = doc.Kind
	}

	field := statusField(cfg)
	raw, ok := parse.Attr(doc.Frontmatter, field)
	if !ok {
		return n
	}
	status, _ := canonicalKindStatus(cfg, doc.Kind, raw)
	n.Status = status
	// Rules read the attribute and the checks read the field, so a projected
	// MADR "superseded by 0003" status has to land on both.
	n.Attrs[field] = status
	return n
}

// edgeRecord is one edge as the builder has it so far: how it entered the graph
// and the attributes the entry that declared it carried.
type edgeRecord struct {
	origin model.Origin
	attrs  map[string]string
}

func recordEdge(records map[edgeKey]edgeRecord, doc, target model.ID, t model.EdgeType, direction string, origin model.Origin, attrs map[string]string) {
	from, to := doc, target
	if direction == config.DirectionReverse {
		from, to = target, doc
	}
	k := edgeKey{from: from, to: to, t: t}
	// One relation declared twice is one edge, and it keeps what its first
	// structured declaration said. Documents are built in name order and a
	// frontmatter list keeps the order it was written in, so first-wins names
	// the same entry on every run; a derived edge still yields to a structured
	// one, which is why only a structured predecessor stops the write.
	if previous, ok := records[k]; ok && previous.origin == model.OriginStructured {
		return
	}
	records[k] = edgeRecord{origin: origin, attrs: attrs}
}

func unresolvableRef(cfg config.Config, doc *parse.Document, key string, t model.EdgeType, ref string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleDanglingRef),
		Rule:     model.RuleDanglingRef,
		ID:       doc.ID,
		Detail:   danglingDetail(t, ref),
		Location: model.Locate(doc.Path, doc.FrontmatterLine, doc.KeyLines, key),
	}
}

// declaresNothing reports an edge key written down and then left empty, which
// reads as a declared relation but builds no edge.
func declaresNothing(doc *parse.Document, key string, entries, invalid int) bool {
	_, present := doc.Frontmatter[key]
	return present && entries == 0 && invalid == 0
}

func emptyEdge(cfg config.Config, doc *parse.Document, key string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleEmptyEdge),
		Rule:     model.RuleEmptyEdge,
		ID:       doc.ID,
		Detail:   fmt.Sprintf("%s is present but names no document", key),
		Location: model.Locate(doc.Path, doc.FrontmatterLine, doc.KeyLines, key),
	}
}

func invalidRef(cfg config.Config, doc *parse.Document, key string, t model.EdgeType, ref string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleInvalidRef),
		Rule:     model.RuleInvalidRef,
		ID:       doc.ID,
		Detail:   fmt.Sprintf("%s reference %q is not an identifier", t, ref),
		Location: model.Locate(doc.Path, doc.FrontmatterLine, doc.KeyLines, key),
	}
}

func danglingReference(doc *parse.Document, link parse.Link, ref string, severity model.Severity) model.Finding {
	return model.Finding{
		Severity: severity,
		Rule:     model.RuleDanglingReference,
		ID:       doc.ID,
		Detail:   fmt.Sprintf("%s reference %q does not name a document", link.Kind, ref),
		Location: model.Location{Path: doc.Path, Line: link.Line},
	}
}

// referenceLinks returns every link of a document that feeds the reference
// layer, each carrying the file line it was written on.
func referenceLinks(doc *parse.Document, cfg config.Config) []parse.Link {
	var links []parse.Link
	if cfg.Scans(config.ScanBody) {
		for _, link := range parse.Links(doc.Body) {
			link.Line += doc.BodyLine - 1
			links = append(links, link)
		}
	}
	if !cfg.Scans(config.ScanFrontmatter) {
		return links
	}
	for _, key := range slices.Sorted(maps.Keys(doc.Frontmatter)) {
		for _, value := range scalarValues(doc.Frontmatter[key]) {
			for _, link := range parse.Links(value) {
				if link.Kind != parse.LinkWiki {
					continue
				}
				link.Line = doc.KeyLines[key]
				links = append(links, link)
			}
		}
	}
	return links
}

// scalarValues renders the strings a frontmatter value holds: the value itself,
// or every string item of a list.
func scalarValues(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// referenceTarget reports the raw reference a body link names, and whether the
// link is an identity reference at all: prose links carry no identity, and only
// a managed file name makes a Markdown link one.
func referenceTarget(cfg config.Config, link parse.Link) (string, bool) {
	if link.Kind == parse.LinkMarkdown {
		return link.Target, config.IsDocumentLink(link.Target)
	}
	ref, _, _ := strings.Cut(link.Target, "#")
	ref = strings.TrimSpace(ref)
	return ref, cfg.IsReference(ref)
}

func sortedEdges(records map[edgeKey]edgeRecord) []model.Edge {
	edges := make([]model.Edge, 0, len(records))
	for k, record := range records {
		edges = append(edges, model.Edge{From: k.from, To: k.to, Type: k.t, Origin: record.origin, Attrs: record.attrs})
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

func statusField(cfg config.Config) string { return cfg.EffectiveStatus() }

// canonicalStatus collapses a status onto the configured vocabulary, under the
// top-level vocabulary a single-kind corpus has.
func canonicalStatus(cfg config.Config, raw string) (string, bool) {
	return canonicalKindStatus(cfg, "", raw)
}

// canonicalKindStatus collapses a status onto the vocabulary its kind answers
// to: a MADR "superseded by 0003" string becomes "superseded". Only a value a
// configured derived-edge pattern claims may collapse, so prose that merely
// opens with a vocabulary word stays unknown. A value the vocabulary does not
// cover comes back unchanged and unknown.
func canonicalKindStatus(cfg config.Config, kind, raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	vocabulary := cfg.KindStatusValues(kind)
	for _, known := range vocabulary {
		if strings.EqualFold(value, known) {
			return value, true
		}
	}
	if !derivesEdge(cfg, value) {
		return value, false
	}
	for _, known := range vocabulary {
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
