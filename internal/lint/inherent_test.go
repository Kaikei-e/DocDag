package lint

import (
	"slices"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// TestUnsatisfiableCondition walks the table of contradictions a conjunction can
// hold. Each configuration writes one rule, so the whole condition is dead and
// the finding is the unfirable rule; what is asserted is the reason it names.
func TestUnsatisfiableCondition(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.Config
		phrase string
	}{
		{
			name: "one attribute pinned to two values",
			cfg: testConfig(testRule("two_values", model.SeverityError, config.Condition{
				Attr: map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)},
				Not:  &config.Condition{Attr: map[string]config.AttrCondition{config.DefaultStatusField: testNot(config.StatusProposed)}},
			})),
			phrase: `cannot be both "accepted" and "proposed"`,
		},
		{
			name: "an equality and its own denial",
			cfg: testConfig(testRule("eq_and_not", model.SeverityError, config.Condition{
				Attr: map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)},
				Not:  &config.Condition{Attr: map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)}},
			})),
			phrase: "cannot both hold",
		},
		{
			name:   "a value outside the status vocabulary",
			cfg:    testConfig(testRule("outside_status", model.SeverityError, testAttr(config.DefaultStatusField, testEq("retired")))),
			phrase: `attr status: "retired" is outside`,
		},
		{
			name: "a value outside a declared field vocabulary",
			cfg: func() config.Config {
				cfg := testConfig(testRule("outside_field", model.SeverityError, testAttr("level", testEq("MIGHT"))))
				cfg.Fields = map[string]config.FieldSpec{"level": {OneOf: []string{"MUST", "SHOULD"}}}
				return cfg
			}(),
			phrase: `attr level: "MIGHT" is outside MUST, SHOULD`,
		},
		{
			name: "a value outside the projection vocabulary",
			cfg: func() config.Config {
				cfg := testConfig(testRule("outside_projection", model.SeverityError, testAttr("current", testEq("yes"))))
				cfg.Projections = []config.ProjectionSpec{{
					Name: "current",
					When: config.Condition{NotInbound: "supersedes"},
				}}
				return cfg
			}(),
			phrase: `attr current: "yes" is outside true, false`,
		},
		{
			name: "a value outside the two words in_force answers with",
			cfg: testConfig(testRule("outside_in_force", model.SeverityError,
				testAttr(config.AttrInForce, testEq("yes")))),
			phrase: `attr in_force: "yes" is outside true, false`,
		},
		{
			name: "an edge required and forbidden",
			cfg: testConfig(testRule("both_ways", model.SeverityError, config.Condition{
				Inbound:    config.EdgeCondition{Edge: "supersedes"},
				NotInbound: "supersedes",
			})),
			phrase: "cannot both hold",
		},
		{
			name: "two degree windows that do not meet",
			cfg: testConfig(testRule("windows", model.SeverityError, config.Condition{
				Inbound: config.EdgeCondition{Edge: "supersedes", Min: testInt(5)},
				AnyOf: []config.Condition{{
					Inbound: config.EdgeCondition{Edge: "supersedes", Max: testInt(2)},
				}},
			})),
			phrase: "ask for at least 5 and at most 2 edges",
		},
		{
			name: "a threshold above the edge's own bound",
			cfg: testConfig(testRule("above_bound", model.SeverityError, config.Condition{
				Inbound: config.EdgeCondition{Edge: "depends-on", Min: testInt(3)},
			})),
			phrase: "above the edge's max_inbound 2",
		},
		{
			name: "a one-hop clause the edge itself forbids",
			cfg: testConfig(testRule("via_absent", model.SeverityError, config.Condition{
				NotOutbound: "supersedes",
				Via: &config.ViaCondition{
					Edge: "supersedes",
					Attr: map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)},
				},
			})),
			phrase: "needs an edge that not_outbound supersedes forbids",
		},
		{
			name: "a subset that leaves the value out",
			cfg: testConfig(testRule("subset", model.SeverityError, config.Condition{
				Attr: map[string]config.AttrCondition{
					"tags": {Eq: strptr("a")},
				},
				AnyOf: []config.Condition{{
					Attr: map[string]config.AttrCondition{"tags": {SubsetOf: []string{"b", "c"}}},
				}},
			})),
			phrase: "a subset of b, c",
		},
		{
			name: "a value it must both hold and not contain",
			cfg: testConfig(testRule("contains", model.SeverityError, config.Condition{
				Attr: map[string]config.AttrCondition{"tags": testEq("a")},
				Not:  &config.Condition{Attr: map[string]config.AttrCondition{"tags": testContains("a")}},
			})),
			phrase: "cannot be \"a\" and not_contains \"a\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v, want a configuration lint can read", err)
			}
			findings := testLint(tt.cfg)

			f := assertFinding(t, findings, FindingUnfirableRule, tt.cfg.Rules[0].Name, model.SeverityError, tt.phrase)
			if f.Fix == "" {
				t.Error("fix is empty, want a remedy on every lint finding that has one")
			}
		})
	}
}

