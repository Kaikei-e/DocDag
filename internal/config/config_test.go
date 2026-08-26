package config

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
)

func testEq(v string) AttrCondition  { return AttrCondition{Eq: &v} }
func testNot(v string) AttrCondition { return AttrCondition{Not: &v} }

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
			if got != tt.want {
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
			name:   "a structural check escalated to an error",
			mutate: func(c *Config) { c.Structural = map[string]model.Severity{model.RuleMissingFrontmatter: model.SeverityError} },
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
			mutate:  func(c *Config) { c.Rules[0].When.Inbound = "relates-to" },
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
				c.Rules[0].When.Inbound = ""
				c.Rules[0].When.Outbound = EdgeDependsOn.String()
			},
		},
		{
			name: "a warn-only rule set is valid",
			mutate: func(c *Config) {
				c.Rules[0].Severity = model.SeverityWarn
				c.Rules[1].When.Attr = map[string]AttrCondition{"status": testNot(StatusAccepted)}
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
				Inbound:     "supersedes",
				NotInbound:  "depends-on",
				Outbound:    "amends",
				NotOutbound: "relates-to",
			},
			want: []EdgeClause{
				{Edge: "supersedes", Inbound: true},
				{Edge: "depends-on", Inbound: true, Negate: true},
				{Edge: "amends"},
				{Edge: "relates-to", Negate: true},
			},
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
		"inbound":      func(edge string) Condition { return Condition{Inbound: edge} },
		"not_inbound":  func(edge string) Condition { return Condition{NotInbound: edge} },
		"outbound":     func(edge string) Condition { return Condition{Outbound: edge} },
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
