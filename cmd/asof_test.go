package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/render"
)

// The corpus the as-of tests are written against: an ADR corpus that declares a
// period, with the binding projection ADR-0005 D4 gives an adr user who adopts
// one. 0001 binds today and 0002 takes over on the day it is dated, which is
// the one thing a period changes about a listing.
const (
	testPeriodConfig = `preset: adr
period: {from: date}
projections:
  - name: has_inforce_successor
    when: {via_inbound: {edge: supersedes, attr: {in_force: {eq: "true"}, status: {eq: accepted}}}}
  - name: accepted_unsuperseded
    when:
      attr: {status: {eq: accepted}, in_force: {eq: "true"}}
      not: {attr: {has_inforce_successor: {eq: "true"}}}
binding: accepted_unsuperseded
rules: []
`
	testCurrentDocument = "---\ntitle: Serve images from the application\nstatus: accepted\ndate: 2026-01-01\n---\n\n" +
		"# Serve images from the application\n"
	testFutureDocument = "---\ntitle: Serve images from a CDN\nstatus: accepted\ndate: 2027-01-01\nsupersedes:\n  - \"0001\"\n---\n\n" +
		"# Serve images from a CDN\n"
)

// testPeriodCorpus is that corpus on disk, with the caller standing in it.
func testPeriodCorpus(t *testing.T) string {
	t.Helper()
	return writeDocs(t, map[string]string{
		"docdag.yaml": testPeriodConfig,
		"docs/adr/0001-serve-images-from-the-application.md": testCurrentDocument,
		"docs/adr/0002-serve-images-from-a-cdn.md":           testFutureDocument,
	})
}

func TestAsOfFlagAndEnvironment(t *testing.T) {
	dir := testPeriodCorpus(t)

	t.Run("the flag names the day", func(t *testing.T) {
		t.Chdir(dir)

		got := run(t, "query", "--binding", "--as-of", "2027-01-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0002"})
	})

	t.Run("the environment names it where no flag does", func(t *testing.T) {
		t.Chdir(dir)
		t.Setenv(envAsOf, "2027-01-01")

		got := run(t, "query", "--binding")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0002"})
	})

	t.Run("the flag wins over the environment", func(t *testing.T) {
		t.Chdir(dir)
		t.Setenv(envAsOf, "2027-01-01")

		got := run(t, "query", "--binding", "--as-of", "2026-06-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0001"})
	})

	t.Run("a day that is not a date is a usage error", func(t *testing.T) {
		t.Chdir(dir)

		got := run(t, "query", "--binding", "--as-of", "next tuesday")

		assertExit(t, got, 2)
		if !strings.Contains(got.stderr, "YYYY-MM-DD") {
			t.Errorf("stderr = %q, want it to name the spelling a day takes", got.stderr)
		}
	})

	t.Run("an environment day that is not a date says which one it read", func(t *testing.T) {
		t.Chdir(dir)
		t.Setenv(envAsOf, "soon")

		got := run(t, "query", "--binding")

		assertExit(t, got, 2)
		if !strings.Contains(got.stderr, envAsOf) {
			t.Errorf("stderr = %q, want it to name the environment variable it read", got.stderr)
		}
	})
}

// TestQueryTheBindingSetAtAFutureDay is ADR-0005's own example: the day the
// successor takes over is a question a caller can ask before it arrives.
func TestQueryTheBindingSetAtAFutureDay(t *testing.T) {
	dir := testPeriodCorpus(t)
	t.Chdir(dir)

	t.Run("today the successor has not begun", func(t *testing.T) {
		got := run(t, "query", "--binding", "--as-of", "2026-06-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0001"})
	})

	t.Run("on the day it begins, membership flips", func(t *testing.T) {
		got := run(t, "query", "--binding", "--as-of", "2027-01-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0002"})
	})

	t.Run("resolve answers the same day the listing does", func(t *testing.T) {
		// The asymmetry ADR-0005 set out to remove: `resolve` walked to the
		// successor while `--binding` still held the predecessor.
		before := run(t, "resolve", "0001", "--as-of", "2026-06-01")
		after := run(t, "resolve", "0001", "--as-of", "2027-01-01")

		assertLines(t, "resolved before", lines(before.stdout), []string{"0001"})
		assertLines(t, "resolved after", lines(after.stdout), []string{"0002"})
	})
}

