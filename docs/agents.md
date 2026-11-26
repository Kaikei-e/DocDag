# For agents

An agent should ask the graph rather than read the directory: one command replaces a fan-out of file
reads, and every answer carries the identity, the status and the position of the documents it names.
See [commands.md](commands.md) for the full command surface and [checks.md](checks.md) for the
findings an agent has to read.

## prime

`docdag prime` is the session-start orientation command. It inspects the corpus and configuration
and prints a compact (~10–14 line) brief naming the validation status, managed directories or kinds,
declared edges/fields/required sections, and corpus-tailored `docdag` commands with a real sample
reference from the vault:

```console
$ docdag prime
docdag validate: OK: 10 docs, 10 typed edges, no cycles
corpus: docs/ADR (preset: adr, 10 docs, 10 binding)
edges: depends-on, supersedes
required sections: コンテキスト, 決定, 検討した代替案|代替案と棄却理由, 影響とトレードオフ|帰結
Prefer docdag over Glob/Grep/Read on managed docs (one call replaces reading the directory):
  docdag context <ref> (e.g. docdag context 0001)
  docdag query --binding --fields id,title,status
  docdag resolve <ref> --fields id,title,status
  docdag query <ref> --ancestors --fields id,title,status  # impact
  docdag new "<title>" [--supersedes <ref>]
  docdag validate --touching <path>
  docdag diff origin/main
```

`prime` is a report rather than a gate: it exits `0` whenever the corpus loads (reporting
`docdag validate: FAILED: …` on line 1 when there are errors), and fails closed with exit `3` on
invalid configuration or an empty corpus. `--if-present` exits `0` silently when no corpus is
discovered (while still failing closed on broken `docdag.yaml`). `--hint <path>` resolves only the
configuration (without parsing any documents) and prints a short 1–2 line tip when `<path>` or a
glob touches a managed directory, which hooks use to steer `Read`/`Glob`/`Grep` calls toward
`docdag context` and `docdag query --binding`. `--format json` emits a versioned JSON object
(`schema_version: 1`).

## context

`docdag context <ref>` is the single-record entry point. `<ref>` accepts an identifier (`0002`,
`ADR-2`, `UZ-V-006`), a filename, or a managed file path (`docs/ADR/0002-….md`,
`spec/clauses/UZ-V-006.md`). It prints the document, the successor it resolves to if it is
superseded, and its neighbourhood over typed edges, each entry followed by a verbatim, unsummarised
excerpt:

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
Without `--section`, `context` quotes the first paragraph under `Decision` (preferring MADR's
`Decision Outcome` over `Decision Drivers`), and falls back cleanly for other layouts: localized
decision/summary headings (`決定`, `結論`, `Summary`, `概要`), child subheadings (such as `### D1.`
immediately under `## 決定`), the opening body paragraph for H1-only documents such as `spec`
clauses, or the first substantive section. Passing `--section <heading>` matches strictly that H2
or H3 heading prefix without falling back.
`--budget N` caps the prose at roughly N tokens, counted as four characters each and defaulting to
2000; entries the budget cannot afford degrade to their one-line form rather than being cut
mid-sentence, and the report ends with a line counting them. `--format md` answers with a Markdown
document, `--format json` with `schema_version`, `preset_version` where the configuration names one,
`ref`, `resolves_to`, `related`, `ancestors`, `descendants`, `suppressed`, `retracted` (when active
edge retractions touch the document or are declared by it, as objects
`{"edge": "...", "from": "...", "to": "...", "by": "..."}`), and `budget`. In text (`retracted`
group) and Markdown (`## Retracted`), each entry prints `<edge> <from> -> <to> (by <by>)`.

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
    The explicit permission: an option a standard states rather than leaves out.
    It stands beside the clause that says graders should not reason, because the
    excepts edge below records the case the permission is for — and it names the
    MUST clause a repository without the option still interoperates through.

related
  topic/inferential-grader  Grading by inference  []  spec/topics/inferential-grader.md  (about)
    Whether a grader may reason its way to a score rather than match a fixed
    pattern. The subject two clauses of this vault disagree about, on purpose.
  UZ-V-008  A grader should not reason about the answer it grades  [accepted]  spec/clauses/UZ-V-008.md  (excepts)
    The general clause: an inferential grader is one more thing that can be wrong.
    UZ-V-006 is the recorded exception to it, which is what keeps the pair from
    being reported as a conflict every day.
  UZ-V-001  Every claim carries evidence  [accepted]  spec/clauses/UZ-V-001.md  (interop)
    A report states what it measured and how, so a reader can repeat the run.

suppressed
  suppressed by excepts UZ-V-006 -> UZ-V-008 (scope: only where the run also records a calibration measure)
