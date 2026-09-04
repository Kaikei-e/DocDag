package graph

import (
	"fmt"
	"slices"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// CheckPathConstraints reports documents from which one path of edges reaches
// a document the comparison path does not. Neither preset declares any, so a
// configuration that writes none pays one length check for the whole corpus
// and reports nothing.
func CheckPathConstraints(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	if len(cfg.PathConstraints) == 0 {
		return findings
	}
	// A path constraint is about the edges the frontmatter declares rather than
	// about what is in force today: it says two walks reach the same documents,
	// which is a statement about the shape of the corpus. So the walk reads
	// every edge, and the index is built over no periods.
	ix := newEdgeIndex(g, Periods{})
	for _, constraint := range cfg.PathConstraints {
		steps := config.PathSteps(constraint.Path)
		compare := config.PathSteps(constraint.SubsetOf)
		// Validation rejects a path of no steps. One that reaches here anyway
		// walks nothing rather than reporting every document against a
		// constraint that names none.
		if len(steps) == 0 {
			continue
		}
		for _, id := range g.NodeIDs() {
			reached := walkPath(g, ix, id, steps)
			if len(reached) == 0 {
				continue
			}
			// `equals: none` writes down the empty set rather than a walk, so
			// there is nothing to compose for it and everything reached is a
			// violation.
			allowed := []model.ID{}
			if len(compare) > 0 {
				allowed = walkPath(g, ix, id, compare)
			}
			extra := missing(reached, allowed)
			if len(extra) == 0 {
				continue
			}
			findings = append(findings, pathMismatch(g, cfg, constraint, id, extra))
		}
	}
	SortFindings(findings)
	return findings
}

// walkPath returns the documents a path reaches from one document, sorted:
// each step is composed over every document the step before it reached, which
// is what makes the answer a set rather than a chain. A path is one or two
// steps long, so the walk terminates without a visited set whatever the graph
// does.
//
// Only documents the corpus holds are reached. A reference naming none is a
// dangling_ref of its own, and a path constraint firing on it would report one
// mistake twice; reference-layer edges are left out for the same reason rule
// evaluation leaves them out — prose carries no invariants.
func walkPath(g *model.Graph, ix edgeIndex, from model.ID, steps []config.PathStep) []model.ID {
	current := []model.ID{from}
	for _, step := range steps {
		next := make(map[model.ID]bool, len(current))
		for _, id := range current {
			for _, peer := range ix.neighbors(id, model.EdgeType(step.Edge), step.Inbound) {
				if _, known := g.Nodes[peer]; known {
					next[peer] = true
				}
			}
		}
		current = make([]model.ID, 0, len(next))
		for id := range next {
			current = append(current, id)
		}
		slices.Sort(current)
	}
	return current
}

// missing returns the members of reached that allowed does not hold, keeping
// the order reached is in.
func missing(reached, allowed []model.ID) []model.ID {
	extra := make([]model.ID, 0, len(reached))
	for _, id := range reached {
		if !slices.Contains(allowed, id) {
			extra = append(extra, id)
		}
	}
	return extra
}

// pathMismatch files a violated path constraint against the document the walk
// started at, on the key that declares its first step: that step is the only
// one of the path the document itself wrote down, and the rest belong to the
// documents it reaches.
func pathMismatch(g *model.Graph, cfg config.Config, constraint config.PathConstraint, id model.ID, extra []model.ID) model.Finding {
	f := model.Finding{
		Severity: cfg.Severity(model.RulePathMismatch),
		Rule:     model.RulePathMismatch,
		ID:       id,
		Detail:   pathMismatchDetail(constraint, extra),
	}
	if n, ok := g.Node(id); ok {
		f.Location = edgeKeyLocation(cfg, n, model.EdgeType(config.PathSteps(constraint.Path)[0].Edge))
	}
	related := make([]model.Location, 0, len(extra))
	for _, reached := range extra {
		n, ok := g.Node(reached)
		if !ok {
			continue
		}
		related = append(related, n.Location(statusField(cfg)))
	}
	f.Related = related
	return f
}

// pathMismatchDetail names the constraint, the path that reached too far and
// what it reached. There is no fix suggestion anywhere below it: which of the
// two paths is the wrong one is not something DocDag guesses, the same policy
// derived_conflict follows.
func pathMismatchDetail(constraint config.PathConstraint, extra []model.ID) string {
	reached := fmt.Sprintf("%s: %s reaches %s", constraint.Name, config.PathString(constraint.Path), joinIDs(extra, ", "))
	if constraint.SubsetOf == nil {
		return reached + ", want " + config.PathEqualsNone
	}
	return fmt.Sprintf("%s, which %s does not", reached, config.PathString(constraint.SubsetOf))
}
