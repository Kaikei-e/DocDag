# DocDag

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
WARN unstructured_supersedes 0002: supersedes edge 0003 -> 0002 comes from a field value; declare it in frontmatter
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
`ERROR status_drift 0001: has inbound supersedes but status is not superseded` and exits 1.

Besides the configured rules, `validate` reports `cycle`, `dangling_ref`, `id_collision`,
`invalid_frontmatter`, `missing_frontmatter`, `unknown_status`, `derived_conflict` and
`unstructured_supersedes`, sorted by severity, rule and identifier so the output diffs cleanly.

## Commands

Global flags: `--dir <path>`, `--config <path>` and `--format text|json`, which every command
answers in; `export` replaces the format flag with its own `mermaid|dot|json`.

| Command | What it prints | Notable failures |
| --- | --- | --- |
| `docdag validate` | one line per finding, then `OK: N docs, M typed edges, no cycles` | exit 1 if any finding is an error |
| `docdag resolve <ref>` | the current successor(s) of a reference, one per line, or the document itself when nothing supersedes it | exit 1 on an unknown reference or a supersedes cycle |
| `docdag query <ref> [--ancestors\|--descendants] [--edge <type>] [--include-refs]` | the reachable set over typed edges, descendants by default; reference-layer hits are suffixed ` (reference)` | exit 1 unknown reference, exit 2 unknown edge type or conflicting flags |
| `docdag query --binding` | every binding document | exit 2 if combined with a walk flag |
| `docdag export [--format mermaid\|dot\|json] [--include-refs] [--out PATH]` | the typed graph; mermaid on stdout by default, `-` also means stdout | exit 3 if the output file cannot be written |
| `docdag stats` | document count, binding count, orphan rate, edge count per type, supersedes chain-depth distribution, top-10 reference in-degree | — |
| `docdag new <title> [--supersedes <ref>]... [--depends-on <ref>]...` | the path of the created document | exit 1 on an unknown reference, exit 3 on a write error |

`docdag new` takes the next free identifier, writes `<id>-<kebab-title>.md` from the template with
`status: proposed` and today's date, and rewrites **only** the `status:` value of each superseded
document: bodies and line endings stay byte-identical, and every rewrite is computed before any file
is touched.

Exit codes: `0` success (warnings allowed), `1` domain failure, `2` usage error, `3` I/O or config
error — including "no documents directory found", so a repository without one needs `--dir`.

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

## License

Apache License 2.0. See [LICENSE](LICENSE).