```

## Columns

`resolve` and `query` take `--fields id,title,status,path` and print those columns, tab
separated, in the order asked for; the default is `id` alone, so a pipeline reading identifiers is
unaffected. A projection the configuration declares is a column too, printed as `true` or `false`,
and so is a key it declares under `fields:`, printed as the document writes it (scalar as a string,
list items joined with `,` and no spaces, and `-` where the document writes nothing or a mapping) —
see [configuration.md](configuration.md). Under `--format json` these commands answer with an array
of objects carrying every configured projection under `projections` (`"true"` or `"false"`) and every
declared field under `fields` (string, array of strings, or `null` when unwritten or a mapping;
omitted when none are configured), whatever `--fields` says (`--fields` is still validated), plus
`"reference": true` on a reference-layer hit — where v0.1 answered with an array of identifiers.

`query --binding` has a default column set of its own: the identifier, and the `modality` beside it
where the configuration declares one, because a set that spans the modalities cannot be read without
it. Naming `--fields` replaces that default like any other.

`query` also takes `--where <key>=<op>:<value>` (repeatable, ANDed) to filter results after the
walk or binding set is computed, preserving order (including `--include-refs` hits). The operators
are the rule `when: attr:` vocabulary — `eq`, `not`, `contains`, `not_contains`, `subset_of`
(comma-separated list) — with identical case-insensitive, scalar-as-list, projection-shadowing,
and `--as-of` semantics. Allowed keys are the configured status field, declared fields, configured
projections, and `in_force`.

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

## The plugin (Claude Code and Antigravity)

This repository is a dual-native plugin for both **Claude Code** (`.claude-plugin/plugin.json`,
`hooks/hooks.json`) and **Antigravity (`agy`)** (`.antigravity-plugin/plugin.json`, root
`hooks.json`, `rules/AGENTS.md`), sharing one Agent Skill ([`skills/docdag/SKILL.md`](../skills/docdag/SKILL.md)).

### Claude Code

```sh
claude plugin marketplace add Kaikei-e/DocDag
claude plugin install docdag@docdag
```

Inside a session the same two steps are `/plugin marketplace add Kaikei-e/DocDag` and then
`/plugin install docdag@docdag`, entered as separate prompts: pasted together, they arrive as one
command and the marketplace source is rejected.

The Claude Code plugin installs three hooks:

- **`SessionStart`** (`scripts/docdag-session-start.sh`, no matcher): silent when the project has
  neither a root `docdag.yaml` nor a well-known documents directory (`docs/adr`, `doc/adr`,
  `docs/decisions`, `docs/ADR`, `adr`). When a corpus marker is present, it runs `docdag prime`
  (falling back to a one- or two-line `docdag validate` summary on older `docdag` binaries) and
  always exits `0`. If `docdag` is missing it prints `docdag: not on PATH, enforcement is OFF`;
  on unexpected exits (such as an empty well-known ADR directory or invalid config) it prints
  `docdag validate: exited <N>: <first line of stderr>`.
- **`PreToolUse`** (`scripts/docdag-pre-read.sh`, matcher `Read|Glob|Grep`): non-blocking advisory
  hook (always exits `0` and omits `permissionDecision` so existing permission rules stay intact).
  When a read, glob, or grep targets a managed document or directory, it runs
  `docdag prime --if-present --hint <target>` (config-only, no document parsing) and injects a
  short `hookSpecificOutput.additionalContext` tip pointing to `docdag context` and
  `docdag query --binding --fields id,title,status`. Outside managed paths it stays silent.
- **`PostToolUse`** (`scripts/docdag-validate.sh`, matcher `Edit|Write`): a Markdown file is passed
  to `docdag validate --touching <file> --if-managed`, which validates when the file sits directly
  in a configured or discovered document directory (including `preset: spec` and per-kind `dir:`
  directories) and exits `0` silently otherwise; `docdag.yaml` itself is linted with
  `docdag lint --config <edited file>` at layer 1. On findings or unexpected `docdag` exit codes
  (such as exit `3` on configuration or I/O failure), the hook fails closed with exit code `2` and
  outputs diagnostics on stderr.

The `PreToolUse` and `PostToolUse` hooks need `docdag` and `jq` on `PATH`. If either tool is
missing (or `docdag` predates `--if-managed`), `PostToolUse` emits a non-blocking stderr notice
that enforcement is OFF and exits `0`, and `PreToolUse` stays silent. Allow the commands once with
`"permissions": {"allow": ["Bash(docdag *)"]}` in `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["Bash(docdag *)"]
  }
}
```

### Antigravity (`agy`)

Install the plugin directly from the repository (or a local checkout):

```sh
agy plugin install https://github.com/Kaikei-e/DocDag
# or from a local clone:
agy plugin install /path/to/DocDag
```

The Antigravity plugin bundles:

- **Always-on rule** (`rules/AGENTS.md`): compact routing rules merged automatically whenever the
  plugin is enabled, directing the agent to `docdag prime`, `context`, `query --binding --fields`,
  `resolve`, `new`, `validate --touching`, and `lint` whenever a workspace has a DocDag corpus.
- **Shared skill** (`skills/docdag/SKILL.md`): full workflow reference loaded on demand.
- **Native hooks** (root `hooks.json`):
  - `SessionStart` (`scripts/agy-session-start.sh`): runs `docdag prime` when a corpus marker is
    present and injects the brief via `injectSteps` (`systemMessage`).
  - `PostToolUse` (`scripts/agy-post-tool.sh`) + `PostInvocation` (`scripts/agy-post-invocation.sh`):
    validates managed `.md` edits (`validate --touching --if-managed`), lints `docdag.yaml` edits
    (`lint --config`), and queues a one-time-per-conversation `docdag prime --hint` tip on managed
    reads/searches, draining queued messages as `injectSteps` on the immediately following
    `PostInvocation`.
