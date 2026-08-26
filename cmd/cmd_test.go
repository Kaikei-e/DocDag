package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// fixturesRoot is resolved before any test changes the working directory.
var fixturesRoot string

func TestMain(m *testing.M) {
	root, err := filepath.Abs(filepath.Join("..", "testdata", "fixtures"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fixturesRoot = root
	os.Exit(m.Run())
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, args ...string) runResult {
	t.Helper()
	var out, errOut bytes.Buffer
	code := executeWith(args, &out, &errOut)
	return runResult{code: code, stdout: out.String(), stderr: errOut.String()}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(fixturesRoot, name)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return dir
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, "\r"))
		}
	}
	return out
}

// findingPattern splits a finding line into its location and the rest, and
// lineSuffix peels the line number off that location, so an expectation can
// name a file without the temporary directory it happens to live in.
var (
	findingPattern = regexp.MustCompile(`^(?:(.+?): )?((?:ERROR|WARN) \S+ .*)$`)
	lineSuffix     = regexp.MustCompile(`:\d+$`)
)

func findingLines(s string) []string {
	var out []string
	for _, l := range lines(s) {
		m := findingPattern.FindStringSubmatch(l)
		switch {
		case m == nil:
			continue
		case m[1] == "":
			out = append(out, m[2])
			continue
		}
		path, suffix := m[1], ""
		if at := lineSuffix.FindStringIndex(path); at != nil {
			path, suffix = path[:at[0]], path[at[0]:]
		}
		out = append(out, filepath.Base(path)+suffix+": "+m[2])
	}
	return out
}

func assertPrefixes(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d lines %q, want %d %q", what, len(got), got, len(want), want)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("%s line %d: got %q, want prefix %q", what, i, got[i], want[i])
		}
	}
}

func assertLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d lines %q, want %d %q", what, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s line %d: got %q, want %q", what, i, got[i], want[i])
		}
	}
}

func assertExit(t *testing.T, got runResult, want int) {
	t.Helper()
	if got.code != want {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", got.code, want, got.stdout, got.stderr)
	}
}

func writeDocs(t *testing.T, files map[string]string) string {
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

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := fixture(t, name)
	dst := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), content, 0o600); err != nil {
			t.Fatalf("write %s: %v", entry.Name(), err)
		}
	}
	return dst
}

func decodeJSON[T any](t *testing.T, payload string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		t.Fatalf("decode JSON %q: %v", payload, err)
	}
	return v
}

func TestRootWithoutArgumentsPrintsHelp(t *testing.T) {
	got := run(t)
	assertExit(t, got, 0)
	for _, name := range []string{"docdag", "validate", "resolve", "query", "export", "stats", "new"} {
		if !strings.Contains(got.stdout, name) {
			t.Errorf("help output does not mention %q: %q", name, got.stdout)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	okBasic := fixture(t, "ok-basic")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"bogus"}},
		{name: "unknown flag", args: []string{"validate", "--nope", "--dir", okBasic}},
		{name: "unknown output format", args: []string{"validate", "--format", "yaml", "--dir", okBasic}, want: "invalid --format"},
		{name: "text is not an export format", args: []string{"export", "--format", "text", "--dir", okBasic}, want: "invalid --format"},
		{name: "resolve without a reference", args: []string{"resolve"}},
		{name: "resolve with two references", args: []string{"resolve", "0001", "0002"}},
		{name: "validate with an argument", args: []string{"validate", "extra"}},
		{name: "new without a title", args: []string{"new"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.args...)
			assertExit(t, got, 2)
			if tt.want != "" && !strings.Contains(got.stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tt.want)
			}
		})
	}
}

func TestIOErrorsExitThree(t *testing.T) {
	okBasic := fixture(t, "ok-basic")
	missing := filepath.Join(t.TempDir(), "absent")
	tests := []struct {
		name string
		args []string
	}{
		{name: "documents directory does not exist", args: []string{"validate", "--dir", missing}},
		{name: "config file does not exist", args: []string{"validate", "--dir", okBasic, "--config", filepath.Join(missing, "docdag.yaml")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.args...)
			assertExit(t, got, 3)
			if got.stderr == "" {
				t.Error("stderr is empty, want a diagnostic")
			}
		})
	}
}

