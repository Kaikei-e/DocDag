package graph

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// testDayOf reads a day the way frontmatter writes one, so a test names the
// date it means rather than a time.Time nobody can read.
func testDayOf(t *testing.T, day string) time.Time {
	t.Helper()
	parsed, err := time.Parse(config.AttrDateLayout, day)
	if err != nil {
		t.Fatalf("parse %q: %v", day, err)
	}
	return parsed
}

// testSupersedes is the entry a clause declares its predecessor under: the
// reference and the reason the preset requires beside it.
func testSupersedes(ref string) []any {
	return []any{map[string]any{config.EdgeRefKey: ref, config.AttrReason: "recurrence"}}
}

// testDeviation is one recorded departure from a clause, optionally expiring.
func testDeviation(id, clause, date, expires string) *parse.Document {
	frontmatter := map[string]any{
		config.DefaultStatusField:        config.StatusAccepted,
		config.KeyDate:                   date,
		config.EdgeDeviatesFrom.String(): []any{clause},
	}
	if expires != "" {
		frontmatter[config.FieldExpires] = expires
	}
	return testKindDoc(config.KindDeviation, id, frontmatter)
}

func TestInForceReadsTheDayAgainstAClosedOpenInterval(t *testing.T) {
	cfg := config.SpecPreset()
	g := Build([]*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, map[string]any{
			config.FieldInForceFrom:  "2026-06-01",
			config.FieldInForceUntil: "2026-09-01",
		}),
		// A clause that writes neither day has always been in force and never
		// stops, which is what an undated standard says.
		testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, nil),
		// A conformance test's kind declares no period at all.
		testEnforcing("UZ-V-001"),
	}, cfg)

	tests := []struct {
		day     string
		dated   bool
		undated bool
	}{
		{day: "2026-05-31", dated: false, undated: true},
		// The day it begins on is inside the interval and the day it ends on is
		// outside it: [from, until).
		{day: "2026-06-01", dated: true, undated: true},
		{day: "2026-08-31", dated: true, undated: true},
		{day: "2026-09-01", dated: false, undated: true},
	}
	for _, tt := range tests {
		t.Run(tt.day, func(t *testing.T) {
			periods := EvalPeriods(g, cfg, testDayOf(t, tt.day))

			if got := periods.InForce("UZ-V-001"); got != tt.dated {
				t.Errorf("InForce(UZ-V-001) on %s = %v, want %v", tt.day, got, tt.dated)
			}
			if got := periods.InForce("UZ-V-002"); got != tt.undated {
				t.Errorf("InForce(UZ-V-002) on %s = %v, want %v", tt.day, got, tt.undated)
			}
			// A kind that declares no period has documents that are in force
			// whatever day is asked about, which is what makes a corpus without
			// periods answer as it always did.
			if !periods.InForce("conform/uz-v-001") {
				t.Error("InForce(conform/uz-v-001) = false, want a kind without a period always in force")
			}
			if periods.Declared("conform/uz-v-001") {
				t.Error("Declared(conform/uz-v-001) = true, want no period on a kind that declares none")
			}
		})
	}
}

func TestInForceIsTrueWhereNoKindDeclaresAPeriod(t *testing.T) {
	// R6: the ADR preset declares no period, so in_force answers true for every
	// document and a rule that reads it changes nothing.
	cfg := config.ADRPreset()
	g := testGraph([]*model.Node{testNode("0001", "accepted")}, nil, nil)

	periods := EvalPeriods(g, cfg, testDayOf(t, "2026-06-01"))

	if !periods.InForce("0001") || periods.Declared("0001") {
		t.Errorf("periods = %+v, want a corpus without periods always in force", periods)
	}
	if !MatchCondition(g, cfg, config.Condition{
		Attr: map[string]config.AttrCondition{config.AttrInForce: testAttrEq(config.ProjectionTrue)},
	}, "0001", testDayOf(t, "2026-06-01")) {
		t.Error("in_force does not read true on a corpus without periods, want it to")
	}
}

