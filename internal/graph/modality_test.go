package graph

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

// The subject the conflict tests are written against, and a second one for the
// pairing that must not happen.
const (
	testTopic      = "topic/grading"
	testOtherTopic = "topic/reporting"
)

// testClause is one clause of a spec-preset corpus: a modality, the subjects it
// speaks to, and whatever else the test is about.
func testClause(id, modality string, about []string, extra map[string]any) *parse.Document {
	frontmatter := map[string]any{
		config.FieldModality:      modality,
		config.DefaultStatusField: config.StatusAccepted,
		config.EdgeAbout.String(): testAnyList(about),
	}
	for key, value := range extra {
		frontmatter[key] = value
	}
	return testKindDoc(config.KindClause, id, frontmatter)
}

// testEnforcing is the conformance test that gives a clause its force, so a
// strict clause is in the binding set the conflict check reads.
func testEnforcing(clause string) *parse.Document {
	return testKindDoc(config.KindConform, "conform/"+strings.ToLower(clause),
		map[string]any{config.EdgeEnforces.String(): []any{clause}})
}

func testTopicDoc(id string) *parse.Document {
	return testKindDoc(config.KindTopic, id, map[string]any{config.KeyTitle: "The subject " + id})
}

func testAnyList(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// testPair builds the two-clause corpus the table is checked against: both
// clauses speak to one subject and both are enforced, so whether they are
// binding never depends on the modality under test.
func testPair(modalityA, modalityB string, extraA, extraB map[string]any) (*model.Graph, config.Config) {
	cfg := config.SpecPreset()
	docs := []*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", modalityA, []string{testTopic}, extraA),
		testClause("UZ-V-002", modalityB, []string{testTopic}, extraB),
		testEnforcing("UZ-V-001"),
		testEnforcing("UZ-V-002"),
	}
	return Build(docs, cfg), cfg
}

// TestModalityConflictTable pins ADR-0003 D3's grid, every ordered cell of it,
// against the check itself rather than against the predicate underneath: the
// table is the decision, and a test that read it the same way the code does
// would agree with a wrong reading.
func TestModalityConflictTable(t *testing.T) {
	// One row per modality of A, one column per modality of B, in the order the
	// vocabulary declares them: '-' is no finding, 'w' a weak conflict an
	// exception can answer, 'S' the strong one nothing can.
	//
	//                   MUST  MUST_NOT  SHOULD  SHOULD_NOT  MAY
	grid := map[string]string{
		config.ModalityMUST:      "-S-w-",
		config.ModalityMUSTNOT:   "S-w-w",
		config.ModalitySHOULD:    "-w-w-",
		config.ModalitySHOULDNOT: "w-w-w",
		config.ModalityMAY:       "-w-w-",
	}
	for _, modalityA := range config.Modalities {
		for column, modalityB := range config.Modalities {
			cell := grid[modalityA][column]
			t.Run(modalityA+" against "+modalityB, func(t *testing.T) {
				g, cfg := testPair(modalityA, modalityB, nil, nil)

				got := CheckModalityConflicts(g, cfg, testAsOf)

				if cell == '-' {
					if len(got) != 0 {
						t.Fatalf("findings = %+v, want none: the table leaves this pair alone", got)
					}
					return
				}
				f := testAssertSingleFinding(t, got, model.RuleModalityConflict, model.SeverityError, "UZ-V-001")
				want := "is " + modalityA + " and UZ-V-002 is " + modalityB + " about " + testTopic
				if f.Detail != want {
					t.Errorf("detail = %q, want %q", f.Detail, want)
				}
				if strong := strings.Contains(f.Fix, "strict rule"); strong != (cell == 'S') {
					t.Errorf("fix = %q, want the %s form", f.Fix, map[bool]string{true: "strong", false: "weak"}[cell == 'S'])
				}
			})
		}
	}
}

