package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// testFixtureOptions runs layer 3 over a fixture directory under a single-kind
// configuration whose corpus is the directory itself.
func testFixtureOptions(cfg config.Config, root, fixtures string) Options {
	return Options{
		Config:   cfg,
		Locator:  NewLocator("", cfg.Preset),
		Fixtures: fixtures,
		Root:     root,
		Reported: root,
	}
}

// testDriftRule is status_drift written out, so a fixture test does not depend
// on the preset keeping the rule.
func testDriftRule() config.Rule {
	return testRule("drift", model.SeverityError, config.Condition{
		Inbound: config.EdgeCondition{Edge: "supersedes"},
		Attr:    map[string]config.AttrCondition{config.DefaultStatusField: testNot(config.StatusSuperseded)},
	})
}

// testDriftFixture is the fixture the rule above deserves: a corpus where the
// replaced decision still calls itself accepted, and one where it does not.
func testDriftFixture() map[string]string {
	return map[string]string{
		"drift/ruleid/0001-the-first-decision.md":  testDocument("The first decision", config.StatusAccepted),
		"drift/ruleid/0002-the-second-decision.md": testDocument("The second decision", config.StatusAccepted, "supersedes:", "  - \"0001\""),
		"drift/ok/0001-the-first-decision.md":      testDocument("The first decision", config.StatusSuperseded),
		"drift/ok/0002-the-second-decision.md":     testDocument("The second decision", config.StatusAccepted, "supersedes:", "  - \"0001\""),
	}
}

func TestFixturesPass(t *testing.T) {
	root := testCorpus(t, testDriftFixture())
	cfg := testConfig(testDriftRule())

	findings, firable, err := Fixtures(testFixtureOptions(cfg, root, root))
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}

	if len(findings) > 0 {
		t.Errorf("findings = %s, want a passing fixture to report nothing", formatFindings(findings))
	}
	if !firable["drift"] {
		t.Error("firable = false, want the ruleid corpus to prove the rule can fire")
	}
}

func TestFixtureMismatch(t *testing.T) {
	t.Run("a rule that does not fire where it has to", func(t *testing.T) {
		files := testDriftFixture()
		files["drift/ruleid/0001-the-first-decision.md"] = testDocument("The first decision", config.StatusSuperseded)
		root := testCorpus(t, files)

		findings, firable, err := Fixtures(testFixtureOptions(testConfig(testDriftRule()), root, root))
		if err != nil {
			t.Fatalf("Fixtures: %v", err)
		}

		f := assertFinding(t, findings, FindingFixtureMismatch, "drift", model.SeverityError, "did not fire in")
		if !strings.HasSuffix(filepath.ToSlash(f.Location.Path), "drift/"+FixtureFires) {
			t.Errorf("location = %q, want the directory the rule had to fire in", f.Location.Path)
		}
		if len(f.Related) != 1 {
			t.Errorf("related = %v, want the rule's own line", f.Related)
		}
		if firable["drift"] {
			t.Error("firable = true, want a fixture that did not fire to prove nothing")
		}
	})

	t.Run("a rule that fires where it must not", func(t *testing.T) {
		files := testDriftFixture()
		files["drift/ok/0001-the-first-decision.md"] = testDocument("The first decision", config.StatusAccepted)
		root := testCorpus(t, files)

		findings, _, err := Fixtures(testFixtureOptions(testConfig(testDriftRule()), root, root))
		if err != nil {
			t.Fatalf("Fixtures: %v", err)
		}

		f := assertFinding(t, findings, FindingFixtureMismatch, "drift", model.SeverityError, "where it must not")
		if !strings.HasSuffix(filepath.ToSlash(f.Location.Path), "0001-the-first-decision.md") {
			t.Errorf("location = %q, want the document that fired", f.Location.Path)
		}
	})
}

func TestMissingFixture(t *testing.T) {
	root := testCorpus(t, map[string]string{
		"drift/ruleid/0001-a-decision.md": testDocument("A decision", config.StatusAccepted),
	})
	cfg := testConfig(testDriftRule())

	findings, _, err := Fixtures(testFixtureOptions(cfg, root, root))
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}

	f := assertFinding(t, findings, FindingMissingFixture, "drift", model.SeverityWarn, "has no ok fixture")
	if f.Fix != "run docdag new --fixture drift" {
		t.Errorf("fix = %q, want the command that writes one", f.Fix)
	}
}

