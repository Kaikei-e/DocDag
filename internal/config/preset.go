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
const (
	KindClause    = "clause"
	KindConform   = "conform"
	KindDeviation = "deviation"
	KindMeasure   = "measure"
	KindPremise   = "premise"
	KindPrinciple = "principle"
	KindPM        = "pm"
)

// Identifier patterns of the spec preset's kinds. Four of them carry a slash,
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
)

// Edge types the spec preset adds to supersedes. The relation is declared on
// the side that generates it: a conformance test declares what it enforces and
// a measure declares what it measured, so a clause is not rewritten every time
// a machine reads it.
const (
	EdgeEnforces       model.EdgeType = "enforces"
	EdgeDeviatesFrom   model.EdgeType = "deviates-from"
	EdgePremise        model.EdgeType = "premise"
	EdgeRationale      model.EdgeType = "rationale"
	EdgeCounterexample model.EdgeType = "counterexample"
	EdgeMeasures       model.EdgeType = "measures"
)

// Edge attributes of the spec preset.
const (
	AttrReason    = "reason"
	AttrExpires   = "expires"
	AttrAgreement = "agreement"
	AttrModel     = "model"
)

// SupersedesReasons is the closed vocabulary a supersedes entry states its
// reason from: the clause recurred, its premise collapsed, it conflicted with
// another clause, or the standard renamed the words it was written in.
var SupersedesReasons = []string{"recurrence", "premise-collapse", "conflict", "vocabulary"}

// FieldLevel is the frontmatter key a clause states the strength of its
// requirement under, and LevelMUST and LevelSHOULD are the two values the
// preset's projections and rules read. ADR-0003 renames the key to modality
// and widens the vocabulary to five values, so every mention of either — in
// the kind's field declaration, in the projections and in the rules — goes
// through these names: that revision is one edit here rather than a search
// through the configuration.
const (
	FieldLevel  = "level"
	LevelMUST   = "MUST"
	LevelSHOULD = "SHOULD"
)

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
// written down: level: MUST is a claim, and it only binds where a conformance
// test enforces it, which is what effective_must says and binding answers with.
const (
	ProjectionEnforced        = "enforced"
	ProjectionEffectiveMust   = "effective_must"
	ProjectionEffectiveShould = "effective_should"
)

// SpecPresetVersion is the revision the built-in spec configuration is at.
const SpecPresetVersion = 1

// SpecPreset returns the built-in normative-clause configuration: seven kinds
// of document, the edges between them declared on the machine-generated side,
// the projections that derive what is in force, and the five rules that report
// a standard hardening into dogma.
//
// It declares no top-level status_values, and each kind that answers to a
// vocabulary carries its own. That is deliberate: a kind inherits the
// top-level vocabulary wherever it declares none, so a top-level one would
// hand conform and measure documents — which a machine writes, and which say
// nothing about their own standing — a vocabulary to fall outside of. With
// none declared anywhere they reach, their status is unchecked, which is what
// "this kind has no status" has to be written as.
func SpecPreset() Config {
	eq := func(v string) AttrCondition { return AttrCondition{Eq: &v} }
	// The clause rules and projections all ask the same two questions, so the
	// pair is written once.
	stated := func(level string) map[string]AttrCondition {
		return map[string]AttrCondition{
			FieldLevel:         eq(level),
			DefaultStatusField: eq(StatusAccepted),
		}
	}
	accepted := func() map[string]AttrCondition {
		return map[string]AttrCondition{DefaultStatusField: eq(StatusAccepted)}
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
				// than another tool's field. Its level is declared as a field so
				// that closed set admits it.
				Closed: true,
				Fields: map[string]FieldSpec{FieldLevel: {}},
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
			},
			KindMeasure: {
				Dir: "spec/measures",
				ID:  IDMeasure,
			},
			KindPremise: {
				Dir: "spec/premises",
				ID:  IDPremise,
				// A premise is retired when the world stops making it true,
				// which is what stale_premise reads one hop away.
				StatusValues: []string{
					StatusProposed, StatusAccepted, StatusRetired, StatusSuperseded,
				},
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
				Attrs: map[string]EdgeAttrSpec{
					AttrExpires: {Required: true, Type: AttrTypeDate},
				},
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
		},
		Projections: []ProjectionSpec{
			{
				Name: ProjectionEnforced,
				When: Condition{Inbound: EdgeCondition{Edge: EdgeEnforces.String()}},
			},
			{
				Name: ProjectionEffectiveMust,
				When: Condition{
					Attr:       stated(LevelMUST),
					Inbound:    EdgeCondition{Edge: EdgeEnforces.String()},
					NotInbound: EdgeSupersedes.String(),
				},
			},
			{
				Name: ProjectionEffectiveShould,
				AnyOf: []ProjectionAlt{
					{When: Condition{
						Attr:       stated(LevelSHOULD),
						NotInbound: EdgeSupersedes.String(),
					}},
					// A MUST nothing enforces carries the force of a SHOULD,
					// which is the point of the projection. That alternative
					// needs two absences and a condition holds one not_inbound,
					// so the second is written as the not: {inbound: …} the
					// vocabulary word is sugar for.
					{When: Condition{
						Attr:       stated(LevelMUST),
						NotInbound: EdgeEnforces.String(),
						Not:        &Condition{Inbound: EdgeCondition{Edge: EdgeSupersedes.String()}},
					}},
				},
			},
		},
		Binding: ProjectionEffectiveMust,
		Rules: []Rule{
			{
				Name:     model.RuleOrphanMust,
				Severity: model.SeverityError,
				When: Condition{
					Attr:       stated(LevelMUST),
					NotInbound: EdgeEnforces.String(),
				},
				Message: "is MUST and accepted but nothing enforces it",
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
				Name:     model.RuleStalePremise,
				Severity: model.SeverityError,
				When: Condition{
					Attr: accepted(),
					Via: &ViaCondition{
						Edge: EdgePremise.String(),
						Attr: map[string]AttrCondition{DefaultStatusField: eq(StatusRetired)},
					},
				},
				Message: "is accepted but a premise is retired",
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
