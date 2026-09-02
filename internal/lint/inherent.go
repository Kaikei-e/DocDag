package lint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// unit is one thing layer 1 reasons about: a rule, a projection or the target
// condition of an edge. Each carries the disjunctive normal form of what it
// asks of a document, the kinds it is evaluated over, and where it was written.
type unit struct {
	section  string
	name     string
	subject  string
	severity model.Severity
	// scope is the kinds a document under this unit may have, which is empty
	// for a rule or a projection — they are evaluated over every document — and
	// the edge's own to: kinds for a target condition.
	scope    []string
	dnf      []conjunct
	complete bool
	// conditions is every condition the unit evaluates, nested ones included,
	// which is what the accounting checks read: they ask which words appear
	// rather than what they imply.
	conditions []config.Condition
}

// rule reports whether a unit is a rule, which is the only kind of unit the
// subsumption and fix checks compare.
func (u unit) rule() bool { return u.section == SectionRules }

// detail prefixes what a finding says with the part of the unit it is about. A
// rule and a projection are named by the finding's subject already; an edge's
// target condition is one clause of a larger declaration and has to say so.
func (u unit) detail(text string) string {
	if u.section == SectionEdges {
		return "target: " + text
	}
	return text
}

// units returns everything layer 1 reasons about, in configuration order: the
// rules, the projections and the edge target conditions.
func (a analyzer) units() []unit {
	units := make([]unit, 0, len(a.cfg.Rules)+len(a.cfg.Projections)+len(a.cfg.Edges))
	for _, rule := range a.cfg.Rules {
		dnf, complete := a.expand(rule.When, false, 0)
		units = append(units, unit{
			section:    SectionRules,
			name:       rule.Name,
			subject:    "rule",
			severity:   rule.Severity,
			dnf:        dnf,
			complete:   complete,
			conditions: rule.When.Conditions(),
		})
	}
	for _, spec := range a.cfg.Projections {
		dnf, complete := []conjunct{}, true
		for _, when := range spec.Whens() {
			expanded, ok := a.expand(when, false, 0)
			dnf, complete = append(dnf, expanded...), complete && ok
		}
		units = append(units, unit{
			section:    SectionProjections,
			name:       spec.Name,
			subject:    "projection",
			dnf:        dedupe(dnf),
			complete:   complete,
			conditions: spec.Conditions(),
		})
	}
	for _, spec := range a.cfg.Edges {
		if spec.Target == nil {
			continue
		}
		when := spec.Target.Condition
		dnf, complete := a.expand(when, false, 0)
		// leaf_of is not_inbound under a name that says what it is for, so the
		// analysis reads it as the clause it stands for.
		if leaf := spec.Target.LeafOf; leaf != "" {
			absent := []conjunct{newConjunct([]literal{{kind: litAbsent, key: leaf, inbound: true}})}
			dnf, complete = product(dnf, absent, complete)
		}
		units = append(units, unit{
			section:    SectionEdges,
			name:       spec.Name,
			subject:    "edge",
			scope:      spec.To,
			dnf:        dnf,
			complete:   complete,
			conditions: when.Conditions(),
		})
	}
	return units
}

// Inherent reports what is wrong with a configuration without reading a single
// document: conditions no document could satisfy, conditions every document
// satisfies, rules that say what another rule already says, and vocabulary
// nothing reads. It is the layer Fisman and colleagues call inherent vacuity —
// a fault of the specification rather than of the model it is checked against.
func Inherent(cfg config.Config, loc Locator) []model.Finding {
	a := analyzer{cfg: cfg}
	units := a.units()
	findings := []model.Finding{}
	for _, u := range units {
		findings = append(findings, a.unitFindings(u, loc)...)
	}
	findings = append(findings, a.subsumption(units, loc)...)
	findings = append(findings, a.ambivalentFixes(units, loc)...)
	findings = append(findings, a.unusedEdges(units, loc)...)
	findings = append(findings, a.unusedStatuses(units, loc)...)
	findings = append(findings, a.preferTarget(loc)...)
	graph.SortFindings(findings)
	return findings
}