func TestDiscoveryFindsAWellKnownDirectory(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		filepath.Join("docs", "adr", "0001-pick-a-directory-layout.md"): "---\ntitle: Pick a directory layout\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Pick a directory layout\n",
	})
	t.Chdir(dir)

	got := run(t, "validate")
	assertExit(t, got, 0)
	want := "OK: 1 docs, 0 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary = %q, want %q", got.stdout, want)
	}
}

func TestDiscoveryFailureExitsThree(t *testing.T) {
	t.Chdir(t.TempDir())

	got := run(t, "validate")
	assertExit(t, got, 3)
	if got.stderr == "" {
		t.Error("stderr is empty, want a diagnostic naming the missing documents directory")
	}
}

func TestConfigFileOverridesThePreset(t *testing.T) {
	dir := writeDocs(t, map[string]string{"docdag.yaml": "id_width: 6\n"})
	got := run(t, "resolve", "1", "--dir", fixture(t, "ok-basic"), "--config", filepath.Join(dir, "docdag.yaml"))
	assertExit(t, got, 0)
	assertLines(t, "resolve", lines(got.stdout), []string{"000004"})
}

func TestTheCommandSetIsExactlyTheDocumentedOne(t *testing.T) {
	want := []string{"validate", "resolve", "query", "export", "stats", "new", "help"}

	// The help command is registered lazily, so ask for it before counting.
	root := newRootCmd()
	root.InitDefaultHelpCmd()
	var got []string
	for _, cmd := range root.Commands() {
		got = append(got, cmd.Name())
	}
	slices.Sort(got)
	if !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("the command tree is %v, want exactly %v", got, want)
	}
	assertExit(t, run(t, "completion", "bash"), 2)
}

func TestAWindowsAuthoredCorpusIsManagedLikeAnyOther(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-store-blobs-on-disk.md": "---\r\ntitle: Store blobs on disk\r\nstatus: superseded\r\ndate: 2025-01-01\r\n---\r\n\r\n# Store blobs on disk\r\n",
		"0002-store-blobs-in-s3.md":   "---\r\ntitle: Store blobs in S3\r\nstatus: accepted\r\nsupersedes:\r\n  - \"0001\"\r\ndate: 2025-02-01\r\n---\r\n\r\n# Store blobs in S3\r\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 0)
	assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	want := "OK: 2 docs, 1 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary line = %q, want %q", got.stdout, want)
	}
}

func TestARenamedStatusFieldCarriesThePresetRules(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"docdag.yaml":              "status_field: state\n",
		"docs/adr/0001-first.md":   "---\ntitle: First\nstate: superseded\ndate: 2025-01-01\n---\n\n# First\n",
		"docs/adr/0002-second.md":  "---\ntitle: Second\nstate: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-02-01\n---\n\n# Second\n",
		"docs/adr/0003-derived.md": "---\ntitle: Derived\nstate: superseded by 0002\ndate: 2025-03-01\n---\n\n# Derived\n",
	})
	t.Chdir(dir)

	got := run(t, "validate")

	assertExit(t, got, 0)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0003-derived.md:3: WARN unstructured_supersedes 0003:"})
	want := "OK: 3 docs, 2 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary line = %q, want %q", got.stdout, want)
	}
}

func TestDirFlagBeatsTheConfigFile(t *testing.T) {
	dir := writeDocs(t, map[string]string{"docdag.yaml": "dir: " + filepath.Join(t.TempDir(), "absent") + "\n"})
	got := run(t, "validate", "--config", filepath.Join(dir, "docdag.yaml"), "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 0)
	want := "OK: 6 docs, 4 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary = %q, want %q", got.stdout, want)
	}
}

func TestVersionFlagReportsTheBuiltVersion(t *testing.T) {
	got := run(t, "--version")

	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "docdag version dev") {
		t.Errorf("version output = %q, want it to contain %q", got.stdout, "docdag version dev")
	}
}
