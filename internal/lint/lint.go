// Package lint reports what is wrong with a DocDag configuration rather than
// with the documents it describes: conditions no document could satisfy,
// conditions every document satisfies, rules the corpus never fires, and rules
// whose own fixtures disagree with them.
//
// It answers in three layers, because "the check never fired" is two different
// facts. Layer 1 reads the configuration alone and reports the faults that are
// there whatever the corpus holds — inherent vacuity, in the model checkers'
// sense. Layer 2 evaluates the configuration against the corpus and reports the
// rules that fire nowhere or everywhere in it. Layer 3 runs each rule against
// the two miniature corpora a fixture directory holds, one where it must fire
// and one where it must not, so a rule that fires nowhere in the vault can
// still be shown to be capable of firing at all.
//
// Nothing here searches: every judgement is a set operation over finite domains
// — the declared vocabularies, the kinds an edge joins, an integer degree
// window — which is what the fixed rule vocabulary buys, and why lint needs no
// SAT solver to be exact about the conditions it does judge.
package lint

import (
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

// The findings lint reports. They are not structural checks: `structural:`
// cannot raise or lower them, because they are about the configuration rather
// than about the corpus the configuration is the contract for.
const (
	FindingUnsatisfiableCondition  = "unsatisfiable_condition"
	FindingUnfirableRule           = "unfirable_rule"
	FindingTautologicalRule        = "tautological_rule"
	FindingSubsumedRule            = "subsumed_rule"
	FindingShadowedRule            = "shadowed_rule"
	FindingAmbivalentFix           = "ambivalent_fix"
	FindingUnusedEdge              = "unused_edge"
	FindingUnusedStatus            = "unused_status"
	FindingUnsatisfiableProjection = "unsatisfiable_projection"
	FindingTautologicalProjection  = "tautological_projection"
	FindingConditionTooWide        = "condition_too_wide"
	FindingPreferTarget            = "prefer_target"
	FindingNeverFired              = "never_fired"
	FindingAlwaysFired             = "always_fired"
	FindingNeverTrue               = "never_true"
	FindingAlwaysTrue              = "always_true"
	FindingUnusedEdgeInCorpus      = "unused_edge_in_corpus"
	FindingNewlyFired              = "newly_fired"
	FindingStoppedFiring           = "stopped_firing"
	FindingMissingFixture          = "missing_fixture"
	FindingFixtureMismatch         = "fixture_mismatch"
)

// DefaultFixtureDir is where `docdag lint --all` looks for the per-rule
// fixtures, and where `docdag new --fixture` writes them.
const DefaultFixtureDir = "lint"

// The two directories a rule's fixture holds. The names are Semgrep's, which
// is a convention agents and reviewers already read: ruleid is where the rule
// must fire, ok is where it must not.
const (
	FixtureFires  = "ruleid"
	FixtureSilent = "ok"
)

// Summary counts what a lint run reported. It is a different shape from a
// validation summary: there are no documents or edges to count when only the
// configuration was read, and info findings are counted so a reader can tell an
// empty report from a quiet one.
type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// Summarize counts the findings by severity.
func Summarize(findings []model.Finding) Summary {
	summary := Summary{}
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityError:
			summary.Errors++
		case model.SeverityWarn:
			summary.Warnings++
		case model.SeverityInfo:
			summary.Infos++
		}
	}
	return summary
}

// Options are one lint run: the configuration to read, where its file is, and
// which of the three layers to answer in.
//
// Corpus is the graph layer 2 evaluates, nil where the run does not read the
// vault. Fixtures is the directory layer 3 walks, empty where the run does not
// read fixtures. Since names the revision layer 2 compares the corpus against,
// and needs Repo to resolve it.
//
// Root and Reported are two different directories on purpose: Root is where the
// configuration describes its corpus from — the directory holding docdag.yaml,
// which a fixture's kind directories are rerooted against — and Reported is
// where the caller is standing, which is what a finding names its files
// relative to.
type Options struct {
	Config   config.Config
	Locator  Locator
	Corpus   *model.Graph
	Fixtures string
	Since    string
	Repo     *vcs.Repo
	Root     string
	Reported string
	AsOf     time.Time
}

// Run answers in every layer the options ask for, in one deterministic report.
//
// The layers are run fixtures first, corpus second, because they answer one
// question together: a rule that fires nowhere in the vault is a warning on its
// own, and only a fact where a fixture has shown it can fire at all. That is the
// whole of the ambiguity a silent check leaves behind — high quality or no
// detection — and the two layers between them settle it.
func Run(opts Options) ([]model.Finding, error) {
	findings := Inherent(opts.Config, opts.Locator)
	firable := map[string]bool{}
	if opts.Fixtures != "" {
		fixtures, proven, err := Fixtures(opts)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fixtures...)
		firable = proven
	}
	if opts.Corpus != nil {
		corpus, err := Corpus(opts, firable)
		if err != nil {
			return nil, err
		}
		findings = append(findings, corpus...)
	}
	graph.SortFindings(findings)
	return findings, nil
}
