# For agents

An agent should ask the graph rather than read the directory: one command replaces a fan-out of file
reads, and every answer carries the identity, the status and the position of the documents it names.
See [commands.md](commands.md) for the full command surface and [checks.md](checks.md) for the
findings an agent has to read.

## context

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
document, `--format json` with `schema_version`, `preset_version` where the configuration names one,
`ref`, `resolves_to`, `ancestors`, `descendants` and `budget`.

## Columns

`resolve` and `query` take `--fields id,title,status,path` and print those columns, tab
separated, in the order asked for; the default is `id` alone, so a pipeline reading identifiers is
unaffected. A projection the configuration declares is a column too, printed as `true` or `false` —
see [configuration.md](configuration.md). Under `--format json` these commands answer with an array of objects carrying every
field, plus `"reference": true` on a reference-layer hit — where v0.1 answered with an array of
identifiers.

```console
$ docdag query --binding --fields id,title,status
0001	Cache rendered thumbnails	accepted
0003	Store thumbnails in object storage	accepted
```

## One edit at a time

`docdag validate --touching <path>` runs the whole corpus and reports only the findings about that
file, the files a finding relates to it, and the documents one typed edge away — what changing it
can break. The flag repeats and accepts a directory. The exit code still answers for the corpus, so
a narrowed report never turns a failing repository into a passing one, and the number of findings
withheld goes to stderr. The summary line and the `summary` object answer for the corpus too: only
the list of findings narrows.

## Writing a record

`docdag new "<title>" --supersedes <ref>` writes the new document and rewrites the superseded one's
status in a single pass — see [commands.md](commands.md#new) for the plan, `--dry-run` and the
identifier rules.

## The plugin

This repository is a Claude Code plugin. It installs a skill teaching the vocabulary above and which
command answers which question, plus a `PostToolUse` hook that runs `docdag validate --touching` on
every Markdown file written inside the documents directory and reports back when that document is
what broke:

```console
$ claude
/plugin marketplace add Kaikei-e/DocDag
/plugin install docdag@docdag
```

The hook needs `docdag` and `jq` on `PATH` and does nothing without them. Allow the commands once
with `"permissions": {"allow": ["Bash(docdag *)"]}` in `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["Bash(docdag *)"]
  }
}
```