func TestPeriodInvalid(t *testing.T) {
	cfg := config.SpecPreset()

	t.Run("a day that is not a date", func(t *testing.T) {
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.FieldInForceFrom: "next quarter",
			}),
		}, cfg)

		got := CheckPeriods(g, cfg, testDayOf(t, "2026-06-01"))

		f := testAssertSingleFinding(t, got, model.RulePeriodInvalid, model.SeverityError, "UZ-V-001")
		if !strings.Contains(f.Detail, "YYYY-MM-DD") {
			t.Errorf("detail = %q, want it to name the spelling a day takes", f.Detail)
		}
		// The finding points at the key that holds the day, which is the line
		// the reader has to rewrite.
		if want := g.Nodes["UZ-V-001"].KeyLines[config.FieldInForceFrom]; f.Location.Line != want {
			t.Errorf("line = %d, want %d, the line the day is written on", f.Location.Line, want)
		}
		// An unreadable day is no day at all, so the clause has not stopped
		// being in force on account of it.
		if !EvalPeriods(g, cfg, testDayOf(t, "2026-06-01")).InForce("UZ-V-001") {
			t.Error("InForce = false, want a day nobody can read to constrain nothing")
		}
	})

	t.Run("an interval that ends before it begins", func(t *testing.T) {
		g := Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.FieldInForceFrom:  "2026-06-01",
				config.FieldInForceUntil: "2026-01-01",
			}),
		}, cfg)

		got := CheckPeriods(g, cfg, testDayOf(t, "2026-06-01"))

		f := testAssertSingleFinding(t, got, model.RulePeriodInvalid, model.SeverityError, "UZ-V-001")
		if !strings.Contains(f.Detail, "is before") {
			t.Errorf("detail = %q, want it to say which day comes first", f.Detail)
		}
	})

	t.Run("a corpus whose kinds declare no period sees none of it", func(t *testing.T) {
		cfg := config.ADRPreset()
		g := testGraph([]*model.Node{testNodeAttrs("0001", "accepted", map[string]any{
			"in_force_until": "not a date",
		})}, nil, nil)

		if got := CheckPeriods(g, cfg, testDayOf(t, "2026-06-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: nothing declares the key as a period", got)
		}
	})
}

func TestDerivedUntilComesFromTheAcceptedSuccessors(t *testing.T) {
	cfg := config.SpecPreset()
	corpus := func(successorStatus string) *model.Graph {
		return Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.DefaultStatusField:          successorStatus,
				config.FieldInForceFrom:            "2026-06-01",
				config.EdgeSupersedes.String():     testSupersedes("UZ-V-001"),
				config.EdgeCounterexample.String(): []any{},
			}),
		}, cfg)
	}

	t.Run("an accepted successor ends its predecessor the day it begins", func(t *testing.T) {
		g := corpus(config.StatusAccepted)

		for _, tt := range []struct {
			day  string
			want bool
		}{{day: "2026-05-31", want: true}, {day: "2026-06-01", want: false}} {
			if got := EvalPeriods(g, cfg, testDayOf(t, tt.day)).InForce("UZ-V-001"); got != tt.want {
				t.Errorf("InForce(UZ-V-001) on %s = %v, want %v", tt.day, got, tt.want)
			}
		}
		// The derived day is never written back: it is an answer about the
		// corpus, and a corpus that stored it would have to keep it true.
		if _, written := EvalPeriods(g, cfg, testDayOf(t, "2026-06-01")).Ended("UZ-V-001"); written {
			t.Error("Ended reports a day the document wrote, want the derived one left out of the frontmatter")
		}
	})

	t.Run("a successor nobody accepted derives nothing", func(t *testing.T) {
		g := corpus(config.StatusTrial)

		if !EvalPeriods(g, cfg, testDayOf(t, "2026-09-01")).InForce("UZ-V-001") {
			t.Error("InForce(UZ-V-001) = false, want a clause its trial successor has not replaced")
		}
	})

	t.Run("a withdrawn successor gives the period back", func(t *testing.T) {
		// Governatori and Rotolo's revocation of an abrogation: the successor
		// stops being accepted and the predecessor is open-ended again, with
		// nothing rewritten but the successor's own status.
		g := corpus(config.StatusWithdrawn)

		if !EvalPeriods(g, cfg, testDayOf(t, "2026-09-01")).InForce("UZ-V-001") {
			t.Error("InForce(UZ-V-001) = false, want the withdrawal to take the derived end away")
		}
	})
}

