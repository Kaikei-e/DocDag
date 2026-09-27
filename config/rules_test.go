package config

import (
	"slices"
	"testing"

	"github.com/Kaikei-e/DocDag/model"
)

func TestStructuralRules(t *testing.T) {
	rules := StructuralRules()
	if len(rules) != len(structuralSeverities) {
		t.Fatalf("StructuralRules() returned %d rules, want %d", len(rules), len(structuralSeverities))
	}
	if !slices.IsSorted(rules) {
		t.Errorf("StructuralRules() is not sorted: %v", rules)
	}
	for _, rule := range rules {
		if _, ok := structuralSeverities[rule]; !ok {
			t.Errorf("StructuralRules() returned unexpected rule %q", rule)
		}
	}
}

func TestPresetRules(t *testing.T) {
	t.Run("adr", func(t *testing.T) {
		rules := PresetRules(PresetADR)
		want := []string{model.RuleStatusDrift, model.RuleSupersededOrphan}
		slices.Sort(want)
		if !slices.Equal(rules, want) {
			t.Errorf("PresetRules(adr) = %v, want %v", rules, want)
		}
	})

	t.Run("spec", func(t *testing.T) {
		rules := PresetRules(PresetSpec)
		want := []string{
			model.RuleDeviationPressure,
			model.RuleInteropNotMust,
			model.RuleMayWithoutInterop,
			model.RuleNoCounterexample,
			model.RuleOrphanMust,
			model.RuleOrphanTest,
			model.RulePendingSuccessor,
			model.RulePrematureSuperseded,
			model.RuleStalePremise,
			model.RuleStatusDrift,
		}
		slices.Sort(want)
		if !slices.Equal(rules, want) {
			t.Errorf("PresetRules(spec) = %v, want %v", rules, want)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		rules := PresetRules("nonexistent")
		if len(rules) != 0 {
			t.Errorf("PresetRules(nonexistent) = %v, want empty", rules)
		}
	})
}

func TestPresets(t *testing.T) {
	presets := Presets()
	want := []string{PresetADR, PresetSpec}
	if !slices.Equal(presets, want) {
		t.Errorf("Presets() = %v, want %v", presets, want)
	}
}
