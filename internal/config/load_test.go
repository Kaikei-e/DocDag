package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
)

const testDocument = "---\ntitle: A decision\nstatus: accepted\ndate: 2025-01-01\n---\n\n# A decision\n"

func testTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func TestDiscoveryPaths(t *testing.T) {
	want := []string{"docs/adr", "doc/adr", "docs/decisions", "docs/ADR", "adr"}

	if got := DiscoveryPaths(); !slices.Equal(got, want) {
		t.Fatalf("DiscoveryPaths = %v, want %v (priority order)", got, want)
	}
}

func TestDiscover(t *testing.T) {
	norm := ADRPreset().Normalizer()

	t.Run("a well-known directory holding a managed document", func(t *testing.T) {
		root := testTree(t, map[string]string{"docs/adr/0001-a-decision.md": testDocument})

		got, err := Discover(root, norm)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if want := filepath.Join(root, "docs", "adr"); got != want {
			t.Fatalf("Discover = %q, want %q", got, want)
		}
	})

	order := []struct {
		name   string
		files  map[string]string
		expect string
	}{
		{
			name: "docs/adr beats every later candidate",
			files: map[string]string{
				"docs/adr/0001-a-decision.md":       testDocument,
				"doc/adr/0002-a-decision.md":        testDocument,
				"docs/decisions/0003-a-decision.md": testDocument,
				"adr/0004-a-decision.md":            testDocument,
			},
			expect: "docs/adr",
		},
		{
			name: "doc/adr beats docs/decisions",
			files: map[string]string{
				"doc/adr/0002-a-decision.md":        testDocument,
				"docs/decisions/0003-a-decision.md": testDocument,
			},
			expect: "doc/adr",
		},
		{
			name: "docs/decisions beats docs/ADR",
			files: map[string]string{
				"docs/decisions/0003-a-decision.md": testDocument,
				"docs/ADR/0004-a-decision.md":       testDocument,
			},
			expect: "docs/decisions",
		},
		{
			name: "docs/ADR beats adr",
			files: map[string]string{
				"docs/ADR/0004-a-decision.md": testDocument,
				"adr/0005-a-decision.md":      testDocument,
			},
			expect: "docs/ADR",
		},
		{
			name:   "adr is the last candidate",
			files:  map[string]string{"adr/0005-a-decision.md": testDocument},
			expect: "adr",
		},
		{
			name: "a candidate without a managed document is passed over",
			files: map[string]string{
				"docs/adr/README.md":     "# Index of decisions\n",
				"adr/0005-a-decision.md": testDocument,
			},
			expect: "adr",
		},
	}
	for _, tt := range order {
		t.Run(tt.name, func(t *testing.T) {
			root := testTree(t, tt.files)

			got, err := Discover(root, norm)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if want := filepath.Join(root, filepath.FromSlash(tt.expect)); got != want {
				t.Fatalf("Discover = %q, want %q", got, want)
			}
		})
	}

	failures := []struct {
		name  string
		files map[string]string
	}{
		{name: "no well-known directory exists", files: map[string]string{"notes.md": "# Notes\n"}},
		{name: "a well-known directory holds no managed document", files: map[string]string{"docs/adr/README.md": "# Index\n"}},
		{name: "a well-known directory is empty of markdown", files: map[string]string{"docs/adr/.keep": ""}},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			root := testTree(t, tt.files)

			got, err := Discover(root, norm)

			if !errors.Is(err, model.ErrNoDocuments) {
				t.Fatalf("Discover = %q, %v, want it to wrap model.ErrNoDocuments", got, err)
			}
			if got != "" {
				t.Errorf("Discover = %q, want an empty path on failure", got)
			}
		})
	}

	t.Run("a root that does not exist finds nothing", func(t *testing.T) {
		_, err := Discover(filepath.Join(t.TempDir(), "absent"), norm)

		if err == nil {
			t.Fatal("Discover succeeded, want an error")
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("a partial file sets only what it names", func(t *testing.T) {
		root := testTree(t, map[string]string{"docdag.yaml": "id_width: 6\n"})

		got, err := Load(filepath.Join(root, "docdag.yaml"))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		want := Config{IDWidth: 6}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Load = %+v, want %+v (everything else stays unset for the merge)", got, want)
		}
	})

	t.Run("an empty list in the file clears the preset list", func(t *testing.T) {
		root := testTree(t, map[string]string{"docdag.yaml": "derived_edges: []\n"})

		file, err := Load(filepath.Join(root, "docdag.yaml"))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		if got := Merge(ADRPreset(), file); len(got.DerivedEdges) != 0 {
			t.Fatalf("derived_edges = %+v, want the preset list cleared", got.DerivedEdges)
		}
	})

	t.Run("every key decodes", func(t *testing.T) {
		file := `preset: adr
dir: docs/decisions
id_width: 6
status_field: state
status_values:
  - draft
  - final
edges:
  - name: supersedes
    key: replaces
    acyclic: true
    direction: reverse
derived_edges:
  - field: state
    pattern: '(?i)^replaced by (\S+)'
    edge: supersedes
    direction: reverse
rules:
  - name: state_drift
    severity: error
    when:
      inbound: supersedes
      attr:
        state:
          not: replaced
    message: inbound replaces but the state disagrees
template: templates/decision.md
`
		root := testTree(t, map[string]string{"docdag.yaml": file})

		got, err := Load(filepath.Join(root, "docdag.yaml"))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Preset != "adr" || got.Dir != "docs/decisions" || got.IDWidth != 6 {
			t.Errorf("preset/dir/id_width = %q/%q/%d", got.Preset, got.Dir, got.IDWidth)
		}
		if got.StatusField != "state" || !slices.Equal(got.StatusValues, []string{"draft", "final"}) {
			t.Errorf("status field = %q, values = %v", got.StatusField, got.StatusValues)
		}
		if got.Template != "templates/decision.md" {
			t.Errorf("template = %q", got.Template)
		}
		wantEdges := []EdgeSpec{{Name: "supersedes", Key: "replaces", Acyclic: true, Direction: DirectionReverse}}
		if !reflect.DeepEqual(got.Edges, wantEdges) {
			t.Errorf("edges = %+v, want %+v", got.Edges, wantEdges)
		}
		wantDerived := []DerivedEdgeSpec{{Field: "state", Pattern: `(?i)^replaced by (\S+)`, Edge: "supersedes", Direction: DirectionReverse}}
		if !slices.Equal(got.DerivedEdges, wantDerived) {
			t.Errorf("derived edges = %+v, want %+v", got.DerivedEdges, wantDerived)
		}
		if len(got.Rules) != 1 {
			t.Fatalf("rules = %+v, want one", got.Rules)
		}
		rule := got.Rules[0]
		if rule.Name != "state_drift" || rule.Severity != model.SeverityError || rule.When.Inbound.Edge != "supersedes" {
			t.Errorf("rule = %+v", rule)
		}
		if got := testDeref(t, "rule attr not", rule.When.Attr["state"].Not); got != "replaced" {
			t.Errorf("rule attr not = %q, want replaced", got)
		}
		if rule.Message == "" {
			t.Error("rule message is empty, want the configured text")
		}
	})

	t.Run("an empty file is an empty configuration", func(t *testing.T) {
		root := testTree(t, map[string]string{"docdag.yaml": ""})

		got, err := Load(filepath.Join(root, "docdag.yaml"))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !reflect.DeepEqual(got, Config{}) {
			t.Fatalf("Load = %+v, want a zero configuration", got)
		}
	})

	t.Run("a missing file is an error", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "docdag.yaml"))

		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("err = %v, want it to wrap fs.ErrNotExist", err)
		}
	})

	t.Run("invalid YAML is an error", func(t *testing.T) {
		root := testTree(t, map[string]string{"docdag.yaml": "id_width: 6\n\tdir: docs/adr\n"})

		if _, err := Load(filepath.Join(root, "docdag.yaml")); err == nil {
			t.Fatal("Load succeeded, want a decode error")
		}
	})
}

