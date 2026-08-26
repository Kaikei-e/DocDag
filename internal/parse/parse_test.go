package parse

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
)

// testFixture resolves a fixture scenario: tests run with the package directory
// as the working directory, and the fixtures live at the repository root.
func testFixture(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "fixtures", name)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return dir
}

func testFixtureFile(t *testing.T, scenario, name string) string {
	t.Helper()
	path := filepath.Join(testFixture(t, scenario), name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture file %s: %v", path, err)
	}
	return path
}

func testWriteDocs(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func testDoc(fm map[string]any) *Document {
	return &Document{
		Path:           "0002-a-decision.md",
		Name:           "0002-a-decision.md",
		ID:             "0002",
		Frontmatter:    fm,
		HasFrontmatter: fm != nil,
		MatchesPattern: true,
	}
}

func testNames(docs []*Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Name)
	}
	return out
}

func testIDs(docs []*Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.ID.String())
	}
	return out
}

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		frontmatter string
		body        string
		ok          bool
	}{
		{
			name:        "a leading delimited block is the frontmatter",
			src:         "---\ntitle: A decision\nstatus: accepted\n---\n\n# A decision\n",
			frontmatter: "title: A decision\nstatus: accepted\n",
			body:        "\n# A decision\n",
			ok:          true,
		},
		{
			name:        "an empty block is still a block",
			src:         "---\n---\n# A decision\n",
			frontmatter: "",
			body:        "# A decision\n",
			ok:          true,
		},
		{
			name:        "a closing delimiter at end of file leaves an empty body",
			src:         "---\ntitle: A decision\n---",
			frontmatter: "title: A decision\n",
			body:        "",
			ok:          true,
		},
		{
			name:        "the first closing delimiter wins and the rest is body",
			src:         "---\ntitle: A decision\n---\n\nintro\n\n---\n\nmore\n",
			frontmatter: "title: A decision\n",
			body:        "\nintro\n\n---\n\nmore\n",
			ok:          true,
		},
		{
			name: "a file without a leading delimiter is all body",
			src:  "# A decision\n\nProse.\n",
			body: "# A decision\n\nProse.\n",
		},
		{
			name: "a delimiter after the first line does not open a block",
			src:  "\n---\ntitle: A decision\n---\n",
			body: "\n---\ntitle: A decision\n---\n",
		},
		{
			name: "an unterminated block is not frontmatter",
			src:  "---\ntitle: A decision\n\n# A decision\n",
			body: "---\ntitle: A decision\n\n# A decision\n",
		},
		{
			name: "a longer dash run is not a delimiter",
			src:  "----\ntitle: A decision\n----\n",
			body: "----\ntitle: A decision\n----\n",
		},
		{
			name: "an empty file has neither",
			src:  "",
			body: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter, body, ok := SplitFrontmatter([]byte(tt.src))

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if string(frontmatter) != tt.frontmatter {
				t.Errorf("frontmatter = %q, want %q", frontmatter, tt.frontmatter)
			}
			if string(body) != tt.body {
				t.Errorf("body = %q, want %q", body, tt.body)
			}
		})
	}
}

