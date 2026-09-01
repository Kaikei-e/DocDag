package graph

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// CheckDocuments reports the file-level structural findings that the graph
// container cannot express: id collisions, undecodable and absent frontmatter.
func CheckDocuments(docs []*parse.Document, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	paths := make(map[model.ID][]string, len(docs))
	for _, doc := range docs {
		// A file in a kind's directory that yields no identity is reported
		// rather than skipped: the directory is what declares it a document of
		// that kind, so a name no identity rule accepts is a mistake, not
		// another tool's file. A single-kind corpus keeps deciding by the file
		// name alone, and parse.Dir hands over nothing it rejected.
		if doc.ID == "" && cfg.Multikind() {
			// A block that does not decode is why the identity could not be
			// read; reporting a missing identifier on top of it would name a
			// symptom and bury the cause.
			if doc.Err != nil {
				findings = append(findings, invalidFrontmatter(cfg, doc))
				continue
			}
			findings = append(findings, idMismatch(cfg, doc))
			continue
		}
		findings = append(findings, kindFindings(cfg, doc)...)
		paths[doc.ID] = append(paths[doc.ID], doc.Path)
		switch {
		case doc.Err != nil:
			findings = append(findings, invalidFrontmatter(cfg, doc))
		case !doc.HasFrontmatter && doc.MatchesPattern:
			findings = append(findings, model.Finding{
				Severity: cfg.Severity(model.RuleMissingFrontmatter),
				Rule:     model.RuleMissingFrontmatter,
				ID:       doc.ID,
				Detail:   "no frontmatter block",
				Location: model.Location{Path: doc.Path, Line: firstFileLine},
			})
		}
	}

	for id, colliding := range paths {
		if len(colliding) < 2 {
			continue
		}
		findings = append(findings, idCollision(cfg, id, colliding))
	}

	SortFindings(findings)
	return findings
}

// firstFileLine is where a finding about a whole file points.
const firstFileLine = 1

// documentLocation points a finding at one of a document's frontmatter keys,
// falling back to the opening delimiter, and to the first line for a file that
// carries no frontmatter block at all.
func documentLocation(doc *parse.Document, keys ...string) model.Location {
	fallback := doc.FrontmatterLine
	if fallback == 0 {
		fallback = firstFileLine
	}
	return model.Locate(doc.Path, fallback, doc.KeyLines, keys...)
}

// idMismatch reports a document whose identity token no identity rule of its
// kind accepts. It names the pattern the kind declares, because that is what
// the token has to be rewritten to satisfy — or, for a pattern no file name can
// carry, what the frontmatter id has to be written as.
func idMismatch(cfg config.Config, doc *parse.Document) model.Finding {
	detail := fmt.Sprintf("%q is not an identifier of kind %q", doc.Identity, doc.Kind)
	if spec, ok := cfg.Kind(doc.Kind); ok && spec.ID != "" {
		detail += fmt.Sprintf(", which reads %s", spec.ID)
	}
	return model.Finding{
		Severity: cfg.Severity(model.RuleIDMismatch),
		Rule:     model.RuleIDMismatch,
		Detail:   detail,
		Location: documentLocation(doc, config.KeyID),
	}
}

// kindFindings reports what a document's kind says about its frontmatter: a
// written kind that disagrees with the directory it lives in, and, where the
// kind is closed, every key the configuration does not know.
func kindFindings(cfg config.Config, doc *parse.Document) []model.Finding {
	findings := []model.Finding{}
	if doc.Kind == "" {
		return findings
	}
	if written, ok := parse.Attr(doc.Frontmatter, config.KeyKind); ok && strings.TrimSpace(written) != doc.Kind {
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleKindMismatch),
			Rule:     model.RuleKindMismatch,
			ID:       doc.ID,
			Detail:   fmt.Sprintf("frontmatter kind %q disagrees with directory kind %q", strings.TrimSpace(written), doc.Kind),
			Location: documentLocation(doc, config.KeyKind),
		})
	}
	spec, ok := cfg.Kind(doc.Kind)
	if !ok || !spec.Closed {
		return findings
	}
	known := cfg.KnownFrontmatterKeys()
	for _, key := range slices.Sorted(maps.Keys(doc.Frontmatter)) {
		if slices.Contains(known, key) {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleUnknownField),
			Rule:     model.RuleUnknownField,
			ID:       doc.ID,
			Detail: fmt.Sprintf("frontmatter key %q is not declared by the closed kind %q, declared: %s",
				key, doc.Kind, strings.Join(known, ", ")),
			Location: documentLocation(doc, key),
		})
	}
	return findings
}

