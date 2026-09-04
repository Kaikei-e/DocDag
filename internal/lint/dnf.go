package lint

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// The two bounds the expansion is held to. maxConjuncts is where a condition
// stops being analyzed and becomes a condition_too_wide finding, and maxDepth
// is the nesting the walk answers for. Both exist for the same reason: the
// vocabulary lets a configuration nest any_of and not to whatever depth it
// likes, and lint must terminate on a configuration nobody would write.
const (
	maxConjuncts = 64
	maxDepth     = 16
)

// litKind names the shape of one literal. The five attribute kinds are the
// operands an attr clause is written with, and the three that follow are the
// edge and one-hop clauses.
type litKind string

const (
	litEq          litKind = "eq"
	litNot         litKind = "not"
	litContains    litKind = "contains"
	litNotContains litKind = "not_contains"
	litSubset      litKind = "subset_of"
	litDegree      litKind = "degree"
	litAbsent      litKind = "absent"
	litVia         litKind = "via"
)

// literal is one indivisible claim a conjunction makes about the document under
// analysis. Every field is comparable, so a conjunction is a set of literals and
// implication between two conjunctions is set inclusion — which is all the
// subsumption checks need, and the direct return of the fixed vocabulary: there
// is no expression to interpret, only a finite set of words.
type literal struct {
	kind    litKind
	key     string // the attribute key, or the edge type
	value   string // what an attribute clause names, or a one-hop clause's shape
	inbound bool
	// negate marks a literal the conjunction requires to be false. It is only
	// ever set where the negation is not itself a word of the vocabulary: an
	// absent edge and an unequal attribute have their own spellings.
	negate   bool
	min, max int
	// unsat marks a one-hop clause no neighbour could satisfy, decided where the
	// literal is built because that is where the neighbour's own vocabulary and
	// kinds are still in hand. reason says why, and is empty otherwise.
	unsat  bool
	reason string
}

// String renders a literal the way the configuration wrote it, so a finding
// names the words a reader has to look for in the file.
func (l literal) String() string {
	direction := "outbound"
	if l.inbound {
		direction = "inbound"
	}
	var written string
	switch l.kind {
	case litDegree:
		written = fmt.Sprintf("%s %s%s", direction, l.key, degreeWindow(l.min, l.max))
	case litAbsent:
		written = fmt.Sprintf("not_%s %s", direction, l.key)
	case litVia:
		word := "via"
		if l.inbound {
			word = "via_inbound"
		}
		written = fmt.Sprintf("%s %s {%s}", word, l.key, l.value)
	default:
		written = fmt.Sprintf("attr %s: %s %s", l.key, l.kind, l.value)
	}
	if l.negate {
		return "not " + written
	}
	return written
}

// degreeWindow renders the degrees a clause accepts, and nothing at all for the
// window a bare edge name asks for: one edge or more is what every clause means
// until it says otherwise.
func degreeWindow(min, max int) string {
	switch {
	case min == 1 && max == 0:
		return ""
	case max == 0:
		return fmt.Sprintf(" (min %d)", min)
	}
	return fmt.Sprintf(" (min %d, max %d)", min, max)
}

