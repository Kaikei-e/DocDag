---
name: docdag
description: Read, query and validate a repository's decision records or normative clauses as a typed graph with the docdag CLI. Use it before writing or changing an ADR or a spec clause, when asked what is in force, what a document replaced, what rests on it, or why a document fails validation.
---

# docdag

`docdag` treats a directory of Markdown documents with YAML frontmatter as a
typed directed graph and answers questions about it. Prefer it over reading the
whole directory: one command replaces a fan-out of file reads.

Two presets ship. `adr` is the default and manages one directory of decision
records. `spec` manages a normative standard — clauses, the conformance tests
that enforce them, deviations, premises, topics — as eight kinds in eight
directories; a corpus is under it when `docdag.yaml` says `preset: spec`. Every
command below answers for either.

## Vocabulary

- **ref** — any way of naming a document: `339`, `0339`, `ADR-339`,
  `0339-use-postgres.md`. A document's identity is its digit run, so they all
  name the same node, and output is zero-padded to the configured width. Where a
  kind declares a pattern the identifier is that spelling exactly — `UZ-V-001`,
  never padded or folded.
- **binding** — the document is `accepted` and nothing supersedes it. This is
  what "in force right now" means. A `proposed` or `superseded` document is not
  binding, whatever it says. `docdag.yaml` may name a different projection under
  `binding:`; the `spec` preset does, adding the day and the clause's strength.
- **kind** — where `docdag.yaml` declares `kinds:`, the directory a document
  sits in decides what it is and which identity pattern it answers to, so
  `spec/clauses/UZ-V-001.md` is a clause. Single-kind corpora have no kinds.
- **modality** — a clause's strength, one of `MUST`, `MUST_NOT`, `SHOULD`,
  `SHOULD_NOT`, `MAY`. `query --binding` prints it beside the identifier
  wherever the configuration declares it, because a set holding both a
  permission and a prohibition cannot be read without it.
- **as-of** — where `docdag.yaml` declares a `period:`, a document is in force
  between the two days it writes, so what binds is an answer about a day.
  `--as-of YYYY-MM-DD` asks about another one; `--at <rev>` reads the documents
  from a revision instead of the working tree. Without a `period:` neither
  changes anything.
- **resolve** — walk the `supersedes` chain forward to the documents that stand
  in for a ref today. A document nothing supersedes resolves to itself.
- **typed edge** — a relation declared in frontmatter (`supersedes:`,
  `depends-on:`) or derived from a field value (`status: superseded by 0003`).
  Only typed edges carry invariants.
- **reference layer** — plain body links (`[[0042]]`, `[0042](0042-x.md)`).
  Never a constraint, and unvalidated unless `docdag.yaml` sets
  `references.dangling`. Surfaced with `--include-refs`.
- **finding** — one validation result, located at `<path>:<line>`, sometimes
  carrying a one-line `fix:` suggestion.

## Which command to run

| Question | Command |
| --- | --- |
| What am I working with here? | `docdag context <ref>` |
| What is in force right now? | `docdag query --binding` |
| What will be in force on a day? | `docdag query --binding --as-of 2027-04-01` |
| What replaced this decision? | `docdag resolve <ref>` |
| What rests on this decision? | `docdag query <ref> --ancestors` |
| What does this decision rest on? | `docdag query <ref> --descendants` |
| What did the standard say then? | `docdag query --binding --at v1.2.0` |
| Is the corpus sound? | `docdag validate` |
| Did my edit break anything? | `docdag validate --touching <path>` |
| Why is a conflict not reported? | `docdag validate --show-suppressed` |
| Are the rules themselves sound? | `docdag lint` |
| Shape of the corpus | `docdag stats` |
| Which frontmatter keys does it write? | `docdag stats --fields` |

Start with `docdag context <ref>`. It prints the document, the successor it
resolves to if it is superseded, and its neighbourhood over typed edges, each
entry quoting the first paragraph of its Decision section. `--depth 2` widens
the walk, `--all` includes documents that are not binding, `--budget N` caps the
prose at roughly N tokens, `--format md` gives a document to paste.

Take the columns needed rather than the whole record:
`docdag query --binding --fields id,title,status` prints tab-separated columns
in the order asked for. `--format json` answers with an array of objects
carrying every field.

A finding an exception already answers is suppressed — computed, then left out
of the report and out of the exit code. `--show-suppressed` prints it with the
`excepts` edge that answers it on the same line, which is the reading to check
before declaring a conflict unreported.

## Before writing or changing a document

1. `docdag query --binding` — do not contradict a binding document without
   superseding it.
2. `docdag context <ref>` for every document about to be touched.
3. Write the document. A new one supersedes an old one by declaring
   `supersedes: [<id>]` in its own frontmatter, and the superseded document's
   `status:` becomes `superseded`. `docdag new "<title>" --supersedes <ref>`
   does both. On a corpus with kinds, `docdag new --kind clause --id UZ-V-007
   "<title>"` is the form: the kind chooses the directory, the template and the
   identity rules, and `--id` is required wherever the kind declares a pattern.
   What comes back is a stub whose required keys are commented placeholders, so
   its first `validate` reports what only the author can fill in.
4. `docdag validate --touching <path>` — and read the `fix:` line, which names
   what to type.

Never edit a document's identifier, and never renumber. Identity is the file's
digit run, or the pattern its kind declares, so renaming silently rewrites the
graph.

## Reading a finding

```
docs/decisions/0001-serve-images.md:3: ERROR status_drift 0001: has inbound supersedes but status is not superseded
  fix: set status: superseded in docs/decisions/0001-serve-images.md
```

`<path>:<line>` is where to look, the rule name is what is wrong, and `fix:` is
what to do about it. `validate` exits 1 when any finding is an error, 2 on a
usage mistake and 3 when the configuration or the documents directory cannot be
read.

## Setup

`docdag` must be on `PATH`:

```sh
go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.4.0
```

Add this to the project's `.claude/settings.json` so the commands run without a
prompt each time:

```json
{
  "permissions": {
    "allow": ["Bash(docdag *)"]
  }
}
```

This plugin also installs a `PostToolUse` hook that runs
`docdag validate --touching <file>` after every `Edit` or `Write` inside the
documents directory and reports back when the edit broke an invariant. An edit
to `docdag.yaml` is linted instead, with `docdag lint`: what breaks when the
configuration changes is the rules, not the documents. The hook reads its
payload with `jq`; without `jq` on `PATH` it does nothing.

## Changing docdag.yaml

`docdag lint` answers about the rules rather than about the documents, and
`validate` never runs it. It has three layers, and which one to run follows from
what changed:

- `docdag lint` alone — after any edit to `docdag.yaml`. It reads no documents,
  costs milliseconds, and reports a rule that can never fire, one that fires on
  every document, and one that says what another rule already says.
- `docdag lint --corpus` — when the question is whether a rule is doing anything
  here. It evaluates every rule and projection against the vault, so it reads
  the whole corpus.
- `docdag lint --fixtures lint/` — after adding or rewriting a rule. It runs each
  rule's own `ruleid/` and `ok/` corpora. `--all` is the three together.

Write a fixture with every rule added: `docdag new --fixture <rule>` generates
both corpora from the rule's own condition, and a generated document carries a
`<!-- TODO: … -->` wherever the condition is satisfiable in more ways than a
generator should choose between. `lint` exits 1 on an error, 2 on warnings
alone, 3 on a configuration that does not validate.