func TestMerge(t *testing.T) {
	t.Run("an empty override changes nothing", func(t *testing.T) {
		base := ADRPreset()

		if got := Merge(base, Config{}); !reflect.DeepEqual(got, base) {
			t.Fatalf("Merge = %+v, want %+v", got, base)
		}
	})

	t.Run("a reference block replaces the base field by field", func(t *testing.T) {
		base := ADRPreset()
		base.References = ReferencesSpec{Dangling: ReferencesOff, Pattern: `^(\d+)$`}

		got := Merge(base, Config{References: ReferencesSpec{Dangling: "error", Scan: []string{ScanFrontmatter}}})

		want := ReferencesSpec{Dangling: "error", Pattern: `^(\d+)$`, Scan: []string{ScanFrontmatter}}
		if !reflect.DeepEqual(got.References, want) {
			t.Fatalf("references = %+v, want %+v", got.References, want)
		}
	})

	t.Run("a set width wins over the preset", func(t *testing.T) {
		if got := Merge(ADRPreset(), Config{IDWidth: 6}); got.IDWidth != 6 {
			t.Fatalf("id_width = %d, want 6", got.IDWidth)
		}
	})

	t.Run("an unset width keeps the preset width", func(t *testing.T) {
		if got := Merge(ADRPreset(), Config{Dir: "docs/adr"}); got.IDWidth != 4 {
			t.Fatalf("id_width = %d, want 4", got.IDWidth)
		}
	})

	t.Run("scalar overrides replace their base", func(t *testing.T) {
		override := Config{Preset: "adr", Dir: "docs/decisions", StatusField: "state", Template: "templates/decision.md"}

		got := Merge(ADRPreset(), override)

		if got.Dir != "docs/decisions" || got.StatusField != "state" || got.Template != "templates/decision.md" {
			t.Fatalf("Merge = %+v, want the override values", got)
		}
	})

	t.Run("an empty scalar keeps the base value", func(t *testing.T) {
		got := Merge(ADRPreset(), Config{IDWidth: 6})

		if got.StatusField != DefaultStatusField {
			t.Fatalf("status_field = %q, want %q", got.StatusField, DefaultStatusField)
		}
	})

	lists := []struct {
		name     string
		override Config
		check    func(t *testing.T, got Config)
	}{
		{
			name:     "status values replace rather than append",
			override: Config{StatusValues: []string{"draft", "final"}},
			check: func(t *testing.T, got Config) {
				if !slices.Equal(got.StatusValues, []string{"draft", "final"}) {
					t.Fatalf("status_values = %v, want exactly the override", got.StatusValues)
				}
			},
		},
		{
			name:     "edges replace rather than merge",
			override: Config{Edges: []EdgeSpec{{Name: "supersedes", Key: "replaces", Acyclic: true, Direction: DirectionForward}}},
			check: func(t *testing.T, got Config) {
				if len(got.Edges) != 1 || got.Edges[0].Key != "replaces" {
					t.Fatalf("edges = %+v, want exactly the override", got.Edges)
				}
			},
		},
		{
			name: "rules replace the preset rules",
			override: Config{Rules: []Rule{{
				Name:     "accepted_with_dependencies",
				Severity: model.SeverityWarn,
				When:     Condition{Outbound: EdgeCondition{Edge: "depends-on"}},
				Message:  "an accepted decision still depends on another",
			}}},
			check: func(t *testing.T, got Config) {
				if len(got.Rules) != 1 || got.Rules[0].Name != "accepted_with_dependencies" {
					t.Fatalf("rules = %+v, want exactly the override", got.Rules)
				}
			},
		},
		{
			name:     "derived edges replace the preset ones",
			override: Config{DerivedEdges: []DerivedEdgeSpec{{Field: "state", Pattern: `^replaced by (\S+)`, Edge: "supersedes", Direction: DirectionReverse}}},
			check: func(t *testing.T, got Config) {
				if len(got.DerivedEdges) != 1 || got.DerivedEdges[0].Field != "state" {
					t.Fatalf("derived_edges = %+v, want exactly the override", got.DerivedEdges)
				}
			},
		},
	}
	for _, tt := range lists {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, Merge(ADRPreset(), tt.override))
		})
	}

	t.Run("an explicit empty list clears the base list", func(t *testing.T) {
		got := Merge(ADRPreset(), Config{Edges: []EdgeSpec{}, Rules: []Rule{}, DerivedEdges: []DerivedEdgeSpec{}})

		if len(got.Edges) != 0 || len(got.Rules) != 0 || len(got.DerivedEdges) != 0 {
			t.Fatalf("edges = %+v, rules = %+v, derived_edges = %+v, want them cleared",
				got.Edges, got.Rules, got.DerivedEdges)
		}
	})

	t.Run("a list the override never mentions keeps the base list", func(t *testing.T) {
		got := Merge(ADRPreset(), Config{IDWidth: 6})

		if len(got.Edges) != 2 || len(got.Rules) != 2 || len(got.DerivedEdges) != 1 {
			t.Fatalf("edges = %+v, rules = %+v, derived_edges = %+v, want the preset lists",
				got.Edges, got.Rules, got.DerivedEdges)
		}
	})

	t.Run("the base is left unchanged", func(t *testing.T) {
		base := ADRPreset()

		Merge(base, Config{IDWidth: 6, StatusValues: []string{"draft"}, Rules: nil})

		if base.IDWidth != 4 || len(base.StatusValues) != 6 || len(base.Rules) != 2 {
			t.Fatalf("base = %+v, want it untouched", base)
		}
	})
}