func TestPeriodConflict(t *testing.T) {
	cfg := config.SpecPreset()
	g := Build([]*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, map[string]any{
			config.FieldInForceUntil: "2026-12-31",
		}),
		testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, map[string]any{
			config.FieldInForceFrom:        "2026-06-01",
			config.EdgeSupersedes.String(): testSupersedes("UZ-V-001"),
		}),
	}, cfg)

	got := CheckPeriods(g, cfg, testDayOf(t, "2026-01-01"))

	f := testAssertSingleFinding(t, got, model.RulePeriodConflict, model.SeverityError, "UZ-V-001")
	for _, want := range []string{"2026-12-31", "2026-06-01", "UZ-V-002"} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("detail = %q, want it to name %q", f.Detail, want)
		}
	}
	if len(f.Related) != 1 || f.Related[0].Path != g.Nodes["UZ-V-002"].Path {
		t.Errorf("related = %+v, want the successor it disagrees with", f.Related)
	}
	// Which of the two days is wrong is not a question the graph answers, so
	// there is no fix — the same silence derived_conflict keeps.
	if fixed := Suggest([]model.Finding{f}, g, cfg, testDayOf(t, "2026-01-01")); fixed[0].Fix != "" {
		t.Errorf("fix = %q, want none: DocDag does not guess which day is right", fixed[0].Fix)
	}
	// The written day is the one that counts, so the clause is still in force
	// on the day the successor begins.
	if !EvalPeriods(g, cfg, testDayOf(t, "2026-06-01")).InForce("UZ-V-001") {
		t.Error("InForce = false, want the explicit day to be the one that decides")
	}
}

func TestExpiredDeviation(t *testing.T) {
	cfg := config.SpecPreset()
	g := Build([]*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, nil),
		testDeviation("dev-0001", "UZ-V-001", "2026-01-01", "2026-06-01"),
		testDeviation("dev-0002", "UZ-V-001", "2026-01-01", ""),
	}, cfg)

	t.Run("before the day it names, nothing is reported", func(t *testing.T) {
		if got := CheckPeriods(g, cfg, testDayOf(t, "2026-05-31")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the departure is still recorded", got)
		}
	})

	t.Run("on the day it expires, the record is past its own deadline", func(t *testing.T) {
		got := CheckPeriods(g, cfg, testDayOf(t, "2026-06-01"))

		f := testAssertSingleFinding(t, got, model.RuleExpiredDeviation, model.SeverityWarn, "dev-0001")
		if !strings.Contains(f.Detail, "2026-06-01") || !strings.Contains(f.Detail, config.StatusAccepted) {
			t.Errorf("detail = %q, want the day and the status that disagree", f.Detail)
		}
		if want := g.Nodes["dev-0001"].KeyLines[config.FieldExpires]; f.Location.Line != want {
			t.Errorf("line = %d, want %d, the line the expiry is written on", f.Location.Line, want)
		}
	})

	t.Run("a departure that names no day never expires", func(t *testing.T) {
		for _, f := range CheckPeriods(g, cfg, testDayOf(t, "2030-01-01")) {
			if f.ID == "dev-0002" {
				t.Fatalf("finding = %+v, want an open-ended departure left alone", f)
			}
		}
	})
}

