package graph

import (
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

func TestCheckDocuments(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("two documents normalizing to one id collide", func(t *testing.T) {
		docs := []*parse.Document{
			{Path: "0004-a.md", Name: "0004-a.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0004-b.md", Name: "0004-b.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
		}

		f := testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleIDCollision, model.SeverityError, "0004")
		if f.Location.Path != "0004-a.md" {
			t.Errorf("location = %+v, want the lexically first path", f.Location)
		}
		if !strings.Contains(f.Detail, "0004-b.md") {
			t.Errorf("detail = %q, want it to name the colliding peer", f.Detail)
		}
	})

	t.Run("a third colliding document does not add a second finding", func(t *testing.T) {
		docs := []*parse.Document{
			{Path: "0004-a.md", Name: "0004-a.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0004-b.md", Name: "0004-b.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0004-c.md", Name: "0004-c.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
		}

		testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleIDCollision, model.SeverityError, "0004")
	})

	t.Run("undecodable frontmatter is an error", func(t *testing.T) {
		docs := []*parse.Document{
			{
				Path:           "0001.md",
				Name:           "0001.md",
				ID:             "0001",
				HasFrontmatter: true,
				MatchesPattern: true,
				Err:            errInvalidFixtureFrontmatter,
			},
		}

		testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleInvalidFrontmatter, model.SeverityError, "0001")
	})

	t.Run("a managed filename without frontmatter is a warning", func(t *testing.T) {
		docs := []*parse.Document{
			{Path: "0001-no-frontmatter.md", Name: "0001-no-frontmatter.md", ID: "0001", MatchesPattern: true},
		}

		testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleMissingFrontmatter, model.SeverityWarn, "0001")
	})

	t.Run("valid documents produce no findings", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, ""),
			testDoc("0002", map[string]any{"status": "accepted"}, ""),
		}

		if got := CheckDocuments(docs, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("findings are ordered deterministically", func(t *testing.T) {
		docs := []*parse.Document{
			{Path: "0009.md", Name: "0009.md", ID: "0009", HasFrontmatter: true, MatchesPattern: true, Err: errInvalidFixtureFrontmatter},
			{Path: "0002-a.md", Name: "0002-a.md", ID: "0002", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0002-b.md", Name: "0002-b.md", ID: "0002", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0001.md", Name: "0001.md", ID: "0001", MatchesPattern: true},
		}

		got := CheckDocuments(docs, cfg)

		testAssertSortedFindings(t, got)
		if len(got) != 3 {
			t.Fatalf("findings = %+v, want 3", got)
		}
	})
}

func TestCheckCycles(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a cycle in an acyclic edge type is an error naming the path", func(t *testing.T) {
		got := CheckCycles(testSupersedesCycle(), cfg)

		f := testAssertSingleFinding(t, got, model.RuleCycle, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, "0001 -> 0002 -> 0003 -> 0001") {
			t.Errorf("detail = %q, want the closed cycle path", f.Detail)
		}
	})

	t.Run("an acyclic graph reports nothing", func(t *testing.T) {
		if got := CheckCycles(testMixedFixture(), cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a cycle in a cyclic edge type is allowed", func(t *testing.T) {
		cyclicOK := config.ADRPreset()
		cyclicOK.Edges = append(cyclicOK.Edges, config.EdgeSpec{
			Name:      "relates-to",
			Key:       "relates-to",
			Acyclic:   false,
			Direction: config.DirectionForward,
		})
		g := testGraph(
			[]*model.Node{testNode("0001", config.StatusAccepted), testNode("0002", config.StatusAccepted)},
			[]model.Edge{
				testEdge("0001", "0002", "relates-to"),
				testEdge("0002", "0001", "relates-to"),
			},
			nil,
		)

		if got := CheckCycles(g, cyclicOK); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("every cycle is reported in deterministic order", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusSuperseded),
				testNode("0007", config.StatusSuperseded),
				testNode("0008", config.StatusSuperseded),
			},
			[]model.Edge{
				testEdge("0001", "0002", config.EdgeSupersedes),
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0007", "0008", config.EdgeSupersedes),
				testEdge("0008", "0007", config.EdgeSupersedes),
			},
			nil,
		)

		got := CheckCycles(g, cfg)

		if len(got) != 2 {
			t.Fatalf("findings = %+v, want 2", got)
		}
		testAssertIDs(t, "cycle ids", testFindingIDs(got, model.RuleCycle), testIDs("0001", "0007"))
	})
}