// compareLiterals orders literals so a conjunction is a canonical set and every
// message built from one reads the same on every run.
func compareLiterals(a, b literal) int {
	if c := strings.Compare(string(a.kind), string(b.kind)); c != 0 {
		return c
	}
	if c := strings.Compare(a.key, b.key); c != 0 {
		return c
	}
	if c := strings.Compare(a.value, b.value); c != 0 {
		return c
	}
	if a.inbound != b.inbound {
		return boolOrder(a.inbound) - boolOrder(b.inbound)
	}
	if a.negate != b.negate {
		return boolOrder(a.negate) - boolOrder(b.negate)
	}
	if c := a.min - b.min; c != 0 {
		return c
	}
	return a.max - b.max
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// negated returns the literal that holds exactly where this one does not.
// Two negations are the vocabulary's own words — an absent edge negates an
// existence clause and back — and everything else carries the flag, which is
// enough for a conjunction to see that a claim and its opposite are both in it.
func negated(l literal) literal {
	if l.negate {
		l.negate = false
		return l
	}
	switch l.kind {
	case litEq:
		l.kind = litNot
	case litNot:
		l.kind = litEq
	case litContains:
		l.kind = litNotContains
	case litNotContains:
		l.kind = litContains
	case litAbsent:
		// No edge of a type is the negation of one edge or more, exactly.
		l.kind, l.min, l.max = litDegree, 1, 0
	case litDegree:
		if l.min == 1 && l.max == 0 {
			l.kind, l.min, l.max = litAbsent, 0, 0
			return l
		}
		// A threshold's negation is another threshold, which the vocabulary
		// cannot write: it stays an opaque claim that this one does not hold.
		l.negate = true
	default:
		l.negate = true
	}
	return l
}

// conjunct is one conjunction of the disjunctive normal form: a finite set of
// literals, sorted and duplicate-free so two of them compare as sets.
type conjunct struct{ literals []literal }

// newConjunct returns the conjunction of a literal set, canonicalized.
func newConjunct(literals []literal) conjunct {
	sorted := slices.Clone(literals)
	slices.SortFunc(sorted, compareLiterals)
	return conjunct{literals: slices.CompactFunc(sorted, func(a, b literal) bool { return a == b })}
}

// merge returns the conjunction of two conjunctions.
func (c conjunct) merge(other conjunct) conjunct {
	return newConjunct(append(slices.Clone(c.literals), other.literals...))
}

// covers reports whether this conjunction claims everything the other one
// does, which is what makes it the stronger of the two: a document satisfying
// this one satisfies that one.
func (c conjunct) covers(other conjunct) bool {
	for _, l := range other.literals {
		if !slices.Contains(c.literals, l) {
			return false
		}
	}
	return true
}

// String renders a conjunction as the clauses it is made of, for a message.
func (c conjunct) String() string {
	parts := make([]string, 0, len(c.literals))
	for _, l := range c.literals {
		parts = append(parts, l.String())
	}
	return strings.Join(parts, " and ")
}

// analyzer reads conditions against one configuration: the vocabularies an
// attribute takes its values from, the kinds an edge joins, and the degrees it
// admits are all questions about the configuration rather than the corpus.
type analyzer struct {
	cfg config.Config
}

// expand returns the disjunctive normal form of a condition: a disjunction of
// conjunctions of literals, with negation pushed down to the literals. complete
// is false where the walk stopped short — a condition wider than maxConjuncts
// or nested deeper than maxDepth — and the caller reports condition_too_wide
// rather than drawing conclusions from a truncated form.
//
// An empty disjunction is false and a disjunction holding an empty conjunction
// is true, which is what makes the negation of an empty condition and the
// expansion of one come out right without a special case for either.
func (a analyzer) expand(cond config.Condition, negate bool, depth int) ([]conjunct, bool) {
	if depth > maxDepth {
		return []conjunct{{}}, false
	}
	own := a.literals(cond)
	if !negate {
		result, complete := []conjunct{newConjunct(own)}, true
		if len(cond.AnyOf) > 0 {
			alternatives := []conjunct{}
			for _, alternative := range cond.AnyOf {
				expanded, ok := a.expand(alternative, false, depth+1)
				complete = complete && ok
				alternatives = append(alternatives, expanded...)
			}
			result, complete = product(result, alternatives, complete)
		}
		if cond.Not != nil {
			expanded, ok := a.expand(*cond.Not, true, depth+1)
			result, complete = product(result, expanded, complete && ok)
		}
		return dedupe(result), complete
	}
	// The negation of a conjunction is the disjunction of the negations:
	// ¬(L₁ ∧ … ∧ Lₙ ∧ ⋁A ∧ ¬N) is ¬L₁ ∨ … ∨ ¬Lₙ ∨ ⋀¬A ∨ N.
	result, complete := make([]conjunct, 0, len(own)+2), true
	for _, l := range own {
		result = append(result, newConjunct([]literal{negated(l)}))
	}
	if len(cond.AnyOf) > 0 {
		combined := []conjunct{{}}
		for _, alternative := range cond.AnyOf {
			expanded, ok := a.expand(alternative, true, depth+1)
			combined, complete = product(combined, expanded, complete && ok)
		}
		result = append(result, combined...)
	}
	if cond.Not != nil {
		expanded, ok := a.expand(*cond.Not, false, depth+1)
		complete = complete && ok
		result = append(result, expanded...)
	}
	return dedupe(result), complete
}

// product distributes a conjunction over a disjunction, which is the one place
// the form can grow. It is where the width bound is enforced: past
// maxConjuncts the result is truncated and reported as incomplete, so a
// configuration cannot make lint take exponential time.
func product(left, right []conjunct, complete bool) ([]conjunct, bool) {
	if len(right) == 0 {
		return nil, complete
	}
	out := make([]conjunct, 0, min(len(left)*len(right), maxConjuncts))
	for _, l := range left {
		for _, r := range right {
			if len(out) >= maxConjuncts {
				return out, false
			}
			out = append(out, l.merge(r))
		}
	}
	return out, complete
}

// dedupe drops the conjunctions a distribution produced twice, keeping the
// order they were produced in so a report reads the way the file does.
func dedupe(conjuncts []conjunct) []conjunct {
	seen := make(map[string]bool, len(conjuncts))
	out := make([]conjunct, 0, len(conjuncts))
	for _, c := range conjuncts {
		key := c.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

// literals returns the literals a condition claims directly, leaving its any_of
// alternatives and its not block to the expansion.
func (a analyzer) literals(cond config.Condition) []literal {
	out := make([]literal, 0, 4)
	for _, clause := range cond.EdgeClauses() {
		if clause.Negate {
			out = append(out, literal{kind: litAbsent, key: clause.Edge, inbound: clause.Inbound})
			continue
		}
		out = append(out, literal{kind: litDegree, key: clause.Edge, inbound: clause.Inbound, min: clause.Min, max: clause.Max})
	}
	for _, clause := range cond.ViaClauses() {
		out = append(out, a.viaLiteral(clause))
	}
	for _, key := range slices.Sorted(maps.Keys(cond.Attr)) {
		if l, ok := attrLiteral(key, cond.Attr[key]); ok {
			out = append(out, l)
		}
	}
	return out
}

// viaLiteral builds the literal for one one-hop clause, deciding there and then
// whether any neighbour could satisfy it: the neighbour's kinds come from the
// edge and its vocabularies from the configuration, and both are in hand here.
func (a analyzer) viaLiteral(clause config.ViaClause) literal {
	l := literal{kind: litVia, key: clause.Edge, inbound: clause.Inbound, value: renderAttrs(clause.Attr)}
	l.reason, l.unsat = a.neighbourProblem(clause)
	return l
}

// neighbourProblem reports why no neighbour across a one-hop clause could
// satisfy it. The neighbour is at the far end of the edge, so its kinds are the
// edge's other side, and every attribute clause is read against the vocabulary
// those kinds answer to.
func (a analyzer) neighbourProblem(clause config.ViaClause) (string, bool) {
	spec, declared := a.cfg.Edge(model.EdgeType(clause.Edge))
	if !declared {
		return "", false
	}
	// A via crosses the edge forwards, so the neighbour is its head; a
	// via_inbound crosses it backwards, so the neighbour is its tail.
	kinds := spec.To
	if clause.Inbound {
		kinds = spec.From
	}
	kind := ""
	if len(kinds) == 1 {
		kind = kinds[0]
	}
	if written, ok := clause.Attr[config.KeyKind]; ok && written.Eq != nil && len(kinds) > 0 && !slices.Contains(kinds, *written.Eq) {
		return fmt.Sprintf("no %s neighbour is of kind %q, which the edge joins to %s",
			clause.Edge, *written.Eq, strings.Join(kinds, ", ")), true
	}
	for _, key := range slices.Sorted(maps.Keys(clause.Attr)) {
		want := clause.Attr[key]
		if want.Eq == nil {
			continue
		}
		values, closed := a.domain(key, kind)
		if closed && !containsFold(values, *want.Eq) {
			return fmt.Sprintf("no %s neighbour has %s %q, which is outside %s",
				clause.Edge, key, *want.Eq, strings.Join(values, ", ")), true
		}
	}
	return "", false
}

// attrLiteral builds the literal one attribute clause claims. A clause with no
// operand, or with more than one, is a configuration error the validator
// already rejected; it claims nothing here rather than being invented.
func attrLiteral(key string, want config.AttrCondition) (literal, bool) {
	switch {
	case want.Eq != nil:
		return literal{kind: litEq, key: key, value: *want.Eq}, true
	case want.Not != nil:
		return literal{kind: litNot, key: key, value: *want.Not}, true
	case want.Contains != nil:
		return literal{kind: litContains, key: key, value: *want.Contains}, true
	case want.NotContains != nil:
		return literal{kind: litNotContains, key: key, value: *want.NotContains}, true
	case want.SubsetOf != nil:
		return literal{kind: litSubset, key: key, value: strings.Join(want.SubsetOf, ", ")}, true
	}
	return literal{}, false
}

// renderAttrs writes a one-hop clause's attribute conditions as one canonical
// string, so two clauses asking for the same neighbour produce the same
// literal and a conjunction holding both counts them once.
func renderAttrs(attrs map[string]config.AttrCondition) string {
	parts := make([]string, 0, len(attrs))
	for _, key := range slices.Sorted(maps.Keys(attrs)) {
		if l, ok := attrLiteral(key, attrs[key]); ok {
			parts = append(parts, fmt.Sprintf("%s %s %s", key, l.kind, l.value))
		}
	}
	return strings.Join(parts, ", ")
}

func containsFold(values []string, want string) bool {
	return slices.ContainsFunc(values, func(v string) bool { return strings.EqualFold(v, want) })
}
