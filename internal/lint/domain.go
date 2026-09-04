package lint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// domain returns the closed vocabulary an attribute takes its values from, and
// whether there is one at all. kind narrows the answer to one document kind
// where the conjunction fixes it; a key whose vocabulary depends on the kind
// has no closed domain until it is fixed, and an open domain is what keeps lint
// from reporting a value it simply cannot judge.
func (a analyzer) domain(key, kind string) ([]string, bool) {
	if key == a.cfg.EffectiveStatus() {
		return a.statusDomain(kind)
	}
	if key == config.KeyKind && a.cfg.Multikind() {
		return a.cfg.KindNames(), true
	}
	// A projection reads as a boolean attribute, so its domain is the two words
	// a condition compares it against — and so does the one attribute the
	// engine computes rather than reads.
	if _, declared := a.cfg.Projection(key); declared || key == config.AttrInForce {
		return []string{config.ProjectionTrue, config.ProjectionFalse}, true
	}
	return a.fieldDomain(key, kind)
}

// statusDomain is the vocabulary a status may come from: one kind's where the
// kind is known, and the union over every kind otherwise. The union is closed
// only where every kind answers to a vocabulary — a kind that declares none
// admits any status at all, and a union that left it out would call a perfectly
// writable value impossible.
func (a analyzer) statusDomain(kind string) ([]string, bool) {
	if kind != "" {
		values := a.cfg.KindStatusValues(kind)
		return values, len(values) > 0
	}
	if !a.cfg.Multikind() {
		return a.cfg.StatusValues, len(a.cfg.StatusValues) > 0
	}
	all := []string{}
	for _, name := range a.cfg.KindNames() {
		values := a.cfg.KindStatusValues(name)
		if len(values) == 0 {
			return nil, false
		}
		all = append(all, values...)
	}
	return compacted(all), len(all) > 0
}

// fieldDomain is the vocabulary a declared field takes, read the way the status
// vocabulary is: one kind's where the kind is known, and the union otherwise,
// closed only where every kind declares one.
func (a analyzer) fieldDomain(key, kind string) ([]string, bool) {
	if kind != "" {
		spec, declared := a.cfg.Field(kind, key)
		return spec.OneOf, declared && len(spec.OneOf) > 0
	}
	if !a.cfg.Multikind() {
		spec, declared := a.cfg.Fields[key]
		return spec.OneOf, declared && len(spec.OneOf) > 0
	}
	all := []string{}
	for _, name := range a.cfg.KindNames() {
		spec, declared := a.cfg.Field(name, key)
		if !declared || len(spec.OneOf) == 0 {
			return nil, false
		}
		all = append(all, spec.OneOf...)
	}
	return compacted(all), len(all) > 0
}

// describeDomain names a vocabulary the way a finding ends: the words a value
// has to be one of.
func describeDomain(values []string) string { return strings.Join(values, ", ") }

func compacted(values []string) []string {
	sorted := slices.Sorted(slices.Values(values))
	return slices.Compact(sorted)
}

// kindConstraint is one narrowing of the kinds a document under analysis may
// have, and the clause that narrowed it.
type kindConstraint struct {
	written string
	kinds   []string
}

