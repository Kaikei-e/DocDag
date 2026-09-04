package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// Fixtures runs every rule against the two miniature corpora its fixture
// directory holds: one where it has to fire, and one where it must not. It
// reports the rules whose fixtures disagree with them, and the rules that have
// no fixture at all, and returns the rules a fixture proved can fire.
//
// The convention is Semgrep's, deliberately: a rule with one true positive and
// one true negative beside it is a rule whose intent survives being rewritten,
// and both the reviewers and the agents that will edit these rules already read
// those two words.
func Fixtures(opts Options) ([]model.Finding, map[string]bool, error) {
	cfg, loc := opts.Config, opts.Locator
	findings := []model.Finding{}
	firable := map[string]bool{}
	bundled := presetRules(cfg)

	for _, rule := range cfg.Rules {
		for _, side := range []string{FixtureFires, FixtureSilent} {
			if _, err := os.Stat(filepath.Join(opts.Fixtures, rule.Name, side)); err == nil {
				continue
			}
			if bundled[rule.Name] {
				// A preset ships its own fixtures, so a corpus that only names
				// the preset is not missing anything it wrote.
				continue
			}
			findings = append(findings, model.Finding{
				Severity: model.SeverityWarn,
				Rule:     FindingMissingFixture,
				ID:       model.ID(rule.Name),
				Detail: fmt.Sprintf("has no %s fixture under %s",
					side, filepath.ToSlash(filepath.Join(opts.Fixtures, rule.Name))),
				Location: loc.Locate(SectionRules, rule.Name),
				Fix:      fmt.Sprintf("run docdag new --fixture %s", rule.Name),
			})
		}
	}

	names, err := fixtureNames(opts.Fixtures)
	if err != nil {
		return nil, nil, err
	}
	for _, name := range names {
		if !checkable(cfg, name) {
			continue
		}
		for _, side := range []string{FixtureFires, FixtureSilent} {
			dir := filepath.Join(opts.Fixtures, name, side)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			fired, err := firings(opts, dir, name)
			if err != nil {
				return nil, nil, err
			}
			if side == FixtureFires && len(fired) > 0 {
				firable[name] = true
			}
			findings = append(findings, mismatches(loc, cfg, name, side, dir, fired)...)
		}
	}
	graph.SortFindings(findings)
	return findings, firable, nil
}

// mismatches reports a fixture that says the opposite of what the rule does:
// nothing fired where the rule had to, or something fired where it had to stay
// silent. The finding is filed on the fixture — that is the file a reader
// opens — and relates the rule's own line in the configuration.
func mismatches(loc Locator, cfg config.Config, name, side, dir string, fired []firing) []model.Finding {
	at := loc.Locate(section(cfg, name), name)
	if side == FixtureFires {
		if len(fired) > 0 {
			return nil
		}
		return []model.Finding{{
			Severity: model.SeverityError,
			Rule:     FindingFixtureMismatch,
			ID:       model.ID(name),
			Detail:   fmt.Sprintf("did not fire in %s, where it has to", filepath.ToSlash(dir)),
			Location: model.Location{Path: filepath.ToSlash(dir)},
			Related:  []model.Location{at},
			Fix:      "write a document there the rule reports, or widen the rule",
		}}
	}
	findings := make([]model.Finding, 0, len(fired))
	for _, f := range fired {
		findings = append(findings, model.Finding{
			Severity: model.SeverityError,
			Rule:     FindingFixtureMismatch,
			ID:       model.ID(name),
			Detail:   fmt.Sprintf("fired on %s in %s, where it must not", f.id, filepath.ToSlash(dir)),
			Location: f.at,
			Related:  []model.Location{at},
			Fix:      "narrow the rule, or move the document to " + FixtureFires,
		})
	}
	return findings
}

// firing is one document a rule reported inside a fixture corpus.
type firing struct {
	id model.ID
	at model.Location
}