func invalidFrontmatter(cfg config.Config, doc *parse.Document) model.Finding {
	f := model.Finding{
		Severity: cfg.Severity(model.RuleInvalidFrontmatter),
		Rule:     model.RuleInvalidFrontmatter,
		ID:       doc.ID,
		Detail:   doc.Err.Error(),
		Location: model.Location{Path: doc.Path, Line: doc.FrontmatterLine},
	}
	var located *parse.FrontmatterError
	if errors.As(doc.Err, &located) {
		f.Detail = located.Message
		f.Location.Line, f.Location.Column = located.Line, located.Column
	}
	return f
}

func idCollision(cfg config.Config, id model.ID, colliding []string) model.Finding {
	sorted := slices.Sorted(slices.Values(colliding))
	related := make([]model.Location, 0, len(sorted)-1)
	for _, path := range sorted[1:] {
		related = append(related, model.Location{Path: path, Line: firstFileLine})
	}
	return model.Finding{
		Severity: cfg.Severity(model.RuleIDCollision),
		Rule:     model.RuleIDCollision,
		ID:       id,
		Detail:   fmt.Sprintf("shares its identifier with %s", strings.Join(sorted[1:], ", ")),
		Location: model.Location{Path: sorted[0], Line: firstFileLine},
		Related:  related,
	}
}

// edgeKeyLocation points at the frontmatter key that declares an edge type,
// falling back to the field a derived edge reads it from and then to the
// status field.
func edgeKeyLocation(cfg config.Config, n *model.Node, types ...model.EdgeType) model.Location {
	keys := make([]string, 0, 2*len(types)+1)
	for _, t := range types {
		if spec, ok := cfg.Edge(t); ok {
			keys = append(keys, spec.Key)
		}
	}
	for _, t := range types {
		keys = append(keys, derivedFields(cfg, t)...)
	}
	return n.Location(append(keys, statusField(cfg))...)
}

// derivedFieldLocation points at the field whose value produced a derived
// edge, which is a different key from the one that would declare it.
func derivedFieldLocation(cfg config.Config, n *model.Node, t model.EdgeType) model.Location {
	return n.Location(append(derivedFields(cfg, t), statusField(cfg))...)
}

func edgeLocation(cfg config.Config, n *model.Node, e model.Edge) model.Location {
	if e.Origin == model.OriginDerived {
		return derivedFieldLocation(cfg, n, e.Type)
	}
	return edgeKeyLocation(cfg, n, e.Type)
}

func derivedFields(cfg config.Config, t model.EdgeType) []string {
	fields := make([]string, 0, len(cfg.DerivedEdges))
	for _, spec := range cfg.DerivedEdges {
		if spec.Edge == t.String() {
			fields = append(fields, spec.Field)
		}
	}
	return fields
}

// CheckCycles reports one finding per cycle found in an acyclic edge type, and
// per cycle that only the union of those types closes.
func CheckCycles(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	types := cfg.AcyclicEdgeTypes()
	for _, t := range types {
		for _, cycle := range FindCycles(Adjacency(g, t)) {
			findings = append(findings, cycleFinding(g, cfg, cycle,
				fmt.Sprintf("%s cycle: %s", t, joinIDs(cycle, " -> ")), t))
		}
	}
	if cfg.AcyclicUnion && len(types) > 1 {
		findings = append(findings, unionCycles(g, cfg, types)...)
	}
	SortFindings(findings)
	return findings
}

