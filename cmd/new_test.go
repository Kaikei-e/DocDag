package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/internal/newdoc"
	"github.com/Kaikei-e/DocDag/internal/render"
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

	wantPath := docPath(dir, "0007-adopt-content-addressed-cache-keys.md")
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
	superseded := docPath(dir, "000004.md")
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

	wantPath := docPath(dir, "0004-rotate-signing-keys-weekly.md")
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
	want := docPath(dir, "0004-rotate-signing-keys-weekly.md")
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

func TestNewWithAnExplicitIdentifier(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	got := run(t, "new", "Adopt content addressed cache keys", "--id", "42", "--dir", dir)

	assertExit(t, got, 0)
	wantPath := docPath(dir, "0042-adopt-content-addressed-cache-keys.md")
	assertLines(t, "created path", lines(got.stdout), []string{wantPath})
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("stat created document: %v", err)
	}
}

func TestNewWithAnExistingIdentifierAndTitleIsIdempotent(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	existing := docPath(dir, "000004.md")
	before, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read %s: %v", existing, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	got := run(t, "new", "Schedule feed polling from the ingestion queue", "--id", "0004", "--dir", dir)

	assertExit(t, got, 0)
	assertLines(t, "existing path", lines(got.stdout), []string{existing})
	after, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read %s: %v", existing, err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the existing document was rewritten:\ngot:\n%s\nwant:\n%s", after, before)
	}
	now, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(now) != len(entries) {
		t.Errorf("the corpus holds %d documents, want the %d it started with", len(now), len(entries))
	}
}

func TestNewWithAnExistingIdentifierAndADifferentTitleFails(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	got := run(t, "new", "Adopt content addressed cache keys", "--id", "0004", "--dir", dir)

	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "Schedule feed polling from the ingestion queue") {
		t.Errorf("stderr = %q, want it to name the title already under the identifier", got.stderr)
	}
}

func TestNewRefusesACorpusWithAnIdentifierCollision(t *testing.T) {
	dir := copyFixture(t, "id-collision")

	got := run(t, "new", "Adopt content addressed cache keys", "--dir", dir)

	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "0004") {
		t.Errorf("stderr = %q, want it to name the colliding identifier", got.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "0005-adopt-content-addressed-cache-keys.md")); !os.IsNotExist(err) {
		t.Error("new wrote a document into a corpus with an identifier collision")
	}
}

func TestNewDryRunOfAnExistingDocumentPlansNoWrite(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	existing := docPath(dir, "000004.md")

	got := run(t, "new", "Schedule feed polling from the ingestion queue", "--id", "0004", "--dry-run", "--dir", dir)

	assertExit(t, got, 0)
	assertLines(t, "plan", lines(got.stdout), []string{"exists 0004 " + existing})
}

func TestNewDryRunPrintsThePlanAndWritesNothing(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	superseded := docPath(dir, "000004.md")
	before, err := os.ReadFile(superseded)
	if err != nil {
		t.Fatalf("read %s: %v", superseded, err)
	}

	got := run(t, "new", "Adopt content addressed cache keys", "--supersedes", "0004", "--dry-run", "--dir", dir)

	assertExit(t, got, 0)
	created := docPath(dir, "0007-adopt-content-addressed-cache-keys.md")
	assertLines(t, "plan", lines(got.stdout), []string{
		"create 0007 " + created,
		"rewrite " + superseded + " status: superseded",
	})
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Error("--dry-run created the document it only planned")
	}
	after, err := os.ReadFile(superseded)
	if err != nil {
		t.Fatalf("read %s: %v", superseded, err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("--dry-run rewrote a document:\ngot:\n%s\nwant:\n%s", after, before)
	}
}