func TestOutOfForceStatementsLoseTheirWeight(t *testing.T) {
	cfg := config.SpecPreset()
	// Five departures from one clause, which is the threshold
	// deviation_pressure reports at — and one of them expires in June.
	docs := []*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, nil),
		testDeviation("dev-0005", "UZ-V-001", "2026-01-01", "2026-06-01"),
	}
	for _, id := range []string{"dev-0001", "dev-0002", "dev-0003", "dev-0004"} {
		docs = append(docs, testDeviation(id, "UZ-V-001", "2026-01-01", ""))
	}
	g := Build(docs, cfg)
	pressure := testRuleNamed(t, cfg, model.RuleDeviationPressure)

	t.Run("every departure counts while they are all recorded", func(t *testing.T) {
		got := EvalRule(g, cfg, pressure, testDayOf(t, "2026-05-31"))

		testAssertSingleFinding(t, got, model.RuleDeviationPressure, model.SeverityWarn, "UZ-V-001")
	})

	t.Run("an expired departure stops counting", func(t *testing.T) {
		// The threshold is five and one of them has left force, so the clause
		// is under pressure from four: an expired departure is a record of
		// something that was, not a statement about what is.
		if got := EvalRule(g, cfg, pressure, testDayOf(t, "2026-06-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: four in-force departures are under the threshold", got)
		}
	})

	t.Run("a corpus without periods counts every edge, whatever the day", func(t *testing.T) {
		// R6 again, at the other end: the ADR preset declares no period, so no
		// edge ever loses its weight and a degree threshold answers the same on
		// every day.
		cfg := config.ADRPreset()
		cfg.Rules = []config.Rule{{
			Name: "fan_in", Severity: model.SeverityWarn,
			When:    config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeDependsOn.String()}},
			Message: "something depends on it",
		}}
		g := testGraph(
			[]*model.Node{testNode("0001", "accepted"), testNode("0002", "superseded")},
			[]model.Edge{testEdge("0002", "0001", config.EdgeDependsOn)}, nil)

		for _, day := range []string{"2020-01-01", "2030-01-01"} {
			if got := EvalRules(g, cfg, testDayOf(t, day)); len(got) != 1 {
				t.Errorf("findings on %s = %+v, want the one the edge always earns", day, got)
			}
		}
	})
}

func TestAnExpiredExceptionStopsSuppressing(t *testing.T) {
	// The defeater is a statement the clause makes, so it defeats nothing once
	// the clause has left force. Binding is read from a projection that does
	// not itself consult the day, so the pair still meets and the conflict is
	// reported rather than quietly disappearing with the exception.
	cfg := config.SpecPreset()
	cfg.Binding = config.ProjectionEnforced
	docs := []*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, map[string]any{
			config.FieldInForceUntil:    "2026-06-01",
			config.EdgeExcepts.String(): []any{map[string]any{config.EdgeRefKey: "UZ-V-002", config.AttrScope: "while it is calibrated"}},
		}),
		testClause("UZ-V-002", config.ModalitySHOULDNOT, []string{testTopic}, nil),
		testEnforcing("UZ-V-001"),
		testEnforcing("UZ-V-002"),
	}
	g := Build(docs, cfg)

	t.Run("while the exception is in force it answers the pair", func(t *testing.T) {
		conflicts := ModalityConflicts(g, cfg, testDayOf(t, "2026-05-31"))

		if len(conflicts) != 1 || !conflicts[0].Suppressed {
			t.Fatalf("conflicts = %+v, want the one the exception answers", conflicts)
		}
	})

	t.Run("once it has expired the conflict stands again", func(t *testing.T) {
		conflicts := ModalityConflicts(g, cfg, testDayOf(t, "2026-06-01"))

		if len(conflicts) != 1 || conflicts[0].Suppressed {
			t.Fatalf("conflicts = %+v, want the pair reported once the exception has left force", conflicts)
		}
	})
}