func TestResolve(t *testing.T) {
	t.Run("zero configuration is the preset plus discovery", func(t *testing.T) {
		root := testTree(t, map[string]string{"docs/adr/0001-a-decision.md": testDocument})

		got, err := Resolve(Options{Root: root})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if want := filepath.Join(root, "docs", "adr"); got.Dir != want {
			t.Errorf("dir = %q, want %q", got.Dir, want)
		}
		if got.Preset != PresetADR || got.IDWidth != DefaultIDWidth {
			t.Errorf("preset = %q, id_width = %d, want the ADR defaults", got.Preset, got.IDWidth)
		}
		if len(got.Edges) != 2 || len(got.Rules) != 2 {
			t.Errorf("edges = %+v, rules = %+v, want the preset defaults", got.Edges, got.Rules)
		}
	})

	t.Run("a directory option skips discovery", func(t *testing.T) {
		docs := testTree(t, map[string]string{"0001-a-decision.md": testDocument})
		root := testTree(t, nil)

		got, err := Resolve(Options{Root: root, Dir: docs})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Dir != docs {
			t.Fatalf("dir = %q, want %q", got.Dir, docs)
		}
	})

	t.Run("docdag.yaml at the root is loaded without an option", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"docdag.yaml":                 "id_width: 6\n",
			"docs/adr/0001-a-decision.md": testDocument,
		})

		got, err := Resolve(Options{Root: root})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.IDWidth != 6 {
			t.Errorf("id_width = %d, want 6 (the file overrides the preset)", got.IDWidth)
		}
		if want := filepath.Join(root, "docs", "adr"); got.Dir != want {
			t.Errorf("dir = %q, want %q", got.Dir, want)
		}
	})

	t.Run("a root without docdag.yaml is not an error", func(t *testing.T) {
		root := testTree(t, map[string]string{"docs/adr/0001-a-decision.md": testDocument})

		if _, err := Resolve(Options{Root: root}); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	})

	t.Run("an explicit config path is loaded from anywhere", func(t *testing.T) {
		docs := testTree(t, map[string]string{"0001-a-decision.md": testDocument})
		elsewhere := testTree(t, map[string]string{"docdag.yaml": "id_width: 6\ndir: " + docs + "\n"})
		root := testTree(t, nil)

		got, err := Resolve(Options{Root: root, ConfigPath: filepath.Join(elsewhere, "docdag.yaml")})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.IDWidth != 6 {
			t.Errorf("id_width = %d, want 6", got.IDWidth)
		}
		if got.Dir != docs {
			t.Errorf("dir = %q, want %q", got.Dir, docs)
		}
	})

	t.Run("the directory option beats the config file", func(t *testing.T) {
		docs := testTree(t, map[string]string{"0001-a-decision.md": testDocument})
		absent := filepath.Join(t.TempDir(), "absent")
		root := testTree(t, map[string]string{"docdag.yaml": "dir: " + absent + "\n"})

		got, err := Resolve(Options{Root: root, Dir: docs})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Dir != docs {
			t.Fatalf("dir = %q, want the flag value %q", got.Dir, docs)
		}
	})

	t.Run("the config file merges over the preset it names", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"docdag.yaml":                 "preset: adr\nid_width: 6\nstatus_values:\n  - draft\n  - final\n",
			"docs/adr/0001-a-decision.md": testDocument,
		})

		got, err := Resolve(Options{Root: root})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.IDWidth != 6 {
			t.Errorf("id_width = %d, want 6", got.IDWidth)
		}
		if !slices.Equal(got.StatusValues, []string{"draft", "final"}) {
			t.Errorf("status_values = %v, want exactly the file's", got.StatusValues)
		}
		if len(got.Edges) != 2 || len(got.Rules) != 2 {
			t.Errorf("edges = %+v, rules = %+v, want the preset's", got.Edges, got.Rules)
		}
	})

	t.Run("a missing config path is an error", func(t *testing.T) {
		root := testTree(t, map[string]string{"docs/adr/0001-a-decision.md": testDocument})

		_, err := Resolve(Options{Root: root, ConfigPath: filepath.Join(root, "absent.yaml")})

		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("err = %v, want it to wrap fs.ErrNotExist", err)
		}
	})

	t.Run("nothing to discover is an error", func(t *testing.T) {
		root := testTree(t, map[string]string{"notes.md": "# Notes\n"})

		_, err := Resolve(Options{Root: root})

		if !errors.Is(err, model.ErrNoDocuments) {
			t.Fatalf("err = %v, want it to wrap model.ErrNoDocuments", err)
		}
	})

	t.Run("an unknown preset in the config file is an error", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"docdag.yaml":                 "preset: mkdocs\n",
			"docs/adr/0001-a-decision.md": testDocument,
		})

		_, err := Resolve(Options{Root: root})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("err = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})

	t.Run("an invalid merged configuration is rejected", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"docdag.yaml":                 "edges:\n  - name: supersedes\n    key: supersedes\n    direction: sideways\n",
			"docs/adr/0001-a-decision.md": testDocument,
		})

		_, err := Resolve(Options{Root: root})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("err = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})

	t.Run("a config file that does not decode is an invalid configuration", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"docdag.yaml":                 "id_width: [not, a, number\n",
			"docs/adr/0001-a-decision.md": testDocument,
		})

		_, err := Resolve(Options{Root: root})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("err = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})
}

