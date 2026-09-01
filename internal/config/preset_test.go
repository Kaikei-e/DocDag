package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
)

func testDeref(t *testing.T, what string, p *string) string {
	t.Helper()
	if p == nil {
		t.Fatalf("%s is nil, want a value", what)
	}
	return *p
}

func TestADRPreset(t *testing.T) {
	cfg := ADRPreset()

	t.Run("identity and status defaults", func(t *testing.T) {
		if cfg.Preset != PresetADR {
			t.Errorf("preset = %q, want %q", cfg.Preset, PresetADR)
		}
		if cfg.IDWidth != 4 {
			t.Errorf("id_width = %d, want 4 (the MADR convention)", cfg.IDWidth)
		}
		if cfg.StatusField != "status" {
			t.Errorf("status_field = %q, want status", cfg.StatusField)
		}
		if cfg.Dir != "" {
			t.Errorf("dir = %q, want it left to discovery", cfg.Dir)
		}
		if cfg.Template != "" {
			t.Errorf("template = %q, want the built-in default", cfg.Template)
		}
	})

	t.Run("the preset names its revision", func(t *testing.T) {
		// The preset predates the versioning, so it starts at 1: an output
		// header that names a revision everywhere is worth more than one that
		// goes missing wherever the built-in preset is used.
		if cfg.PresetVersion != 1 {
			t.Errorf("preset_version = %d, want 1", cfg.PresetVersion)
		}
		if len(cfg.Fields) != 0 {
			t.Errorf("fields = %+v, want the preset to retire nothing", cfg.Fields)
		}
	})

	t.Run("the standard status values", func(t *testing.T) {
		want := []string{"proposed", "accepted", "rejected", "deprecated", "superseded", "withdrawn"}

		if !slices.Equal(cfg.StatusValues, want) {
			t.Fatalf("status_values = %v, want %v", cfg.StatusValues, want)
		}
	})

	t.Run("two acyclic edge types", func(t *testing.T) {
		want := []EdgeSpec{
			{Name: "supersedes", Key: "supersedes", Acyclic: true, Direction: DirectionForward},
			{Name: "depends-on", Key: "depends-on", Acyclic: true, Direction: DirectionForward},
		}

		if !reflect.DeepEqual(cfg.Edges, want) {
			t.Fatalf("edges = %+v, want %+v", cfg.Edges, want)
		}
	})

	t.Run("one derived edge for the MADR status string", func(t *testing.T) {
		if len(cfg.DerivedEdges) != 1 {
			t.Fatalf("derived_edges = %+v, want one", cfg.DerivedEdges)
		}
		spec := cfg.DerivedEdges[0]
		if spec.Field != cfg.StatusField {
			t.Errorf("field = %q, want %q", spec.Field, cfg.StatusField)
		}
		if spec.Edge != EdgeSupersedes.String() {
			t.Errorf("edge = %q, want %q", spec.Edge, EdgeSupersedes)
		}
		if spec.Direction != DirectionReverse {
			t.Errorf("direction = %q, want %q (the referenced document is the new one)", spec.Direction, DirectionReverse)
		}
		re, err := regexp.Compile(spec.Pattern)
		if err != nil {
			t.Fatalf("compile %q: %v", spec.Pattern, err)
		}
		for _, value := range []string{"superseded by 0003", "Superseded By 0003", "superseded-by 0003"} {
			m := re.FindStringSubmatch(value)
			if len(m) < 2 || m[1] != "0003" {
				t.Errorf("pattern on %q captured %v, want 0003", value, m)
			}
		}
	})

	t.Run("one projection, and it is what binding names", func(t *testing.T) {
		if len(cfg.Projections) != 1 {
			t.Fatalf("projections = %+v, want one", cfg.Projections)
		}
		spec := cfg.Projections[0]
		if spec.Name != ProjectionAcceptedUnsuperseded {
			t.Errorf("projection name = %q, want %q", spec.Name, ProjectionAcceptedUnsuperseded)
		}
		if cfg.Binding != ProjectionAcceptedUnsuperseded {
			t.Errorf("binding = %q, want %q", cfg.Binding, ProjectionAcceptedUnsuperseded)
		}
		if spec.When.NotInbound != EdgeSupersedes.String() {
			t.Errorf("projection not_inbound = %q, want %q", spec.When.NotInbound, EdgeSupersedes)
		}
		if got := testDeref(t, "projection attr eq", spec.When.Attr[cfg.StatusField].Eq); got != StatusAccepted {
			t.Errorf("projection attr eq = %q, want %q", got, StatusAccepted)
		}
		if len(spec.AnyOf) != 0 {
			t.Errorf("projection any_of = %+v, want none", spec.AnyOf)
		}
	})

	t.Run("two default rules", func(t *testing.T) {
		if len(cfg.Rules) != 2 {
			t.Fatalf("rules = %+v, want two", cfg.Rules)
		}
		drift, orphan := cfg.Rules[0], cfg.Rules[1]

		if drift.Name != model.RuleStatusDrift {
			t.Errorf("rule name = %q, want %q", drift.Name, model.RuleStatusDrift)
		}
		if drift.Severity != model.SeverityError {
			t.Errorf("%s severity = %q, want %q", drift.Name, drift.Severity, model.SeverityError)
		}
		if drift.When.Inbound.Edge != EdgeSupersedes.String() {
			t.Errorf("%s inbound = %q, want %q", drift.Name, drift.When.Inbound.Edge, EdgeSupersedes)
		}
		if got := testDeref(t, "status_drift attr not", drift.When.Attr[cfg.StatusField].Not); got != StatusSuperseded {
			t.Errorf("%s attr not = %q, want %q", drift.Name, got, StatusSuperseded)
		}
		if drift.When.Attr[cfg.StatusField].Eq != nil {
			t.Errorf("%s attr eq = %v, want none", drift.Name, drift.When.Attr[cfg.StatusField].Eq)
		}
		if drift.Message == "" {
			t.Errorf("%s message is empty, want an explanation", drift.Name)
		}

		if orphan.Name != model.RuleSupersededOrphan {
			t.Errorf("rule name = %q, want %q", orphan.Name, model.RuleSupersededOrphan)
		}
		if orphan.Severity != model.SeverityWarn {
			t.Errorf("%s severity = %q, want %q", orphan.Name, orphan.Severity, model.SeverityWarn)
		}
		if orphan.When.NotInbound != EdgeSupersedes.String() {
			t.Errorf("%s not_inbound = %q, want %q", orphan.Name, orphan.When.NotInbound, EdgeSupersedes)
		}
		if got := testDeref(t, "superseded_orphan attr eq", orphan.When.Attr[cfg.StatusField].Eq); got != StatusSuperseded {
			t.Errorf("%s attr eq = %q, want %q", orphan.Name, got, StatusSuperseded)
		}
		if orphan.When.Attr[cfg.StatusField].Not != nil {
			t.Errorf("%s attr not = %v, want none", orphan.Name, orphan.When.Attr[cfg.StatusField].Not)
		}
		if orphan.Message == "" {
			t.Errorf("%s message is empty, want an explanation", orphan.Name)
		}
	})

	t.Run("the preset is a valid configuration", func(t *testing.T) {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("each call returns an independent value", func(t *testing.T) {
		first := ADRPreset()
		first.IDWidth = 6
		first.Edges[0].Acyclic = false
		first.Rules = nil

		second := ADRPreset()
		if second.IDWidth != 4 || !second.Edges[0].Acyclic || len(second.Rules) != 2 {
			t.Fatalf("preset = %+v, want an untouched copy", second)
		}
	})
}

func TestPreset(t *testing.T) {
	t.Run("an empty name is the ADR preset", func(t *testing.T) {
		got, err := Preset("")
		if err != nil {
			t.Fatalf("Preset: %v", err)
		}
		if got.Preset != PresetADR {
			t.Fatalf("preset = %q, want %q", got.Preset, PresetADR)
		}
	})

	t.Run("the ADR preset by name", func(t *testing.T) {
		got, err := Preset(PresetADR)
		if err != nil {
			t.Fatalf("Preset: %v", err)
		}
		if len(got.Edges) != 2 {
			t.Fatalf("edges = %+v, want the ADR preset", got.Edges)
		}
	})

	t.Run("the spec preset by name", func(t *testing.T) {
		got, err := Preset(PresetSpec)
		if err != nil {
			t.Fatalf("Preset: %v", err)
		}
		if !reflect.DeepEqual(got, SpecPreset()) {
			t.Fatalf("preset = %+v, want the spec preset", got)
		}
	})

	t.Run("an unknown preset is a configuration error", func(t *testing.T) {
		_, err := Preset("mkdocs")

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("err = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})
}

func TestSpecPreset(t *testing.T) {
	cfg := SpecPreset()

	t.Run("the preset is a valid configuration", func(t *testing.T) {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if cfg.Preset != PresetSpec {
			t.Errorf("preset = %q, want %q", cfg.Preset, PresetSpec)
		}
		if cfg.PresetVersion != SpecPresetVersion {
			t.Errorf("preset_version = %d, want %d", cfg.PresetVersion, SpecPresetVersion)
		}
	})

	t.Run("seven kinds, each with a directory and an identifier pattern", func(t *testing.T) {
		want := map[string]string{
			KindClause:    IDClause,
			KindConform:   IDConform,
			KindDeviation: IDDeviation,
			KindMeasure:   IDMeasure,
			KindPremise:   IDPremise,
			KindPrinciple: IDPrinciple,
			KindPM:        IDPM,
		}
		if got := cfg.KindNames(); len(got) != len(want) {
			t.Fatalf("kinds = %v, want the seven of the standard", got)
		}
		for name, pattern := range want {
			spec, declared := cfg.Kind(name)
			switch {
			case !declared:
				t.Errorf("kind %q is not declared", name)
			case spec.ID != pattern:
				t.Errorf("kind %q id = %q, want %q", name, spec.ID, pattern)
			case spec.Dir == "":
				t.Errorf("kind %q has no dir", name)
			}
		}
	})

	t.Run("the machine-written kinds answer to no status vocabulary", func(t *testing.T) {
		// A kind inherits the top-level vocabulary wherever it declares none,
		// so the preset declares no top-level one: that is the only way a
		// conformance test or a measurement — which a machine writes, and
		// which say nothing about their own standing — reaches the empty
		// vocabulary the status check skips.
		if len(cfg.StatusValues) != 0 {
			t.Errorf("status_values = %v, want none at the top level", cfg.StatusValues)
		}
		for _, name := range []string{KindConform, KindMeasure} {
			if got := cfg.KindStatusValues(name); len(got) != 0 {
				t.Errorf("kind %q status_values = %v, want none", name, got)
			}
		}
		for _, name := range []string{KindClause, KindDeviation, KindPremise, KindPrinciple, KindPM} {
			if got := cfg.KindStatusValues(name); len(got) == 0 {
				t.Errorf("kind %q answers to no status vocabulary, want one of its own", name)
			}
		}
		if got := cfg.KindStatusValues(KindPremise); !slices.Contains(got, StatusRetired) {
			t.Errorf("premise status_values = %v, want %q among them, which stale_premise reads", got, StatusRetired)
		}
		if got := cfg.KindStatusValues(KindPM); slices.Contains(got, StatusAccepted) {
			t.Errorf("pm status_values = %v, want no %q: a post-mortem is written, not decided", got, StatusAccepted)
		}
	})

	t.Run("a closed clause knows every key the preset expects on one", func(t *testing.T) {
		known := cfg.KnownFrontmatterKeys(KindClause)
		if spec, _ := cfg.Kind(KindClause); !spec.Closed {
			t.Error("the clause kind is open, want it closed")
		}
		want := []string{
			KeyID, KeyKind, KeyTitle, KeyDate, cfg.EffectiveStatus(), FieldLevel,
			EdgeSupersedes.String(), EdgePremise.String(), EdgeRationale.String(), EdgeCounterexample.String(),
		}
		for _, key := range want {
			if !slices.Contains(known, key) {
				t.Errorf("known keys of a clause = %v, want %q among them", known, key)
			}
		}
	})

	t.Run("the conformance document declares the key that names its test body", func(t *testing.T) {
		if _, declared := cfg.Field(KindConform, FieldTest); !declared {
			t.Errorf("kind %q declares no %q field, want one so stats --fields can count it", KindConform, FieldTest)
		}
	})

	t.Run("seven edges, each declared on the side that generates it", func(t *testing.T) {
		want := []struct {
			edge     model.EdgeType
			from, to []string
			attrs    []string
		}{
			{EdgeSupersedes, []string{KindClause, KindPremise}, []string{KindClause, KindPremise}, []string{AttrReason}},
			{EdgeEnforces, []string{KindConform}, []string{KindClause}, nil},
			{EdgeDeviatesFrom, []string{KindDeviation}, []string{KindClause}, []string{AttrExpires}},
			{EdgePremise, []string{KindClause}, []string{KindPremise}, nil},
			{EdgeRationale, []string{KindClause}, []string{KindPrinciple}, nil},
			{EdgeCounterexample, []string{KindClause, KindPrinciple}, []string{KindPM}, nil},
			{EdgeMeasures, []string{KindMeasure}, []string{KindClause}, []string{AttrAgreement, AttrModel}},
		}
		if len(cfg.Edges) != len(want) {
			t.Fatalf("edges = %+v, want %d", cfg.Edges, len(want))
		}
		for i, tt := range want {
			spec := cfg.Edges[i]
			switch {
			case spec.Name != tt.edge.String():
				t.Errorf("edge %d = %q, want %q", i, spec.Name, tt.edge)
			case spec.Key != tt.edge.String():
				t.Errorf("edge %q key = %q, want the edge's own name", spec.Name, spec.Key)
			case spec.Direction != DirectionForward:
				t.Errorf("edge %q direction = %q, want %q", spec.Name, spec.Direction, DirectionForward)
			case !slices.Equal(spec.From, tt.from):
				t.Errorf("edge %q from = %v, want %v", spec.Name, spec.From, tt.from)
			case !slices.Equal(spec.To, tt.to):
				t.Errorf("edge %q to = %v, want %v", spec.Name, spec.To, tt.to)
			case !slices.Equal(spec.AttrNames(), tt.attrs):
				t.Errorf("edge %q attrs = %v, want %v", spec.Name, spec.AttrNames(), tt.attrs)
			}
			for _, name := range tt.attrs {
				if attr, _ := spec.Attr(name); !attr.Required {
					t.Errorf("edge %q attribute %q is optional, want it required", spec.Name, name)
				}
			}
		}
	})

	t.Run("force is a projection, and binding is what it names", func(t *testing.T) {
		if !slices.Equal(cfg.ProjectionNames(), []string{
			ProjectionEnforced, ProjectionEffectiveMust, ProjectionEffectiveShould,
		}) {
			t.Fatalf("projections = %v, want the three of the preset", cfg.ProjectionNames())
		}
		if cfg.Binding != ProjectionEffectiveMust {
			t.Errorf("binding = %q, want %q", cfg.Binding, ProjectionEffectiveMust)
		}
		must, _ := cfg.Projection(ProjectionEffectiveMust)
		if got := testDeref(t, "effective_must level", must.When.Attr[FieldLevel].Eq); got != LevelMUST {
			t.Errorf("effective_must level = %q, want %q", got, LevelMUST)
		}
		if must.When.Inbound.Edge != EdgeEnforces.String() {
			t.Errorf("effective_must inbound = %q, want %q: a MUST nothing enforces does not bind",
				must.When.Inbound.Edge, EdgeEnforces)
		}
		should, _ := cfg.Projection(ProjectionEffectiveShould)
		if len(should.AnyOf) != 2 {
			t.Fatalf("effective_should any_of = %+v, want the two alternatives", should.AnyOf)
		}
		// The second alternative needs two absences and a condition holds one
		// not_inbound, so the other is written as the not: {inbound: …} that
		// vocabulary word is sugar for.
		unenforced := should.AnyOf[1].When
		if unenforced.NotInbound != EdgeEnforces.String() || unenforced.Not == nil ||
			unenforced.Not.Inbound.Edge != EdgeSupersedes.String() {
			t.Errorf("effective_should second alternative = %+v, want an unenforced, unsuperseded MUST", unenforced)
		}
	})

	t.Run("five rules over the whole standard", func(t *testing.T) {
		want := []struct {
			name     string
			severity model.Severity
		}{
			{model.RuleOrphanMust, model.SeverityError},
			{model.RuleOrphanTest, model.SeverityError},
			{model.RuleStalePremise, model.SeverityError},
			{model.RuleDeviationPressure, model.SeverityWarn},
			{model.RuleNoCounterexample, model.SeverityWarn},
		}
		if len(cfg.Rules) != len(want) {
			t.Fatalf("rules = %+v, want %d", cfg.Rules, len(want))
		}
		for i, tt := range want {
			rule := cfg.Rules[i]
			switch {
			case rule.Name != tt.name:
				t.Errorf("rule %d = %q, want %q", i, rule.Name, tt.name)
			case rule.Severity != tt.severity:
				t.Errorf("rule %q severity = %q, want %q", rule.Name, rule.Severity, tt.severity)
			case rule.Message == "":
				t.Errorf("rule %q has no message, want an explanation", rule.Name)
			}
		}
		// The kind a document was read under is an attribute, which is what
		// lets a rule speak about one kind alone.
		test := cfg.Rules[1]
		if got := testDeref(t, "orphan_test kind", test.When.Attr[KeyKind].Eq); got != KindConform {
			t.Errorf("orphan_test kind = %q, want %q", got, KindConform)
		}
		pressure := cfg.Rules[3]
		if pressure.When.Inbound.Min == nil || *pressure.When.Inbound.Min != 5 {
			t.Errorf("deviation_pressure inbound = %+v, want a threshold of five", pressure.When.Inbound)
		}
	})

	t.Run("each call returns an independent value", func(t *testing.T) {
		first := SpecPreset()
		first.Binding = ""
		first.Edges[0].Attrs[AttrReason] = EdgeAttrSpec{}
		delete(first.Kinds, KindClause)

		second := SpecPreset()
		if second.Binding != ProjectionEffectiveMust || len(second.Kinds) != 7 {
			t.Fatalf("preset = %+v, want an untouched copy", second)
		}
		if attr, _ := second.Edges[0].Attr(AttrReason); !attr.Required {
			t.Fatalf("supersedes reason = %+v, want an untouched copy", attr)
		}
	})
}

// specPresetYAML is the `spec` preset as docs/configuration.md prints it. The
// documentation is a copy of a Go value, so it is pinned here rather than
// trusted: a preset the file describes differently is a preset nobody can
// adopt by reading about it.
const specPresetYAML = `preset: spec
preset_version: 1
status_field: status

kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    status_values: [proposed, trial, accepted, superseded, withdrawn]
    closed: true
    fields:
      level: {}                 # MUST | SHOULD | MAY, the strength the clause claims
  conform:
    dir: spec/conform
    id: '^conform/[a-z0-9-]+$'
    fields:
      test: {}                  # path to the executable test body
  deviation:
    dir: spec/deviations
    id: '^dev-\d{4}$'
    status_values: [proposed, accepted, resolved, withdrawn]
    closed: true
  measure:
    dir: spec/measures
    id: '^interp/UZ-[A-Z]-\d{3}@\d{4}-\d{2}-\d{2}$'
  premise:
    dir: spec/premises
    id: '^premise/[a-z0-9/-]+$'
    status_values: [proposed, accepted, retired, superseded]
  principle:
    dir: spec/principles
    id: '^principle/[a-z0-9/-]+$'
    status_values: [proposed, accepted, superseded, withdrawn]
  pm:
    dir: spec/pm
    id: '^pm-\d{4}$'
    status_values: [draft, published]

edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    from: [clause, premise]
    to: [clause, premise]
    attrs:
      reason: {required: true, one_of: [recurrence, premise-collapse, conflict, vocabulary]}
  - name: enforces
    key: enforces
    direction: forward
    from: [conform]
    to: [clause]
  - name: deviates-from
    key: deviates-from
    direction: forward
    from: [deviation]
    to: [clause]
    attrs:
      expires: {required: true, type: date}
  - name: premise
    key: premise
    direction: forward
    from: [clause]
    to: [premise]
  - name: rationale
    key: rationale
    direction: forward
    from: [clause]
    to: [principle]
  - name: counterexample
    key: counterexample
    direction: forward
    from: [clause, principle]
    to: [pm]
  - name: measures
    key: measures
    direction: forward
    from: [measure]
    to: [clause]
    attrs:
      agreement: {required: true, type: number}
      model: {required: true, type: string}

projections:
  - name: enforced
    when: {inbound: enforces}
  - name: effective_must
    when:
      attr: {level: {eq: MUST}, status: {eq: accepted}}
      inbound: enforces
      not_inbound: supersedes
  - name: effective_should
    any_of:
      - when:
          attr: {level: {eq: SHOULD}, status: {eq: accepted}}
          not_inbound: supersedes
      - when:
          attr: {level: {eq: MUST}, status: {eq: accepted}}
          not_inbound: enforces
          not: {inbound: supersedes}      # a condition holds one not_inbound; this is the other

binding: effective_must

rules:
  - name: orphan_must
    severity: error
    when:
      attr: {level: {eq: MUST}, status: {eq: accepted}}
      not_inbound: enforces
    message: "is MUST and accepted but nothing enforces it"
  - name: orphan_test
    severity: error
    when:
      attr: {kind: {eq: conform}}
      not_outbound: enforces
    message: "enforces no clause"
  - name: stale_premise
    severity: error
    when:
      attr: {status: {eq: accepted}}
      via: {edge: premise, attr: {status: {eq: retired}}}
    message: "is accepted but a premise is retired"
  - name: deviation_pressure
    severity: warn
    when:
      attr: {status: {eq: accepted}}
      inbound: {edge: deviates-from, min: 5}
    message: "has 5+ deviations; reconsider the clause"
  - name: no_counterexample
    severity: warn
    when:
      attr: {kind: {eq: clause}, status: {eq: accepted}}
      not_outbound: counterexample
    message: "is accepted without a counterexample"
`

func TestSpecPresetMatchesTheDocumentedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultConfigFile)
	if err := os.WriteFile(path, []byte(specPresetYAML), 0o600); err != nil {
		t.Fatalf("write configuration: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	base, err := Preset(file.Preset)
	if err != nil {
		t.Fatalf("Preset(%q): %v", file.Preset, err)
	}
	got := Merge(base, file)
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate the documented configuration: %v", err)
	}
	if !reflect.DeepEqual(got, SpecPreset()) {
		t.Errorf("the documented spec preset is not the built-in one:\ngot:  %+v\nwant: %+v", got, SpecPreset())
	}
}