// TestUnsatisfiableConditionInKinds covers the two contradictions only a
// multi-kind corpus can write: a kind an edge cannot join, and a value outside
// the vocabulary of the kind the rule pinned.
func TestUnsatisfiableConditionInKinds(t *testing.T) {
	base := func() config.Config {
		return config.Config{
			Preset:      config.PresetSpec,
			StatusField: config.DefaultStatusField,
			Kinds: map[string]config.KindSpec{
				"clause":  {Dir: "spec/clauses", StatusValues: []string{"proposed", "accepted"}},
				"conform": {Dir: "spec/conform", StatusValues: []string{"draft"}},
			},
			Edges: []config.EdgeSpec{{
				Name: "enforces", Key: "enforces", Direction: config.DirectionForward,
				From: []string{"conform"}, To: []string{"clause"},
			}},
		}
	}

	t.Run("a kind no edge endpoint admits", func(t *testing.T) {
		cfg := base()
		cfg.Rules = []config.Rule{testRule("wrong_kind", model.SeverityError, config.Condition{
			Inbound: config.EdgeCondition{Edge: "enforces"},
			Attr:    map[string]config.AttrCondition{config.KeyKind: testEq("conform")},
		})}

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnfirableRule, "wrong_kind", model.SeverityError, "no document kind satisfies")
	})

	t.Run("a status outside the pinned kind's vocabulary", func(t *testing.T) {
		cfg := base()
		cfg.Rules = []config.Rule{testRule("wrong_status", model.SeverityError, config.Condition{
			Attr: map[string]config.AttrCondition{
				config.KeyKind:            testEq("conform"),
				config.DefaultStatusField: testEq("accepted"),
			},
		})}

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnfirableRule, "wrong_status", model.SeverityError, `is outside draft`)
	})

	t.Run("a one-hop clause no neighbour could answer", func(t *testing.T) {
		cfg := base()
		cfg.Rules = []config.Rule{testRule("bad_neighbour", model.SeverityError, config.Condition{
			ViaInbound: &config.ViaCondition{
				Edge: "enforces",
				Attr: map[string]config.AttrCondition{config.DefaultStatusField: testEq("accepted")},
			},
		})}

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnfirableRule, "bad_neighbour", model.SeverityError,
			"no enforces neighbour has status \"accepted\"")
	})
}

// TestUnsatisfiableAlternative reports the one dead alternative of a rule that
// still has a live one, which is a different finding from a dead rule.
func TestUnsatisfiableAlternative(t *testing.T) {
	cfg := testConfig(testRule("half_dead", model.SeverityWarn, config.Condition{
		AnyOf: []config.Condition{
			testAttr(config.DefaultStatusField, testEq(config.StatusAccepted)),
			testAttr(config.DefaultStatusField, testEq("retired")),
		},
	}))

	findings := testLint(cfg)

	assertFinding(t, findings, FindingUnsatisfiableCondition, "half_dead", model.SeverityError, "one alternative contradicts itself")
	assertNoFinding(t, findings, FindingUnfirableRule)
}

func TestTautologicalRule(t *testing.T) {
	t.Run("a rule that constrains nothing", func(t *testing.T) {
		cfg := testConfig(testRule("everything", model.SeverityWarn, config.Condition{}))

		findings := testLint(cfg)

		assertFinding(t, findings, FindingTautologicalRule, "everything", model.SeverityWarn, "constrains nothing")
	})

	t.Run("alternatives that exhaust a vocabulary", func(t *testing.T) {
		alternatives := make([]config.Condition, 0, len(testStatuses))
		for _, status := range testStatuses {
			alternatives = append(alternatives, testAttr(config.DefaultStatusField, testEq(status)))
		}
		cfg := testConfig(testRule("every_status", model.SeverityWarn, config.Condition{AnyOf: alternatives}))

		findings := testLint(cfg)

		assertFinding(t, findings, FindingTautologicalRule, "every_status", model.SeverityWarn,
			"cover the whole vocabulary of status")
	})

	t.Run("alternatives that leave one value out", func(t *testing.T) {
		alternatives := make([]config.Condition, 0, len(testStatuses))
		for _, status := range testStatuses[1:] {
			alternatives = append(alternatives, testAttr(config.DefaultStatusField, testEq(status)))
		}
		cfg := testConfig(testRule("most_statuses", model.SeverityWarn, config.Condition{AnyOf: alternatives}))

		assertNoFinding(t, testLint(cfg), FindingTautologicalRule)
	})
}

