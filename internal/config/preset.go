package config

import (
	"fmt"

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

// Preset returns the built-in configuration registered under name.
func Preset(name string) (Config, error) {
	switch name {
	case "", PresetADR:
		return ADRPreset(), nil
	default:
		return Config{}, fmt.Errorf("unknown preset %q: %w", name, model.ErrInvalidConfig)
	}
}
