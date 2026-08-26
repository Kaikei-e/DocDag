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
		paths[doc.ID] = append(paths[doc.ID], doc.Path)
		switch {
		case doc.Err != nil:
			findings = append(findings, invalidFrontmatter(doc))
		case !doc.HasFrontmatter && doc.MatchesPattern:
			findings = append(findings, model.Finding{
				Severity: model.SeverityWarn,
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
		findings = append(findings, idCollision(id, colliding))
	}

	SortFindings(findings)
	return findings
}

// firstFileLine is where a finding about a whole file points.
const firstFileLine = 1

func invalidFrontmatter(doc *parse.Document) model.Finding {
	f := model.Finding{
		Severity: model.SeverityError,
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

func idCollision(id model.ID, colliding []string) model.Finding {
	sorted := slices.Sorted(slices.Values(colliding))
	related := make([]model.Location, 0, len(sorted)-1)
	for _, path := range sorted[1:] {
		related = append(related, model.Location{Path: path, Line: firstFileLine})
	}
	return model.Finding{
		Severity: model.SeverityError,
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
func edgeKeyLocation(cfg config.Config, n *model.Node, t model.EdgeType) model.Location {
	keys := make([]string, 0, 3)
	if spec, ok := cfg.Edge(t); ok {
		keys = append(keys, spec.Key)
	}
	keys = append(keys, derivedFields(cfg, t)...)
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

// CheckCycles reports one finding per cycle found in an acyclic edge type.
func CheckCycles(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, t := range cfg.AcyclicEdgeTypes() {
		for _, cycle := range FindCycles(Adjacency(g, t)) {
			findings = append(findings, cycleFinding(g, cfg, t, cycle))
		}
	}
	SortFindings(findings)
	return findings
}

// cycleFinding files a cycle against its lexically smallest member and relates
// the others, so a reader opens one file and sees every edge on the path.
func cycleFinding(g *model.Graph, cfg config.Config, t model.EdgeType, cycle []model.ID) model.Finding {
	f := model.Finding{
		Severity: model.SeverityError,
		Rule:     model.RuleCycle,
		ID:       cycle[0],
		Detail:   fmt.Sprintf("%s cycle: %s", t, joinIDs(cycle, " -> ")),
	}
	members := slices.Compact(slices.Sorted(slices.Values(cycle)))
	locations := make([]model.Location, 0, len(members))
	for _, id := range members {
		if n, ok := g.Node(id); ok {
			locations = append(locations, edgeKeyLocation(cfg, n, t))
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
		Severity: model.SeverityError,
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
	normalizer := cfg.Normalizer()
	listed := make(map[edgeKey]bool)

	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		loc := n.Location(spec.Inverse, spec.Key, statusField(cfg))
		refs, invalid := parse.Refs(n.Attrs, spec.Inverse)
		for _, entry := range append(slices.Clone(invalid), unshaped(refs)...) {
			findings = append(findings, model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleInvalidRef,
				ID:       id,
				Detail:   fmt.Sprintf("%s reference %q is not an identifier", spec.Inverse, entry),
				Location: loc,
			})
		}
		for _, ref := range refs {
			source, ok := normalizer.Normalize(ref)
			if !ok || !config.IDShaped(ref) {
				continue
			}
			if _, known := g.Node(source); !known {
				findings = append(findings, model.Finding{
					Severity: model.SeverityError,
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
		Severity: model.SeverityError,
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

// unshaped returns the references that name no identity at all.
func unshaped(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !config.IDShaped(ref) {
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
			Severity: model.SeverityError,
			Rule:     model.RuleCardinality,
			ID:       id,
			Detail:   detail,
			Location: edgeKeyLocation(cfg, g.Nodes[id], t),
		})
	}
	return findings
}

// CheckStatusVocabulary reports statuses outside the configured vocabulary.
func CheckStatusVocabulary(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	if len(cfg.StatusValues) == 0 {
		return findings
	}
	for _, id := range g.NodeIDs() {
		raw := g.Nodes[id].Status
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if _, known := canonicalStatus(cfg, raw); known {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityError,
			Rule:     model.RuleUnknownStatus,
			ID:       id,
			Detail:   fmt.Sprintf("status %q is outside the vocabulary %s", raw, strings.Join(cfg.StatusValues, ", ")),
			Location: g.Nodes[id].Location(statusField(cfg)),
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
			Severity: model.SeverityWarn,
			Rule:     model.RuleUnstructuredSupersedes,
			ID:       owner,
			Detail:   fmt.Sprintf("%s edge %s -> %s comes from a field value; declare it in frontmatter", e.Type, e.From, e.To),
			Location: loc,
		})
		if !structured[edgeKey{from: e.To, to: e.From, t: e.Type}] {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityError,
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
	findings = append(findings, CheckStatusVocabulary(g, cfg)...)
	findings = append(findings, CheckDerived(g, cfg)...)
	SortFindings(findings)
	return findings
}

// edgeIndex answers "does this document have an edge of type t" in constant
// time, so a rule pass stays linear in the number of edges.
type edgeIndex struct {
	inbound  map[model.ID]map[model.EdgeType]bool
	outbound map[model.ID]map[model.EdgeType]bool
}

func newEdgeIndex(g *model.Graph) edgeIndex {
	ix := edgeIndex{
		inbound:  make(map[model.ID]map[model.EdgeType]bool, len(g.Nodes)),
		outbound: make(map[model.ID]map[model.EdgeType]bool, len(g.Nodes)),
	}
	for _, e := range g.Edges {
		if e.Origin == model.OriginReference {
			continue
		}
		mark(ix.outbound, e.From, e.Type)
		mark(ix.inbound, e.To, e.Type)
	}
	return ix
}

func mark(index map[model.ID]map[model.EdgeType]bool, id model.ID, t model.EdgeType) {
	types, ok := index[id]
	if !ok {
		types = make(map[model.EdgeType]bool, 1)
		index[id] = types
	}
	types[t] = true
}

func (ix edgeIndex) match(g *model.Graph, cond config.Condition, id model.ID) bool {
	n, ok := g.Nodes[id]
	if !ok {
		return false
	}
	for _, clause := range cond.EdgeClauses() {
		types := ix.outbound
		if clause.Inbound {
			types = ix.inbound
		}
		if types[id][model.EdgeType(clause.Edge)] == clause.Negate {
			return false
		}
	}
	for key, want := range cond.Attr {
		value, present := n.Attr(key)
		if want.Eq != nil && (!present || !strings.EqualFold(value, *want.Eq)) {
			return false
		}
		if want.Not != nil && present && strings.EqualFold(value, *want.Not) {
			return false
		}
	}
	return true
}

// MatchCondition reports whether one node satisfies every clause of a rule
// condition.
func MatchCondition(g *model.Graph, cond config.Condition, id model.ID) bool {
	return newEdgeIndex(g).match(g, cond, id)
}

// EvalRule evaluates one declarative rule over every node.
func EvalRule(g *model.Graph, cfg config.Config, rule config.Rule) []model.Finding {
	return evalRule(g, cfg, newEdgeIndex(g), rule)
}

// EvalRules evaluates every configured rule over every node. The edge index is
// built once: it is linear in the graph, and rebuilding it per rule is not.
func EvalRules(g *model.Graph, cfg config.Config) []model.Finding {
	ix := newEdgeIndex(g)
	findings := []model.Finding{}
	for _, rule := range cfg.Rules {
		findings = append(findings, evalRule(g, cfg, ix, rule)...)
	}
	return findings
}

func evalRule(g *model.Graph, cfg config.Config, ix edgeIndex, rule config.Rule) []model.Finding {
	findings := []model.Finding{}
	for _, id := range g.NodeIDs() {
		if !ix.match(g, rule.When, id) {
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
	keys := slices.Sorted(maps.Keys(cond.Attr))
	for _, clause := range cond.EdgeClauses() {
		if spec, ok := cfg.Edge(model.EdgeType(clause.Edge)); ok {
			keys = append(keys, spec.Key)
		}
	}
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