// unionCycles reports the cycles that need more than one edge type to close.
// A cycle inside a single type is already a finding of its own, and it may
// share a component with one that is not, so each component is searched for a
// cycle no single type covers rather than for any cycle at all.
func unionCycles(g *model.Graph, cfg config.Config, types []model.EdgeType) []model.Finding {
	adj := Adjacency(g, types...)
	labels, all := edgeLabels(g, types)
	findings := []model.Finding{}
	for _, component := range cyclicComponents(adj) {
		cycle := uncoveredCycle(inducedSubgraph(adj, component), labels, all)
		if cycle == nil {
			continue
		}
		findings = append(findings, cycleFinding(g, cfg, cycle,
			fmt.Sprintf("cycle over %s: %s", strings.Join(cycleTypes(g, cycle, types), ", "), joinIDs(cycle, " -> ")), types...))
	}
	return findings
}

// edgeLabels marks each arc with the edge types that carry it, one bit per
// type, so asking whether a single type covers a whole cycle is one bitwise
// intersection.
func edgeLabels(g *model.Graph, types []model.EdgeType) (labels map[edgeKey]uint, all uint) {
	labels = make(map[edgeKey]uint, len(g.Edges))
	for i, t := range types {
		bit := uint(1) << uint(i)
		all |= bit
		for _, e := range g.EdgesOfType(t) {
			labels[edgeKey{from: e.From, to: e.To}] |= bit
		}
	}
	return labels, all
}

// cycleTypes names the edge types a closed path travels on, in configuration
// order, so a reader knows which keys to look at.
func cycleTypes(g *model.Graph, cycle []model.ID, types []model.EdgeType) []string {
	used := make(map[model.EdgeType]bool, len(types))
	for i := 1; i < len(cycle); i++ {
		for _, e := range g.Edges {
			if e.From == cycle[i-1] && e.To == cycle[i] && slices.Contains(types, e.Type) {
				used[e.Type] = true
			}
		}
	}
	names := make([]string, 0, len(used))
	for _, t := range types {
		if used[t] {
			names = append(names, t.String())
		}
	}
	return names
}

// cycleFinding files a cycle against its lexically smallest member and relates
// the others, so a reader opens one file and sees every edge on the path.
func cycleFinding(g *model.Graph, cfg config.Config, cycle []model.ID, detail string, types ...model.EdgeType) model.Finding {
	f := model.Finding{
		Severity: cfg.Severity(model.RuleCycle),
		Rule:     model.RuleCycle,
		ID:       cycle[0],
		Detail:   detail,
	}
	members := slices.Compact(slices.Sorted(slices.Values(cycle)))
	locations := make([]model.Location, 0, len(members))
	for _, id := range members {
		if n, ok := g.Node(id); ok {
			locations = append(locations, edgeKeyLocation(cfg, n, types...))
		}
	}
	if len(locations) > 0 {
		f.Location = locations[0]
	}
	if len(locations) > 1 {
		f.Related = locations[1:]
	}
	return f
}

// CheckDangling reports typed edges with an endpoint that is not a known
// document. Either endpoint can be the unknown one: a reverse-direction edge,
// such as the MADR "superseded by <ref>" status, puts the referenced document
// at the source. The finding is filed against the document that declared it.
func CheckDangling(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, e := range g.Edges {
		_, fromKnown := g.Nodes[e.From]
		_, toKnown := g.Nodes[e.To]
		switch {
		case fromKnown && !toKnown:
			findings = append(findings, danglingRef(cfg, g.Nodes[e.From], e, e.To.String()))
		case toKnown && !fromKnown:
			findings = append(findings, danglingRef(cfg, g.Nodes[e.To], e, e.From.String()))
		}
	}
	SortFindings(findings)
	return findings
}

func danglingRef(cfg config.Config, owner *model.Node, e model.Edge, missing string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleDanglingRef),
		Rule:     model.RuleDanglingRef,
		ID:       owner.ID,
		Detail:   danglingDetail(e.Type, missing),
		Location: edgeLocation(cfg, owner, e),
	}
}

// danglingDetail is the single wording for a reference that names no document,
// whichever layer noticed it.
func danglingDetail(t model.EdgeType, ref string) string {
	return fmt.Sprintf("%s reference %q does not name a document", t, ref)
}

// CheckInverse reports frontmatter that disagrees with the edges it mirrors:
// an edge whose target does not name its source under the inverse key, and an
// entry under the inverse key that no edge backs.
func CheckInverse(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, spec := range cfg.Edges {
		if spec.Inverse == "" {
			continue
		}
		findings = append(findings, checkInverseKey(g, cfg, spec)...)
	}
	SortFindings(findings)
	return findings
}