// firings evaluates one fixture corpus and reports where the named check fired.
// The corpus is read under the same configuration as the vault, with the
// documents directories rerooted onto the fixture: a fixture is not a
// configuration of its own, or it would be testing something else.
func firings(opts Options, dir, name string) ([]firing, error) {
	cfg := opts.Config.Reroot(fixtureDirs(opts.Config, opts.Root, dir))
	g, err := corpusGraph(cfg)
	if err != nil {
		return nil, err
	}
	for _, n := range g.Nodes {
		n.Path = parse.LocalPath(opts.Reported, n.Path)
	}
	if _, declared := cfg.Projection(name); declared {
		held := graph.EvalProjections(g, cfg, opts.AsOf).Set(name)
		out := make([]firing, 0, len(held))
		for _, id := range held {
			out = append(out, firing{id: id, at: g.Nodes[id].Location(cfg.EffectiveStatus())})
		}
		return out, nil
	}
	out := []firing{}
	for _, f := range graph.Validate(g, cfg, opts.AsOf) {
		// A finding the corpus has already answered has not fired: a suppressed
		// conflict is exactly what an `ok` fixture recording an exception is
		// written to show.
		if f.Rule != name || f.Suppressed {
			continue
		}
		out = append(out, firing{id: f.ID, at: model.Location{
			Path: parse.LocalPath(opts.Reported, f.Location.Path),
			Line: f.Location.Line,
		}})
	}
	return out, nil
}

// fixtureDirs maps each documents directory onto its place inside a fixture. A
// single-kind corpus reads the fixture directory itself — there is one
// directory of documents and this is it — and a corpus that declares kinds
// keeps its own kind-relative layout below the fixture, so a document sits in
// the directory that declares what kind it is.
func fixtureDirs(cfg config.Config, root, fixture string) map[string]string {
	if !cfg.Multikind() {
		return map[string]string{"": fixture}
	}
	dirs := make(map[string]string, len(cfg.Kinds))
	for name, spec := range cfg.Kinds {
		dirs[name] = filepath.Join(fixture, kindRelative(root, spec.Dir))
	}
	return dirs
}

// kindRelative names one kind's directory the way its configuration wrote it:
// relative to the corpus root. Both are resolved against the working directory
// first, because a configuration read from a relative path roots its kinds
// relatively too and the two have to be compared in one spelling. A directory
// outside the root has no relative spelling at all, and its last component is
// the closest thing to one.
func kindRelative(root, dir string) string {
	absoluteRoot, rootErr := filepath.Abs(root)
	absoluteDir, dirErr := filepath.Abs(dir)
	if rootErr != nil || dirErr != nil {
		return filepath.Base(dir)
	}
	rel, err := filepath.Rel(absoluteRoot, absoluteDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Base(dir)
	}
	return rel
}

// fixtureNames lists the rules a fixture directory holds fixtures for, sorted.
// A directory that is not there holds none, which is not an error: a corpus
// adopting fixtures writes the first one after asking for them.
func fixtureNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read fixtures directory %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// checkable reports whether a name is something a fixture can be written for: a
// configured rule, a projection, or one of the built-in structural checks.
func checkable(cfg config.Config, name string) bool {
	if slices.ContainsFunc(cfg.Rules, func(rule config.Rule) bool { return rule.Name == name }) {
		return true
	}
	if _, declared := cfg.Projection(name); declared {
		return true
	}
	return cfg.Severity(name) != ""
}

// section names the part of the configuration a checkable name was written in,
// which is where its finding points.
func section(cfg config.Config, name string) string {
	if _, declared := cfg.Projection(name); declared {
		return SectionProjections
	}
	return SectionRules
}

// presetRules names the rules the configuration inherited from its preset. They
// are exempt from missing_fixture: DocDag ships their fixtures, and a corpus
// that names a preset did not write those rules.
func presetRules(cfg config.Config) map[string]bool {
	preset, err := config.Preset(cfg.Preset)
	if err != nil {
		return nil
	}
	names := make(map[string]bool, len(preset.Rules))
	for _, rule := range preset.Rules {
		names[rule.Name] = true
	}
	return names
}
