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
`validate` reports `cardinality`, `cycle`, `dangling_ref`, `dangling_reference`, `derived_conflict`,
`empty_edge`, `id_collision`, `immutable_violation`, `invalid_frontmatter`, `invalid_ref`,
`inverse_mismatch`, `missing_frontmatter`, `unknown_status` and `unstructured_supersedes`.

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
template `docdag new` uses.

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

## License

Apache License 2.0. See [LICENSE](LICENSE).
