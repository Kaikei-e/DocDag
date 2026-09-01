package config

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// testKinds is the multi-kind vocabulary the kind tests are written against:
// one kind whose identifiers a file name can carry, one whose identifiers carry
// a slash and therefore cannot, and one keeping a status vocabulary of its own.
func testKinds() map[string]KindSpec {
	return map[string]KindSpec{
		"clause":  {Dir: "spec/clauses", ID: `^UZ-[A-Z]-\d{3}$`, StatusValues: []string{"trial", "accepted"}, Closed: true},
		"conform": {Dir: "spec/conform", ID: `^conform/[a-z0-9-]+$`},
		"pm":      {Dir: "spec/pm"},
	}
}

func testEq(v string) AttrCondition  { return AttrCondition{Eq: &v} }
func testNot(v string) AttrCondition { return AttrCondition{Not: &v} }
func strptr(v string) *string        { return &v }
func testInt(v int) *int             { return &v }

func TestConfigEdge(t *testing.T) {
	cfg := ADRPreset()
	tests := []struct {
		name string
		edge model.EdgeType
		want EdgeSpec
		ok   bool
	}{
		{
			name: "the supersedes spec",
			edge: EdgeSupersedes,
			want: EdgeSpec{Name: "supersedes", Key: "supersedes", Acyclic: true, Direction: DirectionForward},
			ok:   true,
		},
		{
			name: "the depends-on spec",
			edge: EdgeDependsOn,
			want: EdgeSpec{Name: "depends-on", Key: "depends-on", Acyclic: true, Direction: DirectionForward},
			ok:   true,
		},
		{name: "an undeclared edge type", edge: "relates-to"},
		{name: "an empty edge type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cfg.Edge(tt.edge)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Edge(%q) = %+v, want %+v", tt.edge, got, tt.want)
			}
		})
	}

	t.Run("the lookup is by edge name, not by frontmatter key", func(t *testing.T) {
		renamed := ADRPreset()
		renamed.Edges = []EdgeSpec{{Name: "supersedes", Key: "superseded-by", Acyclic: true, Direction: DirectionReverse}}

		if _, ok := renamed.Edge("superseded-by"); ok {
			t.Error("Edge(superseded-by) found a spec, want the lookup to use the edge name")
		}
		if _, ok := renamed.Edge(EdgeSupersedes); !ok {
			t.Error("Edge(supersedes) found nothing, want the renamed spec")
		}
	})
}

func TestConfigEdgeTypes(t *testing.T) {
	t.Run("declaration order is preserved", func(t *testing.T) {
		want := []model.EdgeType{EdgeSupersedes, EdgeDependsOn}

		if got := ADRPreset().EdgeTypes(); !slices.Equal(got, want) {
			t.Fatalf("EdgeTypes = %v, want %v", got, want)
		}
	})

	t.Run("a reordered configuration reports its own order", func(t *testing.T) {
		reordered := ADRPreset()
		reordered.Edges = []EdgeSpec{reordered.Edges[1], reordered.Edges[0]}
		want := []model.EdgeType{EdgeDependsOn, EdgeSupersedes}

		if got := reordered.EdgeTypes(); !slices.Equal(got, want) {
			t.Fatalf("EdgeTypes = %v, want %v", got, want)
		}
	})

	t.Run("a configuration without edges has none", func(t *testing.T) {
		if got := (Config{}).EdgeTypes(); len(got) != 0 {
			t.Fatalf("EdgeTypes = %v, want none", got)
		}
	})
}