func checkInverseKey(g *model.Graph, cfg config.Config, spec config.EdgeSpec) []model.Finding {
	findings := []model.Finding{}
	t := model.EdgeType(spec.Name)
	// An inverse key names the sources of the edges it mirrors, which the
	// edge's own to: kinds say nothing about, so every kind resolves them.
	normalizer := cfg.Normalizer()
	listed := make(map[edgeKey]bool)

	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		loc := n.Location(spec.Inverse, spec.Key, statusField(cfg))
		refs, invalid := parse.Refs(n.Attrs, spec.Inverse)
		for _, entry := range append(slices.Clone(invalid), unshaped(cfg, refs)...) {
			findings = append(findings, model.Finding{
				Severity: cfg.Severity(model.RuleInvalidRef),
				Rule:     model.RuleInvalidRef,
				ID:       id,
				Detail:   fmt.Sprintf("%s reference %q is not an identifier", spec.Inverse, entry),
				Location: loc,
			})
		}
		for _, ref := range refs {
			source, ok := normalizer.Normalize(ref)
			if !ok || !cfg.IDShaped(ref) {
				continue
			}
			if _, known := g.Node(source); !known {
				findings = append(findings, model.Finding{
					Severity: cfg.Severity(model.RuleDanglingRef),
					Rule:     model.RuleDanglingRef,
					ID:       id,
					Detail:   danglingDetail(model.EdgeType(spec.Inverse), ref),
					Location: loc,
				})
				continue
			}
			listed[edgeKey{from: source, to: id, t: t}] = true
		}
	}

	for _, e := range g.EdgesOfType(t) {
		if _, known := g.Node(e.To); !known {
			continue
		}
		if listed[edgeKey{from: e.From, to: e.To, t: t}] {
			continue
		}
		findings = append(findings, inverseMismatch(g, cfg, spec, e.To, e.From,
			fmt.Sprintf("%s does not list %s, which declares %s", spec.Inverse, e.From, spec.Key)))
	}

	declared := make(map[edgeKey]bool, len(g.Edges))
	for _, e := range g.EdgesOfType(t) {
		declared[edgeKey{from: e.From, to: e.To, t: t}] = true
	}
	for k := range listed {
		if declared[k] {
			continue
		}
		findings = append(findings, inverseMismatch(g, cfg, spec, k.to, k.from,
			fmt.Sprintf("%s lists %s, which declares no %s edge to this document", spec.Inverse, k.from, t)))
	}
	return findings
}

func inverseMismatch(g *model.Graph, cfg config.Config, spec config.EdgeSpec, owner, peer model.ID, detail string) model.Finding {
	f := model.Finding{
		Severity: cfg.Severity(model.RuleInverseMismatch),
		Rule:     model.RuleInverseMismatch,
		ID:       owner,
		Detail:   detail,
	}
	if n, ok := g.Node(owner); ok {
		f.Location = n.Location(spec.Inverse, spec.Key, statusField(cfg))
	}
	if n, ok := g.Node(peer); ok {
		f.Related = []model.Location{n.Location(spec.Key, statusField(cfg))}
	}
	return f
}

// unshaped returns the references that name no identity at all, under whatever
// identity rules the configuration holds.
func unshaped(cfg config.Config, refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !cfg.IDShaped(ref) {
			out = append(out, ref)
		}
	}
	return out
}

// CheckCardinality reports documents whose edge degree leaves the bounds the
// configuration puts on an edge type.
func CheckCardinality(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, spec := range cfg.Edges {
		if spec.MaxInbound == 0 && spec.MaxOutbound == 0 && spec.MinOutbound == 0 {
			continue
		}
		t := model.EdgeType(spec.Name)
		inbound, outbound := degrees(g, t)
		for _, id := range g.NodeIDs() {
			findings = append(findings, cardinality(g, cfg, spec, id, inbound[id], outbound[id])...)
		}
	}
	SortFindings(findings)
	return findings
}