func TestModalityConflictsAreAboutOneSubject(t *testing.T) {
	t.Run("two clauses speaking to different subjects never meet", func(t *testing.T) {
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testTopicDoc(testOtherTopic),
			testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULDNOT, []string{testOtherTopic}, nil),
		}, cfg)

		if got := CheckModalityConflicts(g, cfg, testAsOf); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: a conflict is about one subject", got)
		}
	})

	t.Run("a pair sharing two subjects disagrees once, over both", func(t *testing.T) {
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testTopicDoc(testOtherTopic),
			testClause("UZ-V-001", config.ModalityMAY, []string{testTopic, testOtherTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULDNOT, []string{testOtherTopic, testTopic}, nil),
		}, cfg)

		f := testAssertSingleFinding(t, CheckModalityConflicts(g, cfg, testAsOf),
			model.RuleModalityConflict, model.SeverityError, "UZ-V-001")
		if !strings.HasSuffix(f.Detail, "about "+testTopic+", "+testOtherTopic) {
			t.Errorf("detail = %q, want both subjects named once", f.Detail)
		}
		// The other clause and each shared subject, so a reader opens one file
		// and finds every document the pairing used.
		if len(f.Related) != 3 {
			t.Errorf("related = %+v, want the other clause and the two subjects", f.Related)
		}
	})

	t.Run("a clause that does not bind states nothing to collide with", func(t *testing.T) {
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULDNOT, []string{testTopic}, map[string]any{
				config.DefaultStatusField: config.StatusSuperseded,
			}),
			testClause("UZ-V-003", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.EdgeSupersedes.String(): []any{map[string]any{"ref": "UZ-V-002", "reason": "vocabulary"}},
			}),
		}, cfg)

		if got := CheckModalityConflicts(g, cfg, testAsOf); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: a superseded clause is not in force", got)
		}
	})

	t.Run("a clause naming no subject is a cardinality finding, not a silent one", func(t *testing.T) {
		// min_outbound is read over the kinds the edge names, so it reaches a
		// clause with no about key at all — and reaches nothing else.
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testKindDoc(config.KindClause, "UZ-V-001", map[string]any{
				config.FieldModality:      config.ModalityMAY,
				config.DefaultStatusField: config.StatusAccepted,
			}),
		}, cfg)

		got := CheckCardinality(g, cfg)

		f := testAssertSingleFinding(t, got, model.RuleCardinality, model.SeverityError, "UZ-V-001")
		if want := "0 outbound about edges fall short of min_outbound 1"; f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
	})
}

func TestModalityConflictSuppression(t *testing.T) {
	except := func(target string) map[string]any {
		return map[string]any{config.EdgeExcepts.String(): []any{
			map[string]any{"ref": target, config.AttrScope: "only where the run is calibrated"},
		}}
	}

	t.Run("an exception from the specific clause suppresses a weak conflict", func(t *testing.T) {
		g, cfg := testPair(config.ModalityMAY, config.ModalitySHOULDNOT, except("UZ-V-002"), nil)

		f := testAssertSingleFinding(t, CheckModalityConflicts(g, cfg, testAsOf),
			model.RuleModalityConflict, model.SeverityError, "UZ-V-001")
		if !f.Suppressed {
			t.Fatalf("finding = %+v, want it suppressed: the corpus records the exception", f)
		}
		want := "suppressed by excepts UZ-V-001 -> UZ-V-002 (scope: only where the run is calibrated)"
		if !strings.HasSuffix(f.Detail, want) {
			t.Errorf("detail = %q, want it to end with %q", f.Detail, want)
		}
	})

	t.Run("the direction of the exception does not matter to the pair", func(t *testing.T) {
		g, cfg := testPair(config.ModalityMAY, config.ModalitySHOULDNOT, nil, except("UZ-V-001"))

		f := testAssertSingleFinding(t, CheckModalityConflicts(g, cfg, testAsOf),
			model.RuleModalityConflict, model.SeverityError, "UZ-V-001")
		if !f.Suppressed || !strings.Contains(f.Detail, "excepts UZ-V-002 -> UZ-V-001") {
			t.Errorf("finding = %+v, want it suppressed by the exception the other clause records", f)
		}
	})

	t.Run("nothing suppresses a strong conflict", func(t *testing.T) {
		g, cfg := testPair(config.ModalityMUST, config.ModalityMUSTNOT, except("UZ-V-002"), nil)

		f := testAssertSingleFinding(t, CheckModalityConflicts(g, cfg, testAsOf),
			model.RuleModalityConflict, model.SeverityError, "UZ-V-001")
		if f.Suppressed {
			t.Fatalf("finding = %+v, want it standing: a strict rule cannot be defeated", f)
		}
		if want := "revise one modality: a strict rule cannot be defeated"; f.Fix != want {
			t.Errorf("fix = %q, want %q", f.Fix, want)
		}
	})

	t.Run("the weak fix names the exception to record and where to record it", func(t *testing.T) {
		g, cfg := testPair(config.ModalityMAY, config.ModalitySHOULDNOT, nil, nil)

		f := testAssertSingleFinding(t, CheckModalityConflicts(g, cfg, testAsOf),
			model.RuleModalityConflict, model.SeverityError, "UZ-V-001")
		want := "declare excepts: UZ-V-002 in UZ-V-001 with scope:, or revise one modality"
		if f.Fix != want {
			t.Errorf("fix = %q, want %q", f.Fix, want)
		}
		// Suggest is a pass over a finished report, and the remedy it would
		// write for a pair — which document is the other one — is not
		// recoverable from the prose, so the check's own fix stands.
		if got := Suggest(CheckModalityConflicts(g, cfg, testAsOf), g, cfg, testAsOf); got[0].Fix != want {
			t.Errorf("fix after Suggest = %q, want %q", got[0].Fix, want)
		}
	})

	t.Run("a summary leaves a suppressed finding out, and validate reports it", func(t *testing.T) {
		g, cfg := testPair(config.ModalityMAY, config.ModalitySHOULDNOT, except("UZ-V-002"), nil)

		findings := Validate(g, cfg, time.Time{})

		if !slices.Contains(testRuleNames(findings), model.RuleModalityConflict) {
			t.Fatalf("rules = %v, want the suppressed conflict among them", testRuleNames(findings))
		}
		errors := 0
		for _, f := range findings {
			if f.Severity == model.SeverityError && !f.Suppressed {
				errors++
			}
		}
		if got := Summarize(g, findings); got.Errors != errors {
			t.Errorf("summary errors = %d, want %d: an answered conflict is not an open failure", got.Errors, errors)
		}
	})
}

