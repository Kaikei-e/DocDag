package newdoc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

const trickyBefore = `---
title: Serve images from the application server
status: accepted
date: 2025-02-01
---

# Serve images from the application server

## Context and Problem Statement

Documents in this corpus open with a frontmatter block that looks like this:

---
status: accepted
---

The block above is prose, not frontmatter, and so is the inline phrase
status: accepted in this very sentence.
`

const trickyAfter = `---
title: Serve images from the application server
status: superseded
date: 2025-02-01
---

# Serve images from the application server

## Context and Problem Statement

Documents in this corpus open with a frontmatter block that looks like this:

---
status: accepted
---

The block above is prose, not frontmatter, and so is the inline phrase
status: accepted in this very sentence.
`

func TestRewriteStatus(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		field   string
		status  string
		want    string
		wantErr bool
	}{
		{
			name:   "only the status value changes",
			src:    "---\ntitle: Serve images from a CDN\nstatus: accepted\ndate: 2025-02-01\n---\n\nThe body stays as it is.\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\ntitle: Serve images from a CDN\nstatus: superseded\ndate: 2025-02-01\n---\n\nThe body stays as it is.\n",
		},
		{
			name:   "the body keeps its own delimiters and status strings",
			src:    trickyBefore,
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   trickyAfter,
		},
		{
			name:   "extra spacing around the value is normalised",
			src:    "---\ntitle: Serve images from a CDN\nstatus:    accepted\n---\n\nBody.\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\ntitle: Serve images from a CDN\nstatus: superseded\n---\n\nBody.\n",
		},
		{
			name:   "a MADR status sentence is replaced as a whole",
			src:    "---\ntitle: Serve images from a CDN\nstatus: superseded by 0003\n---\n\nBody.\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\ntitle: Serve images from a CDN\nstatus: superseded\n---\n\nBody.\n",
		},
		{
			name:   "the configured field is the one that is rewritten",
			src:    "---\ntitle: Serve images from a CDN\nstate: accepted\nstatus: accepted\n---\n\nBody.\n",
			field:  "state",
			status: config.StatusSuperseded,
			want:   "---\ntitle: Serve images from a CDN\nstate: superseded\nstatus: accepted\n---\n\nBody.\n",
		},
		{
			name:   "an absent field is appended to the frontmatter",
			src:    "---\ntitle: Serve images from a CDN\ndate: 2025-02-01\n---\n\nBody.\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\ntitle: Serve images from a CDN\ndate: 2025-02-01\nstatus: superseded\n---\n\nBody.\n",
		},
		{
			name:   "rewriting to the value already there changes nothing",
			src:    "---\ntitle: Serve images from a CDN\nstatus: superseded\n---\n\nBody.\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\ntitle: Serve images from a CDN\nstatus: superseded\n---\n\nBody.\n",
		},
		{
			name:    "a document without frontmatter is refused",
			src:     "# Serve images from a CDN\n\nstatus: accepted\n",
			field:   config.DefaultStatusField,
			status:  config.StatusSuperseded,
			wantErr: true,
		},
		{
			name:    "an unterminated frontmatter block is refused",
			src:     "---\ntitle: Serve images from a CDN\nstatus: accepted\n\nBody.\n",
			field:   config.DefaultStatusField,
			status:  config.StatusSuperseded,
			wantErr: true,
		},
		{
			name:   "an empty frontmatter block takes the status the parser accepts",
			src:    "---\n---\nBody only.\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\nstatus: superseded\n---\nBody only.\n",
		},
		{
			name:   "windows line endings survive the rewrite",
			src:    "---\r\ntitle: Serve images from a CDN\r\nstatus: accepted\r\n---\r\n\r\nBody.\r\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\r\ntitle: Serve images from a CDN\r\nstatus: superseded\r\n---\r\n\r\nBody.\r\n",
		},
		{
			name:   "an absent field on a windows document is appended with its line ending",
			src:    "---\r\ntitle: Serve images from a CDN\r\n---\r\n\r\nBody.\r\n",
			field:  config.DefaultStatusField,
			status: config.StatusSuperseded,
			want:   "---\r\ntitle: Serve images from a CDN\r\nstatus: superseded\r\n---\r\n\r\nBody.\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RewriteStatus([]byte(tt.src), tt.field, tt.status)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RewriteStatus = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RewriteStatus: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("RewriteStatus =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestRewriteStatusDoesNotMutateItsInput(t *testing.T) {
	src := []byte(trickyBefore)
	before := bytes.Clone(src)

	if _, err := RewriteStatus(src, config.DefaultStatusField, config.StatusSuperseded); err != nil {
		t.Fatalf("RewriteStatus: %v", err)
	}
	if !bytes.Equal(src, before) {
		t.Errorf("RewriteStatus modified the source slice:\ngot:\n%s\nwant:\n%s", src, before)
	}
}

func TestRewriteStatusFile(t *testing.T) {
	t.Run("the file is rewritten in place with its mode kept", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "0001-serve-images-from-a-cdn.md")
		if err := os.WriteFile(path, []byte(trickyBefore), 0o644); err != nil {
			t.Fatalf("write document: %v", err)
		}
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatalf("chmod document: %v", err)
		}

		if err := RewriteStatusFile(path, config.DefaultStatusField, config.StatusSuperseded); err != nil {
			t.Fatalf("RewriteStatusFile: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read document: %v", err)
		}
		if string(got) != trickyAfter {
			t.Errorf("document =\n%s\nwant:\n%s", got, trickyAfter)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat document: %v", err)
		}
		// Windows only models a read-only bit, not POSIX permission sets.
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
			t.Errorf("mode = %v, want %v", info.Mode().Perm(), os.FileMode(0o640))
		}
	})

	t.Run("a missing file is an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent.md")
		if err := RewriteStatusFile(path, config.DefaultStatusField, config.StatusSuperseded); err == nil {
			t.Error("RewriteStatusFile of a missing file returned no error")
		}
	})
}