// degrees counts the edges of one type at each known document. An edge with an
// unknown endpoint is a dangling reference, reported on its own; it still
// counts at the endpoint that exists.
func degrees(g *model.Graph, t model.EdgeType) (inbound, outbound map[model.ID]int) {
	inbound = make(map[model.ID]int, len(g.Nodes))
	outbound = make(map[model.ID]int, len(g.Nodes))
	for _, e := range g.EdgesOfType(t) {
		outbound[e.From]++
		inbound[e.To]++
	}
	return inbound, outbound
}

func cardinality(g *model.Graph, cfg config.Config, spec config.EdgeSpec, id model.ID, inbound, outbound int) []model.Finding {
	t := model.EdgeType(spec.Name)
	var details []string
	if spec.MaxInbound > 0 && inbound > spec.MaxInbound {
		details = append(details, fmt.Sprintf("%d inbound %s edges exceed max_inbound %d", inbound, t, spec.MaxInbound))
	}
	if spec.MaxOutbound > 0 && outbound > spec.MaxOutbound {
		details = append(details, fmt.Sprintf("%d outbound %s edges exceed max_outbound %d", outbound, t, spec.MaxOutbound))
	}
	if outbound < spec.MinOutbound {
		details = append(details, fmt.Sprintf("%d outbound %s edges fall short of min_outbound %d", outbound, t, spec.MinOutbound))
	}
	findings := make([]model.Finding, 0, len(details))
	for _, detail := range details {
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleCardinality),
			Rule:     model.RuleCardinality,
			ID:       id,
			Detail:   detail,
			Location: edgeKeyLocation(cfg, g.Nodes[id], t),
		})
	}
	return findings
}

// CheckEdgeKinds reports edges whose endpoints are of a kind the edge does not
// allow. Only an endpoint the corpus holds is checked: a reference naming no
// document has no kind to be wrong about, and is a dangling_ref of its own.
func CheckEdgeKinds(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, spec := range cfg.Edges {
		if len(spec.From) == 0 && len(spec.To) == 0 {
			continue
		}
		for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
			for _, side := range []struct {
				name    string
				id      model.ID
				allowed []string
			}{{"source", e.From, spec.From}, {"target", e.To, spec.To}} {
				n, known := g.Node(side.id)
				if !known || len(side.allowed) == 0 || slices.Contains(side.allowed, n.Kind) {
					continue
				}
				findings = append(findings, edgeKindMismatch(g, cfg, spec, e, side.name, n, side.allowed))
			}
		}
	}
	SortFindings(findings)
	return findings
}

// edgeKindMismatch files the finding against the document that declared the
// edge, on the key it declared it under: that is the line a reader has to
// change, whichever end of the edge is of the wrong kind.
func edgeKindMismatch(g *model.Graph, cfg config.Config, spec config.EdgeSpec, e model.Edge, side string, endpoint *model.Node, allowed []string) model.Finding {
	owner := edgeOwner(spec, e)
	f := model.Finding{
		Severity: cfg.Severity(model.RuleEdgeKindMismatch),
		Rule:     model.RuleEdgeKindMismatch,
		ID:       owner,
		Detail: fmt.Sprintf("%s %s %s is kind %q, want one of: %s",
			spec.Name, side, endpoint.ID, endpoint.Kind, strings.Join(allowed, ", ")),
	}
	if n, ok := g.Node(owner); ok {
		f.Location = edgeLocation(cfg, n, e)
	}
	if endpoint.ID != owner {
		f.Related = []model.Location{endpoint.Location(config.KeyKind)}
	}
	return f
}

// edgeOwner names the document that declared an edge: its source, or its target
// when the spec reads the key in reverse.
func edgeOwner(spec config.EdgeSpec, e model.Edge) model.ID {
	if spec.Direction == config.DirectionReverse {
		return e.To
	}
	return e.From
}

// CheckStatusVocabulary reports statuses outside the vocabulary their kind
// answers to, which is the top-level one wherever a kind declares none.
func CheckStatusVocabulary(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		vocabulary := cfg.KindStatusValues(n.Kind)
		if len(vocabulary) == 0 || strings.TrimSpace(n.Status) == "" {
			continue
		}
		if _, known := canonicalKindStatus(cfg, n.Kind, n.Status); known {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleUnknownStatus),
			Rule:     model.RuleUnknownStatus,
			ID:       id,
			Detail:   fmt.Sprintf("status %q is outside the vocabulary %s", n.Status, strings.Join(vocabulary, ", ")),
			Location: n.Location(statusField(cfg)),
		})
	}
	return findings
}

