package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/brief"
)

func TestContextTextReportsTheReferenceAndItsResolution(t *testing.T) {
	got := run(t, "context", "0002", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  0002  Store thumbnails on the local disk  [superseded]  ",
		"    Chosen option: a directory on the local disk, sharded by the first two characters",
		"    of the cache key.",
		"resolves to (0002 is superseded)",
		"  0003  Store thumbnails in object storage  [accepted]  ",
		"    Chosen option: object storage behind a read-through layer, keeping the",
		"    content-addressed key from the caching decision unchanged.",
	})
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
}

func TestContextTextGroupsTheNeighbourhood(t *testing.T) {
	got := run(t, "context", "0001", "--depth", "2", "--all", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  0001  Cache rendered thumbnails  [accepted]  ",
		"    Chosen option: cache rendered thumbnails keyed by source URL and requested size.",
		"    The key is content-addressed, so an entry is never stale and never invalidated.",
		"ancestors",
		"  0003  Store thumbnails in object storage  [accepted]  ",
		"    Chosen option: object storage behind a read-through layer, keeping the",
		"    content-addressed key from the caching decision unchanged.",
		"  0004  Expire thumbnails after thirty days  [proposed]  ",
		"    Proposed: a lifecycle rule on the bucket, so expiry is configuration rather than",
		"    code.",
	})
}

func TestContextKeepsOnlyBindingNeighboursByDefault(t *testing.T) {
	got := run(t, "context", "0001", "--depth", "2", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	if strings.Contains(got.stdout, "0004") {
		t.Errorf("context = %q, want the proposed document left out without --all", got.stdout)
	}
	if !strings.Contains(got.stdout, "0003") {
		t.Errorf("context = %q, want the binding ancestor kept", got.stdout)
	}
}

func TestContextRestrictsTheWalkToAnEdgeType(t *testing.T) {
	got := run(t, "context", "0001", "--depth", "2", "--all", "--edge", "supersedes", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	if strings.Contains(got.stdout, "ancestors") {
		t.Errorf("context = %q, want no ancestors over supersedes alone", got.stdout)
	}
}

func TestContextReadsTheRequestedSection(t *testing.T) {
	got := run(t, "context", "0002", "--section", "Consequences", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "Nothing new to operate") {
		t.Errorf("context = %q, want the consequences paragraph", got.stdout)
	}
	if strings.Contains(got.stdout, "Chosen option") {
		t.Errorf("context = %q, want only the requested section", got.stdout)
	}
}

func TestContextBudgetDegradesEntriesToOneLine(t *testing.T) {
	got := run(t, "context", "0002", "--budget", "1", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	for _, line := range lines(got.stdout) {
		if strings.HasPrefix(line, "    ") {
			t.Errorf("line %q survived a budget of one token, want the one-line form", line)
		}
	}
	if !strings.Contains(got.stdout, "budget: 1 token") {
		t.Errorf("context = %q, want the spent budget reported", got.stdout)
	}
}

func TestContextJSONCarriesTheWholeBrief(t *testing.T) {
	got := run(t, "context", "0002", "--format", "json", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	b := decodeJSON[brief.Brief](t, got.stdout)
	if b.SchemaVersion != brief.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", b.SchemaVersion, brief.SchemaVersion)
	}
	if b.PresetVersion != config.ADRPresetVersion {
		t.Errorf("preset_version = %d, want the preset's %d", b.PresetVersion, config.ADRPresetVersion)
	}
	if b.Ref.ID != "0002" || b.Ref.Status != "superseded" {
		t.Errorf("ref = %+v, want the superseded document", b.Ref)
	}
	if len(b.ResolvesTo) != 1 || b.ResolvesTo[0].ID != "0003" {
		t.Errorf("resolves_to = %+v, want 0003", b.ResolvesTo)
	}
	if b.Budget.Limit != 2000 {
		t.Errorf("budget limit = %d, want the default of 2000 tokens", b.Budget.Limit)
	}
	if b.Budget.Used == 0 {
		t.Error("budget used = 0, want the tokens the report costs")
	}
	for _, key := range []string{`"ancestors"`, `"descendants"`} {
		if !strings.Contains(got.stdout, key) {
			t.Errorf("report = %q, want the %s key present even when empty", got.stdout, key)
		}
	}
}

func TestContextMarkdown(t *testing.T) {
	got := run(t, "context", "0002", "--format", "md", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	for _, want := range []string{
		"# 0002 Store thumbnails on the local disk",
		"- status: superseded",
		"## Resolves to",
		"### 0003 Store thumbnails in object storage",
		"Chosen option: object storage behind a read-through layer, keeping the",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("markdown = %q, want it to contain %q", got.stdout, want)
		}
	}
}

func TestContextIsDeterministic(t *testing.T) {
	dir := fixture(t, "ok-madr")
	first := run(t, "context", "0001", "--depth", "2", "--all", "--dir", dir)
	second := run(t, "context", "0001", "--depth", "2", "--all", "--dir", dir)

	if first.stdout != second.stdout {
		t.Errorf("context output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.stdout, second.stdout)
	}
}

func TestContextFailures(t *testing.T) {
	dir := fixture(t, "ok-madr")
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "unknown document", args: []string{"context", "0099"}, want: 1},
		{name: "unrecognized reference", args: []string{"context", "not-a-reference"}, want: 1},
		{name: "no reference", args: []string{"context"}, want: 2},
		{name: "unknown edge type", args: []string{"context", "0001", "--edge", "relates-to"}, want: 2},
		{name: "report format", args: []string{"context", "0001", "--format", "github"}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, append(tt.args, "--dir", dir)...)

			assertExit(t, got, tt.want)
			if got.stderr == "" {
				t.Error("stderr is empty, want a diagnostic")
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty on failure", got.stdout)
			}
		})
	}
}

