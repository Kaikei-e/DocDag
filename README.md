# DocDag

[![CI](https://github.com/Kaikei-e/DocDag/actions/workflows/ci.yml/badge.svg)](https://github.com/Kaikei-e/DocDag/actions/workflows/ci.yml)
[![Release](https://github.com/Kaikei-e/DocDag/actions/workflows/release.yml/badge.svg)](https://github.com/Kaikei-e/DocDag/actions/workflows/release.yml)

DocDag reads a directory of Markdown documents with YAML frontmatter, extracts a **typed directed
graph** from it, enforces DAG invariants on that graph, and answers queries about it. Decision
records rot in ways review does not catch: a decision superseded twice with the status never
updated, a supersession cycle, a `supersedes: 0042` pointing at a file nobody wrote. Those are graph
properties, so a graph check can enforce them, and `docdag validate` exits 1 on any error, in one CI
line.

DocDag ships two presets. `adr` is the default: one directory of Architecture Decision Records,
identified by a digit run, superseding one another. `spec` is a normative standard as a graph —
clauses, the conformance tests that enforce them, the deviations recorded against them and the
measurements taken of them, each kind in a directory of its own with an identifier shape of its own —
where a `MUST` that no test enforces is a finding rather than a rule. Both are plain configuration:
[docs/configuration.md](docs/configuration.md) prints each in full, and `docdag.yaml` overrides
either.

## The model

DocDag keeps two layers apart. The **constraint layer** is the typed edges declared in frontmatter
(`supersedes:`, `depends-on:`) plus the edges derived from configured field patterns; only these
carry invariants, acyclicity and rules. The **reference layer** is the untyped links found in
bodies — `[[wikilink]]` and relative Markdown links to other managed documents — surfaced by
`--include-refs` and `stats`, never part of a constraint, and unvalidated unless the configuration
asks for it.

A document's identity is its digit run, so `339`, `ADR-339`, `000339` and `0339-use-postgres.md` all
name the same node, displayed zero-padded to `id_width`; renaming a file's title suffix therefore
does not change what it is, and two files that normalize alike are an `id_collision` error. Status
is a projection of that graph rather than an independent fact: a document is *binding* when its
status is `accepted` and nothing supersedes it, and a status the edges contradict is a finding
rather than a matter of opinion.

## Install

Install a pinned version locally:

```sh
go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.2.0
```

This installs `docdag` into `$(go env GOPATH)/bin`. Prebuilt binaries for tagged versions, and the
`checksums.txt` that covers them, are attached to the repository's Releases page. `docdag --version`
reports the tag the binary was built from, and `dev` for one built from a checkout.

In CI, use the composite action, which downloads the release binary, verifies it against that
release's `checksums.txt`, and needs no Go toolchain:

```yaml
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.2.0
        with:
          args: validate --format github   # this is the default
```

## Quickstart

No configuration is needed. From the root of a repository whose decisions live in `docs/decisions`:

```console
$ docdag validate
docs/decisions/0002-store-thumbnails-on-the-local-disk.md:3: WARN unstructured_supersedes 0002: supersedes edge 0003 -> 0002 comes from a field value; declare it in frontmatter
  fix: declare supersedes: 0002 in 0003
OK: 4 docs, 3 typed edges, no cycles

$ docdag resolve 0002          # what replaced this decision?
0003

$ docdag query --binding       # what is in force right now?
0001
0003

$ docdag query 0001 --ancestors --edge depends-on   # what rests on this decision?
0003
0004

$ docdag query --binding --as-of 2027-04-01        # what will be in force then?
0001
0005
```

Where a corpus declares a `period:`, what is in force is an answer about a day: `--as-of` moves the
day and `--at <rev>` moves the revision the documents are read from, and the two compose into "what
the vault at that revision said was in force on that day".

DocDag looks in `docs/adr`, `doc/adr`, `docs/decisions`, `docs/ADR`, `adr` — the first that exists
and holds a file named `NNNN.md` or `NNNN-kebab-title.md`, 3 to 6 digits; `--dir` overrides it.

## What it checks

`validate` prints one line per finding, followed by an indented `fix:` wherever there is a
mechanical remedy, and exits 1 if any finding is an error:

```
<path>:<line>: <SEVERITY> <rule> <id>: <detail>
```

- **Structural** — `invalid_frontmatter`, `missing_frontmatter`, `id_collision`, `unknown_status`,
  `empty_edge`, `invalid_ref`, `dangling_ref`, `unstructured_supersedes`, `derived_conflict`, and,
  for an edge that declares `attrs:`, `edge_attr_unknown`, `edge_attr_missing`, `edge_attr_invalid`.
- **Graph** — `cycle`, `cardinality`, `inverse_mismatch`, and, for an edge that declares `target:`
  or a corpus that declares `path_constraints:`, `stale_target` and `path_mismatch`; plus the
  preset's two status rules, `status_drift` and `superseded_orphan`.
- **Periods** — for a kind that declares `period:`, the two days a document writes are read against
  the day the run is about: `period_invalid`, `period_conflict`, `expired_deviation`.
- **Reference layer** — `dangling_reference`, off until `references.dangling` asks for it.
- **History** — `immutable_violation`, under `--immutable-since <rev>`.

[docs/checks.md](docs/checks.md) gives each finding its severity, its trigger and its remedy.

`docdag lint` asks the other question — whether the rules themselves hold up. It reports a rule that
can never fire, one that fires on every document, one that says what another rule already says, and,
with `--corpus` and `--fixtures`, the rules the vault never fires and the rules whose own fixtures
disagree with them. `validate` never runs it: a configuration's health and a corpus's state have
different lifecycles.

## For agents

An agent should ask the graph rather than read the directory: one command replaces a fan-out of file
reads. `docdag context <ref>` prints a document, what it resolves to and its neighbourhood, each
entry quoting the first paragraph of its `Decision` section; `--fields id,title,status,path` gives
`resolve` and `query` tab-separated columns; `docdag validate --touching <path>` reports only what
one edit can break. This repository is also a Claude Code plugin, installing a skill and a
`PostToolUse` hook that validates a decision record as it is written:

```console
$ claude
/plugin marketplace add Kaikei-e/DocDag
/plugin install docdag@docdag
```

[docs/agents.md](docs/agents.md) covers the flags, the output shapes and the hook's requirements.

## Documentation

- [docs/commands.md](docs/commands.md) — every command, the global flags, the exit codes and the
  four `validate` output formats.
- [docs/configuration.md](docs/configuration.md) — the `docdag.yaml` reference: the preset in full,
  every optional key, and the rule vocabulary.
- [docs/checks.md](docs/checks.md) — one entry per finding: what triggers it and what clears it.
- [docs/ci.md](docs/ci.md) — the composite action, append-only history, linting the configuration
  and the pre-commit hook.
- [docs/agents.md](docs/agents.md) — `context`, `--fields`, `--touching` and the plugin.

## MADR compatibility

A conventional MADR repository needs no changes — the invariants hold as the files already are:

- Filenames `NNNN-kebab-title.md` and bare `NNNNNN.md` are both recognized, 3 to 6 digits, and
  unrecognized frontmatter keys are ignored, so another tool's fields are safe.
- `status: superseded by 0003` derives the `supersedes` edge with `0003` as the newer document and
  makes the containing one count as superseded, raising an `unstructured_supersedes` warning — a
  suggestion to declare the edge in frontmatter, not a failure. The graph is the same either way.
- Body links stay in the reference layer, so prose cannot fail a build unless `references.dangling`
  opts in.

[docs/checks.md](docs/checks.md) has the rest: how `withdrawn` differs from `superseded`, what makes
a `supersedes:` entry `invalid_ref` rather than `dangling_ref`, and which links never join the
reference layer at all.

## Changelog

[CHANGELOG.md](CHANGELOG.md) records what each release changed, including the output formats v0.2.0
broke and how to migrate off them.

## License

Apache License 2.0. See [LICENSE](LICENSE).