func TestNewDryRunJSONPlan(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	got := run(t, "new", "Adopt content addressed cache keys", "--supersedes", "0004", "--dry-run", "--format", "json", "--dir", dir)

	assertExit(t, got, 0)
	plan := decodeJSON[render.Plan](t, got.stdout)
	want := render.Plan{
		SchemaVersion: render.PlanSchemaVersion,
		ID:            "0007",
		Path:          docPath(dir, "0007-adopt-content-addressed-cache-keys.md"),
		Rewrites: []render.PlanRewrite{
			{Path: docPath(dir, "000004.md"), Status: "superseded"},
		},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan = %+v, want %+v", plan, want)
	}
}

func TestNewNamesFilesTheWayTheCallerWouldTypeThem(t *testing.T) {
	corpus := map[string]string{
		"docs/adr/0001-a-decision.md": "---\ntitle: A decision\nstatus: accepted\ndate: 2025-01-01\n---\n\n# A decision\n",
	}

	t.Run("the plan is relative to the working directory", func(t *testing.T) {
		t.Chdir(writeDocs(t, corpus))

		got := run(t, "new", "Another decision", "--supersedes", "0001", "--dry-run", "--format", "json")

		assertExit(t, got, 0)
		plan := decodeJSON[render.Plan](t, got.stdout)
		want := render.Plan{
			SchemaVersion: render.PlanSchemaVersion,
			ID:            "0002",
			Path:          "docs/adr/0002-another-decision.md",
			Rewrites:      []render.PlanRewrite{{Path: "docs/adr/0001-a-decision.md", Status: "superseded"}},
		}
		if !reflect.DeepEqual(plan, want) {
			t.Errorf("plan = %+v, want %+v", plan, want)
		}
	})

	t.Run("the created path is relative to the working directory", func(t *testing.T) {
		t.Chdir(writeDocs(t, corpus))

		got := run(t, "new", "Another decision")

		assertExit(t, got, 0)
		assertLines(t, "created path", lines(got.stdout), []string{"docs/adr/0002-another-decision.md"})
	})
}

func TestNewDryRunJSONSaysWhetherTheDocumentIsAlreadyThere(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	t.Run("an identifier the corpus already holds", func(t *testing.T) {
		got := run(t, "new", "Schedule feed polling from the ingestion queue", "--id", "0004", "--dry-run", "--format", "json", "--dir", dir)

		assertExit(t, got, 0)
		payload := decodeJSON[map[string]any](t, got.stdout)
		if payload["exists"] != true {
			t.Errorf("payload = %v, want \"exists\": true", payload)
		}
	})

	t.Run("a document that would be written", func(t *testing.T) {
		got := run(t, "new", "Adopt content addressed cache keys", "--dry-run", "--format", "json", "--dir", dir)

		assertExit(t, got, 0)
		payload := decodeJSON[map[string]any](t, got.stdout)
		exists, present := payload["exists"]
		if !present || exists != false {
			t.Errorf("payload = %v, want \"exists\": false, which is always there", payload)
		}
	})
}

func TestNewDryRunKeepsTheExitCodesOfARealRun(t *testing.T) {
	dir := copyFixture(t, "ok-basic")

	got := run(t, "new", "Adopt content addressed cache keys", "--supersedes", "0009", "--dry-run", "--dir", dir)

	assertExit(t, got, 1)
}

func TestNewHonoursTheConfiguredFilenameTemplate(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	cfg := writeDocs(t, map[string]string{"docdag.yaml": "id_width: 6\nfilename: \"{id}.md\"\n"})

	got := run(t, "new", "Adopt content addressed cache keys", "--config", filepath.Join(cfg, "docdag.yaml"), "--dir", dir)

	assertExit(t, got, 0)
	wantPath := docPath(dir, "000007.md")
	assertLines(t, "created path", lines(got.stdout), []string{wantPath})
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("stat created document: %v", err)
	}
}

