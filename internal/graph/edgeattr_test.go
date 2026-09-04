package graph

import (
	"maps"
	"slices"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// The edge types the attribute corpus declares.
const (
	testEdgeMeasures  = model.EdgeType("measures")
	testEdgeDependsOn = model.EdgeType("depends-on")
)

// testAttrsConfig is the ADR preset with the attributes the ADR's own example
// writes: a reason from a closed vocabulary on supersedes, a measurement with a
// number, a string and a date, and one edge that declares no attributes at all,
// so a test can watch the two readings of an edge key side by side.
func testAttrsConfig() config.Config {
	cfg := config.ADRPreset()
	cfg.Edges = []config.EdgeSpec{
		{
			Name: config.EdgeSupersedes.String(), Key: "supersedes",
			Acyclic: true, Direction: config.DirectionForward,
			Attrs: map[string]config.EdgeAttrSpec{
				"reason": {Required: true, OneOf: []string{"recurrence", "premise-collapse", "conflict", "vocabulary"}},
			},
		},
		{
			Name: testEdgeMeasures.String(), Key: "measures", Direction: config.DirectionForward,
			Attrs: map[string]config.EdgeAttrSpec{
				"agreement": {Required: true, Type: config.AttrTypeNumber},
				"model":     {Required: true, Type: config.AttrTypeString},
				"expires":   {Type: config.AttrTypeDate},
			},
		},
		{Name: testEdgeDependsOn.String(), Key: "depends-on", Acyclic: true, Direction: config.DirectionForward},
	}
	return cfg
}

// testAttrsGraph builds a two-document corpus: 0001 is the target and 0002
// carries the frontmatter under test.
func testAttrsGraph(t *testing.T, fm map[string]any) (*model.Graph, *parse.Document) {
	t.Helper()
	subject := testDoc("0002", fm, "")
	docs := []*parse.Document{testDoc("0001", map[string]any{"status": "superseded"}, ""), subject}
	return Build(docs, testAttrsConfig()), subject
}

// testEdgeAttrs reports the attributes of the one edge of a type, failing when
// the corpus built anything other than one.
func testEdgeAttrs(t *testing.T, g *model.Graph, edge model.EdgeType) map[string]string {
	t.Helper()
	edges := g.EdgesOfType(edge)
	if len(edges) != 1 {
		t.Fatalf("%s edges = %+v, want exactly one", edge, edges)
	}
	return edges[0].Attrs
}

func TestBuildRecordsEdgeAttributes(t *testing.T) {
	t.Run("an attributed reference carries its attributes onto the edge", func(t *testing.T) {
		g, _ := testAttrsGraph(t, map[string]any{
			"status":     "accepted",
			"supersedes": []any{map[string]any{"ref": "0001", "reason": "conflict"}},
		})

		want := []model.Edge{{
			From: "0002", To: "0001", Type: config.EdgeSupersedes, Origin: model.OriginStructured,
			Attrs: map[string]string{"reason": "conflict"},
		}}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
		if got := testFindingsFor(g.Findings, model.RuleEdgeAttrInvalid); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("numbers and dates are recorded in the form they were written in", func(t *testing.T) {
		g, _ := testAttrsGraph(t, map[string]any{
			"status": "accepted",
			"measures": []any{map[string]any{
				"ref": "0001", "agreement": 0.90, "model": "haiku", "expires": "2026-01-01",
			}},
		})

		want := map[string]string{"agreement": "0.9", "model": "haiku", "expires": "2026-01-01"}
		if got := testEdgeAttrs(t, g, testEdgeMeasures); !maps.Equal(got, want) {
			t.Fatalf("attrs = %v, want %v", got, want)
		}
	})

	t.Run("a plain reference on an attributed edge carries no attributes", func(t *testing.T) {
		g, _ := testAttrsGraph(t, map[string]any{
			"status":     "accepted",
			"supersedes": []any{map[string]any{"ref": "0001", "reason": "conflict"}},
			"depends-on": []any{"0001"},
		})

		if got := testEdgeAttrs(t, g, testEdgeDependsOn); len(got) != 0 {
			t.Fatalf("attrs = %v, want none", got)
		}
	})

	t.Run("a derived edge carries no attributes", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, ""),
			testDoc("0002", map[string]any{"status": "superseded by 0001"}, ""),
		}

		g := Build(docs, testAttrsConfig())

		want := []model.Edge{testDerivedEdge("0001", "0002", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("one relation declared twice keeps the first entry's attributes", func(t *testing.T) {
		g, _ := testAttrsGraph(t, map[string]any{
			"status": "accepted",
			"supersedes": []any{
				map[string]any{"ref": "0001", "reason": "conflict"},
				map[string]any{"ref": "0001", "reason": "vocabulary"},
			},
		})

		want := map[string]string{"reason": "conflict"}
		if got := testEdgeAttrs(t, g, config.EdgeSupersedes); !maps.Equal(got, want) {
			t.Fatalf("attrs = %v, want %v", got, want)
		}
	})

	t.Run("a rejected value is not recorded, so an edge only carries what its spec accepts", func(t *testing.T) {
		g, _ := testAttrsGraph(t, map[string]any{
			"status":     "accepted",
			"supersedes": []any{map[string]any{"ref": "0001", "reason": "rewrite"}},
		})

		if got := testEdgeAttrs(t, g, config.EdgeSupersedes); len(got) != 0 {
			t.Fatalf("attrs = %v, want none", got)
		}
	})
}

func TestBuildReportsEdgeAttributeFindings(t *testing.T) {
	tests := []struct {
		name   string
		fm     map[string]any
		rule   string
		key    string
		detail string
	}{
		{
			name: "an attribute the edge does not declare",
			fm: map[string]any{
				"status":     "accepted",
				"supersedes": []any{map[string]any{"ref": "0001", "reason": "conflict", "note": "moved"}},
			},
			rule:   model.RuleEdgeAttrUnknown,
			key:    "supersedes",
			detail: `supersedes reference "0001" carries unknown attribute "note", declared: reason`,
		},
		{
			name: "a required attribute an attributed entry left out",
			fm: map[string]any{
				"status":     "accepted",
				"supersedes": []any{map[string]any{"ref": "0001"}},
			},
			rule:   model.RuleEdgeAttrMissing,
			key:    "supersedes",
			detail: `supersedes reference "0001" is missing required attribute "reason"`,
		},
		{
			name: "a required attribute a plain reference cannot carry",
			fm: map[string]any{
				"status":     "accepted",
				"supersedes": []any{"0001"},
			},
			rule:   model.RuleEdgeAttrMissing,
			key:    "supersedes",
			detail: `supersedes reference "0001" is missing required attribute "reason"`,
		},
		{
			name: "a word outside the vocabulary",
			fm: map[string]any{
				"status":     "accepted",
				"supersedes": []any{map[string]any{"ref": "0001", "reason": "rewrite"}},
			},
			rule:   model.RuleEdgeAttrInvalid,
			key:    "supersedes",
			detail: `supersedes reference "0001" attribute "reason" is "rewrite", want one of: recurrence, premise-collapse, conflict, vocabulary`,
		},
		{
			name: "another spelling of a word in the vocabulary",
			fm: map[string]any{
				"status":     "accepted",
				"supersedes": []any{map[string]any{"ref": "0001", "reason": "Conflict"}},
			},
			rule:   model.RuleEdgeAttrInvalid,
			key:    "supersedes",
			detail: `supersedes reference "0001" attribute "reason" is "Conflict", want one of: recurrence, premise-collapse, conflict, vocabulary`,
		},
		{
			name: "a number that is not one",
			fm: map[string]any{
				"status":   "accepted",
				"measures": []any{map[string]any{"ref": "0001", "agreement": "high", "model": "haiku"}},
			},
			rule:   model.RuleEdgeAttrInvalid,
			key:    "measures",
			detail: `measures reference "0001" attribute "agreement" is "high", want a number`,
		},
		{
			name: "a date that is not one",
			fm: map[string]any{
				"status": "accepted",
				"measures": []any{map[string]any{
					"ref": "0001", "agreement": 0.5, "model": "haiku", "expires": "soon",
				}},
			},
			rule:   model.RuleEdgeAttrInvalid,
			key:    "measures",
			detail: `measures reference "0001" attribute "expires" is "soon", want a date as YYYY-MM-DD`,
		},
		{
			name: "a value that is not a scalar at all",
			fm: map[string]any{
				"status": "accepted",
				"measures": []any{map[string]any{
					"ref": "0001", "agreement": 0.5, "model": []any{"haiku", "sonnet"},
				}},
			},
			rule:   model.RuleEdgeAttrInvalid,
			key:    "measures",
			detail: `measures reference "0001" attribute "model" is "[haiku sonnet]", want a string`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, doc := testAttrsGraph(t, tt.fm)

			finding := testAssertSingleFinding(t, g.Findings, tt.rule, model.SeverityError, "0002")

			if finding.Detail != tt.detail {
				t.Errorf("detail = %q, want %q", finding.Detail, tt.detail)
			}
			want := model.Location{Path: doc.Path, Line: doc.KeyLines[tt.key]}
			if finding.Location != want {
				t.Errorf("location = %+v, want %+v (the line the edge key is on)", finding.Location, want)
			}
			testAssertSortedFindings(t, g.Findings)
		})
	}
}

func TestBuildChecksAttributesWhateverTheReferenceResolvesTo(t *testing.T) {
	// The attributes are what a document wrote down, so they are checked even
	// where the reference beside them is no reference at all: the entry has two
	// problems and a reader is told about both.
	g, _ := testAttrsGraph(t, map[string]any{
		"status":     "accepted",
		"supersedes": []any{map[string]any{"ref": "see 0001", "reason": "rewrite"}},
	})

	testAssertSingleFinding(t, g.Findings, model.RuleInvalidRef, model.SeverityError, "0002")
	invalid := testAssertSingleFinding(t, g.Findings, model.RuleEdgeAttrInvalid, model.SeverityError, "0002")

	want := `supersedes reference "see 0001" attribute "reason" is "rewrite", want one of: recurrence, premise-collapse, conflict, vocabulary`
	if invalid.Detail != want {
		t.Errorf("detail = %q, want %q", invalid.Detail, want)
	}
	if len(g.Edges) != 0 {
		t.Errorf("edges = %+v, want none: the entry names no document", g.Edges)
	}
}

func TestBuildLeavesAnEdgeWithoutAttributesAlone(t *testing.T) {
	// The gate the ADR draws: only an edge that declares attributes reads a
	// mapping as a reference. Under every other edge a mapping stays what it has
	// always been, an entry that names no document, and it builds no edge.
	t.Run("a mapping under an edge that declares no attributes", func(t *testing.T) {
		g, doc := testAttrsGraph(t, map[string]any{
			"status":     "accepted",
			"depends-on": []any{map[string]any{"ref": "0001", "reason": "conflict"}},
		})

		finding := testAssertSingleFinding(t, g.Findings, model.RuleDanglingRef, model.SeverityError, "0002")

		want := `depends-on reference "map[reason:conflict ref:0001]" does not name a document`
		if finding.Detail != want {
			t.Errorf("detail = %q, want %q", finding.Detail, want)
		}
		if got := finding.Location.Line; got != doc.KeyLines["depends-on"] {
			t.Errorf("location line = %d, want %d", got, doc.KeyLines["depends-on"])
		}
		if len(g.Edges) != 0 {
			t.Errorf("edges = %+v, want none", g.Edges)
		}
		for _, rule := range []string{model.RuleEdgeAttrUnknown, model.RuleEdgeAttrMissing, model.RuleEdgeAttrInvalid} {
			if got := testFindingsFor(g.Findings, rule); len(got) != 0 {
				t.Errorf("%s findings = %+v, want none on an edge that declares no attributes", rule, got)
			}
		}
	})

	t.Run("the ADR preset reports a mapping exactly as it always did", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, ""),
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{map[string]any{"ref": "0001", "reason": "conflict"}},
			}, ""),
		}

		g := Build(docs, config.ADRPreset())

		finding := testAssertSingleFinding(t, g.Findings, model.RuleDanglingRef, model.SeverityError, "0002")
		want := `supersedes reference "map[reason:conflict ref:0001]" does not name a document`
		if finding.Detail != want {
			t.Errorf("detail = %q, want %q", finding.Detail, want)
		}
	})
}