// unitFindings reports what one rule, projection or target condition says about
// itself: that no document can satisfy it, or that every document does.
func (a analyzer) unitFindings(u unit, loc Locator) []model.Finding {
	findings := []model.Finding{}
	at := loc.Locate(u.section, u.name)
	if !u.complete {
		findings = append(findings, model.Finding{
			Severity: model.SeverityWarn,
			Rule:     FindingConditionTooWide,
			ID:       model.ID(u.name),
			Detail: u.detail(fmt.Sprintf("expands past %d alternatives, so the rest is not analyzed",
				maxConjuncts)),
			Location: at,
			Fix:      "split the any_of nesting into separate " + u.section,
		})
	}
	reasons := make([]string, 0, len(u.dnf))
	unsatisfiable := 0
	for _, c := range u.dnf {
		reason, unsat := a.unsatisfiable(c, u.scope)
		if !unsat {
			continue
		}
		unsatisfiable++
		if !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}
	switch {
	case len(u.dnf) > 0 && unsatisfiable == len(u.dnf):
		findings = append(findings, a.deadUnit(u, at, reasons[0]))
	default:
		for _, reason := range reasons {
			findings = append(findings, model.Finding{
				Severity: model.SeverityError,
				Rule:     FindingUnsatisfiableCondition,
				ID:       model.ID(u.name),
				Detail:   u.detail("one alternative contradicts itself: " + reason),
				Location: at,
				Fix:      "drop the alternative or the clause that contradicts it",
			})
		}
	}
	if u.complete && unsatisfiable == 0 {
		findings = append(findings, a.tautology(u, at)...)
	}
	return findings
}

// deadUnit reports a unit no document can satisfy under the name its own
// section carries: a rule that can never fire, a projection that can never
// hold, and a target condition that no target could answer.
func (a analyzer) deadUnit(u unit, at model.Location, reason string) model.Finding {
	f := model.Finding{
		Severity: model.SeverityError,
		ID:       model.ID(u.name),
		Location: at,
		Detail:   u.detail("every alternative contradicts itself: " + reason),
	}
	switch u.section {
	case SectionRules:
		f.Rule = FindingUnfirableRule
		f.Fix = "drop the rule, or the clause that contradicts the rest"
	case SectionProjections:
		f.Rule = FindingUnsatisfiableProjection
		f.Fix = "drop the projection, or the clause that contradicts the rest"
	default:
		f.Rule = FindingUnsatisfiableCondition
		f.Fix = "drop the target condition, or the clause that contradicts the rest"
	}
	return f
}

// tautology reports a unit every document satisfies: one that constrains
// nothing, and one whose alternatives leave no value of a vocabulary out. Both
// are vacuous in the sense the model checkers mean — the antecedent is always
// true, so what the unit reports says nothing about the document it is filed on.
func (a analyzer) tautology(u unit, at model.Location) []model.Finding {
	name := FindingTautologicalRule
	if u.section == SectionProjections {
		name = FindingTautologicalProjection
	}
	if u.section == SectionEdges {
		return nil
	}
	empty := slices.ContainsFunc(u.dnf, func(c conjunct) bool { return len(c.literals) == 0 })
	if empty {
		return []model.Finding{{
			Severity: model.SeverityWarn,
			Rule:     name,
			ID:       model.ID(u.name),
			Detail:   "constrains nothing, so it holds for every document",
			Location: at,
			Fix:      "write a when block, or drop the " + u.subject,
		}}
	}
	key, values, exhausted := a.covers(u.dnf, u.scope)
	if !exhausted {
		return nil
	}
	return []model.Finding{{
		Severity: model.SeverityWarn,
		Rule:     name,
		ID:       model.ID(u.name),
		Detail: fmt.Sprintf("the alternatives cover the whole vocabulary of %s (%s), so it holds for every document that writes it",
			key, describeDomain(values)),
		Location: at,
		Fix:      "drop an alternative, or the " + u.subject,
	}}
}

// subsumption reports a rule that says what another rule already says. A rule
// whose every alternative is at least as demanding as one of another rule's
// fires only where that other rule fires too, so one of the two reports
// nothing a reader has not already read.
//
// A tautological or unfirable rule is left out of the comparison: everything is
// subsumed by a rule that fires everywhere and nothing is subsumed by one that
// fires nowhere, and both are already reported as what they are.
func (a analyzer) subsumption(units []unit, loc Locator) []model.Finding {
	comparable := make([]unit, 0, len(units))
	for _, u := range units {
		if !u.rule() || !u.complete || !a.live(u) {
			continue
		}
		comparable = append(comparable, u)
	}
	findings := []model.Finding{}
	for i, narrow := range comparable {
		for j, wide := range comparable {
			if i == j || !implies(narrow, wide) {
				continue
			}
			// Two rules that imply each other are one rule written twice: it is
			// reported once, against the one written later.
			if implies(wide, narrow) && i < j {
				continue
			}
			findings = append(findings, subsumedFinding(narrow, wide, loc))
		}
	}
	return findings
}