func TestSubsumedAndShadowedRules(t *testing.T) {
	narrow := config.Condition{
		NotInbound: "supersedes",
		Attr:       map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)},
	}
	wide := testAttr(config.DefaultStatusField, testEq(config.StatusAccepted))

	t.Run("a rule that says what a weaker one already says", func(t *testing.T) {
		cfg := testConfig(
			testRule("wide_rule", model.SeverityWarn, wide),
			testRule("narrow_rule", model.SeverityWarn, narrow),
		)

		findings := testLint(cfg)

		f := assertFinding(t, findings, FindingSubsumedRule, "narrow_rule", model.SeverityWarn, "fires only where wide_rule fires")
		if len(f.Related) != 1 {
			t.Errorf("related = %v, want the rule that subsumes it", f.Related)
		}
		assertNoFinding(t, findings, FindingShadowedRule)
	})

	t.Run("a stronger rule inside a weaker one is shadowed", func(t *testing.T) {
		cfg := testConfig(
			testRule("wide_rule", model.SeverityWarn, wide),
			testRule("narrow_rule", model.SeverityError, narrow),
		)

		findings := testLint(cfg)

		assertFinding(t, findings, FindingShadowedRule, "narrow_rule", model.SeverityWarn, "already reports at warn")
	})

	t.Run("two rules that say the same thing are reported once", func(t *testing.T) {
		cfg := testConfig(
			testRule("first", model.SeverityWarn, wide),
			testRule("second", model.SeverityWarn, wide),
		)

		findings := testLint(cfg)

		if _, reported := findingFor(findings, FindingSubsumedRule, "first"); reported {
			t.Error("the earlier rule was reported, want the pair reported against the later one")
		}
		assertFinding(t, findings, FindingSubsumedRule, "second", model.SeverityWarn, "fires only where first fires")
	})

	t.Run("unrelated rules are left alone", func(t *testing.T) {
		cfg := testConfig(
			testRule("first", model.SeverityWarn, testAttr(config.DefaultStatusField, testEq(config.StatusAccepted))),
			testRule("second", model.SeverityWarn, testAttr(config.DefaultStatusField, testEq(config.StatusProposed))),
		)

		assertNoFinding(t, testLint(cfg), FindingSubsumedRule)
	})
}

// TestAmbivalentFix constructs the conflict the built-in remedies could have:
// two rules DocDag generates a `set status:` fix for, written so they can both
// fire on one document and demand two different values there.
func TestAmbivalentFix(t *testing.T) {
	t.Run("two remedies demanding different values", func(t *testing.T) {
		cfg := testConfig(
			testRule(model.RuleStatusDrift, model.SeverityError, testAttr(config.DefaultStatusField, testEq(config.StatusAccepted))),
			testRule(model.RuleSupersededOrphan, model.SeverityWarn, testAttr(config.DefaultStatusField, testEq(config.StatusAccepted))),
		)

		findings := testLint(cfg)

		assertFinding(t, findings, FindingAmbivalentFix, model.RuleSupersededOrphan, model.SeverityError,
			"its fix sets status: withdrawn where status_drift sets status: superseded")
	})

	t.Run("the preset's own pair cannot both fire", func(t *testing.T) {
		assertNoFinding(t, testLint(config.ADRPreset()), FindingAmbivalentFix)
	})
}