func TestConfigAcyclicEdgeTypes(t *testing.T) {
	t.Run("both preset edge types are acyclic", func(t *testing.T) {
		want := []model.EdgeType{EdgeSupersedes, EdgeDependsOn}

		if got := ADRPreset().AcyclicEdgeTypes(); !slices.Equal(got, want) {
			t.Fatalf("AcyclicEdgeTypes = %v, want %v", got, want)
		}
	})

	t.Run("a cyclic edge type is excluded", func(t *testing.T) {
		mixed := ADRPreset()
		mixed.Edges = append(mixed.Edges, EdgeSpec{Name: "relates-to", Key: "relates-to", Direction: DirectionForward})
		want := []model.EdgeType{EdgeSupersedes, EdgeDependsOn}

		if got := mixed.AcyclicEdgeTypes(); !slices.Equal(got, want) {
			t.Fatalf("AcyclicEdgeTypes = %v, want %v", got, want)
		}
	})

	t.Run("no acyclic edge type at all", func(t *testing.T) {
		none := ADRPreset()
		for i := range none.Edges {
			none.Edges[i].Acyclic = false
		}

		if got := none.AcyclicEdgeTypes(); len(got) != 0 {
			t.Fatalf("AcyclicEdgeTypes = %v, want none", got)
		}
	})
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr bool
	}{
		{name: "the preset is valid"},
		{
			name:    "an unknown edge direction",
			mutate:  func(c *Config) { c.Edges[0].Direction = "sideways" },
			wantErr: true,
		},
		{
			name:    "an edge without a name",
			mutate:  func(c *Config) { c.Edges[0].Name = "" },
			wantErr: true,
		},
		{
			name:    "an edge without a frontmatter key",
			mutate:  func(c *Config) { c.Edges[0].Key = "" },
			wantErr: true,
		},
		{
			name:    "two edges with the same name",
			mutate:  func(c *Config) { c.Edges[1].Name = c.Edges[0].Name },
			wantErr: true,
		},
		{
			name:    "a rule without a name",
			mutate:  func(c *Config) { c.Rules[0].Name = "" },
			wantErr: true,
		},
		{
			name:    "a rule with an unknown severity",
			mutate:  func(c *Config) { c.Rules[0].Severity = "fatal" },
			wantErr: true,
		},
		{
			name: "a structural check escalated to an error",
			mutate: func(c *Config) {
				c.Structural = map[string]model.Severity{model.RuleMissingFrontmatter: model.SeverityError}
			},
		},
		{
			name:    "a structural check lowered to a warning",
			mutate:  func(c *Config) { c.Structural = map[string]model.Severity{model.RuleCycle: model.SeverityWarn} },
			wantErr: true,
		},
		{
			name:    "a structural check at an unknown severity",
			mutate:  func(c *Config) { c.Structural = map[string]model.Severity{model.RuleCycle: "fatal"} },
			wantErr: true,
		},
		{
			name:    "a structural check nobody runs",
			mutate:  func(c *Config) { c.Structural = map[string]model.Severity{"status_drift": model.SeverityError} },
			wantErr: true,
		},
		{
			name:   "an edge with cardinality bounds",
			mutate: func(c *Config) { c.Edges[0].MaxInbound, c.Edges[0].MinOutbound = 1, 1 },
		},
		{
			name:    "a negative bound",
			mutate:  func(c *Config) { c.Edges[0].MaxInbound = -1 },
			wantErr: true,
		},
		{
			name:    "a minimum above the maximum",
			mutate:  func(c *Config) { c.Edges[0].MinOutbound, c.Edges[0].MaxOutbound = 2, 1 },
			wantErr: true,
		},
		{
			name:   "an edge with an inverse key",
			mutate: func(c *Config) { c.Edges[0].Inverse = "superseded_by" },
		},
		{
			name:    "an inverse key that is the edge's own key",
			mutate:  func(c *Config) { c.Edges[0].Inverse = c.Edges[0].Key },
			wantErr: true,
		},
		{
			name:    "an inverse key another edge already declares",
			mutate:  func(c *Config) { c.Edges[0].Inverse = c.Edges[1].Key },
			wantErr: true,
		},
		{
			name: "two edges sharing one inverse key",
			mutate: func(c *Config) {
				c.Edges[0].Inverse, c.Edges[1].Inverse = "linked_from", "linked_from"
			},
			wantErr: true,
		},
		{
			name: "an edge declaring attributes",
			mutate: func(c *Config) {
				c.Edges[0].Attrs = map[string]EdgeAttrSpec{
					"reason":    {Required: true, OneOf: []string{"conflict", "vocabulary"}},
					"agreement": {Type: AttrTypeNumber},
					"expires":   {Type: AttrTypeDate},
					"model":     {Required: true, Type: AttrTypeString},
				}
			},
		},
		{
			name: "an edge attribute with an unknown type",
			mutate: func(c *Config) {
				c.Edges[0].Attrs = map[string]EdgeAttrSpec{"agreement": {Type: "float"}}
			},
			wantErr: true,
		},
		{
			name: "an edge attribute bounding a number by a string vocabulary",
			mutate: func(c *Config) {
				c.Edges[0].Attrs = map[string]EdgeAttrSpec{
					"agreement": {Type: AttrTypeNumber, OneOf: []string{"high", "low"}},
				}
			},
			wantErr: true,
		},
		{
			name: "an edge attribute named after the reserved reference key",
			mutate: func(c *Config) {
				c.Edges[0].Attrs = map[string]EdgeAttrSpec{EdgeRefKey: {Required: true}}
			},
			wantErr: true,
		},
		{
			name: "an edge attribute without a name",
			mutate: func(c *Config) {
				c.Edges[0].Attrs = map[string]EdgeAttrSpec{"": {Required: true}}
			},
			wantErr: true,
		},
		{
			name: "an edge attribute check escalated to an error it already speaks at",
			mutate: func(c *Config) {
				c.Structural = map[string]model.Severity{model.RuleEdgeAttrInvalid: model.SeverityError}
			},
		},
		{
			name: "an edge attribute check lowered to a warning",
			mutate: func(c *Config) {
				c.Structural = map[string]model.Severity{model.RuleEdgeAttrMissing: model.SeverityWarn}
			},
			wantErr: true,
		},
		{
			name:   "reference validation switched off explicitly",
			mutate: func(c *Config) { c.References.Dangling = ReferencesOff },
		},
		{
			name:   "reference validation at warning severity",
			mutate: func(c *Config) { c.References.Dangling = string(model.SeverityWarn) },
		},
		{
			name:    "an unknown reference validation mode",
			mutate:  func(c *Config) { c.References.Dangling = "fatal" },
			wantErr: true,
		},
		{
			name:   "the scannable regions",
			mutate: func(c *Config) { c.References.Scan = []string{ScanBody, ScanFrontmatter} },
		},
		{
			name:    "an unknown reference scan region",
			mutate:  func(c *Config) { c.References.Scan = []string{"footnotes"} },
			wantErr: true,
		},
		{
			name:    "a reference pattern that does not compile",
			mutate:  func(c *Config) { c.References.Pattern = "(" },
			wantErr: true,
		},
		{
			name:    "a rule condition naming an undeclared edge type",
			mutate:  func(c *Config) { c.Rules[0].When.Inbound = EdgeCondition{Edge: "relates-to"} },
			wantErr: true,
		},
		{
			name:    "a rule condition negating an undeclared edge type",
			mutate:  func(c *Config) { c.Rules[1].When.NotInbound = "relates-to" },
			wantErr: true,
		},
		{
			name: "an attribute condition setting both eq and not",
			mutate: func(c *Config) {
				both := testEq(StatusSuperseded)
				not := StatusAccepted
				both.Not = &not
				c.Rules[0].When.Attr = map[string]AttrCondition{"status": both}
			},
			wantErr: true,
		},
		{
			name: "an attribute condition setting neither eq nor not",
			mutate: func(c *Config) {
				c.Rules[0].When.Attr = map[string]AttrCondition{"status": {}}
			},
			wantErr: true,
		},
		{
			name: "an attribute condition on a list",
			mutate: func(c *Config) {
				c.Rules[0].When.Attr = map[string]AttrCondition{"tags": {Contains: strptr("legacy")}}
			},
		},
		{
			name: "an attribute condition bounding a list",
			mutate: func(c *Config) {
				c.Rules[0].When.Attr = map[string]AttrCondition{"tags": {SubsetOf: []string{"legacy"}}}
			},
		},
		{
			name: "an attribute condition setting a scalar and a list operand",
			mutate: func(c *Config) {
				c.Rules[0].When.Attr = map[string]AttrCondition{
					"tags": {Eq: strptr("legacy"), Contains: strptr("legacy")},
				}
			},
			wantErr: true,
		},
		{
			name: "an attribute condition setting two list operands",
			mutate: func(c *Config) {
				c.Rules[0].When.Attr = map[string]AttrCondition{
					"tags": {Contains: strptr("legacy"), NotContains: strptr("current")},
				}
			},
			wantErr: true,
		},
		{
			name: "a condition with alternatives and a negation",
			mutate: func(c *Config) {
				c.Rules[0].When = Condition{
					AnyOf: []Condition{{Inbound: EdgeCondition{Edge: EdgeSupersedes.String()}}, {Attr: map[string]AttrCondition{"status": testEq(StatusRejected)}}},
					Not:   &Condition{Attr: map[string]AttrCondition{"status": testEq(StatusProposed)}},
				}
			},
		},
		{
			name: "an alternative naming an undeclared edge type",
			mutate: func(c *Config) {
				c.Rules[0].When.AnyOf = []Condition{{Inbound: EdgeCondition{Edge: "relates-to"}}}
			},
			wantErr: true,
		},
		{
			name: "a negation naming an undeclared edge type",
			mutate: func(c *Config) {
				c.Rules[0].When.Not = &Condition{Outbound: EdgeCondition{Edge: "relates-to"}}
			},
			wantErr: true,
		},
		{
			name: "a nested attribute condition with no operand",
			mutate: func(c *Config) {
				c.Rules[0].When.Not = &Condition{Attr: map[string]AttrCondition{"status": {}}}
			},
			wantErr: true,
		},
		{
			name:    "an empty list of alternatives",
			mutate:  func(c *Config) { c.Rules[0].When.AnyOf = []Condition{} },
			wantErr: true,
		},
		{
			name:    "a derived edge naming an undeclared edge type",
			mutate:  func(c *Config) { c.DerivedEdges[0].Edge = "relates-to" },
			wantErr: true,
		},
		{
			name:    "a derived edge without a field",
			mutate:  func(c *Config) { c.DerivedEdges[0].Field = "" },
			wantErr: true,
		},
		{
			name:    "a derived edge whose pattern does not compile",
			mutate:  func(c *Config) { c.DerivedEdges[0].Pattern = "([0-9" },
			wantErr: true,
		},
		{
			name:    "a derived edge with an unknown direction",
			mutate:  func(c *Config) { c.DerivedEdges[0].Direction = "sideways" },
			wantErr: true,
		},
		{
			// Without a capture group the pattern has nothing to point at, and
			// every match would be dropped at parse time without a word.
			name:    "a derived edge whose pattern captures nothing",
			mutate:  func(c *Config) { c.DerivedEdges[0].Pattern = "(?i)^superseded by" },
			wantErr: true,
		},
		{
			name:    "a negative identifier width",
			mutate:  func(c *Config) { c.IDWidth = -1 },
			wantErr: true,
		},
		{
			name: "a configuration without rules is valid",
			mutate: func(c *Config) {
				c.Rules = nil
				c.DerivedEdges = nil
			},
		},
		{
			name: "an outbound condition on a declared edge type is valid",
			mutate: func(c *Config) {
				c.Rules[0].When.Inbound = EdgeCondition{}
				c.Rules[0].When.Outbound = EdgeCondition{Edge: EdgeDependsOn.String()}
			},
		},
		{
			name: "a warn-only rule set is valid",
			mutate: func(c *Config) {
				c.Rules[0].Severity = model.SeverityWarn
				c.Rules[1].When.Attr = map[string]AttrCondition{"status": testNot(StatusAccepted)}
			},
		},
		{
			name:   "a corpus of several kinds",
			mutate: func(c *Config) { c.Kinds = testKinds() },
		},
		{
			name: "a kind without an id pattern keeps the digit-run rules",
			mutate: func(c *Config) {
				c.Kinds = map[string]KindSpec{"note": {Dir: "notes"}}
			},
		},
		{
			name: "a kind without a directory",
			mutate: func(c *Config) {
				c.Kinds = map[string]KindSpec{"clause": {ID: `^UZ-[A-Z]-\d{3}$`}}
			},
			wantErr: true,
		},
		{
			name: "two kinds sharing a directory",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Kinds["deviation"] = KindSpec{Dir: "spec/clauses", ID: `^dev-\d{4}$`}
			},
			wantErr: true,
		},
		{
			name: "two kinds whose directories differ only in spelling",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Kinds["deviation"] = KindSpec{Dir: "spec/./clauses/", ID: `^dev-\d{4}$`}
			},
			wantErr: true,
		},
		{
			name: "a kind whose id pattern does not compile",
			mutate: func(c *Config) {
				c.Kinds = map[string]KindSpec{"clause": {Dir: "spec/clauses", ID: "^UZ-([A-Z]$"}}
			},
			wantErr: true,
		},
		{
			name: "a kind without a name",
			mutate: func(c *Config) {
				c.Kinds = map[string]KindSpec{"": {Dir: "spec/clauses"}}
			},
			wantErr: true,
		},
		{
			name: "a top-level directory beside the kinds",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Dir = "docs/adr"
			},
			wantErr: true,
		},
		{
			name: "an edge constrained to declared kinds",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Edges[0].From, c.Edges[0].To = []string{"clause"}, []string{"clause"}
			},
		},
		{
			name: "an edge whose from names an undeclared kind",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Edges[0].From = []string{"principle"}
			},
			wantErr: true,
		},
		{
			name: "an edge whose to names an undeclared kind",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Edges[0].To = []string{"principle"}
			},
			wantErr: true,
		},
		{
			name:    "an edge constrained by kind on a corpus without kinds",
			mutate:  func(c *Config) { c.Edges[0].To = []string{"clause"} },
			wantErr: true,
		},
		{
			name: "a retired field with the whole lifecycle written down",
			mutate: func(c *Config) {
				c.PresetVersion = 3
				c.Fields = map[string]FieldSpec{
					"owner": {Deprecated: true, Since: 2, MigrateTo: "owned-by", Sunset: "2027-01-01"},
				}
			},
		},
		{
			name:   "a field declared without retiring it",
			mutate: func(c *Config) { c.Fields = map[string]FieldSpec{"owner": {}} },
		},
		{
			name:    "a sunset that is not a date",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"owner": {Deprecated: true, Sunset: "soon"}} },
			wantErr: true,
		},
		{
			name: "a sunset written the American way",
			mutate: func(c *Config) {
				c.Fields = map[string]FieldSpec{"owner": {Deprecated: true, Sunset: "01/01/2027"}}
			},
			wantErr: true,
		},
		{
			name:    "a negative since",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"owner": {Deprecated: true, Since: -1}} },
			wantErr: true,
		},
		{
			name:    "a migrate_to without a deprecation",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"owner": {MigrateTo: "owned-by"}} },
			wantErr: true,
		},
		{
			name:    "a sunset without a deprecation",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"owner": {Sunset: "2027-01-01"}} },
			wantErr: true,
		},
		{
			name:    "a since without a deprecation",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"owner": {Since: 2}} },
			wantErr: true,
		},
		{
			name:    "a field without a name",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"": {Deprecated: true}} },
			wantErr: true,
		},
		{
			name: "a field named after an edge key",
			mutate: func(c *Config) {
				c.Fields = map[string]FieldSpec{c.Edges[0].Key: {Deprecated: true}}
			},
			wantErr: true,
		},
		{
			name: "a field named after an inverse key",
			mutate: func(c *Config) {
				c.Edges[0].Inverse = "superseded_by"
				c.Fields = map[string]FieldSpec{"superseded_by": {Deprecated: true}}
			},
			wantErr: true,
		},
		{
			name: "a kind declaring fields of its own",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Kinds["clause"] = KindSpec{Dir: "spec/clauses", Fields: map[string]FieldSpec{
					"owner": {Deprecated: true, MigrateTo: "owned-by"},
				}}
			},
		},
		{
			name: "a kind's field declaration is held to the same shape",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Kinds["clause"] = KindSpec{Dir: "spec/clauses", Fields: map[string]FieldSpec{
					"owner": {Deprecated: true, Sunset: "whenever"},
				}}
			},
			wantErr: true,
		},
		{
			name: "a field with a closed vocabulary a document has to state",
			mutate: func(c *Config) {
				c.Fields = map[string]FieldSpec{"modality": {OneOf: []string{"MUST", "MAY"}, Required: true}}
			},
		},
		{
			name:    "a vocabulary holding an empty value",
			mutate:  func(c *Config) { c.Fields = map[string]FieldSpec{"modality": {OneOf: []string{"MUST", ""}}} },
			wantErr: true,
		},
		{
			name: "a vocabulary naming one value twice",
			mutate: func(c *Config) {
				c.Fields = map[string]FieldSpec{"modality": {OneOf: []string{"MUST", "MAY", "MUST"}}}
			},
			wantErr: true,
		},
		{
			name: "a required field the corpus is also retiring",
			mutate: func(c *Config) {
				c.Fields = map[string]FieldSpec{"owner": {Required: true, Deprecated: true}}
			},
			wantErr: true,
		},
		{
			name: "a vocabulary for a field the corpus is retiring",
			mutate: func(c *Config) {
				c.Fields = map[string]FieldSpec{"owner": {OneOf: []string{"platform"}, Deprecated: true}}
			},
			wantErr: true,
		},
		{
			name: "a kind's vocabulary is held to the same shape",
			mutate: func(c *Config) {
				c.Kinds = testKinds()
				c.Kinds["clause"] = KindSpec{Dir: "spec/clauses", Fields: map[string]FieldSpec{
					"modality": {OneOf: []string{"MUST", "MUST"}},
				}}
			},
			wantErr: true,
		},
		{
			name: "a deprecation escalated to an error",
			mutate: func(c *Config) {
				c.Structural = map[string]model.Severity{model.RuleDeprecatedField: model.SeverityError}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate()

			if tt.wantErr {
				if !errors.Is(err, model.ErrInvalidConfig) {
					t.Fatalf("Validate = %v, want it to wrap model.ErrInvalidConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate = %v, want no error", err)
			}
		})
	}
}