func TestLeafOfReadsTheDay(t *testing.T) {
	// ADR-0002's "current leaf" under ADR-0005: a conformance test may point at
	// the clause a successor has not taken over from yet, and must not point at
	// one an in-force accepted successor replaced.
	cfg := config.SpecPreset()
	corpus := func(status, from string) *model.Graph {
		return Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.DefaultStatusField:      status,
				config.FieldInForceFrom:        from,
				config.EdgeSupersedes.String(): testSupersedes("UZ-V-001"),
			}),
			testEnforcing("UZ-V-001"),
		}, cfg)
	}

	t.Run("a successor nobody has accepted leaves the predecessor the leaf", func(t *testing.T) {
		g := corpus(config.StatusTrial, "")

		if got := CheckTargets(g, cfg, testDayOf(t, "2026-09-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: a trial successor has replaced nothing", got)
		}
	})

	t.Run("an accepted successor that has not begun leaves it too", func(t *testing.T) {
		g := corpus(config.StatusAccepted, "2026-12-01")

		if got := CheckTargets(g, cfg, testDayOf(t, "2026-09-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the successor takes over in December", got)
		}
	})

	t.Run("once the successor is in force the test is pointing at the wrong clause", func(t *testing.T) {
		g := corpus(config.StatusAccepted, "2026-12-01")

		got := CheckTargets(g, cfg, testDayOf(t, "2026-12-01"))

		f := testAssertSingleFinding(t, got, model.RuleStaleTarget, model.SeverityError, "conform/uz-v-001")
		// The remedy walks the same lineage the check read, so it names the
		// clause that is actually in force.
		fixed := Suggest([]model.Finding{f}, g, cfg, testDayOf(t, "2026-12-01"))
		if fixed[0].Fix != "did you mean UZ-V-002?" {
			t.Errorf("fix = %q, want the successor that has taken over", fixed[0].Fix)
		}
	})

	t.Run("resolve stops where the lineage stops", func(t *testing.T) {
		g := corpus(config.StatusAccepted, "2026-12-01")

		for _, tt := range []struct {
			day  string
			want []model.ID
		}{
			{day: "2026-09-01", want: testIDs("UZ-V-001")},
			{day: "2026-12-01", want: testIDs("UZ-V-002")},
		} {
			got, err := ResolveAt(g, cfg, "UZ-V-001", config.EdgeSupersedes, testDayOf(t, tt.day))
			if err != nil {
				t.Fatalf("ResolveAt on %s: %v", tt.day, err)
			}
			testAssertIDs(t, "resolved on "+tt.day, got, tt.want)
		}
	})
}

func TestTimeDependentStatusRules(t *testing.T) {
	cfg := config.SpecPreset()
	corpus := func(predecessor, successor, from string) *model.Graph {
		return Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.DefaultStatusField: predecessor,
			}),
			testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.DefaultStatusField:      successor,
				config.FieldInForceFrom:        from,
				config.EdgeSupersedes.String(): testSupersedes("UZ-V-001"),
			}),
		}, cfg)
	}
	fired := func(t *testing.T, g *model.Graph, day string) []string {
		t.Helper()
		out := []string{}
		for _, f := range EvalRules(g, cfg, testDayOf(t, day)) {
			if slices.Contains([]string{
				model.RuleStatusDrift, model.RulePendingSuccessor, model.RulePrematureSuperseded,
			}, f.Rule) {
				out = append(out, f.Rule+" "+f.ID.String())
			}
		}
		slices.Sort(out)
		return out
	}

	tests := []struct {
		name                              string
		predecessor, successor, from, day string
		want                              []string
	}{
		{
			name:        "a successor in trial is a change in flight",
			predecessor: config.StatusAccepted, successor: config.StatusTrial, day: "2026-09-01",
			want: []string{"pending_successor UZ-V-001"},
		},
		{
			name:        "an accepted successor dated ahead is one too",
			predecessor: config.StatusAccepted, successor: config.StatusAccepted,
			from: "2026-12-01", day: "2026-09-01",
			want: []string{"pending_successor UZ-V-001"},
		},
		{
			name:        "once it is in force the predecessor has to say so",
			predecessor: config.StatusAccepted, successor: config.StatusAccepted,
			from: "2026-12-01", day: "2026-12-01",
			want: []string{"status_drift UZ-V-001"},
		},
		{
			name:        "the pair in its settled state reports nothing",
			predecessor: config.StatusSuperseded, successor: config.StatusAccepted,
			from: "2026-12-01", day: "2026-12-01",
			want: []string{},
		},
		{
			name:        "superseded before anything took over is a gap",
			predecessor: config.StatusSuperseded, successor: config.StatusTrial, day: "2026-09-01",
			want: []string{"premature_superseded UZ-V-001"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := corpus(tt.predecessor, tt.successor, tt.from)

			if got := fired(t, g, tt.day); !slices.Equal(got, tt.want) {
				t.Errorf("fired = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("the adr preset keeps the time-independent reading", func(t *testing.T) {
		// R6: the same shape under the adr preset, which declares no period,
		// reports status_drift the moment a successor exists.
		cfg := config.ADRPreset()
		g := testGraph(
			[]*model.Node{testNode("0001", "accepted"), testNode("0002", "proposed")},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)}, nil)

		got := EvalRules(g, cfg, testDayOf(t, "2026-09-01"))

		testAssertSingleFinding(t, got, model.RuleStatusDrift, model.SeverityError, "0001")
	})
}

