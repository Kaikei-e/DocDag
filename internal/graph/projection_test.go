package graph

import (
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// testProjectionFixture is a corpus with one superseded decision, one accepted
// MUST and one accepted decision carrying no level at all.
func testProjectionFixture() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNodeAttrs("0001", config.StatusAccepted, map[string]any{"level": "MUST"}),
			testNodeAttrs("0002", config.StatusSuperseded, map[string]any{"level": "MUST"}),
			testNode("0003", config.StatusAccepted),
			testNodeAttrs("0004", config.StatusAccepted, map[string]any{"level": "SHOULD"}),
		},
		[]model.Edge{testEdge("0001", "0002", config.EdgeSupersedes)},
		nil,
	)
}

// testCurrent is the ADR preset's own projection, written out here so a test
// can layer others on top of it.
func testCurrent() config.ProjectionSpec {
	return testProjection(config.ProjectionAcceptedUnsuperseded, config.Condition{
		NotInbound: config.EdgeSupersedes.String(),
		Attr:       map[string]config.AttrCondition{config.DefaultStatusField: testAttrEq(config.StatusAccepted)},
	})
}

func TestProjectionValue(t *testing.T) {
	if got := ProjectionValue(true); got != ProjectionTrue {
		t.Errorf("ProjectionValue(true) = %q, want %q", got, ProjectionTrue)
	}
	if got := ProjectionValue(false); got != ProjectionFalse {
		t.Errorf("ProjectionValue(false) = %q, want %q", got, ProjectionFalse)
	}
}

func TestEvalProjections(t *testing.T) {
	g := testProjectionFixture()
	cfg := config.ADRPreset()

	t.Run("the preset's projection holds for the current documents", func(t *testing.T) {
		got := EvalProjections(g, cfg, testAsOf)

		testAssertIDs(t, "binding projection", got.Set(config.ProjectionAcceptedUnsuperseded), testIDs("0001", "0003", "0004"))
		if !got.Declares(config.ProjectionAcceptedUnsuperseded) {
			t.Error("Declares = false, want the declared projection to be a virtual attribute")
		}
		if got.Declares("enforced") {
			t.Error("Declares(enforced) = true, want only the declared projections")
		}
	})

	t.Run("a projection reads another one as an attribute", func(t *testing.T) {
		chained := cfg
		chained.Projections = []config.ProjectionSpec{
			testCurrent(),
			testProjection("effective_must", config.Condition{Attr: map[string]config.AttrCondition{
				"level":                               testAttrEq("MUST"),
				config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionTrue),
			}}),
		}

		got := EvalProjections(g, chained, testAsOf)

		testAssertIDs(t, "effective_must", got.Set("effective_must"), testIDs("0001"))
	})

	t.Run("declaration order does not decide evaluation order", func(t *testing.T) {
		forward := cfg
		forward.Projections = []config.ProjectionSpec{
			testProjection("effective_must", config.Condition{Attr: map[string]config.AttrCondition{
				"level":                               testAttrEq("MUST"),
				config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionTrue),
			}}),
			testCurrent(),
		}

		got := EvalProjections(g, forward, testAsOf)

		testAssertIDs(t, "effective_must", got.Set("effective_must"), testIDs("0001"))
	})

	t.Run("a chain three deep is evaluated in one pass", func(t *testing.T) {
		deep := cfg
		deep.Projections = []config.ProjectionSpec{
			testProjection("settled", config.Condition{Attr: map[string]config.AttrCondition{"effective_must": testAttrEq(ProjectionTrue)}}),
			testProjection("effective_must", config.Condition{Attr: map[string]config.AttrCondition{
				"level":                               testAttrEq("MUST"),
				config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionTrue),
			}}),
			testCurrent(),
		}

		got := EvalProjections(g, deep, testAsOf)

		testAssertIDs(t, "settled", got.Set("settled"), testIDs("0001"))
	})

	t.Run("alternatives hold when any of them does", func(t *testing.T) {
		alternatives := cfg
		alternatives.Projections = []config.ProjectionSpec{
			testCurrent(),
			{
				Name: "levelled",
				AnyOf: []config.ProjectionAlt{
					{When: config.Condition{Attr: map[string]config.AttrCondition{"level": testAttrEq("MUST")}}},
					{When: config.Condition{Attr: map[string]config.AttrCondition{"level": testAttrEq("SHOULD")}}},
				},
			},
		}

		got := EvalProjections(g, alternatives, testAsOf)

		testAssertIDs(t, "levelled", got.Set("levelled"), testIDs("0001", "0002", "0004"))
	})

	t.Run("names come back in declaration order", func(t *testing.T) {
		want := []string{config.ProjectionAcceptedUnsuperseded}

		if got := EvalProjections(g, cfg, testAsOf).Names(); !slices.Equal(got, want) {
			t.Fatalf("Names = %v, want %v", got, want)
		}
	})

	t.Run("a configuration without projections declares none", func(t *testing.T) {
		bare := cfg
		bare.Projections, bare.Binding = nil, ""

		got := EvalProjections(g, bare, testAsOf)

		if len(got.Names()) != 0 || got.Declares(config.ProjectionAcceptedUnsuperseded) {
			t.Fatalf("Names = %v, want none", got.Names())
		}
		if ids := got.Set(config.ProjectionAcceptedUnsuperseded); len(ids) != 0 {
			t.Fatalf("Set = %v, want none", ids)
		}
	})
}

