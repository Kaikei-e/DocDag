package graph

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// testLeafOfConfig puts a leaf_of condition on the preset's depends-on edge: a
// dependency has to name the current leaf of a supersedes lineage.
func testLeafOfConfig() config.Config {
	cfg := config.ADRPreset()
	cfg.Edges[1].Target = &config.TargetCondition{LeafOf: config.EdgeSupersedes.String()}
	return cfg
}

// testReplacedGraph is one replaced decision, its replacement, and a document
// that still depends on the one that was replaced.
func testReplacedGraph() *model.Graph {
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

func TestCheckTargets(t *testing.T) {
	t.Run("an edge declaring no target reports nothing", func(t *testing.T) {
		if got := CheckTargets(testReplacedGraph(), config.ADRPreset(), testAsOf); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: no edge declares a target", got)
		}
	})

	t.Run("a dependency on a replaced document is an error on the document that declared it", func(t *testing.T) {
		g := testReplacedGraph()

		f := testAssertSingleFinding(t, CheckTargets(g, testLeafOfConfig(), testAsOf), model.RuleStaleTarget, model.SeverityError, "0003")

		if f.Detail != "depends-on targets 0001, which 0002 supersedes" {
			t.Errorf("detail = %q, want the edge, the target and what replaced it", f.Detail)
		}
		if f.Location != testNodeLocation("0003", testDependsOnLine) {
			t.Errorf("location = %+v, want the edge key line of the declaring document", f.Location)
		}
		// The target is where the reader looks first, and the replacement is
		// what they are looking for.
		want := []model.Location{
			testNodeLocation("0001", testStatusLine),
			testNodeLocation("0002", testSupersedesLine),
		}
		if !slices.Equal(f.Related, want) {
			t.Errorf("related = %+v, want the target and what replaced it %+v", f.Related, want)
		}
	})

	t.Run("a dependency on the leaf reports nothing", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusAccepted),
				testNode("0003", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0003", "0002", config.EdgeDependsOn),
			},
			nil,
		)

		if got := CheckTargets(g, testLeafOfConfig(), testAsOf); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the dependency names the leaf", got)
		}
	})

	t.Run("a target the corpus does not hold is left to dangling_ref", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0003", config.StatusAccepted)},
			[]model.Edge{testEdge("0003", "0009", config.EdgeDependsOn)},
			nil,
		)

		if got := CheckTargets(g, testLeafOfConfig(), testAsOf); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: there is no target to hold a condition against", got)
		}
	})

	t.Run("a plain condition on the target names no successor", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Edges[1].Target = &config.TargetCondition{
			Condition: config.Condition{Attr: map[string]config.AttrCondition{
				config.DefaultStatusField: testAttrEq(config.StatusAccepted),
			}},
		}
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusDeprecated),
				testNode("0003", config.StatusAccepted),
			},
			[]model.Edge{testEdge("0003", "0001", config.EdgeDependsOn)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckTargets(g, cfg, testAsOf), model.RuleStaleTarget, model.SeverityError, "0003")

		if f.Detail != "depends-on targets 0001, which does not satisfy the edge's target condition" {
			t.Errorf("detail = %q, want the generic wording: the condition is whatever the corpus wrote", f.Detail)
		}
		if want := []model.Location{testNodeLocation("0001", testStatusLine)}; !slices.Equal(f.Related, want) {
			t.Errorf("related = %+v, want the target alone %+v", f.Related, want)
		}
	})

	t.Run("a projection reads as a virtual attribute of the target", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Edges[1].Target = &config.TargetCondition{
			Condition: config.Condition{Attr: map[string]config.AttrCondition{
				config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionTrue),
			}},
		}

		f := testAssertSingleFinding(t, CheckTargets(testReplacedGraph(), cfg, testAsOf), model.RuleStaleTarget, model.SeverityError, "0003")
		if !strings.Contains(f.Detail, "0001") {
			t.Errorf("detail = %q, want the target the binding projection does not hold for", f.Detail)
		}
	})

	t.Run("the condition reaches a derived edge, on the field that produced it", func(t *testing.T) {
		// Nothing may depend on a document that something supersedes, checked
		// on the supersedes edge a MADR status string derives.
		cfg := config.ADRPreset()
		cfg.Edges[0].Target = &config.TargetCondition{
			Condition: config.Condition{NotInbound: config.EdgeDependsOn.String()},
		}
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusAccepted),
				testNode("0003", config.StatusAccepted),
			},
			[]model.Edge{
				testDerivedEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0003", "0001", config.EdgeDependsOn),
			},
			nil,
		)

		// The derived edge was written by 0001's status field, so that is the
		// document the finding is filed against and the line it points at.
		f := testAssertSingleFinding(t, CheckTargets(g, cfg, testAsOf), model.RuleStaleTarget, model.SeverityError, "0001")
		if f.Location != testNodeLocation("0001", testStatusLine) {
			t.Errorf("location = %+v, want the derived field's line", f.Location)
		}
		// A document does not relate itself; the target here is the declaring
		// document.
		if len(f.Related) != 0 {
			t.Errorf("related = %+v, want none: the target declared the edge", f.Related)
		}
	})

	t.Run("a reverse edge holds the condition against the document that wrote the key", func(t *testing.T) {
		// The key names the edge's source, so the document it points at — and
		// the one the condition is about — is the one carrying the key.
		cfg := config.ADRPreset()
		cfg.Edges = []config.EdgeSpec{{
			Name:      config.EdgeSupersedes.String(),
			Key:       testInverseKey,
			Acyclic:   true,
			Direction: config.DirectionReverse,
			Target: &config.TargetCondition{
				Condition: config.Condition{Attr: map[string]config.AttrCondition{
					config.DefaultStatusField: testAttrEq(config.StatusSuperseded),
				}},
			},
		}}
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusAccepted),
				testNode("0002", config.StatusSuperseded),
			},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		f := testAssertSingleFinding(t, CheckTargets(g, cfg, testAsOf), model.RuleStaleTarget, model.SeverityError, "0001")
		if f.Detail != "supersedes targets 0001, which does not satisfy the edge's target condition" {
			t.Errorf("detail = %q, want the head of the edge rather than the identifier the key names", f.Detail)
		}
		if f.Location != testNodeLocation("0001", testInverseLine) {
			t.Errorf("location = %+v, want the reverse key's line", f.Location)
		}
	})

	t.Run("findings are sorted", func(t *testing.T) {
		g := testReplacedGraph()
		g.Nodes["0004"] = testNode("0004", config.StatusAccepted)
		g.Edges = append(g.Edges, testEdge("0004", "0001", config.EdgeDependsOn))

		got := CheckTargets(g, testLeafOfConfig(), testAsOf)

		testAssertIDs(t, "stale target ids", testFindingIDs(got, model.RuleStaleTarget), testIDs("0003", "0004"))
		testAssertSortedFindings(t, got)
	})

	t.Run("the escalation table holds it at error", func(t *testing.T) {
		cfg := testLeafOfConfig()
		cfg.Structural = map[string]model.Severity{model.RuleStaleTarget: model.SeverityWarn}

		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate accepted a lowered stale_target, want a configuration error")
		}
	})
}