// kinds reports the document kinds a conjunction admits, given the kinds the
// unit it belongs to is evaluated over. narrowed is false on a corpus that
// declares no kinds, where there is nothing to narrow and no contradiction to
// find. A contradiction is an empty set of kinds, and written says which two
// clauses closed it.
func (a analyzer) kinds(c conjunct, scope []string) (kinds []string, written string, narrowed bool) {
	if !a.cfg.Multikind() {
		return nil, "", false
	}
	constraints := make([]kindConstraint, 0, len(c.literals)+1)
	if len(scope) > 0 {
		constraints = append(constraints, kindConstraint{written: "the edge's to: kinds", kinds: scope})
	}
	for _, l := range c.literals {
		if l.negate {
			continue
		}
		switch {
		case l.kind == litEq && l.key == config.KeyKind:
			constraints = append(constraints, kindConstraint{written: l.String(), kinds: []string{l.value}})
		case l.kind == litDegree || l.kind == litVia:
			spec, declared := a.cfg.Edge(model.EdgeType(l.key))
			// The document under analysis is at the near end of the clause: the
			// head of an inbound edge, the tail of an outbound one.
			ends := spec.From
			if l.inbound {
				ends = spec.To
			}
			if !declared || len(ends) == 0 {
				continue
			}
			constraints = append(constraints, kindConstraint{written: l.String(), kinds: ends})
		}
	}
	if len(constraints) == 0 {
		return a.cfg.KindNames(), "", false
	}
	kinds, written = constraints[0].kinds, constraints[0].written
	for _, constraint := range constraints[1:] {
		next := make([]string, 0, len(kinds))
		for _, name := range kinds {
			if slices.Contains(constraint.kinds, name) {
				next = append(next, name)
			}
		}
		kinds = next
		if len(kinds) == 0 {
			return nil, fmt.Sprintf("%s and %s", written, constraint.written), true
		}
		written = fmt.Sprintf("%s and %s", written, constraint.written)
	}
	return kinds, written, true
}

// fixedKind names the one kind a conjunction leaves, or the empty string where
// it leaves several. It is what the vocabularies are read against.
func (a analyzer) fixedKind(c conjunct, scope []string) string {
	kinds, _, narrowed := a.kinds(c, scope)
	if !narrowed || len(kinds) != 1 {
		return ""
	}
	return kinds[0]
}

// unsatisfiable reports why no document could satisfy a conjunction, and
// whether that is so. The checks run in a fixed order, so one contradictory
// conjunction always reports the same reason.
//
// Every judgement here is a set operation over finite domains: the attribute
// vocabularies are closed, the degrees are an integer interval, and the kinds an
// edge joins are a list. Nothing here searches, which is why lint needs no
// solver.
func (a analyzer) unsatisfiable(c conjunct, scope []string) (string, bool) {
	if reason, unsat := contradictoryLiterals(c); unsat {
		return reason, true
	}
	if kinds, written, narrowed := a.kinds(c, scope); narrowed && len(kinds) == 0 {
		return "no document kind satisfies " + written, true
	}
	if reason, unsat := a.contradictoryDegrees(c); unsat {
		return reason, true
	}
	return a.contradictoryAttrs(c, scope)
}

// contradictoryLiterals reports a claim a conjunction makes together with its
// own negation, and a one-hop clause no neighbour could satisfy.
func contradictoryLiterals(c conjunct) (string, bool) {
	for _, l := range c.literals {
		if l.unsat {
			return l.reason, true
		}
		if slices.Contains(c.literals, negated(l)) {
			// The pair is reported once, from the side that reads as the claim
			// rather than as its denial.
			if l.negate || l.kind == litNot || l.kind == litNotContains {
				continue
			}
			return fmt.Sprintf("%s and %s cannot both hold", l, negated(l)), true
		}
	}
	return "", false
}

// contradictoryDegrees reports two degree clauses about one edge that no
// document can answer at once, and a clause asking for more edges than the edge
// itself admits.
func (a analyzer) contradictoryDegrees(c conjunct) (string, bool) {
	for i, l := range c.literals {
		if l.kind != litDegree || l.negate {
			continue
		}
		if reason, unsat := a.aboveDeclaredDegree(l); unsat {
			return reason, true
		}
		for _, other := range c.literals[i+1:] {
			if other.kind != litDegree || other.negate || other.key != l.key || other.inbound != l.inbound {
				continue
			}
			atLeast := max(l.min, other.min)
			atMost := 0
			for _, bound := range []int{l.max, other.max} {
				if bound > 0 && (atMost == 0 || bound < atMost) {
					atMost = bound
				}
			}
			if atMost > 0 && atLeast > atMost {
				return fmt.Sprintf("%s and %s ask for at least %d and at most %d edges", l, other, atLeast, atMost), true
			}
		}
		// A one-hop clause needs an edge to cross, so a clause forbidding that
		// edge and a clause crossing it cannot both hold.
		if slices.Contains(c.literals, literal{kind: litAbsent, key: l.key, inbound: l.inbound}) {
			return fmt.Sprintf("%s and not_%s cannot both hold", l, l.key), true
		}
	}
	for _, l := range c.literals {
		if l.kind != litVia || l.negate {
			continue
		}
		if slices.Contains(c.literals, literal{kind: litAbsent, key: l.key, inbound: l.inbound}) {
			return fmt.Sprintf("%s needs an edge that %s forbids", l,
				literal{kind: litAbsent, key: l.key, inbound: l.inbound}), true
		}
	}
	return "", false
}