// CheckDerived reports derived edges that contradict the structured edges and
// warns wherever a derived edge stands in for structured frontmatter.
func CheckDerived(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	structured := make(map[edgeKey]bool, len(g.Edges))
	for _, e := range g.Edges {
		if e.Origin == model.OriginStructured {
			structured[edgeKey{from: e.From, to: e.To, t: e.Type}] = true
		}
	}

	for _, e := range g.Edges {
		if e.Origin != model.OriginDerived {
			continue
		}
		owner := derivedOwner(cfg, e)
		loc := model.Location{}
		if n, ok := g.Node(owner); ok {
			loc = derivedFieldLocation(cfg, n, e.Type)
		}
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleUnstructuredSupersedes),
			Rule:     model.RuleUnstructuredSupersedes,
			ID:       owner,
			Detail:   fmt.Sprintf("%s edge %s -> %s comes from a field value; declare it in frontmatter", e.Type, e.From, e.To),
			Location: loc,
		})
		if !structured[edgeKey{from: e.To, to: e.From, t: e.Type}] {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleDerivedConflict),
			Rule:     model.RuleDerivedConflict,
			ID:       owner,
			Detail:   fmt.Sprintf("derived %s edge %s -> %s contradicts the structured edge %s -> %s", e.Type, e.From, e.To, e.To, e.From),
			Location: loc,
		})
	}

	SortFindings(findings)
	return findings
}

// derivedOwner returns the document whose field value produced a derived edge.
func derivedOwner(cfg config.Config, e model.Edge) model.ID {
	for _, spec := range cfg.DerivedEdges {
		if spec.Edge != e.Type.String() {
			continue
		}
		if spec.Direction == config.DirectionReverse {
			return e.To
		}
		return e.From
	}
	return e.From
}

// Check runs every built-in structural check. These cannot be disabled.
func Check(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	findings = append(findings, CheckCycles(g, cfg)...)
	findings = append(findings, CheckDangling(g, cfg)...)
	findings = append(findings, CheckInverse(g, cfg)...)
	findings = append(findings, CheckCardinality(g, cfg)...)
	findings = append(findings, CheckEdgeKinds(g, cfg)...)
	findings = append(findings, CheckStatusVocabulary(g, cfg)...)
	findings = append(findings, CheckDerived(g, cfg)...)
	SortFindings(findings)
	return findings
}

// edgeIndex answers "which documents does this one reach over an edge type" in
// constant time, so a rule pass stays linear in the number of edges. The graph
// holds one edge per source, target and type, so a degree is the length of a
// neighbour list.
type edgeIndex struct {
	inbound  map[model.ID]map[model.EdgeType][]model.ID
	outbound map[model.ID]map[model.EdgeType][]model.ID
}

func newEdgeIndex(g *model.Graph) edgeIndex {
	ix := edgeIndex{
		inbound:  make(map[model.ID]map[model.EdgeType][]model.ID, len(g.Nodes)),
		outbound: make(map[model.ID]map[model.EdgeType][]model.ID, len(g.Nodes)),
	}
	for _, e := range g.Edges {
		if e.Origin == model.OriginReference {
			continue
		}
		mark(ix.outbound, e.From, e.Type, e.To)
		mark(ix.inbound, e.To, e.Type, e.From)
	}
	return ix
}

func mark(index map[model.ID]map[model.EdgeType][]model.ID, id model.ID, t model.EdgeType, peer model.ID) {
	types, ok := index[id]
	if !ok {
		types = make(map[model.EdgeType][]model.ID, 1)
		index[id] = types
	}
	types[t] = append(types[t], peer)
}

// neighbors returns the documents one edge type reaches from a document, in
// graph order. An endpoint the corpus does not hold is among them: it is a
// dangling reference, reported on its own, and the edge still exists.
func (ix edgeIndex) neighbors(id model.ID, t model.EdgeType, inbound bool) []model.ID {
	if inbound {
		return ix.inbound[id][t]
	}
	return ix.outbound[id][t]
}

