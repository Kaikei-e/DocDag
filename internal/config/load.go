package config

import "github.com/Kaikei-e/DocDag/internal/model"

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
	return "", model.ErrNotImplemented
}

// Load reads a docdag.yaml file into a partial configuration.
func Load(path string) (Config, error) { return Config{}, model.ErrNotImplemented }

// Merge overlays a partial configuration onto a base, returning the effective
// configuration. Set fields and non-empty lists replace their base counterpart.
func Merge(base, override Config) Config { return Config{} }

// Resolve produces the effective configuration for one CLI invocation,
// including documents-directory discovery when nothing selected one.
func Resolve(opts Options) (Config, error) { return Config{}, model.ErrNotImplemented }