// aboveDeclaredDegree reports a threshold no document can reach because the
// edge's own bound stops it, per side: an inbound clause answers to
// max_inbound and an outbound one to max_outbound.
func (a analyzer) aboveDeclaredDegree(l literal) (string, bool) {
	spec, declared := a.cfg.Edge(model.EdgeType(l.key))
	if !declared {
		return "", false
	}
	bound, key := spec.MaxOutbound, "max_outbound"
	if l.inbound {
		bound, key = spec.MaxInbound, "max_inbound"
	}
	if bound > 0 && l.min > bound {
		return fmt.Sprintf("%s asks for %d edges, above the edge's %s %d", l, l.min, key, bound), true
	}
	return "", false
}

// contradictoryAttrs reports two attribute clauses about one key that no value
// satisfies. Only a conjunction holding an eq is judged: eq is the one clause
// that forces the key to be present and to hold one definite value, and every
// other clause is satisfied by a document that does not write the key at all.
func (a analyzer) contradictoryAttrs(c conjunct, scope []string) (string, bool) {
	kind := a.fixedKind(c, scope)
	for i, l := range c.literals {
		if l.kind != litEq || l.negate {
			continue
		}
		for _, other := range c.literals[i+1:] {
			if other.key != l.key || other.negate {
				continue
			}
			switch {
			case other.kind == litEq && !strings.EqualFold(other.value, l.value):
				return fmt.Sprintf("attr %s cannot be both %q and %q", l.key, l.value, other.value), true
			case other.kind == litContains && !strings.EqualFold(other.value, l.value):
				return fmt.Sprintf("attr %s cannot be %q and contain %q", l.key, l.value, other.value), true
			case (other.kind == litNot || other.kind == litNotContains) && strings.EqualFold(other.value, l.value):
				return fmt.Sprintf("attr %s cannot be %q and %s %q", l.key, l.value, other.kind, other.value), true
			case other.kind == litSubset && !containsFold(strings.Split(other.value, ", "), l.value):
				return fmt.Sprintf("attr %s cannot be %q and a subset of %s", l.key, l.value, other.value), true
			}
		}
		values, closed := a.domain(l.key, kind)
		if closed && !containsFold(values, l.value) {
			return fmt.Sprintf("attr %s: %q is outside %s", l.key, l.value, describeDomain(values)), true
		}
	}
	return "", false
}

// covers reports whether a disjunction leaves no value of an attribute's
// vocabulary out, which is what makes a condition written as an any_of over a
// vocabulary hold for every document that writes the key. It answers only for
// the shape the anomaly takes: alternatives that each pin one key to one value.
func (a analyzer) covers(conjuncts []conjunct, scope []string) (key string, values []string, exhausted bool) {
	if len(conjuncts) == 0 {
		return "", nil, false
	}
	named := []string{}
	for _, c := range conjuncts {
		if len(c.literals) != 1 || c.literals[0].kind != litEq || c.literals[0].negate {
			return "", nil, false
		}
		l := c.literals[0]
		if key != "" && l.key != key {
			return "", nil, false
		}
		key = l.key
		named = append(named, l.value)
	}
	domain, closed := a.domain(key, a.fixedKind(conjunct{}, scope))
	if !closed {
		return "", nil, false
	}
	for _, value := range domain {
		if !containsFold(named, value) {
			return "", nil, false
		}
	}
	return key, domain, true
}
