package newdoc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
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
