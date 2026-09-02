package graph

import (
	"slices"
	"strings"
	"testing"
	"time"

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

// testInverseConfig declares the supersedes edge with an inverse key, so a
// superseded document has to name what replaced it.
func testInverseConfig() config.Config {
	cfg := config.ADRPreset()
	cfg.Edges[0].Inverse = testInverseKey
	return cfg
}

func TestCheckInverse(t *testing.T) {
	cfg := testInverseConfig()

	t.Run("an agreeing pair reports nothing", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNodeAttrs("0001", config.StatusSuperseded, map[string]any{testInverseKey: []any{"0002"}}),
				testNode("0002", config.StatusAccepted),
			},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		if got := CheckInverse(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a target that does not name its source is a mismatch", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusAccepted)},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckInverse(g, cfg), model.RuleInverseMismatch, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, testInverseKey) || !strings.Contains(f.Detail, "0002") {
			t.Errorf("detail = %q, want the inverse key and the missing entry", f.Detail)
		}
		if len(f.Related) != 1 || f.Related[0].Path != "0002.md" {
			t.Errorf("related = %+v, want the other endpoint", f.Related)
		}
	})

	t.Run("an entry without a forward edge is a mismatch", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNodeAttrs("0001", config.StatusAccepted, map[string]any{testInverseKey: []any{"0002"}}),
				testNode("0002", config.StatusAccepted),
			},
			nil,
			nil,
		)

		f := testAssertSingleFinding(t, CheckInverse(g, cfg), model.RuleInverseMismatch, model.SeverityError, "0001")
		if f.Location != testNodeLocation("0001", testInverseLine) {
			t.Errorf("location = %+v, want the inverse key line", f.Location)
		}
		if !strings.Contains(f.Detail, config.EdgeSupersedes.String()) {
			t.Errorf("detail = %q, want the edge type that is missing", f.Detail)
		}
	})

	t.Run("an entry naming no document is dangling, not a mismatch", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNodeAttrs("0001", config.StatusAccepted, map[string]any{testInverseKey: []any{"0099"}})},
			nil,
			nil,
		)

		got := CheckInverse(g, cfg)
		testAssertSingleFinding(t, got, model.RuleDanglingRef, model.SeverityError, "0001")
		if len(testFindingsFor(got, model.RuleInverseMismatch)) != 0 {
			t.Errorf("findings = %+v, want the dangling entry reported once", got)
		}
	})

	t.Run("an entry that names no identity is invalid", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNodeAttrs("0001", config.StatusAccepted, map[string]any{testInverseKey: []any{"see 0002"}})},
			nil,
			nil,
		)

		testAssertSingleFinding(t, CheckInverse(g, cfg), model.RuleInvalidRef, model.SeverityError, "0001")
	})

	t.Run("an edge type without an inverse key is not checked", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusAccepted)},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		if got := CheckInverse(g, config.ADRPreset()); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("the inverse key creates no edges", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded", testInverseKey: []any{"0002"}}, ""),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, ""),
		}

		g := Build(docs, cfg)

		want := []model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("an empty inverse key is an empty edge", func(t *testing.T) {
		docs := []*parse.Document{testDoc("0001", map[string]any{"status": "accepted", testInverseKey: nil}, "")}

		g := Build(docs, cfg)

		testAssertSingleFinding(t, g.Findings, model.RuleEmptyEdge, model.SeverityError, "0001")
	})
}

func TestStructuralSeverityEscalation(t *testing.T) {
	escalated := config.ADRPreset()
	escalated.Structural = map[string]model.Severity{
		model.RuleMissingFrontmatter:     model.SeverityError,
		model.RuleUnstructuredSupersedes: model.SeverityError,
	}

	t.Run("a warned file-level check becomes an error", func(t *testing.T) {
		docs := []*parse.Document{{Path: "0001.md", ID: "0001", MatchesPattern: true}}

		testAssertSingleFinding(t, CheckDocuments(docs, escalated),
			model.RuleMissingFrontmatter, model.SeverityError, "0001")
	})

	t.Run("a warned graph check becomes an error", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusSuperseded), testNode("0003", config.StatusAccepted)},
			[]model.Edge{testDerivedEdge("0003", "0002", config.EdgeSupersedes)},
			nil,
		)

		testAssertSingleFinding(t, CheckDerived(g, escalated),
			model.RuleUnstructuredSupersedes, model.SeverityError, "0002")
	})

	t.Run("an unescalated corpus keeps the default severity", func(t *testing.T) {
		docs := []*parse.Document{{Path: "0001.md", ID: "0001", MatchesPattern: true}}

		testAssertSingleFinding(t, CheckDocuments(docs, config.ADRPreset()),
			model.RuleMissingFrontmatter, model.SeverityWarn, "0001")
	})
}