func TestOneDayDrivesEveryTimeDependentAnswer(t *testing.T) {
	// A field's sunset and a kind's period are different declarations, and
	// Validate compares both against the day it is handed: one run is about one
	// day, whatever it is asking.
	cfg := config.SpecPreset()
	clause := cfg.Kinds[config.KindClause]
	clause.Fields = map[string]config.FieldSpec{
		config.FieldModality:     clause.Fields[config.FieldModality],
		config.FieldInForceFrom:  {},
		config.FieldInForceUntil: {},
		"owner":                  {Deprecated: true, Sunset: "2026-06-01"},
	}
	cfg.Kinds[config.KindClause] = clause
	g := Build([]*parse.Document{
		testTopicDoc(testTopic),
		testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, map[string]any{
			"owner":                  "the grading team",
			config.FieldInForceUntil: "2026-06-01",
		}),
	}, cfg)

	answers := func(t *testing.T, day string) (deprecated model.Severity, inForce bool) {
		t.Helper()
		for _, f := range Validate(g, cfg, testDayOf(t, day)) {
			if f.Rule == model.RuleDeprecatedField {
				deprecated = f.Severity
			}
		}
		return deprecated, EvalPeriods(g, cfg, testDayOf(t, day)).InForce("UZ-V-001")
	}

	t.Run("on the last day both are still tolerated", func(t *testing.T) {
		// The sunset day itself still warns, and the period ends on it: the two
		// declarations mean different things about the same day, and each keeps
		// its own meaning.
		severity, inForce := answers(t, "2026-06-01")

		if severity != model.SeverityWarn {
			t.Errorf("deprecated_field severity = %q, want %q on the sunset day", severity, model.SeverityWarn)
		}
		if inForce {
			t.Error("InForce = true on the day the period ends, want the interval closed-open")
		}
	})

	t.Run("the day after, the deadline has passed", func(t *testing.T) {
		severity, inForce := answers(t, "2026-06-02")

		if severity != model.SeverityError {
			t.Errorf("deprecated_field severity = %q, want %q past the sunset", severity, model.SeverityError)
		}
		if inForce {
			t.Error("InForce = true, want the clause out of force after its end")
		}
	})
}

// testRuleNamed returns one rule of a configuration, so a test can evaluate the
// preset's own rule rather than a copy of it that might drift.
func testRuleNamed(t *testing.T, cfg config.Config, name string) config.Rule {
	t.Helper()
	for _, rule := range cfg.Rules {
		if rule.Name == name {
			return rule
		}
	}
	t.Fatalf("the configuration declares no rule %q", name)
	return config.Rule{}
}