func TestCheckDangling(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("an edge to an unknown document is an error on the source", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusAccepted)},
			[]model.Edge{testEdge("0002", "0099", config.EdgeSupersedes)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckDangling(g, cfg), model.RuleDanglingRef, model.SeverityError, "0002")
		if !strings.Contains(f.Detail, "0099") {
			t.Errorf("detail = %q, want it to name the missing target", f.Detail)
		}
		if !strings.Contains(f.Detail, config.EdgeSupersedes.String()) {
			t.Errorf("detail = %q, want it to name the edge type", f.Detail)
		}
	})

	t.Run("known targets report nothing", func(t *testing.T) {
		if got := CheckDangling(testMixedFixture(), cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("reference edges are never validated", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusAccepted)},
			nil,
			[]model.Edge{testRefEdge("0002", "0099")},
		)

		if got := CheckDangling(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("findings are sorted", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusAccepted), testNode("0003", config.StatusAccepted)},
			[]model.Edge{
				testEdge("0003", "0098", config.EdgeDependsOn),
				testEdge("0002", "0099", config.EdgeSupersedes),
			},
			nil,
		)

		got := CheckDangling(g, cfg)

		if len(got) != 2 {
			t.Fatalf("findings = %+v, want 2", got)
		}
		testAssertSortedFindings(t, got)
		testAssertIDs(t, "dangling ids", testFindingIDs(got, model.RuleDanglingRef), testIDs("0002", "0003"))
	})

	t.Run("a reverse edge from an unknown document is an error on its owner", func(t *testing.T) {
		// A MADR "status: superseded by 0099" with no 0099 in the corpus puts
		// the unknown document at the source of the edge, not at its target.
		g := testGraph(
			[]*model.Node{testNode("0007", config.StatusSuperseded)},
			[]model.Edge{testDerivedEdge("0099", "0007", config.EdgeSupersedes)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckDangling(g, cfg), model.RuleDanglingRef, model.SeverityError, "0007")
		if !strings.Contains(f.Detail, "0099") {
			t.Errorf("detail = %q, want it to name the missing document", f.Detail)
		}
		if !strings.Contains(f.Detail, config.EdgeSupersedes.String()) {
			t.Errorf("detail = %q, want it to name the edge type", f.Detail)
		}
	})

	t.Run("a structured edge declared in the reverse direction is checked too", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0007", config.StatusSuperseded)},
			[]model.Edge{testEdge("0099", "0007", "superseded-by")},
			nil,
		)

		testAssertSingleFinding(t, CheckDangling(g, cfg), model.RuleDanglingRef, model.SeverityError, "0007")
	})
}

func TestCheckStatusVocabularyRejectsProseAroundAVocabularyWord(t *testing.T) {
	cfg := config.ADRPreset()
	// Only a status that a derived-edge pattern claims may project onto a
	// vocabulary word; everything else outside the vocabulary is an error.
	for _, status := range []string{
		"accepted by the architecture board",
		"proposed - pending review",
		"rejected-in-favour-of-0004",
	} {
		t.Run(status, func(t *testing.T) {
			g := testGraph([]*model.Node{testNode("0001", status)}, nil, nil)

			f := testAssertSingleFinding(t, CheckStatusVocabulary(g, cfg), model.RuleUnknownStatus, model.SeverityError, "0001")
			if !strings.Contains(f.Detail, status) {
				t.Errorf("detail = %q, want it to quote the offending status", f.Detail)
			}
		})
	}
}