func TestCheckCyclesOverTheUnionOfAcyclicEdgeTypes(t *testing.T) {
	union := func() config.Config {
		cfg := config.ADRPreset()
		cfg.AcyclicUnion = true
		return cfg
	}
	mixed := testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0001", "0002", config.EdgeDependsOn),
			testEdge("0002", "0003", config.EdgeDependsOn),
			testEdge("0003", "0001", config.EdgeSupersedes),
		},
		nil,
	)

	t.Run("each edge type on its own stays acyclic", func(t *testing.T) {
		if got := CheckCycles(mixed, config.ADRPreset()); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("the union closes the loop", func(t *testing.T) {
		f := testAssertSingleFinding(t, CheckCycles(mixed, union()), model.RuleCycle, model.SeverityError, "0001")
		for _, want := range []string{config.EdgeDependsOn.String(), config.EdgeSupersedes.String(), "0002", "0003"} {
			if !strings.Contains(f.Detail, want) {
				t.Errorf("detail = %q, want it to name %q", f.Detail, want)
			}
		}
		if len(f.Related) != 2 {
			t.Errorf("related = %+v, want the other two members", f.Related)
		}
	})

	t.Run("a cross-type cycle sharing a component with a single-type one is still reported", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusSuperseded),
				testNode("0003", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0001", "0002", config.EdgeSupersedes),
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0002", "0003", config.EdgeDependsOn),
				testEdge("0003", "0001", config.EdgeDependsOn),
			},
			nil,
		)

		got := testFindingsFor(CheckCycles(g, union()), model.RuleCycle)

		if len(got) != 2 {
			t.Fatalf("findings = %+v, want the supersedes cycle and the one only the union closes", got)
		}
		details := strings.Join([]string{got[0].Detail, got[1].Detail}, "\n")
		for _, want := range []string{"supersedes cycle: 0001 -> 0002 -> 0001", "cycle over supersedes, depends-on: 0001 -> 0002 -> 0003 -> 0001"} {
			if !strings.Contains(details, want) {
				t.Errorf("details = %q, want them to carry %q", details, want)
			}
		}
	})

	t.Run("a cycle every edge type carries on its own is not a union cycle", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusSuperseded)},
			[]model.Edge{
				testEdge("0001", "0002", config.EdgeSupersedes),
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0001", "0002", config.EdgeDependsOn),
				testEdge("0002", "0001", config.EdgeDependsOn),
			},
			nil,
		)

		got := testFindingsFor(CheckCycles(g, union()), model.RuleCycle)

		if len(got) != 2 {
			t.Fatalf("findings = %+v, want one cycle per edge type and nothing extra", got)
		}
	})

	t.Run("a cycle inside one edge type is reported once", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusSuperseded)},
			[]model.Edge{
				testEdge("0001", "0002", config.EdgeSupersedes),
				testEdge("0002", "0001", config.EdgeSupersedes),
			},
			nil,
		)

		got := CheckCycles(g, union())

		if len(testFindingsFor(got, model.RuleCycle)) != 1 {
			t.Fatalf("findings = %+v, want one", got)
		}
	})
}

