package graph

import (
	"fmt"
	"slices"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// CheckTargets reports edges whose target document does not satisfy the
// condition its edge spec puts on it. Only an edge that declares `target:` is
// checked, so a configuration that declares none — both presets did before the
// spec preset took one — sees nothing new.
//
// The check is local: it asks one question about one document, one hop away.
// Walking the lineage to a replacement is what the fix suggestion does, and
// keeping the two apart is what keeps transitive reach out of the vocabulary.
func CheckTargets(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	specs := targetedEdges(cfg)
	if len(specs) == 0 {
		return findings
	}
	ctx := newEvalContext(g, cfg)
	for _, spec := range specs {
		for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
			f, violated := staleTarget(g, cfg, ctx, spec, e)
			if violated {
				findings = append(findings, f)
			}
		}
	}
	SortFindings(findings)
	return findings
}

// targetedEdges returns the edge specs that declare a target condition, in
// declaration order.
func targetedEdges(cfg config.Config) []config.EdgeSpec {
	specs := make([]config.EdgeSpec, 0, len(cfg.Edges))
	for _, spec := range cfg.Edges {
		if spec.Target != nil {
			specs = append(specs, spec)
		}
	}
	return specs
}

// staleTarget evaluates one edge's target condition and, where it fails, files
// the finding against the document that declared the edge.
//
// The target of an edge is its head — e.To — whichever end of it the
// frontmatter named. A direction: reverse key, such as the MADR "superseded by
// 0003" a derived edge comes from, writes down the edge's *source*, so the
// document the relation points at is the one that wrote the key: the graph
// already swapped the endpoints when it recorded the edge, and reading e.To
// after that swap is what "the document the reference points at" means. It is
// the same endpoint the `to:` kinds constrain, so `target:` and `to:` speak
// about one document rather than two.
func staleTarget(g *model.Graph, cfg config.Config, ctx evalContext, spec config.EdgeSpec, e model.Edge) (model.Finding, bool) {
	target, known := g.Node(e.To)
	if !known {
		// A reference naming no document is a dangling_ref, which says the same
		// mistake better: there is no target to hold a condition against.
		return model.Finding{}, false
	}
	owner, ownerKnown := g.Node(declaringDoc(cfg, spec, e))
	if !ownerKnown {
		return model.Finding{}, false
	}
	breakers := leafBreakers(ctx.ix, spec, e.To)
	if len(breakers) == 0 && ctx.match(spec.Target.Condition, e.To) {
		return model.Finding{}, false
	}

	f := model.Finding{
		Severity: cfg.Severity(model.RuleStaleTarget),
		Rule:     model.RuleStaleTarget,
		ID:       owner.ID,
		Detail:   staleTargetDetail(spec, e.To, breakers),
		Location: edgeLocation(cfg, owner, e),
	}
	related := make([]model.Location, 0, len(breakers)+1)
	// A reverse-direction edge is declared by its own target, and a finding
	// does not relate a document to itself.
	if target.ID != owner.ID {
		related = append(related, target.Location(statusField(cfg)))
	}
	for _, id := range breakers {
		n, ok := g.Node(id)
		if !ok {
			continue
		}
		related = append(related, edgeKeyLocation(cfg, n, model.EdgeType(spec.Target.LeafOf)))
	}
	f.Related = related
	return f, true
}

// declaringDoc names the document whose frontmatter produced an edge: the
// endpoint the edge's direction puts the key on, or, for an edge a
// derived_edges pattern lifted out of a field value, the document that field
// belongs to. A target condition reaches derived edges too, and the finding
// belongs on the line that can be changed.
func declaringDoc(cfg config.Config, spec config.EdgeSpec, e model.Edge) model.ID {
	if e.Origin == model.OriginDerived {
		return derivedOwner(cfg, e)
	}
	return edgeOwner(spec, e)
}

// leafBreakers names the documents that keep a target from being the leaf of
// the lineage a `leaf_of` target asks for, sorted. An edge into a document the
// corpus does not hold counts among them, exactly as it counts for the
// `not_inbound` this word is sugar for: the sugar has to mean what it spells.
func leafBreakers(ix edgeIndex, spec config.EdgeSpec, target model.ID) []model.ID {
	if spec.Target.LeafOf == "" {
		return nil
	}
	breakers := slices.Clone(ix.neighbors(target, model.EdgeType(spec.Target.LeafOf), true))
	slices.Sort(breakers)
	return slices.Compact(breakers)
}

// staleTargetDetail names the edge, the document it points at and why that
// document is the wrong one. A `leaf_of` failure names the successors, because
// they are what the reader has to look at; any other condition is whatever the
// configuration wrote, and the target's own location under `related` is where
// the reader goes to read it.
//
// The edge names are written as the configuration declared them rather than
// inflected for one successor or several: they are the tokens a reader greps
// the configuration and the frontmatter for.
func staleTargetDetail(spec config.EdgeSpec, target model.ID, breakers []model.ID) string {
	if len(breakers) > 0 {
		return fmt.Sprintf("%s targets %s, which %s %s", spec.Name, target, joinIDs(breakers, ", "), spec.Target.LeafOf)
	}
	return fmt.Sprintf("%s targets %s, which does not satisfy the edge's target condition", spec.Name, target)
}

// leafSuggestion names the current leaf of the lineage a stale target belongs
// to. Only a `leaf_of` condition earns a suggestion: it says the target has
// been replaced, and the replacement is a walk away. Any other condition is a
// statement about the target document, and which document satisfies it instead
// is not a question the graph answers.
func leafSuggestion(g *model.Graph, cfg config.Config, f model.Finding) string {
	ix := newEdgeIndex(g)
	for _, spec := range targetedEdges(cfg) {
		if spec.Target.LeafOf == "" {
			continue
		}
		for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
			// The finding does not carry the edge it was filed for, so the edge
			// is recognized by the report it produced: recomputing the detail is
			// exact, where reading identifiers back out of prose is not.
			if declaringDoc(cfg, spec, e) != f.ID || staleTargetDetail(spec, e.To, leafBreakers(ix, spec, e.To)) != f.Detail {
				continue
			}
			return leafFix(g, e.To, model.EdgeType(spec.Target.LeafOf))
		}
	}
	return ""
}

// leafFix names the documents the lineage of a replaced target ends at. A
// lineage that loops has no leaf to name — Resolve reports the cycle, which is
// a finding of its own — so the stale target stands and only the suggestion
// goes.
func leafFix(g *model.Graph, target model.ID, t model.EdgeType) string {
	leaves, err := Resolve(g, target, t)
	if err != nil || len(leaves) == 0 {
		return ""
	}
	if len(leaves) == 1 {
		return fmt.Sprintf("did you mean %s?", leaves[0])
	}
	// A branched lineage has no single answer, and DocDag does not choose one:
	// the candidates are listed and the reader picks.
	return fmt.Sprintf("did you mean one of: %s?", joinIDs(leaves, ", "))
}
