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
`ref`, `resolves_to`, `related`, `ancestors`, `descendants`, `suppressed` and `budget`.

Where the configuration declares the normative vocabulary — the `spec` preset does — a `related`
group comes before the walks, naming why each document is in it rather than which direction reached
it: the subject a clause is `about`, the clause it `excepts` or is `excepted by`, the requirement its
option leans on under `interop`, and the other binding clauses about the same subject. It is
reported whether or not those documents bind, because a subject is a definition and an exception is
worth reading exactly where it does not bind on its own. A conflict an exception answers is one line
under `suppressed`:

```console
$ docdag context UZ-V-006
ref
  UZ-V-006  A grader may reason before it scores  [accepted]  spec/clauses/UZ-V-006.md

related
  topic/inferential-grader  Grading by inference  []  spec/topics/inferential-grader.md  (about)
  UZ-V-008  A grader should not reason about the answer it grades  [accepted]  spec/clauses/UZ-V-008.md  (excepts)
  UZ-V-001  Every claim carries evidence  [accepted]  spec/clauses/UZ-V-001.md  (interop)

suppressed
  suppressed by excepts UZ-V-006 -> UZ-V-008 (scope: only where the run also records a calibration measure)
```

## Columns

`resolve` and `query` take `--fields id,title,status,path` and print those columns, tab
separated, in the order asked for; the default is `id` alone, so a pipeline reading identifiers is
unaffected. A projection the configuration declares is a column too, printed as `true` or `false`,
and so is a key it declares under `fields:`, printed as the document writes it and as `-` where the
document writes nothing — see [configuration.md](configuration.md). Under `--format json` these
commands answer with an array of objects carrying every field, plus `"reference": true` on a
reference-layer hit — where v0.1 answered with an array of identifiers.

`query --binding` has a default column set of its own: the identifier, and the `modality` beside it
where the configuration declares one, because a set that spans the modalities cannot be read without
it. Naming `--fields` replaces that default like any other.

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

## Asking about another day

Where a corpus declares a [`period:`](configuration.md#periods-and-as-of), an answer is about a day
rather than standing, and an agent names the day: `--as-of YYYY-MM-DD` on `query`, `resolve`,
`context`, `stats`, `validate` and `lint`. The shipped `spec` vault records a departure that expires
on 2026-08-01, so the two days either side of it are two reports, narrowed by the `--touching` above
to the record the question is about:

```console
$ cd testdata/fixtures/spec-vault
$ docdag validate --touching spec/deviations/dev-0001.md --as-of 2026-07-31
spec/clauses/UZ-V-002.md:4: ERROR orphan_must UZ-V-002: is MUST or MUST_NOT and accepted but nothing enforces it
as of 2026-07-31
(6 findings hidden)

$ docdag validate --touching spec/deviations/dev-0001.md --as-of 2026-08-02
spec/clauses/UZ-V-002.md:4: ERROR orphan_must UZ-V-002: is MUST or MUST_NOT and accepted but nothing enforces it
spec/deviations/dev-0001.md:7: WARN expired_deviation dev-0001: expires 2026-08-01 has passed and the status is still accepted
as of 2026-08-02
(6 findings hidden)
```

`query --binding --as-of <day>` asks the same question of the binding set, which is how to find out
what a change lands into — what is in force on the day a revision takes over — before writing
anything.

`--at <rev>` is the other axis: every managed document read from a revision rather than from the
working tree, which is what "what did the standard say then" needs. The two compose. `validate` and
`lint --corpus` answer for the day HEAD was committed on unless told otherwise, so an agent
reproducing a CI failure locally gets the same findings the gate did; the JSON reports carry `as_of`
and `at` so an answer says which corpus at which moment it came from.

## Changing the rules

`docdag lint` is the command for an edit to `docdag.yaml` rather than to a document. It reports the
rules that cannot fire, the ones that fire on everything, and the ones that say what another rule
already says — with `--corpus` the ones the vault never fires, and with `--fixtures` the ones whose
own fixtures disagree with them. `validate` never runs it, so a rule change is checked by asking:

```console
$ docdag lint
docdag.yaml:12: ERROR unfirable_rule orphan_should: every alternative contradicts itself: inbound enforces and not_inbound enforces cannot both hold
  fix: drop the rule, or the clause that contradicts the rest
```

An agent adding a rule should add its fixture in the same edit: `docdag new --fixture <rule>` writes
the `ruleid/` corpus the rule has to fire in and the `ok/` corpus it must not, derived from the
rule's own condition, and `docdag lint --fixtures lint/` reads them back. The pair is what keeps the
intent of a rule readable after the rule is rewritten — see
[commands.md](commands.md#lint).

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

The hook reads what was edited: a Markdown file inside the documents directory is validated, and
`docdag.yaml` itself is linted — `docdag lint --config <the file that was edited>` at layer 1, which
reads no documents and costs milliseconds — so a rule that contradicts itself is reported in the same
turn it was written. The edited file is named explicitly, so a nested configuration is linted rather
than the project root's. An edit that leaves it unreadable at all says so on stderr instead of
passing quietly.

The hook needs `docdag` and `jq` on `PATH` and does nothing without them. Allow the commands once
with `"permissions": {"allow": ["Bash(docdag *)"]}` in `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["Bash(docdag *)"]
  }
}
```