func TestMergeRetargetsThePresetOntoARenamedStatusField(t *testing.T) {
	// A one-line "status_field: state" override has to carry the preset rules
	// and derived edges with it, or they inspect an attribute nothing writes.
	got := Merge(ADRPreset(), Config{StatusField: "state"})

	for _, rule := range got.Rules {
		if _, ok := rule.When.Attr["state"]; !ok {
			t.Errorf("rule %q inspects %v, want the configured status field", rule.Name, rule.When.Attr)
		}
	}
	for _, spec := range got.DerivedEdges {
		if spec.Field != "state" {
			t.Errorf("derived edge reads field %q, want the configured status field", spec.Field)
		}
	}
	if base := ADRPreset(); base.Rules[0].When.Attr[DefaultStatusField].Not == nil {
		t.Error("Merge rewrote the preset it was given rather than a copy")
	}
}

func TestMergeKeepsAnExplicitRuleOverrideOnARenamedStatusField(t *testing.T) {
	override := Config{
		StatusField: "state",
		Rules: []Rule{{
			Name:     "custom",
			Severity: model.SeverityWarn,
			When:     Condition{Attr: map[string]AttrCondition{"lifecycle": {Eq: ptr("done")}}},
			Message:  "the author's own rule",
		}},
	}

	got := Merge(ADRPreset(), override)

	if len(got.Rules) != 1 {
		t.Fatalf("rules = %+v, want only the override", got.Rules)
	}
	if _, ok := got.Rules[0].When.Attr["lifecycle"]; !ok {
		t.Fatalf("rule attr = %v, want the override's own key untouched", got.Rules[0].When.Attr)
	}
}