func TestUnmarshalFrontmatter(t *testing.T) {
	t.Run("recognized keys decode as scalars", func(t *testing.T) {
		got, err := UnmarshalFrontmatter([]byte("title: A decision\nstatus: accepted\ndate: 2025-03-08\n"))
		if err != nil {
			t.Fatalf("UnmarshalFrontmatter: %v", err)
		}
		for key, want := range map[string]any{
			"title":  "A decision",
			"status": "accepted",
			"date":   "2025-03-08",
		} {
			if got[key] != want {
				t.Errorf("%s = %#v, want %#v", key, got[key], want)
			}
		}
	})

	t.Run("unknown keys are kept", func(t *testing.T) {
		got, err := UnmarshalFrontmatter([]byte("title: A decision\nowner: platform\n"))
		if err != nil {
			t.Fatalf("UnmarshalFrontmatter: %v", err)
		}
		if got["owner"] != "platform" {
			t.Fatalf("owner = %#v, want platform (other repositories carry extra fields)", got["owner"])
		}
	})

	t.Run("a list value decodes as a list", func(t *testing.T) {
		got, err := UnmarshalFrontmatter([]byte("supersedes:\n  - 0001\n  - ADR-2\n"))
		if err != nil {
			t.Fatalf("UnmarshalFrontmatter: %v", err)
		}
		list, ok := got["supersedes"].([]any)
		if !ok {
			t.Fatalf("supersedes = %#v, want a list", got["supersedes"])
		}
		if len(list) != 2 {
			t.Fatalf("supersedes = %#v, want two entries", list)
		}
	})

	t.Run("an empty block decodes to nothing", func(t *testing.T) {
		got, err := UnmarshalFrontmatter(nil)
		if err != nil {
			t.Fatalf("UnmarshalFrontmatter: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("frontmatter = %#v, want none", got)
		}
	})

	failures := []struct {
		name string
		src  string
	}{
		{name: "a reserved character is a decode failure", src: "title: `X-Api-Version` selects the contract\nstatus: proposed\n"},
		{name: "a tab indent is a decode failure", src: "title: A decision\n\tstatus: proposed\n"},
		{name: "an unterminated sequence is a decode failure", src: "supersedes: [0001\n"},
		{name: "a duplicate key is a decode failure", src: "status: proposed\nstatus: accepted\n"},
		{name: "a scalar document is not a mapping", src: "just a string\n"},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalFrontmatter([]byte(tt.src))

			if err == nil {
				t.Fatalf("UnmarshalFrontmatter = %#v, want an error (strict YAML only)", got)
			}
			if len(got) != 0 {
				t.Errorf("frontmatter = %#v, want none on failure", got)
			}
		})
	}
}

func TestAttr(t *testing.T) {
	fm := map[string]any{
		"title":      "A decision",
		"status":     "accepted",
		"id":         uint64(7),
		"draft":      true,
		"ratio":      1.5,
		"empty":      "",
		"absent":     nil,
		"supersedes": []any{"0001"},
		"nested":     map[string]any{"k": "v"},
	}
	tests := []struct {
		name string
		fm   map[string]any
		key  string
		want string
		ok   bool
	}{
		{name: "a string value", fm: fm, key: "title", want: "A decision", ok: true},
		{name: "an integer scalar becomes its digits", fm: fm, key: "id", want: "7", ok: true},
		{name: "a boolean scalar becomes its literal", fm: fm, key: "draft", want: "true", ok: true},
		{name: "an empty string is still a value", fm: fm, key: "empty", want: "", ok: true},
		{name: "a missing key has no value", fm: fm, key: "owner"},
		{name: "an explicit null has no value", fm: fm, key: "absent"},
		{name: "a list is not a scalar", fm: fm, key: "supersedes"},
		{name: "a mapping is not a scalar", fm: fm, key: "nested"},
		{name: "a nil frontmatter has no values", fm: nil, key: "title"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Attr(tt.fm, tt.key)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("Attr(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	t.Run("a float scalar keeps its written form", func(t *testing.T) {
		got, ok := Attr(fm, "ratio")
		if !ok {
			t.Fatal("ratio has no value, want one")
		}
		if got != "1.5" {
			t.Fatalf("ratio = %q, want 1.5", got)
		}
	})
}

func TestRefs(t *testing.T) {
	tests := []struct {
		name string
		fm   map[string]any
		key  string
		want []string
	}{
		{
			name: "a list of references",
			fm:   map[string]any{"supersedes": []any{"0001", "ADR-2"}},
			key:  "supersedes",
			want: []string{"0001", "ADR-2"},
		},
		{
			name: "a scalar is a single-element list",
			fm:   map[string]any{"supersedes": "0001"},
			key:  "supersedes",
			want: []string{"0001"},
		},
		{
			name: "numeric references are stringified without padding",
			fm:   map[string]any{"supersedes": []any{uint64(2), uint64(13)}},
			key:  "supersedes",
			want: []string{"2", "13"},
		},
		{
			name: "a numeric scalar is a single-element list",
			fm:   map[string]any{"supersedes": uint64(2)},
			key:  "supersedes",
			want: []string{"2"},
		},
		{
			name: "surrounding whitespace is trimmed",
			fm:   map[string]any{"supersedes": []any{"  0001  "}},
			key:  "supersedes",
			want: []string{"0001"},
		},
		{
			name: "blank entries are dropped",
			fm:   map[string]any{"supersedes": []any{"0001", "", "   "}},
			key:  "supersedes",
			want: []string{"0001"},
		},
		{
			name: "entries that are not scalars are dropped",
			fm:   map[string]any{"supersedes": []any{"0001", []any{"0002"}, map[string]any{"k": "v"}}},
			key:  "supersedes",
			want: []string{"0001"},
		},
		{
			name: "an empty list has no references",
			fm:   map[string]any{"supersedes": []any{}},
			key:  "supersedes",
		},
		{
			name: "a missing key has no references",
			fm:   map[string]any{"status": "accepted"},
			key:  "supersedes",
		},
		{
			name: "a nil frontmatter has no references",
			key:  "supersedes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := Refs(tt.fm, tt.key); !slices.Equal(got, tt.want) {
				t.Fatalf("Refs(%q) = %#v, want %#v", tt.key, got, tt.want)
			}
		})
	}
}

