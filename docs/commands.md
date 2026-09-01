# Commands

Every command reads the document corpus, builds the typed graph and answers from it. See
[checks.md](checks.md) for what `validate` reports, [configuration.md](configuration.md) for the
`docdag.yaml` that shapes the graph, and the [README](../README.md) for the model the commands
share.

## Global flags

`--dir <path>`, `--config <path>` and `--format text|json` are answered by every command.
`validate` also answers in `github` and `rdjson`, `context` in `md`, and `export` replaces the
format flag with its own `mermaid|dot|json`.

Without `--dir`, DocDag looks in `docs/adr`, `doc/adr`, `docs/decisions`, `docs/ADR`, `adr` — the
first that exists and holds a file named `NNNN.md` or `NNNN-kebab-title.md`, 3 to 6 digits.

## The commands

| Command | What it prints | Notable failures |
| --- | --- | --- |
| `docdag validate [--touching <path>]... [--immutable-since <rev>] [--format text\|json\|github\|rdjson]` | one line per finding, each followed by an indented `fix:` where there is a remedy, then `OK: N docs, M typed edges, no cycles` when nothing errored | exit 1 if any finding is an error, exit 3 if `--immutable-since` is given outside a git repository or without `git` |
| `docdag resolve <ref> [--fields <list>]` | the current successor(s) of a reference, one per line, or the document itself when nothing supersedes it | exit 1 on an unknown reference or a supersedes cycle |
| `docdag query <ref> [--ancestors\|--descendants] [--edge <type>] [--include-refs] [--fields <list>]` | the reachable set over typed edges, descendants by default; reference-layer hits are suffixed ` (reference)` | exit 1 unknown reference, exit 2 unknown edge type or conflicting flags |
| `docdag query --binding [--fields <list>]` | every binding document | exit 2 if combined with `--ancestors`, `--descendants`, `--edge` or `--include-refs` |
| `docdag context <ref> [--depth N] [--edge <type>]... [--section <heading>] [--budget N] [--all]` | the document, what it resolves to and its neighbourhood, each quoting one section | exit 1 unknown reference, exit 2 unknown edge type |
| `docdag export [--format mermaid\|dot\|json] [--include-refs] [--connected] [--edge <type>]... [--out PATH]` | the typed graph; mermaid on stdout by default, `-` also means stdout | exit 2 on an unknown edge type, exit 3 if the output file cannot be written |
| `docdag stats [--fields]` | document count, binding count, orphan rate, edge count per type, supersedes chain-depth distribution, top-10 reference in-degree; with `--fields`, the frontmatter fields instead | exit 3 without `--fields` if the configuration declares no `supersedes` edge |
| `docdag new <title> [--kind <name>] [--id <ref>] [--supersedes <ref>]... [--depends-on <ref>]... [--dry-run]` | the path of the created document, or the plan under `--dry-run` | exit 1 on an unknown reference, a claimed identifier or a `--kind` the corpus cannot answer, exit 3 on a write error |

`docdag context` and `--fields` are covered in [agents.md](agents.md), along with
`validate --touching`.

## export

`docdag export --edge <type>` keeps only the named edge types, repeat it to keep several, and
`--connected` drops every document no remaining typed edge touches — the two compose, so
`--connected --edge supersedes` draws the supersession chains alone.

## stats

`docdag stats --fields` answers about frontmatter rather than degrees: one row per field, with the
documents that write it, the day one of those documents last changed, and whether `fields:` retired
it. It is what a removal is decided on, so a declared field nobody writes is still a row, at zero:

```
field   documents  last change  deprecated
status  6          2026-03-04   -
owner   2          2026-01-02   yes
team    0          -            yes
```

The day comes from `git log`, in one invocation for the whole corpus. Outside a repository, or where
git cannot answer, the column is `-` rather than an error: `stats` is a report, not a check. On a
corpus that declares `kinds:`, `kind` is the directory's answer and is read on every document, so
its row counts the whole corpus rather than the documents that write the key. The
field report needs no edge type, so it answers for a corpus the degree statistics cannot describe.
`--format json` writes `{"fields": [...]}` with the same values, `deprecated` always present and
`last_change` left out where there is none.

## new

`docdag new` takes the next free identifier, writes `<id>-<kebab-title>.md` from the template with
`status: proposed` and today's date, and rewrites **only** the `status:` value of each superseded
document: bodies and line endings stay byte-identical, and every rewrite is computed before any file
is touched. The name comes from the `filename:` template, so a corpus of bare `NNNNNN.md` files
configures `filename: "{id}.md"` and keeps that shape.

`--dry-run` runs every computation, writes nothing and prints the plan, as `create <id> <path>`
followed by one `rewrite <path> status: superseded` line each, or as JSON under `--format json`. It
exits exactly as the real run would, so it is a safe way to see what a creation would cost. The
plan names its files the way a finding does, relative to the working directory, and its JSON always
carries `exists` — `true` when the corpus already holds the document and nothing would be written,
where the text form says `exists` instead of `create`.