func TestUnusedEdge(t *testing.T) {
	t.Run("an edge nothing reads", func(t *testing.T) {
		cfg := testConfig()
		cfg.Edges = append(cfg.Edges, config.EdgeSpec{Name: "mentions", Key: "mentions", Direction: config.DirectionForward})

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnusedEdge, "mentions", model.SeverityWarn, "no rule, projection, target")
	})

	readers := []struct {
		name string
		read func(cfg *config.Config)
	}{
		{name: "a rule that names it", read: func(cfg *config.Config) {
			cfg.Rules = append(cfg.Rules, testRule("reads", model.SeverityWarn, config.Condition{NotOutbound: "mentions"}))
		}},
		{name: "a projection that names it", read: func(cfg *config.Config) {
			cfg.Projections = append(cfg.Projections, config.ProjectionSpec{
				Name: "mentioned",
				When: config.Condition{Inbound: config.EdgeCondition{Edge: "mentions"}},
			})
		}},
		{name: "a path constraint that walks it", read: func(cfg *config.Config) {
			cfg.PathConstraints = []config.PathConstraint{{
				Name: "no_mentions", Path: []string{"mentions", "supersedes"}, Equals: config.PathEqualsNone,
			}}
		}},
		{name: "another edge's leaf_of", read: func(cfg *config.Config) {
			cfg.Edges[0].Target = &config.TargetCondition{LeafOf: "mentions"}
		}},
		{name: "a derived edge that produces it", read: func(cfg *config.Config) {
			cfg.DerivedEdges = []config.DerivedEdgeSpec{{
				Field: "note", Pattern: `mentions (\S+)`, Edge: "mentions", Direction: config.DirectionForward,
			}}
		}},
		{name: "an inverse key it mirrors", read: func(cfg *config.Config) {
			cfg.Edges[len(cfg.Edges)-1].Inverse = "mentioned-by"
		}},
		{name: "a degree bound a check reads", read: func(cfg *config.Config) {
			cfg.Edges[len(cfg.Edges)-1].MaxInbound = 1
		}},
	}
	for _, tt := range readers {
		t.Run(tt.name+" is a use", func(t *testing.T) {
			cfg := testConfig()
			cfg.Edges = append(cfg.Edges, config.EdgeSpec{Name: "mentions", Key: "mentions", Direction: config.DirectionForward})
			tt.read(&cfg)

			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			assertNoFinding(t, testLint(cfg), FindingUnusedEdge)
		})
	}

	t.Run("the engine's own edges are never unused", func(t *testing.T) {
		cfg := testConfig()
		cfg.Edges = []config.EdgeSpec{
			{Name: "supersedes", Key: "supersedes", Direction: config.DirectionForward},
			{Name: "depends-on", Key: "depends-on", Direction: config.DirectionForward},
		}

		assertNoFinding(t, testLint(cfg), FindingUnusedEdge)
	})
}

func TestUnusedStatus(t *testing.T) {
	t.Run("a vocabulary no condition reads", func(t *testing.T) {
		cfg := testConfig(testRule("degree_only", model.SeverityWarn, config.Condition{NotInbound: "supersedes"}))

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnusedStatus, "status_values", model.SeverityWarn,
			"are read by no rule, projection or target condition")
	})

	t.Run("a vocabulary a rule reads", func(t *testing.T) {
		cfg := testConfig(testRule("reads_status", model.SeverityWarn, testAttr(config.DefaultStatusField, testEq(config.StatusAccepted))))

		assertNoFinding(t, testLint(cfg), FindingUnusedStatus)
	})

	t.Run("a kind's vocabulary nothing reads", func(t *testing.T) {
		cfg := config.Config{
			Preset:      config.PresetSpec,
			StatusField: config.DefaultStatusField,
			Kinds: map[string]config.KindSpec{
				"clause": {Dir: "spec/clauses", StatusValues: []string{"proposed", "accepted"}},
				"pm":     {Dir: "spec/pm", StatusValues: []string{"draft", "published"}},
			},
			Edges: []config.EdgeSpec{{Name: "about", Key: "about", Direction: config.DirectionForward, From: []string{"clause"}, To: []string{"pm"}}},
			Rules: []config.Rule{testRule("clause_status", model.SeverityWarn, config.Condition{
				Attr: map[string]config.AttrCondition{
					config.KeyKind:            testEq("clause"),
					config.DefaultStatusField: testEq("accepted"),
				},
			})},
		}

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnusedStatus, "pm", model.SeverityWarn, `the status_values of kind "pm"`)
		if _, reported := findingFor(findings, FindingUnusedStatus, "clause"); reported {
			t.Error("the clause vocabulary was reported, want the kind a rule reads left alone")
		}
	})
}

func TestProjectionFindings(t *testing.T) {
	t.Run("a projection no document can satisfy", func(t *testing.T) {
		cfg := testConfig()
		cfg.Projections = []config.ProjectionSpec{{
			Name: "impossible",
			When: config.Condition{
				Inbound:    config.EdgeCondition{Edge: "supersedes"},
				NotInbound: "supersedes",
			},
		}}

		findings := testLint(cfg)

		assertFinding(t, findings, FindingUnsatisfiableProjection, "impossible", model.SeverityError, "every alternative contradicts itself")
	})

	t.Run("a projection every document satisfies", func(t *testing.T) {
		alternatives := make([]config.ProjectionAlt, 0, len(testStatuses))
		for _, status := range testStatuses {
			alternatives = append(alternatives, config.ProjectionAlt{When: testAttr(config.DefaultStatusField, testEq(status))})
		}
		cfg := testConfig()
		cfg.Projections = []config.ProjectionSpec{{Name: "always", AnyOf: alternatives}}

		findings := testLint(cfg)

		assertFinding(t, findings, FindingTautologicalProjection, "always", model.SeverityWarn, "cover the whole vocabulary")
	})
}

