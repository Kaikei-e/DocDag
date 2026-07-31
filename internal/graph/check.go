package graph

import (
	"fmt"
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
			findings = append(findings, model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleInvalidFrontmatter,
				ID:       doc.ID,
				Detail:   firstLine(fmt.Sprintf("%s: %v", doc.Path, doc.Err)),
			})
		case !doc.HasFrontmatter && doc.MatchesPattern:
			findings = append(findings, model.Finding{
				Severity: model.SeverityWarn,
				Rule:     model.RuleMissingFrontmatter,
				ID:       doc.ID,
				Detail:   fmt.Sprintf("%s has no frontmatter block", doc.Path),
			})
		}
	}

	for id, colliding := range paths {
		if len(colliding) < 2 {
			continue
		}
		sorted := slices.Clone(colliding)
		slices.Sort(sorted)
		findings = append(findings, model.Finding{
			Severity: model.SeverityError,
			Rule:     model.RuleIDCollision,
			ID:       id,
			Detail:   fmt.Sprintf("%s carry the same identifier", strings.Join(sorted, ", ")),
		})
	}

	SortFindings(findings)
	return findings
}

// CheckCycles reports one finding per cycle found in an acyclic edge type.
func CheckCycles(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, spec := range cfg.Edges {
		if !spec.Acyclic {
			continue
		}
		t := model.EdgeType(spec.Name)
		for _, cycle := range FindCycles(Adjacency(g, t)) {
			findings = append(findings, model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleCycle,
				ID:       cycle[0],
				Detail:   fmt.Sprintf("%s cycle: %s", t, joinIDs(cycle, " -> ")),
			})
		}
	}
	SortFindings(findings)
	return findings
}

// CheckDangling reports typed edges whose target is not a known document.
func CheckDangling(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, e := range g.Edges {
		if _, known := g.Nodes[e.To]; known {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityError,
			Rule:     model.RuleDanglingRef,
			ID:       e.From,
			Detail:   fmt.Sprintf("%s reference %s is not a known document", e.Type, e.To),
		})
	}
	SortFindings(findings)
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
		findings = append(findings, model.Finding{
			Severity: model.SeverityWarn,
			Rule:     model.RuleUnstructuredSupersedes,
			ID:       owner,
			Detail:   fmt.Sprintf("%s edge %s -> %s comes from a field value; declare it in frontmatter", e.Type, e.From, e.To),
		})
		if !structured[edgeKey{from: e.To, to: e.From, t: e.Type}] {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityError,
			Rule:     model.RuleDerivedConflict,
			ID:       owner,
			Detail:   fmt.Sprintf("derived %s edge %s -> %s contradicts the structured edge %s -> %s", e.Type, e.From, e.To, e.To, e.From),
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
	if cond.Inbound != "" && !ix.inbound[id][model.EdgeType(cond.Inbound)] {
		return false
	}
	if cond.NotInbound != "" && ix.inbound[id][model.EdgeType(cond.NotInbound)] {
		return false
	}
	if cond.Outbound != "" && !ix.outbound[id][model.EdgeType(cond.Outbound)] {
		return false
	}
	if cond.NotOutbound != "" && ix.outbound[id][model.EdgeType(cond.NotOutbound)] {
		return false
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
func EvalRule(g *model.Graph, rule config.Rule) []model.Finding {
	ix := newEdgeIndex(g)
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
		})
	}
	return findings
}

// EvalRules evaluates every configured rule over every node.
func EvalRules(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, rule := range cfg.Rules {
		findings = append(findings, EvalRule(g, rule)...)
	}
	return findings
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

// SortFindings orders findings by severity, then rule, then id, then detail.
func SortFindings(findings []model.Finding) {
	slices.SortFunc(findings, func(a, b model.Finding) int {
		if c := a.Severity.Rank() - b.Severity.Rank(); c != 0 {
			return c
		}
		if c := strings.Compare(a.Rule, b.Rule); c != 0 {
			return c
		}
		if c := strings.Compare(string(a.ID), string(b.ID)); c != 0 {
			return c
		}
		return strings.Compare(a.Detail, b.Detail)
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

// firstLine keeps a detail printable on the single line a finding occupies:
// YAML decode errors arrive with a multi-line source excerpt attached.
func firstLine(text string) string {
	if cut := strings.IndexByte(text, '\n'); cut >= 0 {
		text = text[:cut]
	}
	return strings.TrimSpace(text)
}

func joinIDs(ids []model.ID, separator string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	return strings.Join(parts, separator)
}
