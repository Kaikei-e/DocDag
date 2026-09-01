package graph

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// ModalityConflict is one pair of binding clauses that say incompatible things
// about a subject they share. A and B are ordered by identifier, so the pair is
// named the same way whichever end the walk reached first, and Topics lists
// every subject they share, sorted.
//
// Strong marks the pair whose members are both strict rules — a MUST against a
// MUST_NOT. Defeasible deontic logic's strict rules are the ones a defeater
// cannot overturn, so a recorded exception does not suppress that pair; every
// other conflicting pair is weak and an excepts edge between the two, in either
// direction, is the corpus saying it already knows.
type ModalityConflict struct {
	A, B                 model.ID
	ModalityA, ModalityB string
	Topics               []model.ID
	Strong               bool
	// Suppressed reports that a weak conflict is defeated by Defeater, the
	// excepts edge recorded between the two clauses.
	Suppressed bool
	Defeater   model.Edge
}

// Detail is what the finding says: the two modalities, the clause the reader is
// not looking at, and the subject they collide over. A suppressed conflict says
// what is holding it down on the same line, because that is the whole of what
// the reader has to check.
func (c ModalityConflict) Detail() string {
	detail := fmt.Sprintf("is %s and %s is %s about %s", c.ModalityA, c.B, c.ModalityB, joinIDs(c.Topics, ", "))
	if c.Suppressed {
		detail += ", " + c.Suppression()
	}
	return detail
}

// Suppression is the one line that says which recorded exception defeats a
// conflict, and what scope it was recorded under. It is empty for a conflict
// nothing defeats.
func (c ModalityConflict) Suppression() string {
	if !c.Suppressed {
		return ""
	}
	line := fmt.Sprintf("suppressed by %s %s -> %s", config.EdgeExcepts, c.Defeater.From, c.Defeater.To)
	if scope, ok := c.Defeater.Attr(config.AttrScope); ok {
		line += fmt.Sprintf(" (%s: %s)", config.AttrScope, scope)
	}
	return line
}

// Fix says what to type. A weak conflict is settled by recording the exception
// or by revising a modality; a strong one only by the revision, because the
// exception it would take is one excepts_strict refuses. A conflict already
// answered has nothing to type: telling a reader to declare the edge they are
// looking at would be worse than saying nothing.
func (c ModalityConflict) Fix() string {
	switch {
	case c.Suppressed:
		return ""
	case c.Strong:
		return fmt.Sprintf("revise one %s: a strict rule cannot be defeated", config.FieldModality)
	}
	return fmt.Sprintf("declare %s: %s in %s with %s:, or revise one %s",
		config.EdgeExcepts, c.B, c.A, config.AttrScope, config.FieldModality)
}

// CheckModalityConflicts reports the pairs of binding clauses whose modalities
// cannot both hold about a subject they share. It answers only for a
// configuration that declares the `about` edge and the `modality` field: what
// two clauses are about, and at what strength, is what the check is made of.
func CheckModalityConflicts(g *model.Graph, cfg config.Config) []model.Finding {
	conflicts := ModalityConflicts(g, cfg)
	findings := make([]model.Finding, 0, len(conflicts))
	for _, c := range conflicts {
		findings = append(findings, modalityConflict(g, cfg, c))
	}
	SortFindings(findings)
	return findings
}

func modalityConflict(g *model.Graph, cfg config.Config, c ModalityConflict) model.Finding {
	f := model.Finding{
		Severity:   cfg.Severity(model.RuleModalityConflict),
		Rule:       model.RuleModalityConflict,
		ID:         c.A,
		Detail:     c.Detail(),
		Suppressed: c.Suppressed,
		Fix:        c.Fix(),
	}
	if n, ok := g.Node(c.A); ok {
		f.Location = modalityLocation(cfg, n)
	}
	related := make([]model.Location, 0, len(c.Topics)+1)
	if n, ok := g.Node(c.B); ok {
		related = append(related, modalityLocation(cfg, n))
	}
	for _, topic := range c.Topics {
		if n, ok := g.Node(topic); ok {
			// The topic's own line is its title: what a reader goes there for
			// is the paragraph that says what the subject is.
			related = append(related, n.Location(config.KeyTitle, statusField(cfg)))
		}
	}
	f.Related = related
	return f
}

// modalityLocation points at the line a clause states its strength on, falling
// back to the subject it states and then to its status: the reader is being
// asked to change one of the two, and the modality is the one they can see the
// conflict in.
func modalityLocation(cfg config.Config, n *model.Node) model.Location {
	keys := []string{config.FieldModality}
	if spec, ok := cfg.Edge(config.EdgeAbout); ok {
		keys = append(keys, spec.Key)
	}
	return n.Location(append(keys, statusField(cfg))...)
}

