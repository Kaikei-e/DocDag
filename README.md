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
  relative Markdown links to other managed documents. Surfaced by `--include-refs` and `stats`;
  never validated, never part of a constraint.

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
`validate` reports `cycle`, `dangling_ref`, `id_collision`, `invalid_frontmatter`,
`missing_frontmatter`, `unknown_status`, `derived_conflict` and `unstructured_supersedes`.

## Commands

Global flags: `--dir <path>`, `--config <path>` and `--format text|json`, which every command
answers in; `validate` also answers in `github` and `rdjson`, and `export` replaces the format flag
with its own `mermaid|dot|json`.

| Command | What it prints | Notable failures |
| --- | --- | --- |
| `docdag validate [--format text\|json\|github\|rdjson]` | one line per finding, then `OK: N docs, M typed edges, no cycles` | exit 1 if any finding is an error |
| `docdag resolve <ref>` | the current successor(s) of a reference, one per line, or the document itself when nothing supersedes it | exit 1 on an unknown reference or a supersedes cycle |
| `docdag query <ref> [--ancestors\|--descendants] [--edge <type>] [--include-refs]` | the reachable set over typed edges, descendants by default; reference-layer hits are suffixed ` (reference)` | exit 1 unknown reference, exit 2 unknown edge type or conflicting flags |
| `docdag query --binding` | every binding document | exit 2 if combined with a walk flag |
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
status_values: [proposed, accepted, rejected, deprecated, superseded]
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

A rule's `when` block ANDs its conditions. The vocabulary is fixed and complete: `inbound`,
`not_inbound`, `outbound`, `not_outbound` — each naming a declared edge type — and
`attr: {<key>: {eq|not: <value>}}`. There is no expression language. Set `template: <path>` to
replace the document template `docdag new` uses, and `filename:` to change what it names the
result — the template must carry `{id}`, may carry `{slug}`, and may not carry a path separator.
Structural checks are not rules and cannot be disabled.

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
- Unrecognized frontmatter keys are ignored, so other tooling's fields are safe.
- A file without frontmatter is skipped, or warned about (`missing_frontmatter`) if its name matches.
- Body links stay in the reference layer, so prose can never fail a build.

## License

Apache License 2.0. See [LICENSE](LICENSE).