func TestCheckStatusVocabulary(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a status outside the vocabulary is an error", func(t *testing.T) {
		g := testGraph([]*model.Node{testNode("0001", "adopted")}, nil, nil)

		f := testAssertSingleFinding(t, CheckStatusVocabulary(g, cfg), model.RuleUnknownStatus, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, "adopted") {
			t.Errorf("detail = %q, want it to name the offending status", f.Detail)
		}
	})

	t.Run("the vocabulary comparison is case-insensitive", func(t *testing.T) {
		g := testGraph([]*model.Node{testNode("0001", "Accepted"), testNode("0002", "PROPOSED")}, nil, nil)

		if got := CheckStatusVocabulary(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a MADR superseded-by status counts as superseded", func(t *testing.T) {
		g := testGraph([]*model.Node{testNode("0001", "superseded by 0003")}, nil, nil)

		if got := CheckStatusVocabulary(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("an absent status is not an unknown status", func(t *testing.T) {
		g := testGraph([]*model.Node{testNode("0001", "")}, nil, nil)

		if got := CheckStatusVocabulary(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("no configured vocabulary disables the check", func(t *testing.T) {
		noVocabulary := config.ADRPreset()
		noVocabulary.StatusValues = nil
		g := testGraph([]*model.Node{testNode("0001", "anything at all")}, nil, nil)

		if got := CheckStatusVocabulary(g, noVocabulary); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})
}

func TestCheckDerived(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a derived edge warns about unstructured frontmatter", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusSuperseded), testNode("0003", config.StatusAccepted)},
			[]model.Edge{testDerivedEdge("0003", "0002", config.EdgeSupersedes)},
			nil,
		)

		testAssertSingleFinding(t, CheckDerived(g, cfg), model.RuleUnstructuredSupersedes, model.SeverityWarn, "0002")
	})

	t.Run("a derived edge contradicting a structured edge is an error", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusSuperseded), testNode("0003", config.StatusSuperseded)},
			[]model.Edge{
				testEdge("0002", "0003", config.EdgeSupersedes),
				testDerivedEdge("0003", "0002", config.EdgeSupersedes),
			},
			nil,
		)

		testAssertSingleFinding(t, CheckDerived(g, cfg), model.RuleDerivedConflict, model.SeverityError, "0002")
	})

	t.Run("structured edges alone report nothing", func(t *testing.T) {
		if got := CheckDerived(testMixedFixture(), cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})
}

func TestCheck(t *testing.T) {
	cfg := config.ADRPreset()
	g := testGraph(
		[]*model.Node{
			testNode("0001", "adopted"),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusSuperseded),
		},
		[]model.Edge{
			testEdge("0001", "0002", config.EdgeSupersedes),
			testEdge("0002", "0003", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeSupersedes),
			testEdge("0002", "0099", config.EdgeDependsOn),
		},
		nil,
	)

	got := Check(g, cfg)

	t.Run("every built-in check contributes", func(t *testing.T) {
		for _, rule := range []string{model.RuleCycle, model.RuleDanglingRef, model.RuleUnknownStatus} {
			if len(testFindingsFor(got, rule)) == 0 {
				t.Errorf("no %s finding in %+v", rule, got)
			}
		}
	})

	t.Run("findings come back in deterministic order", func(t *testing.T) {
		testAssertSortedFindings(t, got)
	})
}

