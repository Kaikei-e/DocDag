package config

import (
	"fmt"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// DerivedSupersededPattern matches the MADR status string "superseded by <ref>"
// (and the hyphenated spellings) and captures the referenced document.
const DerivedSupersededPattern = `(?i)^superseded[\s-]+by[\s-]+(\S+)`

// ADRPreset returns the built-in Architecture Decision Record configuration.
func ADRPreset() Config {
	eq := func(v string) AttrCondition { return AttrCondition{Eq: &v} }
	not := func(v string) AttrCondition { return AttrCondition{Not: &v} }
	return Config{
		Preset:      PresetADR,
		IDWidth:     DefaultIDWidth,
		StatusField: DefaultStatusField,
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
		Rules: []Rule{
			{
				Name:     model.RuleStatusDrift,
				Severity: model.SeverityError,
				When: Condition{
					Inbound: EdgeSupersedes.String(),
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