func TestEvalProjectionsDoesNotLoopOnACycle(t *testing.T) {
	// Configuration validation rejects a reference cycle, so a cyclic
	// configuration only reaches the evaluator when a caller built one by hand.
	// It has to come back rather than chase the loop.
	g := testProjectionFixture()
	cfg := config.ADRPreset()
	cfg.Projections = []config.ProjectionSpec{
		testProjection("a", config.Condition{Attr: map[string]config.AttrCondition{"b": testAttrEq(ProjectionTrue)}}),
		testProjection("b", config.Condition{Attr: map[string]config.AttrCondition{"a": testAttrEq(ProjectionTrue)}}),
	}
	cfg.Binding = "a"

	var got Projections
	testMustNotHang(t, time.Second, func() { got = EvalProjections(g, cfg, testAsOf) })

	for _, name := range []string{"a", "b"} {
		if !got.Declares(name) {
			t.Errorf("Declares(%q) = false, want a declared projection to stay a virtual attribute", name)
		}
		if ids := got.Set(name); len(ids) != 0 {
			t.Errorf("Set(%q) = %v, want a projection nobody can evaluate to hold nowhere", name, ids)
		}
	}
}

func TestProjectionsAreVirtualAttributes(t *testing.T) {
	g := testGraph(
		[]*model.Node{
			testNodeAttrs("0001", config.StatusAccepted, map[string]any{config.ProjectionAcceptedUnsuperseded: "false"}),
			testNode("0002", config.StatusProposed),
		},
		nil,
		nil,
	)
	cfg := config.ADRPreset()

	t.Run("a projection shadows a frontmatter key spelled the same way", func(t *testing.T) {
		cond := config.Condition{Attr: map[string]config.AttrCondition{
			config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionTrue),
		}}

		if !MatchCondition(g, cfg, cond, "0001", testAsOf) {
			t.Fatal("the written attribute won, want the projection to shadow it")
		}
	})

	t.Run("a projection that does not hold reads as false", func(t *testing.T) {
		holds := config.Condition{Attr: map[string]config.AttrCondition{
			config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionFalse),
		}}
		negated := config.Condition{Attr: map[string]config.AttrCondition{
			config.ProjectionAcceptedUnsuperseded: testAttrNot(ProjectionTrue),
		}}

		if !MatchCondition(g, cfg, holds, "0002", testAsOf) || !MatchCondition(g, cfg, negated, "0002", testAsOf) {
			t.Fatal("a projection that does not hold did not read as false")
		}
	})

	t.Run("a rule reads a projection", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Rules = []config.Rule{{
			Name:     "not_current",
			Severity: model.SeverityWarn,
			When: config.Condition{Attr: map[string]config.AttrCondition{
				config.ProjectionAcceptedUnsuperseded: testAttrEq(ProjectionFalse),
			}},
			Message: "is not current",
		}}

		got := EvalRules(g, cfg, testAsOf)

		testAssertIDs(t, "not_current", testFindingIDs(got, "not_current"), testIDs("0002"))
	})
}
