---
name: docdag
description: Read, query and validate a repository's decision records as a typed graph with the docdag CLI. Use it before writing or changing an ADR, when asked what decision is in force, what a decision replaced, what rests on it, or why a decision record fails validation.
---

# docdag

`docdag` treats a directory of Markdown documents with YAML frontmatter as a
typed directed graph and answers questions about it. Prefer it over reading the
whole directory: one command replaces a fan-out of file reads.

## Vocabulary

- **ref** — any way of naming a document: `339`, `0339`, `ADR-339`,
  `0339-use-postgres.md`. A document's identity is its digit run, so they all
  name the same node. Output is zero-padded to the configured width.
- **binding** — the document is `accepted` and nothing supersedes it. This is
  what "in force right now" means. A `proposed` or `superseded` document is not
  binding, whatever it says.
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
| What replaced this decision? | `docdag resolve <ref>` |
| What rests on this decision? | `docdag query <ref> --ancestors` |
| What does this decision rest on? | `docdag query <ref> --descendants` |
| Is the corpus sound? | `docdag validate` |
| Did my edit break anything? | `docdag validate --touching <path>` |
| Shape of the corpus | `docdag stats` |

Start with `docdag context <ref>`. It prints the document, the successor it
resolves to if it is superseded, and its neighbourhood over typed edges, each
entry quoting the first paragraph of its Decision section. `--depth 2` widens
the walk, `--all` includes documents that are not binding, `--budget N` caps the
prose at roughly N tokens, `--format md` gives a document you can paste.

Take the columns you need rather than the whole record:
`docdag query --binding --fields id,title,status` prints tab-separated columns
in the order you asked for. `--format json` answers with an array of objects
carrying every field.

## Before writing or changing a decision record

1. `docdag query --binding` — do not contradict a binding decision without
   superseding it.
2. `docdag context <ref>` for every decision you are about to touch.
3. Write the document. A new one supersedes an old one by declaring
   `supersedes: [<id>]` in its own frontmatter, and the superseded document's
   `status:` becomes `superseded`. `docdag new "<title>" --supersedes <ref>`
   does both.
4. `docdag validate --touching <path>` — and read the `fix:` line, which names
   what to type.

Never edit a document's identifier, and never renumber. Identity is the file's
digit run, so renumbering silently rewrites the graph.

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
go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.2.0
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
documents directory and reports back when the edit broke an invariant. The hook
reads its payload with `jq`; without `jq` on `PATH` it does nothing.