func TestMatchCondition(t *testing.T) {
	g := testRulesFixture()

	tests := []struct {
		name string
		cond config.Condition
		id   string
		want bool
	}{
		{
			name: "inbound matches a document with that inbound edge type",
			cond: config.Condition{Inbound: config.EdgeSupersedes.String()},
			id:   "0001",
			want: true,
		},
		{
			name: "inbound does not match a document without one",
			cond: config.Condition{Inbound: config.EdgeSupersedes.String()},
			id:   "0002",
			want: false,
		},
		{
			name: "inbound is edge-type specific",
			cond: config.Condition{Inbound: config.EdgeSupersedes.String()},
			id:   "0003",
			want: false,
		},
		{
			name: "not_inbound is the negation",
			cond: config.Condition{NotInbound: config.EdgeSupersedes.String()},
			id:   "0003",
			want: true,
		},
		{
			name: "not_inbound does not match a document with that edge",
			cond: config.Condition{NotInbound: config.EdgeSupersedes.String()},
			id:   "0001",
			want: false,
		},
		{
			name: "outbound matches a document with that outbound edge type",
			cond: config.Condition{Outbound: config.EdgeSupersedes.String()},
			id:   "0002",
			want: true,
		},
		{
			name: "outbound does not match a sink",
			cond: config.Condition{Outbound: config.EdgeSupersedes.String()},
			id:   "0001",
			want: false,
		},
		{
			name: "not_outbound is the negation",
			cond: config.Condition{NotOutbound: config.EdgeSupersedes.String()},
			id:   "0001",
			want: true,
		},
		{
			name: "attr eq matches the attribute value",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrEq(config.StatusAccepted)}},
			id:   "0001",
			want: true,
		},
		{
			name: "attr eq rejects a different value",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrEq(config.StatusAccepted)}},
			id:   "0003",
			want: false,
		},
		{
			name: "attr eq is case-insensitive",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrEq(config.StatusAccepted)}},
			id:   "0005",
			want: true,
		},
		{
			name: "attr eq rejects an absent attribute",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"owner": testAttrEq("platform")}},
			id:   "0001",
			want: false,
		},
		{
			name: "attr eq reads any frontmatter key",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"owner": testAttrEq("platform")}},
			id:   "0006",
			want: true,
		},
		{
			name: "attr not matches a different value",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrNot(config.StatusSuperseded)}},
			id:   "0001",
			want: true,
		},
		{
			name: "attr not rejects the excluded value",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrNot(config.StatusSuperseded)}},
			id:   "0003",
			want: false,
		},
		{
			name: "attr not matches an absent attribute",
			cond: config.Condition{Attr: map[string]config.AttrCondition{"owner": testAttrNot("platform")}},
			id:   "0001",
			want: true,
		},
		{
			name: "several attributes are ANDed",
			cond: config.Condition{Attr: map[string]config.AttrCondition{
				"status": testAttrEq(config.StatusProposed),
				"owner":  testAttrEq("platform"),
			}},
			id:   "0006",
			want: false,
		},
		{
			name: "edge and attribute clauses are ANDed",
			cond: config.Condition{
				Inbound: config.EdgeSupersedes.String(),
				Attr:    map[string]config.AttrCondition{"status": testAttrNot(config.StatusSuperseded)},
			},
			id:   "0001",
			want: true,
		},
		{
			name: "a failing clause fails the whole condition",
			cond: config.Condition{
				Inbound: config.EdgeSupersedes.String(),
				Attr:    map[string]config.AttrCondition{"status": testAttrNot(config.StatusSuperseded)},
			},
			id:   "0003",
			want: false,
		},
		{
			name: "an empty condition matches every document",
			cond: config.Condition{},
			id:   "0002",
			want: true,
		},
		{
			name: "an unknown document matches nothing",
			cond: config.Condition{},
			id:   "0099",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchCondition(g, tt.cond, model.ID(tt.id)); got != tt.want {
				t.Fatalf("MatchCondition(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestEvalRule(t *testing.T) {
	cfg := config.ADRPreset()
	g := testRulesFixture()
	drift := cfg.Rules[0]

	t.Run("one finding per matching document, sorted", func(t *testing.T) {
		want := []model.Finding{
			{Severity: drift.Severity, Rule: drift.Name, ID: "0001", Detail: drift.Message, Location: testNodeLocation("0001", testStatusLine)},
			{Severity: drift.Severity, Rule: drift.Name, ID: "0004", Detail: drift.Message, Location: testNodeLocation("0004", testStatusLine)},
		}

		testAssertFindings(t, "EvalRule", EvalRule(g, cfg, drift), want)
	})

	t.Run("a rule matching nothing reports nothing", func(t *testing.T) {
		unmatched := config.Rule{
			Name:     "never",
			Severity: model.SeverityWarn,
			When:     config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrEq("withdrawn")}},
			Message:  "no document has this status",
		}

		if got := EvalRule(g, cfg, unmatched); len(got) != 0 {
			t.Fatalf("EvalRule = %+v, want none", got)
		}
	})
}