func TestValidateAsOfDefaultsToTheCommitterDate(t *testing.T) {
	t.Run("inside a repository the day belongs to the commit", func(t *testing.T) {
		dir := gitRepoAt(t, "2026-05-04", map[string]string{
			"docdag.yaml": testPeriodConfig,
			"docs/adr/0001-serve-images-from-the-application.md": testCurrentDocument,
		})
		t.Chdir(dir)

		got := run(t, "validate", "--format", "json")

		assertExit(t, got, 0)
		report := decodeJSON[render.Report](t, got.stdout)
		if report.AsOf != "2026-05-04" {
			t.Errorf("as_of = %q, want the day the commit carries: a gate answers for the commit, not for the clock", report.AsOf)
		}
	})

	t.Run("outside a repository it is the day the run happens on", func(t *testing.T) {
		dir := writeDocs(t, map[string]string{
			"docdag.yaml": testPeriodConfig,
			"docs/adr/0001-serve-images-from-the-application.md": testCurrentDocument,
		})
		t.Chdir(dir)

		got := run(t, "validate", "--format", "json")

		assertExit(t, got, 0)
		if decodeJSON[render.Report](t, got.stdout).AsOf == "" {
			t.Error("as_of is empty, want the day the run happens on where there is no commit to read")
		}
	})

	t.Run("a corpus without periods says nothing about the day in its text report", func(t *testing.T) {
		// The text report is what a person reads, and a corpus that answers the
		// same on every day has no day worth printing.
		dir := writeDocs(t, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDocument,
		})
		t.Chdir(dir)

		got := run(t, "validate")

		assertExit(t, got, 0)
		if strings.Contains(got.stdout, "as of") {
			t.Errorf("stdout = %q, want no as-of line without a period", got.stdout)
		}
	})
}

func TestAtReadsTheDocumentsOfARevision(t *testing.T) {
	dir := gitRepoAt(t, "2026-05-04", map[string]string{
		"docdag.yaml": testPeriodConfig,
		"docs/adr/0001-serve-images-from-the-application.md": testCurrentDocument,
	})
	// The second revision adds the successor, so the two revisions disagree
	// about what the vault holds and the two days disagree about what binds.
	successor := filepath.Join(dir, "docs", "adr", "0002-serve-images-from-a-cdn.md")
	if err := os.WriteFile(successor, []byte(testFutureDocument), 0o600); err != nil {
		t.Fatalf("write the second revision: %v", err)
	}
	git(t, dir, "add", "-A")
	gitCommit(t, dir, "2026-06-05", "the second revision")
	t.Chdir(dir)

	t.Run("the working tree holds both documents", func(t *testing.T) {
		got := run(t, "query", "--binding", "--as-of", "2027-01-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0002"})
	})

	t.Run("the first revision held one", func(t *testing.T) {
		// Transaction time on its own: at that revision the vault said nothing
		// about a successor, so 0001 binds however far ahead the day is.
		got := run(t, "query", "--binding", "--at", "HEAD~1", "--as-of", "2027-01-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0001"})
	})

	t.Run("the two axes are independent", func(t *testing.T) {
		// The bitemporal question: what the vault at this revision said was in
		// force on that day. The revision holds both documents, and on the
		// earlier day the successor has not begun.
		got := run(t, "query", "--binding", "--at", "HEAD", "--as-of", "2026-06-01")

		assertExit(t, got, 0)
		assertLines(t, "binding", lines(got.stdout), []string{"0001"})
	})

	t.Run("the report names the revision it read", func(t *testing.T) {
		got := run(t, "validate", "--format", "json", "--at", "HEAD~1")

		report := decodeJSON[render.Report](t, got.stdout)
		if report.At != "HEAD~1" {
			t.Errorf("at = %q, want the revision the documents were read from", report.At)
		}
		if report.Summary.Documents != 1 {
			t.Errorf("documents = %d, want the one the first revision held", report.Summary.Documents)
		}
		// The paths name the revision's own files rather than a temporary
		// directory nobody can open.
		if strings.Contains(got.stdout, os.TempDir()) {
			t.Errorf("report = %s, want the revision's paths rather than the tree it was written into", got.stdout)
		}
	})

	t.Run("a revision git cannot resolve is a setup error", func(t *testing.T) {
		got := run(t, "validate", "--at", "no-such-revision")

		assertExit(t, got, 3)
		if got.stderr == "" {
			t.Error("stderr is empty, want a diagnostic naming the revision")
		}
	})
}

