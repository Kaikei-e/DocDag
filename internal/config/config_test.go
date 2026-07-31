package config

import (
	"errors"
	"slices"
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
