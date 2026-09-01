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
// configuration. Set fields replace their base counterpart, and so does a list
// the override writes down: an explicitly empty one clears the preset's, which
// is how a corpus says it derives no edges at all.
func Merge(base, override Config) Config {
	merged := base
	if override.Preset != "" {
		merged.Preset = override.Preset
	}
	if override.PresetVersion != 0 {
		merged.PresetVersion = override.PresetVersion
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
	// The kinds are a vocabulary, not a set of defaults to extend: a corpus
	// that writes kinds: describes every kind it has, the way writing edges:
	// describes every edge it has.
	if override.Kinds != nil {
		merged.Kinds = maps.Clone(override.Kinds)
	}
	// The field declarations are a vocabulary too: a corpus that writes fields:
	// describes every field it declares, and an explicitly empty map says it
	// declares none.
	if override.Fields != nil {
		merged.Fields = maps.Clone(override.Fields)
	}
	// A period is one declaration rather than a set, so writing one replaces
	// the preset's: a corpus that dates its documents differently says so once.
	if override.Period != nil {
		merged.Period = override.Period
	}
	if override.Edges != nil {
		merged.Edges = slices.Clone(override.Edges)
	}
	if override.DerivedEdges != nil {
		merged.DerivedEdges = slices.Clone(override.DerivedEdges)
	}
	if override.Rules != nil {
		merged.Rules = slices.Clone(override.Rules)
	}
	if override.Projections != nil {
		merged.Projections = slices.Clone(override.Projections)
	}
	if override.PathConstraints != nil {
		merged.PathConstraints = slices.Clone(override.PathConstraints)
	}
	if override.Binding != "" {
		merged.Binding = override.Binding
	}
	merged.References = mergeReferences(base.References, override.References)
	if override.AcyclicUnion {
		merged.AcyclicUnion = true
	}
	if len(override.Structural) > 0 {
		merged.Structural = maps.Clone(override.Structural)
	}
	if override.Edges != nil && override.Projections == nil {
		merged = dropUnsupportedProjections(merged)
	}
	if merged.StatusField != base.StatusField {
		if override.Rules == nil {
			merged.Rules = retargetRules(base.Rules, base.StatusField, merged.StatusField)
		}
		if override.Projections == nil {
			merged.Projections = retargetProjections(base.Projections, base.StatusField, merged.StatusField)
		}
		if override.DerivedEdges == nil {
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
		rule.When.Attr = retargetAttrs(rule.When.Attr, from, to)
		out = append(out, rule)
	}
	return out
}

// retargetProjections moves the inherited projections onto a renamed status
// field, for the same reason the rules move: a binding projection still reading
// the preset's status key would hold everywhere, and every listing would call
// every document current.
func retargetProjections(projections []ProjectionSpec, from, to string) []ProjectionSpec {
	out := make([]ProjectionSpec, 0, len(projections))
	for _, spec := range projections {
		spec.When.Attr = retargetAttrs(spec.When.Attr, from, to)
		if spec.AnyOf != nil {
			alternatives := make([]ProjectionAlt, 0, len(spec.AnyOf))
			for _, alt := range spec.AnyOf {
				alt.When.Attr = retargetAttrs(alt.When.Attr, from, to)
				alternatives = append(alternatives, alt)
			}
			spec.AnyOf = alternatives
		}
		out = append(out, spec)
	}
	return out
}

// dropUnsupportedProjections drops the inherited projections that read an edge
// type the corpus replaced away, and the binding that named one of them. The
// preset's projections are written against the preset's edges: a corpus that
// declares an edge vocabulary of its own and no projections is not asking for a
// projection it has no vocabulary for, and keeping one would fail validation on
// a configuration nobody wrote. What is binding then falls back on the built-in
// definition, the same answer the corpus got before it had projections at all.
func dropUnsupportedProjections(cfg Config) Config {
	kept := make([]ProjectionSpec, 0, len(cfg.Projections))
	for _, spec := range cfg.Projections {
		if !cfg.supports(spec) {
			if cfg.Binding == spec.Name {
				cfg.Binding = ""
			}
			continue
		}
		kept = append(kept, spec)
	}
	cfg.Projections = kept
	return cfg
}

// supports reports whether every edge type a projection reads is declared.
func (c Config) supports(spec ProjectionSpec) bool {
	for _, cond := range spec.Conditions() {
		for _, clause := range cond.EdgeClauses() {
			if _, ok := c.Edge(model.EdgeType(clause.Edge)); !ok {
				return false
			}
		}
		for _, clause := range cond.ViaClauses() {
			if _, ok := c.Edge(model.EdgeType(clause.Edge)); !ok {
				return false
			}
		}
	}
	return true
}

// retargetAttrs moves one attribute clause onto a renamed key, copying the map
// so the inherited configuration keeps the clause it was written with.
func retargetAttrs(attrs map[string]AttrCondition, from, to string) map[string]AttrCondition {
	cond, ok := attrs[from]
	if !ok {
		return attrs
	}
	moved := make(map[string]AttrCondition, len(attrs))
	maps.Copy(moved, attrs)
	delete(moved, from)
	moved[to] = cond
	return moved
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
	file, from, err := loadOptional(root, opts.ConfigPath)
	if err != nil {
		return Config{}, err
	}
	if err := validateWrittenKinds(file); err != nil {
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
	if cfg.Multikind() {
		// A kind's directory is written beside the kinds that declare it, so it
		// is read relative to the file that wrote it down rather than to the
		// process's directory: a corpus is described from where it lives.
		cfg.Kinds = rootedKinds(cfg.Kinds, kindRoot(root, from))
		return cfg, nil
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

// validateWrittenKinds rejects a configuration file that declares kinds and
// also writes a top-level id_width. Only the file can be asked this: the preset
// supplies the width every digit-run kind still pads to, so the merged
// configuration cannot tell an inherited width from a written one, while the
// file that writes both is describing one identity model twice.
func validateWrittenKinds(file Config) error {
	if file.Multikind() && file.IDWidth != 0 {
		return fmt.Errorf("id_width %d describes nothing beside kinds, which declare their own id patterns: %w",
			file.IDWidth, model.ErrInvalidConfig)
	}
	return nil
}

// kindRoot is the directory kind directories are resolved against: the one
// holding the configuration file that declared them, or the root when no file
// did.
func kindRoot(root, from string) string {
	if from == "" {
		return root
	}
	return filepath.Dir(from)
}

// rootedKinds resolves each kind's directory against root, leaving an absolute
// one alone, so every caller can open a kind's directory without knowing where
// the configuration came from.
func rootedKinds(kinds map[string]KindSpec, root string) map[string]KindSpec {
	rooted := make(map[string]KindSpec, len(kinds))
	for name, spec := range kinds {
		dir := filepath.FromSlash(spec.Dir)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		spec.Dir = dir
		rooted[name] = spec
	}
	return rooted
}

// loadOptional reads the named configuration file, or the one at the root when
// none was named, and reports the path it read. A root without a configuration
// file is not an error, and reports no path.
func loadOptional(root, path string) (Config, string, error) {
	if path == "" {
		candidate := filepath.Join(root, DefaultConfigFile)
		if _, err := os.Stat(candidate); err != nil {
			return Config{}, "", nil
		}
		path = candidate
	}
	cfg, err := Load(path)
	return cfg, path, err
}