func TestSuggestTheLeafOfAStaleTarget(t *testing.T) {
	t.Run("a single leaf is named", func(t *testing.T) {
		g := testReplacedGraph()
		cfg := testLeafOfConfig()

		got := Suggest(CheckTargets(g, cfg, testAsOf), g, cfg, testAsOf)

		if len(got) != 1 || got[0].Fix != "did you mean 0002?" {
			t.Fatalf("fix = %+v, want the leaf of the lineage", got)
		}
	})

	t.Run("a lineage walked to its end names the last document", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusSuperseded),
				testNode("0003", config.StatusAccepted),
				testNode("0004", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0003", "0002", config.EdgeSupersedes),
				testEdge("0004", "0001", config.EdgeDependsOn),
			},
			nil,
		)
		cfg := testLeafOfConfig()

		got := Suggest(CheckTargets(g, cfg, testAsOf), g, cfg, testAsOf)

		// The check itself is local — it saw only that 0002 supersedes 0001 —
		// and only the suggestion walks the rest of the lineage.
		if len(got) != 1 || got[0].Fix != "did you mean 0003?" {
			t.Fatalf("fix = %+v, want the leaf two hops away", got)
		}
		if got[0].Detail != "depends-on targets 0001, which 0002 supersedes" {
			t.Errorf("detail = %q, want the one hop the check read", got[0].Detail)
		}
	})

	t.Run("a branched lineage lists its leaves", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusAccepted),
				testNode("0003", config.StatusAccepted),
				testNode("0004", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0003", "0001", config.EdgeSupersedes),
				testEdge("0004", "0001", config.EdgeDependsOn),
			},
			nil,
		)
		cfg := testLeafOfConfig()

		got := Suggest(CheckTargets(g, cfg, testAsOf), g, cfg, testAsOf)

		if len(got) != 1 || got[0].Fix != "did you mean one of: 0002, 0003?" {
			t.Fatalf("fix = %+v, want both leaves listed", got)
		}
		if got[0].Detail != "depends-on targets 0001, which 0002, 0003 supersedes" {
			t.Errorf("detail = %q, want both successors named", got[0].Detail)
		}
	})

	t.Run("a lineage that loops keeps the finding and drops the suggestion", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusSuperseded),
				testNode("0002", config.StatusSuperseded),
				testNode("0003", config.StatusAccepted),
			},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testEdge("0001", "0002", config.EdgeSupersedes),
				testEdge("0003", "0001", config.EdgeDependsOn),
			},
			nil,
		)
		cfg := testLeafOfConfig()

		got := Suggest(CheckTargets(g, cfg, testAsOf), g, cfg, testAsOf)

		if len(got) != 1 {
			t.Fatalf("findings = %+v, want the stale target to stand", got)
		}
		if got[0].Fix != "" {
			t.Errorf("fix = %q, want none: the lineage has no leaf to name", got[0].Fix)
		}
	})

	t.Run("a plain condition carries no suggestion", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Edges[1].Target = &config.TargetCondition{
			Condition: config.Condition{Attr: map[string]config.AttrCondition{
				config.DefaultStatusField: testAttrEq(config.StatusAccepted),
			}},
		}
		g := testReplacedGraph()

		got := Suggest(CheckTargets(g, cfg, testAsOf), g, cfg, testAsOf)

		if len(got) != 1 || got[0].Fix != "" {
			t.Fatalf("fix = %+v, want none: which document satisfies the condition is not the graph's answer", got)
		}
	})
}

func TestValidateRunsTheTargetCheck(t *testing.T) {
	cfg := testLeafOfConfig()

	got := Validate(testReplacedGraph(), cfg, time.Time{})

	if ids := testFindingIDs(got, model.RuleStaleTarget); len(ids) != 1 {
		t.Fatalf("stale target ids = %v, want the check to run as part of Validate", ids)
	}
}
