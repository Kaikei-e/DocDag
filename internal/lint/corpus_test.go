package lint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

// testCorpusGraph reads a single-kind corpus out of a directory under one
// configuration, the way the corpus layer reads the vault.
func testCorpusGraph(t *testing.T, cfg config.Config, dir string) *model.Graph {
	t.Helper()
	g, err := corpusGraph(rootedConfig(cfg, map[string]string{"": dir}))
	if err != nil {
		t.Fatalf("read the corpus at %s: %v", dir, err)
	}
	return g
}

// testCorpusLint runs layers 1 and 2 over a corpus on disk.
func testCorpusLint(t *testing.T, cfg config.Config, dir string) []model.Finding {
	t.Helper()
	findings, err := Corpus(Options{
		Config:  cfg,
		Locator: NewLocator("", cfg.Preset),
		Corpus:  testCorpusGraph(t, cfg, dir),
	}, nil)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	return findings
}

// testTwoDecisions is a corpus of one superseded decision and the one that
// replaced it, which is what the ADR preset's two rules are written about.
func testTwoDecisions() map[string]string {
	return map[string]string{
		"0001-the-first-decision.md":  testDocument("The first decision", config.StatusSuperseded),
		"0002-the-second-decision.md": testDocument("The second decision", config.StatusAccepted, "supersedes:", "  - \"0001\""),
	}
}

func TestCorpusFiring(t *testing.T) {
	dir := testCorpus(t, testTwoDecisions())

	t.Run("a rule the corpus never fires", func(t *testing.T) {
		cfg := config.ADRPreset()

		findings := testCorpusLint(t, cfg, dir)

		f := assertFinding(t, findings, FindingNeverFired, model.RuleStatusDrift, model.SeverityWarn, "fired on 0 of 2 documents")
		if !strings.Contains(f.Fix, "docdag lint --fixtures") {
			t.Errorf("fix = %q, want it to name the fixture that would settle the doubt", f.Fix)
		}
	})

	t.Run("a rule the corpus fires everywhere", func(t *testing.T) {
		cfg := testConfig(testRule("every_document", model.SeverityWarn, testAttr(config.KeyTitle, testNot("nothing"))))

		findings := testCorpusLint(t, cfg, dir)

		assertFinding(t, findings, FindingAlwaysFired, "every_document", model.SeverityWarn, "fired on 2 of 2 documents")
	})

	t.Run("a rule that fires on some of the corpus is not reported", func(t *testing.T) {
		cfg := testConfig(testRule("the_superseded", model.SeverityWarn, testAttr(config.DefaultStatusField, testEq(config.StatusSuperseded))))

		findings := testCorpusLint(t, cfg, dir)

		assertNoFinding(t, findings, FindingNeverFired)
		assertNoFinding(t, findings, FindingAlwaysFired)
	})

	t.Run("an edge type the corpus holds none of", func(t *testing.T) {
		cfg := config.ADRPreset()

		findings := testCorpusLint(t, cfg, dir)

		assertFinding(t, findings, FindingUnusedEdgeInCorpus, config.EdgeDependsOn.String(), model.SeverityInfo,
			"the corpus holds no edge of it")
	})
}

// TestCorpusKindNarrowing reads a fixture corpus of the spec preset, where a
// rule that pins a kind is answered over that kind's documents alone.
func TestCorpusKindNarrowing(t *testing.T) {
	cfg := config.SpecPreset()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "lint", "spec", "orphan_must", "ruleid"))
	if err != nil {
		t.Fatalf("resolve the fixture: %v", err)
	}
	dirs := map[string]string{}
	for name, spec := range cfg.Kinds {
		dirs[name] = filepath.Join(root, spec.Dir)
	}
	g, err := corpusGraph(rootedConfig(cfg, dirs))
	if err != nil {
		t.Fatalf("read the corpus: %v", err)
	}
	findings, err := Corpus(Options{Config: cfg, Locator: NewLocator("", cfg.Preset), Corpus: g}, nil)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}

	// orphan_test pins kind: conform, and the corpus holds one clause and no
	// conformance test at all, so the rule has nothing it could apply to.
	assertFinding(t, findings, FindingNeverFired, model.RuleOrphanTest, model.SeverityInfo,
		"fired on 0 of 0 conform documents")
	// no_counterexample pins kind: clause, and the one clause fires it.
	assertFinding(t, findings, FindingAlwaysFired, model.RuleNoCounterexample, model.SeverityWarn,
		"fired on 1 of 1 clause documents")
}

func TestCorpusProjections(t *testing.T) {
	dir := testCorpus(t, testTwoDecisions())

	t.Run("a binding projection that holds nowhere is an error", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Projections = []config.ProjectionSpec{{
			Name: config.ProjectionAcceptedUnsuperseded,
			When: testAttr(config.DefaultStatusField, testEq(config.StatusRejected)),
		}}

		findings := testCorpusLint(t, cfg, dir)

		assertFinding(t, findings, FindingNeverTrue, config.ProjectionAcceptedUnsuperseded, model.SeverityError,
			"binding: names it, so the binding set is empty")
	})

	t.Run("a projection that holds nowhere is a warning otherwise", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Projections = append(cfg.Projections, config.ProjectionSpec{
			Name: "rejected",
			When: testAttr(config.DefaultStatusField, testEq(config.StatusRejected)),
		})

		findings := testCorpusLint(t, cfg, dir)

		assertFinding(t, findings, FindingNeverTrue, "rejected", model.SeverityWarn, "holds for 0 of 2 documents")
	})

	t.Run("a projection that holds everywhere", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Projections = append(cfg.Projections, config.ProjectionSpec{
			Name: "written",
			When: testAttr(config.KeyTitle, testNot("nothing")),
		})

		findings := testCorpusLint(t, cfg, dir)

		assertFinding(t, findings, FindingAlwaysTrue, "written", model.SeverityWarn, "holds for 2 of 2 documents")
	})
}

// TestCorpusSince compares the corpus against the one a revision holds, which
// is where a rule that started or stopped firing is reported.
func TestCorpusSince(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs", "adr")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("create the documents directory: %v", err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(docs, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}

	// The base revision is a corpus nothing is wrong with: one decision,
	// superseded by the second, which says so.
	write("0001-the-first-decision.md", testDocument("The first decision", config.StatusSuperseded))
	write("0002-the-second-decision.md", testDocument("The second decision", config.StatusAccepted, "supersedes:", "  - \"0001\""))
	git("init", "--quiet")
	git("add", "-A")
	git("-c", "user.name=DocDag Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "the first revision")

	// The working tree drifts: the superseded decision calls itself accepted.
	write("0001-the-first-decision.md", testDocument("The first decision", config.StatusAccepted))

	repo, err := vcs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cfg := config.ADRPreset()
	cfg.Dir = docs
	g, err := corpusGraph(cfg)
	if err != nil {
		t.Fatalf("read the corpus: %v", err)
	}
	findings, err := Corpus(Options{
		Config:  cfg,
		Locator: NewLocator("", cfg.Preset),
		Corpus:  g,
		Since:   "HEAD",
		Repo:    repo,
	}, nil)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}

	assertFinding(t, findings, FindingNewlyFired, model.RuleStatusDrift, model.SeverityInfo, "and on none at HEAD")
	assertNoFinding(t, findings, FindingStoppedFiring)
}
