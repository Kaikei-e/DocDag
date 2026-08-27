# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-08-27

v0.2.0 turns `docdag validate` from a pass/fail line into a check whose findings a reviewer, a CI
annotation and an agent can each act on, and extends the constraint layer far enough that a decision
log's own conventions — reciprocal edges, append-only history, a bounded vocabulary — are enforced
rather than described. It is a **breaking release**: the text and JSON output formats change, and a
few things that used to pass now fail. The migration section at the end lists every one.

### Every finding now has a place

- Findings carry `path`, `line` and, where it applies, `column`, taken from the YAML frontmatter
  itself. The text line is `path:line: SEVERITY rule id: detail`, so an editor or a problem matcher
  can jump to it. Findings that involve several files (`cycle`, `id_collision`, `inverse_mismatch`)
  list the others under `related`.
- `--format github` prints one workflow command per finding —
  `::error file=…,line=…,title=status_drift::…` — so a pull request shows the problem inline. GitHub
  caps inline annotations at ten per step; pair it with the text report in the step summary.
- `--format rdjson` emits reviewdog's Diagnostic Format for review comments that escape that cap.
- Findings that have a mechanical remedy say so: `fix: did you mean 0042?`,
  `fix: set status: superseded in docs/adr/0001-….md`. The text report prints it indented under the
  finding; JSON carries it as `fix`.
- The JSON report is versioned (`"schema_version": 1`).

### A larger constraint vocabulary, still without an expression language

- **`inverse:`** on an edge declares the frontmatter key the *target* must carry back
  (`amends` ↔ `amended_by`). Both directions are checked pairwise; a missing or extra entry is
  `inverse_mismatch`. The inverse key creates no edges of its own.
- **`references:`** opts the reference layer into validation. `dangling: warn|error` reports a body
  or frontmatter link that names no document; `pattern` says what an identifier-shaped target looks
  like, so `[[upstream]]`, `[[3days-recap]]` or a footnote `[[1]]` are never references;
  `scan: [body, frontmatter]` extends the walk to the wikilinks in frontmatter scalars and list
  items. Off by default: prose still cannot fail a build unless a configuration says so.
- **Cardinality** — `max_inbound`, `max_outbound`, `min_outbound` — bounds the degree of an edge
  type. The bounds are opt-in and default to unbounded; an edge type that declares `max_inbound: 1`
  reports a second document superseding the same one as a `cardinality` error.
- **`acyclic_union: true`** additionally checks for cycles across the union of the acyclic edge
  types, so `A amends B, B supersedes A` is caught.
- **List attributes**: rules can read `tags:`-style lists with `contains`, `not_contains` and
  `subset_of`; `eq`/`not` stay scalar.
- **`any_of:`** and **`not:`** nest inside `when:`, so a rule can express an alternative and a
  negation without an expression language. The vocabulary is still fixed and complete; the README
  restates it.
- **`structural:`** raises the severity of a built-in check (`missing_frontmatter: error`). Lowering
  one, or naming a check that does not exist, is a configuration error.
- **`empty_edge`**: an edge key written down and left empty (`supersedes:` with nothing under it,
  `[]`, or blank items) is an error instead of a silently absent edge.
- **`withdrawn`** joins the preset status vocabulary for a decision that was dropped rather than
  replaced. It binds nothing and raises no `superseded_orphan`.
- An explicitly empty list in `docdag.yaml` (`derived_edges: []`) now clears the preset's list, as
  the README always said it did.

### Append-only history

`docdag validate --immutable-since <rev>` treats a document that was `accepted`, `superseded` or
`withdrawn` at `<rev>` as a record. Against `git merge-base <rev> HEAD`, such a document may only
change its status value, gain entries under a configured `inverse:` key, or grow lines at the end of
its body; anything else — a rewritten paragraph, another frontmatter key, a deletion or rename — is
an `immutable_violation` naming what changed. It execs `git` from `PATH` and needs full history in
CI (`fetch-depth: 0`); a corpus outside a git repository, or a machine without `git`, exits 3. Off
unless the flag is given.

### For agents

- **`docdag context <ref>`** answers "what is decided around this?" in one call: the document, what
  it resolves to if superseded, and its typed-edge neighbourhood, each entry followed by the first
  paragraph of its `Decision` section, verbatim. `--budget` caps the prose in tokens and degrades
  whole entries rather than cutting mid-sentence; `--format md|json` for prompts and pipelines.
