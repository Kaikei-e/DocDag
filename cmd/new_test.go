package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/internal/newdoc"
)

func splitDocument(t *testing.T, src []byte) (map[string]any, string) {
	t.Helper()
	const opener = "---\n"
	text := string(src)
	if !strings.HasPrefix(text, opener) {
		t.Fatalf("document does not open with a frontmatter delimiter: %q", text)
	}
	rest := text[len(opener):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		t.Fatalf("document has no closing frontmatter delimiter: %q", text)
	}
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end+1]), &frontmatter); err != nil {
		t.Fatalf("decode frontmatter %q: %v", rest[:end+1], err)
	}
	return frontmatter, rest[end+len("\n---\n"):]
}

// refDigits reduces a frontmatter reference list to bare digits, so the
// assertion holds whether the value was written quoted or bare.
func refDigits(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		digits := strings.TrimLeft(fmt.Sprint(item), "0")
		if digits == "" {
			digits = "0"
		}
		out = append(out, digits)
	}
	return out
}

func TestNewCreatesTheNextDocument(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	title := "Adopt content addressed cache keys"

	got := run(t, "new", title, "--supersedes", "0004", "--depends-on", "0003", "--dir", dir)
	assertExit(t, got, 0)

	wantPath := filepath.Join(dir, "0007-adopt-content-addressed-cache-keys.md")
	assertLines(t, "created path", lines(got.stdout), []string{wantPath})

	created, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	frontmatter, body := splitDocument(t, created)

	if frontmatter["title"] != title {
		t.Errorf("title = %v, want %q", frontmatter["title"], title)
	}
	if frontmatter["status"] != "proposed" {
		t.Errorf("status = %v, want %q", frontmatter["status"], "proposed")
	}
	date := fmt.Sprint(frontmatter["date"])
	if _, err := time.Parse(newdoc.DateLayout, date); err != nil {
		t.Errorf("date = %q, want a %s date: %v", date, newdoc.DateLayout, err)
	}
	if refs := refDigits(t, frontmatter["supersedes"]); !slices.Equal(refs, []string{"4"}) {
		t.Errorf("supersedes = %v, want the reference to document 0004", frontmatter["supersedes"])
	}
	if refs := refDigits(t, frontmatter["depends-on"]); !slices.Equal(refs, []string{"3"}) {
		t.Errorf("depends-on = %v, want the reference to document 0003", frontmatter["depends-on"])
	}
	for _, heading := range []string{
		"# " + title,
		"## Context and Problem Statement",
		"## Decision Drivers",
		"## Considered Options",
		"## Decision Outcome",
	} {
		if !strings.Contains(body, heading) {
			t.Errorf("body is missing %q:\n%s", heading, body)
		}
	}
}

func TestNewRewritesOnlyTheStatusOfTheSupersededDocument(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	superseded := filepath.Join(dir, "000004.md")
	untouched := filepath.Join(dir, "000003.md")

	before, err := os.ReadFile(superseded)
	if err != nil {
		t.Fatalf("read %s: %v", superseded, err)
	}
	beforeUntouched, err := os.ReadFile(untouched)
	if err != nil {
		t.Fatalf("read %s: %v", untouched, err)
	}

	got := run(t, "new", "Adopt content addressed cache keys", "--supersedes", "0004", "--dir", dir)
	assertExit(t, got, 0)

	after, err := os.ReadFile(superseded)
	if err != nil {
		t.Fatalf("read %s: %v", superseded, err)
	}
	want := bytes.Replace(before, []byte("status: accepted\n"), []byte("status: superseded\n"), 1)
	if !bytes.Equal(after, want) {
		t.Errorf("superseded document changed beyond its status value:\ngot:\n%s\nwant:\n%s", after, want)
	}

	afterUntouched, err := os.ReadFile(untouched)
	if err != nil {
		t.Fatalf("read %s: %v", untouched, err)
	}
	if !bytes.Equal(beforeUntouched, afterUntouched) {
		t.Errorf("a document that is not superseded was rewritten:\ngot:\n%s\nwant:\n%s", afterUntouched, beforeUntouched)
	}
}

func TestNewWithoutEdgesOmitsTheEdgeKeys(t *testing.T) {
	dir := copyFixture(t, "fan-in")

	got := run(t, "new", "Rotate signing keys weekly", "--dir", dir)
	assertExit(t, got, 0)

	wantPath := filepath.Join(dir, "0004-rotate-signing-keys-weekly.md")
	assertLines(t, "created path", lines(got.stdout), []string{wantPath})

	created, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	frontmatter, _ := splitDocument(t, created)
	for _, key := range []string{"supersedes", "depends-on"} {
		if _, ok := frontmatter[key]; ok {
			t.Errorf("frontmatter carries %q with no edge requested: %+v", key, frontmatter)
		}
	}
}

func TestNewLeavesTheCorpusValid(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	created := run(t, "new", "Adopt content addressed cache keys", "--supersedes", "0004", "--depends-on", "0003", "--dir", dir)
	assertExit(t, created, 0)

	validated := run(t, "validate", "--dir", dir)
	assertExit(t, validated, 0)
	assertPrefixes(t, "findings", findingLines(validated.stdout), nil)
	want := "OK: 7 docs, 6 typed edges, no cycles"
	if ls := lines(validated.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary line = %q, want %q", validated.stdout, want)
	}
}

func TestNewJSONReportsTheCreatedPath(t *testing.T) {
	dir := copyFixture(t, "fan-in")

	got := run(t, "new", "Rotate signing keys weekly", "--format", "json", "--dir", dir)

	assertExit(t, got, 0)
	payload := decodeJSON[map[string]string](t, got.stdout)
	want := filepath.Join(dir, "0004-rotate-signing-keys-weekly.md")
	if payload["path"] != want {
		t.Errorf("payload = %v, want the created path %q", payload, want)
	}
}

func TestNewRejectsAnUnknownFormat(t *testing.T) {
	dir := copyFixture(t, "fan-in")

	got := run(t, "new", "Rotate signing keys weekly", "--format", "yaml", "--dir", dir)

	assertExit(t, got, 2)
	if _, err := os.Stat(filepath.Join(dir, "0004-rotate-signing-keys-weekly.md")); !os.IsNotExist(err) {
		t.Error("new created a document despite a usage error")
	}
}

func TestNewResolvesToTheCreatedDocument(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	created := run(t, "new", "Adopt content addressed cache keys", "--supersedes", "0004", "--dir", dir)
	assertExit(t, created, 0)

	resolved := run(t, "resolve", "0001", "--dir", dir)
	assertExit(t, resolved, 0)
	assertLines(t, "resolve", lines(resolved.stdout), []string{"0007"})
}