// TestADepartureFromAClauseWithAPendingSuccessor is the contradiction the
// `spec` preset's deviates-from target used to state in one run: a successor
// that has not taken over leaves its predecessor binding, which is what
// pending_successor reports, while a target spelled `not_inbound: supersedes`
// called every departure from that same predecessor stale. Both cannot be
// true, and the day is what decides.
func TestADepartureFromAClauseWithAPendingSuccessor(t *testing.T) {
	cfg := config.SpecPreset()
	corpus := func(status, from string) *model.Graph {
		return Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.DefaultStatusField:      status,
				config.FieldInForceFrom:        from,
				config.EdgeSupersedes.String(): testSupersedes("UZ-V-001"),
			}),
			// A departure with no expiry runs from the day it was recorded and
			// never ends, so the day under test only moves the successor.
			testDeviation("dev-0001", "UZ-V-001", "2026-03-01", ""),
		}, cfg)
	}
	pending := func(t *testing.T, g *model.Graph, day string) bool {
		t.Helper()
		for _, f := range EvalRules(g, cfg, testDayOf(t, day)) {
			if f.Rule == model.RulePendingSuccessor && f.ID == "UZ-V-001" {
				return true
			}
		}
		return false
	}

	for _, tt := range []struct {
		name, status, from string
	}{
		{name: "a successor nobody has accepted", status: config.StatusTrial},
		{name: "an accepted successor that has not begun", status: config.StatusAccepted, from: "2026-12-01"},
	} {
		t.Run(tt.name+" leaves the departure alone", func(t *testing.T) {
			g := corpus(tt.status, tt.from)

			if got := CheckTargets(g, cfg, testDayOf(t, "2026-09-01")); len(got) != 0 {
				t.Fatalf("findings = %+v, want none: the clause departed from is still what binds", got)
			}
			if !pending(t, g, "2026-09-01") {
				t.Error("pending_successor did not fire on UZ-V-001, want the change in flight reported")
			}
		})
	}

	t.Run("once the successor is in force the departure is stale", func(t *testing.T) {
		g := corpus(config.StatusAccepted, "2026-12-01")
		day := testDayOf(t, "2026-12-01")

		f := testAssertSingleFinding(t, CheckTargets(g, cfg, day), model.RuleStaleTarget, model.SeverityError, "dev-0001")

		if f.Detail != "deviates-from targets UZ-V-001, which UZ-V-002 supersedes" {
			t.Errorf("detail = %q, want the clause that replaced the one departed from", f.Detail)
		}
		// The spelling is what earns the remedy: leaf_of has a lineage to walk.
		fixed := Suggest([]model.Finding{f}, g, cfg, day)
		if fixed[0].Fix != "did you mean UZ-V-002?" {
			t.Errorf("fix = %q, want the clause that took over", fixed[0].Fix)
		}
		if pending(t, g, "2026-12-01") {
			t.Error("pending_successor fired as well, want the two findings never to disagree")
		}
	})
}

// TestATargetIsCheckedOnlyWhileItsDeclarerIsInForce covers the weight rule on
// the target checks: an expired departure is a record of something that was,
// and holding a live clause to it would mean no clause any historical
// deviation ever named could be superseded again — append-first history as a
// ratchet rather than an archive.
func TestATargetIsCheckedOnlyWhileItsDeclarerIsInForce(t *testing.T) {
	cfg := config.SpecPreset()
	corpus := func(expires string) *model.Graph {
		return Build([]*parse.Document{
			testTopicDoc(testTopic),
			testClause("UZ-V-001", config.ModalitySHOULD, []string{testTopic}, nil),
			testClause("UZ-V-002", config.ModalitySHOULD, []string{testTopic}, map[string]any{
				config.EdgeSupersedes.String(): testSupersedes("UZ-V-001"),
			}),
			testDeviation("dev-0001", "UZ-V-001", "2026-03-01", expires),
		}, cfg)
	}

	t.Run("a departure still running holds its clause to the condition", func(t *testing.T) {
		got := CheckTargets(corpus("2026-12-01"), cfg, testDayOf(t, "2026-09-01"))

		testAssertSingleFinding(t, got, model.RuleStaleTarget, model.SeverityError, "dev-0001")
	})

	t.Run("a departure that has expired holds it to nothing", func(t *testing.T) {
		g := corpus("2026-06-01")

		if got := CheckTargets(g, cfg, testDayOf(t, "2026-09-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: an expired departure states nothing about what is current", got)
		}
		// The whole check runs the same way: superseding a clause an expired
		// record once departed from must leave nothing behind that a corpus
		// cannot clear.
		for _, f := range Check(g, cfg, testDayOf(t, "2026-09-01")) {
			if f.Rule == model.RuleStaleTarget {
				t.Errorf("Check reported %+v, want no stale target from a record that has ended", f)
			}
		}
	})
}