const corpusFirst = `---
title: Authenticate browsers with session cookies
status: accepted
date: 2025-01-13
---

# Authenticate browsers with session cookies

## Decision Outcome

A signed, http-only session cookie is issued at sign-in. A document of this
corpus starts with a block such as:

---
status: accepted
---

which is prose and must survive any rewrite.
`

const corpusSecond = `---
title: Authenticate integrations with API keys
status: accepted
date: 2025-01-28
---

# Authenticate integrations with API keys

## Decision Outcome

Each integration gets a long-lived API key sent in an authorization header.
`

// testCorpus lays out a two-document directory and the graph that matches it.
func testCorpus(t *testing.T) (string, *model.Graph, config.Config) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"0001-authenticate-with-session-cookies.md": corpusFirst,
		"0002-authenticate-with-api-keys.md":        corpusSecond,
	}
	g := &model.Graph{Nodes: map[model.ID]*model.Node{}}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		id := model.ID(name[:4])
		g.Nodes[id] = &model.Node{ID: id, Path: path, Status: config.StatusAccepted}
	}
	cfg := config.ADRPreset()
	cfg.Dir = dir
	return dir, g, cfg
}

// testCreate plans a document and applies the plan, which is the sequence the
// CLI runs.
func testCreate(g *model.Graph, cfg config.Config, req Request) (string, error) {
	plan, err := NewPlan(g, cfg, req)
	if err != nil {
		return "", err
	}
	return plan.Apply()
}

func testFixedDate() time.Time {
	return time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
}