func TestContextReadsAMultiKindCorpus(t *testing.T) {
	got := run(t, "context", "UZ-V-001", "--config", filepath.Join(fixture(t, "kinds"), "docdag.yaml"))

	assertExit(t, got, 0)
	// One brief over three kinds: the clause, the conformance test that
	// enforces it and the deviation recorded against it.
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  UZ-V-001  Every claim carries evidence  [accepted]  ",
		"    A report states what it measured and how, so a reader can repeat the run.",
		"ancestors",
		"  UZ-V-006  Thresholds are declared before the run  [accepted]  ",
		"    A clause is not a conformance test, so it may not declare an enforces edge.",
		"  conform/uz-v-001  Check that every claim carries evidence  [accepted]  ",
		"    The identifier carries a slash, so no file name can hold it: it is written in",
		"    the frontmatter instead.",
		"  dev-0001  The importer reports counts without inputs  [accepted]  ",
		"    Recorded until the importer learns to attach the inputs it read.",
	})
}

func TestContextReadsACorpusUnderTheSpecPreset(t *testing.T) {
	// One brief over six kinds: the clause, its subject, the conformance test
	// that enforces it, the measurement taken of it, and the premise, principle
	// and post-mortem it rests on, argues from and failed as. The subject and
	// the clauses that share it are the related group, which is reported
	// whatever --all says: the walks reach the rest.
	got := run(t, "context", "UZ-V-001", "--all", "--config", specVaultConfig(t))

	assertExit(t, got, 0)
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  UZ-V-001  Every claim carries evidence  [accepted]  ",
		"    A report states what it measured and how, so a reader can repeat the run.",
		"related",
		"  topic/evidence  Evidence behind a claim  []  ",
		"    What a report has to carry for a reader to believe a number in it: the run that",
		"    produced it, the inputs it read, and enough of both to repeat the run.",
		"  UZ-V-002  A failing run names the input it read  [accepted]  ",
		"    No conformance test enforces this clause, so calling it MUST is a claim rather",
		"    than a property: orphan_must says so.",
		"ancestors",
		"  UZ-V-006  A grader may reason before it scores  [accepted]  ",
		"    The explicit permission: an option a standard states rather than leaves out.",
		"    It stands beside the clause that says graders should not reason, because the",
		"    excepts edge below records the case the permission is for — and it names the",
		"    MUST clause a repository without the option still interoperates through.",
		"  conform/uz-v-001  Check that every claim carries evidence  []  ",
		"    The test body is a script rather than Markdown, so this document is the thin",
		"    frontmatter wrapper that names its path. A conformance test states no status of",
		"    its own: a harness writes it, and whether it runs is what the file says.",
		"  interp/UZ-V-001@2026-08-01  Agreement on the evidence check, August 2026  []  ",
		"    Generated by the standard's own CLI. Nothing here is written by hand, and the",
		"    freshness of the measurement is the file's existence and its git history rather",
		"    than a field inside it.",
		"descendants",
		"  pm-0001  The nightly report cited a run nobody could repeat  [published]  ",
		"    The case the clauses point at as their counterexample: the report was accurate",
		"    and useless, because the run behind it was gone.",
		"  premise/runs-are-reproducible  A run can be repeated from what the report names  [accepted]  ",
		"    The clause that asks for evidence rests on this being true.",
		"  principle/evidence-over-assertion  Evidence over assertion  [accepted]  ",
		"    A standard that cannot be checked is a preference. Clauses point back here",
		"    through rationale, and a principle may carry a counterexample of its own.",
	})
}