// live reports whether a rule has an alternative a document could satisfy and
// is not satisfied by every document.
func (a analyzer) live(u unit) bool {
	if len(u.dnf) == 0 {
		return false
	}
	if slices.ContainsFunc(u.dnf, func(c conjunct) bool { return len(c.literals) == 0 }) {
		return false
	}
	if _, _, exhausted := a.covers(u.dnf, u.scope); exhausted {
		return false
	}
	for _, c := range u.dnf {
		if _, unsat := a.unsatisfiable(c, u.scope); unsat {
			return false
		}
	}
	return true
}

// implies reports whether every alternative of one rule claims everything some
// alternative of the other claims, which is exactly when the first fires only
// where the second does.
func implies(narrow, wide unit) bool {
	for _, c := range narrow.dnf {
		if !slices.ContainsFunc(wide.dnf, func(other conjunct) bool { return c.covers(other) }) {
			return false
		}
	}
	return true
}

// subsumedFinding names the two rules and which of them a reader loses nothing
// by dropping. A rule subsumed by a weaker one is a duplicate; one subsumed by
// a stronger one is shadowed — it fires only where the stronger rule already
// failed the build, so nobody ever sees it.
func subsumedFinding(narrow, wide unit, loc Locator) model.Finding {
	f := model.Finding{
		Severity: model.SeverityWarn,
		Rule:     FindingSubsumedRule,
		ID:       model.ID(narrow.name),
		Detail:   fmt.Sprintf("fires only where %s fires, and says the same at %s", wide.name, narrow.severity),
		Location: loc.Locate(narrow.section, narrow.name),
		Related:  []model.Location{loc.Locate(wide.section, wide.name)},
		Fix:      "drop one of the two, or narrow this one",
	}
	if narrow.severity.Rank() < wide.severity.Rank() {
		f.Rule = FindingShadowedRule
		f.Detail = fmt.Sprintf("fires only where %s fires, which already reports at %s", wide.name, wide.severity)
	}
	return f
}

// ambivalentFixes reports two rules whose remedies tell a reader to write two
// different values into one frontmatter key. Only the remedies DocDag itself
// generates are compared — the `set <field>: <value>` shapes named in
// graph.FixSetsField — because a rule's own message is prose and lint does not
// read prose.
func (a analyzer) ambivalentFixes(units []unit, loc Locator) []model.Finding {
	type demand struct {
		rule  config.Rule
		unit  unit
		field string
		value string
	}
	demands := []demand{}
	for _, rule := range a.cfg.Rules {
		field, value, ok := graph.FixSetsField(a.cfg, rule.Name)
		if !ok {
			continue
		}
		index := slices.IndexFunc(units, func(u unit) bool { return u.rule() && u.name == rule.Name })
		if index < 0 {
			continue
		}
		demands = append(demands, demand{rule: rule, unit: units[index], field: field, value: value})
	}
	findings := []model.Finding{}
	for i, first := range demands {
		for _, second := range demands[i+1:] {
			if first.field != second.field || strings.EqualFold(first.value, second.value) {
				continue
			}
			if !a.compatible(first.unit, second.unit) {
				continue
			}
			findings = append(findings, model.Finding{
				Severity: model.SeverityError,
				Rule:     FindingAmbivalentFix,
				ID:       model.ID(second.rule.Name),
				Detail: fmt.Sprintf("its fix sets %s: %s where %s sets %s: %s, and both can fire on one document",
					second.field, second.value, first.rule.Name, first.field, first.value),
				Location: loc.Locate(SectionRules, second.rule.Name),
				Related:  []model.Location{loc.Locate(SectionRules, first.rule.Name)},
				Fix:      "narrow one of the two rules so they cannot both fire",
			})
		}
	}
	return findings
}

