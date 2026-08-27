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
| `docdag stats` | document count, binding count, orphan rate, edge count per type, supersedes chain-depth distribution, top-10 reference in-degree | — |
| `docdag new <title> [--id <ref>] [--supersedes <ref>]... [--depends-on <ref>]... [--dry-run]` | the path of the created document, or the plan under `--dry-run` | exit 1 on an unknown reference or a claimed identifier, exit 3 on a write error |

`docdag context` and `--fields` are covered in [agents.md](agents.md), along with
`validate --touching`.

## export

`docdag export --edge <type>` keeps only the named edge types, repeat it to keep several, and
`--connected` drops every document no remaining typed edge touches — the two compose, so
`--connected --edge supersedes` draws the supersession chains alone.

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