func TestCreate(t *testing.T) {
	const want = `---
title: Rotate signing keys weekly
status: proposed
date: 2026-01-05
---

# Rotate signing keys weekly

## Context and Problem Statement

## Decision Drivers

## Considered Options

## Decision Outcome
`

	dir, g, cfg := testCorpus(t)
	path, err := testCreate(g, cfg, Request{Title: "Rotate signing keys weekly", Date: testFixedDate()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	wantPath := filepath.Join(dir, "0003-rotate-signing-keys-weekly.md")
	if path != wantPath {
		t.Errorf("Create = %q, want %q", path, wantPath)
	}
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	if string(got) != want {
		t.Errorf("created document =\n%s\nwant:\n%s", got, want)
	}
}

func TestCreateWithEdges(t *testing.T) {
	const want = `---
title: Rotate signing keys weekly
status: proposed
date: 2026-01-05
supersedes:
  - "0001"
depends-on:
  - "0002"
---

# Rotate signing keys weekly

## Context and Problem Statement

## Decision Drivers

## Considered Options

## Decision Outcome
`

	dir, g, cfg := testCorpus(t)
	superseded := filepath.Join(dir, "0001-authenticate-with-session-cookies.md")
	untouched := filepath.Join(dir, "0002-authenticate-with-api-keys.md")

	path, err := testCreate(g, cfg, Request{
		Title:      "Rotate signing keys weekly",
		Supersedes: []string{"0001"},
		DependsOn:  []string{"0002"},
		Date:       testFixedDate(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	if string(got) != want {
		t.Errorf("created document =\n%s\nwant:\n%s", got, want)
	}

	rewritten, err := os.ReadFile(superseded)
	if err != nil {
		t.Fatalf("read superseded document: %v", err)
	}
	wantRewritten := strings.Replace(corpusFirst, "status: accepted\ndate: 2025-01-13\n", "status: superseded\ndate: 2025-01-13\n", 1)
	if string(rewritten) != wantRewritten {
		t.Errorf("superseded document changed beyond its status value:\ngot:\n%s\nwant:\n%s", rewritten, wantRewritten)
	}

	kept, err := os.ReadFile(untouched)
	if err != nil {
		t.Fatalf("read dependency document: %v", err)
	}
	if string(kept) != corpusSecond {
		t.Errorf("a document that is only depended on was rewritten:\ngot:\n%s\nwant:\n%s", kept, corpusSecond)
	}
}

func TestCreateUnknownSupersedesReference(t *testing.T) {
	dir, g, cfg := testCorpus(t)

	path, err := testCreate(g, cfg, Request{
		Title:      "Rotate signing keys weekly",
		Supersedes: []string{"0009"},
		Date:       testFixedDate(),
	})
	if !errors.Is(err, model.ErrUnknownID) {
		t.Fatalf("Create = (%q, %v), want an error wrapping %v", path, err, model.ErrUnknownID)
	}
	if _, err := os.Stat(filepath.Join(dir, "0003-rotate-signing-keys-weekly.md")); !errors.Is(err, os.ErrNotExist) {
		t.Error("Create wrote a document even though a reference was unknown")
	}
}

func TestCreateRefusesToOverwrite(t *testing.T) {
	_, g, cfg := testCorpus(t)
	req := Request{Title: "Rotate signing keys weekly", Date: testFixedDate()}

	path, err := testCreate(g, cfg, req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}

	if _, err := testCreate(g, cfg, req); err == nil {
		t.Error("Create overwrote an existing document without an error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the existing document was rewritten:\ngot:\n%s\nwant:\n%s", after, before)
	}
}

func TestCreateWithoutADateUsesToday(t *testing.T) {
	_, g, cfg := testCorpus(t)

	path, err := testCreate(g, cfg, Request{Title: "Rotate signing keys weekly"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}

	var date string
	for _, line := range strings.Split(string(got), "\n") {
		if value, ok := strings.CutPrefix(line, "date: "); ok {
			date = value
			break
		}
	}
	if _, err := time.Parse(DateLayout, date); err != nil {
		t.Errorf("date = %q, want a %s date: %v", date, DateLayout, err)
	}
}

func TestCreateLeavesNothingHalfApplied(t *testing.T) {
	dir, g, cfg := testCorpus(t)
	first := filepath.Join(dir, "0001-authenticate-with-session-cookies.md")
	broken := filepath.Join(dir, "0002-authenticate-with-api-keys.md")
	if err := os.WriteFile(broken, []byte("---\ntitle: Authenticate integrations with API keys\nstatus: accepted\n\nBody.\n"), 0o644); err != nil {
		t.Fatalf("write malformed document: %v", err)
	}

	path, err := testCreate(g, cfg, Request{
		Title:      "Rotate signing keys weekly",
		Supersedes: []string{"0001", "0002"},
		Date:       testFixedDate(),
	})

	if err == nil {
		t.Fatalf("Create = %q, want an error: one of the superseded documents cannot be rewritten", path)
	}
	if _, err := os.Stat(filepath.Join(dir, "0003-rotate-signing-keys-weekly.md")); !errors.Is(err, os.ErrNotExist) {
		t.Error("Create wrote the new document even though a rewrite failed")
	}
	kept, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first document: %v", err)
	}
	if string(kept) != corpusFirst {
		t.Errorf("a document was rewritten before the failure:\ngot:\n%s\nwant:\n%s", kept, corpusFirst)
	}
}

func TestNewPlanComputesEverythingWithoutWriting(t *testing.T) {
	dir, g, cfg := testCorpus(t)
	superseded := filepath.Join(dir, "0001-authenticate-with-session-cookies.md")

	plan, err := NewPlan(g, cfg, Request{
		Title:      "Rotate signing keys weekly",
		Supersedes: []string{"0001"},
		Date:       testFixedDate(),
	})
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}

	if plan.ID != "0003" {
		t.Errorf("ID = %q, want %q", plan.ID, "0003")
	}
	if want := filepath.Join(dir, "0003-rotate-signing-keys-weekly.md"); plan.Path != want {
		t.Errorf("Path = %q, want %q", plan.Path, want)
	}
	if len(plan.Rewrites) != 1 {
		t.Fatalf("Rewrites = %+v, want one entry", plan.Rewrites)
	}
	if plan.Rewrites[0].Path != superseded {
		t.Errorf("rewrite path = %q, want %q", plan.Rewrites[0].Path, superseded)
	}
	if plan.Rewrites[0].Status != config.StatusSuperseded {
		t.Errorf("rewrite status = %q, want %q", plan.Rewrites[0].Status, config.StatusSuperseded)
	}
	if !strings.Contains(string(plan.Content), "title: Rotate signing keys weekly") {
		t.Errorf("Content =\n%s\nwant the rendered document", plan.Content)
	}

	if _, err := os.Stat(plan.Path); !errors.Is(err, os.ErrNotExist) {
		t.Error("NewPlan wrote the document it only planned")
	}
	kept, err := os.ReadFile(superseded)
	if err != nil {
		t.Fatalf("read superseded document: %v", err)
	}
	if string(kept) != corpusFirst {
		t.Errorf("NewPlan rewrote a document:\ngot:\n%s\nwant:\n%s", kept, corpusFirst)
	}
}

func TestPlanApplyWritesWhatWasPlanned(t *testing.T) {
	dir, g, cfg := testCorpus(t)

	plan, err := NewPlan(g, cfg, Request{
		Title:      "Rotate signing keys weekly",
		Supersedes: []string{"0001"},
		Date:       testFixedDate(),
	})
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}
	path, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if path != plan.Path {
		t.Errorf("Apply = %q, want the planned path %q", path, plan.Path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	if !bytes.Equal(got, plan.Content) {
		t.Errorf("created document =\n%s\nwant the planned content:\n%s", got, plan.Content)
	}
	rewritten, err := os.ReadFile(filepath.Join(dir, "0001-authenticate-with-session-cookies.md"))
	if err != nil {
		t.Fatalf("read superseded document: %v", err)
	}
	if !strings.Contains(string(rewritten), "status: superseded") {
		t.Errorf("superseded document was not rewritten:\n%s", rewritten)
	}
}

func TestNewPlanUsesTheRequestedIdentifier(t *testing.T) {
	dir, g, cfg := testCorpus(t)

	plan, err := NewPlan(g, cfg, Request{ID: "42", Title: "Rotate signing keys weekly", Date: testFixedDate()})
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}

	if plan.ID != "0042" {
		t.Errorf("ID = %q, want the requested identifier %q", plan.ID, "0042")
	}
	if want := filepath.Join(dir, "0042-rotate-signing-keys-weekly.md"); plan.Path != want {
		t.Errorf("Path = %q, want %q", plan.Path, want)
	}
	if plan.Exists {
		t.Error("Exists is true for an identifier the corpus does not hold")
	}
}

func TestNewPlanForAnExistingDocumentWithTheSameTitleWritesNothing(t *testing.T) {
	_, g, cfg := testCorpus(t)
	existing := g.Nodes["0001"]
	existing.Title = "Authenticate browsers with session cookies"

	plan, err := NewPlan(g, cfg, Request{ID: "0001", Title: existing.Title, Date: testFixedDate()})
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}

	if !plan.Exists {
		t.Error("Exists is false for a document the corpus already holds")
	}
	if plan.Path != existing.Path {
		t.Errorf("Path = %q, want the existing document %q", plan.Path, existing.Path)
	}
	if plan.Content != nil || len(plan.Rewrites) != 0 {
		t.Errorf("plan = %+v, want nothing to write", plan)
	}
}

func TestNewPlanRefusesADifferentTitleUnderAnExistingIdentifier(t *testing.T) {
	_, g, cfg := testCorpus(t)
	g.Nodes["0001"].Title = "Authenticate browsers with session cookies"

	plan, err := NewPlan(g, cfg, Request{ID: "0001", Title: "Rotate signing keys weekly", Date: testFixedDate()})

	if !errors.Is(err, model.ErrIDConflict) {
		t.Fatalf("NewPlan = (%+v, %v), want an error wrapping %v", plan, err, model.ErrIDConflict)
	}
	if !strings.Contains(err.Error(), "Authenticate browsers with session cookies") {
		t.Errorf("error = %v, want it to name the title already under the identifier", err)
	}
}

func TestNewPlanRefusesACorpusWithAnIdentifierCollision(t *testing.T) {
	_, g, cfg := testCorpus(t)
	g.Findings = []model.Finding{{
		Severity: model.SeverityError,
		Rule:     model.RuleIDCollision,
		ID:       "0001",
		Detail:   "shares its identifier with 0001-again.md",
	}}

	plan, err := NewPlan(g, cfg, Request{Title: "Rotate signing keys weekly", Date: testFixedDate()})

	if !errors.Is(err, model.ErrIDConflict) {
		t.Fatalf("NewPlan = (%+v, %v), want an error wrapping %v", plan, err, model.ErrIDConflict)
	}
	if !strings.Contains(err.Error(), "0001") {
		t.Errorf("error = %v, want it to name the colliding identifier", err)
	}
}

func TestRewriteStatusRefusesADocumentRatherThanAConfiguration(t *testing.T) {
	for _, src := range []string{"# No frontmatter\n", "---\ntitle: Unterminated\n"} {
		err := func() error {
			_, err := RewriteStatus([]byte(src), config.DefaultStatusField, config.StatusSuperseded)
			return err
		}()
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("RewriteStatus(%q) = %v, want it to wrap model.ErrInvalidDocument", src, err)
		}
		if errors.Is(err, model.ErrInvalidConfig) {
			t.Errorf("RewriteStatus(%q) = %v, want a malformed document not to read as a broken configuration", src, err)
		}
	}
}
