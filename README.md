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

```sh
go install github.com/Kaikei-e/DocDag/cmd/docdag@latest
```

This installs `docdag` into `$(go env GOPATH)/bin`. Prebuilt binaries for tagged versions are
attached to the repository's Releases page.

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
answers in; `validate` also answers in `github` and `rdjson`, `context` in `md`, and `export`
replaces the format flag with its own `mermaid|dot|json`.

| Command | What it prints | Notable failures |
| --- | --- | --- |
| `docdag validate [--touching <path>]... [--format text\|json\|github\|rdjson]` | one line per finding, each followed by an indented `fix:` where there is a remedy, then `OK: N docs, M typed edges, no cycles` | exit 1 if any finding is an error |
| `docdag resolve <ref> [--fields <list>]` | the current successor(s) of a reference, one per line, or the document itself when nothing supersedes it | exit 1 on an unknown reference or a supersedes cycle |
| `docdag query <ref> [--ancestors\|--descendants] [--edge <type>] [--include-refs] [--fields <list>]` | the reachable set over typed edges, descendants by default; reference-layer hits are suffixed ` (reference)` | exit 1 unknown reference, exit 2 unknown edge type or conflicting flags |
| `docdag query --binding [--fields <list>]` | every binding document | exit 2 if combined with a walk flag |
| `docdag context <ref> [--depth N] [--edge <type>]... [--section <heading>] [--budget N] [--all]` | the document, what it resolves to and its neighbourhood, each quoting one section | exit 1 unknown reference, exit 2 unknown edge type |
| `docdag export [--format mermaid\|dot\|json] [--include-refs] [--out PATH]` | the typed graph; mermaid on stdout by default, `-` also means stdout | exit 3 if the output file cannot be written |
| `docdag stats` | document count, binding count, orphan rate, edge count per type, supersedes chain-depth distribution, top-10 reference in-degree | — |
| `docdag new <title> [--supersedes <ref>]... [--depends-on <ref>]...` | the path of the created document | exit 1 on an unknown reference, exit 3 on a write error |

`docdag new` takes the next free identifier, writes `<id>-<kebab-title>.md` from the template with
`status: proposed` and today's date, and rewrites **only** the `status:` value of each superseded
document: bodies and line endings stay byte-identical, and every rewrite is computed before any file
is touched.

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
status_values: [proposed, accepted, rejected, deprecated, superseded]

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
replace the document template `docdag new` uses. Structural checks are not rules and cannot be
disabled.

## Continuous integration

```yaml
name: decisions
on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go install github.com/Kaikei-e/DocDag/cmd/docdag@latest
      - run: docdag validate
```

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
