# DocDag

[![CI](https://github.com/Kaikei-e/DocDag/actions/workflows/ci.yml/badge.svg)](https://github.com/Kaikei-e/DocDag/actions/workflows/ci.yml)
[![Release](https://github.com/Kaikei-e/DocDag/actions/workflows/release.yml/badge.svg)](https://github.com/Kaikei-e/DocDag/actions/workflows/release.yml)

DocDag reads a directory of Markdown documents with YAML frontmatter, extracts a **typed directed
graph** from it, enforces DAG invariants on that graph, and answers queries about it. It ships one
preset, `adr`, for Architecture Decision Records.

## The model

DocDag keeps two layers apart:

- **Constraint layer** — typed edges declared in frontmatter (`supersedes:`, `depends-on:`) plus
  edges derived from configured field patterns. Only these carry invariants: acyclicity and rules.
- **Reference layer** — untyped links found in bodies: `[[wikilink]]`, `[[wikilink|alias]]` and
  relative Markdown links to other managed documents. A link joins the layer only when its whole
  target is identifier-shaped, or, for a Markdown link, names a managed file; a link inside a fenced
  code block or an inline code span is an example and is skipped. Surfaced by `--include-refs` and
  `stats`; never part of a constraint, and unvalidated unless `references.dangling` asks for it.

A document's identity is its digit run, so `339`, `ADR-339`, `000339` and `0339-use-postgres.md` all
name the same node, displayed zero-padded to `id_width`. Renaming a file's title suffix does not
change identity; two files that normalize to the same identifier are an `id_collision` error.

**Status is a projection of the graph, not an independent fact.** With the ADR preset, an inbound
`supersedes` edge and a status other than `superseded` is an error (`status_drift`); `superseded`
with nothing superseding it is a warning (`superseded_orphan`). A document is *binding* when its
status is `accepted` and no document supersedes it.

## Why

Decision records rot in ways review does not catch: a decision superseded twice with the status
never updated, a supersession cycle, a `supersedes: 0042` pointing at a file nobody wrote. Those are
graph properties, so a graph check can enforce them, and `docdag validate` exits 1 on any error, in
one CI line. An existing MADR repository needs no edits first: `status: superseded by 0003` becomes
a derived `supersedes` edge, so the invariants hold as the files already are.

## Install

In CI, use the composite action — see [Continuous integration](#continuous-integration). Locally,
install a pinned version:

```sh
go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.2.0
```

This installs `docdag` into `$(go env GOPATH)/bin`. Prebuilt binaries for tagged versions, and the
`checksums.txt` that covers them, are attached to the repository's Releases page.

## Quickstart

No configuration is needed. From the root of a repository whose decisions live in `docs/decisions`:

```console
$ docdag validate
docs/decisions/0002-store-thumbnails-on-the-local-disk.md:3: WARN unstructured_supersedes 0002: supersedes edge 0003 -> 0002 comes from a field value; declare it in frontmatter
OK: 4 docs, 3 typed edges, no cycles

$ docdag resolve 0002          # what replaced this decision?
0003

$ docdag query --binding       # what is in force right now?
0001
0003

$ docdag query 0001 --ancestors --edge depends-on   # what rests on this decision?
0003
0004
```

DocDag looks in `docs/adr`, `doc/adr`, `docs/decisions`, `docs/ADR`, `adr` — the first that exists
and holds a file named `NNNN.md` or `NNNN-kebab-title.md`, 3 to 6 digits; `--dir` overrides it.

The corpus above ships in this repository as `testdata/fixtures/ok-madr`, next to one corpus per
failure mode. From a checkout, `docdag validate --dir testdata/fixtures/status-drift` prints
`testdata/fixtures/status-drift/0001-serve-images-from-the-application-server.md:3: ERROR
status_drift 0001: has inbound supersedes but status is not superseded` and exits 1.

Every finding names a file and, wherever a frontmatter key carries the fault, the line of that key:

```
<path>:<line>: <SEVERITY> <rule> <id>: <detail>
```

`:<line>` is dropped when the position is unknown. Findings sort by severity, path, line, rule and
identifier, so a report reads in file order and diffs cleanly. Besides the configured rules,
`validate` reports `cardinality`, `cycle`, `dangling_ref`, `dangling_reference`, `derived_conflict`,
`empty_edge`, `id_collision`, `immutable_violation`, `invalid_frontmatter`, `invalid_ref`,
`inverse_mismatch`, `missing_frontmatter`, `unknown_status` and `unstructured_supersedes`.

## Commands

Global flags: `--dir <path>`, `--config <path>` and `--format text|json`, which every command
answers in; `validate` also answers in `github` and `rdjson`, `context` in `md`, and `export`
replaces the format flag with its own `mermaid|dot|json`.

| Command | What it prints | Notable failures |
| --- | --- | --- |
| `docdag validate [--touching <path>]... [--format text\|json\|github\|rdjson]` | one line per finding, each followed by an indented `fix:` where there is a remedy, then `OK: N docs, M typed edges, no cycles` | exit 1 if any finding is an error |
| `docdag resolve <ref> [--fields <list>]` | the current successor(s) of a reference, one per line, or the document itself when nothing supersedes it | exit 1 on an unknown reference or a supersedes cycle |
| `docdag query <ref> [--ancestors\|--descendants] [--edge <type>] [--include-refs] [--fields <list>]` | the reachable set over typed edges, descendants by default; reference-layer hits are suffixed ` (reference)` | exit 1 unknown reference, exit 2 unknown edge type or conflicting flags |
| `docdag query --binding [--fields <list>]` | every binding document | exit 2 if combined with a walk flag |
| `docdag context <ref> [--depth N] [--edge <type>]... [--section <heading>] [--budget N] [--all]` | the document, what it resolves to and its neighbourhood, each quoting one section | exit 1 unknown reference, exit 2 unknown edge type |
| `docdag export [--format mermaid\|dot\|json] [--include-refs] [--connected] [--edge <type>]... [--out PATH]` | the typed graph; mermaid on stdout by default, `-` also means stdout | exit 2 on an unknown edge type, exit 3 if the output file cannot be written |
| `docdag stats` | document count, binding count, orphan rate, edge count per type, supersedes chain-depth distribution, top-10 reference in-degree | — |
| `docdag new <title> [--id <ref>] [--supersedes <ref>]... [--depends-on <ref>]... [--dry-run]` | the path of the created document, or the plan under `--dry-run` | exit 1 on an unknown reference or a claimed identifier, exit 3 on a write error |

`docdag export --edge <type>` keeps only the named edge types, repeat it to keep several, and
`--connected` drops every document no remaining typed edge touches — the two compose, so
`--connected --edge supersedes` draws the supersession chains alone.

`docdag new` takes the next free identifier, writes `<id>-<kebab-title>.md` from the template with
`status: proposed` and today's date, and rewrites **only** the `status:` value of each superseded
document: bodies and line endings stay byte-identical, and every rewrite is computed before any file
is touched. The name comes from the `filename:` template, so a corpus of bare `NNNNNN.md` files
configures `filename: "{id}.md"` and keeps that shape.

`--dry-run` runs every computation, writes nothing and prints the plan, as `create <id> <path>`
followed by one `rewrite <path> status: superseded` line each, or as JSON under `--format json`. It
exits exactly as the real run would, so it is a safe way to see what a creation would cost.

`--id <ref>` creates the document under a chosen identifier instead of the next free one. If that
identifier already names a document with the same title, `new` prints its path and writes nothing,
so re-running an agent's command is harmless; a different title under that identifier is an error.
`new` also refuses to run at all while the corpus carries an `id_collision`.

Exit codes: `0` success (warnings allowed), `1` domain failure, `2` usage error, `3` I/O or config
error — including "no documents directory found", so a repository without one needs `--dir`.

## validate output

A finding with a mechanical remedy carries one, indented under the finding in `text` and as a `fix`
key in `json` and `rdjson`:

```
docs/decisions/0002-ship-logs.md:4: ERROR dangling_ref 0002: supersedes reference "0009" does not name a document
  fix: did you mean 0001?
```

`--format json` writes one object, versioned so a consumer can tell a shape change from a content
change. `location` is the primary position and `related` names the other files a finding involves —
the peers of a collision, the rest of a cycle:

```json
{
  "schema_version": 1,
  "findings": [
    {
      "severity": "error",
      "rule": "status_drift",
      "id": "0001",
      "detail": "has inbound supersedes but status is not superseded",
      "location": { "path": "docs/decisions/0001-serve-images-from-the-app.md", "line": 3 }
    }
  ],
  "summary": { "documents": 2, "edges": 1, "errors": 1, "warnings": 0, "cycles": 0 }
}
```

`--format github` writes one GitHub Actions workflow command per finding, followed by the same
summary line as `text`:

```
::error file=docs/decisions/0001-serve-images-from-the-app.md,line=3,title=status_drift::0001: has inbound supersedes but status is not superseded
```

A workflow step renders at most ten annotations, so a corpus with more findings than that needs a
second `--format text` run written into `$GITHUB_STEP_SUMMARY` to show the rest.

`--format rdjson` writes the [reviewdog](https://github.com/reviewdog/reviewdog) diagnostic format
as a single `DiagnosticResult`, for `reviewdog -f=rdjson`. It carries no summary line.

## docdag.yaml

Optional, read from the repository root or from `--config <path>`, merged over the preset; flags win
over both. The file below is the ADR preset in full — what DocDag applies with no configuration:

```yaml
preset: adr
dir: docs/decisions            # default: discovered, see above
id_width: 4
status_field: status
status_values: [proposed, accepted, rejected, deprecated, superseded, withdrawn]
filename: "{id}-{slug}.md"     # what `docdag new` names a document

edges:
  - name: supersedes
    key: supersedes            # frontmatter key holding the references
    acyclic: true              # subject to the cycle check
    direction: forward         # the containing document is the edge source
  - name: depends-on
    key: depends-on
    acyclic: true
    direction: forward

derived_edges:
  - field: status
    pattern: '(?i)^superseded[\s-]+by[\s-]+(\S+)'   # capture group 1 is the reference
    edge: supersedes
    direction: reverse         # the referenced document is the edge source

rules:
  - name: status_drift
    severity: error
    when:
      inbound: supersedes
      attr:
        status: { not: superseded }
    message: "has inbound supersedes but status is not superseded"
  - name: superseded_orphan
    severity: warn
    when:
      not_inbound: supersedes
      attr:
        status: { eq: superseded }
    message: "status is superseded but no document supersedes it"
```

Every other key is optional, and off unless the file says otherwise:

```yaml
acyclic_union: true            # also check for cycles over the union of the acyclic edge types

references:                    # reference-layer validation; without it the layer is unvalidated
  dangling: error              # off (default) | warn | error
  pattern: '^(?i)(?:adr-?)?(\d{3,6})$'   # the default: what an identifier-shaped target looks like
  scan: [body, frontmatter]    # default [body]; frontmatter scans string scalars and list items

structural:                    # raise a built-in check; lowering one is a configuration error
  missing_frontmatter: error

edges:
  - name: amends
    key: amends
    acyclic: true
    direction: forward
    inverse: amended_by        # the target must list the source here; the key declares no edges
    max_inbound: 1             # 0, the default, is unbounded
    max_outbound: 0
    min_outbound: 1

rules:
  - name: unexplained_retirement
    severity: warn
    when:
      not_inbound: supersedes
      any_of:                                    # holds when any alternative holds
        - attr: { status: { eq: deprecated } }
        - attr: { status: { eq: rejected } }
      not:                                       # holds when this condition does not
        attr: { tags: { contains: explained } }
    message: "is retired, nothing supersedes it, and it carries no explained tag"
```

The `edges:` and `rules:` lists above show where the new keys go; writing either one replaces the
preset's list rather than adding to it.

A rule's `when` block ANDs its top-level clauses. The vocabulary is fixed and complete: `inbound`,
`not_inbound`, `outbound`, `not_outbound` — each naming a declared edge type — `attr: {<key>:
{eq|not: <value>}}` on a scalar, `attr: {<key>: {contains|not_contains: <value>}}` and
`attr: {<key>: {subset_of: [<value>, …]}}` on a list, and the two combinators `any_of: [<condition>,
…]` and `not: <condition>`, which nest. A scalar read as a list is a one-element list; comparison is
case-insensitive; a positive clause needs the attribute to be there and a negative one is satisfied
by its absence. There is no expression language. Set `template: <path>` to replace the document
template `docdag new` uses, and `filename:` to change what it names the result — the template must
carry `{id}`, may carry `{slug}`, and may not carry a path separator.

Structural checks are not rules. `structural:` may raise one — `missing_frontmatter` and
`unstructured_supersedes` are the two that warn by default — but lowering one, or naming a check
that does not exist, is a configuration error (exit 3), and no check can be disabled.

## Append-only history

`docdag validate --immutable-since <rev>` treats a decision that was `accepted`, `superseded` or
`withdrawn` at `<rev>` as a record rather than a draft. It compares the working tree against
`git merge-base <rev> HEAD` and allows exactly three kinds of change to such a document:

- the value of the `status_field`,
- entries **added** under a configured `inverse:` key, so a later decision can record that it
  replaced this one,
- lines appended to the end of the body.

Anything else — another frontmatter key changed, added or removed; an inverse entry removed; a line
of the existing body rewritten; the file deleted — is an `immutable_violation` error naming what
changed:

```console
$ docdag validate --immutable-since origin/main
docs/adr/0001-serve-images-from-the-application.md:11: ERROR immutable_violation 0001: the body changed at line 11, which append-only history forbids
```

A document that `<rev>` did not hold is new and is always allowed. The check runs `git` from `PATH`
and is off unless the flag is given; a corpus outside a git repository, or a machine without `git`,
exits 3.

## Continuous integration

The composite action downloads the release binary for the runner, verifies it against the release's
`checksums.txt` before running it, and needs no Go toolchain:

```yaml
name: decisions
on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.2.0
        with:
          version: v0.2.0                  # default: latest
          args: validate --format github   # this is the default
          working-directory: .             # this is the default
```

A tag can be moved, so pin the action by commit SHA when the supply chain matters —
`uses: Kaikei-e/DocDag@<40-char-sha> # v0.2.0`. This repository pins the actions it uses that way.

The action runs on Linux and macOS runners, amd64 and arm64. On a Windows runner it fails with a
diagnostic rather than passing silently; install with `go install` there instead.

A [pre-commit](https://pre-commit.com) hook is also shipped:

```yaml
repos:
  - repo: https://github.com/Kaikei-e/DocDag
    rev: v0.2.0
    hooks:
      - id: docdag-validate
```

Hooks are advisory: `git commit --no-verify` walks straight past them, and a contributor who never
ran `pre-commit install` never had them. CI is the gate; the hook only shortens the loop.

## MADR compatibility

A conventional MADR repository needs no changes:

- Filenames `NNNN-kebab-title.md` and bare `NNNNNN.md` are both recognized, 3 to 6 digits.
- `status: superseded by 0003` derives the `supersedes` edge with `0003` as the newer document and
  makes the containing one count as superseded. Each derived edge raises an
  `unstructured_supersedes` warning — a suggestion to declare the edge in frontmatter, not a failure.
  Moving the string to a `supersedes:` key clears it; the graph is the same either way.
- Status comparison is case-insensitive; a value outside the vocabulary is `unknown_status`.
  `withdrawn` is in the vocabulary for a proposal that was dropped rather than replaced: it binds
  nothing, and because nothing supersedes it, it raises no `superseded_orphan` warning either.
- Unrecognized frontmatter keys are ignored, so other tooling's fields are safe.
- A file without frontmatter is skipped, or warned about (`missing_frontmatter`) if its name matches.
- An edge key written down and then left empty — `supersedes:` with nothing under it, `[]`, or a list
  of blank items — is `empty_edge`, because it reads as a declared relation and builds none.
- A `supersedes:` entry that is not an identifier (`see 0042`, a sentence, a slug) is `invalid_ref`;
  one that is an identifier the corpus does not hold is `dangling_ref`.
- Body links stay in the reference layer, so prose cannot fail a build unless `references.dangling`
  opts in — and even then `[[upstream]]`, `[[3days-recap]]` and a link inside a code fence are not
  references at all.

## For agents

An agent should ask the graph rather than read the directory: one command replaces a fan-out of file
reads, and every answer carries the identity, the status and the position of the documents it names.

`docdag context <ref>` is the entry point. It prints the document, the successor it resolves to if it
is superseded, and its neighbourhood over typed edges, each entry followed by the first paragraph of
its `Decision` section, verbatim and unsummarised:

```console
$ docdag context 0002
ref
  0002  Store thumbnails on the local disk  [superseded]  docs/decisions/0002-store-thumbnails-on-the-local-disk.md
    Chosen option: a directory on the local disk, sharded by the first two characters
    of the cache key.

resolves to (0002 is superseded)
  0003  Store thumbnails in object storage  [accepted]  docs/decisions/0003-store-thumbnails-in-object-storage.md
    Chosen option: object storage behind a read-through layer, keeping the
    content-addressed key from the caching decision unchanged.
```

`--depth N` widens the walk from its default of one hop, `--edge <type>` narrows it to one relation
and repeats, and `--all` adds the documents that are not binding, which are left out by default.
`--section <heading>` quotes a different section: the value matches an H2 or H3 whose text starts
with it, so `Decision` finds MADR's `Decision Outcome` in preference to its `Decision Drivers`.
`--budget N` caps the prose at roughly N tokens, counted as four characters each and defaulting to
2000; entries the budget cannot afford degrade to their one-line form rather than being cut
mid-sentence, and the report ends with a line counting them. `--format md` answers with a Markdown
document, `--format json` with `schema_version`, `ref`, `resolves_to`, `ancestors`, `descendants`
and `budget`.

**Columns.** `resolve` and `query` take `--fields id,title,status,path` and print those columns, tab
separated, in the order asked for; the default is `id` alone, so a pipeline reading identifiers is
unaffected. Under `--format json` these commands answer with an array of objects carrying every
field, plus `"reference": true` on a reference-layer hit — where v0.1 answered with an array of
identifiers.

```console
$ docdag query --binding --fields id,title,status
0001	Cache rendered thumbnails	accepted
0003	Store thumbnails in object storage	accepted
```

**One edit at a time.** `docdag validate --touching <path>` runs the whole corpus and reports only
the findings about that file, the files a finding relates to it, and the documents one typed edge
away — what changing it can break. The flag repeats and accepts a directory. The exit code still
answers for the corpus, so a narrowed report never turns a failing repository into a passing one,
and the number of findings withheld goes to stderr.

**The plugin.** This repository is a Claude Code plugin. It installs a skill teaching the
vocabulary above and which command answers which question, plus a `PostToolUse` hook that runs
`docdag validate --touching` on every Markdown file written inside the documents directory and
reports back when that document is what broke:

```console
$ claude
/plugin marketplace add Kaikei-e/DocDag
/plugin install docdag@docdag
```

The hook needs `docdag` and `jq` on `PATH` and does nothing without them. Allow the commands once
with `"permissions": {"allow": ["Bash(docdag *)"]}` in `.claude/settings.json`.

## License

Apache License 2.0. See [LICENSE](LICENSE).