// ModalityConflicts returns every conflicting pair of binding clauses, ordered
// by the pair's own identifiers. It is exported because a conflict is a fact
// about the corpus rather than only a finding: `context` reports the suppressed
// ones as part of a clause's neighbourhood and `stats` counts them.
//
// The pairing is per subject and quadratic in the clauses that share one, which
// is the granularity the topics are cut at: a subject a paragraph defines
// carries a handful of clauses, so the walk is linear in practice. A vault that
// hangs a hundred clauses off one topic has a topic that says too little, which
// is a matter for the preset lint rather than for a cleverer algorithm here.
func ModalityConflicts(g *model.Graph, cfg config.Config) []ModalityConflict {
	conflicts := []ModalityConflict{}
	about, declared := cfg.Edge(config.EdgeAbout)
	if !declared {
		return conflicts
	}
	binding := make(map[model.ID]bool)
	for _, id := range BindingSet(g, cfg) {
		binding[id] = true
	}

	// The subjects first: a clause that does not bind states nothing that could
	// collide, so it never joins a group.
	subjects := make(map[model.ID][]model.ID, len(g.Nodes))
	for _, e := range g.EdgesOfType(model.EdgeType(about.Name)) {
		if !binding[e.From] {
			continue
		}
		if _, known := g.Node(e.To); !known {
			continue
		}
		subjects[e.To] = append(subjects[e.To], e.From)
	}

	// Then the pairs, accumulated across subjects: two clauses about two shared
	// topics disagree once, over both of them.
	type pair struct{ a, b model.ID }
	shared := make(map[pair][]model.ID)
	for _, topic := range slices.Sorted(maps.Keys(subjects)) {
		clauses := slices.Compact(slices.Sorted(slices.Values(subjects[topic])))
		for i := range clauses {
			for j := i + 1; j < len(clauses); j++ {
				key := pair{a: clauses[i], b: clauses[j]}
				shared[key] = append(shared[key], topic)
			}
		}
	}

	defeaters := exceptsIndex(g, cfg)
	keys := make([]pair, 0, len(shared))
	for key := range shared {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(x, y pair) int { return cmp.Or(cmp.Compare(x.a, y.a), cmp.Compare(x.b, y.b)) })
	for _, key := range keys {
		modalityA, modalityB := clauseModality(g, key.a), clauseModality(g, key.b)
		strong, conflicting := incompatible(modalityA, modalityB)
		if !conflicting {
			continue
		}
		c := ModalityConflict{
			A: key.a, B: key.b,
			ModalityA: modalityA, ModalityB: modalityB,
			Topics: shared[key],
			Strong: strong,
		}
		// A strict pair is not suppressible, so no defeater is looked up for
		// one: an excepts edge pointing at a strict rule is a finding of its
		// own, not an annotation on this one.
		if !strong {
			c.Defeater, c.Suppressed = defeater(defeaters, key.a, key.b)
		}
		conflicts = append(conflicts, c)
	}
	return conflicts
}

// clauseModality reads the strength a clause states: the empty string where the
// corpus does not hold the document or the document states none, which collides
// with nothing.
func clauseModality(g *model.Graph, id model.ID) string {
	n, known := g.Node(id)
	if !known {
		return ""
	}
	modality, _ := n.Attr(config.FieldModality)
	return modality
}

// defeater returns the excepts edge recorded between two clauses, whichever of
// them declared it: an exception answers the pair, and which side wrote it down
// is a matter of which clause is the more specific one.
func defeater(index map[edgeKey]model.Edge, a, b model.ID) (model.Edge, bool) {
	if e, ok := index[edgeKey{from: a, to: b}]; ok {
		return e, true
	}
	e, ok := index[edgeKey{from: b, to: a}]
	return e, ok
}

// incompatible answers ADR-0003's conflict table. Two modalities collide
// exactly when one of them forbids and the other does not — every × of the
// table and no other cell — and the collision is strong where both are strict
// rules, which is the MUST against MUST_NOT. A value outside the vocabulary
// states nothing to collide with; it is an unknown_field_value of its own.
func incompatible(a, b string) (strong, conflicting bool) {
	if !slices.Contains(config.Modalities, a) || !slices.Contains(config.Modalities, b) {
		return false, false
	}
	if config.Prohibition(a) == config.Prohibition(b) {
		return false, false
	}
	return config.StrictModality(a) && config.StrictModality(b), true
}

// exceptsIndex maps each recorded exception onto the edge that records it, so a
// pair can ask for the defeater in either direction without walking the edges
// again. It is empty for a configuration that declares no excepts edge, which
// is what "this corpus records no exceptions" is.
func exceptsIndex(g *model.Graph, cfg config.Config) map[edgeKey]model.Edge {
	index := map[edgeKey]model.Edge{}
	spec, declared := cfg.Edge(config.EdgeExcepts)
	if !declared {
		return index
	}
	for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
		index[edgeKey{from: e.From, to: e.To}] = e
	}
	return index
}

// CheckExceptsStrict reports an exception recorded against a strict rule. A
// defeater does not draw a conclusion of its own; it stops a defeasible one
// from being drawn, and a strict rule's consequence follows without exception —
// so an excepts edge pointing at a MUST or a MUST_NOT records something that
// cannot happen. It is a finding of its own rather than the edge's `target:`
// because what is wrong is the exception, not a target gone stale.
func CheckExceptsStrict(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	spec, declared := cfg.Edge(config.EdgeExcepts)
	if !declared {
		return findings
	}
	for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
		target, known := g.Node(e.To)
		if !known {
			continue
		}
		modality, _ := target.Attr(config.FieldModality)
		if !config.StrictModality(modality) {
			continue
		}
		owner, ownerKnown := g.Node(edgeOwner(spec, e))
		if !ownerKnown {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: cfg.Severity(model.RuleExceptsStrict),
			Rule:     model.RuleExceptsStrict,
			ID:       owner.ID,
			Detail: fmt.Sprintf("%s targets %s, which is %s and cannot be defeated",
				spec.Name, target.ID, modality),
			Location: edgeLocation(cfg, owner, e),
			Related:  []model.Location{modalityLocation(cfg, target)},
		})
	}
	SortFindings(findings)
	return findings
}