func TestConditionTooWide(t *testing.T) {
	cfg := testConfig(testRule("wide", model.SeverityWarn, testWide(3, 5)))

	findings := testLint(cfg)

	assertFinding(t, findings, FindingConditionTooWide, "wide", model.SeverityWarn, "expands past 64 alternatives")
}

func TestPreferTarget(t *testing.T) {
	withPath := func(constraint config.PathConstraint) config.Config {
		cfg := testConfig()
		cfg.PathConstraints = []config.PathConstraint{constraint}
		return cfg
	}

	t.Run("a two-step path that must reach nothing", func(t *testing.T) {
		cfg := withPath(config.PathConstraint{
			Name: "no_replaced_dependencies",
			Path: []string{"depends-on", "supersedes"},
			// The walk is supersedes ∘ depends-on: what a document depends on
			// must have no outbound supersedes, which is what an edge's own
			// target condition says one hop shorter.
			Equals: config.PathEqualsNone,
		})

		findings := testLint(cfg)

		f := assertFinding(t, findings, FindingPreferTarget, "no_replaced_dependencies", model.SeverityWarn,
			"target: {not_outbound: supersedes} on edge depends-on")
		if len(f.Related) != 1 {
			t.Errorf("related = %v, want the edge that would carry the target", f.Related)
		}
	})

	t.Run("a reversed second step reads as not_inbound", func(t *testing.T) {
		cfg := withPath(config.PathConstraint{
			Name:   "leaves_only",
			Path:   []string{"depends-on", "^supersedes"},
			Equals: config.PathEqualsNone,
		})

		assertFinding(t, testLint(cfg), FindingPreferTarget, "leaves_only", model.SeverityWarn,
			"target: {not_inbound: supersedes} on edge depends-on")
	})

	misses := []struct {
		name       string
		constraint config.PathConstraint
	}{
		{
			name:       "a one-step path is about the documents an edge leaves",
			constraint: config.PathConstraint{Name: "one_step", Path: []string{"depends-on"}, Equals: config.PathEqualsNone},
		},
		{
			name:       "a path walked backwards starts where no target condition can",
			constraint: config.PathConstraint{Name: "reversed", Path: []string{"^depends-on", "supersedes"}, Equals: config.PathEqualsNone},
		},
		{
			name:       "a comparison against a second walk is not an empty set",
			constraint: config.PathConstraint{Name: "subset", Path: []string{"depends-on", "supersedes"}, SubsetOf: []string{"supersedes"}},
		},
	}
	for _, tt := range misses {
		t.Run(tt.name, func(t *testing.T) {
			cfg := withPath(tt.constraint)
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}

			assertNoFinding(t, testLint(cfg), FindingPreferTarget)
		})
	}
}

// TestPresetsLintClean is what keeps a preset edit honest: DocDag ships two
// configurations, and a check that reports on configurations has to answer for
// its own.
func TestPresetsLintClean(t *testing.T) {
	for _, name := range []string{config.PresetADR, config.PresetSpec} {
		t.Run(name+" lints clean", func(t *testing.T) {
			cfg, err := config.Preset(name)
			if err != nil {
				t.Fatalf("Preset(%q): %v", name, err)
			}

			findings := testLint(cfg)

			if len(findings) > 0 {
				t.Errorf("findings = %s, want the shipped preset to lint clean", formatFindings(findings))
			}
		})
	}
}

// TestFindingsAreSorted checks the order every report is read in.
func TestFindingsAreSorted(t *testing.T) {
	cfg := testConfig(
		testRule("b_rule", model.SeverityWarn, config.Condition{}),
		testRule("a_rule", model.SeverityError, config.Condition{
			Inbound:    config.EdgeCondition{Edge: "supersedes"},
			NotInbound: "supersedes",
		}),
	)

	findings := testLint(cfg)

	if len(findings) < 2 {
		t.Fatalf("findings = %s, want both rules reported", formatFindings(findings))
	}
	if !slices.IsSortedFunc(findings, func(a, b model.Finding) int { return a.Severity.Rank() - b.Severity.Rank() }) {
		t.Errorf("findings = %s, want errors before warnings", formatFindings(findings))
	}
}

func strptr(v string) *string { return &v }