func TestMergeProjections(t *testing.T) {
	t.Run("projections replace the preset's rather than merge", func(t *testing.T) {
		override := Config{
			Projections: []ProjectionSpec{{Name: "enforced", When: Condition{Inbound: EdgeCondition{Edge: "depends-on"}}}},
			Binding:     "enforced",
		}

		got := Merge(ADRPreset(), override)

		if len(got.Projections) != 1 || got.Projections[0].Name != "enforced" {
			t.Fatalf("projections = %+v, want exactly the override", got.Projections)
		}
		if got.Binding != "enforced" {
			t.Fatalf("binding = %q, want the override", got.Binding)
		}
	})

	t.Run("an explicit empty list clears the preset's projections", func(t *testing.T) {
		got := Merge(ADRPreset(), Config{Projections: []ProjectionSpec{}})

		if len(got.Projections) != 0 {
			t.Fatalf("projections = %+v, want them cleared", got.Projections)
		}
		if _, ok := got.BindingProjection(); ok {
			t.Fatal("BindingProjection resolved one, want the built-in definition to stand in")
		}
	})

	t.Run("a file that mentions neither keeps the preset's", func(t *testing.T) {
		got := Merge(ADRPreset(), Config{IDWidth: 6})

		spec, ok := got.BindingProjection()
		if !ok || spec.Name != ProjectionAcceptedUnsuperseded {
			t.Fatalf("BindingProjection = %+v, %v, want the preset's", spec, ok)
		}
	})

	t.Run("a replaced edge vocabulary drops the projections written against it", func(t *testing.T) {
		override := Config{
			Edges:        []EdgeSpec{{Name: "replaces", Key: "replaces", Acyclic: true, Direction: DirectionForward}},
			Rules:        []Rule{},
			DerivedEdges: []DerivedEdgeSpec{},
		}

		got := Merge(ADRPreset(), override)

		if len(got.Projections) != 0 {
			t.Fatalf("projections = %+v, want the ones reading supersedes dropped", got.Projections)
		}
		if got.Binding != "" {
			t.Fatalf("binding = %q, want it cleared with the projection it named", got.Binding)
		}
		if err := got.Validate(); err != nil {
			t.Fatalf("Validate = %v, want no error on a configuration nobody wrote", err)
		}
	})

	t.Run("a replaced edge vocabulary that still declares the edge keeps them", func(t *testing.T) {
		override := Config{Edges: []EdgeSpec{
			{Name: "supersedes", Key: "supersedes", Acyclic: true, Direction: DirectionForward},
			{Name: "amends", Key: "amends", Direction: DirectionForward},
		}}

		got := Merge(ADRPreset(), override)

		if len(got.Projections) != 1 || got.Binding != ProjectionAcceptedUnsuperseded {
			t.Fatalf("projections = %+v, binding = %q, want the preset's kept", got.Projections, got.Binding)
		}
	})
}

func TestMergeRetargetsProjectionsOntoARenamedStatusField(t *testing.T) {
	// The binding projection reads the status field. Left on the preset's key
	// it would hold for no document at all, and every listing would go empty.
	got := Merge(ADRPreset(), Config{StatusField: "state"})

	spec, ok := got.BindingProjection()
	if !ok {
		t.Fatalf("BindingProjection = %+v, want the preset's", got.Projections)
	}
	if _, ok := spec.When.Attr["state"]; !ok {
		t.Fatalf("projection inspects %v, want the configured status field", spec.When.Attr)
	}
	if _, ok := spec.When.Attr[DefaultStatusField]; ok {
		t.Fatalf("projection still inspects %q", DefaultStatusField)
	}
	if base := ADRPreset(); base.Projections[0].When.Attr[DefaultStatusField].Eq == nil {
		t.Error("Merge rewrote the preset it was given rather than a copy")
	}

	t.Run("alternatives are retargeted too", func(t *testing.T) {
		base := ADRPreset()
		base.Projections = []ProjectionSpec{{
			Name: ProjectionAcceptedUnsuperseded,
			AnyOf: []ProjectionAlt{
				{When: Condition{Attr: map[string]AttrCondition{DefaultStatusField: testEq(StatusAccepted)}}},
			},
		}}

		merged := Merge(base, Config{StatusField: "state"})

		if _, ok := merged.Projections[0].AnyOf[0].When.Attr["state"]; !ok {
			t.Fatalf("alternative inspects %v, want the configured status field", merged.Projections[0].AnyOf[0].When.Attr)
		}
	})

	t.Run("a file that writes its own projections keeps them untouched", func(t *testing.T) {
		override := Config{
			StatusField: "state",
			Projections: []ProjectionSpec{{
				Name: "own",
				When: Condition{Attr: map[string]AttrCondition{"lifecycle": {Eq: ptr("done")}}},
			}},
			Binding: "own",
		}

		merged := Merge(ADRPreset(), override)

		if len(merged.Projections) != 1 {
			t.Fatalf("projections = %+v, want only the override", merged.Projections)
		}
		if _, ok := merged.Projections[0].When.Attr["lifecycle"]; !ok {
			t.Fatalf("projection attr = %v, want the override's own key untouched", merged.Projections[0].When.Attr)
		}
	})
}