// degree counts the edges of one type at a document, in the direction asked
// for, counting the ones whose other endpoint is missing for the same reason
// neighbors lists them.
func (ix edgeIndex) degree(id model.ID, t model.EdgeType, inbound bool) int {
	return len(ix.neighbors(id, t, inbound))
}

// evalContext is everything a condition is evaluated against: the graph, the
// edge index, and the projections that answer as virtual attributes.
type evalContext struct {
	g         *model.Graph
	ix        edgeIndex
	projected Projections
}

func newEvalContext(g *model.Graph, cfg config.Config) evalContext {
	ix := newEdgeIndex(g)
	return evalContext{g: g, ix: ix, projected: evalProjections(g, cfg, ix)}
}

func (e evalContext) match(cond config.Condition, id model.ID) bool {
	n, ok := e.g.Nodes[id]
	if !ok {
		return false
	}
	for _, clause := range cond.EdgeClauses() {
		if !clause.Holds(e.ix.degree(id, model.EdgeType(clause.Edge), clause.Inbound)) {
			return false
		}
	}
	for _, clause := range cond.ViaClauses() {
		if !e.matchVia(clause, id) {
			return false
		}
	}
	for key, want := range cond.Attr {
		if !e.matchAttr(n, key, want) {
			return false
		}
	}
	if len(cond.AnyOf) > 0 && !slices.ContainsFunc(cond.AnyOf, func(alternative config.Condition) bool {
		return e.match(alternative, id)
	}) {
		return false
	}
	return cond.Not == nil || !e.match(*cond.Not, id)
}

// matchVia holds when at least one neighbour one hop away satisfies every
// attribute clause. A neighbour the corpus does not hold carries no attributes
// at all and cannot be that witness: an edge into a missing document is a
// dangling reference, not evidence about one.
func (e evalContext) matchVia(clause config.ViaClause, id model.ID) bool {
	for _, neighbor := range e.ix.neighbors(id, model.EdgeType(clause.Edge), clause.Inbound) {
		n, known := e.g.Nodes[neighbor]
		if !known {
			continue
		}
		if e.matchAttrs(n, clause.Attr) {
			return true
		}
	}
	return false
}

func (e evalContext) matchAttrs(n *model.Node, want map[string]config.AttrCondition) bool {
	for key, clause := range want {
		if !e.matchAttr(n, key, clause) {
			return false
		}
	}
	return true
}

// matchAttr applies one attribute clause. A positive clause needs the attribute
// to be there; a negative one is satisfied by an attribute that is not.
func (e evalContext) matchAttr(n *model.Node, key string, want config.AttrCondition) bool {
	switch {
	case want.Eq != nil:
		value, present := e.attr(n, key)
		return present && strings.EqualFold(value, *want.Eq)
	case want.Not != nil:
		value, present := e.attr(n, key)
		return !present || !strings.EqualFold(value, *want.Not)
	case want.Contains != nil:
		items, present := e.attrList(n, key)
		return present && containsFold(items, *want.Contains)
	case want.NotContains != nil:
		items, present := e.attrList(n, key)
		return !present || !containsFold(items, *want.NotContains)
	case want.SubsetOf != nil:
		items, present := e.attrList(n, key)
		if !present {
			return false
		}
		for _, item := range items {
			if !containsFold(want.SubsetOf, item) {
				return false
			}
		}
		return true
	}
	return true
}

// attr reads one attribute of a document, a projection of that name first: a
// projection is a virtual attribute, and it shadows a frontmatter key spelled
// the same way. The derived value is the one the configuration meant, and a
// document must not be able to take it back by writing the key down.
func (e evalContext) attr(n *model.Node, key string) (string, bool) {
	if value, ok := e.virtual(n.ID, key); ok {
		return value, true
	}
	return n.Attr(key)
}

// attrList reads one attribute as a list. A projection is a scalar, so it reads
// as the one-element list a scalar always does.
func (e evalContext) attrList(n *model.Node, key string) ([]string, bool) {
	if value, ok := e.virtual(n.ID, key); ok {
		return []string{value}, true
	}
	return n.AttrList(key)
}

func (e evalContext) virtual(id model.ID, key string) (string, bool) {
	if !e.projected.Declares(key) {
		return "", false
	}
	return ProjectionValue(e.projected.Holds(key, id)), true
}