// TestContextReadsTheNormativeNeighbourhood is the reading a person or an agent
// needs to make sense of a permission standing beside a prohibition: what the
// clause is about, the exception recorded between the two, the requirement the
// option leans on, and the one line that says the conflict is answered.
func TestContextReadsTheNormativeNeighbourhood(t *testing.T) {
	config := specVaultConfig(t)

	t.Run("the exception, the subject and the interoperation requirement", func(t *testing.T) {
		got := run(t, "context", "UZ-V-006", "--config", config)

		assertExit(t, got, 0)
		assertPrefixes(t, "context", lines(got.stdout), []string{
			"ref",
			"  UZ-V-006  A grader may reason before it scores  [accepted]  ",
			"    The explicit permission: an option a standard states rather than leaves out.",
			"    It stands beside the clause that says graders should not reason, because the",
			"    excepts edge below records the case the permission is for — and it names the",
			"    MUST clause a repository without the option still interoperates through.",
			"related",
			"  topic/inferential-grader  Grading by inference  []  ",
			"    Whether a grader may reason its way to a score rather than match a fixed",
			"    pattern. The subject two clauses of this vault disagree about, on purpose.",
			"  UZ-V-008  A grader should not reason about the answer it grades  [accepted]  ",
			"    The general clause: an inferential grader is one more thing that can be wrong.",
			"    UZ-V-006 is the recorded exception to it, which is what keeps the pair from",
			"    being reported as a conflict every day.",
			"  UZ-V-001  Every claim carries evidence  [accepted]  ",
			"    A report states what it measured and how, so a reader can repeat the run.",
			"suppressed",
			"  suppressed by excepts UZ-V-006 -> UZ-V-008 (scope: only where the run also records a calibration measure)",
		})
		// The specific relation wins over the shared subject: UZ-V-008 is
		// reported as the clause this one excepts, not as one more clause
		// about the same thing.
		if !strings.Contains(got.stdout, "UZ-V-008.md  (excepts)") {
			t.Errorf("stdout = %q, want UZ-V-008 named as the excepted clause", got.stdout)
		}
		if !strings.Contains(got.stdout, "UZ-V-001.md  (interop)") {
			t.Errorf("stdout = %q, want UZ-V-001 named as the interoperation requirement", got.stdout)
		}
	})

	t.Run("the general clause reads the exception from its own side", func(t *testing.T) {
		got := run(t, "context", "UZ-V-008", "--config", config)

		assertExit(t, got, 0)
		if !strings.Contains(got.stdout, "UZ-V-006.md  (excepted by)") {
			t.Errorf("stdout = %q, want UZ-V-006 named as the exception to this clause", got.stdout)
		}
		if !strings.Contains(got.stdout, "suppressed by excepts UZ-V-006 -> UZ-V-008") {
			t.Errorf("stdout = %q, want the suppressed conflict on both sides of the pair", got.stdout)
		}
	})

	t.Run("json carries the relation and the suppression", func(t *testing.T) {
		got := run(t, "context", "UZ-V-006", "--format", "json", "--config", config)

		assertExit(t, got, 0)
		b := decodeJSON[brief.Brief](t, got.stdout)
		if len(b.Related) != 3 {
			t.Fatalf("related = %+v, want the subject, the excepted clause and the interop target", b.Related)
		}
		if b.Related[0].Relation != brief.RelationAbout {
			t.Errorf("first relation = %q, want %q", b.Related[0].Relation, brief.RelationAbout)
		}
		if len(b.Suppressed) != 1 {
			t.Errorf("suppressed = %v, want the one conflict the exception answers", b.Suppressed)
		}
	})

	t.Run("a corpus without the vocabulary reports neither", func(t *testing.T) {
		got := run(t, "context", "0002", "--format", "json", "--dir", fixture(t, "ok-madr"))

		assertExit(t, got, 0)
		b := decodeJSON[brief.Brief](t, got.stdout)
		if len(b.Related) != 0 || len(b.Suppressed) != 0 {
			t.Errorf("brief = %+v, want no normative neighbourhood: the adr preset declares none", b)
		}
	})
}

