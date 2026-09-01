package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// Corpus reports what the current vault says about the configuration: the rules
// that fire nowhere in it, the rules that fire on every document they could
// apply to, the projections that hold nowhere or everywhere, and the edge types
// the corpus holds none of.
//
// This is model-dependent vacuity: unlike layer 1's findings it changes as the
// corpus changes, which is the point — a rule that fires nowhere today may be
// the one that catches tomorrow's mistake. firable names the rules a fixture
// has already shown can fire, and their silence in the corpus is a fact rather
// than a warning.
func Corpus(opts Options, firable map[string]bool) ([]model.Finding, error) {
	a := analyzer{cfg: opts.Config}
	g, cfg, loc := opts.Corpus, opts.Config, opts.Locator
	units := a.units()
	findings := []model.Finding{}

	fired := map[string]int{}
	for _, rule := range cfg.Rules {
		u, ok := named(units, SectionRules, rule.Name)
		if !ok {
			continue
		}
		count := len(firedOn(g, cfg, rule))
		fired[rule.Name] = count
		findings = append(findings, a.firingFindings(u, loc, count, a.scope(g, u), firable[rule.Name])...)
	}

	held := graph.EvalProjections(g, cfg)
	for _, spec := range cfg.Projections {
		u, ok := named(units, SectionProjections, spec.Name)
		if !ok {
			continue
		}
		findings = append(findings, a.holdingFindings(cfg, u, loc, len(held.Set(spec.Name)), a.scope(g, u))...)
	}

	counted := map[model.EdgeType]int{}
	for _, e := range g.Edges {
		counted[e.Type]++
	}
	for _, spec := range cfg.Edges {
		if counted[model.EdgeType(spec.Name)] > 0 {
			continue
		}
		findings = append(findings, model.Finding{
			Severity: model.SeverityInfo,
			Rule:     FindingUnusedEdgeInCorpus,
			ID:       model.ID(spec.Name),
			Detail:   "is declared and the corpus holds no edge of it",
			Location: loc.Locate(SectionEdges, spec.Name),
		})
	}

	if opts.Since == "" {
		return findings, nil
	}
	compared, err := a.sinceFindings(opts, fired)
	if err != nil {
		return nil, err
	}
	return append(findings, compared...), nil
}

// named returns the analyzed unit one declaration produced.
func named(units []unit, section, name string) (unit, bool) {
	for _, u := range units {
		if u.section == section && u.name == name {
			return u, true
		}
	}
	return unit{}, false
}

// subjects is the set of documents a rule could apply to: how many there are,
// and what to call them.
type subjects struct {
	count int
	noun  string
}

// scope counts the documents a unit can apply to, narrowed by the kinds its
// condition fixes. A rule that pins a kind — by an attr clause, or by naming an
// edge only one kind can be at the near end of — is answered over that kind
// alone, because "fired on 0 of 128" is a different fact when 120 of the 128
// documents could never have matched.
func (a analyzer) scope(g *model.Graph, u unit) subjects {
	kinds, narrowed := a.unitKinds(u)
	if !narrowed {
		return subjects{count: len(g.Nodes), noun: "documents"}
	}
	count := 0
	for _, n := range g.Nodes {
		if slices.Contains(kinds, n.Kind) {
			count++
		}
	}
	noun := "documents"
	if len(kinds) == 1 {
		noun = kinds[0] + " documents"
	}
	return subjects{count: count, noun: noun}
}

// unitKinds reports the kinds a unit's documents may have: the union over its
// alternatives, since a document matching any of them matches the unit. An
// alternative that pins nothing widens the answer to every kind, which is what
// narrowed being false says.
func (a analyzer) unitKinds(u unit) (kinds []string, narrowed bool) {
	if !a.cfg.Multikind() || len(u.dnf) == 0 {
		return nil, false
	}
	all := []string{}
	for _, c := range u.dnf {
		names, _, pinned := a.kinds(c, u.scope)
		if !pinned {
			return nil, false
		}
		all = append(all, names...)
	}
	return compacted(all), true
}