func ptr(v string) *string { return &v }

func TestDiscoverReportsAnUnreadableCandidate(t *testing.T) {
	// A candidate that exists but cannot be read must not be mistaken for one
	// that is absent: silently validating a later directory answers a question
	// nobody asked.
	root := testTree(t, map[string]string{
		"docs/adr":               "not a directory\n",
		"adr/0001-a-decision.md": testDocument,
	})

	got, err := Discover(root, ADRPreset().Normalizer())

	if err == nil {
		t.Fatalf("Discover = %q, want an error naming the unreadable candidate", got)
	}
	if !strings.Contains(err.Error(), filepath.Join("docs", "adr")) {
		t.Errorf("err = %v, want it to name docs/adr", err)
	}
}

func TestDiscoverHonorsOnDiskCasing(t *testing.T) {
	// On a case-insensitive filesystem a stat of docs/adr also answers for a
	// directory spelled docs/ADR; discovery must match the on-disk spelling.
	root := testTree(t, map[string]string{
		"docs/ADR/0001-a-decision.md": testDocument,
	})

	for name, tc := range map[string]struct {
		candidate string
		want      bool
	}{
		"the on-disk spelling matches":     {candidate: "docs/ADR", want: true},
		"a differently-cased spelling":     {candidate: "docs/adr", want: false},
		"an absent path does not match":    {candidate: "docs/decisions", want: false},
		"a parent component is case-exact": {candidate: "DOCS/ADR", want: false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := matchesOnDiskCase(root, tc.candidate)
			if err != nil {
				t.Fatalf("matchesOnDiskCase: %v", err)
			}
			if got != tc.want {
				t.Errorf("matchesOnDiskCase(root, %q) = %v, want %v", tc.candidate, got, tc.want)
			}
		})
	}

	got, err := Discover(root, ADRPreset().Normalizer())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if want := filepath.Join(root, "docs", "ADR"); got != want {
		t.Errorf("Discover = %q, want %q", got, want)
	}
}

func TestMergeOverridesTheFilenameTemplate(t *testing.T) {
	if merged := Merge(ADRPreset(), Config{Filename: "{id}.md"}); merged.Filename != "{id}.md" {
		t.Errorf("Filename = %q, want the override %q", merged.Filename, "{id}.md")
	}
	if kept := Merge(ADRPreset(), Config{}); kept.Filename != ADRPreset().Filename {
		t.Errorf("Filename = %q, want the base %q", kept.Filename, ADRPreset().Filename)
	}
}