func containsFold(items []string, want string) bool {
	return slices.ContainsFunc(items, func(item string) bool { return strings.EqualFold(item, want) })
}

// MatchCondition reports whether one node satisfies every clause of a rule
// condition, the configured projections included.
func MatchCondition(g *model.Graph, cfg config.Config, cond config.Condition, id model.ID) bool {
	return newEvalContext(g, cfg).match(cond, id)
}

// EvalRule evaluates one declarative rule over every node.
func EvalRule(g *model.Graph, cfg config.Config, rule config.Rule) []model.Finding {
	return evalRule(g, cfg, newEvalContext(g, cfg), rule)
}

// EvalRules evaluates every configured rule over every node. The evaluation
// context is built once: the edge index and the projections are each linear in
// the graph, and rebuilding them per rule is not.
func EvalRules(g *model.Graph, cfg config.Config) []model.Finding {
	ctx := newEvalContext(g, cfg)
	findings := []model.Finding{}
	for _, rule := range cfg.Rules {
		findings = append(findings, evalRule(g, cfg, ctx, rule)...)
	}
	return findings
}

func evalRule(g *model.Graph, cfg config.Config, ctx evalContext, rule config.Rule) []model.Finding {
	findings := []model.Finding{}
	for _, id := range g.NodeIDs() {
		if !ctx.match(rule.When, id) {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: rule.Severity,
			Rule:     rule.Name,
			ID:       id,
			Detail:   rule.Message,
			Location: ruleLocation(cfg, g.Nodes[id], rule.When),
		})
	}
	return findings
}

// ruleLocation points a rule finding at the clause the reader has to change:
// the attribute the condition reads, else the key declaring the edge it names.
func ruleLocation(cfg config.Config, n *model.Node, cond config.Condition) model.Location {
	attrs := make(map[string]bool)
	var edges []string
	for _, nested := range cond.Conditions() {
		for key := range nested.Attr {
			attrs[key] = true
		}
		for _, clause := range nested.EdgeClauses() {
			if spec, ok := cfg.Edge(model.EdgeType(clause.Edge)); ok {
				edges = append(edges, spec.Key)
			}
		}
		// A one-hop clause reads the neighbour's attributes, which are in
		// another file; what this document can change is the edge that reaches
		// it, so that is the key the finding points at.
		for _, clause := range nested.ViaClauses() {
			if spec, ok := cfg.Edge(model.EdgeType(clause.Edge)); ok {
				edges = append(edges, spec.Key)
			}
		}
	}
	keys := append(slices.Sorted(maps.Keys(attrs)), edges...)
	return n.Location(append(keys, statusField(cfg))...)
}

// Validate runs the structural checks and the configured rules, returning the
// findings already recorded on the graph too, in deterministic order.
func Validate(g *model.Graph, cfg config.Config) []model.Finding {
	findings := make([]model.Finding, 0, len(g.Findings))
	findings = append(findings, g.Findings...)
	findings = append(findings, Check(g, cfg)...)
	findings = append(findings, EvalRules(g, cfg)...)
	SortFindings(findings)
	return findings
}

// SortFindings orders findings by severity, then path, line, rule, id and
// detail, so a report reads in file order and diffs cleanly.
func SortFindings(findings []model.Finding) {
	slices.SortFunc(findings, func(a, b model.Finding) int {
		return cmp.Or(
			cmp.Compare(a.Severity.Rank(), b.Severity.Rank()),
			cmp.Compare(a.Location.Path, b.Location.Path),
			cmp.Compare(a.Location.Line, b.Location.Line),
			cmp.Compare(a.Rule, b.Rule),
			cmp.Compare(a.ID, b.ID),
			cmp.Compare(a.Detail, b.Detail),
		)
	})
}

// Summarize counts documents, typed edges and findings for the summary line.
func Summarize(g *model.Graph, findings []model.Finding) model.Summary {
	summary := model.Summary{Documents: len(g.Nodes), Edges: len(g.Edges)}
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityError:
			summary.Errors++
		case model.SeverityWarn:
			summary.Warnings++
		}
		if f.Rule == model.RuleCycle {
			summary.Cycles++
		}
	}
	return summary
}

func joinIDs(ids []model.ID, separator string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	return strings.Join(parts, separator)
}