func TestNewRejectsAFilenameTemplateWithoutAnIdentifier(t *testing.T) {
	dir := copyFixture(t, "ok-basic")
	cfg := writeDocs(t, map[string]string{"docdag.yaml": "filename: \"{slug}.md\"\n"})

	got := run(t, "new", "Adopt content addressed cache keys", "--config", filepath.Join(cfg, "docdag.yaml"), "--dir", dir)

	assertExit(t, got, 3)
	if !strings.Contains(got.stderr, "{id}") {
		t.Errorf("stderr = %q, want it to name the missing placeholder", got.stderr)
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

func TestNewRefusesAMultiKindCorpus(t *testing.T) {
	// Which kind to create, under which identity rules and from which template,
	// is a question the corpus has no default answer to.
	got := run(t, "new", "Evidence is addressable", "--config", filepath.Join(fixture(t, "kinds"), "docdag.yaml"))

	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "new requires --kind on a multi-kind corpus") {
		t.Errorf("stderr = %q, want it to ask for --kind", got.stderr)
	}
}

func TestNewRefusesAKindOnACorpusWithoutKinds(t *testing.T) {
	got := run(t, "new", "Adopt content addressed cache keys", "--kind", "clause", "--dir", fixture(t, "ok-basic"))

	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "describes nothing") {
		t.Errorf("stderr = %q, want it to say the corpus declares no kinds", got.stderr)
	}
}

func TestNewRefusesAKindNobodyDeclared(t *testing.T) {
	got := run(t, "new", "Runs are recorded with their seed", "--kind", "policy", "--config", specVaultConfig(t))

	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "unknown kind") || !strings.Contains(got.stderr, "clause") {
		t.Errorf("stderr = %q, want it to name the unknown kind and the declared ones", got.stderr)
	}
}

func TestNewRequiresAnIdentifierForAPatternKind(t *testing.T) {
	// A declared pattern is a spelling, not a sequence: there is no next
	// UZ-V-007 to take, so the caller has to say which one it is.
	got := run(t, "new", "Runs are recorded with their seed", "--kind", "clause", "--config", specVaultConfig(t))

	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "--id") {
		t.Errorf("stderr = %q, want it to ask for the identifier", got.stderr)
	}
}

func TestNewCreatesADocumentOfAKind(t *testing.T) {
	vault := copyFixtureTree(t, "spec-vault")
	cfg := filepath.Join(vault, "docdag.yaml")
	title := "Runs are recorded with their seed"

	got := run(t, "new", title, "--kind", "clause", "--id", "UZ-V-007", "--config", cfg)

	assertExit(t, got, 0)
	// The identifier names the file: the kind's pattern accepts that stem, so
	// the name and the identity agree without a slug between them.
	wantPath := docPath(vault, "spec", "clauses", "UZ-V-007.md")
	assertLines(t, "created path", lines(got.stdout), []string{wantPath})

	created, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	frontmatter, body := splitDocument(t, created)
	for key, want := range map[string]any{"id": "UZ-V-007", "kind": "clause", "title": title, "status": "proposed"} {
		if frontmatter[key] != want {
			t.Errorf("%s = %v, want %v", key, frontmatter[key], want)
		}
	}
	if date := fmt.Sprint(frontmatter["date"]); date == "" {
		t.Errorf("date = %q, want today", date)
	}
	// The edges a clause may declare are offered as comments: an edge key that
	// is present and names nothing is the empty_edge finding.
	for _, key := range []string{"supersedes", "premise", "rationale", "counterexample"} {
		if _, present := frontmatter[key]; present {
			t.Errorf("frontmatter carries %q, want it commented out: %+v", key, frontmatter)
		}
		if !strings.Contains(string(created), "# "+key+":") {
			t.Errorf("document offers no %q stub:\n%s", key, created)
		}
	}
	// A clause writes no edge a clause may not declare.
	for _, key := range []string{"enforces", "deviates-from", "measures"} {
		if strings.Contains(string(created), key+":") {
			t.Errorf("document offers %q, which a clause may not declare:\n%s", key, created)
		}
	}
	if !strings.Contains(body, "# "+title) {
		t.Errorf("body is missing the title heading:\n%s", body)
	}
}

