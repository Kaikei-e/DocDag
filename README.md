# DocDag

[![CI](https://github.com/Kaikei-e/DocDag/actions/workflows/ci.yml/badge.svg)](https://github.com/Kaikei-e/DocDag/actions/workflows/ci.yml)
[![Release](https://github.com/Kaikei-e/DocDag/actions/workflows/release.yml/badge.svg)](https://github.com/Kaikei-e/DocDag/actions/workflows/release.yml)

DocDag reads a directory of Markdown documents with YAML frontmatter, extracts a **typed directed
graph** from it, enforces DAG invariants on that graph, and answers queries about it as a single Go
binary (`github.com/goccy/go-yaml` is the only external dependency) and as public Go packages
(`config`, `model`, `lint`). Decision records and normative specifications rot in ways prose review
does not catch: a decision superseded twice with the status never updated, a supersession cycle, a
`supersedes: 0042` pointing at a file nobody wrote, a `MUST` clause with no conformance test, or a
test still pointing at a superseded clause. Those are graph properties, so a deterministic graph
check can enforce them, and `docdag validate` exits 1 on any error — and exits 3 if a check could
not actually run — in one CI line.

DocDag ships two presets. `adr` is the default: one directory of Architecture Decision Records,
identified by a digit run, superseding one another. `spec` is a normative standard as a graph —
eight kinds, among them the clauses, the conformance tests that enforce them and the deviations
recorded against them, each in a directory of its own and with an identifier shape of its own —
where a `MUST` that no test enforces is a finding rather than a rule. Both are plain configuration:
[docs/configuration.md](docs/configuration.md) prints each in full, and `docdag.yaml` overrides
either.

## Assembling a configuration in Go

A program that owns a vault can import `config`, `model` and `lint` to build the same configuration
the YAML file describes, validate it without writing a file, and marshal it deterministically into
`docdag.yaml`. The field names and YAML tags of `config.Config` are the contract; see
[docs/ADR/0006](docs/ADR/0006-public-config-yaml-roundtrip-and-append-only.md) for what is stable.

```go
import (
    "github.com/goccy/go-yaml"
    "github.com/Kaikei-e/DocDag/config"
)

cfg := config.SpecPreset()
cfg.PresetVersion = 3
// … add kinds, edges, rules …
if err := cfg.Validate(); err != nil {
    return err
}
out, err := yaml.Marshal(cfg)
```

`lint.Check(cfg, vaultDir, fixturesDir)` runs the three lint layers in process and returns
`[]model.Finding`. An empty `vaultDir` answers only the inherent layer.

## The model

DocDag keeps two layers apart. The **constraint layer** is the typed edges declared in frontmatter
(`supersedes:`, `depends-on:`) plus the edges derived from configured field patterns; only these
carry invariants, acyclicity and rules. The **reference layer** is the untyped links found in
bodies — `[[wikilink]]` and relative Markdown links to other managed documents — surfaced by
`--include-refs` and `stats`, never part of a constraint, and unvalidated unless `references.dangling`
asks for it.

A document's identity is its digit run, so `339`, `ADR-339`, `000339` and `0339-use-postgres.md` all
name the same node, displayed zero-padded to `id_width` (and unquoted leading-zero tokens such as
`0011` are read as literal text rather than YAML octal); renaming a file's title suffix therefore
does not change what it is, and two files that normalize alike are an `id_collision` error. Where a
corpus declares `kinds:`, identity is per kind — a pattern of its own, such as `UZ-V-001` (with
`padding_mismatch` reporting a reference whose only fault is its zero width), or the digit run where
a kind declares none — because the directory a document sits in is what chose the rules it was read
under. Status is a projection of that graph rather than an independent fact: a document is *binding*
when its status is `accepted`, nothing in force supersedes it and, where a kind declares a `period:`,
the day being asked about falls inside it; a status the edges contradict is a finding rather than a
matter of opinion.

To keep every configuration decidable and verifiable without a solver, the engine enforces four
structural boundaries:

- **No expression language** — rules, projections and target checks use a closed declarative
  condition vocabulary (`attr`, `inbound`, `outbound`, `via`, `via_inbound`, `any_of`, `not`) over
  finite declared domains.
- **Modal depth fixed at 2** — `via` and `via_inbound` inspect one hop (an edge and a local
  condition on the neighbour) and never nest.
- **1–2 step path constraints** — `path_constraints:` compares two paths of one or two steps each
  (`equals: none` or `subset_of:`); paths of three or more steps turn implication into an
  undecidable regular-path problem, while transitive supersession reach is answered by `resolve`.
- **Zero new runtime dependencies** — single static binary built on the Go standard library plus
  `github.com/goccy/go-yaml`.

## Install

Install a pinned version locally:

```sh
go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.5.0
```

This installs `docdag` into `$(go env GOPATH)/bin`. Prebuilt binaries for tagged versions, and the
`checksums.txt` that covers them, are attached to the repository's Releases page. `docdag --version`
reports the tag the binary was built from, and `dev` for one built from a checkout.

In CI, use the composite action, which downloads the release binary, verifies it against that
release's `checksums.txt`, and needs no Go toolchain:

```yaml
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.5.0
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

$ docdag resolve 0002          # what replaced this decision? (stops at proposed successors)
0003

$ docdag query --binding       # what is in force right now?
0001
0003

$ docdag query 0001 --ancestors --edge depends-on   # what rests on this decision?
0003
0004

$ docdag new "Use read-through caching" --supersedes 0003   # creates 0005 and rewrites 0003 status in place
docs/decisions/0005-use-read-through-caching.md

$ docdag export --connected --edge supersedes       # Mermaid (default), Graphviz DOT, or JSON
```

DocDag looks in `docs/adr`, `doc/adr`, `docs/decisions`, `docs/ADR`, `adr` — the first that exists
and holds a file named `NNNN.md` or `NNNN-kebab-title.md`, 3 to 6 digits; `--dir` overrides it.
`docdag new` rewrites **only** the `status:` value of each superseded document in place, keeping
bodies and line endings byte-identical and computing every rewrite before any file is touched
(`--dry-run` prints the plan without writing). `docdag export` emits the typed graph as `mermaid`
(on stdout by default), `dot`, or `json`.

### The spec preset

One line adopts the second preset whole, and the eight directories it names appear as the corpus
grows into them. The corpus below holds three clauses, the conformance test enforcing one of them,
the subject all three speak to and the post-mortem they cite as their counterexample:

```console
$ cat docdag.yaml
preset: spec

$ docdag validate
spec/clauses/UZ-V-002.md:4: ERROR orphan_must UZ-V-002: is MUST or MUST_NOT and accepted but nothing enforces it
as of 2026-09-02

$ docdag query --binding                      # what is in force, and at what strength
UZ-V-001	MUST
UZ-V-002	MUST
UZ-V-003	SHOULD

$ docdag query --binding --as-of 2027-06-01   # UZ-V-003 runs out on 2027-01-01
UZ-V-001	MUST
UZ-V-002	MUST

$ docdag new --kind clause --id UZ-V-004 "A report names its grader"
spec/clauses/UZ-V-004.md
```

`UZ-V-002` claims a `MUST` that no conformance test enforces, which is a standard saying one thing
and checking another; the remedy is the test or a weaker modality, so the finding carries no `fix:`
line — which of the two is right is a decision rather than an edit. The modality column is there
because a set spanning permissions and prohibitions cannot be read without it. And the closing
`as of` line is the `--as-of` run's premise: where a corpus declares a `period:`, what is in force
is an answer about a day, `--at <rev>` moves the revision the documents are read from, and the two
compose into "what the vault at that revision said was in force on that day". The `adr` preset
declares no period, so the corpus above answers the same way on every day.

## What it checks

`validate` prints one line per finding, followed by an indented `fix:` wherever there is a
mechanical remedy, in `--format text|json|github|rdjson` (including GitHub Actions workflow commands
and [reviewdog](https://github.com/reviewdog/reviewdog) `rdjson` diagnostics), and exits 1 if any
finding is an error:

```
<path>:<line>: <SEVERITY> <rule> <id>: <detail>
```

- **Structural** — `invalid_frontmatter`, `missing_frontmatter`, `id_collision`, `unknown_status`,
  `empty_edge`, `invalid_ref`, `dangling_ref`, `padding_mismatch`, `unmanaged_file`, `unstructured_supersedes`,
  `derived_conflict`, and, for an edge that declares `attrs:`, `edge_attr_unknown`,
  `edge_attr_missing`, `edge_attr_invalid`.
- **Sections** — for a corpus that opts into `sections:`, `missing_section` and `section_order`
  (with `|` heading alternatives, heading `level:`, `ordered: true`, and `when: {status: [...]}`).
- **Kinds and declared fields** — for a corpus that declares `kinds:` or `fields:`, an identity its
  kind's pattern rejects, a `kind:` its directory disagrees with, an undeclared key on a closed
  kind, an endpoint of the wrong kind and a field value the vocabulary does not hold:
  `id_mismatch`, `kind_mismatch`, `unknown_field`, `edge_kind_mismatch`, `unknown_field_value`,
  `missing_field`, `deprecated_field`.
- **Graph, target leaf and modality** — `cycle`, `cardinality`, `inverse_mismatch`, and, for an edge
  that declares `target:` (such as `leaf_of: supersedes`, flagging references to superseded
  intermediate nodes with a `fix:` naming the current leaf) or a corpus that declares
  `path_constraints:`, `stale_target` and `path_mismatch`; for one that declares a `modality`
  vocabulary, `modality_conflict` (with weak conflicts answered by a scoped `excepts:` edge
  suppressed and inspectable via `validate --show-suppressed`) and `excepts_strict`; plus the two
  status rules `adr` ships, `status_drift` and `superseded_orphan`, which `spec` replaces with ten of
  its own (including inbound coverage rules such as `orphan_must` built on `inbound`/`via_inbound`).
- **Periods (bitemporal valid/transaction time)** — for a kind that declares `period:`, the interval
  a document writes (or the end day derived from its earliest accepted successor, never written back
  to frontmatter) is evaluated against `--as-of <day>` and `--at <rev>`: `period_invalid`,
  `period_conflict`, `expired_deviation`.
- **Reference layer** — `dangling_reference`, off until `references.dangling` asks for it.
- **Append-only history** — `immutable_violation`, under `--immutable-since <rev>` (scoped to
  `append_only: true` kinds such as `conform` and `measure` on multi-kind corpora): a settled
  document may only change its `status`, gain entries under an `inverse:` key, or append lines to
  the end of its body.
- **Fail-closed execution (exit 3)** — every reading command exits 3 (`model.ErrEmptyCorpus`) when
  zero managed documents match; `docdag.yaml` rejects unknown keys (`DisallowUnknownField`) and
  unknown `structural:` check names; and explicit `dir:` / kind `dir:` paths must exist on disk with
  exact case.

[docs/checks.md](docs/checks.md) gives each finding its severity, its trigger and its remedy.

`docdag lint` asks the other question — whether the rules themselves hold up — and emits the same
`text|json|github|rdjson` formats. Layer 1 expands conditions to disjunctive normal form over the
declared finite vocabularies and reports a rule that can never fire (`unfirable_rule`), one that
fires on every document (`always_fires`), one that says what another rule already says
(`duplicate_rule`), conflicting remedies (`conflicting_fix`), and unused declarations. Layer 2
(`--corpus`, `--since <rev>`) reports rules the vault never fires, and Layer 3 (`--fixtures <dir>`,
seeded by `docdag new --fixture <rule>`) runs each rule against its own `ruleid/` and `ok/` fixtures
to prove a silent rule can still ring. `validate` never runs it: a configuration's health and a
corpus's state have different lifecycles.

## For agents

An agent should ask the graph rather than read the directory: one command replaces a fan-out of file
reads. `docdag context <ref>` prints a document, what it resolves to (stopping at non-binding
`proposed` successors and reporting them as pending) and its neighbourhood, each entry quoting the
first paragraph of its `Decision` section, with `--budget N` capping the prose in tokens and
`--format text|md|json` shaping the output; `--fields id,title,status,path` gives `resolve` and
`query` tab-separated columns; `docdag stats` (and `stats --fields`) summarises corpus degrees,
modalities and frontmatter field freshness; `docdag validate --touching <path>` reports only what
one edit can break; and `docdag lint` answers for an edit to `docdag.yaml` rather than to a
document. This repository is also a Claude Code plugin, installing a skill and a fail-closed
`PostToolUse` hook that validates a decision record as it is written and lints the configuration when
that is what changed:

```console
$ claude
/plugin marketplace add Kaikei-e/DocDag
/plugin install docdag@docdag
```

[docs/agents.md](docs/agents.md) covers the flags, the output shapes and the hook's requirements.

## Documentation

- [docs/commands.md](docs/commands.md) — every command (`validate`, `lint`, `resolve`, `query`,
  `context`, `export`, `stats`, `new`), the global `--as-of` / `--at` time flags, the exit codes and
  the `text|json|github|rdjson` and `mermaid|dot|json` output formats.
- [docs/configuration.md](docs/configuration.md) — the `docdag.yaml` reference: the `adr` and `spec`
  presets in full, every optional key (`kinds:`, `fields:`, `edges:`, `target:`, `path_constraints:`,
  `projections:`, `period:`, `sections:`, `structural:`), the closed rule vocabulary, and the Go API.
- [docs/checks.md](docs/checks.md) — one entry per finding (33 structural checks, preset rules,
  reference and history checks, and `lint` findings): what triggers it and what clears it.
- [docs/ci.md](docs/ci.md) — the composite action, `github` and `rdjson` annotations, append-only
  history (`--immutable-since`), scheduled `--as-of` expiry runs, linting the configuration and the
  pre-commit hooks.
- [docs/agents.md](docs/agents.md) — `context --budget`, `--fields`, `--touching`, `--as-of` /
  `--at`, `lint`, `new --fixture` and the Claude Code plugin.
- [docs/ADR/](docs/ADR/) — the nine architecture decision records (0001–0009) behind the design,
  indexed and in reading order: the `spec` preset without an expression language, target conditions
  and 1–2 step path constraints, modality and exception suppression, three-layer `lint` vacuity
  checking, bitemporal in-force periods, the public `config`/`model`/`lint` packages and
  `append_only` kinds, fail-closed enforcement, opt-in body `sections:`, and `resolve` stopping at
  non-binding successors.
- [CHANGELOG.md](CHANGELOG.md) — what each release changed across v0.1.0–v0.5.0, including the
  v0.5.0 fail-closed breaking changes and migration steps.

## MADR compatibility

A conventional MADR repository needs no changes — the invariants hold as the files already are:

- Filenames `NNNN-kebab-title.md` and bare `NNNNNN.md` are both recognized, 3 to 6 digits, and
  unrecognized frontmatter keys on open kinds are ignored, so another tool's fields are safe
  (`docdag.yaml` itself is decoded strictly, and kinds with `closed: true` reject undeclared
  frontmatter keys). Stray `.md` files in the documents directory not matching the filename pattern
  are warned as `unmanaged_file` (exempt: `README.md`, `index.md`, `template.md`, and files starting
  with `_` or `.`).
- `status: superseded by 0003` derives the `supersedes` edge with `0003` as the newer document and
  makes the containing one count as superseded, raising an `unstructured_supersedes` warning — a
  suggestion to declare the edge in frontmatter, not a failure. The graph is the same either way.
- Body links stay in the reference layer, so prose cannot fail a build unless `references.dangling`
  opts in.
- Optional `sections:` configuration can enforce standard MADR or Nygard headings and ordering.

[docs/checks.md](docs/checks.md) has the rest: how `withdrawn` differs from `superseded`, what makes
a `supersedes:` entry `invalid_ref` rather than `dangling_ref`, and which links never join the
reference layer at all.

## License

Apache License 2.0. See [LICENSE](LICENSE).
