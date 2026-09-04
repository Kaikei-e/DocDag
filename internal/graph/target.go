package graph

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// CheckTargets reports edges whose target document does not satisfy the
// condition its edge spec puts on it. Only an edge that declares `target:` is
// checked, so a configuration that declares none — both presets did before the
// spec preset took one — sees nothing new.
//
// The check is local: it asks one question about one document, one hop away.
// Walking the lineage to a replacement is what the fix suggestion does, and
// keeping the two apart is what keeps transitive reach out of the vocabulary.
func CheckTargets(g *model.Graph, cfg config.Config, asOf time.Time) []model.Finding {
	findings := []model.Finding{}
	specs := targetedEdges(cfg)
	if len(specs) == 0 {
		return findings
	}
	ctx := newEvalContext(g, cfg, asOf)
	for _, spec := range specs {
		for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
			if !declaredInForce(ctx.periods, e) {
				continue
			}
			f, violated := staleTarget(g, cfg, ctx, spec, e)
			if violated {
				findings = append(findings, f)
			}
		}
	}
	SortFindings(findings)
	return findings
}

// declaredInForce reports whether the document an edge runs from still carries
// normative weight on the day the run is about — the same rule the edge index
// applies (check.go's carriesWeight), applied here to the declarations a target
// condition reads.
//
// A target check needs it for a reason of its own: without it, a departure that
// expired or was resolved holds its clause hostage forever. Superseding a
// clause any historical deviation ever pointed at would be a stale_target
// nobody can clear, because append-first history keeps the record — and a
// history that cannot be added to without breaking the build is a ratchet
// rather than an archive.
//
// The index exempts the supersedes lineage and this does not, because that
// exemption is about the index: the ends of a period are derived over that
// lineage, and dropping it would erase the "successor not in force yet" that
// pending_successor exists to report. A target check derives nothing from the
// lineage, so the principle applies to it plainly — an out-of-force document's
// outbound declarations lose their weight, under whatever edge type they were
// written. No preset DocDag ships declares a `target:` on supersedes, so the
// question does not arise for any shipped configuration.
func declaredInForce(periods Periods, e model.Edge) bool {
	if !periods.Declared(e.From) {
		return true
	}
	return periods.InForce(e.From)
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
	breakers := leafBreakers(g, cfg, ctx, spec, e.To)
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
//
// Where the target's kind declares a period, "the current leaf" is read at the
// day the run is about: a successor that has not begun, or one nobody has
// accepted yet, leaves its predecessor the current leaf, because that is the
// document a reader is still bound by. A kind without a period keeps the plain
// reading — any successor at all breaks the leaf — which is what every corpus
// answered before periods existed.
func leafBreakers(g *model.Graph, cfg config.Config, ctx evalContext, spec config.EdgeSpec, target model.ID) []model.ID {
	if spec.Target.LeafOf == "" {
		return nil
	}
	breakers := slices.Clone(ctx.ix.neighbors(target, model.EdgeType(spec.Target.LeafOf), true))
	if n, known := g.Node(target); known && ctx.periods.Declared(n.ID) {
		breakers = slices.DeleteFunc(breakers, func(id model.ID) bool {
			return !replaces(g, cfg, ctx.periods, id)
		})
	}
	slices.Sort(breakers)
	return slices.Compact(breakers)
}

// replaces reports whether a successor actually stands in for its predecessor
// on the day being asked about: the corpus holds it, somebody accepted it, and
// its own period has begun and not ended.
func replaces(g *model.Graph, cfg config.Config, periods Periods, id model.ID) bool {
	n, known := g.Node(id)
	if !known {
		// A successor the corpus does not hold is a dangling reference, reported
		// on its own. It says nothing about what replaced anything, but it is
		// still a declared successor, so it breaks the leaf as it always did.
		return true
	}
	status, ok := canonicalKindStatus(cfg, n.Kind, n.Status)
	return ok && strings.EqualFold(status, config.StatusAccepted) && periods.InForce(id)
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
func leafSuggestion(g *model.Graph, cfg config.Config, f model.Finding, asOf time.Time) string {
	ctx := newEvalContext(g, cfg, asOf)
	for _, spec := range targetedEdges(cfg) {
		if spec.Target.LeafOf == "" {
			continue
		}
		for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
			// The finding does not carry the edge it was filed for, so the edge
			// is recognized by the report it produced: recomputing the detail is
			// exact, where reading identifiers back out of prose is not. The
			// edges the check passed over are passed over here too, so the two
			// walks read one set of declarations.
			if !declaredInForce(ctx.periods, e) {
				continue
			}
			if declaringDoc(cfg, spec, e) != f.ID || staleTargetDetail(spec, e.To, leafBreakers(g, cfg, ctx, spec, e.To)) != f.Detail {
				continue
			}
			return leafFix(g, cfg, e.To, model.EdgeType(spec.Target.LeafOf), asOf)
		}
	}
	return ""
}

// leafFix names the documents the lineage of a replaced target ends at, walked
// the way the check read it: a lineage whose kind declares a period stops at
// the successor that is in force, because that is the document the reader is
// being sent to. A lineage that loops has no leaf to name — Resolve reports the
// cycle, which is a finding of its own — so the stale target stands and only
// the suggestion goes.
func leafFix(g *model.Graph, cfg config.Config, target model.ID, t model.EdgeType, asOf time.Time) string {
	leaves, err := ResolveAt(g, cfg, target, t, asOf)
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