func TestConfigSeverity(t *testing.T) {
	tests := []struct {
		name       string
		structural map[string]model.Severity
		rule       string
		want       model.Severity
	}{
		{name: "an unconfigured warning", rule: model.RuleMissingFrontmatter, want: model.SeverityWarn},
		{name: "an unconfigured error", rule: model.RuleCycle, want: model.SeverityError},
		{
			name:       "an escalated warning",
			structural: map[string]model.Severity{model.RuleMissingFrontmatter: model.SeverityError},
			rule:       model.RuleMissingFrontmatter,
			want:       model.SeverityError,
		},
		{
			name:       "a check the escalation does not name",
			structural: map[string]model.Severity{model.RuleMissingFrontmatter: model.SeverityError},
			rule:       model.RuleUnstructuredSupersedes,
			want:       model.SeverityWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.Structural = tt.structural

			if got := cfg.Severity(tt.rule); got != tt.want {
				t.Fatalf("Severity(%q) = %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}

func TestConditionEdgeClauses(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
		want []EdgeClause
	}{
		{name: "an empty condition names no edge"},
		{
			name: "every clause of the vocabulary, in declaration order",
			cond: Condition{
				Inbound:     EdgeCondition{Edge: "supersedes"},
				NotInbound:  "depends-on",
				Outbound:    EdgeCondition{Edge: "amends"},
				NotOutbound: "relates-to",
			},
			want: []EdgeClause{
				{Edge: "supersedes", Inbound: true, Min: 1},
				{Edge: "depends-on", Inbound: true, Negate: true},
				{Edge: "amends", Min: 1},
				{Edge: "relates-to", Negate: true},
			},
		},
		{
			name: "a threshold carries the window it asks for",
			cond: Condition{Inbound: EdgeCondition{Edge: "supersedes", Min: testInt(2), Max: testInt(5)}},
			want: []EdgeClause{{Edge: "supersedes", Inbound: true, Min: 2, Max: 5}},
		},
		{
			name: "a threshold without a min asks for one edge or more",
			cond: Condition{Outbound: EdgeCondition{Edge: "amends", Max: testInt(3)}},
			want: []EdgeClause{{Edge: "amends", Min: 1, Max: 3}},
		},
		{
			name: "a threshold without an edge type names no edge",
			cond: Condition{Inbound: EdgeCondition{Min: testInt(2)}},
		},
		{
			name: "only the populated clauses come back",
			cond: Condition{NotInbound: "supersedes", Attr: map[string]AttrCondition{"status": testEq("superseded")}},
			want: []EdgeClause{{Edge: "supersedes", Inbound: true, Negate: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cond.EdgeClauses(); !slices.Equal(got, tt.want) {
				t.Fatalf("EdgeClauses = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidateRejectsAnUndeclaredEdgeInEveryClause(t *testing.T) {
	clauses := map[string]func(edge string) Condition{
		"inbound":      func(edge string) Condition { return Condition{Inbound: EdgeCondition{Edge: edge}} },
		"not_inbound":  func(edge string) Condition { return Condition{NotInbound: edge} },
		"outbound":     func(edge string) Condition { return Condition{Outbound: EdgeCondition{Edge: edge}} },
		"not_outbound": func(edge string) Condition { return Condition{NotOutbound: edge} },
	}
	for name, build := range clauses {
		t.Run(name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.Rules = []Rule{{Name: "r", Severity: model.SeverityWarn, When: build("relates-to"), Message: "m"}}

			err := cfg.Validate()

			if !errors.Is(err, model.ErrInvalidConfig) {
				t.Fatalf("Validate = %v, want an invalid configuration error", err)
			}
			if !strings.Contains(err.Error(), "relates-to") {
				t.Errorf("error = %v, want it to name the undeclared edge type", err)
			}
		})
	}
}

func TestFilenameTemplate(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "an unconfigured corpus takes the default", want: DefaultFilename},
		{name: "a configured template is used as written", filename: "{id}.md", want: "{id}.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.Filename = tt.filename
			if got := cfg.FilenameTemplate(); got != tt.want {
				t.Errorf("FilenameTemplate = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigValidateFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "an unconfigured template is valid"},
		{name: "the default carries both placeholders", filename: DefaultFilename},
		{name: "a bare numeric corpus needs no slug", filename: "{id}.md"},
		{name: "an underscore separator is allowed", filename: "{id}_{slug}.md"},
		{name: "a template without an identifier", filename: "{slug}.md", want: "{id}"},
		{name: "a template with no placeholder at all", filename: "decision.md", want: "{id}"},
		{name: "a slash reaches outside the documents directory", filename: "notes/{id}.md", want: "separator"},
		{name: "a backslash reaches outside the documents directory", filename: `notes\{id}.md`, want: "separator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.Filename = tt.filename

			err := cfg.Validate()

			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if !errors.Is(err, model.ErrInvalidConfig) {
				t.Fatalf("Validate = %v, want it to wrap %v", err, model.ErrInvalidConfig)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// testProjectionConfig is the ADR preset with a second projection layered on,
// so a mutation can break one projection without emptying the list.
func testProjectionConfig() Config {
	cfg := ADRPreset()
	cfg.Projections = append(slices.Clone(cfg.Projections), ProjectionSpec{
		Name: "depended_on",
		When: Condition{Inbound: EdgeCondition{Edge: EdgeDependsOn.String()}},
	})
	return cfg
}

func TestEdgeConditionAcceptsBothForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want EdgeCondition
	}{
		{
			name: "the edge name alone",
			src:  "inbound: supersedes\n",
			want: EdgeCondition{Edge: "supersedes"},
		},
		{
			name: "a threshold with a minimum",
			src:  "inbound: {edge: supersedes, min: 5}\n",
			want: EdgeCondition{Edge: "supersedes", Min: testInt(5)},
		},
		{
			name: "a threshold with both bounds, written as a block",
			src:  "inbound:\n  edge: supersedes\n  min: 2\n  max: 4\n",
			want: EdgeCondition{Edge: "supersedes", Min: testInt(2), Max: testInt(4)},
		},
		{
			name: "a threshold with a maximum alone",
			src:  "inbound: {edge: supersedes, max: 1}\n",
			want: EdgeCondition{Edge: "supersedes", Max: testInt(1)},
		},
		{name: "an absent clause"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Condition

			if err := yaml.Unmarshal([]byte(tt.src), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(got.Inbound, tt.want) {
				t.Fatalf("inbound = %+v, want %+v", got.Inbound, tt.want)
			}
		})
	}

	t.Run("the string form is sugar for one edge or more", func(t *testing.T) {
		var sugar, spelled Condition
		if err := yaml.Unmarshal([]byte("outbound: supersedes\n"), &sugar); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if err := yaml.Unmarshal([]byte("outbound: {edge: supersedes, min: 1}\n"), &spelled); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !slices.Equal(sugar.EdgeClauses(), spelled.EdgeClauses()) {
			t.Fatalf("clauses = %+v, want %+v", sugar.EdgeClauses(), spelled.EdgeClauses())
		}
	})

	t.Run("a clause that is neither a name nor a mapping is a decode error", func(t *testing.T) {
		var got Condition

		if err := yaml.Unmarshal([]byte("inbound: [supersedes]\n"), &got); err == nil {
			t.Fatalf("Unmarshal = %+v, want an error", got)
		}
	})
}

func TestConditionOneHopClauses(t *testing.T) {
	src := "via: {edge: depends-on, attr: {status: {eq: deprecated}}}\n" +
		"via_inbound: {edge: supersedes, attr: {status: {eq: accepted}}}\n"
	var cond Condition

	if err := yaml.Unmarshal([]byte(src), &cond); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	clauses := cond.ViaClauses()
	if len(clauses) != 2 {
		t.Fatalf("ViaClauses = %+v, want two", clauses)
	}
	if clauses[0].Edge != "depends-on" || clauses[0].Inbound || clauses[0].Key() != "via" {
		t.Errorf("outbound clause = %+v", clauses[0])
	}
	if clauses[1].Edge != "supersedes" || !clauses[1].Inbound || clauses[1].Key() != "via_inbound" {
		t.Errorf("inbound clause = %+v", clauses[1])
	}
	if got := testDeref(t, "via attr eq", clauses[0].Attr["status"].Eq); got != StatusDeprecated {
		t.Errorf("via attr eq = %q, want %q", got, StatusDeprecated)
	}
	if len(cond.EdgeClauses()) != 0 {
		t.Errorf("EdgeClauses = %+v, want none: a one-hop clause is not an existence question", cond.EdgeClauses())
	}
}

func TestEdgeClauseHolds(t *testing.T) {
	tests := []struct {
		name   string
		clause EdgeClause
		degree int
		want   bool
	}{
		{name: "one edge or more, met", clause: EdgeClause{Min: 1}, degree: 1, want: true},
		{name: "one edge or more, unmet", clause: EdgeClause{Min: 1}},
		{name: "a minimum met exactly", clause: EdgeClause{Min: 5}, degree: 5, want: true},
		{name: "a minimum missed by one", clause: EdgeClause{Min: 5}, degree: 4},
		{name: "a maximum met exactly", clause: EdgeClause{Min: 1, Max: 3}, degree: 3, want: true},
		{name: "a maximum exceeded by one", clause: EdgeClause{Min: 1, Max: 3}, degree: 4},
		{name: "an absent maximum is unbounded", clause: EdgeClause{Min: 1}, degree: 99, want: true},
		{name: "a negated clause wants no edge at all", clause: EdgeClause{Negate: true}, want: true},
		{name: "a negated clause with an edge", clause: EdgeClause{Negate: true}, degree: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clause.Holds(tt.degree); got != tt.want {
				t.Fatalf("Holds(%d) = %v, want %v", tt.degree, got, tt.want)
			}
		})
	}
}

func TestValidateTheExtendedConditionVocabulary(t *testing.T) {
	tests := []struct {
		name    string
		when    Condition
		wantErr bool
	}{
		{name: "a threshold on a declared edge", when: Condition{Inbound: EdgeCondition{Edge: "supersedes", Min: testInt(5)}}},
		{name: "a window on a declared edge", when: Condition{Outbound: EdgeCondition{Edge: "depends-on", Min: testInt(1), Max: testInt(3)}}},
		{
			name:    "a threshold on an undeclared edge",
			when:    Condition{Inbound: EdgeCondition{Edge: "relates-to", Min: testInt(2)}},
			wantErr: true,
		},
		{
			name:    "a threshold naming no edge at all",
			when:    Condition{Inbound: EdgeCondition{Min: testInt(2)}},
			wantErr: true,
		},
		{
			name:    "a minimum below one",
			when:    Condition{Inbound: EdgeCondition{Edge: "supersedes", Min: testInt(0)}},
			wantErr: true,
		},
		{
			name:    "a maximum of zero, which is absence written the wrong way",
			when:    Condition{Inbound: EdgeCondition{Edge: "supersedes", Max: testInt(0)}},
			wantErr: true,
		},
		{
			name:    "a minimum above the maximum",
			when:    Condition{Inbound: EdgeCondition{Edge: "supersedes", Min: testInt(3), Max: testInt(2)}},
			wantErr: true,
		},
		{
			name: "a one-hop clause on a declared edge",
			when: Condition{Via: &ViaCondition{Edge: "depends-on", Attr: map[string]AttrCondition{"status": testEq(StatusDeprecated)}}},
		},
		{
			name: "an inbound one-hop clause",
			when: Condition{ViaInbound: &ViaCondition{Edge: "supersedes", Attr: map[string]AttrCondition{"status": testEq(StatusAccepted)}}},
		},
		{
			name:    "a one-hop clause on an undeclared edge",
			when:    Condition{Via: &ViaCondition{Edge: "relates-to", Attr: map[string]AttrCondition{"status": testEq(StatusAccepted)}}},
			wantErr: true,
		},
		{
			name:    "a one-hop clause naming no edge",
			when:    Condition{Via: &ViaCondition{Attr: map[string]AttrCondition{"status": testEq(StatusAccepted)}}},
			wantErr: true,
		},
		{
			name:    "a one-hop attribute with two operands",
			when:    Condition{Via: &ViaCondition{Edge: "depends-on", Attr: map[string]AttrCondition{"status": {Eq: strptr("a"), Not: strptr("b")}}}},
			wantErr: true,
		},
		{
			name:    "a one-hop attribute with no operand",
			when:    Condition{Via: &ViaCondition{Edge: "depends-on", Attr: map[string]AttrCondition{"status": {}}}},
			wantErr: true,
		},
		{
			name:    "a threshold nested inside an alternative",
			when:    Condition{AnyOf: []Condition{{Inbound: EdgeCondition{Edge: "relates-to"}}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.Rules = []Rule{{Name: "r", Severity: model.SeverityWarn, When: tt.when, Message: "m"}}

			err := cfg.Validate()

			if tt.wantErr {
				if !errors.Is(err, model.ErrInvalidConfig) {
					t.Fatalf("Validate = %v, want it to wrap model.ErrInvalidConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate = %v, want no error", err)
			}
		})
	}
}

func TestValidateProjections(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "the preset's own projection"},
		{
			name: "a projection reading another one",
			mutate: func(c *Config) {
				c.Projections = append(c.Projections, ProjectionSpec{
					Name: "settled",
					When: Condition{Attr: map[string]AttrCondition{ProjectionAcceptedUnsuperseded: testEq("true")}},
				})
			},
		},
		{
			name: "a projection with alternatives",
			mutate: func(c *Config) {
				c.Projections[0] = ProjectionSpec{
					Name: ProjectionAcceptedUnsuperseded,
					AnyOf: []ProjectionAlt{
						{When: Condition{Attr: map[string]AttrCondition{DefaultStatusField: testEq(StatusAccepted)}}},
						{When: Condition{NotInbound: EdgeSupersedes.String()}},
					},
				}
			},
		},
		{
			name:    "a projection without a name",
			mutate:  func(c *Config) { c.Projections[0].Name = "" },
			wantErr: true,
		},
		{
			name: "two projections with the same name",
			mutate: func(c *Config) {
				c.Projections = append(c.Projections, c.Projections[0])
			},
			wantErr: true,
		},
		{
			name: "a projection with neither when nor any_of",
			mutate: func(c *Config) {
				c.Projections[0] = ProjectionSpec{Name: "empty"}
				c.Binding = "empty"
			},
			wantErr: true,
		},
		{
			name: "a projection with both when and any_of",
			mutate: func(c *Config) {
				c.Projections[0].AnyOf = []ProjectionAlt{{When: Condition{NotInbound: EdgeSupersedes.String()}}}
			},
			wantErr: true,
		},
		{
			name: "an alternative without a when block",
			mutate: func(c *Config) {
				c.Projections[0] = ProjectionSpec{Name: ProjectionAcceptedUnsuperseded, AnyOf: []ProjectionAlt{{}}}
			},
			wantErr: true,
		},
		{
			name:    "a projection naming an undeclared edge type",
			mutate:  func(c *Config) { c.Projections[0].When.Inbound = EdgeCondition{Edge: "relates-to"} },
			wantErr: true,
		},
		{
			name:    "a projection attribute with no operand",
			mutate:  func(c *Config) { c.Projections[0].When.Attr = map[string]AttrCondition{"status": {}} },
			wantErr: true,
		},
		{
			name:    "a binding naming no declared projection",
			mutate:  func(c *Config) { c.Binding = "effective" },
			wantErr: true,
		},
		{
			name: "a binding beside no projections at all, which asks for the built-in definition",
			mutate: func(c *Config) {
				c.Projections = []ProjectionSpec{}
			},
		},
		{
			name: "a rule reading a projection",
			mutate: func(c *Config) {
				c.Rules[0].When.Attr = map[string]AttrCondition{ProjectionAcceptedUnsuperseded: testEq("true")}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate()

			if tt.wantErr {
				if !errors.Is(err, model.ErrInvalidConfig) {
					t.Fatalf("Validate = %v, want it to wrap model.ErrInvalidConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate = %v, want no error", err)
			}
		})
	}
}

func TestValidateRejectsAProjectionReferenceCycle(t *testing.T) {
	reads := func(name, other string) ProjectionSpec {
		return ProjectionSpec{Name: name, When: Condition{Attr: map[string]AttrCondition{other: testEq("true")}}}
	}
	tests := []struct {
		name        string
		projections []ProjectionSpec
	}{
		{
			name:        "a projection reading itself",
			projections: []ProjectionSpec{reads("a", "a")},
		},
		{
			name:        "two projections reading each other",
			projections: []ProjectionSpec{reads("a", "b"), reads("b", "a")},
		},
		{
			name:        "a longer loop",
			projections: []ProjectionSpec{reads("a", "b"), reads("b", "c"), reads("c", "a")},
		},
		{
			name: "a loop closed through a one-hop clause",
			projections: []ProjectionSpec{
				reads("a", "b"),
				{Name: "b", When: Condition{Via: &ViaCondition{Edge: EdgeDependsOn.String(), Attr: map[string]AttrCondition{"a": testEq("true")}}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.Projections, cfg.Binding = tt.projections, ""

			err := cfg.Validate()

			if !errors.Is(err, model.ErrInvalidConfig) {
				t.Fatalf("Validate = %v, want it to wrap model.ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), "cycle") {
				t.Errorf("error = %v, want it to name the cycle", err)
			}
		})
	}

	t.Run("a forward reference in list order is not a cycle", func(t *testing.T) {
		cfg := ADRPreset()
		cfg.Projections = []ProjectionSpec{
			reads("a", "b"),
			{Name: "b", When: Condition{NotInbound: EdgeSupersedes.String()}},
		}
		cfg.Binding = "a"

		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate = %v, want no error", err)
		}
	})
}

func TestProjectionAccessors(t *testing.T) {
	cfg := testProjectionConfig()

	t.Run("names come back in declaration order", func(t *testing.T) {
		want := []string{ProjectionAcceptedUnsuperseded, "depended_on"}

		if got := cfg.ProjectionNames(); !slices.Equal(got, want) {
			t.Fatalf("ProjectionNames = %v, want %v", got, want)
		}
	})

	t.Run("the binding projection is the one binding names", func(t *testing.T) {
		spec, ok := cfg.BindingProjection()

		if !ok || spec.Name != ProjectionAcceptedUnsuperseded {
			t.Fatalf("BindingProjection = %+v, %v, want %q", spec, ok, ProjectionAcceptedUnsuperseded)
		}
	})

	t.Run("a configuration without projections resolves no binding", func(t *testing.T) {
		bare := cfg
		bare.Projections = nil

		if spec, ok := bare.BindingProjection(); ok {
			t.Fatalf("BindingProjection = %+v, want none", spec)
		}
	})

	t.Run("a projection reads its own attribute keys and its neighbours'", func(t *testing.T) {
		spec := ProjectionSpec{
			Name: "settled",
			AnyOf: []ProjectionAlt{
				{When: Condition{Attr: map[string]AttrCondition{"level": testEq("MUST")}}},
				{When: Condition{Via: &ViaCondition{Edge: "depends-on", Attr: map[string]AttrCondition{"enforced": testEq("true")}}}},
			},
		}
		want := []string{"enforced", "level"}

		if got := spec.AttrKeys(); !slices.Equal(got, want) {
			t.Fatalf("AttrKeys = %v, want %v", got, want)
		}
		if got := len(spec.Whens()); got != 2 {
			t.Fatalf("Whens = %d, want 2", got)
		}
	})
}

func TestEdgeAttrSpecAcceptsAndWants(t *testing.T) {
	tests := []struct {
		name  string
		spec  EdgeAttrSpec
		value string
		want  bool
		wants string
	}{
		{name: "a string takes anything", spec: EdgeAttrSpec{}, value: "haiku", want: true, wants: "a string"},
		{name: "a string takes an empty value", spec: EdgeAttrSpec{Type: AttrTypeString}, value: "", want: true, wants: "a string"},
		{name: "a number takes a decimal", spec: EdgeAttrSpec{Type: AttrTypeNumber}, value: "0.92", want: true, wants: "a number"},
		{name: "a number takes an integer", spec: EdgeAttrSpec{Type: AttrTypeNumber}, value: "3", want: true, wants: "a number"},
		{name: "a number takes a negative", spec: EdgeAttrSpec{Type: AttrTypeNumber}, value: "-1.5", want: true, wants: "a number"},
		{name: "a number refuses a word", spec: EdgeAttrSpec{Type: AttrTypeNumber}, value: "high", wants: "a number"},
		{name: "a number refuses an empty value", spec: EdgeAttrSpec{Type: AttrTypeNumber}, value: "", wants: "a number"},
		{name: "a date takes an ISO 8601 day", spec: EdgeAttrSpec{Type: AttrTypeDate}, value: "2026-01-01", want: true, wants: "a date as YYYY-MM-DD"},
		{name: "a date refuses a month nobody has", spec: EdgeAttrSpec{Type: AttrTypeDate}, value: "2026-13-01", wants: "a date as YYYY-MM-DD"},
		{name: "a date refuses a timestamp", spec: EdgeAttrSpec{Type: AttrTypeDate}, value: "2026-01-01T00:00:00Z", wants: "a date as YYYY-MM-DD"},
		{name: "a date refuses a shortened day", spec: EdgeAttrSpec{Type: AttrTypeDate}, value: "2026-1-1", wants: "a date as YYYY-MM-DD"},
		{
			name:  "a vocabulary takes one of its words",
			spec:  EdgeAttrSpec{OneOf: []string{"recurrence", "conflict"}},
			value: "conflict",
			want:  true,
			wants: "one of: recurrence, conflict",
		},
		{
			// Statuses fold case because a person writes them in prose; an edge
			// attribute is a closed machine vocabulary a preset revision renames
			// wholesale, so it compares exactly.
			name:  "a vocabulary refuses another spelling of one of its words",
			spec:  EdgeAttrSpec{OneOf: []string{"recurrence", "conflict"}},
			value: "Conflict",
			wants: "one of: recurrence, conflict",
		},
		{
			name:  "a vocabulary refuses a word nobody declared",
			spec:  EdgeAttrSpec{OneOf: []string{"recurrence", "conflict"}},
			value: "rewrite",
			wants: "one of: recurrence, conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.Accepts(tt.value); got != tt.want {
				t.Errorf("Accepts(%q) = %v, want %v", tt.value, got, tt.want)
			}
			if got := tt.spec.Wants(); got != tt.wants {
				t.Errorf("Wants = %q, want %q", got, tt.wants)
			}
		})
	}
}

func TestEdgeSpecAttrs(t *testing.T) {
	spec := EdgeSpec{
		Name: "measures",
		Key:  "measures",
		Attrs: map[string]EdgeAttrSpec{
			"model":     {Required: true},
			"agreement": {Required: true, Type: AttrTypeNumber},
		},
	}

	t.Run("the declared names are sorted", func(t *testing.T) {
		if got := spec.AttrNames(); !slices.Equal(got, []string{"agreement", "model"}) {
			t.Fatalf("AttrNames = %v, want them sorted", got)
		}
	})

	t.Run("a declared attribute is found by name", func(t *testing.T) {
		attr, ok := spec.Attr("agreement")

		if !ok || attr.ValueType() != AttrTypeNumber {
			t.Fatalf("Attr(agreement) = %+v, %v, want the number declaration", attr, ok)
		}
	})

	t.Run("an attribute nobody declared is not found", func(t *testing.T) {
		if attr, ok := spec.Attr("expires"); ok {
			t.Fatalf("Attr(expires) = %+v, want none", attr)
		}
	})

	t.Run("an edge declaring no attributes has none", func(t *testing.T) {
		bare := EdgeSpec{Name: "supersedes", Key: "supersedes"}

		if got := bare.AttrNames(); len(got) != 0 {
			t.Fatalf("AttrNames = %v, want none", got)
		}
	})

	t.Run("a declaration without a type reads a string", func(t *testing.T) {
		if got := spec.Attrs["model"].ValueType(); got != AttrTypeString {
			t.Fatalf("ValueType = %q, want %q", got, AttrTypeString)
		}
	})
}

func TestConfigKinds(t *testing.T) {
	cfg := ADRPreset()
	cfg.Kinds = testKinds()

	t.Run("a configuration without kinds is single-kind", func(t *testing.T) {
		if ADRPreset().Multikind() {
			t.Fatal("Multikind = true for the preset, want the single-kind corpus it has always been")
		}
	})

	t.Run("the kinds are reported in sorted order", func(t *testing.T) {
		if got := cfg.KindNames(); !slices.Equal(got, []string{"clause", "conform", "pm"}) {
			t.Fatalf("KindNames = %v, want them sorted", got)
		}
	})

	t.Run("a declared kind is found by name", func(t *testing.T) {
		spec, ok := cfg.Kind("conform")

		if !ok || spec.Dir != "spec/conform" {
			t.Fatalf("Kind(conform) = %+v, %v, want the declared spec", spec, ok)
		}
	})

	t.Run("a kind nobody declared is not found", func(t *testing.T) {
		if spec, ok := cfg.Kind("principle"); ok {
			t.Fatalf("Kind(principle) = %+v, want none", spec)
		}
	})

	t.Run("a kind keeps its own status vocabulary", func(t *testing.T) {
		if got := cfg.KindStatusValues("clause"); !slices.Equal(got, []string{"trial", "accepted"}) {
			t.Fatalf("KindStatusValues(clause) = %v, want the kind's own vocabulary", got)
		}
	})

	t.Run("a kind without one inherits the top-level vocabulary", func(t *testing.T) {
		if got := cfg.KindStatusValues("conform"); !slices.Equal(got, cfg.StatusValues) {
			t.Fatalf("KindStatusValues(conform) = %v, want the top-level %v", got, cfg.StatusValues)
		}
	})

	t.Run("a kind nobody declared answers to the top-level vocabulary", func(t *testing.T) {
		if got := cfg.KindStatusValues("principle"); !slices.Equal(got, cfg.StatusValues) {
			t.Fatalf("KindStatusValues(principle) = %v, want the top-level %v", got, cfg.StatusValues)
		}
	})
}

func TestConfigKnownFrontmatterKeys(t *testing.T) {
	t.Run("the preset knows its own vocabulary", func(t *testing.T) {
		want := []string{"date", "depends-on", "id", "kind", "status", "supersedes", "title"}

		if got := ADRPreset().KnownFrontmatterKeys(""); !slices.Equal(got, want) {
			t.Fatalf("KnownFrontmatterKeys = %v, want %v", got, want)
		}
	})

	t.Run("a renamed status field is the key that is known", func(t *testing.T) {
		// The merge retargets the derived edges onto the renamed field, so a
		// corpus that renames status carries the new key everywhere.
		cfg := Merge(ADRPreset(), Config{StatusField: "state"})

		got := cfg.KnownFrontmatterKeys("")

		if slices.Contains(got, "status") || !slices.Contains(got, "state") {
			t.Fatalf("KnownFrontmatterKeys = %v, want the renamed field rather than status", got)
		}
	})

	t.Run("an inverse key is known", func(t *testing.T) {
		cfg := ADRPreset()
		cfg.Edges[0].Inverse = "superseded_by"

		if got := cfg.KnownFrontmatterKeys(""); !slices.Contains(got, "superseded_by") {
			t.Fatalf("KnownFrontmatterKeys = %v, want it to carry the inverse key", got)
		}
	})

	t.Run("a declared field is a known key, deprecated or not", func(t *testing.T) {
		cfg := ADRPreset()
		cfg.Fields = map[string]FieldSpec{
			"owner": {Deprecated: true, MigrateTo: "owned-by"},
			"team":  {},
		}

		got := cfg.KnownFrontmatterKeys("")

		for _, want := range []string{"owner", "team"} {
			if !slices.Contains(got, want) {
				t.Fatalf("KnownFrontmatterKeys = %v, want it to carry the declared field %q", got, want)
			}
		}
	})

	t.Run("a kind's own fields are known to that kind alone", func(t *testing.T) {
		cfg := ADRPreset()
		cfg.Fields = map[string]FieldSpec{"owner": {Deprecated: true}}
		cfg.Kinds = map[string]KindSpec{
			"clause":  {Dir: "spec/clauses", Fields: map[string]FieldSpec{"level": {}}},
			"conform": {Dir: "spec/conform"},
		}

		clause, conform := cfg.KnownFrontmatterKeys("clause"), cfg.KnownFrontmatterKeys("conform")

		if !slices.Contains(clause, "level") || !slices.Contains(clause, "owner") {
			t.Fatalf("KnownFrontmatterKeys(clause) = %v, want the kind's field and the top-level one", clause)
		}
		if slices.Contains(conform, "level") {
			t.Fatalf("KnownFrontmatterKeys(conform) = %v, want another kind's field left out", conform)
		}
	})
}

func TestConfigFields(t *testing.T) {
	cfg := ADRPreset()
	cfg.Fields = map[string]FieldSpec{
		"owner": {Deprecated: true, Since: 2, MigrateTo: "owned-by", Sunset: "2027-01-01"},
		"team":  {},
	}
	cfg.Kinds = map[string]KindSpec{
		"clause": {Dir: "spec/clauses", Fields: map[string]FieldSpec{
			"owner": {},
			"level": {Deprecated: true},
		}},
		"conform": {Dir: "spec/conform"},
	}

	t.Run("a kind reads the top-level declarations", func(t *testing.T) {
		spec, ok := cfg.Field("conform", "owner")

		if !ok || !spec.Deprecated || spec.MigrateTo != "owned-by" {
			t.Fatalf("Field(conform, owner) = %+v, %v, want the top-level declaration", spec, ok)
		}
	})

	t.Run("a kind's own declaration wins over the top-level one", func(t *testing.T) {
		// The kind is describing that key for its own documents, so a clause
		// writing owner is not writing a retired field.
		spec, ok := cfg.Field("clause", "owner")

		if !ok || spec.Deprecated {
			t.Fatalf("Field(clause, owner) = %+v, %v, want the kind's own declaration", spec, ok)
		}
	})

	t.Run("a field nobody declared is not found", func(t *testing.T) {
		if spec, ok := cfg.Field("", "tags"); ok {
			t.Fatalf("Field(tags) = %+v, want none", spec)
		}
	})

	t.Run("every declared name is reported, sorted", func(t *testing.T) {
		if got := cfg.DeclaredFields(); !slices.Equal(got, []string{"level", "owner", "team"}) {
			t.Fatalf("DeclaredFields = %v, want the top-level and per-kind names sorted", got)
		}
	})

	t.Run("a corpus-wide report flags a field any kind retired", func(t *testing.T) {
		if !cfg.FieldDeprecated("level") {
			t.Error("FieldDeprecated(level) = false, want the clause kind's retirement to show")
		}
		if !cfg.FieldDeprecated("owner") {
			t.Error("FieldDeprecated(owner) = false, want the top-level retirement to show")
		}
		if cfg.FieldDeprecated("team") {
			t.Error("FieldDeprecated(team) = true, want a declared field nobody retired")
		}
	})

	t.Run("a sunset reads as a day, and an unwritten one as nothing", func(t *testing.T) {
		spec, _ := cfg.Field("", "owner")
		day, ok := spec.SunsetDate()

		if !ok || day.Format(AttrDateLayout) != "2027-01-01" {
			t.Fatalf("SunsetDate = %v, %v, want the declared day", day, ok)
		}
		if _, ok := (FieldSpec{Deprecated: true}).SunsetDate(); ok {
			t.Error("SunsetDate reported a day for a declaration that names none")
		}
	})
}

// testTarget declares a target condition on the preset's depends-on edge,
// which is the edge every target test is written against.
func testTarget(cfg *Config, target *TargetCondition) {
	cfg.Edges[1].Target = target
}

func TestValidateEdgeTargets(t *testing.T) {
	tests := []struct {
		name    string
		target  *TargetCondition
		wantErr bool
	}{
		{name: "no target at all constrains nothing"},
		{name: "the leaf of a declared lineage", target: &TargetCondition{LeafOf: "supersedes"}},
		{
			name:   "a local condition on the target",
			target: &TargetCondition{Condition: Condition{NotInbound: "supersedes", Attr: map[string]AttrCondition{"status": testEq(StatusAccepted)}}},
		},
		{
			name:   "a degree window on the target",
			target: &TargetCondition{Condition: Condition{Outbound: EdgeCondition{Edge: "depends-on", Min: testInt(1), Max: testInt(3)}}},
		},
		{
			name:   "the combinators, which nest",
			target: &TargetCondition{Condition: Condition{AnyOf: []Condition{{NotInbound: "supersedes"}, {Not: &Condition{Inbound: EdgeCondition{Edge: "depends-on"}}}}}},
		},
		{
			name:   "a projection read as a virtual attribute",
			target: &TargetCondition{Condition: Condition{Attr: map[string]AttrCondition{ProjectionAcceptedUnsuperseded: testEq("true")}}},
		},
		{
			name:    "a leaf_of naming an undeclared edge",
			target:  &TargetCondition{LeafOf: "relates-to"},
			wantErr: true,
		},
		{
			name:    "a target that constrains nothing",
			target:  &TargetCondition{},
			wantErr: true,
		},
		{
			name:    "a one-hop clause, which would be a second hop",
			target:  &TargetCondition{Condition: Condition{Via: &ViaCondition{Edge: "supersedes", Attr: map[string]AttrCondition{"status": testEq(StatusAccepted)}}}},
			wantErr: true,
		},
		{
			name:    "an inbound one-hop clause, for the same reason",
			target:  &TargetCondition{Condition: Condition{ViaInbound: &ViaCondition{Edge: "supersedes", Attr: map[string]AttrCondition{"status": testEq(StatusAccepted)}}}},
			wantErr: true,
		},
		{
			name:    "a one-hop clause nested inside an alternative",
			target:  &TargetCondition{Condition: Condition{AnyOf: []Condition{{Via: &ViaCondition{Edge: "supersedes"}}}}},
			wantErr: true,
		},
		{
			name:    "an undeclared edge in the condition itself",
			target:  &TargetCondition{Condition: Condition{NotInbound: "relates-to"}},
			wantErr: true,
		},
		{
			name:    "an attribute clause with no operand",
			target:  &TargetCondition{Condition: Condition{Attr: map[string]AttrCondition{"status": {}}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			testTarget(&cfg, tt.target)

			err := cfg.Validate()

			if tt.wantErr {
				if !errors.Is(err, model.ErrInvalidConfig) {
					t.Fatalf("Validate = %v, want it to wrap model.ErrInvalidConfig", err)
				}
				if !strings.Contains(err.Error(), `edge "depends-on" target`) {
					t.Errorf("Validate = %v, want it to name the edge and the key it was written under", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate = %v, want no error", err)
			}
		})
	}
}

func TestValidatePathConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints []PathConstraint
		wantErr     bool
	}{
		{name: "a corpus declaring none"},
		{
			name:        "a two-step path that must reach nothing",
			constraints: []PathConstraint{{Name: "amend_targets_current", Path: []string{"depends-on", "^supersedes"}, Equals: PathEqualsNone}},
		},
		{
			name:        "a one-step path compared against another",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes"}, SubsetOf: []string{"depends-on"}}},
		},
		{
			name: "two constraints over one corpus",
			constraints: []PathConstraint{
				{Name: "first", Path: []string{"supersedes"}, Equals: PathEqualsNone},
				{Name: "second", Path: []string{"depends-on", "supersedes"}, SubsetOf: []string{"depends-on"}},
			},
		},
		{
			name:        "a constraint without a name",
			constraints: []PathConstraint{{Path: []string{"supersedes"}, Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name: "one name declared twice",
			constraints: []PathConstraint{
				{Name: "scope", Path: []string{"supersedes"}, Equals: PathEqualsNone},
				{Name: "scope", Path: []string{"depends-on"}, Equals: PathEqualsNone},
			},
			wantErr: true,
		},
		{
			name:        "a path of no steps",
			constraints: []PathConstraint{{Name: "scope", Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name:        "a path of three steps",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes", "supersedes", "supersedes"}, Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name:        "a comparison path of three steps",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes"}, SubsetOf: []string{"depends-on", "depends-on", "depends-on"}}},
			wantErr:     true,
		},
		{
			name:        "a step naming an undeclared edge",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"relates-to"}, Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name:        "a reversed step naming an undeclared edge",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"^relates-to"}, Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name:        "a repetition, which is not an edge name",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes+"}, Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name:        "a step that is nothing but the reverse prefix",
			constraints: []PathConstraint{{Name: "scope", Path: []string{PathReverse}, Equals: PathEqualsNone}},
			wantErr:     true,
		},
		{
			name:        "a set the vocabulary does not write",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes"}, Equals: "all"}},
			wantErr:     true,
		},
		{
			name:        "both comparisons at once",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes"}, Equals: PathEqualsNone, SubsetOf: []string{"depends-on"}}},
			wantErr:     true,
		},
		{
			name:        "neither comparison",
			constraints: []PathConstraint{{Name: "scope", Path: []string{"supersedes"}}},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.PathConstraints = tt.constraints

			err := cfg.Validate()

			if tt.wantErr {
				if !errors.Is(err, model.ErrInvalidConfig) {
					t.Fatalf("Validate = %v, want it to wrap model.ErrInvalidConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate = %v, want no error", err)
			}
		})
	}
}

func TestPathSteps(t *testing.T) {
	got := PathSteps([]string{"amends", "^supersedes"})

	want := []PathStep{{Edge: "amends"}, {Edge: "supersedes", Inbound: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PathSteps = %+v, want %+v", got, want)
	}
	// A step renders as the configuration wrote it, so a finding names what the
	// reader has to look for in the file.
	if rendered := PathString([]string{got[0].String(), got[1].String()}); rendered != "amends -> ^supersedes" {
		t.Fatalf("PathString = %q, want the written path", rendered)
	}
	if steps := PathSteps(nil); len(steps) != 0 {
		t.Fatalf("PathSteps(nil) = %+v, want no steps", steps)
	}
}

func TestTargetConditionEmpty(t *testing.T) {
	tests := []struct {
		name   string
		target TargetCondition
		want   bool
	}{
		{name: "a target nobody finished writing", target: TargetCondition{}, want: true},
		{name: "the sugar alone", target: TargetCondition{LeafOf: "supersedes"}},
		{name: "a condition alone", target: TargetCondition{Condition: Condition{NotInbound: "supersedes"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.Empty(); got != tt.want {
				t.Fatalf("Empty = %v, want %v", got, tt.want)
			}
		})
	}
}
