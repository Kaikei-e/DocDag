---
name: docdag
description: "Read, query, search, create, supersede, or validate ADRs, architecture decision records, decision logs, or normative spec clauses (docs/adr/, docs/ADR/, docs/decisions/, spec/, docdag.yaml). Use instead of globbing, grepping, or reading ADR/spec directories directly: what decisions are in force/binding, what replaced/superseded an ADR, what depends/rests on a decision, context for ADR/clause, branch diff of decisions, or why validation failed."
---

# docdag

Do NOT `Glob`, `Grep`, or `Read` across managed ADR or spec directories (`docs/adr/`, `docs/ADR/`, `docs/decisions/`, `spec/`) to find active decisions or follow links — one `docdag` command replaces a fan-out of file reads. Start with `docdag prime` to inspect corpus layout, declared fields, and health, or run `docdag context <ref>` / `docdag query --binding --fields id,title,status`.

`docdag` treats Markdown documents with YAML frontmatter as a typed directed graph across both the `adr` preset (single directory) and `preset: spec` (multi-kind normative corpora).

## Command routing

| Question or task | Command |
| --- | --- |
| Orient on corpus layout, fields, sections & health | `docdag prime` |
| Read a record + resolved successor + neighbors + excerpt | `docdag context <ref>` (ID or managed file path) |
| List what is in force right now (with titles) | `docdag query --binding --fields id,title,status` |
| Filter binding records by a declared field | `docdag query --binding --where <key>=contains:<val> --fields id,title` |
| What will be in force on a day / at a tag | `docdag query --binding --as-of YYYY-MM-DD` / `--at v1.2.0` |
| What replaced this decision | `docdag resolve <ref> --fields id,title,status` |
| What rests on / depends on this decision (impact) | `docdag query <ref> --ancestors --fields id,title,status` |
| What does this decision rest on | `docdag query <ref> --descendants --fields id,title,status` |
| Create or supersede a record | `docdag new "<title>" [--supersedes <ref>]` (or `--kind <kind> --id <id>`) |
| Check corpus / check only your edit | `docdag validate` / `docdag validate --touching <path>` |
| Why a conflict is suppressed | `docdag validate --show-suppressed` |
| Graph & binding delta on a branch | `docdag diff origin/main` |
| Lint `docdag.yaml` rules | `docdag lint` |
| Corpus shape / frontmatter keys | `docdag stats` / `docdag stats --fields` |

## Context & query usage

`docdag context <ref>` accepts either an ID (`0001`, `ADR-1`, `UZ-V-001`) or a managed file path (`docs/adr/0001-use-postgres.md`, `spec/clauses/UZ-V-001.md`). It prints the record, any resolved successor, normative neighbors, suppressed conflicts, retractions, and typed-edge neighbors with automatic excerpt extraction (falling back across `Decision`, localized headings, and lead paragraphs for spec clauses). Useful flags: `--depth 2`, `--edge <type>`, `--all` (include non-binding nodes), `--budget 600`, `--section <heading>`, `--format md|json`.

`docdag query` accepts `--fields <cols>` for tab-separated columns (`-` for unwritten keys, comma-joined lists), `--format json` for all projections and declared fields, and repeatable `--where <key>=<op>:<value>` filters (ANDed; ops `eq`, `not`, `contains`, `not_contains`, `subset_of`).

## Before writing or changing a document

1. Run `docdag query --binding --fields id,title,status` — do not contradict a binding document without superseding it.
2. Run `docdag context <ref>` for every document about to be touched or superseded.
3. Write the record: `docdag new "<title>" --supersedes <ref>` sets `supersedes: [<id>]` in the new frontmatter and sets `status: superseded` on the target. On multi-kind corpora, pass `--kind <kind>` (and `--id <id>` when the kind declares a pattern), then fill commented placeholder keys.
4. Run `docdag validate --touching <path>` and follow any `fix:` suggestion.

Never edit a document's identifier or renumber files: identity comes from the file's digit run or kind pattern, so renaming silently rewrites the graph.

## Vocabulary

- **ref**: any identifier (`339`, `0339`, `ADR-339`) or managed file path (`docs/adr/0339-use-postgres.md`); digit-run IDs zero-pad to configured width, while patterned kinds (`UZ-V-001`) match verbatim.
- **binding**: `accepted` and in force with no `accepted` or `superseded` successor (`proposed` or `withdrawn` never un-binds an `accepted` record); `spec` projects `modality` alongside `id`.
- **kind**: under `kinds:` in `docdag.yaml`, directory determines document kind and ID rules (`spec/clauses/UZ-V-001.md` is a `clause`).
- **modality**: clause strength (`MUST`, `MUST_NOT`, `SHOULD`, `SHOULD_NOT`, `MAY`), printed by `query --binding` when declared.
- **as-of**: under `period:`, `--as-of YYYY-MM-DD` evaluates active dates on that day; `--at <rev>` reads from git revision `<rev>`.
- **resolve**: walks `supersedes` forward to standing successors (and reports non-binding pending successors).
- **typed edge**: relation in frontmatter (`supersedes:`, `depends-on:`) or derived (`status: superseded by 0003`) that carries DAG invariants.
- **reference layer**: plain body links (`[[0042]]`, `[0042](0042-x.md)`); unvalidated unless `references.dangling` is set, surfaced via `--include-refs`.
- **retracts**: frontmatter list of `{from, edge, to}` mappings on an `accepted` (or `superseded`) record that removes an earlier typed edge without mutating closed history.
- **finding**: `<path>:<line>: <SEVERITY> <rule> ...`, often with a `fix:` line; `--show-suppressed` prints conflicts defeated by an `excepts` edge.

## Reading a finding

```
docs/decisions/0001-serve-images.md:3: ERROR status_drift 0001: has inbound supersedes but status is not superseded
  fix: set status: superseded in docs/decisions/0001-serve-images.md
```

`validate` exits `1` on errors, `2` on usage mistakes, and `3` when config/directories cannot be read (including 0 matched docs or directory case mismatches). Validation fails closed: unknown config keys fail with line numbers, stray `.md` files report `unmanaged_file`, and opt-in `sections:` checks `missing_section` / `section_order`.

## Changing docdag.yaml

`validate` never lints rules; run `docdag lint` after editing `docdag.yaml` (layer 1 reads config only), `docdag lint --corpus` to test rules against the vault, or `docdag lint --fixtures lint/` (`--all` runs all three; `docdag new --fixture <rule>` scaffolds `ruleid/` and `ok/` fixtures). `lint` exits `1` on errors, `2` on warnings alone, `3` on invalid config.

## Setup

Install `docdag` (`go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.8.0`) and allow it in `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["Bash(docdag *)"]
  }
}
```

This plugin supports both Claude Code and Antigravity (`agy`). In Claude Code it registers three hooks: `SessionStart` (`docdag prime`, falling back to `docdag validate` on older binaries), `PreToolUse` on `Read|Glob|Grep` (`docdag prime --if-present --hint <target>`, non-blocking advisory context), and `PostToolUse` on `Edit|Write` (`docdag validate --touching <file> --if-managed` for managed `.md` files and `docdag lint --config <file>` for `docdag.yaml`, failing closed with exit `2` on findings).