// TestAtRefusesTheHistoryCheck draws the other line: --immutable-since compares
// the working tree against a base, and --at replaces the working tree.
func TestAtRefusesTheHistoryCheck(t *testing.T) {
	t.Chdir(gitRepo(t, map[string]string{
		"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDocument,
	}))

	got := run(t, "validate", "--immutable-since", "HEAD", "--at", "HEAD")

	assertExit(t, got, 2)
	if !strings.Contains(got.stderr, flagImmutableSince) {
		t.Errorf("stderr = %q, want it to name the two flags that disagree", got.stderr)
	}
}

// TestAtReadsAMultiKindCorpus: --at reroots every kind's directory, not one, so
// a corpus that keeps its documents in a directory per kind is readable at a
// revision exactly as a single-kind one is.
func TestAtReadsAMultiKindCorpus(t *testing.T) {
	config := `preset: adr
kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    period: {from: date}
  conform:
    dir: spec/conform
    id: '^conform/[a-z0-9-]+$'
edges:
  - {name: supersedes, key: supersedes, acyclic: true, direction: forward}
  - {name: enforces, key: enforces, direction: forward, from: [conform], to: [clause]}
rules: []
projections: []
`
	clause := "---\ntitle: Every claim carries evidence\nkind: clause\nstatus: accepted\ndate: 2026-01-01\n---\n\n# Every claim carries evidence\n"
	test := "---\ntitle: Check it\nkind: conform\nid: conform/uz-v-001\nenforces:\n  - UZ-V-001\ndate: 2026-01-02\n---\n\n# Check it\n"
	dir := gitRepoAt(t, "2026-05-04", map[string]string{
		"docdag.yaml":              config,
		"spec/clauses/UZ-V-001.md": clause,
		"spec/conform/uz-v-001.md": test,
	})
	// The second revision adds a clause the first does not hold, in the other
	// kind's directory.
	later := filepath.Join(dir, "spec", "clauses", "UZ-V-002.md")
	if err := os.WriteFile(later, []byte(strings.Replace(clause, "UZ-V-001", "UZ-V-002", 1)), 0o600); err != nil {
		t.Fatalf("write the second revision: %v", err)
	}
	git(t, dir, "add", "-A")
	gitCommit(t, dir, "2026-06-05", "the second revision")
	t.Chdir(dir)

	t.Run("the working tree holds three documents", func(t *testing.T) {
		got := run(t, "validate", "--format", "json")

		assertExit(t, got, 0)
		if n := decodeJSON[render.Report](t, got.stdout).Summary.Documents; n != 3 {
			t.Errorf("documents = %d, want the three the working tree holds", n)
		}
	})

	t.Run("the first revision held two, across both kinds", func(t *testing.T) {
		got := run(t, "validate", "--format", "json", "--at", "HEAD~1")

		assertExit(t, got, 0)
		report := decodeJSON[render.Report](t, got.stdout)
		if report.Summary.Documents != 2 {
			t.Errorf("documents = %d, want the two that revision held", report.Summary.Documents)
		}
		if report.Summary.Edges != 1 {
			t.Errorf("edges = %d, want the enforces edge, which needs both kinds rerooted", report.Summary.Edges)
		}
	})
}