func TestEvalRules(t *testing.T) {
	g := testRulesFixture()

	t.Run("the preset status rules are ordinary declarative rules", func(t *testing.T) {
		cfg := config.ADRPreset()
		drift, orphan := cfg.Rules[0], cfg.Rules[1]
		want := []model.Finding{
			{Severity: drift.Severity, Rule: drift.Name, ID: "0001", Detail: drift.Message, Location: testNodeLocation("0001", testStatusLine)},
			{Severity: drift.Severity, Rule: drift.Name, ID: "0004", Detail: drift.Message, Location: testNodeLocation("0004", testStatusLine)},
			{Severity: orphan.Severity, Rule: orphan.Name, ID: "0003", Detail: orphan.Message, Location: testNodeLocation("0003", testStatusLine)},
		}

		testAssertFindings(t, "EvalRules", EvalRules(g, cfg), want)
	})

	t.Run("configured rules replace the preset rules", func(t *testing.T) {
		custom := config.ADRPreset()
		custom.Rules = []config.Rule{{
			Name:     "accepted_with_dependencies",
			Severity: model.SeverityWarn,
			When: config.Condition{
				Outbound: config.EdgeDependsOn.String(),
				Attr:     map[string]config.AttrCondition{"status": testAttrEq(config.StatusAccepted)},
			},
			Message: "an accepted decision still depends on another",
		}}
		want := []model.Finding{{
			Severity: model.SeverityWarn,
			Rule:     "accepted_with_dependencies",
			ID:       "0002",
			Detail:   "an accepted decision still depends on another",
			Location: testNodeLocation("0002", testStatusLine),
		}}

		testAssertFindings(t, "EvalRules", EvalRules(g, custom), want)
	})

	t.Run("no configured rules report nothing", func(t *testing.T) {
		none := config.ADRPreset()
		none.Rules = nil

		if got := EvalRules(g, none); len(got) != 0 {
			t.Fatalf("EvalRules = %+v, want none", got)
		}
	})
}

func TestValidate(t *testing.T) {
	cfg := config.ADRPreset()
	g := testGraph(
		[]*model.Node{
			testNode("0001", config.StatusAccepted),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusSuperseded),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0002", "0099", config.EdgeSupersedes),
		},
		nil,
	)
	g.Findings = []model.Finding{{
		Severity: model.SeverityError,
		Rule:     model.RuleInvalidFrontmatter,
		ID:       "0004",
		Detail:   "frontmatter does not decode",
		Location: testNodeLocation("0004", 2),
	}}

	got := Validate(g, cfg)

	t.Run("structural checks, rules and build findings all appear", func(t *testing.T) {
		want := []string{
			model.RuleStatusDrift,
			model.RuleDanglingRef,
			model.RuleInvalidFrontmatter,
			model.RuleSupersededOrphan,
		}

		if !slices.Equal(testRuleNames(got), want) {
			t.Fatalf("rules = %v, want %v (path order, then line)", testRuleNames(got), want)
		}
	})

	t.Run("errors come before warnings", func(t *testing.T) {
		testAssertSortedFindings(t, got)
		if len(got) == 0 {
			t.Fatal("findings = none, want the warning last")
		}
		if got[len(got)-1].Severity != model.SeverityWarn {
			t.Fatalf("last finding = %+v, want the warning last", got[len(got)-1])
		}
	})

	t.Run("a clean graph validates without findings", func(t *testing.T) {
		clean := testGraph(
			[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusAccepted)},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		if findings := Validate(clean, cfg); len(findings) != 0 {
			t.Fatalf("findings = %+v, want none", findings)
		}
	})
}