func TestCheckCardinality(t *testing.T) {
	bounded := func(mutate func(*config.EdgeSpec)) config.Config {
		cfg := config.ADRPreset()
		mutate(&cfg.Edges[0])
		return cfg
	}
	fanIn := testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeSupersedes),
		},
		nil,
	)

	t.Run("an unbounded edge type reports nothing", func(t *testing.T) {
		if got := CheckCardinality(fanIn, config.ADRPreset()); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("too many inbound edges is an error on the target", func(t *testing.T) {
		cfg := bounded(func(s *config.EdgeSpec) { s.MaxInbound = 1 })

		f := testAssertSingleFinding(t, CheckCardinality(fanIn, cfg), model.RuleCardinality, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, "max_inbound") || !strings.Contains(f.Detail, "2") {
			t.Errorf("detail = %q, want the count and the bound", f.Detail)
		}
		if f.Location != testNodeLocation("0001", testSupersedesLine) {
			t.Errorf("location = %+v, want the edge key line", f.Location)
		}
	})

	t.Run("too many outbound edges is an error on the source", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusSuperseded),
				testNode("0003", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0003", "0001", config.EdgeSupersedes),
				testEdge("0003", "0002", config.EdgeSupersedes),
			},
			nil,
		)
		cfg := bounded(func(s *config.EdgeSpec) { s.MaxOutbound = 1 })

		f := testAssertSingleFinding(t, CheckCardinality(g, cfg), model.RuleCardinality, model.SeverityError, "0003")
		if !strings.Contains(f.Detail, "max_outbound") {
			t.Errorf("detail = %q, want the bound named", f.Detail)
		}
	})

	t.Run("too few outbound edges is an error on every document short of the minimum", func(t *testing.T) {
		cfg := bounded(func(s *config.EdgeSpec) { s.MinOutbound = 1 })

		got := CheckCardinality(fanIn, cfg)

		testAssertIDs(t, "cardinality ids", testFindingIDs(got, model.RuleCardinality), testIDs("0001"))
		if !strings.Contains(got[0].Detail, "min_outbound") {
			t.Errorf("detail = %q, want the bound named", got[0].Detail)
		}
	})

	t.Run("the bound is per edge type", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Edges[1].MaxInbound = 1

		if got := CheckCardinality(fanIn, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the bound is on another edge type", got)
		}
	})

	t.Run("an edge that names its endpoint kinds is bounded over those kinds alone", func(t *testing.T) {
		// A lower bound is the one bound a document with no such key can
		// violate, so without the scoping min_outbound: 1 on an edge from one
		// kind would report every document of every other kind — the edge's own
		// targets included.
		cfg := testKindsConfig()
		cfg.Edges[1].MinOutbound = 1
		g := Build([]*parse.Document{
			testKindDoc("conform", "conform/uz-v-001", map[string]any{"enforces": []any{"UZ-V-001"}}),
			testKindDoc("conform", "conform/uz-v-002", nil),
			testKindDoc("clause", "UZ-V-001", nil),
		}, cfg)

		got := CheckCardinality(g, cfg)

		f := testAssertSingleFinding(t, got, model.RuleCardinality, model.SeverityError, "conform/uz-v-002")
		if !strings.Contains(f.Detail, "min_outbound") {
			t.Errorf("detail = %q, want the bound named", f.Detail)
		}
	})

	t.Run("findings are sorted", func(t *testing.T) {
		cfg := bounded(func(s *config.EdgeSpec) { s.MinOutbound = 1 })
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusAccepted), testNode("0001", config.StatusAccepted)},
			nil,
			nil,
		)

		got := CheckCardinality(g, cfg)

		if len(got) != 2 {
			t.Fatalf("findings = %+v, want 2", got)
		}
		testAssertSortedFindings(t, got)
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

	got := Check(g, cfg, time.Time{})

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
	cfg := config.ADRPreset()

	tests := []struct {
		name string
		cond config.Condition
		id   string
		want bool
	}{
		{
			name: "inbound matches a document with that inbound edge type",
			cond: config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}},
			id:   "0001",
			want: true,
		},
		{
			name: "inbound does not match a document without one",
			cond: config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}},
			id:   "0002",
			want: false,
		},
		{
			name: "inbound is edge-type specific",
			cond: config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}},
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
			cond: config.Condition{Outbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}},
			id:   "0002",
			want: true,
		},
		{
			name: "outbound does not match a sink",
			cond: config.Condition{Outbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}},
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
				Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()},
				Attr:    map[string]config.AttrCondition{"status": testAttrNot(config.StatusSuperseded)},
			},
			id:   "0001",
			want: true,
		},
		{
			name: "a failing clause fails the whole condition",
			cond: config.Condition{
				Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()},
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
			if got := MatchCondition(g, cfg, tt.cond, model.ID(tt.id), testAsOf); got != tt.want {
				t.Fatalf("MatchCondition(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestMatchConditionOnListAttributes(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		cond  config.AttrCondition
		want  bool
	}{
		{
			name:  "contains finds a member",
			attrs: map[string]any{testListAttrsKey: []any{"security", "storage"}},
			cond:  config.AttrCondition{Contains: testStr("security")},
			want:  true,
		},
		{
			name:  "contains misses a member the list does not hold",
			attrs: map[string]any{testListAttrsKey: []any{"storage"}},
			cond:  config.AttrCondition{Contains: testStr("security")},
		},
		{
			name:  "contains reads a scalar as a one element list",
			attrs: map[string]any{testListAttrsKey: "security"},
			cond:  config.AttrCondition{Contains: testStr("security")},
			want:  true,
		},
		{
			name:  "contains is case-insensitive",
			attrs: map[string]any{testListAttrsKey: []any{"Security"}},
			cond:  config.AttrCondition{Contains: testStr("security")},
			want:  true,
		},
		{
			name:  "contains needs the attribute",
			attrs: map[string]any{},
			cond:  config.AttrCondition{Contains: testStr("security")},
		},
		{
			name:  "not_contains holds when the member is absent",
			attrs: map[string]any{testListAttrsKey: []any{"storage"}},
			cond:  config.AttrCondition{NotContains: testStr("security")},
			want:  true,
		},
		{
			name:  "not_contains fails when the member is present",
			attrs: map[string]any{testListAttrsKey: []any{"security"}},
			cond:  config.AttrCondition{NotContains: testStr("security")},
		},
		{
			name:  "an absent attribute contains nothing",
			attrs: map[string]any{},
			cond:  config.AttrCondition{NotContains: testStr("security")},
			want:  true,
		},
		{
			name:  "subset_of holds for a covered list",
			attrs: map[string]any{testListAttrsKey: []any{"legacy"}},
			cond:  config.AttrCondition{SubsetOf: []string{"legacy", "deprecated"}},
			want:  true,
		},
		{
			name:  "subset_of fails on a member outside the set",
			attrs: map[string]any{testListAttrsKey: []any{"legacy", "security"}},
			cond:  config.AttrCondition{SubsetOf: []string{"legacy", "deprecated"}},
		},
		{
			name:  "subset_of needs the attribute",
			attrs: map[string]any{},
			cond:  config.AttrCondition{SubsetOf: []string{"legacy"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNodeAttrs("0001", config.StatusAccepted, tt.attrs)
			g := testGraph([]*model.Node{n}, nil, nil)
			cfg := config.ADRPreset()
			cond := config.Condition{Attr: map[string]config.AttrCondition{testListAttrsKey: tt.cond}}

			if got := MatchCondition(g, cfg, cond, "0001", testAsOf); got != tt.want {
				t.Fatalf("MatchCondition = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchConditionWithAlternativesAndNegation(t *testing.T) {
	g := testGraph(
		[]*model.Node{
			testNodeAttrs("0001", config.StatusDeprecated, map[string]any{testListAttrsKey: []any{"storage"}}),
			testNodeAttrs("0002", config.StatusRejected, map[string]any{testListAttrsKey: []any{"explained"}}),
			testNode("0003", config.StatusAccepted),
			testNode("0004", config.StatusSuperseded),
		},
		[]model.Edge{testEdge("0003", "0004", config.EdgeSupersedes)},
		nil,
	)
	cfg := config.ADRPreset()
	retired := config.Condition{
		AnyOf: []config.Condition{
			{Attr: map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusDeprecated)}},
			{Attr: map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusRejected)}},
		},
		Not: &config.Condition{Attr: map[string]config.AttrCondition{testListAttrsKey: {Contains: testStr("explained")}}},
	}

	tests := []struct {
		name string
		cond config.Condition
		id   model.ID
		want bool
	}{
		{name: "the first alternative matches", cond: retired, id: "0001", want: true},
		{name: "the negation blocks the second alternative", cond: retired, id: "0002"},
		{name: "no alternative matches", cond: retired, id: "0003"},
		{
			name: "top-level clauses still and with the alternatives",
			cond: config.Condition{
				NotInbound: config.EdgeSupersedes.String(),
				AnyOf:      retired.AnyOf,
			},
			id:   "0001",
			want: true,
		},
		{
			name: "a top-level clause can veto every alternative",
			cond: config.Condition{
				Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()},
				AnyOf:   retired.AnyOf,
			},
			id: "0001",
		},
		{
			name: "a negation of an edge clause",
			cond: config.Condition{Not: &config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}}},
			id:   "0004",
		},
		{
			name: "a negation that does not hold leaves the condition alone",
			cond: config.Condition{Not: &config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}}},
			id:   "0003",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchCondition(g, cfg, tt.cond, tt.id, testAsOf); got != tt.want {
				t.Fatalf("MatchCondition(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestEvalRuleLocatesANestedAttributeClause(t *testing.T) {
	g := testGraph([]*model.Node{testNode("0001", config.StatusDeprecated)}, nil, nil)
	rule := config.Rule{
		Name:     "unexplained_retirement",
		Severity: model.SeverityWarn,
		When: config.Condition{
			AnyOf: []config.Condition{
				{Attr: map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusDeprecated)}},
			},
		},
		Message: "is retired without an explanation",
	}

	f := testAssertSingleFinding(t, EvalRule(g, config.ADRPreset(), rule, testAsOf), rule.Name, model.SeverityWarn, "0001")
	if f.Location != testNodeLocation("0001", testStatusLine) {
		t.Errorf("location = %+v, want the status line the alternative reads", f.Location)
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

		testAssertFindings(t, "EvalRule", EvalRule(g, cfg, drift, testAsOf), want)
	})

	t.Run("a rule matching nothing reports nothing", func(t *testing.T) {
		unmatched := config.Rule{
			Name:     "never",
			Severity: model.SeverityWarn,
			When:     config.Condition{Attr: map[string]config.AttrCondition{"status": testAttrEq("withdrawn")}},
			Message:  "no document has this status",
		}

		if got := EvalRule(g, cfg, unmatched, testAsOf); len(got) != 0 {
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

		testAssertFindings(t, "EvalRules", EvalRules(g, cfg, testAsOf), want)
	})

	t.Run("configured rules replace the preset rules", func(t *testing.T) {
		custom := config.ADRPreset()
		custom.Rules = []config.Rule{{
			Name:     "accepted_with_dependencies",
			Severity: model.SeverityWarn,
			When: config.Condition{
				Outbound: config.EdgeCondition{Edge: config.EdgeDependsOn.String()},
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

		testAssertFindings(t, "EvalRules", EvalRules(g, custom, testAsOf), want)
	})

	t.Run("no configured rules report nothing", func(t *testing.T) {
		none := config.ADRPreset()
		none.Rules = nil

		if got := EvalRules(g, none, testAsOf); len(got) != 0 {
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

	got := Validate(g, cfg, time.Time{})

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

		if findings := Validate(clean, cfg, time.Time{}); len(findings) != 0 {
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
			testNodeAttrs("0006", config.StatusRejected, map[string]any{"owner": "platform"}),
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

		got := EvalRule(g, cfg, cfg.Rules[0], testAsOf)

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
			When:     config.Condition{Outbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String()}},
			Message:  "supersedes another document",
		}

		got := EvalRule(g, config.ADRPreset(), rule, testAsOf)

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

// testDegreeFixture carries a fan-in of depends-on edges, one of them from a
// document the corpus does not hold.
func testDegreeFixture() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusAccepted),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusAccepted),
			testNode("0004", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeDependsOn),
			testEdge("0003", "0001", config.EdgeDependsOn),
			testEdge("0004", "0001", config.EdgeDependsOn),
			testEdge("0002", "0003", config.EdgeDependsOn),
			testEdge("0099", "0004", config.EdgeDependsOn),
		},
		nil,
	)
}

func TestMatchConditionOnDegreeThresholds(t *testing.T) {
	g := testDegreeFixture()
	cfg := config.ADRPreset()
	inbound := func(min, max *int) config.Condition {
		return config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeDependsOn.String(), Min: min, Max: max}}
	}

	tests := []struct {
		name string
		cond config.Condition
		id   string
		want bool
	}{
		{name: "the string form is one edge or more", cond: config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeDependsOn.String()}}, id: "0001", want: true},
		{name: "a minimum met exactly", cond: inbound(testGraphInt(3), nil), id: "0001", want: true},
		{name: "a minimum missed by one", cond: inbound(testGraphInt(4), nil), id: "0001"},
		{name: "a minimum on a document with one edge", cond: inbound(testGraphInt(1), nil), id: "0003", want: true},
		{name: "a minimum on a document with none", cond: inbound(testGraphInt(1), nil), id: "0002"},
		{name: "a maximum met exactly", cond: inbound(nil, testGraphInt(3)), id: "0001", want: true},
		{name: "a maximum exceeded", cond: inbound(nil, testGraphInt(2)), id: "0001"},
		{name: "a maximum still needs one edge", cond: inbound(nil, testGraphInt(3)), id: "0002"},
		{name: "a window met", cond: inbound(testGraphInt(1), testGraphInt(1)), id: "0003", want: true},
		{name: "a window missed from above", cond: inbound(testGraphInt(1), testGraphInt(2)), id: "0001"},
		{
			name: "an edge from a document the corpus does not hold still counts",
			cond: inbound(testGraphInt(1), nil),
			id:   "0004",
			want: true,
		},
		{
			name: "an outbound threshold counts the other direction",
			cond: config.Condition{Outbound: config.EdgeCondition{Edge: config.EdgeDependsOn.String(), Min: testGraphInt(2)}},
			id:   "0002",
			want: true,
		},
		{
			name: "a threshold is edge-type specific",
			cond: config.Condition{Inbound: config.EdgeCondition{Edge: config.EdgeSupersedes.String(), Min: testGraphInt(1)}},
			id:   "0001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchCondition(g, cfg, tt.cond, model.ID(tt.id), testAsOf); got != tt.want {
				t.Fatalf("MatchCondition(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// testViaFixture carries one accepted document depending on a deprecated one,
// one depending only on a current one, and one depending on a document the
// corpus does not hold.
func testViaFixture() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusAccepted),
			testNode("0002", config.StatusDeprecated),
			testNodeAttrs("0003", config.StatusAccepted, map[string]any{"owner": "platform"}),
			testNode("0004", config.StatusAccepted),
			testNode("0005", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0001", "0002", config.EdgeDependsOn),
			testEdge("0001", "0003", config.EdgeDependsOn),
			testEdge("0004", "0003", config.EdgeDependsOn),
			testEdge("0005", "0099", config.EdgeDependsOn),
		},
		nil,
	)
}

