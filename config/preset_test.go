package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/model"
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

	t.Run("eight kinds, each with a directory and an identifier pattern", func(t *testing.T) {
		want := map[string]string{
			KindClause:    IDClause,
			KindConform:   IDConform,
			KindDeviation: IDDeviation,
			KindMeasure:   IDMeasure,
			KindPremise:   IDPremise,
			KindPrinciple: IDPrinciple,
			KindPM:        IDPM,
			KindTopic:     IDTopic,
		}
		if got := cfg.KindNames(); len(got) != len(want) {
			t.Fatalf("kinds = %v, want the eight of the standard", got)
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
		for _, name := range []string{KindConform, KindMeasure, KindTopic} {
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
			KeyID, KeyKind, KeyTitle, KeyDate, cfg.EffectiveStatus(), FieldModality,
			EdgeSupersedes.String(), EdgePremise.String(), EdgeRationale.String(), EdgeCounterexample.String(),
			EdgeAbout.String(), EdgeExcepts.String(), EdgeInterop.String(),
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

	t.Run("ten edges, each declared on the side that generates it", func(t *testing.T) {
		want := []struct {
			edge     model.EdgeType
			from, to []string
			attrs    []string
		}{
			{EdgeSupersedes, []string{KindClause, KindPremise}, []string{KindClause, KindPremise}, []string{AttrReason}},
			{EdgeEnforces, []string{KindConform}, []string{KindClause}, nil},
			{EdgeDeviatesFrom, []string{KindDeviation}, []string{KindClause}, nil},
			{EdgePremise, []string{KindClause}, []string{KindPremise}, nil},
			{EdgeRationale, []string{KindClause}, []string{KindPrinciple}, nil},
			{EdgeCounterexample, []string{KindClause, KindPrinciple}, []string{KindPM}, nil},
			{EdgeMeasures, []string{KindMeasure}, []string{KindClause}, []string{AttrAgreement, AttrModel}},
			{EdgeAbout, []string{KindClause}, []string{KindTopic}, nil},
			{EdgeExcepts, []string{KindClause}, []string{KindClause}, []string{AttrScope}},
			{EdgeInterop, []string{KindClause}, []string{KindClause}, nil},
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
			ProjectionEnforced, ProjectionInForceSuccessor,
			ProjectionEffectiveMust, ProjectionEffectiveShould, ProjectionEffective,
		}) {
			t.Fatalf("projections = %v, want the five of the preset", cfg.ProjectionNames())
		}
		if cfg.Binding != ProjectionEffective {
			t.Errorf("binding = %q, want %q", cfg.Binding, ProjectionEffective)
		}
		// A MUST_NOT is a strict rule too, so effective_must is the two of them
		// and each alternative wants the test that gives it its force.
		must, _ := cfg.Projection(ProjectionEffectiveMust)
		if len(must.AnyOf) != 2 {
			t.Fatalf("effective_must any_of = %+v, want the two strict modalities", must.AnyOf)
		}
		for i, want := range []string{ModalityMUST, ModalityMUSTNOT} {
			alt := must.AnyOf[i].When
			if got := testDeref(t, "effective_must modality", alt.Attr[FieldModality].Eq); got != want {
				t.Errorf("effective_must alternative %d modality = %q, want %q", i, got, want)
			}
			if alt.Inbound.Edge != EdgeEnforces.String() {
				t.Errorf("effective_must alternative %d inbound = %q, want %q: a strict clause nothing enforces does not carry strict force",
					i, alt.Inbound.Edge, EdgeEnforces)
			}
		}
		should, _ := cfg.Projection(ProjectionEffectiveShould)
		if len(should.AnyOf) != 4 {
			t.Fatalf("effective_should any_of = %+v, want the four alternatives", should.AnyOf)
		}
		// The unenforced-strict alternatives need two absences and a condition
		// holds one not: block, so the missing conformance test is the
		// not_inbound word and the missing replacement the projection.
		for i, want := range []string{ModalityMUST, ModalityMUSTNOT} {
			unenforced := should.AnyOf[2+i].When
			if got := testDeref(t, "effective_should modality", unenforced.Attr[FieldModality].Eq); got != want {
				t.Errorf("effective_should alternative %d modality = %q, want %q", 2+i, got, want)
			}
			if unenforced.NotInbound != EdgeEnforces.String() || unenforced.Not == nil ||
				testDeref(t, "effective_should successor", unenforced.Not.Attr[ProjectionInForceSuccessor].Eq) != ProjectionTrue {
				t.Errorf("effective_should alternative %d = %+v, want an unenforced strict clause nothing has replaced", 2+i, unenforced)
			}
		}
		// effective reads the other two rather than repeating them, and adds
		// the explicit permission, which nothing enforces and nothing needs to.
		force, _ := cfg.Projection(ProjectionEffective)
		if len(force.AnyOf) != 3 {
			t.Fatalf("effective any_of = %+v, want the three alternatives", force.AnyOf)
		}
		if got := testDeref(t, "effective may", force.AnyOf[2].When.Attr[FieldModality].Eq); got != ModalityMAY {
			t.Errorf("effective third alternative = %q, want %q: a MAY is effective as the permission it states", got, ModalityMAY)
		}
		if !slices.Contains(force.AttrKeys(), ProjectionEffectiveMust) {
			t.Errorf("effective reads %v, want %q among them", force.AttrKeys(), ProjectionEffectiveMust)
		}
	})

	t.Run("the clause modality is a closed vocabulary a clause has to state", func(t *testing.T) {
		spec, declared := cfg.Field(KindClause, FieldModality)
		switch {
		case !declared:
			t.Fatalf("kind %q declares no %q field", KindClause, FieldModality)
		case !spec.Required:
			t.Errorf("%s is optional, want it required: a clause without one states no strength", FieldModality)
		case !slices.Equal(spec.OneOf, Modalities):
			t.Errorf("%s one_of = %v, want %v", FieldModality, spec.OneOf, Modalities)
		}
		for _, value := range Modalities {
			if !spec.Accepts(value) {
				t.Errorf("%s rejects %q, want the whole vocabulary", FieldModality, value)
			}
		}
		if spec.Accepts("SHALL") {
			t.Errorf("%s accepts SHALL, want the five of BCP 14 alone", FieldModality)
		}
	})

	t.Run("a clause states its subject, and the subject is a document", func(t *testing.T) {
		about, declared := cfg.Edge(EdgeAbout)
		switch {
		case !declared:
			t.Fatalf("no %s edge, want one so two clauses can be known to speak about one thing", EdgeAbout)
		case about.MinOutbound != 1:
			t.Errorf("%s min_outbound = %d, want 1: a clause with no subject is invisible to the conflict check",
				EdgeAbout, about.MinOutbound)
		}
		excepts, declared := cfg.Edge(EdgeExcepts)
		switch {
		case !declared:
			t.Fatalf("no %s edge, want one so an exception can be recorded", EdgeExcepts)
		case !excepts.Acyclic:
			t.Errorf("%s is cyclic, want it acyclic: two clauses excepting each other defeat nothing", EdgeExcepts)
		}
		if scope, ok := excepts.Attr(AttrScope); !ok || !scope.Required {
			t.Errorf("%s scope = %+v, want it required: an exception without one says nothing", EdgeExcepts, scope)
		}
	})

	t.Run("every effective alternative reads the day", func(t *testing.T) {
		// ADR-0005 D2: what carries force is what the standard says and what
		// the calendar says, so every alternative that pins a modality also
		// pins in_force, and the two that read another projection inherit it.
		for _, name := range []string{ProjectionEffectiveMust, ProjectionEffectiveShould, ProjectionEffective} {
			spec, _ := cfg.Projection(name)
			for i, when := range spec.Whens() {
				if _, reads := when.Attr[FieldModality]; !reads {
					continue
				}
				if got := testDeref(t, name+" in_force", when.Attr[AttrInForce].Eq); got != ProjectionTrue {
					t.Errorf("%s alternative %d in_force = %q, want %q", name, i, got, ProjectionTrue)
				}
				if when.NotInbound == EdgeSupersedes.String() {
					t.Errorf("%s alternative %d still reads not_inbound: %s, want the in-force successor projection",
						name, i, EdgeSupersedes)
				}
			}
		}
		successor, declared := cfg.Projection(ProjectionInForceSuccessor)
		if !declared {
			t.Fatalf("no %q projection, want the one the alternatives read", ProjectionInForceSuccessor)
		}
		via := successor.When.ViaInbound
		if via == nil || via.Edge != EdgeSupersedes.String() {
			t.Fatalf("%s via_inbound = %+v, want a hop back along %s", ProjectionInForceSuccessor, via, EdgeSupersedes)
		}
		if got := testDeref(t, "successor in_force", via.Attr[AttrInForce].Eq); got != ProjectionTrue {
			t.Errorf("%s in_force = %q, want %q", ProjectionInForceSuccessor, got, ProjectionTrue)
		}
		if got := testDeref(t, "successor status", via.Attr[DefaultStatusField].Eq); got != StatusAccepted {
			t.Errorf("%s status = %q, want %q", ProjectionInForceSuccessor, got, StatusAccepted)
		}
	})

	t.Run("the three kinds with a lifetime declare where they write it", func(t *testing.T) {
		want := map[string]PeriodSpec{
			KindClause:    {From: FieldInForceFrom, Until: FieldInForceUntil},
			KindDeviation: {From: KeyDate, Until: FieldExpires},
			KindPremise:   {From: KeyDate, Until: FieldRetiredOn},
		}
		for name, period := range want {
			got, declared := cfg.KindPeriod(name)
			switch {
			case !declared:
				t.Errorf("kind %q declares no period, want %+v", name, period)
			case got != period:
				t.Errorf("kind %q period = %+v, want %+v", name, got, period)
			}
			// A closed kind has to admit the keys its own period reads, and
			// every kind has to declare them under fields: so stats --fields
			// can report how much of the corpus has dated itself.
			for _, key := range period.Fields() {
				if !slices.Contains(cfg.KnownFrontmatterKeys(name), key) {
					t.Errorf("kind %q known keys = %v, want %q among them", name, cfg.KnownFrontmatterKeys(name), key)
				}
				if _, ok := cfg.Field(name, key); !ok && key != KeyDate {
					t.Errorf("kind %q declares no %q field, want one so stats --fields can count it", name, key)
				}
			}
		}
		for _, name := range []string{KindConform, KindMeasure, KindPrinciple, KindPM, KindTopic} {
			if _, declared := cfg.KindPeriod(name); declared {
				t.Errorf("kind %q declares a period, want none: it has no lifetime of its own", name)
			}
		}
		if !cfg.Periods() {
			t.Error("the preset declares no periods at all, want the three kinds that have one")
		}
	})

	t.Run("the deviation carries its own expiry, not the edge", func(t *testing.T) {
		spec, _ := cfg.Edge(EdgeDeviatesFrom)
		if len(spec.Attrs) != 0 {
			t.Errorf("%s attrs = %+v, want none: the expiry belongs to the deviation", EdgeDeviatesFrom, spec.Attrs)
		}
		if _, declared := cfg.Field(KindDeviation, FieldExpires); !declared {
			t.Errorf("kind %q declares no %q field, want the one the period reads", KindDeviation, FieldExpires)
		}
	})

	t.Run("both targets say the current leaf the way that reads the day", func(t *testing.T) {
		// ADR-0005 gave a clause a lifetime, and not_inbound cannot see one: it
		// counts a successor nobody has accepted and one whose period has not
		// begun, so a departure from a clause a trial revision names would be
		// reported stale on the same run pending_successor calls it binding.
		// leaf_of reads the day, so both edges ask the same question the
		// standard asks.
		for _, edge := range []model.EdgeType{EdgeEnforces, EdgeDeviatesFrom} {
			spec, _ := cfg.Edge(edge)
			target := spec.Target
			if target == nil {
				t.Errorf("%s declares no target, want the current leaf of the lineage", edge)
				continue
			}
			if target.LeafOf != EdgeSupersedes.String() {
				t.Errorf("%s target leaf_of = %q, want %q", edge, target.LeafOf, EdgeSupersedes)
			}
			if target.NotInbound != "" {
				t.Errorf("%s target not_inbound = %q, want none: it cannot read a period", edge, target.NotInbound)
			}
		}
		// The departure asks one thing more than the conformance test does: a
		// clause nobody accepted is not something to depart from either.
		deviates, _ := cfg.Edge(EdgeDeviatesFrom)
		if got := testDeref(t, "deviates-from target status", deviates.Target.Attr[DefaultStatusField].Eq); got != StatusAccepted {
			t.Errorf("%s target status = %q, want %q", EdgeDeviatesFrom, got, StatusAccepted)
		}
	})

	t.Run("ten rules over the whole standard", func(t *testing.T) {
		want := []struct {
			name     string
			severity model.Severity
		}{
			{model.RuleOrphanMust, model.SeverityError},
			{model.RuleOrphanTest, model.SeverityError},
			{model.RuleStalePremise, model.SeverityError},
			{model.RuleDeviationPressure, model.SeverityWarn},
			{model.RuleNoCounterexample, model.SeverityWarn},
			{model.RuleMayWithoutInterop, model.SeverityWarn},
			{model.RuleInteropNotMust, model.SeverityError},
			{model.RuleStatusDrift, model.SeverityError},
			{model.RulePendingSuccessor, model.SeverityWarn},
			{model.RulePrematureSuperseded, model.SeverityError},
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
		// A via clause holds where some neighbour matches, so "some interop
		// target is not a MUST" is the whole of the rule.
		interop := cfg.Rules[6]
		if interop.When.Via == nil || interop.When.Via.Edge != EdgeInterop.String() {
			t.Fatalf("interop_not_must via = %+v, want a hop across %s", interop.When.Via, EdgeInterop)
		}
		if got := testDeref(t, "interop_not_must modality", interop.When.Via.Attr[FieldModality].Not); got != ModalityMUST {
			t.Errorf("interop_not_must modality = %q, want not %q", got, ModalityMUST)
		}
		// The premise the clause rests on has left force, which is the day
		// rather than the word: `retired` stays in the vocabulary for the
		// person writing the document, and the rule reads in_force.
		premise := cfg.Rules[2]
		if premise.When.Via == nil || premise.When.Via.Edge != EdgePremise.String() {
			t.Fatalf("stale_premise via = %+v, want a hop across %s", premise.When.Via, EdgePremise)
		}
		if got := testDeref(t, "stale_premise in_force", premise.When.Via.Attr[AttrInForce].Eq); got != ProjectionFalse {
			t.Errorf("stale_premise in_force = %q, want %q", got, ProjectionFalse)
		}
		if !slices.Contains(cfg.KindStatusValues(KindPremise), StatusRetired) {
			t.Errorf("premise status_values = %v, want %q kept for the person who writes it",
				cfg.KindStatusValues(KindPremise), StatusRetired)
		}
		// The three time-dependent status rules of ADR-0005 D4.
		drift := cfg.Rules[7]
		if drift.When.ViaInbound == nil ||
			testDeref(t, "status_drift in_force", drift.When.ViaInbound.Attr[AttrInForce].Eq) != ProjectionTrue {
			t.Errorf("status_drift = %+v, want an in-force accepted successor one hop back", drift.When)
		}
		for _, i := range []int{8, 9} {
			rule := cfg.Rules[i]
			if rule.When.Not == nil ||
				testDeref(t, rule.Name+" successor", rule.When.Not.Attr[ProjectionInForceSuccessor].Eq) != ProjectionTrue {
				t.Errorf("rule %q = %+v, want it to read the absence of an in-force successor", rule.Name, rule.When)
			}
		}
	})

	t.Run("each call returns an independent value", func(t *testing.T) {
		first := SpecPreset()
		first.Binding = ""
		first.Edges[0].Attrs[AttrReason] = EdgeAttrSpec{}
		delete(first.Kinds, KindClause)

		second := SpecPreset()
		if second.Binding != ProjectionEffective || len(second.Kinds) != 8 {
			t.Fatalf("preset = %+v, want an untouched copy", second)
		}
		if attr, _ := second.Edges[0].Attr(AttrReason); !attr.Required {
			t.Fatalf("supersedes reason = %+v, want an untouched copy", attr)
		}
	})
}

// specPresetMarker is the sentence docs/configuration.md introduces the whole
// preset with. The block that follows it is the one the pin below reads:
// naming the prose rather than a line number keeps the pin pointing at the
// right block while the file around it is edited.
const specPresetMarker = "The file below is that preset in full:"

// documentedSpecPreset returns the fenced YAML the documentation prints the
// `spec` preset as, read out of the file itself. Reading the file is the whole
// point: a copy kept beside the test drifts from the documentation silently,
// and it is the documentation a person adopts the preset by reading.
func documentedSpecPreset(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "docs", "configuration.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	_, after, found := strings.Cut(string(content), specPresetMarker)
	if !found {
		t.Fatalf("%s no longer says %q, so the documented preset cannot be located", path, specPresetMarker)
	}
	_, block, found := strings.Cut(after, "```yaml\n")
	if !found {
		t.Fatalf("%s: no YAML block follows %q", path, specPresetMarker)
	}
	block, _, found = strings.Cut(block, "\n```")
	if !found {
		t.Fatalf("%s: the YAML block following %q is never closed", path, specPresetMarker)
	}
	return block + "\n"
}

// TestSpecPresetMatchesTheDocumentedYAML holds docs/configuration.md to the
// built-in preset. The preset is the source of truth; a file that describes it
// differently is a preset nobody can adopt by reading about it.
//
// The documented configuration is merged over an **empty** Config rather than
// over SpecPreset(). Merging it over the preset it describes would hide every
// omission — a file holding nothing but `preset: spec` would pass — and an
// omission is exactly what this pin exists to catch. Merging it over nothing
// means the documented block has to carry the whole preset itself.
func TestSpecPresetMatchesTheDocumentedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultConfigFile)
	if err := os.WriteFile(path, []byte(documentedSpecPreset(t)), 0o600); err != nil {
		t.Fatalf("write configuration: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := Merge(Config{}, file)
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate the documented configuration: %v", err)
	}
	want := SpecPreset()
	// Retargeting a status field an empty base never declared leaves an empty
	// derived_edges list where the preset holds nil. Both say "this
	// configuration derives no edges", and every reader of the field ranges
	// over it, so the comparison is made blind to the difference rather than
	// the preset bent to fit it.
	if len(got.DerivedEdges) == 0 && len(want.DerivedEdges) == 0 {
		got.DerivedEdges, want.DerivedEdges = nil, nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the documented spec preset is not the built-in one:\ngot:  %+v\nwant: %+v", got, want)
	}
}
