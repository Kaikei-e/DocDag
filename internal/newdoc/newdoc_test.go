package newdoc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

func testConfig(width int) config.Config {
	cfg := config.ADRPreset()
	cfg.IDWidth = width
	return cfg
}

func testGraph(ids ...string) *model.Graph {
	g := &model.Graph{Nodes: make(map[model.ID]*model.Node, len(ids))}
	for _, id := range ids {
		g.Nodes[model.ID(id)] = &model.Node{
			ID:     model.ID(id),
			Path:   id + ".md",
			Title:  "Decision " + id,
			Status: config.StatusAccepted,
		}
	}
	return g
}

func testEdgeSpec(name, key string) config.EdgeSpec {
	return config.EdgeSpec{Name: name, Key: key, Acyclic: true, Direction: config.DirectionForward}
}

func TestNextID(t *testing.T) {
	tests := []struct {
		name  string
		ids   []string
		width int
		want  model.ID
	}{
		{
			name:  "the next identifier follows the highest one",
			ids:   []string{"0001", "0002", "0003", "0004", "0005", "0006"},
			width: 4,
			want:  "0007",
		},
		{
			name:  "an empty corpus starts at one",
			width: 4,
			want:  "0001",
		},
		{
			name:  "gaps in the corpus are not filled",
			ids:   []string{"0001", "0009"},
			width: 4,
			want:  "0010",
		},
		{
			name:  "the identifier is padded to the configured width",
			ids:   []string{"000001", "000009"},
			width: 6,
			want:  "000010",
		},
		{
			name:  "a number that outgrows the width is not truncated",
			ids:   []string{"9999"},
			width: 4,
			want:  "10000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextID(testGraph(tt.ids...), testConfig(tt.width))
			if err != nil {
				t.Fatalf("NextID: %v", err)
			}
			if got != tt.want {
				t.Errorf("NextID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKebab(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "words become hyphen separated and lowercase",
			title: "Adopt content addressed cache keys",
			want:  "adopt-content-addressed-cache-keys",
		},
		{
			name:  "punctuation is dropped",
			title: `Adopt the "strangler fig" pattern`,
			want:  "adopt-the-strangler-fig-pattern",
		},
		{
			name:  "runs of separators collapse into one hyphen",
			title: "  Trim   the   spaces  ",
			want:  "trim-the-spaces",
		},
		{
			name:  "underscores and slashes are separators",
			title: "Use HTTP/2 for internal_traffic",
			want:  "use-http-2-for-internal-traffic",
		},
		{
			name:  "an already kebab-cased title is unchanged",
			title: "already-kebab-cased",
			want:  "already-kebab-cased",
		},
		{
			name:  "digits are kept",
			title: "Version 1.0.0 of the public API",
			want:  "version-1-0-0-of-the-public-api",
		},
		{
			name:  "letters outside ASCII survive lowercased",
			title: "Décider d'utiliser une file",
			want:  "décider-d-utiliser-une-file",
		},
		{
			name:  "a title without letters or digits has no slug",
			title: "!!!",
			want:  "",
		},
		{
			name:  "an empty title has no slug",
			title: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Kebab(tt.title); got != tt.want {
				t.Errorf("Kebab(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestFilename(t *testing.T) {
	tests := []struct {
		name     string
		template string
		id       model.ID
		title    string
		want     string
	}{
		{
			name:  "the identifier prefixes the slug",
			id:    "0007",
			title: "Adopt content addressed cache keys",
			want:  "0007-adopt-content-addressed-cache-keys.md",
		},
		{
			name:  "the identifier keeps the width it was given",
			id:    "000007",
			title: "Adopt content addressed cache keys",
			want:  "000007-adopt-content-addressed-cache-keys.md",
		},
		{
			name:  "a title without a slug leaves the identifier alone",
			id:    "0007",
			title: "!!!",
			want:  "0007.md",
		},
		{
			name:     "a bare numeric template drops the slug",
			template: "{id}.md",
			id:       "000007",
			title:    "Adopt content addressed cache keys",
			want:     "000007.md",
		},
		{
			name:     "an underscore separator is honoured",
			template: "{id}_{slug}.md",
			id:       "0007",
			title:    "Adopt content addressed cache keys",
			want:     "0007_adopt-content-addressed-cache-keys.md",
		},
		{
			name:     "an empty slug takes its separator with it",
			template: "{id}_{slug}.md",
			id:       "0007",
			title:    "!!!",
			want:     "0007.md",
		},
		{
			name:     "the slug may lead",
			template: "{slug}-{id}.md",
			id:       "0007",
			title:    "Adopt caches",
			want:     "adopt-caches-0007.md",
		},
		{
			name:     "an empty leading slug takes its separator with it",
			template: "{slug}-{id}.md",
			id:       "0007",
			title:    "!!!",
			want:     "0007.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ADRPreset()
			cfg.Filename = tt.template
			if got := Filename(cfg, tt.id, tt.title); got != tt.want {
				t.Errorf("Filename(%q, %q) = %q, want %q", tt.id, tt.title, got, tt.want)
			}
		})
	}
}

func TestLoadTemplate(t *testing.T) {
	t.Run("an unconfigured template falls back to the built-in one", func(t *testing.T) {
		got, err := LoadTemplate(config.ADRPreset())
		if err != nil {
			t.Fatalf("LoadTemplate: %v", err)
		}
		if got != DefaultTemplate {
			t.Errorf("LoadTemplate = %q, want the built-in template", got)
		}
	})

	t.Run("a configured template is read from disk", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "template.md")
		const custom = "---\ntitle: {{ .Title }}\n---\n"
		if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
			t.Fatalf("write template: %v", err)
		}
		cfg := config.ADRPreset()
		cfg.Template = path

		got, err := LoadTemplate(cfg)
		if err != nil {
			t.Fatalf("LoadTemplate: %v", err)
		}
		if got != custom {
			t.Errorf("LoadTemplate = %q, want %q", got, custom)
		}
	})

	t.Run("a missing template file is an error", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Template = filepath.Join(t.TempDir(), "absent.md")
		if _, err := LoadTemplate(cfg); err == nil {
			t.Error("LoadTemplate of a missing file returned no error")
		}
	})
}

func TestEdgeBlock(t *testing.T) {
	reversed := config.ADRPreset()
	reversed.Edges = []config.EdgeSpec{
		testEdgeSpec(config.EdgeDependsOn.String(), config.EdgeDependsOn.String()),
		testEdgeSpec(config.EdgeSupersedes.String(), config.EdgeSupersedes.String()),
	}

	renamed := config.ADRPreset()
	renamed.Edges = []config.EdgeSpec{testEdgeSpec(config.EdgeSupersedes.String(), "replaces")}

	tests := []struct {
		name    string
		cfg     config.Config
		req     Request
		want    string
		wantErr bool
	}{
		{
			name: "a request without edges emits nothing",
			cfg:  config.ADRPreset(),
			req:  Request{Title: "Adopt content addressed cache keys"},
			want: "",
		},
		{
			name: "a supersedes reference becomes a quoted list entry",
			cfg:  config.ADRPreset(),
			req:  Request{Supersedes: []string{"0004"}},
			want: "supersedes:\n  - \"0004\"\n",
		},
		{
			name: "both edge keys are emitted in declaration order",
			cfg:  config.ADRPreset(),
			req:  Request{Supersedes: []string{"0004"}, DependsOn: []string{"0003"}},
			want: "supersedes:\n  - \"0004\"\ndepends-on:\n  - \"0003\"\n",
		},
		{
			name: "declaration order decides, not the request",
			cfg:  reversed,
			req:  Request{Supersedes: []string{"0004"}, DependsOn: []string{"0003"}},
			want: "depends-on:\n  - \"0003\"\nsupersedes:\n  - \"0004\"\n",
		},
		{
			name: "several references keep the order they were requested in",
			cfg:  config.ADRPreset(),
			req:  Request{Supersedes: []string{"0002", "0001"}},
			want: "supersedes:\n  - \"0002\"\n  - \"0001\"\n",
		},
		{
			name: "references are normalized to the configured width",
			cfg:  config.ADRPreset(),
			req:  Request{Supersedes: []string{"4", "ADR-2"}},
			want: "supersedes:\n  - \"0004\"\n  - \"0002\"\n",
		},
		{
			name: "the configured frontmatter key is used",
			cfg:  renamed,
			req:  Request{Supersedes: []string{"0004"}},
			want: "replaces:\n  - \"0004\"\n",
		},
		{
			name:    "a reference that does not normalize is an error",
			cfg:     config.ADRPreset(),
			req:     Request{Supersedes: []string{"not-a-document"}},
			wantErr: true,
		},
		{
			name:    "an edge the configuration does not declare is an error",
			cfg:     renamed,
			req:     Request{DependsOn: []string{"0003"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EdgeBlock(tt.cfg, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("EdgeBlock = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("EdgeBlock: %v", err)
			}
			if got != tt.want {
				t.Errorf("EdgeBlock = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	const plain = `---
title: Adopt content addressed cache keys
status: proposed
date: 2026-01-05
---

# Adopt content addressed cache keys

## Context and Problem Statement

## Decision Drivers

## Considered Options

## Decision Outcome
`

	const withEdges = `---
title: Adopt content addressed cache keys
status: proposed
date: 2026-01-05
supersedes:
  - "0004"
depends-on:
  - "0003"
---

# Adopt content addressed cache keys

## Context and Problem Statement

## Decision Drivers

## Considered Options

## Decision Outcome
`

	data := TemplateData{
		ID:     "0007",
		Title:  "Adopt content addressed cache keys",
		Status: config.StatusProposed,
		Date:   "2026-01-05",
	}

	tests := []struct {
		name     string
		template string
		data     TemplateData
		want     string
		wantErr  bool
	}{
		{
			name:     "the built-in template carries the frontmatter and the MADR sections",
			template: DefaultTemplate,
			data:     data,
			want:     plain,
		},
		{
			name:     "the edge block sits inside the frontmatter",
			template: DefaultTemplate,
			data: TemplateData{
				ID:        data.ID,
				Title:     data.Title,
				Status:    data.Status,
				Date:      data.Date,
				EdgeBlock: "supersedes:\n  - \"0004\"\ndepends-on:\n  - \"0003\"\n",
			},
			want: withEdges,
		},
		{
			name:     "a custom template sees every field",
			template: "{{ .ID }} {{ .Status }} {{ .Date }} {{ .Title }}\n",
			data:     data,
			want:     "0007 proposed 2026-01-05 Adopt content addressed cache keys\n",
		},
		{
			name:     "an unparsable template is an error",
			template: "{{ .Title",
			data:     data,
			wantErr:  true,
		},
		{
			name:     "a template referring to an unknown field is an error",
			template: "{{ .Nope }}",
			data:     data,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.template, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Render = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Render =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// testKindConfig is the spec preset with its kind directories left as the
// preset writes them, which is what a plan joins a file name onto.
func testKindConfig() config.Config { return config.SpecPreset() }

func testKindGraph(kinds map[string]string) *model.Graph {
	g := &model.Graph{Nodes: make(map[model.ID]*model.Node, len(kinds))}
	for id, kind := range kinds {
		g.Nodes[model.ID(id)] = &model.Node{
			ID:    model.ID(id),
			Kind:  kind,
			Path:  id + ".md",
			Title: "Document " + id,
		}
	}
	return g
}

func TestNextKindID(t *testing.T) {
	// A digit-run kind beside kinds whose identifiers are not numbers at all:
	// only its own documents can be counted, and the others must not make the
	// count fail.
	cfg := config.ADRPreset()
	cfg.IDWidth = 4
	cfg.Kinds = map[string]config.KindSpec{
		"note":   {Dir: "notes"},
		"clause": {Dir: "clauses", ID: config.IDClause},
	}
	g := testKindGraph(map[string]string{
		"0001": "note", "0007": "note", "UZ-V-001": "clause", "UZ-V-009": "clause",
	})

	got, err := NextKindID(g, cfg, "note")
	if err != nil {
		t.Fatalf("NextKindID: %v", err)
	}
	if got != "0008" {
		t.Errorf("NextKindID = %q, want 0008, the one after the highest note", got)
	}
}

func TestKindFilename(t *testing.T) {
	cfg := testKindConfig()
	tests := []struct {
		name  string
		kind  string
		id    model.ID
		title string
		want  string
	}{
		{
			name:  "a file-name shaped identifier names the file, slug and all left out",
			kind:  config.KindClause,
			id:    "UZ-V-007",
			title: "Runs are recorded with their seed",
			want:  "UZ-V-007.md",
		},
		{
			name:  "an identifier carrying a slash is named by its last segment",
			kind:  config.KindConform,
			id:    "conform/uz-v-007",
			title: "Check that runs record their seed",
			want:  "uz-v-007.md",
		},
		{
			name:  "a generated measurement keeps the day in its name",
			kind:  config.KindMeasure,
			id:    "interp/UZ-V-001@2026-08-01",
			title: "Agreement on the evidence check",
			want:  "UZ-V-001@2026-08-01.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindFilename(cfg, tt.kind, tt.id, tt.title); got != tt.want {
				t.Errorf("KindFilename(%q, %q) = %q, want %q", tt.kind, tt.id, got, tt.want)
			}
		})
	}

	t.Run("a kind that declares no pattern keeps the filename template", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Kinds = map[string]config.KindSpec{"note": {Dir: "notes"}}

		if got := KindFilename(cfg, "note", "0007", "Adopt caches"); got != "0007-adopt-caches.md" {
			t.Errorf("KindFilename = %q, want the template's answer", got)
		}
	})

	t.Run("a corpus without kinds is named as it always was", func(t *testing.T) {
		if got := KindFilename(config.ADRPreset(), "", "0007", "Adopt caches"); got != "0007-adopt-caches.md" {
			t.Errorf("KindFilename = %q, want the template's answer", got)
		}
	})
}

func TestEdgeBlockForAKind(t *testing.T) {
	cfg := testKindConfig()

	t.Run("a clause is offered the edges a clause may declare", func(t *testing.T) {
		got, err := EdgeBlock(cfg, Request{Kind: config.KindClause})
		if err != nil {
			t.Fatalf("EdgeBlock: %v", err)
		}
		want := "# supersedes:\n" +
			"#   - {ref: <clause|premise>, reason: <recurrence|premise-collapse|conflict|vocabulary>}\n" +
			"# premise:\n#   - <premise>\n" +
			"# rationale:\n#   - <principle>\n" +
			"# counterexample:\n#   - <pm>\n" +
			"# about:\n#   - <topic>\n" +
			"# excepts:\n#   - {ref: <clause>, scope: <string>}\n" +
			"# interop:\n#   - <clause>\n"
		if got != want {
			t.Errorf("EdgeBlock =\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("a conformance test is offered the one edge it declares", func(t *testing.T) {
		got, err := EdgeBlock(cfg, Request{Kind: config.KindConform})
		if err != nil {
			t.Fatalf("EdgeBlock: %v", err)
		}
		if got != "# enforces:\n#   - <clause>\n" {
			t.Errorf("EdgeBlock = %q, want the enforces stub alone", got)
		}
	})

	t.Run("an edge that requires an attribute is not written for the caller", func(t *testing.T) {
		// The entry would be incomplete, and a creation has no value to put
		// there: refusing beats handing back a document that fails validation.
		_, err := EdgeBlock(cfg, Request{Kind: config.KindClause, Supersedes: []string{"UZ-V-001"}})

		if !errors.Is(err, model.ErrInvalidConfig) {
			t.Fatalf("err = %v, want it to wrap model.ErrInvalidConfig", err)
		}
		if !strings.Contains(err.Error(), config.AttrReason) {
			t.Errorf("err = %v, want it to name the required attribute", err)
		}
	})

	t.Run("a corpus without kinds is offered no stubs", func(t *testing.T) {
		got, err := EdgeBlock(config.ADRPreset(), Request{Title: "Adopt caches"})
		if err != nil {
			t.Fatalf("EdgeBlock: %v", err)
		}
		if got != "" {
			t.Errorf("EdgeBlock = %q, want nothing: a single-kind corpus writes what it asked for", got)
		}
	})
}

func TestNewPlanForAKind(t *testing.T) {
	cfg := testKindConfig()
	g := testKindGraph(map[string]string{"UZ-V-001": config.KindClause})
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	t.Run("a clause is written into its own directory, named after its identifier", func(t *testing.T) {
		plan, err := NewPlan(g, cfg, Request{
			Kind: config.KindClause, ID: "UZ-V-007", Title: "Runs are recorded with their seed", Date: date,
		})
		if err != nil {
			t.Fatalf("NewPlan: %v", err)
		}
		if want := filepath.Join("spec", "clauses", "UZ-V-007.md"); plan.Path != want {
			t.Errorf("path = %q, want %q", plan.Path, want)
		}
		want := "---\n" +
			"id: UZ-V-007\n" +
			"kind: clause\n" +
			"title: Runs are recorded with their seed\n" +
			"status: proposed\n" +
			"date: 2026-09-01\n" +
			// The vocabulary a clause has to state comes first, as a stub for
			// the same reason the edges are stubs: a placeholder written as a
			// value would be a finding of its own.
			"# modality: <MUST|MUST_NOT|SHOULD|SHOULD_NOT|MAY>\n" +
			"# supersedes:\n" +
			"#   - {ref: <clause|premise>, reason: <recurrence|premise-collapse|conflict|vocabulary>}\n" +
			"# premise:\n#   - <premise>\n" +
			"# rationale:\n#   - <principle>\n" +
			"# counterexample:\n#   - <pm>\n" +
			"# about:\n#   - <topic>\n" +
			"# excepts:\n#   - {ref: <clause>, scope: <string>}\n" +
			"# interop:\n#   - <clause>\n" +
			"---\n\n# Runs are recorded with their seed\n"
		if string(plan.Content) != want {
			t.Errorf("content =\n%s\nwant:\n%s", plan.Content, want)
		}
	})

	t.Run("a kind that answers to no status vocabulary is written without one", func(t *testing.T) {
		plan, err := NewPlan(g, cfg, Request{
			Kind: config.KindConform, ID: "conform/uz-v-007", Title: "Check the seed", Date: date,
		})
		if err != nil {
			t.Fatalf("NewPlan: %v", err)
		}
		if want := filepath.Join("spec", "conform", "uz-v-007.md"); plan.Path != want {
			t.Errorf("path = %q, want %q", plan.Path, want)
		}
		if strings.Contains(string(plan.Content), cfg.EffectiveStatus()+":") {
			t.Errorf("content =\n%s\nwant no status key: the kind answers to no vocabulary", plan.Content)
		}
		if !strings.Contains(string(plan.Content), "id: conform/uz-v-007\n") {
			t.Errorf("content =\n%s\nwant the identifier, which no file name of this kind can carry", plan.Content)
		}
	})

	t.Run("a declared pattern has no next identifier to take", func(t *testing.T) {
		_, err := NewPlan(g, cfg, Request{Kind: config.KindClause, Title: "Runs are recorded with their seed"})

		if !errors.Is(err, model.ErrUnknownID) {
			t.Fatalf("err = %v, want it to wrap model.ErrUnknownID", err)
		}
	})

	t.Run("an identifier the kind's pattern rejects is refused", func(t *testing.T) {
		_, err := NewPlan(g, cfg, Request{Kind: config.KindClause, ID: "0007", Title: "Runs are recorded"})

		if !errors.Is(err, model.ErrUnknownID) {
			t.Fatalf("err = %v, want it to wrap model.ErrUnknownID", err)
		}
	})

	t.Run("a digit-run kind counts up from its own documents", func(t *testing.T) {
		counted := config.ADRPreset()
		counted.Kinds = map[string]config.KindSpec{"note": {Dir: "notes"}}
		notes := testKindGraph(map[string]string{"0003": "note"})

		plan, err := NewPlan(notes, counted, Request{Kind: "note", Title: "A note", Date: date})
		if err != nil {
			t.Fatalf("NewPlan: %v", err)
		}
		if plan.ID != "0004" {
			t.Errorf("id = %q, want 0004", plan.ID)
		}
		if want := filepath.Join("notes", "0004-a-note.md"); plan.Path != want {
			t.Errorf("path = %q, want %q", plan.Path, want)
		}
		// A kind with no pattern writes no identifier: the file name carries it,
		// exactly as it does on a corpus that declares no kinds at all.
		if strings.Contains(string(plan.Content), "id:") {
			t.Errorf("content =\n%s\nwant no id key", plan.Content)
		}
	})

	t.Run("a kind nobody declared is refused", func(t *testing.T) {
		_, err := NewPlan(g, cfg, Request{Kind: "bogus", ID: "UZ-V-007", Title: "Runs"})

		if !errors.Is(err, model.ErrUnknownID) {
			t.Fatalf("err = %v, want it to wrap model.ErrUnknownID", err)
		}
	})
}