func TestCheckExceptsStrict(t *testing.T) {
	except := map[string]any{config.EdgeExcepts.String(): []any{
		map[string]any{"ref": "UZ-V-002", config.AttrScope: "only on Tuesdays"},
	}}

	for _, modality := range []string{config.ModalityMUST, config.ModalityMUSTNOT} {
		t.Run("an exception against a "+modality+" cannot be recorded", func(t *testing.T) {
			g, cfg := testPair(config.ModalityMAY, modality, except, nil)

			got := CheckExceptsStrict(g, cfg)

			f := testAssertSingleFinding(t, got, model.RuleExceptsStrict, model.SeverityError, "UZ-V-001")
			want := "excepts targets UZ-V-002, which is " + modality + " and cannot be defeated"
			if f.Detail != want {
				t.Errorf("detail = %q, want %q", f.Detail, want)
			}
			// The line to change is the one that declares the exception.
			if f.Location.Line != g.Nodes["UZ-V-001"].KeyLines[config.EdgeExcepts.String()] {
				t.Errorf("location = %+v, want the excepts key's line", f.Location)
			}
		})
	}

	for _, modality := range []string{config.ModalitySHOULD, config.ModalitySHOULDNOT, config.ModalityMAY} {
		t.Run("an exception against a "+modality+" is what the edge is for", func(t *testing.T) {
			g, cfg := testPair(config.ModalityMAY, modality, except, nil)

			if got := CheckExceptsStrict(g, cfg); len(got) != 0 {
				t.Fatalf("findings = %+v, want none: a defeasible rule is what a defeater defeats", got)
			}
		})
	}

	t.Run("a corpus that declares no exceptions is asked nothing", func(t *testing.T) {
		cfg := config.ADRPreset()
		g := Build([]*parse.Document{testDoc("0001", map[string]any{"status": config.StatusAccepted}, "")}, cfg)

		if got := CheckExceptsStrict(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
		if got := CheckModalityConflicts(g, cfg, testAsOf); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the adr preset declares no subjects to disagree about", got)
		}
	})
}

func TestSpecPresetInteropRules(t *testing.T) {
	t.Run("a MAY that names no interoperation requirement warns", func(t *testing.T) {
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, nil),
		}, cfg)

		f := testAssertSingleFinding(t, EvalRules(g, cfg, testAsOf), model.RuleMayWithoutInterop, model.SeverityWarn, "UZ-V-001")
		if !strings.Contains(f.Detail, "MUST clause") {
			t.Errorf("detail = %q, want it to name what the edge has to point at", f.Detail)
		}
	})

	t.Run("an interoperation requirement that is not a MUST is an error", func(t *testing.T) {
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, map[string]any{
				config.EdgeInterop.String(): []any{"UZ-V-002"},
			}),
			testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, nil),
		}, cfg)

		testAssertSingleFinding(t, EvalRules(g, cfg, testAsOf), model.RuleInteropNotMust, model.SeverityError, "UZ-V-001")
	})

	t.Run("a MAY leaning on a MUST reports neither", func(t *testing.T) {
		cfg := config.SpecPreset()
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, map[string]any{
				config.EdgeInterop.String(): []any{"UZ-V-002"},
			}),
			testClause("UZ-V-002", config.ModalityMUST, []string{testTopic}, nil),
			testEnforcing("UZ-V-002"),
		}, cfg)

		for _, rule := range []string{model.RuleMayWithoutInterop, model.RuleInteropNotMust} {
			if got := testFindingsFor(EvalRules(g, cfg, testAsOf), rule); len(got) != 0 {
				t.Errorf("%s = %+v, want none", rule, got)
			}
		}
	})
}

func TestSuppressedConflictsCarryNoRemedy(t *testing.T) {
	// The remedy for an unanswered weak conflict is the edge this pair already
	// has, so a fix line would tell the reader to write what is on the line
	// above it.
	g, cfg := testPair(config.ModalityMAY, config.ModalitySHOULDNOT, map[string]any{
		config.EdgeExcepts.String(): []any{
			map[string]any{"ref": "UZ-V-002", config.AttrScope: "only where the run is calibrated"},
		},
	}, nil)

	got := Suggest(CheckModalityConflicts(g, cfg, testAsOf), g, cfg, testAsOf)

	if len(got) != 1 || got[0].Fix != "" {
		t.Fatalf("findings = %+v, want the one suppressed conflict, without a fix", got)
	}
}