func TestBuildStillReportsAnEmptyAttributedEdgeKey(t *testing.T) {
	// An attributed edge key written down and left empty reads as a declared
	// relation like any other, and builds no edge like any other.
	g, _ := testAttrsGraph(t, map[string]any{"status": "accepted", "supersedes": []any{}})

	finding := testAssertSingleFinding(t, g.Findings, model.RuleEmptyEdge, model.SeverityError, "0002")

	if want := "supersedes is present but names no document"; finding.Detail != want {
		t.Errorf("detail = %q, want %q", finding.Detail, want)
	}
}

func TestBuildReportsEveryAttributeProblemOfOneEntry(t *testing.T) {
	// One entry can be wrong in several ways at once, and each way is its own
	// finding: the reader fixes the entry once, not once per run.
	g, _ := testAttrsGraph(t, map[string]any{
		"status": "accepted",
		"measures": []any{map[string]any{
			"ref": "0001", "agreement": "high", "sampled": true,
		}},
	})

	want := []string{model.RuleEdgeAttrInvalid, model.RuleEdgeAttrMissing, model.RuleEdgeAttrUnknown}
	got := []string{}
	for _, f := range g.Findings {
		got = append(got, f.Rule)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("findings = %+v, want one of each of %v", g.Findings, want)
	}
}

func TestEdgeAttributeConfigurationIsValid(t *testing.T) {
	// The corpus these tests build is one a person could write down, so the
	// configuration behind them has to pass the same validation their does.
	if err := testAttrsConfig().Validate(); err != nil {
		t.Fatalf("Validate = %v, want the attribute configuration to be valid", err)
	}
}
