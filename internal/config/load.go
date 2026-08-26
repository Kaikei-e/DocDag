package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
		info, err := os.Stat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		// A candidate that exists but cannot be read is not the same as an
		// absent one: falling through would validate a directory nobody named.
		if err != nil {
			return "", fmt.Errorf("read documents directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("documents candidate %s is not a directory", dir)
		}
		exact, err := matchesOnDiskCase(root, candidate)
		if err != nil {
			return "", fmt.Errorf("read documents directory %s: %w", dir, err)
		}
		if !exact {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", fmt.Errorf("read documents directory %s: %w", dir, err)
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

// matchesOnDiskCase reports whether every component of the slash-separated
// candidate exists under root spelled exactly this way. A plain stat cannot
// tell: on a case-insensitive filesystem it also answers for docs/ADR when
// asked about docs/adr.
func matchesOnDiskCase(root, candidate string) (bool, error) {
	parent := root
	for component := range strings.SplitSeq(candidate, "/") {
		entries, err := os.ReadDir(parent)
		if err != nil {
			return false, err
		}
		if !slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return e.Name() == component }) {
			return false, nil
		}
		parent = filepath.Join(parent, component)
	}
	return true, nil
}

// Load reads a docdag.yaml file into a partial configuration.
func Load(path string) (Config, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read configuration %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(src, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration %s: %v: %w", path, err, model.ErrInvalidConfig)
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
	if override.Filename != "" {
		merged.Filename = override.Filename
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
	merged.References = mergeReferences(base.References, override.References)
	if override.AcyclicUnion {
		merged.AcyclicUnion = true
	}
	if len(override.Structural) > 0 {
		merged.Structural = maps.Clone(override.Structural)
	}
	if merged.StatusField != base.StatusField {
		if len(override.Rules) == 0 {
			merged.Rules = retargetRules(base.Rules, base.StatusField, merged.StatusField)
		}
		if len(override.DerivedEdges) == 0 {
			merged.DerivedEdges = retargetDerivedEdges(base.DerivedEdges, base.StatusField, merged.StatusField)
		}
	}
	return merged
}

func mergeReferences(base, override ReferencesSpec) ReferencesSpec {
	merged := base
	if override.Dangling != "" {
		merged.Dangling = override.Dangling
	}
	if override.Pattern != "" {
		merged.Pattern = override.Pattern
	}
	if len(override.Scan) > 0 {
		merged.Scan = slices.Clone(override.Scan)
	}
	return merged
}

// retargetRules moves the inherited rules onto a renamed status field. A rule
// that keeps inspecting the old attribute would silently pass, because a
// missing attribute satisfies every "not" clause.
func retargetRules(rules []Rule, from, to string) []Rule {
	out := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		cond, ok := rule.When.Attr[from]
		if ok {
			attr := make(map[string]AttrCondition, len(rule.When.Attr))
			maps.Copy(attr, rule.When.Attr)
			delete(attr, from)
			attr[to] = cond
			rule.When.Attr = attr
		}
		out = append(out, rule)
	}
	return out
}

// retargetDerivedEdges moves the inherited derived edges onto a renamed status
// field, so a MADR status string keeps deriving its edge.
func retargetDerivedEdges(specs []DerivedEdgeSpec, from, to string) []DerivedEdgeSpec {
	out := make([]DerivedEdgeSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Field == from {
			spec.Field = to
		}
		out = append(out, spec)
	}
	return out
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
