package config

import (
	"errors"
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

		if !slices.Equal(cfg.Edges, want) {
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

	t.Run("an unknown preset is a configuration error", func(t *testing.T) {
		_, err := Preset("mkdocs")

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("err = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})
}