func TestContextSurfacesRetractedEdges(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-bulk.md":      "---\ntitle: Bulk update\nstatus: accepted\nsupersedes:\n  - \"0002\"\ndate: 2025-02-01\n---\n\n# Bulk update\n",
		"0002-original.md":  "---\ntitle: Original\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Original\n",
		"0003-retract.md":   "---\ntitle: Retract\nstatus: accepted\ndate: 2025-03-01\nretracts:\n  - from: \"0001\"\n    edge: supersedes\n    to: \"0002\"\n---\n\n# Retract\n",
		"0004-unrelated.md": "---\ntitle: Unrelated\nstatus: accepted\ndate: 2025-04-01\n---\n\n# Unrelated\n",
	})
	wantText := "retracted\n  supersedes 0001 -> 0002 (by 0003)\n"
	wantRetraction := brief.Retraction{Edge: config.EdgeSupersedes, From: "0001", To: "0002", By: "0003"}

	for _, id := range []string{"0001", "0002", "0003"} {
		t.Run("text on "+id, func(t *testing.T) {
			got := run(t, "context", id, "--dir", dir)
			assertExit(t, got, 0)
			if !strings.Contains(got.stdout, wantText) {
				t.Errorf("context %s = %q, want %q", id, got.stdout, wantText)
			}
			if strings.Contains(got.stdout, "retracted: ") {
				t.Errorf("context %s = %q, want no redundant 'retracted: ' prefix", id, got.stdout)
			}
		})
	}

	t.Run("markdown on 0001", func(t *testing.T) {
		got := run(t, "context", "0001", "--format", "md", "--dir", dir)
		assertExit(t, got, 0)
		for _, want := range []string{"## Retracted", "- supersedes 0001 -> 0002 (by 0003)"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("context md = %q, want %q", got.stdout, want)
			}
		}
	})

	t.Run("json carries retracted array on involved doc and omits on unrelated doc", func(t *testing.T) {
		got := run(t, "context", "0001", "--format", "json", "--dir", dir)
		assertExit(t, got, 0)
		b := decodeJSON[brief.Brief](t, got.stdout)
		if len(b.Retracted) != 1 || b.Retracted[0] != wantRetraction {
			t.Errorf("Retracted = %+v, want [%+v]", b.Retracted, wantRetraction)
		}

		unrelated := run(t, "context", "0004", "--format", "json", "--dir", dir)
		assertExit(t, unrelated, 0)
		bu := decodeJSON[brief.Brief](t, unrelated.stdout)
		if len(bu.Retracted) != 0 {
			t.Errorf("unrelated Retracted = %+v, want empty", bu.Retracted)
		}
		if strings.Contains(unrelated.stdout, `"retracted"`) {
			t.Errorf("unrelated json = %q, want retracted omitted", unrelated.stdout)
		}
	})
}

func TestContextJapaneseHeadingAndExplicitMissingSection(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-direct.md": "---\ntitle: Direct decision\nstatus: accepted\ndate: 2026-09-01\n---\n\n# 0001\n\n## 決定\n\n直接の決定本文。\n",
		"0002-child.md":  "---\ntitle: Child heading decision\nstatus: accepted\ndate: 2026-09-02\ndepends-on:\n  - \"0001\"\n---\n\n# 0002\n\n## ステータス\n\nAccepted\n\n## 決定\n\n### D1. 辺仕様に target を追加する\n\n```yaml\nedges: []\n```\n",
	})

	got := run(t, "context", "0002", "--dir", dir)
	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "D1. 辺仕様に target を追加する") {
		t.Errorf("stdout = %q, want child heading auto-excerpt", got.stdout)
	}
	if !strings.Contains(got.stdout, "直接の決定本文。") {
		t.Errorf("stdout = %q, want Japanese direct decision excerpt on dependency", got.stdout)
	}

	missing := run(t, "context", "0002", "--section", "Missing", "--format", "json", "--dir", dir)
	assertExit(t, missing, 0)
	b := decodeJSON[brief.Brief](t, missing.stdout)
	if b.Ref.Excerpt != "" {
		t.Errorf("explicit missing section excerpt = %q, want empty", b.Ref.Excerpt)
	}
}