// compatible reports whether two units can hold for one document, which is the
// case when some alternative of each is satisfiable together with some
// alternative of the other.
func (a analyzer) compatible(first, second unit) bool {
	for _, left := range first.dnf {
		for _, right := range second.dnf {
			if _, unsat := a.unsatisfiable(left.merge(right), append(slices.Clone(first.scope), second.scope...)); !unsat {
				return true
			}
		}
	}
	return false
}

// engineEdges are the edge types DocDag reads by name whatever a configuration
// says about them: supersedes resolves lineages, counts chain depth and is what
// `new --supersedes` writes; depends-on is the other edge `new` writes; about
// and excepts are what the modality conflict check and its suppression are made
// of. An edge of one of those names is used by the engine itself, so it is
// never reported as unused.
var engineEdges = []model.EdgeType{
	config.EdgeSupersedes,
	config.EdgeDependsOn,
	config.EdgeAbout,
	config.EdgeExcepts,
}

// unusedEdges reports an edge type nothing reads: no rule, projection, target
// condition or path constraint names it, no derived edge produces it, it
// mirrors nothing under an inverse key, it constrains neither its endpoints nor
// its degree nor its attributes, and the engine does not read it by name. Such
// an edge is parsed and drawn and nothing else — Preece and Shinghal's unused
// input, which is a symptom of a rule that was renamed or never written.
func (a analyzer) unusedEdges(units []unit, loc Locator) []model.Finding {
	read := a.readEdges(units)
	findings := []model.Finding{}
	for _, spec := range a.cfg.Edges {
		if read[spec.Name] {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityWarn,
			Rule:     FindingUnusedEdge,
			ID:       model.ID(spec.Name),
			Detail:   "is declared but no rule, projection, target, path constraint or derived edge reads it",
			Location: loc.Locate(SectionEdges, spec.Name),
			Fix:      "write a rule that reads it, constrain its endpoints or degree, or drop the edge",
		})
	}
	return findings
}

// readEdges names every edge type something in the configuration reads.
func (a analyzer) readEdges(units []unit) map[string]bool {
	read := make(map[string]bool, len(a.cfg.Edges))
	for _, t := range engineEdges {
		read[t.String()] = true
	}
	for _, spec := range a.cfg.Edges {
		// A declaration that a check reads is a use: the endpoint kinds are
		// edge_kind_mismatch, the bounds are cardinality, the attributes are the
		// three edge_attr checks, acyclic is cycle, and an inverse key is
		// inverse_mismatch.
		if len(spec.From) > 0 || len(spec.To) > 0 || spec.Acyclic || spec.Inverse != "" ||
			spec.MaxInbound > 0 || spec.MaxOutbound > 0 || spec.MinOutbound > 0 || len(spec.Attrs) > 0 {
			read[spec.Name] = true
		}
		if spec.Target == nil {
			continue
		}
		read[spec.Name] = true
		if spec.Target.LeafOf != "" {
			read[spec.Target.LeafOf] = true
		}
	}
	for _, spec := range a.cfg.DerivedEdges {
		read[spec.Edge] = true
	}
	for _, constraint := range a.cfg.PathConstraints {
		for _, step := range config.PathSteps(append(slices.Clone(constraint.Path), constraint.SubsetOf...)) {
			read[step.Edge] = true
		}
	}
	for _, u := range units {
		for _, cond := range u.conditions {
			for _, clause := range cond.EdgeClauses() {
				read[clause.Edge] = true
			}
			for _, clause := range cond.ViaClauses() {
				read[clause.Edge] = true
			}
		}
	}
	return read
}

// unusedStatuses reports a status vocabulary nothing reasons about. The finding
// is about the vocabulary rather than about a single value, because a value a
// rule does not name still changes what the rule answers: `status: {eq:
// accepted}` answers differently for `accepted` than for every other word of
// the vocabulary, so the unused input is the vocabulary no condition reads at
// all — for the documents of a kind whose status nothing asks about,
// unknown_status is the only check the words ever reach.
func (a analyzer) unusedStatuses(units []unit, loc Locator) []model.Finding {
	read := a.statusReaders(units)
	findings := []model.Finding{}
	if !a.cfg.Multikind() {
		if len(a.cfg.StatusValues) == 0 || read[""] {
			return findings
		}
		return append(findings, unusedStatus("", a.cfg.StatusValues, loc.Section(SectionStatusValues)))
	}
	for _, name := range a.cfg.KindNames() {
		values := a.cfg.Kinds[name].StatusValues
		if len(values) == 0 || read[name] {
			continue
		}
		findings = append(findings, unusedStatus(name, values, loc.Locate(SectionKinds, name)))
	}
	return findings
}

