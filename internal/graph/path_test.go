package graph

import (
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// testPathConfig declares one path constraint over the preset's two edges.
func testPathConfig(constraint config.PathConstraint) config.Config {
	cfg := config.ADRPreset()
	cfg.PathConstraints = []config.PathConstraint{constraint}
	return cfg
}

// testDependsOnReplaced is a dependency on a replaced decision: 0003 depends on
// 0001, which 0002 replaced.
func testDependsOnReplaced() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeDependsOn),
		},
		nil,
	)
}

func TestCheckPathConstraints(t *testing.T) {
	t.Run("a corpus declaring none reports nothing", func(t *testing.T) {
		if got := CheckPathConstraints(testDependsOnReplaced(), config.ADRPreset()); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a two-step path that must reach nothing", func(t *testing.T) {
		cfg := testPathConfig(config.PathConstraint{
			Name:   "dependency_targets_current",
			Path:   []string{config.EdgeDependsOn.String(), config.PathReverse + config.EdgeSupersedes.String()},
			Equals: config.PathEqualsNone,
		})

		f := testAssertSingleFinding(t, CheckPathConstraints(testDependsOnReplaced(), cfg),
			model.RulePathMismatch, model.SeverityError, "0003")

		if f.Detail != "dependency_targets_current: depends-on -> ^supersedes reaches 0002, want none" {
			t.Errorf("detail = %q, want the constraint, the path as written and what it reached", f.Detail)
		}
		// The document wrote the first step down and nothing else of the path.
		if f.Location != testNodeLocation("0003", testDependsOnLine) {
			t.Errorf("location = %+v, want the first step's key line", f.Location)
		}
		if want := []model.Location{testNodeLocation("0002", testStatusLine)}; !slices.Equal(f.Related, want) {
			t.Errorf("related = %+v, want the documents the path reached %+v", f.Related, want)
		}
		if f.Fix != "" {
			t.Errorf("fix = %q, want none: which of the two paths is wrong is not DocDag's guess", f.Fix)
		}
	})

	t.Run("a step composes over every document the step before it reached", func(t *testing.T) {
		// 0004 depends on both 0002 and 0003, and each of them supersedes one
		// document, so the composition reaches two.
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusAccepted),
				testNode("0003", config.StatusAccepted),
				testNode("0004", config.StatusAccepted),
				testNode("0005", config.StatusSuperseded),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0003", "0005", config.EdgeSupersedes),
				testEdge("0004", "0002", config.EdgeDependsOn),
				testEdge("0004", "0003", config.EdgeDependsOn),
			},
			nil,
		)
		cfg := testPathConfig(config.PathConstraint{
			Name:   "dependencies_replace_nothing",
			Path:   []string{config.EdgeDependsOn.String(), config.EdgeSupersedes.String()},
			Equals: config.PathEqualsNone,
		})

		f := testAssertSingleFinding(t, CheckPathConstraints(g, cfg), model.RulePathMismatch, model.SeverityError, "0004")
		if f.Detail != "dependencies_replace_nothing: depends-on -> supersedes reaches 0001, 0005, want none" {
			t.Errorf("detail = %q, want both documents the composition reached, sorted", f.Detail)
		}
	})

	t.Run("a one-step path is a constraint about a document's own edges", func(t *testing.T) {
		cfg := testPathConfig(config.PathConstraint{
			Name:   "nothing_supersedes",
			Path:   []string{config.EdgeSupersedes.String()},
			Equals: config.PathEqualsNone,
		})

		f := testAssertSingleFinding(t, CheckPathConstraints(testDependsOnReplaced(), cfg),
			model.RulePathMismatch, model.SeverityError, "0002")
		if f.Detail != "nothing_supersedes: supersedes reaches 0001, want none" {
			t.Errorf("detail = %q, want the one-step path", f.Detail)
		}
		if f.Location != testNodeLocation("0002", testSupersedesLine) {
			t.Errorf("location = %+v, want the only step's key line", f.Location)
		}
	})

	t.Run("a reversed first step falls back on a key the document does carry", func(t *testing.T) {
		g := testDependsOnReplaced()
		// A document does not write down the edges pointing at it, so there is
		// no key for the finding to sit on.
		delete(g.Nodes["0001"].KeyLines, config.EdgeSupersedes.String())
		cfg := testPathConfig(config.PathConstraint{
			Name:   "nothing_replaced_it",
			Path:   []string{config.PathReverse + config.EdgeSupersedes.String()},
			Equals: config.PathEqualsNone,
		})

		f := testAssertSingleFinding(t, CheckPathConstraints(g, cfg), model.RulePathMismatch, model.SeverityError, "0001")
		if f.Location != testNodeLocation("0001", testStatusLine) {
			t.Errorf("location = %+v, want the fallback line", f.Location)
		}
	})

	t.Run("a comparison path names what the constraint allows", func(t *testing.T) {
		// What the replaced decision depended on, the replacement depends on
		// too — except 0004.
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusAccepted),
				testNode("0004", config.StatusAccepted),
				testNode("0005", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0001", "0004", config.EdgeDependsOn),
				testEdge("0001", "0005", config.EdgeDependsOn),
				testEdge("0002", "0005", config.EdgeDependsOn),
			},
			nil,
		)
		cfg := testPathConfig(config.PathConstraint{
			Name:     "replacement_keeps_the_dependencies",
			Path:     []string{config.EdgeSupersedes.String(), config.EdgeDependsOn.String()},
			SubsetOf: []string{config.EdgeDependsOn.String()},
		})

		f := testAssertSingleFinding(t, CheckPathConstraints(g, cfg), model.RulePathMismatch, model.SeverityError, "0002")

		// Only the difference is reported: 0005 is on both sides.
		if f.Detail != "replacement_keeps_the_dependencies: supersedes -> depends-on reaches 0004, which depends-on does not" {
			t.Errorf("detail = %q, want the difference alone", f.Detail)
		}
		if want := []model.Location{testNodeLocation("0004", testStatusLine)}; !slices.Equal(f.Related, want) {
			t.Errorf("related = %+v, want the difference %+v", f.Related, want)
		}
	})

	t.Run("a path inside the comparison reports nothing", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusAccepted),
				testNode("0005", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0001", "0005", config.EdgeDependsOn),
				testEdge("0002", "0005", config.EdgeDependsOn),
			},
			nil,
		)
		cfg := testPathConfig(config.PathConstraint{
			Name:     "replacement_keeps_the_dependencies",
			Path:     []string{config.EdgeSupersedes.String(), config.EdgeDependsOn.String()},
			SubsetOf: []string{config.EdgeDependsOn.String()},
		})

		if got := CheckPathConstraints(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the path stays inside the comparison", got)
		}
	})

	t.Run("a reference-layer link is not a step", func(t *testing.T) {
		cfg := testPathConfig(config.PathConstraint{
			Name:   "nothing_depends_on_anything",
			Path:   []string{config.EdgeDependsOn.String()},
			Equals: config.PathEqualsNone,
		})
		linked := testGraph(
			[]*model.Node{testNode("0001", config.StatusAccepted), testNode("0003", config.StatusAccepted)},
			nil,
			[]model.Edge{testRefEdge("0003", "0001")},
		)

		if got := CheckPathConstraints(linked, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: prose carries no invariants", got)
		}

		typed := testGraph(
			[]*model.Node{testNode("0001", config.StatusAccepted), testNode("0003", config.StatusAccepted)},
			[]model.Edge{testEdge("0003", "0001", config.EdgeDependsOn)},
			[]model.Edge{testRefEdge("0003", "0001")},
		)
		if got := CheckPathConstraints(typed, cfg); len(got) != 1 {
			t.Fatalf("findings = %+v, want the typed edge of the same pair to be walked", got)
		}
	})

	t.Run("a step into a document the corpus does not hold reaches nothing", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0003", config.StatusAccepted)},
			[]model.Edge{testEdge("0003", "0009", config.EdgeDependsOn)},
			nil,
		)
		cfg := testPathConfig(config.PathConstraint{
			Name:   "nothing_depends_on_anything",
			Path:   []string{config.EdgeDependsOn.String()},
			Equals: config.PathEqualsNone,
		})

		if got := CheckPathConstraints(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: a reference naming no document is a dangling_ref", got)
		}
	})

	t.Run("findings are sorted", func(t *testing.T) {
		g := testDependsOnReplaced()
		g.Nodes["0004"] = testNode("0004", config.StatusAccepted)
		g.Edges = append(g.Edges, testEdge("0004", "0001", config.EdgeDependsOn))
		cfg := testPathConfig(config.PathConstraint{
			Name:   "dependency_targets_current",
			Path:   []string{config.EdgeDependsOn.String(), config.PathReverse + config.EdgeSupersedes.String()},
			Equals: config.PathEqualsNone,
		})

		got := CheckPathConstraints(g, cfg)

		testAssertIDs(t, "path mismatch ids", testFindingIDs(got, model.RulePathMismatch), testIDs("0003", "0004"))
		testAssertSortedFindings(t, got)
	})

	t.Run("Validate runs the check and Suggest leaves it alone", func(t *testing.T) {
		g := testDependsOnReplaced()
		cfg := testPathConfig(config.PathConstraint{
			Name:   "dependency_targets_current",
			Path:   []string{config.EdgeDependsOn.String(), config.PathReverse + config.EdgeSupersedes.String()},
			Equals: config.PathEqualsNone,
		})

		got := Suggest(Validate(g, cfg, time.Time{}), g, cfg, testAsOf)

		for _, f := range testFindingsFor(got, model.RulePathMismatch) {
			if f.Fix != "" {
				t.Errorf("fix = %q, want none", f.Fix)
			}
		}
		if ids := testFindingIDs(got, model.RulePathMismatch); len(ids) != 1 {
			t.Fatalf("path mismatch ids = %v, want the check to run as part of Validate", ids)
		}
	})
}
