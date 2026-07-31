package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// Options are the inputs a CLI invocation contributes to configuration
// resolution. Flags win over the config file, which wins over the preset.
type Options struct {
	Root       string
	Dir        string
	ConfigPath string
}

// DiscoveryPaths lists the well-known documents directories, in priority order.
func DiscoveryPaths() []string {
	return []string{
		"docs/adr",
		"doc/adr",
		"docs/decisions",
		"docs/ADR",
		"adr",
	}
}

// Discover returns the first well-known documents directory below root that
// exists and holds at least one document matching the preset filename pattern.
func Discover(root string, norm IDNormalizer) (string, error) {
	for _, candidate := range DiscoveryPaths() {
		dir := filepath.Join(root, filepath.FromSlash(candidate))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !norm.MatchesFilename(entry.Name()) {
				continue
			}
			return dir, nil
		}
	}
	return "", fmt.Errorf("no well-known documents directory under %s: %w", root, model.ErrNoDocuments)
}

// Load reads a docdag.yaml file into a partial configuration.
func Load(path string) (Config, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read configuration %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(src, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration %s: %w", path, err)
	}
	return cfg, nil
}

// Merge overlays a partial configuration onto a base, returning the effective
// configuration. Set fields and non-empty lists replace their base counterpart.
func Merge(base, override Config) Config {
	merged := base
	if override.Preset != "" {
		merged.Preset = override.Preset
	}
	if override.Dir != "" {
		merged.Dir = override.Dir
	}
	if override.IDWidth != 0 {
		merged.IDWidth = override.IDWidth
	}
	if override.StatusField != "" {
		merged.StatusField = override.StatusField
	}
	if override.Template != "" {
		merged.Template = override.Template
	}
	if len(override.StatusValues) > 0 {
		merged.StatusValues = slices.Clone(override.StatusValues)
	}
	if len(override.Edges) > 0 {
		merged.Edges = slices.Clone(override.Edges)
	}
	if len(override.DerivedEdges) > 0 {
		merged.DerivedEdges = slices.Clone(override.DerivedEdges)
	}
	if len(override.Rules) > 0 {
		merged.Rules = slices.Clone(override.Rules)
	}
	return merged
}

// Resolve produces the effective configuration for one CLI invocation,
// including documents-directory discovery when nothing selected one.
func Resolve(opts Options) (Config, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	file, err := loadOptional(root, opts.ConfigPath)
	if err != nil {
		return Config{}, err
	}
	base, err := Preset(file.Preset)
	if err != nil {
		return Config{}, err
	}
	cfg := Merge(base, file)
	if opts.Dir != "" {
		cfg.Dir = opts.Dir
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.Dir == "" {
		dir, err := Discover(root, cfg.Normalizer())
		if err != nil {
			return Config{}, err
		}
		cfg.Dir = dir
	}
	return cfg, nil
}

// loadOptional reads the named configuration file, or the one at the root when
// none was named. A root without a configuration file is not an error.
func loadOptional(root, path string) (Config, error) {
	if path == "" {
		candidate := filepath.Join(root, DefaultConfigFile)
		if _, err := os.Stat(candidate); err != nil {
			return Config{}, nil
		}
		path = candidate
	}
	return Load(path)
}