`--id <ref>` creates the document under a chosen identifier instead of the next free one. If that
identifier already names a document with the same title, `new` prints its path and writes nothing,
so re-running an agent's command is harmless; a different title under that identifier is an error.
`new` also refuses to run at all while the corpus carries an `id_collision`.

### new --kind

A corpus that declares [`kinds:`](configuration.md#kinds) needs `--kind <name>`: which kind to
create, under which identity rules and from which template, has no default answer. The document is
written into that kind's directory, and its frontmatter is what the kind's own declarations call
for — the identifier where the kind declares an `id:` pattern, the kind, the title, the first word
of the kind's `status_values`, today's date:

```console
$ docdag new "Runs are recorded with their seed" --kind clause --id UZ-V-007
spec/clauses/UZ-V-007.md
```

```yaml
---
id: UZ-V-007
kind: clause
title: Runs are recorded with their seed
status: proposed
date: 2026-09-01
# supersedes:
#   - {ref: <clause|premise>, reason: <recurrence|premise-collapse|conflict|vocabulary>}
# premise:
#   - <premise>
# rationale:
#   - <principle>
# counterexample:
#   - <pm>
---
```

The edges the kind may declare — the ones whose `from:` names it, and the ones constrained to no
kind at all — follow as commented stubs naming what each reaches and the attributes it requires. A
stub is a comment rather than an empty key because a key present and naming nothing is the
`empty_edge` finding: the point is to show what the configuration offers, not to hand back a
document whose first validation fails. A kind that answers to no status vocabulary gets no `status:`
key rather than an invented value.

`--id` is **required** for a kind that declares an `id:` pattern: a pattern is a spelling, not a
sequence, so there is no next identifier to take, and one the pattern rejects is an error. A kind
that declares no pattern keeps the digit-run identity, and `new --kind` counts up from the highest
identifier *that kind* holds. Naming a kind the configuration does not declare, and passing `--kind`
to a corpus that declares no kinds at all, are both errors (exit 1). `--dry-run`, `--format json`
and the identifier rules above work exactly as they do without `--kind`.

`--supersedes` and `--depends-on` refuse an edge that declares a required attribute (exit 3): the
entry would be incomplete, and a creation has no value to put there — write that edge into the
created document instead.

## Exit codes

`0` success (warnings allowed), `1` domain failure, `2` usage error, `3` I/O or config error —
including "no documents directory found", so a repository without one needs `--dir`.

## validate output

Every finding names a file and, wherever a frontmatter key carries the fault, the line of that key:

```
<path>:<line>: <SEVERITY> <rule> <id>: <detail>
```

`:<line>` is dropped when the position is unknown. Findings sort by severity, path, line, rule,
identifier and detail, so a report reads in file order and diffs cleanly.

### text

The default. A finding with a mechanical remedy carries one, indented under the finding (every
example below is `docdag validate --dir testdata/fixtures/<corpus>` from a checkout):

```
testdata/fixtures/dangling/0002-ship-logs-to-a-central-collector.md:4: ERROR dangling_ref 0002: supersedes reference "0009" does not name a document
  fix: did you mean 0001?
```

### json

`--format json` writes one object, versioned so a consumer can tell a shape change from a content
change. `preset_version` is the revision of the configuration the corpus was checked under, left out
where it names none; `location` is the primary position and `related` names the other files a
finding involves — the peers of a collision, the rest of a cycle:

```json
{
  "schema_version": 2,
  "preset_version": 1,
  "findings": [
    {
      "severity": "error",
      "rule": "status_drift",
      "id": "0001",
      "detail": "has inbound supersedes but status is not superseded",
      "location": {
        "path": "testdata/fixtures/status-drift/0001-serve-images-from-the-application-server.md",
        "line": 3
      },
      "fix": "set status: superseded in testdata/fixtures/status-drift/0001-serve-images-from-the-application-server.md"
    }
  ],
  "summary": {
    "documents": 2,
    "edges": 1,
    "errors": 1,
    "warnings": 0,
    "cycles": 0
  }
}
```

### github

`--format github` writes one GitHub Actions workflow command per finding, followed by the same
summary line as `text` when nothing errored:

```
::error file=testdata/fixtures/status-drift/0001-serve-images-from-the-application-server.md,line=3,title=status_drift::0001: has inbound supersedes but status is not superseded
```

A workflow step renders at most ten annotations, so a corpus with more findings than that needs a
second `--format text` run written into `$GITHUB_STEP_SUMMARY` to show the rest — see
[ci.md](ci.md).

### rdjson

`--format rdjson` writes the [reviewdog](https://github.com/reviewdog/reviewdog) diagnostic format
as a single `DiagnosticResult`, for `reviewdog -f=rdjson`. It carries neither a summary line nor a
remedy: the format has no field for one.

## JSON headers

The two JSON outputs that are objects — `validate --format json` and `context --format json` — are
headed by a `schema_version` and, where the configuration names one, a `preset_version`. Both are at
`schema_version: 2`. The listings are arrays of records rather than objects (`query`, `query
--fields`, `resolve`), so they carry no header; `preset_version` is read from `validate` beside
them. The text, `md`, `github` and `rdjson` formats are unversioned: a header is a JSON affair.