func TestContextAndCommandsResolveByManagedFilePath(t *testing.T) {
	t.Run("adr corpus by file path", func(t *testing.T) {
		root := writeDocs(t, map[string]string{
			filepath.Join("docs", "adr", "0001-first.md"):                              "---\ntitle: First\nstatus: superseded\ndate: 2025-01-01\n---\n\n# First\n\n## Decision\n\nFirst decision.\n",
			filepath.Join("docs", "adr", "0002-store-thumbnails-on-the-local-disk.md"): "---\ntitle: Store thumbnails on the local disk\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-02-01\n---\n\n# Store thumbnails\n\n## Decision\n\nStore on local disk.\n",
		})
		t.Chdir(root)

		relPath := filepath.Join("docs", "adr", "0002-store-thumbnails-on-the-local-disk.md")
		absPath := filepath.Join(root, relPath)
		for _, ref := range []string{relPath, "./" + filepath.ToSlash(relPath), absPath} {
			got := run(t, "context", ref)
			assertExit(t, got, 0)
			if !strings.Contains(got.stdout, "0002  Store thumbnails on the local disk") {
				t.Errorf("context(%q) = %q, want resolved 0002", ref, got.stdout)
			}

			res := run(t, "resolve", filepath.Join("docs", "adr", "0001-first.md"))
			assertExit(t, res, 0)
			assertLines(t, "resolve by path", lines(res.stdout), []string{"0002"})

			q := run(t, "query", ref, "--descendants")
			assertExit(t, q, 0)
			assertLines(t, "query by path", lines(q.stdout), []string{"0001"})
		}
	})

	t.Run("spec corpus by file path across context, resolve, and query", func(t *testing.T) {
		root := copyFixtureTree(t, "spec-vault")
		t.Chdir(root)

		for _, tc := range []struct {
			name string
			args []string
			want string
		}{
			{
				name: "context by relative clause path",
				args: []string{"context", "spec/clauses/UZ-V-001.md"},
				want: "UZ-V-001  Every claim carries evidence",
			},
			{
				name: "context by conform wrapper path whose frontmatter id has slash",
				args: []string{"context", "spec/conform/uz-v-001.md"},
				want: "conform/uz-v-001  Check that every claim carries evidence",
			},
			{
				name: "resolve by relative clause path",
				args: []string{"resolve", "spec/clauses/UZ-V-001.md"},
				want: "UZ-V-001",
			},
			{
				name: "query by relative clause path",
				args: []string{"query", "spec/clauses/UZ-V-001.md", "--ancestors"},
				want: "UZ-V-006",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got := run(t, tc.args...)
				assertExit(t, got, 0)
				if !strings.Contains(got.stdout, tc.want) {
					t.Errorf("stdout = %q, want %q", got.stdout, tc.want)
				}
			})
		}

		for _, invalid := range []string{"uz-v-002", "not-a-reference"} {
			got := run(t, "context", invalid)
			assertExit(t, got, 1)
			if !strings.Contains(got.stderr, "unrecognized reference") {
				t.Errorf("stderr(%q) = %q, want unrecognized reference", invalid, got.stderr)
			}
		}
	})
}

func TestActionableErrorHintsAndHelpExamples(t *testing.T) {
	dir := fixture(t, "ok-madr")

	unknownDoc := run(t, "context", "0099", "--dir", dir)
	assertExit(t, unknownDoc, 1)
	if !strings.Contains(unknownDoc.stderr, "docdag query --binding --fields id,title") {
		t.Errorf("stderr = %q, want actionable query hint", unknownDoc.stderr)
	}

	badRef := run(t, "context", "not-a-reference", "--dir", dir)
	assertExit(t, badRef, 1)
	if !strings.Contains(badRef.stderr, "docdag query --binding --fields id,title") {
		t.Errorf("stderr = %q, want actionable query hint", badRef.stderr)
	}

	for _, cmdArgs := range [][]string{
		{"context", "0001", "--edge", "relates-to"},
		{"query", "0001", "--edge", "relates-to"},
		{"export", "--edge", "relates-to"},
	} {
		got := run(t, append(cmdArgs, "--dir", dir)...)
		assertExit(t, got, 2)
		if !strings.Contains(got.stderr, "allowed: depends-on, supersedes") {
			t.Errorf("%v stderr = %q, want allowed edge types", cmdArgs, got.stderr)
		}
	}

	queryHelp := run(t, "query", "--help")
	assertExit(t, queryHelp, 0)
	if !strings.Contains(queryHelp.stdout, "--binding") {
		t.Errorf("query --help = %q, want --binding documented prominently", queryHelp.stdout)
	}

	for _, sub := range [][]string{
		{"--help"},
		{"context", "--help"},
		{"query", "--help"},
		{"resolve", "--help"},
		{"validate", "--help"},
		{"lint", "--help"},
		{"new", "--help"},
		{"diff", "--help"},
		{"stats", "--help"},
		{"export", "--help"},
	} {
		got := run(t, sub...)
		assertExit(t, got, 0)
		if !strings.Contains(got.stdout, "Examples:") {
			t.Errorf("%v help output missing Examples section:\n%s", sub, got.stdout)
		}
	}
}
