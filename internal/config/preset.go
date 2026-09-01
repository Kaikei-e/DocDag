package config

import (
	"fmt"
	"slices"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// DerivedSupersededPattern matches the MADR status string "superseded by <ref>"
// (and the hyphenated spellings) and captures the referenced document.
const DerivedSupersededPattern = `(?i)^superseded[\s-]+by[\s-]+(\S+)`

// ProjectionAcceptedUnsuperseded is the ADR preset's definition of what is in
// force: accepted, and superseded by nothing. It is a projection rather than
// code so another preset can hold a different notion of force.
const ProjectionAcceptedUnsuperseded = "accepted_unsuperseded"

// ADRPresetVersion is the revision the built-in ADR configuration is at. The
// preset predates the versioning, so it starts at 1 like every other: a header
// that names a revision everywhere is worth more than one that is absent
// wherever the corpus happens to use the built-in preset.
const ADRPresetVersion = 1

// ADRPreset returns the built-in Architecture Decision Record configuration.
func ADRPreset() Config {
	eq := func(v string) AttrCondition { return AttrCondition{Eq: &v} }
	not := func(v string) AttrCondition { return AttrCondition{Not: &v} }
	return Config{
		Preset:        PresetADR,
		PresetVersion: ADRPresetVersion,
		IDWidth:       DefaultIDWidth,
		StatusField:   DefaultStatusField,
		StatusValues: []string{
			StatusProposed,
			StatusAccepted,
			StatusRejected,
			StatusDeprecated,
			StatusSuperseded,
			StatusWithdrawn,
		},
		Edges: []EdgeSpec{
			{
				Name:      EdgeSupersedes.String(),
				Key:       EdgeSupersedes.String(),
				Acyclic:   true,
				Direction: DirectionForward,
			},
			{
				Name:      EdgeDependsOn.String(),
				Key:       EdgeDependsOn.String(),
				Acyclic:   true,
				Direction: DirectionForward,
			},
		},
		DerivedEdges: []DerivedEdgeSpec{
			{
				Field:     DefaultStatusField,
				Pattern:   DerivedSupersededPattern,
				Edge:      EdgeSupersedes.String(),
				Direction: DirectionReverse,
			},
		},
		Projections: []ProjectionSpec{
			{
				Name: ProjectionAcceptedUnsuperseded,
				When: Condition{
					NotInbound: EdgeSupersedes.String(),
					Attr:       map[string]AttrCondition{DefaultStatusField: eq(StatusAccepted)},
				},
			},
		},
		Binding: ProjectionAcceptedUnsuperseded,
		Rules: []Rule{
			{
				Name:     model.RuleStatusDrift,
				Severity: model.SeverityError,
				When: Condition{
					Inbound: EdgeCondition{Edge: EdgeSupersedes.String()},
					Attr:    map[string]AttrCondition{DefaultStatusField: not(StatusSuperseded)},
				},
				Message: "has inbound supersedes but status is not superseded",
			},
			{
				Name:     model.RuleSupersededOrphan,
				Severity: model.SeverityWarn,
				When: Condition{
					NotInbound: EdgeSupersedes.String(),
					Attr:       map[string]AttrCondition{DefaultStatusField: eq(StatusSuperseded)},
				},
				Message: "status is superseded but no document supersedes it",
			},
		},
	}
}

// Kinds of the spec preset. A normative standard is not one sort of document:
// the clause states the requirement, the conformance test decides whether a
// system meets it, the deviation records a departure from it, the measure
// records how a run scored it, and the premise, the principle and the
// post-mortem are what a clause rests on, argues from and failed as.
//
// The subject a clause is about is a document of its own — the topic kind —
// rather than a string attribute, so a misspelling is a dangling_ref instead of
// a second subject nobody notices.
const (
	KindClause    = "clause"
	KindConform   = "conform"
	KindDeviation = "deviation"
	KindMeasure   = "measure"
	KindPremise   = "premise"
	KindPrinciple = "principle"
	KindPM        = "pm"
	KindTopic     = "topic"
)

// Identifier patterns of the spec preset's kinds. Five of them carry a slash,
// which no file name can hold, so documents of those kinds write the
// identifier into their frontmatter; the other three are file-name shaped, and
// a document of one of those is named after its identifier.
const (
	IDClause    = `^UZ-[A-Z]-\d{3}$`
	IDConform   = `^conform/[a-z0-9-]+$`
	IDDeviation = `^dev-\d{4}$`
	IDMeasure   = `^interp/UZ-[A-Z]-\d{3}@\d{4}-\d{2}-\d{2}$`
	IDPremise   = `^premise/[a-z0-9/-]+$`
	IDPrinciple = `^principle/[a-z0-9/-]+$`
	IDPM        = `^pm-\d{4}$`
	IDTopic     = `^topic/[a-z0-9/-]+$`
)

// Edge types the spec preset adds to supersedes. The relation is declared on
// the side that generates it: a conformance test declares what it enforces and
// a measure declares what it measured, so a clause is not rewritten every time
// a machine reads it.
//
// The last three are declared by the clause itself, because all three are about
// what the clause says rather than about a machine having run: the subject it
// speaks to, the clause it makes an exception of, and the requirement that
// keeps its option from breaking interoperation.
const (
	EdgeEnforces       model.EdgeType = "enforces"
	EdgeDeviatesFrom   model.EdgeType = "deviates-from"
	EdgePremise        model.EdgeType = "premise"
	EdgeRationale      model.EdgeType = "rationale"
	EdgeCounterexample model.EdgeType = "counterexample"
	EdgeMeasures       model.EdgeType = "measures"
	EdgeAbout          model.EdgeType = "about"
	EdgeExcepts        model.EdgeType = "excepts"
	EdgeInterop        model.EdgeType = "interop"
)

// Edge attributes of the spec preset.
const (
	AttrReason    = "reason"
	AttrAgreement = "agreement"
	AttrModel     = "model"
	AttrScope     = "scope"
)

// The frontmatter keys the spec preset's periods are written under: a clause
// states the days it is in force between, a deviation the day its departure
// expires, and a premise the day the world stopped making it true.
//
// A clause names its own two keys rather than reading date, because the day a
// clause was written and the day it takes effect are different facts: a
// standard's release date is chosen, and a clause that has not been released
// yet is not in force. A deviation and a premise both begin the day they were
// recorded, which is what date already says.
const (
	FieldInForceFrom  = "in_force_from"
	FieldInForceUntil = "in_force_until"
	FieldExpires      = "expires"
	FieldRetiredOn    = "retired_on"
)

// SupersedesReasons is the closed vocabulary a supersedes entry states its
// reason from: the clause recurred, its premise collapsed, it conflicted with
// another clause, or the standard renamed the words it was written in.
var SupersedesReasons = []string{"recurrence", "premise-collapse", "conflict", "vocabulary"}

// FieldModality is the frontmatter key a clause states the strength of its
// requirement under, and the five values are BCP 14's keywords. Every mention
// of either — the kind's field declaration, the projections, the rules and the
// conflict check — goes through these names, so a revision of the vocabulary is
// one edit here rather than a search through the configuration.
//
// The vocabulary is five values rather than a strength and a polarity because
// BCP 14 has no MAY NOT: a pair of fields would have an invalid combination to
// check for, where a closed set of five has nothing to get wrong.
const (
	FieldModality     = "modality"
	ModalityMUST      = "MUST"
	ModalityMUSTNOT   = "MUST_NOT"
	ModalitySHOULD    = "SHOULD"
	ModalitySHOULDNOT = "SHOULD_NOT"
	ModalityMAY       = "MAY"
)

// Modalities is the closed vocabulary a clause states its modality from, in the
// order BCP 14 defines the keywords: the two strict ones, the two defeasible
// ones, and the explicit permission.
var Modalities = []string{ModalityMUST, ModalityMUSTNOT, ModalitySHOULD, ModalitySHOULDNOT, ModalityMAY}

// StrictModality reports whether a modality states a strict rule — one whose
// consequence follows without exception. Defeasible deontic logic's strict
// rules are the ones a defeater cannot overturn, which is why an excepts edge
// may not point at one and why a conflict between two of them stands however it
// is annotated.
func StrictModality(value string) bool {
	return value == ModalityMUST || value == ModalityMUSTNOT
}

// Prohibition reports whether a modality forbids rather than requires or
// permits. Two clauses about one topic conflict exactly when one of them is a
// prohibition and the other is not: that is the whole of ADR-0003's table, and
// the check reads it from here rather than from a copy of the grid.
func Prohibition(value string) bool {
	return value == ModalityMUSTNOT || value == ModalitySHOULDNOT
}

// FieldTest is the key a conformance document names the executable test body
// under. The body is a script rather than Markdown, so the document is a thin
// frontmatter wrapper that points at it.
const FieldTest = "test"

// Status values the spec preset adds to the ADR vocabulary. A premise is
// retired rather than superseded when the world stops making it true, a
// deviation is resolved when the departure it recorded is gone, and a
// post-mortem is written rather than decided, so it is drafted and published.
const (
	StatusTrial     = "trial"
	StatusRetired   = "retired"
	StatusResolved  = "resolved"
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Projections of the spec preset. A clause's force is derived rather than
// written down: modality: MUST is a claim, and it only carries the force of one
// where a conformance test enforces it, which is what effective_must says.
//
// effective is what binding names: a clause is effective at whatever strength
// the three projections leave it with, and a MAY is effective as the explicit
// permission it states. That is wider than effective_must, and deliberately so
// — a permission and a prohibition can only be seen to conflict if both are in
// the set, and ADR-0003 R2 puts MAY in `--binding` for exactly that reason.
//
// It is not called in_force: that name is the attribute the engine computes
// from a kind's `period:` declaration, and a projection shadows an attribute
// spelled the same way — so a projection of that name would mask the very
// attribute these projections read. The engine rejects one, and the
// alternatives below read `in_force: {eq: "true"}` instead.
//
// has_inforce_successor is the other half of that reading: a clause stops being
// effective when a successor actually takes over, which is a successor somebody
// accepted whose own period has begun. A successor that is proposed, in trial,
// or accepted but dated next quarter leaves its predecessor effective, which is
// the state the standard had no way to say before.
const (
	ProjectionEnforced         = "enforced"
	ProjectionInForceSuccessor = "has_inforce_successor"
	ProjectionEffectiveMust    = "effective_must"
	ProjectionEffectiveShould  = "effective_should"
	ProjectionEffective        = "effective"
)

// SpecPresetVersion is the revision the built-in spec configuration is at.
// Revision 2 is ADR-0005: the three kinds that have a lifetime declare the keys
// they write it under, `expires` moved from the deviates-from edge to the
// deviation that carries it, and the rules read the day.
const SpecPresetVersion = 2

// SpecPreset returns the built-in normative-clause configuration: eight kinds
// of document, the edges between them declared on the side that generates
// them, the projections that derive what is in force and at what strength, and
// the seven rules that report a standard hardening into dogma or drifting out
// of interoperability.
//
// It declares no top-level status_values, and each kind that answers to a
// vocabulary carries its own. That is deliberate: a kind inherits the
// top-level vocabulary wherever it declares none, so a top-level one would
// hand conform, measure and topic documents — which a machine writes, or which
// say nothing about their own standing — a vocabulary to fall outside of. With
// none declared anywhere they reach, their status is unchecked, which is what
// "this kind has no status" has to be written as.
func SpecPreset() Config {
	eq := func(v string) AttrCondition { return AttrCondition{Eq: &v} }
	not := func(v string) AttrCondition { return AttrCondition{Not: &v} }
	// The clause rules and projections all ask the same two questions, so the
	// pair is written once.
	stated := func(modality string) map[string]AttrCondition {
		return map[string]AttrCondition{
			FieldModality:      eq(modality),
			DefaultStatusField: eq(StatusAccepted),
		}
	}
	// A clause carries force where it says so and the day says so: the same two
	// questions plus the one the periods answer.
	inForce := func(modality string) map[string]AttrCondition {
		attrs := stated(modality)
		attrs[AttrInForce] = eq(ProjectionTrue)
		return attrs
	}
	accepted := func() map[string]AttrCondition {
		return map[string]AttrCondition{DefaultStatusField: eq(StatusAccepted)}
	}
	// Not replaced yet: written as the negation of the projection rather than
	// as not_inbound: supersedes, because a successor that has not taken over
	// is still a declared successor.
	unreplaced := func() *Condition {
		return &Condition{Attr: map[string]AttrCondition{ProjectionInForceSuccessor: eq(ProjectionTrue)}}
	}
	atLeast := func(n int) *int { return &n }
	return Config{
		Preset:        PresetSpec,
		PresetVersion: SpecPresetVersion,
		StatusField:   DefaultStatusField,
		Kinds: map[string]KindSpec{
			KindClause: {
				Dir: "spec/clauses",
				ID:  IDClause,
				StatusValues: []string{
					StatusProposed, StatusTrial, StatusAccepted, StatusSuperseded, StatusWithdrawn,
				},
				// A clause is the one document a person writes by hand and the
				// one every other kind points at, so its frontmatter is a closed
				// set: a key nobody declared is a mistake worth reporting rather
				// than another tool's field. Its modality is declared as a field
				// so that closed set admits it, and declared with a vocabulary
				// because a clause whose strength is a typo states nothing.
				Closed: true,
				Fields: map[string]FieldSpec{
					FieldModality: {OneOf: slices.Clone(Modalities), Required: true},
					// The two days the period is read from are declared as
					// fields as well, so `stats --fields` counts how much of
					// the standard has dated itself.
					FieldInForceFrom:  {},
					FieldInForceUntil: {},
				},
				// A clause is in force from the day the standard released it
				// until the day its successor takes over, and a clause that
				// names neither day is in force from the beginning with no end
				// in sight — which is what an undated standard says.
				Period: &PeriodSpec{From: FieldInForceFrom, Until: FieldInForceUntil},
			},
			KindConform: {
				Dir: "spec/conform",
				ID:  IDConform,
				// The test body is not Markdown, so the document names the path
				// to it. The kind is open — a harness writes these — but the key
				// is declared anyway, so stats --fields can count it.
				Fields: map[string]FieldSpec{FieldTest: {}},
			},
			KindDeviation: {
				Dir: "spec/deviations",
				ID:  IDDeviation,
				StatusValues: []string{
					StatusProposed, StatusAccepted, StatusResolved, StatusWithdrawn,
				},
				Closed: true,
				Fields: map[string]FieldSpec{FieldExpires: {}},
				// A departure runs from the day it was recorded to the day it
				// expires. The expiry is the deviation's own fact rather than
				// the edge's: a record with two departures would otherwise have
				// two lifetimes, and only one of them could be its own.
				Period: &PeriodSpec{From: KeyDate, Until: FieldExpires},
			},
			KindMeasure: {
				Dir: "spec/measures",
				ID:  IDMeasure,
			},
			KindPremise: {
				Dir: "spec/premises",
				ID:  IDPremise,
				// A premise holds until the day the world stopped making it
				// true, which is what stale_premise reads one hop away.
				//
				// `retired` stays in the vocabulary beside it: the day is what
				// the rule reads, and the word is what a person writing the
				// document reaches for. Dropping it would report every premise
				// already written as an unknown_status, and a status a person
				// writes is prose about the document rather than a fact the
				// engine derives anything from.
				StatusValues: []string{
					StatusProposed, StatusAccepted, StatusRetired, StatusSuperseded,
				},
				Fields: map[string]FieldSpec{FieldRetiredOn: {}},
				Period: &PeriodSpec{From: KeyDate, Until: FieldRetiredOn},
			},
			KindPrinciple: {
				Dir: "spec/principles",
				ID:  IDPrinciple,
				StatusValues: []string{
					StatusProposed, StatusAccepted, StatusSuperseded, StatusWithdrawn,
				},
			},
			KindPM: {
				Dir: "spec/pm",
				ID:  IDPM,
				// A post-mortem records what happened; it is written, not
				// decided, so it is never accepted and the rules that read
				// accepted never reach one.
				StatusValues: []string{StatusDraft, StatusPublished},
			},
			// A topic is a subject, not a decision: its body defines the subject
			// in a paragraph, and it answers to no status vocabulary because
			// there is nothing about a subject to decide.
			KindTopic: {
				Dir: "spec/topics",
				ID:  IDTopic,
			},
		},
		Edges: []EdgeSpec{
			{
				Name:      EdgeSupersedes.String(),
				Key:       EdgeSupersedes.String(),
				Acyclic:   true,
				Direction: DirectionForward,
				From:      []string{KindClause, KindPremise},
				To:        []string{KindClause, KindPremise},
				Attrs: map[string]EdgeAttrSpec{
					AttrReason: {Required: true, OneOf: slices.Clone(SupersedesReasons)},
				},
			},
			{
				Name:      EdgeEnforces.String(),
				Key:       EdgeEnforces.String(),
				Direction: DirectionForward,
				From:      []string{KindConform},
				To:        []string{KindClause},
				// A test that enforces a retired clause keeps passing while the
				// clause that replaced it goes unenforced: the suite is green and
				// the standard is dead. The target has to be the current leaf.
				Target: &TargetCondition{LeafOf: EdgeSupersedes.String()},
			},
			{
				Name:      EdgeDeviatesFrom.String(),
				Key:       EdgeDeviatesFrom.String(),
				Direction: DirectionForward,
				From:      []string{KindDeviation},
				To:        []string{KindClause},
				// The edge carries no attributes: the expiry it used to carry
				// belongs to the deviation, which has one lifetime however many
				// clauses it departs from.
				//
				// A departure is only a departure from something in force, so the
				// target is binding: accepted and superseded by nothing. That is
				// written out rather than deferred to the binding projection,
				// which reads effective_must and would make every deviation from
				// an unenforced clause a finding.
				Target: &TargetCondition{
					Condition: Condition{
						NotInbound: EdgeSupersedes.String(),
						Attr:       accepted(),
					},
				},
			},
			{
				Name:      EdgePremise.String(),
				Key:       EdgePremise.String(),
				Direction: DirectionForward,
				From:      []string{KindClause},
				To:        []string{KindPremise},
			},
			{
				Name:      EdgeRationale.String(),
				Key:       EdgeRationale.String(),
				Direction: DirectionForward,
				From:      []string{KindClause},
				To:        []string{KindPrinciple},
			},
			{
				Name:      EdgeCounterexample.String(),
				Key:       EdgeCounterexample.String(),
				Direction: DirectionForward,
				From:      []string{KindClause, KindPrinciple},
				To:        []string{KindPM},
			},
			{
				Name:      EdgeMeasures.String(),
				Key:       EdgeMeasures.String(),
				Direction: DirectionForward,
				From:      []string{KindMeasure},
				To:        []string{KindClause},
				Attrs: map[string]EdgeAttrSpec{
					AttrAgreement: {Required: true, Type: AttrTypeNumber},
					AttrModel:     {Required: true, Type: AttrTypeString},
				},
				// A measurement of a replaced clause reads as the current
				// agreement rate of a clause nobody is bound by.
				Target: &TargetCondition{LeafOf: EdgeSupersedes.String()},
			},
			{
				Name:      EdgeAbout.String(),
				Key:       EdgeAbout.String(),
				Direction: DirectionForward,
				From:      []string{KindClause},
				To:        []string{KindTopic},
				// A clause states its subject, always: two clauses can only be
				// seen to disagree where they are known to be about one thing,
				// and a clause with no subject is invisible to that comparison.
				MinOutbound: 1,
			},
			{
				Name:      EdgeExcepts.String(),
				Key:       EdgeExcepts.String(),
				Acyclic:   true,
				Direction: DirectionForward,
				From:      []string{KindClause},
				To:        []string{KindClause},
				// The direction is the exception — the more specific clause —
				// pointing at the general one it defeats. Acyclic because two
				// clauses each excepting the other defeat nothing.
				Attrs: map[string]EdgeAttrSpec{
					// DocDag records the scope and never evaluates it: it is
					// prose, and the people and agents reading `context` are
					// what it is written for.
					AttrScope: {Required: true, Type: AttrTypeString},
				},
			},
			{
				Name:      EdgeInterop.String(),
				Key:       EdgeInterop.String(),
				Direction: DirectionForward,
				From:      []string{KindClause},
				To:        []string{KindClause},
			},
		},
		Projections: []ProjectionSpec{
			{
				Name: ProjectionEnforced,
				When: Condition{Inbound: EdgeCondition{Edge: EdgeEnforces.String()}},
			},
			{
				// What "replaced" means once a clause has a lifetime: a
				// successor somebody accepted, whose own period has begun. It
				// is a projection of its own because four alternatives below
				// read it, and because the difference between "a successor
				// exists" and "a successor has taken over" is the whole of
				// what ADR-0005 adds to the standard.
				Name: ProjectionInForceSuccessor,
				When: Condition{ViaInbound: &ViaCondition{
					Edge: EdgeSupersedes.String(),
					Attr: map[string]AttrCondition{
						AttrInForce:        eq(ProjectionTrue),
						DefaultStatusField: eq(StatusAccepted),
					},
				}},
			},
			{
				// A MUST_NOT is a strict rule exactly as a MUST is, so it needs
				// the same conformance test behind it and falls the same way
				// without one. The two are alternatives of one projection
				// rather than two projections, because what reads this is
				// "which clauses carry strict force", never "which are
				// positive": the effective projection, the deviates-from
				// target and the listing all want the pair.
				Name: ProjectionEffectiveMust,
				AnyOf: []ProjectionAlt{
					{When: Condition{
						Attr:    inForce(ModalityMUST),
						Inbound: EdgeCondition{Edge: EdgeEnforces.String()},
						Not:     unreplaced(),
					}},
					{When: Condition{
						Attr:    inForce(ModalityMUSTNOT),
						Inbound: EdgeCondition{Edge: EdgeEnforces.String()},
						Not:     unreplaced(),
					}},
				},
			},
			{
				Name: ProjectionEffectiveShould,
				AnyOf: []ProjectionAlt{
					{When: Condition{
						Attr: inForce(ModalitySHOULD),
						Not:  unreplaced(),
					}},
					{When: Condition{
						Attr: inForce(ModalitySHOULDNOT),
						Not:  unreplaced(),
					}},
					// A MUST nothing enforces carries the force of a SHOULD,
					// which is the point of the projection, and an unenforced
					// MUST_NOT falls to a SHOULD_NOT the same way. A condition
					// holds one not: block, so the absent conformance test is
					// written as the not_inbound word and the missing
					// replacement as the projection it reads.
					{When: Condition{
						Attr:       inForce(ModalityMUST),
						NotInbound: EdgeEnforces.String(),
						Not:        unreplaced(),
					}},
					{When: Condition{
						Attr:       inForce(ModalityMUSTNOT),
						NotInbound: EdgeEnforces.String(),
						Not:        unreplaced(),
					}},
				},
			},
			{
				// What binding names: a clause effective at whatever strength
				// the projections above leave it with, plus the explicit
				// permission, which nothing enforces and nothing needs to. It
				// reads the other two as attributes rather than repeating
				// them, so a revision of what force means is one edit — and
				// the day is already in what they answer, which is why only
				// the permission spells it out here.
				Name: ProjectionEffective,
				AnyOf: []ProjectionAlt{
					{When: Condition{Attr: map[string]AttrCondition{ProjectionEffectiveMust: eq(ProjectionTrue)}}},
					{When: Condition{Attr: map[string]AttrCondition{ProjectionEffectiveShould: eq(ProjectionTrue)}}},
					{When: Condition{
						Attr: inForce(ModalityMAY),
						Not:  unreplaced(),
					}},
				},
			},
		},
		Binding: ProjectionEffective,
		Rules: []Rule{
			{
				Name:     model.RuleOrphanMust,
				Severity: model.SeverityError,
				When: Condition{
					AnyOf: []Condition{
						{Attr: stated(ModalityMUST)},
						{Attr: stated(ModalityMUSTNOT)},
					},
					NotInbound: EdgeEnforces.String(),
				},
				Message: "is MUST or MUST_NOT and accepted but nothing enforces it",
			},
			{
				Name:     model.RuleOrphanTest,
				Severity: model.SeverityError,
				When: Condition{
					Attr:        map[string]AttrCondition{KeyKind: eq(KindConform)},
					NotOutbound: EdgeEnforces.String(),
				},
				Message: "enforces no clause",
			},
			{
				// The premise the clause rests on has left force — its
				// retired_on has passed. The rule reads the day rather than
				// the word, so a premise retired next month stops holding the
				// clause up next month rather than the moment somebody typed
				// the word.
				Name:     model.RuleStalePremise,
				Severity: model.SeverityError,
				When: Condition{
					Attr: accepted(),
					Via: &ViaCondition{
						Edge: EdgePremise.String(),
						Attr: map[string]AttrCondition{AttrInForce: eq(ProjectionFalse)},
					},
				},
				Message: "is accepted but a premise is no longer in force",
			},
			{
				Name:     model.RuleDeviationPressure,
				Severity: model.SeverityWarn,
				When: Condition{
					Attr:    accepted(),
					Inbound: EdgeCondition{Edge: EdgeDeviatesFrom.String(), Min: atLeast(5)},
				},
				Message: "has 5+ deviations; reconsider the clause",
			},
			{
				Name:     model.RuleNoCounterexample,
				Severity: model.SeverityWarn,
				When: Condition{
					Attr: map[string]AttrCondition{
						KeyKind:            eq(KindClause),
						DefaultStatusField: eq(StatusAccepted),
					},
					NotOutbound: EdgeCounterexample.String(),
				},
				Message: "is accepted without a counterexample",
			},
			{
				// RFC 2119 §5: an implementation without the option must be
				// prepared to interoperate with one that has it, and the other
				// way round. That obligation is a MUST clause of its own, and
				// the interop edge is where the MAY names it. A warning rather
				// than an error, because interoperation is obvious for most
				// options and §6 warns against making the keywords a ritual.
				Name:     model.RuleMayWithoutInterop,
				Severity: model.SeverityWarn,
				When: Condition{
					Attr:        stated(ModalityMAY),
					NotOutbound: EdgeInterop.String(),
				},
				Message: "is MAY but names no MUST clause that guarantees interoperation without it",
			},
			{
				// The obligation the option carries is a MUST or it is not an
				// obligation. A via clause holds where some neighbour matches,
				// so a via over "not MUST" holds exactly where some interop
				// target is something else.
				Name:     model.RuleInteropNotMust,
				Severity: model.SeverityError,
				When: Condition{
					Outbound: EdgeCondition{Edge: EdgeInterop.String()},
					Via: &ViaCondition{
						Edge: EdgeInterop.String(),
						Attr: map[string]AttrCondition{FieldModality: not(ModalityMUST)},
					},
				},
				Message: "interop must point at a MUST clause",
			},
			{
				// The three that read the day. status_drift here is the
				// time-dependent reading of the rule the adr preset carries
				// under the same name: a successor that has not taken over
				// does not make its predecessor superseded, and saying so is
				// the whole point of declaring a period.
				Name:     model.RuleStatusDrift,
				Severity: model.SeverityError,
				When: Condition{
					Attr: map[string]AttrCondition{DefaultStatusField: not(StatusSuperseded)},
					ViaInbound: &ViaCondition{
						Edge: EdgeSupersedes.String(),
						Attr: map[string]AttrCondition{
							DefaultStatusField: eq(StatusAccepted),
							AttrInForce:        eq(ProjectionTrue),
						},
					},
				},
				Message: "an in-force successor supersedes it but status is not superseded",
			},
			{
				// The state the standard had no word for: the revision is
				// written and the clause it replaces is still what binds. A
				// warning rather than an error — nothing is wrong, and a
				// reader wants to know the change is in flight.
				Name:     model.RulePendingSuccessor,
				Severity: model.SeverityWarn,
				When: Condition{
					Attr:    accepted(),
					Inbound: EdgeCondition{Edge: EdgeSupersedes.String()},
					Not:     unreplaced(),
				},
				Message: "a successor is declared but not yet in force; this clause remains binding until then",
			},
			{
				// The mirror image, and an error: a clause marked superseded
				// while nothing has taken over leaves the standard with a gap
				// nobody is bound by.
				Name:     model.RulePrematureSuperseded,
				Severity: model.SeverityError,
				When: Condition{
					Attr: map[string]AttrCondition{DefaultStatusField: eq(StatusSuperseded)},
					Not:  unreplaced(),
				},
				Message: "status is superseded but no successor is in force yet",
			},
		},
	}
}

// Preset returns the built-in configuration registered under name.
func Preset(name string) (Config, error) {
	switch name {
	case "", PresetADR:
		return ADRPreset(), nil
	case PresetSpec:
		return SpecPreset(), nil
	default:
		return Config{}, fmt.Errorf("unknown preset %q: %w", name, model.ErrInvalidConfig)
	}
}
