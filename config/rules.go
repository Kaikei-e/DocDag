package config

import (
	"maps"
	"slices"
)

// StructuralRules returns the sorted rule names of every built-in structural check.
func StructuralRules() []string {
	return slices.Sorted(maps.Keys(structuralSeverities))
}

// PresetRules returns the sorted rule names declared by the named preset.
func PresetRules(preset string) []string {
	cfg, err := Preset(preset)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		if r.Name != "" && !slices.Contains(names, r.Name) {
			names = append(names, r.Name)
		}
	}
	slices.Sort(names)
	return names
}

// Presets returns the names of all built-in presets.
func Presets() []string {
	return []string{PresetADR, PresetSpec}
}