func TestSortFindings(t *testing.T) {
	tests := []struct {
		name  string
		input []model.Finding
		want  []model.Finding
	}{
		{
			name:  "empty stays empty",
			input: []model.Finding{},
			want:  []model.Finding{},
		},
		{
			name: "errors sort before warnings",
			input: []model.Finding{
				{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0001"},
				{Severity: model.SeverityError, Rule: model.RuleUnknownStatus, ID: "0009"},
			},
			want: []model.Finding{
				{Severity: model.SeverityError, Rule: model.RuleUnknownStatus, ID: "0009"},
				{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0001"},
			},
		},
		{
			name: "then rule, then id, then detail",
			input: []model.Finding{
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0002", Detail: "z"},
				{Severity: model.SeverityError, Rule: model.RuleStatusDrift, ID: "0001", Detail: "a"},
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001", Detail: "y"},
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001", Detail: "x"},
			},
			want: []model.Finding{
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001", Detail: "x"},
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001", Detail: "y"},
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0002", Detail: "z"},
				{Severity: model.SeverityError, Rule: model.RuleStatusDrift, ID: "0001", Detail: "a"},
			},
		},
		{
			name: "an already sorted slice is left alone",
			input: []model.Finding{
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001"},
				{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0002"},
			},
			want: []model.Finding{
				{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001"},
				{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0002"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Clone(tt.input)

			SortFindings(got)

			testAssertFindings(t, "SortFindings", got, tt.want)
		})
	}
}

func TestSummarize(t *testing.T) {
	g := testTypedFixture()
	findings := []model.Finding{
		{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001"},
		{Severity: model.SeverityError, Rule: model.RuleDanglingRef, ID: "0002"},
		{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0003"},
	}
	want := model.Summary{Documents: 4, Edges: 3, Errors: 2, Warnings: 1, Cycles: 1}

	if got := Summarize(g, findings); got != want {
		t.Fatalf("Summarize = %+v, want %+v", got, want)
	}
}

// testRulesFixture drifts 0001 and 0004 (inbound supersedes, other status),
// leaves 0003 a superseded orphan behind a depends-on edge, and gives 0006 a
// frontmatter key outside the recognized set.
func testRulesFixture() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusAccepted),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusSuperseded),
			testNode("0004", config.StatusProposed),
			testNode("0005", "Accepted"),
			testNodeAttrs("0006", config.StatusRejected, map[string]string{"owner": "platform"}),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0002", "0004", config.EdgeSupersedes),
			testEdge("0002", "0003", config.EdgeDependsOn),
		},
		nil,
	)
}

func TestCheckDocumentsLocatesFindings(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a decode failure is located at the offending token", func(t *testing.T) {
		docs := []*parse.Document{{
			Path:            "0001.md",
			Name:            "0001.md",
			ID:              "0001",
			HasFrontmatter:  true,
			MatchesPattern:  true,
			FrontmatterLine: 1,
			Err:             &parse.FrontmatterError{Message: "mapping value is not allowed in this context", Line: 4, Column: 9},
		}}

		f := testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleInvalidFrontmatter, model.SeverityError, "0001")
		want := model.Location{Path: "0001.md", Line: 4, Column: 9}
		if f.Location != want {
			t.Errorf("location = %+v, want %+v", f.Location, want)
		}
		if f.Detail != "mapping value is not allowed in this context" {
			t.Errorf("detail = %q, want the decoder message alone", f.Detail)
		}
	})

	t.Run("an absent frontmatter block is located on the first line", func(t *testing.T) {
		docs := []*parse.Document{{Path: "0001-bare.md", Name: "0001-bare.md", ID: "0001", MatchesPattern: true}}

		f := testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleMissingFrontmatter, model.SeverityWarn, "0001")
		want := model.Location{Path: "0001-bare.md", Line: 1}
		if f.Location != want {
			t.Errorf("location = %+v, want %+v", f.Location, want)
		}
	})

	t.Run("a collision is located on the first path and relates the others", func(t *testing.T) {
		docs := []*parse.Document{
			{Path: "0004-c.md", Name: "0004-c.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0004-a.md", Name: "0004-a.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
			{Path: "0004-b.md", Name: "0004-b.md", ID: "0004", HasFrontmatter: true, MatchesPattern: true},
		}

		f := testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleIDCollision, model.SeverityError, "0004")
		if f.Location != (model.Location{Path: "0004-a.md", Line: 1}) {
			t.Errorf("location = %+v, want the lexically first path", f.Location)
		}
		want := []model.Location{{Path: "0004-b.md", Line: 1}, {Path: "0004-c.md", Line: 1}}
		if !slices.Equal(f.Related, want) {
			t.Errorf("related = %+v, want %+v", f.Related, want)
		}
		for _, path := range []string{"0004-b.md", "0004-c.md"} {
			if !strings.Contains(f.Detail, path) {
				t.Errorf("detail = %q, want it to name %s", f.Detail, path)
			}
		}
	})
}

func TestCheckCyclesLocatesTheSmallestMember(t *testing.T) {
	f := testAssertSingleFinding(t, CheckCycles(testSupersedesCycle(), config.ADRPreset()), model.RuleCycle, model.SeverityError, "0001")

	if want := testNodeLocation("0001", testSupersedesLine); f.Location != want {
		t.Errorf("location = %+v, want %+v", f.Location, want)
	}
	want := []model.Location{testNodeLocation("0002", testSupersedesLine), testNodeLocation("0003", testSupersedesLine)}
	if !slices.Equal(f.Related, want) {
		t.Errorf("related = %+v, want the other members %+v", f.Related, want)
	}
}