func TestLoadEdgeAttributes(t *testing.T) {
	file := `edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    attrs:
      reason: {required: true, one_of: [recurrence, conflict]}
  - name: measures
    key: measures
    direction: forward
    attrs:
      agreement: {required: true, type: number}
      expires: {type: date}
`
	root := testTree(t, map[string]string{"docdag.yaml": file})

	got, err := Load(filepath.Join(root, "docdag.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := []EdgeSpec{
		{
			Name: "supersedes", Key: "supersedes", Acyclic: true, Direction: DirectionForward,
			Attrs: map[string]EdgeAttrSpec{"reason": {Required: true, OneOf: []string{"recurrence", "conflict"}}},
		},
		{
			Name: "measures", Key: "measures", Direction: DirectionForward,
			Attrs: map[string]EdgeAttrSpec{
				"agreement": {Required: true, Type: AttrTypeNumber},
				"expires":   {Type: AttrTypeDate},
			},
		},
	}
	if !reflect.DeepEqual(got.Edges, want) {
		t.Fatalf("edges = %+v, want %+v", got.Edges, want)
	}
	if err := Merge(ADRPreset(), got).Validate(); err != nil {
		t.Fatalf("Validate = %v, want the merged configuration to be valid", err)
	}
}

// testKindsFile is a configuration file declaring two kinds, the shape a
// multi-kind corpus is described in.
const testKindsFile = `kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    closed: true
  conform:
    dir: spec/conform
    id: '^conform/[a-z0-9-]+$'
`

func TestLoadKinds(t *testing.T) {
	file := `kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    status_values: [trial, accepted]
    closed: true
  conform:
    dir: spec/conform
edges:
  - name: enforces
    key: enforces
    direction: forward
    from: [conform]
    to: [clause]
`
	root := testTree(t, map[string]string{"docdag.yaml": file})

	got, err := Load(filepath.Join(root, "docdag.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantKinds := map[string]KindSpec{
		"clause":  {Dir: "spec/clauses", ID: `^UZ-[A-Z]-\d{3}$`, StatusValues: []string{"trial", "accepted"}, Closed: true},
		"conform": {Dir: "spec/conform"},
	}
	if !reflect.DeepEqual(got.Kinds, wantKinds) {
		t.Errorf("kinds = %+v, want %+v", got.Kinds, wantKinds)
	}
	wantEdges := []EdgeSpec{{
		Name: "enforces", Key: "enforces", Direction: DirectionForward,
		From: []string{"conform"}, To: []string{"clause"},
	}}
	if !reflect.DeepEqual(got.Edges, wantEdges) {
		t.Errorf("edges = %+v, want %+v", got.Edges, wantEdges)
	}
}

func TestLoadFields(t *testing.T) {
	// The unquoted sunset is how a person writes a date in YAML, and it has to
	// reach the checker as the day it reads as.
	file := `preset_version: 3
fields:
  owner: {deprecated: true, since: 2, migrate_to: "owned-by", sunset: 2027-01-01}
  team: {}
kinds:
  clause:
    dir: spec/clauses
    fields:
      level: {deprecated: true, migrate_to: modality}
`
	root := testTree(t, map[string]string{"docdag.yaml": file})

	got, err := Load(filepath.Join(root, "docdag.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.PresetVersion != 3 {
		t.Errorf("preset_version = %d, want 3", got.PresetVersion)
	}
	want := map[string]FieldSpec{
		"owner": {Deprecated: true, Since: 2, MigrateTo: "owned-by", Sunset: "2027-01-01"},
		"team":  {},
	}
	if !reflect.DeepEqual(got.Fields, want) {
		t.Errorf("fields = %+v, want %+v", got.Fields, want)
	}
	wantKind := map[string]FieldSpec{"level": {Deprecated: true, MigrateTo: "modality"}}
	if !reflect.DeepEqual(got.Kinds["clause"].Fields, wantKind) {
		t.Errorf("kind fields = %+v, want %+v", got.Kinds["clause"].Fields, wantKind)
	}
}

func TestMergeFields(t *testing.T) {
	t.Run("a written fields map replaces the base wholesale", func(t *testing.T) {
		base := ADRPreset()
		base.Fields = map[string]FieldSpec{"owner": {Deprecated: true}, "team": {}}

		got := Merge(base, Config{Fields: map[string]FieldSpec{"team": {Deprecated: true}}})

		want := map[string]FieldSpec{"team": {Deprecated: true}}
		if !reflect.DeepEqual(got.Fields, want) {
			t.Fatalf("fields = %+v, want exactly the override %+v", got.Fields, want)
		}
	})

	t.Run("an empty fields map clears the base", func(t *testing.T) {
		base := ADRPreset()
		base.Fields = map[string]FieldSpec{"owner": {Deprecated: true}}

		if got := Merge(base, Config{Fields: map[string]FieldSpec{}}); len(got.Fields) != 0 {
			t.Fatalf("fields = %+v, want the explicit empty map to clear them", got.Fields)
		}
	})

	t.Run("an unwritten fields map keeps the base", func(t *testing.T) {
		base := ADRPreset()
		base.Fields = map[string]FieldSpec{"owner": {Deprecated: true}}

		if got := Merge(base, Config{IDWidth: 6}); !reflect.DeepEqual(got.Fields, base.Fields) {
			t.Fatalf("fields = %+v, want the base %+v", got.Fields, base.Fields)
		}
	})

	t.Run("the merged map is a copy of the override", func(t *testing.T) {
		override := Config{Fields: map[string]FieldSpec{"owner": {Deprecated: true}}}

		got := Merge(ADRPreset(), override)
		override.Fields["team"] = FieldSpec{}

		if _, leaked := got.Fields["team"]; leaked {
			t.Fatalf("fields = %+v, want the merge to have copied the map", got.Fields)
		}
	})

	t.Run("a written preset version replaces the preset's", func(t *testing.T) {
		if got := Merge(ADRPreset(), Config{PresetVersion: 3}); got.PresetVersion != 3 {
			t.Fatalf("preset_version = %d, want the override's 3", got.PresetVersion)
		}
	})

	t.Run("an unwritten preset version keeps the preset's", func(t *testing.T) {
		if got := Merge(ADRPreset(), Config{IDWidth: 6}); got.PresetVersion != ADRPresetVersion {
			t.Fatalf("preset_version = %d, want the preset's %d", got.PresetVersion, ADRPresetVersion)
		}
	})
}

func TestMergeKinds(t *testing.T) {
	t.Run("a written kinds map replaces the base wholesale", func(t *testing.T) {
		base := ADRPreset()
		base.Kinds = map[string]KindSpec{
			"clause":  {Dir: "spec/clauses"},
			"premise": {Dir: "spec/premises"},
		}

		got := Merge(base, Config{Kinds: map[string]KindSpec{"clause": {Dir: "standard/clauses"}}})

		want := map[string]KindSpec{"clause": {Dir: "standard/clauses"}}
		if !reflect.DeepEqual(got.Kinds, want) {
			t.Fatalf("kinds = %+v, want exactly the override %+v", got.Kinds, want)
		}
	})

	t.Run("an empty kinds map clears the base", func(t *testing.T) {
		base := ADRPreset()
		base.Kinds = map[string]KindSpec{"clause": {Dir: "spec/clauses"}}

		got := Merge(base, Config{Kinds: map[string]KindSpec{}})

		if got.Multikind() {
			t.Fatalf("kinds = %+v, want the explicit empty map to clear them", got.Kinds)
		}
	})

	t.Run("an unwritten kinds map keeps the base", func(t *testing.T) {
		base := ADRPreset()
		base.Kinds = map[string]KindSpec{"clause": {Dir: "spec/clauses"}}

		got := Merge(base, Config{IDWidth: 6})

		if !reflect.DeepEqual(got.Kinds, base.Kinds) {
			t.Fatalf("kinds = %+v, want the base %+v", got.Kinds, base.Kinds)
		}
	})

	t.Run("the merged map is a copy of the override", func(t *testing.T) {
		override := Config{Kinds: map[string]KindSpec{"clause": {Dir: "spec/clauses"}}}

		got := Merge(ADRPreset(), override)
		override.Kinds["conform"] = KindSpec{Dir: "spec/conform"}

		if _, leaked := got.Kinds["conform"]; leaked {
			t.Fatalf("kinds = %+v, want the merge to have copied the map", got.Kinds)
		}
	})
}

func TestResolveKinds(t *testing.T) {
	t.Run("kind directories are read relative to the configuration file", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"standard/docdag.yaml":              testKindsFile,
			"standard/spec/clauses/UZ-V-001.md": testDocument,
		})

		got, err := Resolve(Options{Root: root, ConfigPath: filepath.Join(root, "standard", "docdag.yaml")})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if want := filepath.Join(root, "standard", "spec", "clauses"); got.Kinds["clause"].Dir != want {
			t.Errorf("clause dir = %q, want %q", got.Kinds["clause"].Dir, want)
		}
		if got.Dir != "" {
			t.Errorf("dir = %q, want none: the kinds carry the directories", got.Dir)
		}
	})

	t.Run("a configuration file at the root roots the kinds there", func(t *testing.T) {
		root := testTree(t, map[string]string{
			"docdag.yaml":              testKindsFile,
			"spec/clauses/UZ-V-001.md": testDocument,
		})

		got, err := Resolve(Options{Root: root})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if want := filepath.Join(root, "spec", "conform"); got.Kinds["conform"].Dir != want {
			t.Errorf("conform dir = %q, want %q", got.Kinds["conform"].Dir, want)
		}
	})

	t.Run("an absolute kind directory is left alone", func(t *testing.T) {
		clauses := testTree(t, map[string]string{"UZ-V-001.md": testDocument})
		root := testTree(t, map[string]string{
			"docdag.yaml": "kinds:\n  clause:\n    dir: " + filepath.ToSlash(clauses) + "\n",
		})

		got, err := Resolve(Options{Root: root})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.Kinds["clause"].Dir != clauses {
			t.Errorf("clause dir = %q, want the absolute %q", got.Kinds["clause"].Dir, clauses)
		}
	})

	t.Run("kinds skip discovery entirely", func(t *testing.T) {
		// Nothing under a well-known documents directory, which single-kind
		// discovery would have failed on.
		root := testTree(t, map[string]string{
			"docdag.yaml":              testKindsFile,
			"spec/clauses/UZ-V-001.md": testDocument,
		})

		if _, err := Resolve(Options{Root: root}); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	})

	t.Run("a written id_width beside kinds is a configuration error", func(t *testing.T) {
		root := testTree(t, map[string]string{"docdag.yaml": testKindsFile + "id_width: 6\n"})

		_, err := Resolve(Options{Root: root})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("Resolve = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})

	t.Run("a written dir beside kinds is a configuration error", func(t *testing.T) {
		root := testTree(t, map[string]string{"docdag.yaml": testKindsFile + "dir: docs/adr\n"})

		_, err := Resolve(Options{Root: root})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("Resolve = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})

	t.Run("a directory option beside kinds is a configuration error", func(t *testing.T) {
		docs := testTree(t, map[string]string{"0001-a-decision.md": testDocument})
		root := testTree(t, map[string]string{"docdag.yaml": testKindsFile})

		_, err := Resolve(Options{Root: root, Dir: docs})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("Resolve = %v, want it to wrap model.ErrInvalidConfig", err)
		}
	})
}