- **`--fields id,title,status,path`** on `resolve` and `query` prints those columns tab-separated.
  Under `--format json` these commands now return objects, not bare identifiers.
- **`validate --touching <path>…`** runs the whole corpus and reports only the findings about the
  given files and their typed-edge neighbours. The exit code and the summary still answer for the
  corpus, and the number of findings withheld goes to stderr.
- **The repository is a Claude Code plugin**: a skill teaching the vocabulary and a `PostToolUse`
  hook that runs `docdag validate --touching` on every Markdown file written under the documents
  directory. `/plugin marketplace add Kaikei-e/DocDag`, then `/plugin install docdag@docdag`.

### `new`, `export`, distribution

- **`filename:`** in `docdag.yaml` (`"{id}.md"` for bare-numeric corpora) chooses what `docdag new`
  names a document; the default `{id}-{slug}.md` is unchanged.
- **`new --dry-run`** prints the plan — identifier, path, the rewrites it would apply — as text or
  JSON and writes nothing. **`new --id <ref>`** creates under a chosen identifier and is idempotent:
  same title, exit 0 and no write; different title, exit 1.
- **`export --connected`** drops documents no typed edge touches, and **`--edge <type>`**
  (repeatable) keeps only the named types; together they draw the supersession chains alone.
- **Composite action**: `uses: Kaikei-e/DocDag@v0.2.0` downloads the release binary for the runner,
  verifies it against `checksums.txt` and runs `validate --format github`. Linux and macOS, amd64
  and arm64; Windows fails with a diagnostic rather than passing silently. No Go toolchain, no
  third-party actions.
- **pre-commit**: `.pre-commit-hooks.yaml` ships `docdag-validate` (`pass_filenames: false`, since a
  graph is validated whole). Hooks are advisory; CI is the gate.
- `make build` works again, and `make check` builds the binary.

### Fixed

- Wikilink targets are taken as references only when the whole target is identifier-shaped. A
  footnote `[[3]](url)` or a note titled `[[3days-recap]]` no longer becomes a phantom edge to
  document 0003, and links inside fenced code blocks and inline code spans are ignored.
- A wikilinked identifier inside a frontmatter edge key (`supersedes: ["[[0042]]"]`) is unwrapped; a
  value that is not an identifier at all (`see 0042`) is `invalid_ref`.
- The two `dangling_ref` wordings are one wording; `invalid_frontmatter` reports the file line rather
  than the block-relative one.
- History findings are answered relative to where the caller stands even when the repository root
  sits behind a symlink or a Windows short name.

### Migrating from 0.1.0

- **Text report**: finding lines gained a `path:line:` prefix. Anything that grepped for `^ERROR ` or
  `^WARN ` must match on ` ERROR ` / ` WARN ` or read `--format json`.
- **JSON**: `validate` output gained `schema_version`, `location`, `related`, `fix`; `resolve` and
  `query` return `[{id,title,status,path,reference?}]` instead of `["0001", …]`. Detail wordings for
  `dangling_ref`, `missing_frontmatter`, `id_collision` and `invalid_frontmatter` changed.
- **Now fails where it used to pass**: an empty edge key (`empty_edge`); a frontmatter reference that
  is not an identifier (`invalid_ref`); with `references.dangling` set, a body link to a missing
  document; with `inverse:` set, an unmirrored edge; with a cardinality bound set, an edge that
  exceeds it.
- **`derived_edges: []`** now really clears the preset's derived edges. A repository that relied on
  it being ignored will lose the `status: superseded by NNNN` derivation — declare `supersedes:` in
  frontmatter instead.
- **`withdrawn`** is a valid status; a corpus that used the word as an unknown value stops raising
  `unknown_status`.
- **`new`** paths in output are relative to the working directory.
- **Configuration**: every new key is optional and off; an unchanged `docdag.yaml` behaves as before
  apart from the items above. Pin the action or
  `go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.2.0`.

87 commits, 119 files, +10,394 / −503. Test suite 155 → 315 test functions. No new dependencies.

## [0.1.0] - 2026-07-31

First release. See the [GitHub release](https://github.com/Kaikei-e/DocDag/releases/tag/v0.1.0).

[0.2.0]: https://github.com/Kaikei-e/DocDag/releases/tag/v0.2.0
[0.1.0]: https://github.com/Kaikei-e/DocDag/releases/tag/v0.1.0