func TestFile(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a fixture document parses into everything the graph needs", func(t *testing.T) {
		path := testFixtureFile(t, "ok-basic", "000004.md")

		doc, err := File(path, cfg)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if doc.Path != path {
			t.Errorf("path = %q, want %q", doc.Path, path)
		}
		if doc.Name != "000004.md" {
			t.Errorf("name = %q, want 000004.md", doc.Name)
		}
		if doc.ID != "0004" {
			t.Errorf("id = %q, want 0004 (the configured width is 4)", doc.ID)
		}
		if !doc.HasFrontmatter || !doc.MatchesPattern {
			t.Errorf("hasFrontmatter = %v, matchesPattern = %v, want both true", doc.HasFrontmatter, doc.MatchesPattern)
		}
		if doc.Err != nil {
			t.Errorf("err = %v, want none", doc.Err)
		}
		for key, want := range map[string]string{
			"title":  "Schedule feed polling from the ingestion queue",
			"status": "accepted",
			"date":   "2025-03-08",
		} {
			got, ok := Attr(doc.Frontmatter, key)
			if !ok || got != want {
				t.Errorf("attr %s = %q (ok=%v), want %q", key, got, ok, want)
			}
		}
		if got, _ := Refs(doc.Frontmatter, "supersedes"); !slices.Equal(got, []string{"2"}) {
			t.Errorf("supersedes = %#v, want the raw un-normalized reference", got)
		}
		if got, _ := Refs(doc.Frontmatter, "depends-on"); !slices.Equal(got, []string{"3"}) {
			t.Errorf("depends-on = %#v, want the raw un-normalized reference", got)
		}
		if !strings.HasPrefix(doc.Body, "\n# Schedule feed polling from the ingestion queue\n") {
			t.Errorf("body = %q, want it to start after the frontmatter block", doc.Body)
		}
		if strings.Contains(doc.Body, "status: accepted") {
			t.Errorf("body = %q, want the frontmatter excluded", doc.Body)
		}
	})

	t.Run("the identifier follows the configured width", func(t *testing.T) {
		wide := config.ADRPreset()
		wide.IDWidth = 6

		doc, err := File(testFixtureFile(t, "ok-basic", "000004.md"), wide)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if doc.ID != "000004" {
			t.Fatalf("id = %q, want 000004", doc.ID)
		}
	})

	t.Run("a MADR status string survives as an attribute", func(t *testing.T) {
		doc, err := File(testFixtureFile(t, "ok-madr", "0002-store-thumbnails-on-the-local-disk.md"), cfg)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		got, ok := Attr(doc.Frontmatter, "status")
		if !ok || got != "superseded by 0003" {
			t.Fatalf("status = %q (ok=%v), want %q", got, ok, "superseded by 0003")
		}
	})

	t.Run("undecodable frontmatter is recorded on the document, not returned", func(t *testing.T) {
		doc, err := File(testFixtureFile(t, "invalid-yaml", "0002-negotiate-api-versions-by-header.md"), cfg)
		if err != nil {
			t.Fatalf("File: %v, want the failure on the document so later checks still run", err)
		}
		if doc.Err == nil {
			t.Fatal("doc.Err is nil, want the decode failure recorded")
		}
		if doc.ID != "0002" {
			t.Errorf("id = %q, want 0002 (the file is still a node)", doc.ID)
		}
		if !doc.HasFrontmatter {
			t.Error("hasFrontmatter = false, want true (the block is present, only its content is broken)")
		}
		if len(doc.Frontmatter) != 0 {
			t.Errorf("frontmatter = %#v, want none", doc.Frontmatter)
		}
		if !strings.Contains(doc.Body, "# Negotiate API versions by header") {
			t.Errorf("body = %q, want the body kept", doc.Body)
		}
	})

	t.Run("a managed filename without frontmatter keeps its body", func(t *testing.T) {
		body := "# No frontmatter here\n\nProse only.\n"
		dir := testWriteDocs(t, map[string]string{"0007-no-frontmatter.md": body})

		doc, err := File(filepath.Join(dir, "0007-no-frontmatter.md"), cfg)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if doc.HasFrontmatter {
			t.Error("hasFrontmatter = true, want false")
		}
		if !doc.MatchesPattern {
			t.Error("matchesPattern = false, want true (this is the missing-frontmatter warning path)")
		}
		if doc.ID != "0007" {
			t.Errorf("id = %q, want 0007", doc.ID)
		}
		if doc.Body != body {
			t.Errorf("body = %q, want the whole file", doc.Body)
		}
		if doc.Err != nil {
			t.Errorf("err = %v, want none", doc.Err)
		}
	})

	t.Run("an unmanaged filename carries no identity", func(t *testing.T) {
		dir := testWriteDocs(t, map[string]string{"notes.md": "# Notes\n"})

		doc, err := File(filepath.Join(dir, "notes.md"), cfg)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if doc.MatchesPattern {
			t.Error("matchesPattern = true, want false")
		}
		if doc.ID != "" {
			t.Errorf("id = %q, want none", doc.ID)
		}
	})

	t.Run("a missing file is an error", func(t *testing.T) {
		doc, err := File(filepath.Join(t.TempDir(), "absent.md"), cfg)

		if err == nil {
			t.Fatalf("File = %+v, want an error", doc)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err = %v, want it to wrap fs.ErrNotExist", err)
		}
	})
}