// firedOn returns the documents one rule reports, which is what "fired" means:
// the rule is evaluated exactly as `validate` evaluates it.
func firedOn(g *model.Graph, cfg config.Config, rule config.Rule) []model.ID {
	ids := []model.ID{}
	for _, f := range graph.EvalRule(g, cfg, rule) {
		if !slices.Contains(ids, f.ID) {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

// firingFindings reports what a rule's firing count says about it.
func (a analyzer) firingFindings(u unit, loc Locator, count int, on subjects, firable bool) []model.Finding {
	at := loc.Locate(u.section, u.name)
	switch {
	case on.count == 0:
		return []model.Finding{{
			Severity: model.SeverityInfo,
			Rule:     FindingNeverFired,
			ID:       model.ID(u.name),
			Detail:   fmt.Sprintf("fired on 0 of 0 %s: the corpus holds none it could apply to", on.noun),
			Location: at,
		}}
	case count == 0:
		f := model.Finding{
			Severity: model.SeverityWarn,
			Rule:     FindingNeverFired,
			ID:       model.ID(u.name),
			Detail:   fmt.Sprintf("fired on 0 of %d %s", on.count, on.noun),
			Location: at,
			Fix: fmt.Sprintf("keep it only if a fixture under %s/ shows it can fire (docdag lint --fixtures)",
				DefaultFixtureDir),
		}
		if firable {
			// A fixture has shown the rule can fire, so its silence here is a
			// fact about the corpus rather than a doubt about the rule.
			f.Severity, f.Fix = model.SeverityInfo, ""
			f.Detail += fmt.Sprintf(", and its %s fixture shows it can fire", FixtureFires)
		}
		return []model.Finding{f}
	case count == on.count:
		return []model.Finding{{
			Severity: model.SeverityWarn,
			Rule:     FindingAlwaysFired,
			ID:       model.ID(u.name),
			Detail:   fmt.Sprintf("fired on %d of %d %s", count, on.count, on.noun),
			Location: at,
			Fix:      "narrow the rule, or fix the corpus it reports on",
		}}
	}
	return nil
}

// holdingFindings reports what a projection's truth count says about it. A
// projection that holds nowhere is an error where `binding:` names it: the set
// of documents in force is then empty, and every command that reads it — the
// listing, the conflict check, the context walk — answers about nothing.
func (a analyzer) holdingFindings(cfg config.Config, u unit, loc Locator, count int, on subjects) []model.Finding {
	at := loc.Locate(u.section, u.name)
	switch {
	case on.count == 0:
		return nil
	case count == 0:
		f := model.Finding{
			Severity: model.SeverityWarn,
			Rule:     FindingNeverTrue,
			ID:       model.ID(u.name),
			Detail:   fmt.Sprintf("holds for 0 of %d %s", on.count, on.noun),
			Location: at,
			Fix:      "widen the projection, or drop it",
		}
		if cfg.Binding == u.name {
			f.Severity = model.SeverityError
			f.Detail += ", and binding: names it, so the binding set is empty"
			f.Fix = "widen the projection, or point binding: at one that holds"
		}
		return []model.Finding{f}
	case count == on.count:
		return []model.Finding{{
			Severity: model.SeverityWarn,
			Rule:     FindingAlwaysTrue,
			ID:       model.ID(u.name),
			Detail:   fmt.Sprintf("holds for %d of %d %s", count, on.count, on.noun),
			Location: at,
			Fix:      "narrow the projection, or drop it",
		}}
	}
	return nil
}

// sinceFindings compares the corpus against the one a revision holds and reports
// the rules whose silence began or ended there. Both are facts rather than
// faults: a rule that started firing is a corpus that changed, and one that
// stopped is a corpus that was fixed — or a rule that was narrowed until it
// says nothing.
func (a analyzer) sinceFindings(opts Options, fired map[string]int) ([]model.Finding, error) {
	base, cleanup, err := baseCorpus(opts)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	findings := []model.Finding{}
	for _, rule := range opts.Config.Rules {
		before := len(firedOn(base, opts.Config, rule))
		now := fired[rule.Name]
		at := opts.Locator.Locate(SectionRules, rule.Name)
		switch {
		case before == 0 && now > 0:
			findings = append(findings, model.Finding{
				Severity: model.SeverityInfo,
				Rule:     FindingNewlyFired,
				ID:       model.ID(rule.Name),
				Detail:   fmt.Sprintf("fired on %s, and on none at %s", documents(now), opts.Since),
				Location: at,
			})
		case before > 0 && now == 0:
			findings = append(findings, model.Finding{
				Severity: model.SeverityInfo,
				Rule:     FindingStoppedFiring,
				ID:       model.ID(rule.Name),
				Detail:   fmt.Sprintf("fires on nothing, and fired on %s at %s", documents(before), opts.Since),
				Location: at,
			})
		}
	}
	return findings, nil
}

// documents counts documents as a phrase, so a report about one of them does
// not read like a machine wrote it.
func documents(n int) string {
	if n == 1 {
		return "1 document"
	}
	return fmt.Sprintf("%d documents", n)
}

// baseCorpus builds the graph of the corpus as of a revision. The committed
// files are written into a temporary tree and read by the same parser the
// working tree is read by, so the comparison is between two corpora rather than
// between a corpus and a guess about one; the caller runs cleanup when it is
// done with the graph.
func baseCorpus(opts Options) (*model.Graph, func(), error) {
	if opts.Repo == nil {
		return nil, nil, fmt.Errorf("--since needs a git repository: %w", model.ErrInvalidConfig)
	}
	root, err := os.MkdirTemp("", "docdag-lint-")
	if err != nil {
		return nil, nil, fmt.Errorf("create a working directory for %s: %w", opts.Since, err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	base := opts.Repo.MergeBase(opts.Since)
	cfg := opts.Config
	rerooted := map[string]string{}
	for name, dir := range corpusDirs(cfg) {
		rel, err := opts.Repo.Rel(dir)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		// Files names the directory the way the caller holds it and relativizes
		// it itself; rel is what the temporary tree is laid out under.
		files, err := opts.Repo.Files(base, dir)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		for _, path := range files {
			content, err := opts.Repo.File(base, path)
			if err != nil {
				continue
			}
			target := filepath.Join(root, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("write the corpus at %s: %w", base, err)
			}
			if err := os.WriteFile(target, content, 0o644); err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("write the corpus at %s: %w", base, err)
			}
		}
		rerooted[name] = filepath.Join(root, filepath.FromSlash(rel))
	}
	g, err := corpusGraph(rootedConfig(cfg, rerooted))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return g, cleanup, nil
}

// corpusDirs names the directories a configuration reads documents from, keyed
// by kind — the empty key being the one directory a single-kind corpus has.
func corpusDirs(cfg config.Config) map[string]string {
	if !cfg.Multikind() {
		return map[string]string{"": cfg.Dir}
	}
	dirs := make(map[string]string, len(cfg.Kinds))
	for name, spec := range cfg.Kinds {
		dirs[name] = spec.Dir
	}
	return dirs
}

// rootedConfig returns the configuration reading its documents from the given
// directories instead of its own. It is how one configuration is evaluated
// against a second corpus — a revision's, or a fixture's — without any of the
// checks knowing there is more than one.
func rootedConfig(cfg config.Config, dirs map[string]string) config.Config {
	if !cfg.Multikind() {
		cfg.Dir = dirs[""]
		return cfg
	}
	kinds := make(map[string]config.KindSpec, len(cfg.Kinds))
	for name, spec := range cfg.Kinds {
		if dir, ok := dirs[name]; ok {
			spec.Dir = dir
		}
		kinds[name] = spec
	}
	cfg.Kinds = kinds
	return cfg
}

// corpusGraph reads the documents a configuration describes and builds the
// graph. A directory the tree does not hold is read as empty rather than as an
// error: a fixture corpus holds the kinds its rule is about and no others, and
// a revision may predate a kind entirely.
func corpusGraph(cfg config.Config) (*model.Graph, error) {
	docs := []*parse.Document{}
	if !cfg.Multikind() {
		read, err := readDir(cfg.Dir, func() ([]*parse.Document, error) { return parse.Dir(cfg.Dir, cfg) })
		if err != nil {
			return nil, err
		}
		return graph.Build(read, cfg), nil
	}
	for _, name := range cfg.KindNames() {
		dir := cfg.Kinds[name].Dir
		read, err := readDir(dir, func() ([]*parse.Document, error) { return parse.KindDir(dir, cfg, name) })
		if err != nil {
			return nil, err
		}
		docs = append(docs, read...)
	}
	return graph.Build(docs, cfg), nil
}

// readDir reads one documents directory, answering with no documents where the
// directory is not there at all.
func readDir(dir string, read func() ([]*parse.Document, error)) ([]*parse.Document, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil
	}
	return read()
}