// TestNewLeavesTheRestOfASpecCorpusExactlyAsItFoundIt holds creation to what it
// can promise once a kind declares what a document of it has to say: every
// other document reads exactly as it did, and the new one carries the blanks
// its own kind requires — the strength it claims and the subject it speaks to,
// which are the two things only its author knows. Both are offered as stubs in
// the created file, so the finding names a line the author is already looking
// at rather than sending them to the configuration.
func TestNewLeavesTheRestOfASpecCorpusExactlyAsItFoundIt(t *testing.T) {
	vault := copyFixtureTree(t, "spec-vault")
	cfg := filepath.Join(vault, "docdag.yaml")
	before := run(t, "validate", "--config", cfg)

	created := run(t, "new", "Runs are recorded with their seed", "--kind", "clause", "--id", "UZ-V-007", "--config", cfg)
	assertExit(t, created, 0)

	after := run(t, "validate", "--config", cfg)
	assertExit(t, after, 1)
	own, rest := []string{}, []string{}
	for _, line := range findingLines(after.stdout) {
		if strings.Contains(line, "UZ-V-007") {
			own = append(own, line)
			continue
		}
		rest = append(rest, line)
	}
	assertLines(t, "findings about the documents that were there", rest, findingLines(before.stdout))
	assertLines(t, "findings about the created document", own, []string{
		"UZ-V-007.md:5: ERROR cardinality UZ-V-007: 0 outbound about edges fall short of min_outbound 1",
		`UZ-V-007.md:5: ERROR missing_field UZ-V-007: frontmatter key "modality" is required, one of: MUST, MUST_NOT, SHOULD, SHOULD_NOT, MAY`,
	})
	document, err := os.ReadFile(docPath(vault, "spec", "clauses", "UZ-V-007.md"))
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	for _, stub := range []string{"# modality: <MUST|MUST_NOT|SHOULD|SHOULD_NOT|MAY>", "# about:"} {
		if !strings.Contains(string(document), stub) {
			t.Errorf("document offers no %q stub:\n%s", stub, document)
		}
	}
}

func TestNewCreatesAKindWithoutAStatusVocabulary(t *testing.T) {
	vault := copyFixtureTree(t, "spec-vault")

	got := run(t, "new", "Check that runs record their seed",
		"--kind", "conform", "--id", "conform/uz-v-007", "--config", filepath.Join(vault, "docdag.yaml"))

	assertExit(t, got, 0)
	// The identifier carries a slash, so its last segment names the file and
	// the frontmatter carries the whole of it.
	wantPath := docPath(vault, "spec", "conform", "uz-v-007.md")
	assertLines(t, "created path", lines(got.stdout), []string{wantPath})

	created, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read created document: %v", err)
	}
	frontmatter, _ := splitDocument(t, created)
	if frontmatter["id"] != "conform/uz-v-007" {
		t.Errorf("id = %v, want the whole identifier", frontmatter["id"])
	}
	if _, present := frontmatter["status"]; present {
		t.Errorf("frontmatter carries a status, want none: the kind answers to no vocabulary: %+v", frontmatter)
	}
}

func TestNewDryRunPlansADocumentOfAKind(t *testing.T) {
	vault := copyFixtureTree(t, "spec-vault")

	got := run(t, "new", "Runs are recorded with their seed", "--kind", "clause", "--id", "UZ-V-007",
		"--dry-run", "--format", "json", "--config", filepath.Join(vault, "docdag.yaml"))

	assertExit(t, got, 0)
	plan := decodeJSON[render.Plan](t, got.stdout)
	want := render.Plan{
		SchemaVersion: render.PlanSchemaVersion,
		ID:            "UZ-V-007",
		Path:          docPath(vault, "spec", "clauses", "UZ-V-007.md"),
		Rewrites:      []render.PlanRewrite{},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan = %+v, want %+v", plan, want)
	}
	if _, err := os.Stat(filepath.Join(vault, "spec", "clauses", "UZ-V-007.md")); !os.IsNotExist(err) {
		t.Error("--dry-run created the document it only planned")
	}
}