func TestMatchConditionOnOneHopClauses(t *testing.T) {
	g := testViaFixture()
	cfg := config.ADRPreset()
	via := func(attr map[string]config.AttrCondition) config.Condition {
		return config.Condition{Via: &config.ViaCondition{Edge: config.EdgeDependsOn.String(), Attr: attr}}
	}
	viaInbound := func(attr map[string]config.AttrCondition) config.Condition {
		return config.Condition{ViaInbound: &config.ViaCondition{Edge: config.EdgeDependsOn.String(), Attr: attr}}
	}
	deprecated := map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusDeprecated)}
	accepted := map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusAccepted)}

	tests := []struct {
		name string
		cond config.Condition
		id   string
		want bool
	}{
		{name: "one neighbour satisfying the clause is enough", cond: via(deprecated), id: "0001", want: true},
		{name: "no neighbour satisfying the clause", cond: via(deprecated), id: "0004"},
		{name: "a document with no such edge at all", cond: via(deprecated), id: "0002"},
		{
			name: "a neighbour the corpus does not hold is not a witness",
			cond: via(deprecated),
			id:   "0005",
		},
		{name: "the inbound direction reads the other way", cond: viaInbound(accepted), id: "0003", want: true},
		{name: "the inbound direction does not read the outbound edges", cond: viaInbound(accepted), id: "0001"},
		{
			name: "every attribute of the clause has to hold on one neighbour",
			cond: via(map[string]config.AttrCondition{
				config.DefaultStatusField: testAttrEq(config.StatusAccepted),
				"owner":                   testAttrEq("platform"),
			}),
			id:   "0001",
			want: true,
		},
		{
			name: "no single neighbour holds every attribute",
			cond: via(map[string]config.AttrCondition{
				config.DefaultStatusField: testAttrEq(config.StatusDeprecated),
				"owner":                   testAttrEq("platform"),
			}),
			id: "0001",
		},
		{
			name: "an attribute the neighbour does not carry",
			cond: via(map[string]config.AttrCondition{"owner": testAttrEq("storage")}),
			id:   "0001",
		},
		{
			name: "a negative clause is satisfied by a neighbour without the attribute",
			cond: via(map[string]config.AttrCondition{"owner": testAttrNot("platform")}),
			id:   "0001",
			want: true,
		},
		{
			name: "a clause without attributes asks only that a neighbour exists",
			cond: via(nil),
			id:   "0004",
			want: true,
		},
		{name: "an unknown document matches nothing", cond: via(deprecated), id: "0099"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchCondition(g, cfg, tt.cond, model.ID(tt.id), testAsOf); got != tt.want {
				t.Fatalf("MatchCondition(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestEvalRuleLocatesAOneHopClauseAtTheEdgeKey(t *testing.T) {
	g := testViaFixture()
	cfg := config.ADRPreset()
	rule := config.Rule{
		Name:     "stale_dependency",
		Severity: model.SeverityWarn,
		When: config.Condition{Via: &config.ViaCondition{
			Edge: config.EdgeDependsOn.String(),
			Attr: map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusDeprecated)},
		}},
		Message: "depends on a deprecated decision",
	}

	f := testAssertSingleFinding(t, EvalRule(g, cfg, rule, testAsOf), rule.Name, model.SeverityWarn, "0001")

	if want := testNodeLocation("0001", testDependsOnLine); f.Location != want {
		t.Fatalf("location = %+v, want %+v (the edge the reader can change)", f.Location, want)
	}
}

func testGraphInt(v int) *int { return &v }

func TestCheckDocumentsAcrossKinds(t *testing.T) {
	cfg := testKindsConfig()

	t.Run("a document takes the kind of its directory", func(t *testing.T) {
		docs := []*parse.Document{testKindDoc("clause", "UZ-V-001", map[string]any{"kind": "clause", "status": "accepted"})}

		if got := CheckDocuments(docs, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a written kind the directory disagrees with is an error", func(t *testing.T) {
		docs := []*parse.Document{testKindDoc("clause", "UZ-V-001", map[string]any{"kind": "conform", "status": "accepted"})}

		f := testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleKindMismatch, model.SeverityError, "UZ-V-001")
		if want := `frontmatter kind "conform" disagrees with directory kind "clause"`; f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		if f.Location.Line != docs[0].KeyLines["kind"] {
			t.Errorf("location = %+v, want the kind key on line %d", f.Location, docs[0].KeyLines["kind"])
		}
	})

	t.Run("a document that writes no kind agrees with its directory", func(t *testing.T) {
		docs := []*parse.Document{testKindDoc("clause", "UZ-V-001", map[string]any{"status": "accepted"})}

		if got := CheckDocuments(docs, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a closed kind reports every key nobody declared, on its own line", func(t *testing.T) {
		docs := []*parse.Document{
			testKindDoc("clause", "UZ-V-001", map[string]any{"status": "accepted", "owner": "platform", "reviewer": "ana"}),
		}

		findings := testFindingsFor(CheckDocuments(docs, cfg), model.RuleUnknownField)

		if len(findings) != 2 {
			t.Fatalf("findings = %+v, want one per unknown key", findings)
		}
		for i, key := range []string{"owner", "reviewer"} {
			if !strings.Contains(findings[i].Detail, `"`+key+`"`) {
				t.Errorf("detail = %q, want it to name %q", findings[i].Detail, key)
			}
			if findings[i].Location.Line != docs[0].KeyLines[key] {
				t.Errorf("%s location = %+v, want line %d", key, findings[i].Location, docs[0].KeyLines[key])
			}
		}
		if want := "declared: date, enforces, id, kind, status, supersedes, title"; !strings.HasSuffix(findings[0].Detail, want) {
			t.Errorf("detail = %q, want it to end with %q", findings[0].Detail, want)
		}
	})

	t.Run("an open kind ignores the keys nobody declared", func(t *testing.T) {
		docs := []*parse.Document{
			testKindDoc("conform", "conform/check", map[string]any{"status": "accepted", "owner": "platform"}),
		}

		if got := CheckDocuments(docs, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the kind is open", got)
		}
	})

	t.Run("a file that yields no identity is reported rather than skipped", func(t *testing.T) {
		stray := testKindDoc("clause", "", nil)
		stray.Identity = "README"
		stray.Path = "spec/clauses/README.md"

		findings := CheckDocuments([]*parse.Document{stray}, cfg)

		f := testAssertSingleFinding(t, findings, model.RuleIDMismatch, model.SeverityError, "")
		want := `"README" is not an identifier of kind "clause", which reads ^UZ-[A-Z]-\d{3}$`
		if f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		if f.Location.Path != "spec/clauses/README.md" {
			t.Errorf("location = %+v, want the file it is about", f.Location)
		}
	})

	t.Run("a written id the kind's pattern rejects is the same finding", func(t *testing.T) {
		wrong := testKindDoc("clause", "", map[string]any{"id": "uz-v-001", "status": "accepted"})
		wrong.Identity = "uz-v-001"

		f := testAssertSingleFinding(t, CheckDocuments([]*parse.Document{wrong}, cfg), model.RuleIDMismatch, model.SeverityError, "")
		if f.Location.Line != wrong.KeyLines["id"] {
			t.Errorf("location = %+v, want the id key on line %d", f.Location, wrong.KeyLines["id"])
		}
	})

	t.Run("frontmatter that does not decode is reported as the cause it is", func(t *testing.T) {
		// The identity could not be read because the block did not parse, so
		// the decode failure is the finding rather than the missing identifier.
		broken := testKindDoc("conform", "", nil)
		broken.Identity = "check"
		broken.Err = errInvalidFixtureFrontmatter

		findings := CheckDocuments([]*parse.Document{broken}, cfg)

		testAssertSingleFinding(t, findings, model.RuleInvalidFrontmatter, model.SeverityError, "")
		if got := testFindingsFor(findings, model.RuleIDMismatch); len(got) != 0 {
			t.Fatalf("findings = %+v, want the decode failure alone", got)
		}
	})

	t.Run("two files without an identity do not collide", func(t *testing.T) {
		// Neither carries an identifier, so they share nothing to collide over.
		strays := []*parse.Document{testKindDoc("clause", "", nil), testKindDoc("conform", "", nil)}

		findings := CheckDocuments(strays, cfg)

		if got := testFindingsFor(findings, model.RuleIDCollision); len(got) != 0 {
			t.Fatalf("findings = %+v, want no collision between two files without an identity", got)
		}
		if got := testFindingsFor(findings, model.RuleIDMismatch); len(got) != 2 {
			t.Fatalf("findings = %+v, want one id_mismatch each", got)
		}
	})

	t.Run("two kinds normalizing to one identifier collide", func(t *testing.T) {
		docs := []*parse.Document{
			testKindDoc("clause", "UZ-V-001", map[string]any{"status": "accepted"}),
			testKindDoc("conform", "UZ-V-001", map[string]any{"status": "accepted"}),
		}

		f := testAssertSingleFinding(t, CheckDocuments(docs, cfg), model.RuleIDCollision, model.SeverityError, "UZ-V-001")
		if !strings.Contains(f.Detail, "spec/conform/UZ-V-001.md") {
			t.Errorf("detail = %q, want it to name the document of the other kind", f.Detail)
		}
	})
}

func TestCheckEdgeKinds(t *testing.T) {
	cfg := testKindsConfig()

	t.Run("an edge between the declared kinds reports nothing", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testKindNode("conform", "conform/check", "accepted"), testKindNode("clause", "UZ-V-001", "accepted")},
			[]model.Edge{testEdge("conform/check", "UZ-V-001", "enforces")},
			nil,
		)

		if got := CheckEdgeKinds(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a source of the wrong kind is an error against the document that declared it", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testKindNode("clause", "UZ-V-002", "accepted"), testKindNode("clause", "UZ-V-001", "accepted")},
			[]model.Edge{testEdge("UZ-V-002", "UZ-V-001", "enforces")},
			nil,
		)

		f := testAssertSingleFinding(t, CheckEdgeKinds(g, cfg), model.RuleEdgeKindMismatch, model.SeverityError, "UZ-V-002")
		if want := `enforces source UZ-V-002 is kind "clause", want one of: conform`; f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		// The finding points at the declaring document's own frontmatter; the
		// test nodes carry no enforces key, so it falls back on the status
		// field the way every edge finding does.
		if f.Location.Path != "spec/clause/UZ-V-002.md" || f.Location.Line != testStatusLine {
			t.Errorf("location = %+v, want the declaring document's key line", f.Location)
		}
	})

	t.Run("a target of the wrong kind is an error", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testKindNode("conform", "conform/check", "accepted"), testKindNode("deviation", "dev-0001", "accepted")},
			[]model.Edge{testEdge("conform/check", "dev-0001", "enforces")},
			nil,
		)

		f := testAssertSingleFinding(t, CheckEdgeKinds(g, cfg), model.RuleEdgeKindMismatch, model.SeverityError, "conform/check")
		if want := `enforces target dev-0001 is kind "deviation", want one of: clause`; f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		if len(f.Related) != 1 || f.Related[0].Path != "spec/deviation/dev-0001.md" {
			t.Errorf("related = %+v, want the endpoint of the wrong kind", f.Related)
		}
	})

	t.Run("both endpoints of the wrong kind are two findings", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testKindNode("deviation", "dev-0001", "accepted"), testKindNode("deviation", "dev-0002", "accepted")},
			[]model.Edge{testEdge("dev-0001", "dev-0002", "enforces")},
			nil,
		)

		if got := testFindingsFor(CheckEdgeKinds(g, cfg), model.RuleEdgeKindMismatch); len(got) != 2 {
			t.Fatalf("findings = %+v, want one per endpoint", got)
		}
	})

	t.Run("an endpoint the corpus does not hold has no kind to be wrong about", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testKindNode("conform", "conform/check", "accepted")},
			[]model.Edge{testEdge("conform/check", "UZ-V-404", "enforces")},
			nil,
		)

		if got := CheckEdgeKinds(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: a dangling reference is its own finding", got)
		}
	})

	t.Run("an edge that constrains nothing reports nothing", func(t *testing.T) {
		unconstrained := testKindsConfig()
		unconstrained.Edges[1].From, unconstrained.Edges[1].To = nil, nil
		g := testGraph(
			[]*model.Node{testKindNode("deviation", "dev-0001", "accepted"), testKindNode("deviation", "dev-0002", "accepted")},
			[]model.Edge{testEdge("dev-0001", "dev-0002", "enforces")},
			nil,
		)

		if got := CheckEdgeKinds(g, unconstrained); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a corpus without kinds is unconstrained", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", "accepted"), testNode("0002", "accepted")},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		if got := CheckEdgeKinds(g, config.ADRPreset()); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a reverse edge is filed against the document whose key declared it", func(t *testing.T) {
		reversed := testKindsConfig()
		reversed.Edges[1].Direction = config.DirectionReverse
		g := testGraph(
			[]*model.Node{testKindNode("clause", "UZ-V-001", "accepted"), testKindNode("deviation", "dev-0001", "accepted")},
			[]model.Edge{testEdge("dev-0001", "UZ-V-001", "enforces")},
			nil,
		)

		// The key lives on the target of a reverse edge, so that is the
		// document a reader has to open.
		testAssertSingleFinding(t, CheckEdgeKinds(g, reversed), model.RuleEdgeKindMismatch, model.SeverityError, "UZ-V-001")
	})
}