func TestCheckDanglingLocatesTheDeclaringKey(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a structured edge is located on its frontmatter key", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusAccepted)},
			[]model.Edge{testEdge("0002", "0099", config.EdgeSupersedes)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckDangling(g, cfg), model.RuleDanglingRef, model.SeverityError, "0002")
		if want := testNodeLocation("0002", testSupersedesLine); f.Location != want {
			t.Errorf("location = %+v, want %+v", f.Location, want)
		}
	})

	t.Run("a derived edge is located on the field it was read from", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", "superseded by 0099")},
			[]model.Edge{testDerivedEdge("0099", "0002", config.EdgeSupersedes)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckDangling(g, cfg), model.RuleDanglingRef, model.SeverityError, "0002")
		if want := testNodeLocation("0002", testStatusLine); f.Location != want {
			t.Errorf("location = %+v, want the field the edge was derived from", f.Location)
		}
	})
}

func TestCheckStatusVocabularyLocatesTheStatusField(t *testing.T) {
	g := testGraph([]*model.Node{testNode("0001", "retired")}, nil, nil)

	f := testAssertSingleFinding(t, CheckStatusVocabulary(g, config.ADRPreset()), model.RuleUnknownStatus, model.SeverityError, "0001")

	if want := testNodeLocation("0001", testStatusLine); f.Location != want {
		t.Errorf("location = %+v, want %+v", f.Location, want)
	}
}

func TestCheckDerivedLocatesTheField(t *testing.T) {
	g := testGraph(
		[]*model.Node{testNode("0002", config.StatusSuperseded), testNode("0003", config.StatusAccepted)},
		[]model.Edge{testDerivedEdge("0003", "0002", config.EdgeSupersedes)},
		nil,
	)

	f := testAssertSingleFinding(t, CheckDerived(g, config.ADRPreset()), model.RuleUnstructuredSupersedes, model.SeverityWarn, "0002")

	if want := testNodeLocation("0002", testStatusLine); f.Location != want {
		t.Errorf("location = %+v, want the field the edge was derived from", f.Location)
	}
}

func TestEvalRuleLocatesTheClauseItRead(t *testing.T) {
	g := testRulesFixture()

	t.Run("an attribute clause points at its key", func(t *testing.T) {
		cfg := config.ADRPreset()

		got := EvalRule(g, cfg, cfg.Rules[0])

		if len(got) == 0 {
			t.Fatal("findings = none, want the drifted documents")
		}
		if want := testNodeLocation("0001", testStatusLine); got[0].Location != want {
			t.Errorf("location = %+v, want %+v", got[0].Location, want)
		}
	})

	t.Run("without an attribute clause the edge key wins", func(t *testing.T) {
		rule := config.Rule{
			Name:     "superseding",
			Severity: model.SeverityWarn,
			When:     config.Condition{Outbound: config.EdgeSupersedes.String()},
			Message:  "supersedes another document",
		}

		got := EvalRule(g, config.ADRPreset(), rule)

		if len(got) == 0 {
			t.Fatal("findings = none, want the superseding document")
		}
		if want := testNodeLocation("0002", testSupersedesLine); got[0].Location != want {
			t.Errorf("location = %+v, want %+v", got[0].Location, want)
		}
	})
}

func TestSortFindingsOrdersByPathThenLine(t *testing.T) {
	at := func(path string, line int, rule string) model.Finding {
		return model.Finding{Severity: model.SeverityError, Rule: rule, Location: model.Location{Path: path, Line: line}}
	}
	got := []model.Finding{
		at("b.md", 2, "cycle"),
		at("a.md", 9, "cycle"),
		at("a.md", 3, "unknown_status"),
		at("a.md", 3, "cycle"),
	}
	want := []model.Finding{
		at("a.md", 3, "cycle"),
		at("a.md", 3, "unknown_status"),
		at("a.md", 9, "cycle"),
		at("b.md", 2, "cycle"),
	}

	SortFindings(got)

	testAssertFindings(t, "SortFindings", got, want)
}