func TestDir(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("every managed document comes back in name order", func(t *testing.T) {
		docs, err := Dir(testFixture(t, "ok-basic"), cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		wantNames := []string{"000001.md", "000002.md", "000003.md", "000004.md", "000005.md", "000006.md"}
		if got := testNames(docs); !slices.Equal(got, wantNames) {
			t.Fatalf("names = %v, want %v", got, wantNames)
		}
		wantIDs := []string{"0001", "0002", "0003", "0004", "0005", "0006"}
		if got := testIDs(docs); !slices.Equal(got, wantIDs) {
			t.Fatalf("ids = %v, want %v", got, wantIDs)
		}
	})

	t.Run("an undecodable document does not stop the directory", func(t *testing.T) {
		docs, err := Dir(testFixture(t, "invalid-yaml"), cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if len(docs) != 2 {
			t.Fatalf("documents = %v, want both files", testNames(docs))
		}
		if docs[0].Err != nil {
			t.Errorf("%s err = %v, want none", docs[0].Name, docs[0].Err)
		}
		if docs[1].Err == nil {
			t.Errorf("%s err = nil, want the decode failure", docs[1].Name)
		}
	})

	t.Run("files that are neither frontmatter-bearing nor managed names are skipped", func(t *testing.T) {
		dir := testWriteDocs(t, map[string]string{
			"0001-keep-this.md":      "---\nstatus: accepted\n---\n\n# Keep this\n",
			"0002-no-frontmatter.md": "# No frontmatter\n",
			"notes.md":               "# Loose notes\n",
			"0003-not-markdown.txt":  "---\nstatus: accepted\n---\n",
			"docdag.yaml":            "id_width: 4\n",
		})

		docs, err := Dir(dir, cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		want := []string{"0001-keep-this.md", "0002-no-frontmatter.md"}
		if got := testNames(docs); !slices.Equal(got, want) {
			t.Fatalf("names = %v, want %v", got, want)
		}
	})

	t.Run("a file whose name carries no identifier is not a managed document", func(t *testing.T) {
		dir := testWriteDocs(t, map[string]string{
			"README.md":         "---\ntitle: Index of decisions\n---\n\n# Index\n",
			"0001-keep-this.md": "---\nstatus: accepted\n---\n\n# Keep this\n",
		})

		docs, err := Dir(dir, cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if got := testNames(docs); !slices.Equal(got, []string{"0001-keep-this.md"}) {
			t.Fatalf("names = %v, want only the identified document", got)
		}
	})

	t.Run("subdirectories are not descended", func(t *testing.T) {
		dir := testWriteDocs(t, map[string]string{
			"0001-keep-this.md":        "---\nstatus: accepted\n---\n\n# Keep this\n",
			"archive/0002-archived.md": "---\nstatus: rejected\n---\n\n# Archived\n",
		})

		docs, err := Dir(dir, cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if got := testNames(docs); !slices.Equal(got, []string{"0001-keep-this.md"}) {
			t.Fatalf("names = %v, want only the top-level document", got)
		}
	})

	t.Run("an empty directory yields no documents", func(t *testing.T) {
		docs, err := Dir(t.TempDir(), cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if len(docs) != 0 {
			t.Fatalf("documents = %v, want none", testNames(docs))
		}
	})

	t.Run("a missing directory is an error", func(t *testing.T) {
		docs, err := Dir(filepath.Join(t.TempDir(), "absent"), cfg)

		if err == nil {
			t.Fatalf("Dir = %v, want an error", testNames(docs))
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err = %v, want it to wrap fs.ErrNotExist", err)
		}
	})

	t.Run("a filename that does not match the pattern is not a managed document", func(t *testing.T) {
		dir := testWriteDocs(t, map[string]string{
			"0001-keep-this.md": "---\nstatus: accepted\n---\n\n# Keep this\n",
			"template-v2.md":    "---\nstatus: proposed\n---\n\n# Template\n",
			"notes-2024.md":     "---\nstatus: accepted\n---\n\n# Notes\n",
		})

		docs, err := Dir(dir, cfg)
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if got := testNames(docs); !slices.Equal(got, []string{"0001-keep-this.md"}) {
			t.Fatalf("names = %v, want only the document whose name matches the pattern", got)
		}
	})
}

// testBOM is the byte order mark a Windows editor may write in front of the
// opening delimiter.
var testBOM = string([]byte{0xEF, 0xBB, 0xBF})

func TestFrontmatterSpan(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		block string
		body  string
	}{
		{
			name:  "unix line endings",
			src:   "---\ntitle: Unix\n---\n\n# Unix\n",
			block: "title: Unix\n",
			body:  "\n# Unix\n",
		},
		{
			name:  "windows line endings",
			src:   "---\r\ntitle: Windows\r\n---\r\n\r\n# Windows\r\n",
			block: "title: Windows\r\n",
			body:  "\r\n# Windows\r\n",
		},
		{
			name:  "a byte order mark in front of the delimiter",
			src:   testBOM + "---\ntitle: Marked\n---\n\n# Marked\n",
			block: "title: Marked\n",
			body:  "\n# Marked\n",
		},
		{
			name:  "an empty block",
			src:   "---\n---\nBody only\n",
			block: "",
			body:  "Body only\n",
		},
		{
			name:  "a closing delimiter without a trailing newline",
			src:   "---\ntitle: Terse\n---",
			block: "title: Terse\n",
			body:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := FrontmatterSpan([]byte(tt.src))
			if !ok {
				t.Fatalf("FrontmatterSpan(%q) reported no block", tt.src)
			}
			if got := tt.src[start:end]; got != tt.block {
				t.Errorf("block = %q, want %q", got, tt.block)
			}
			frontmatter, body, ok := SplitFrontmatter([]byte(tt.src))
			if !ok {
				t.Fatalf("SplitFrontmatter(%q) reported no block", tt.src)
			}
			if string(frontmatter) != tt.block {
				t.Errorf("frontmatter = %q, want %q", frontmatter, tt.block)
			}
			if string(body) != tt.body {
				t.Errorf("body = %q, want %q", body, tt.body)
			}
		})
	}

	t.Run("a document without a block reports none", func(t *testing.T) {
		for _, src := range []string{"# No frontmatter\n", "---\ntitle: Unterminated\n", "", "---\n"} {
			if _, _, ok := FrontmatterSpan([]byte(src)); ok {
				t.Errorf("FrontmatterSpan(%q) reported a block", src)
			}
		}
	})
}

func TestFileParsesAWindowsAuthoredDocument(t *testing.T) {
	cfg := config.ADRPreset()
	dir := testWriteDocs(t, map[string]string{
		"0002-ship-logs.md": "---\r\ntitle: Ship logs\r\nstatus: accepted\r\nsupersedes:\r\n  - \"0001\"\r\n---\r\n\r\n# Ship logs\r\n",
	})

	doc, err := File(filepath.Join(dir, "0002-ship-logs.md"), cfg)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if !doc.HasFrontmatter {
		t.Fatal("hasFrontmatter = false, want true: CRLF is a line ending, not a missing block")
	}
	if doc.Err != nil {
		t.Fatalf("err = %v, want none", doc.Err)
	}
	for key, want := range map[string]string{"title": "Ship logs", "status": "accepted"} {
		if got, ok := Attr(doc.Frontmatter, key); !ok || got != want {
			t.Errorf("attr %s = %q (ok=%v), want %q", key, got, ok, want)
		}
	}
	refs, invalid := Refs(doc.Frontmatter, "supersedes")
	if !slices.Equal(refs, []string{"0001"}) {
		t.Errorf("supersedes = %#v, want the declared reference", refs)
	}
	if len(invalid) != 0 {
		t.Errorf("invalid = %#v, want none", invalid)
	}
}

func TestRefsReportsEntriesThatAreNotReferences(t *testing.T) {
	// An unquoted Obsidian wikilink decodes as a nested sequence, and a mapping
	// value is just as wrong: a tool for finding missing links must not drop a
	// malformed reference without saying so.
	fm := map[string]any{"supersedes": []any{"0001", []any{uint64(2)}, map[string]any{"k": "v"}}}

	refs, invalid := Refs(fm, "supersedes")

	if !slices.Equal(refs, []string{"0001"}) {
		t.Errorf("refs = %#v, want only the scalar entry", refs)
	}
	if len(invalid) != 2 {
		t.Fatalf("invalid = %#v, want both malformed entries", invalid)
	}
	for _, entry := range invalid {
		if strings.TrimSpace(entry) == "" {
			t.Errorf("invalid entry = %q, want a rendering of the offending value", entry)
		}
	}
}

func TestFileRecordsFrontmatterPositions(t *testing.T) {
	cfg := config.ADRPreset()
	tests := []struct {
		name     string
		content  string
		wantKeys map[string]int
	}{
		{
			name:     "unix line endings",
			content:  "---\ntitle: A decision\nstatus: accepted\nsupersedes:\n  - 0001\ndate: 2025-01-01\n---\n\n# A decision\n",
			wantKeys: map[string]int{"title": 2, "status": 3, "supersedes": 4, "date": 6},
		},
		{
			name:     "windows line endings",
			content:  "---\r\ntitle: A decision\r\nstatus: accepted\r\nsupersedes:\r\n  - 0001\r\ndate: 2025-01-01\r\n---\r\n\r\n# A decision\r\n",
			wantKeys: map[string]int{"title": 2, "status": 3, "supersedes": 4, "date": 6},
		},
		{
			name:     "a byte order mark does not shift the block",
			content:  "\xef\xbb\xbf---\ntitle: A decision\nstatus: accepted\n---\n\n# A decision\n",
			wantKeys: map[string]int{"title": 2, "status": 3},
		},
		{
			name:     "a comment and a blank line inside the block are counted",
			content:  "---\n# a note\n\ntitle: A decision\n\nstatus: accepted\n---\n\n# A decision\n",
			wantKeys: map[string]int{"title": 4, "status": 6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testWriteDocs(t, map[string]string{"0001-a-decision.md": tt.content})

			doc, err := File(filepath.Join(dir, "0001-a-decision.md"), cfg)
			if err != nil {
				t.Fatalf("File: %v", err)
			}
			if doc.FrontmatterLine != 1 {
				t.Errorf("frontmatterLine = %d, want 1", doc.FrontmatterLine)
			}
			for key, want := range tt.wantKeys {
				if got := doc.KeyLines[key]; got != want {
					t.Errorf("keyLines[%s] = %d, want %d (all lines: %v)", key, got, want, doc.KeyLines)
				}
			}
			if len(doc.KeyLines) != len(tt.wantKeys) {
				t.Errorf("keyLines = %v, want exactly %v", doc.KeyLines, tt.wantKeys)
			}
		})
	}
}

func TestFileRecordsWhereTheBodyStarts(t *testing.T) {
	cfg := config.ADRPreset()
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "the line after the closing delimiter",
			content: "---\ntitle: A decision\nstatus: accepted\n---\n\n# A decision\n",
			want:    5,
		},
		{
			name:    "windows line endings",
			content: "---\r\ntitle: A decision\r\nstatus: accepted\r\n---\r\n\r\n# A decision\r\n",
			want:    5,
		},
		{
			name:    "a byte order mark does not shift the body",
			content: "\xef\xbb\xbf---\ntitle: A decision\nstatus: accepted\n---\n# A decision\n",
			want:    5,
		},
		{
			name:    "a file without frontmatter is all body",
			content: "# Bare\n",
			want:    1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testWriteDocs(t, map[string]string{"0001-a-decision.md": tt.content})

			doc, err := File(filepath.Join(dir, "0001-a-decision.md"), cfg)
			if err != nil {
				t.Fatalf("File: %v", err)
			}
			if doc.BodyLine != tt.want {
				t.Errorf("bodyLine = %d, want %d", doc.BodyLine, tt.want)
			}
		})
	}
}

func TestFileWithoutFrontmatterHasNoPositions(t *testing.T) {
	dir := testWriteDocs(t, map[string]string{"0001-bare.md": "# Bare\n"})

	doc, err := File(filepath.Join(dir, "0001-bare.md"), config.ADRPreset())
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if doc.FrontmatterLine != 0 {
		t.Errorf("frontmatterLine = %d, want 0", doc.FrontmatterLine)
	}
	if len(doc.KeyLines) != 0 {
		t.Errorf("keyLines = %v, want none", doc.KeyLines)
	}
}

func TestFileLocatesADecodeFailureInTheFile(t *testing.T) {
	doc, err := File(testFixtureFile(t, "invalid-yaml", "0002-negotiate-api-versions-by-header.md"), config.ADRPreset())
	if err != nil {
		t.Fatalf("File: %v", err)
	}

	var fe *FrontmatterError
	if !errors.As(doc.Err, &fe) {
		t.Fatalf("doc.Err = %v (%T), want a *FrontmatterError", doc.Err, doc.Err)
	}
	if fe.Line != 2 {
		t.Errorf("line = %d, want 2: the offending key sits on the second line of the file", fe.Line)
	}
	if fe.Column < 1 {
		t.Errorf("column = %d, want a 1-based column", fe.Column)
	}
	if strings.ContainsAny(fe.Message, "\n\r") {
		t.Errorf("message = %q, want one line without the source excerpt", fe.Message)
	}
	if strings.Contains(fe.Message, "[") {
		t.Errorf("message = %q, want the position carried by the fields, not the text", fe.Message)
	}
}

func TestUnmarshalFrontmatterReportsTheBlockRelativePosition(t *testing.T) {
	const block = "title: A decision\nstatus: accepted\n  bogus: 1\n"
	dir := testWriteDocs(t, map[string]string{"0001-a-decision.md": Delimiter + "\n" + block + Delimiter + "\n"})

	_, err := UnmarshalFrontmatter([]byte(block))
	var relative *FrontmatterError
	if !errors.As(err, &relative) {
		t.Fatalf("err = %v (%T), want a *FrontmatterError", err, err)
	}
	if relative.Line < 1 || relative.Line > 3 {
		t.Fatalf("line = %d, want a line inside the block", relative.Line)
	}
	if relative.Error() == "" {
		t.Error("Error() is empty, want a diagnostic")
	}

	doc, err := File(filepath.Join(dir, "0001-a-decision.md"), config.ADRPreset())
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	var absolute *FrontmatterError
	if !errors.As(doc.Err, &absolute) {
		t.Fatalf("doc.Err = %v (%T), want a *FrontmatterError", doc.Err, doc.Err)
	}
	if absolute.Line != relative.Line+1 {
		t.Errorf("file line = %d, block line = %d: want the block line offset by the delimiter", absolute.Line, relative.Line)
	}
	if absolute.Column != relative.Column {
		t.Errorf("column = %d, want the block column %d unchanged", absolute.Column, relative.Column)
	}
}

func TestLocalize(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "repo", "root")
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "a document under the base becomes relative with forward slashes",
			path: filepath.Join(base, "docs", "adr", "0001-a.md"),
			want: "docs/adr/0001-a.md",
		},
		{
			name: "a document outside the base keeps its absolute path",
			path: filepath.Join(string(filepath.Separator), "elsewhere", "0001-a.md"),
			want: filepath.ToSlash(filepath.Join(string(filepath.Separator), "elsewhere", "0001-a.md")),
		},
		{
			name: "an already relative path only loses its separators",
			path: filepath.Join("..", "..", "testdata", "0001-a.md"),
			want: "../../testdata/0001-a.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := []*Document{{Path: tt.path}}

			Localize(docs, base)

			if docs[0].Path != tt.want {
				t.Fatalf("path = %q, want %q", docs[0].Path, tt.want)
			}
		})
	}
}