// TestPresetRulesAreExempt keeps a corpus that only names a preset from being
// asked for fixtures DocDag itself ships.
func TestPresetRulesAreExempt(t *testing.T) {
	root := testCorpus(t, map[string]string{"unused/ok/0001-a-decision.md": testDocument("A decision", config.StatusAccepted)})

	findings, _, err := Fixtures(testFixtureOptions(config.ADRPreset(), root, root))
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}

	assertNoFinding(t, findings, FindingMissingFixture)
}

// TestNeverFiredDowngrade is the interplay the two layers answer together: the
// corpus says the rule is silent, the fixture says it can speak, and the finding
// falls to a fact.
func TestNeverFiredDowngrade(t *testing.T) {
	fixtures := testCorpus(t, testDriftFixture())
	corpus := testCorpus(t, testTwoDecisions())
	cfg := testConfig(testDriftRule())
	opts := testFixtureOptions(cfg, corpus, fixtures)
	opts.Corpus = testCorpusGraph(t, cfg, corpus)

	t.Run("without a fixture the silence is a warning", func(t *testing.T) {
		quiet := opts
		quiet.Fixtures = ""

		findings, err := Run(quiet)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		assertFinding(t, findings, FindingNeverFired, "drift", model.SeverityWarn, "fired on 0 of 2 documents")
	})

	t.Run("with a passing fixture it is a fact", func(t *testing.T) {
		findings, err := Run(opts)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		f := assertFinding(t, findings, FindingNeverFired, "drift", model.SeverityInfo, "its ruleid fixture shows it can fire")
		if f.Fix != "" {
			t.Errorf("fix = %q, want nothing to do about a rule that works", f.Fix)
		}
	})
}

// TestShippedFixtures runs the fixtures DocDag ships for its own presets. They
// are executable documentation: each rule's ruleid/ shows what it reports and
// its ok/ shows what it does not, and a preset edit that breaks either fails
// here rather than in somebody's repository.
func TestShippedFixtures(t *testing.T) {
	for _, preset := range []string{config.PresetADR, config.PresetSpec} {
		t.Run(preset+" fixtures pass", func(t *testing.T) {
			root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "lint", preset))
			if err != nil {
				t.Fatalf("resolve the fixtures: %v", err)
			}
			options := config.Options{Root: root, ConfigPath: filepath.Join(root, config.DefaultConfigFile)}
			// A single-kind corpus discovers its documents directory, and these
			// fixtures are not one: the fixture directory is the corpus.
			if preset == config.PresetADR {
				options.Dir = root
			}
			cfg, err := config.Resolve(options)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			findings, firable, err := Fixtures(testFixtureOptions(cfg, root, root))
			if err != nil {
				t.Fatalf("Fixtures: %v", err)
			}

			if len(findings) > 0 {
				t.Errorf("findings = %s, want every shipped fixture to pass", formatFindings(findings))
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatalf("read %s: %v", root, err)
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if !firable[entry.Name()] {
					t.Errorf("%s: the ruleid corpus fired nothing, want it to show the check can fire", entry.Name())
				}
			}
		})
	}
}

// TestShippedFixturesCoverThePresets keeps the shipped fixtures honest about
// what they cover: every rule of each preset, and the structural checks the
// spec preset's own declarations turn on.
func TestShippedFixturesCoverThePresets(t *testing.T) {
	covered := func(preset string) map[string]bool {
		root := filepath.Join("..", "..", "testdata", "lint", preset)
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		names := map[string]bool{}
		for _, entry := range entries {
			if entry.IsDir() {
				names[entry.Name()] = true
			}
		}
		return names
	}

	for _, preset := range []string{config.PresetADR, config.PresetSpec} {
		cfg, err := config.Preset(preset)
		if err != nil {
			t.Fatalf("Preset(%q): %v", preset, err)
		}
		names := covered(preset)
		for _, rule := range cfg.Rules {
			if !names[rule.Name] {
				t.Errorf("%s: no fixture for the rule %q", preset, rule.Name)
			}
		}
	}

	structural := covered(config.PresetSpec)
	for _, name := range []string{model.RuleStaleTarget, model.RuleModalityConflict, model.RuleExceptsStrict} {
		if !structural[name] {
			t.Errorf("spec: no fixture for the structural check %q", name)
		}
	}
}
