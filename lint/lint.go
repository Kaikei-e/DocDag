// Package lint is the small public entry for checking a DocDag configuration
// in process. The CLI-facing surface under internal/lint stays wider — callers
// that only need "does this Config lint" reach for Check.
package lint

import (
	"fmt"
	"path/filepath"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	internallint "github.com/Kaikei-e/DocDag/internal/lint"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

// Check reports what is wrong with cfg rather than with the documents it
// describes. vaultDir empty skips the corpus layer; fixturesDir empty skips the
// fixture layer. Both empty answers only the inherent (DNF) layer — the one a
// Go test that assembled cfg can run without a vault on disk.
//
// When vaultDir is set, kind and document directories in cfg that are still
// relative are resolved against it, the way Resolve roots them against the
// directory holding docdag.yaml.
func Check(cfg config.Config, vaultDir, fixturesDir string) ([]model.Finding, error) {
	root := vaultDir
	reported := vaultDir
	if root == "" {
		root = "."
		reported = "."
	}
	rooted := rootConfig(cfg, vaultDir)
	opts := internallint.Options{
		Config:   rooted,
		Locator:  internallint.NewLocator("", cfg.Preset),
		Fixtures: fixturesDir,
		Root:     root,
		Reported: reported,
	}
	if vaultDir != "" {
		docs, err := parse.Documents(rooted)
		if err != nil {
			return nil, fmt.Errorf("lint: read vault %s: %w", vaultDir, err)
		}
		opts.Corpus = graph.Build(docs, rooted)
	}
	return internallint.Run(opts)
}

// rootConfig resolves document directories against vaultDir. An empty vaultDir
// leaves cfg alone so the inherent layer can run without a tree on disk.
func rootConfig(cfg config.Config, vaultDir string) config.Config {
	if vaultDir == "" {
		return cfg
	}
	if !cfg.Multikind() {
		if cfg.Dir == "" {
			cfg.Dir = vaultDir
		} else if !filepath.IsAbs(cfg.Dir) {
			cfg.Dir = filepath.Join(vaultDir, cfg.Dir)
		}
		return cfg
	}
	dirs := make(map[string]string, len(cfg.Kinds))
	for name, spec := range cfg.Kinds {
		switch {
		case spec.Dir == "":
			dirs[name] = vaultDir
		case filepath.IsAbs(spec.Dir):
			dirs[name] = spec.Dir
		default:
			dirs[name] = filepath.Join(vaultDir, spec.Dir)
		}
	}
	return cfg.Reroot(dirs)
}