func unusedStatus(kind string, values []string, at model.Location) model.Finding {
	subject, id := "status_values", "status_values"
	if kind != "" {
		subject, id = fmt.Sprintf("the status_values of kind %q", kind), kind
	}
	return model.Finding{
		Severity: model.SeverityWarn,
		Rule:     FindingUnusedStatus,
		ID:       model.ID(id),
		Detail: fmt.Sprintf("%s (%s) are read by no rule, projection or target condition",
			subject, describeDomain(values)),
		Location: at,
		Fix:      "write a rule that reads the status, or drop the vocabulary",
	}
}

// statusReaders reports which kinds' status field some condition reads. The key
// "" answers for a corpus that declares no kinds, and a condition that pins no
// kind reads the status of every one of them.
func (a analyzer) statusReaders(units []unit) map[string]bool {
	field := a.cfg.EffectiveStatus()
	read := map[string]bool{}
	mark := func(kinds []string, all bool) {
		if all || !a.cfg.Multikind() {
			read[""] = true
			for _, name := range a.cfg.KindNames() {
				read[name] = true
			}
			return
		}
		for _, name := range kinds {
			read[name] = true
		}
	}
	for _, u := range units {
		for _, c := range u.dnf {
			if !slices.ContainsFunc(c.literals, func(l literal) bool { return l.key == field && isAttr(l.kind) }) {
				continue
			}
			kinds, _, narrowed := a.kinds(c, u.scope)
			mark(kinds, !narrowed)
		}
		// A one-hop clause reads the neighbour's status, which answers to the
		// vocabulary of the kinds at the far end of the edge.
		for _, cond := range u.conditions {
			for _, clause := range cond.ViaClauses() {
				if _, reads := clause.Attr[field]; !reads {
					continue
				}
				spec, declared := a.cfg.Edge(model.EdgeType(clause.Edge))
				if !declared {
					continue
				}
				far := spec.To
				if clause.Inbound {
					far = spec.From
				}
				mark(far, len(far) == 0)
			}
		}
	}
	return read
}

// isAttr reports whether a literal is one an attr clause wrote, which is what
// "reads this key" means: an edge literal's key is an edge type.
func isAttr(kind litKind) bool {
	switch kind {
	case litEq, litNot, litContains, litNotContains, litSubset:
		return true
	}
	return false
}

// preferTarget reports a path constraint the shorter vocabulary already says.
// A two-step path that must reach nothing says that whatever the first step
// reaches has no second step to take, and an edge's own `target:` says exactly
// that about the document it points at — the same walk, one hop shorter, filed
// on the document that declared the edge rather than on the one the path
// started from.
//
// Only that shape is reported. A path whose first step is walked backwards
// starts at a document the edge points at rather than at one that declared it,
// which no target condition can constrain, and a one-step path is about the
// documents an edge leaves rather than about the one it reaches.
func (a analyzer) preferTarget(loc Locator) []model.Finding {
	findings := []model.Finding{}
	for _, constraint := range a.cfg.PathConstraints {
		steps := config.PathSteps(constraint.Path)
		if constraint.Equals != config.PathEqualsNone || len(steps) != 2 || steps[0].Inbound {
			continue
		}
		word := "not_outbound"
		if steps[1].Inbound {
			word = "not_inbound"
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityWarn,
			Rule:     FindingPreferTarget,
			ID:       model.ID(constraint.Name),
			Detail: fmt.Sprintf("%s says what target: {%s: %s} on edge %s says, one hop shorter",
				config.PathString(constraint.Path), word, steps[1].Edge, steps[0].Edge),
			Location: loc.Locate(SectionPathConstraints, constraint.Name),
			Related:  []model.Location{loc.Locate(SectionEdges, steps[0].Edge)},
			Fix: fmt.Sprintf("declare target: {%s: %s} on edge %s and drop the path constraint",
				word, steps[1].Edge, steps[0].Edge),
		})
	}
	return findings
}