func TestAtOutsideARepository(t *testing.T) {
	t.Chdir(writeDocs(t, map[string]string{
		"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDocument,
	}))

	got := run(t, "validate", "--at", "HEAD")

	assertExit(t, got, 3)
	if !strings.Contains(got.stderr, "git repository") {
		t.Errorf("stderr = %q, want it to say there is no repository to read", got.stderr)
	}
}

// TestNewRefusesAt draws the line --at is on: it changes which documents are
// read, and `new` writes one.
func TestNewRefusesAt(t *testing.T) {
	t.Chdir(gitRepo(t, map[string]string{
		"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDocument,
	}))

	got := run(t, "new", "Serve images from a CDN", "--at", "HEAD")

	assertExit(t, got, 2)
	if !strings.Contains(got.stderr, flagAt) {
		t.Errorf("stderr = %q, want it to name the flag that does not apply", got.stderr)
	}
}

func TestLintCorpusReadsTheDay(t *testing.T) {
	dir := testPeriodCorpus(t)
	t.Chdir(dir)

	t.Run("the corpus layer is evaluated at the day it was given", func(t *testing.T) {
		// The projection holds for whichever document is in force, so a lint
		// run about a future day reads a different corpus answer.
		got := run(t, "lint", "--corpus", "--format", "json", "--as-of", "2027-01-01")

		report := decodeJSON[render.LintReport](t, got.stdout)
		if report.AsOf != "2027-01-01" {
			t.Errorf("as_of = %q, want the day the run was given", report.AsOf)
		}
	})

	t.Run("the text report of a corpus run says which day", func(t *testing.T) {
		got := run(t, "lint", "--corpus", "--as-of", "2027-01-01")

		if !strings.Contains(got.stdout, "as of 2027-01-01") {
			t.Errorf("stdout = %q, want the closing line to carry the day", got.stdout)
		}
	})

	t.Run("a run that reads no corpus says nothing about a day", func(t *testing.T) {
		// Layer 1 answers about the configuration, which means the same thing
		// on every day.
		got := run(t, "lint", "--as-of", "2027-01-01")

		if strings.Contains(got.stdout, "as of") {
			t.Errorf("stdout = %q, want no day on a run that read no documents", got.stdout)
		}
	})
}

func TestStatsCarriesTheDay(t *testing.T) {
	dir := testPeriodCorpus(t)
	t.Chdir(dir)

	t.Run("json heads the counts with the day they are about", func(t *testing.T) {
		got := run(t, "stats", "--format", "json", "--as-of", "2027-01-01")

		assertExit(t, got, 0)
		report := decodeJSON[render.StatsReport](t, got.stdout)
		if report.AsOf != "2027-01-01" {
			t.Errorf("as_of = %q, want the day the counts are about", report.AsOf)
		}
		if report.Binding != 1 {
			t.Errorf("binding = %d, want the one document in force that day", report.Binding)
		}
	})

	t.Run("the text table carries a row for it", func(t *testing.T) {
		got := run(t, "stats", "--as-of", "2027-01-01")

		assertExit(t, got, 0)
		if !strings.Contains(got.stdout, "as of") {
			t.Errorf("stdout = %q, want an as-of row where the counts depend on the day", got.stdout)
		}
	})
}

func TestContextCarriesTheDay(t *testing.T) {
	dir := testPeriodCorpus(t)
	t.Chdir(dir)

	got := run(t, "context", "0001", "--format", "json", "--as-of", "2026-06-01")

	assertExit(t, got, 0)
	var brief struct {
		AsOf       string `json:"as_of"`
		At         string `json:"at"`
		ResolvesTo []struct {
			ID string `json:"id"`
		} `json:"resolves_to"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &brief); err != nil {
		t.Fatalf("decode the brief %q: %v", got.stdout, err)
	}
	if brief.AsOf != "2026-06-01" {
		t.Errorf("as_of = %q, want the day the brief is about", brief.AsOf)
	}
	if brief.At != "" {
		t.Errorf("at = %q, want none: the brief was read from the working tree", brief.At)
	}
}
