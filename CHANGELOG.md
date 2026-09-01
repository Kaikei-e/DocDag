# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-09-01

v0.3.0 takes the graph past one directory of decision records. A corpus may hold several kinds of
document, each with an identity, a status vocabulary and a frontmatter of its own; an edge may carry
attributes and state what it requires of the document it points at; a projection may derive what
*binding* means instead of leaving the definition in the engine; and a document may declare the days
it is in force, so what is current becomes an answer about a day rather than a standing fact. The
second preset, `spec`, is what those keys were added for: a normative standard as a graph, where a
`MUST` no conformance test enforces is a finding rather than a rule. It is also the first release
with architecture decision records of its own — the five under [docs/adr/](docs/adr/) record what
each phase decided, and why none of them reached for an expression language. It is a **breaking
release**: the JSON reports move to `schema_version: 2` and gain headers, and a configuration that
adopts the new keys fails on documents that used to pass. The migration section at the end lists
every one.

### The `spec` preset, and a corpus of several kinds

`kinds:` says that several sorts of document share one corpus, each in a directory of its own and
each with its own idea of an identifier — a pattern matched against the whole identifier, so
`UZ-V-001` and `conform/uz-v-001` name documents as surely as a digit run does, and a kind that
declares none keeps the digit run every corpus had before. The directory decides which kind a
document is, because the directory is what chose the identity rules the document was read under, and
`kind` is readable from every condition as an attribute. A kind may carry its own `status_values`,
so clauses can be `trial` while deviations keep the preset's words, and `closed: true` makes its
frontmatter a closed set, where a key nobody declared is a finding rather than another tool's field.
`from:` and `to:` hold an edge's endpoints to a set of kinds. The four findings this turns on —
`id_mismatch`, `kind_mismatch`, `unknown_field`, `edge_kind_mismatch` — exist only where a corpus
declares kinds, and a configuration that declares none is read exactly as it was.

An edge may declare `attrs:`, and an entry under such a key is then either a plain reference or a
mapping naming one — `{ref: "0001", reason: conflict}` — with `required`, a `type` of `string`,
`number` or `date`, and a closed `one_of`, checked as `edge_attr_unknown`, `edge_attr_missing` and
`edge_attr_invalid`. An attribute is a fact about the relation rather than about either end of it,
which is why a lifetime is not one: a record that departs from two clauses has one lifetime and not
two. `export --format json` carries the attributes on the link; an edge that declares none — every
edge of the `adr` preset — takes plain references and nothing else.

`fields:` declares the frontmatter keys the documents write: the closed `one_of` a value comes from,
`required:` for a key a document has to write, and `deprecated:` with `since`, `migrate_to` and
`sunset` for one being retired, which is `deprecated_field` — a warning until the sunset day and an
error after it. A declared key is a known key, so a `closed: true` kind accepts it: a migration in
progress is not a mistake. `preset_version:` is the revision of the configuration a corpus is
written against, an integer DocDag never interprets and only writes into the JSON headers, so a
repository can pin what its documents were checked under; whether a revision is compatible is
answered by running `validate` under both and diffing the findings, rather than by a feature.
`docdag stats --fields` counts the corpus by field, with the day each field's documents last changed
taken from `git log`, because a removal is decided on numbers and a declared field nobody writes is
what a finished migration looks like.

`preset: spec` assembles all of it into the second built-in configuration: eight kinds — clause,
topic, conform, deviation, measure, premise, principle and post-mortem — edges declared on the
machine-generated side, so the side that changes rarely stays out of the diff, and force derived
from the graph rather than written down in any document. `docdag new --kind <name>` writes a
document of one kind, under that kind's directory and identity rules, offering the fields and edges
it may declare as commented stubs: a placeholder written as a value would be the first finding of a
document nobody made a mistake in.

### Projections, and a vocabulary that stayed closed

`projections:` derives a named boolean attribute from the same `when` vocabulary a rule writes, and
`binding:` names the projection that defines the documents in force — what `query --binding` lists,
what `stats` counts and what `context` keeps. Binding used to be a definition held in the engine;
the `adr` preset now writes it down as `accepted_unsuperseded` and answers identically, and a corpus
that means something else by "in force" says so rather than forking the engine. Projections read
each other and are evaluated in dependency order, so the list may be written in any order and a
reference cycle among them is a configuration error. A projection is a column of `query --fields`
and `resolve --fields` as well, since a derived attribute a rule can read is one a pipeline can.

The condition vocabulary grew and stayed closed. `inbound` and `outbound` read a degree threshold —
`{edge: deviates-from, min: 5}` — as well as an edge name, and `via` and `via_inbound` reach exactly
one hop to test a neighbour's attributes, with no `via` inside a `via`, so a condition stays a
question about a document and its immediate neighbourhood. An edge spec's `target:` writes the same
vocabulary against the document the edge points at, with `leaf_of: <edge>` as the spelling of
`not_inbound:` that keeps the intent and earns a fix naming the lineage's current leaf; a violation
is `stale_target`, the check staying local while only the suggestion is transitive.
`path_constraints:` states what a target condition cannot — an invariant over two composed edges,
`equals: none` or `subset_of:` a second path — and reports `path_mismatch` naming the documents one
path reaches and the other does not, without guessing which of the two is the wrong one. Both paths
are one or two steps long: a longer one is a regular path expression whose implication problem is
undecidable, and transitive reach is what `resolve` already answers. There is still no expression
language — no arithmetic, no string operations, no variables — and the modal depth stays fixed at
two, an edge and then a local condition, which is the bar every future word is reviewed against.

### Modality, and a prohibition that can be seen to collide with a permission

A clause states its strength in `modality:`, one of BCP 14's five keywords — `MUST`, `MUST_NOT`,
`SHOULD`, `SHOULD_NOT`, `MAY` — a closed set of five rather than a strength and a polarity, because
BCP 14 has no `MAY NOT` and a pair of fields would have an invalid combination to check for. `MAY`
is a node like any other, which is what lets a later prohibition be seen to collide with a
permission rather than silently overwrite it: an explicit permission does not follow from the
absence of a prohibition, so it has to be written down to exist. The subject a clause speaks to is a
`topic` document reached by `about:`, with `min_outbound: 1` on that edge, because two clauses can
only be seen to disagree where they are known to be about one thing — and a misspelled subject is
then a `dangling_ref` rather than a second subject nobody notices.

`modality_conflict` reports two binding clauses about one topic whose modalities cannot both hold:
one forbids and the other does not. The collision is *strong* where both are strict rules, which is
`MUST` against `MUST_NOT`. A weak one the corpus has already answered — an `excepts:` edge between
the two clauses, carrying the `scope:` prose that says when the exception applies — is suppressed:
computed, then left out of the report, out of the summary counts and therefore out of the exit code,
and shown by `validate --show-suppressed` with the edge that answers it named on the same line.
DocDag never reads that prose; it records it, and `context` shows it. A strong conflict is never
suppressed, and `excepts_strict` rejects an exception recorded against a strict rule, since a
defeater stops a defeasible conclusion being drawn and a strict one has nothing for it to stop.
`interop:` records the requirement a permission leans on, because RFC 2119 makes interoperation with
and without an option a MUST-level obligation, and a `MAY` that names none is a warning.

The commands say which is which. `query --binding` prints the modality beside each identifier, since
a set that spans the modalities cannot be read without it; `stats` gains a row per declared modality
— at zero where nobody states it, a standard with no `MUST_NOT` being a fact about the standard —
the clauses per topic, which is what topic granularity is watched with, and the number of conflicts
a recorded exception answers; and `context` gains a `related` group naming why each document is in
it rather than which direction reached it.

### `docdag lint`, and a sensor that can prove it can ring

A check that never fires says nothing on its own about whether the corpus is healthy or the check is
dead — the question a harness's own coverage asks, and the one formal verification calls vacuity.
`docdag lint` reads `docdag.yaml` rather than the documents and answers it in three layers. Layer
one expands every condition into disjunctive normal form over the finite vocabularies the
configuration declares and reports what contradicts itself, the rules no document could fire, the
rules that fire on everything, the rules that say what another rule already says, two remedies that
would write different values into one key, and the declarations nothing reads — a set operation over
finite domains, with no solver and no search, which is the direct return of a vocabulary with no
expression language in it. Layer two, `--corpus`, evaluates every rule and projection against the
vault and reports what never fires and what always does, counted against the documents a rule could
apply to rather than against the whole vault, with `--since <rev>` naming what started and stopped
firing. Layer three, `--fixtures`, runs each rule against a `ruleid/` corpus it must fire in and an
`ok/` corpus it must not — Semgrep's names — and `docdag new --fixture <rule>` generates both from
the rule's own condition, writing a `TODO` for whatever it could not derive rather than guessing.

Together the layers answer what a silent rule leaves open: a fixture proves a rule *can* fire, the
corpus shows it is not firing now, and a `never_fired` rule whose fixture passes falls to `info`.
`info` is a severity of lint's own — it never raises an exit code and `--strict` does not raise it
either, because a fact about the corpus is not a fault. The exit codes are `0`, `1` for an error,
`2` for warnings alone and `3` for a configuration that does not validate. `validate` never runs any
of it: a configuration's health and a corpus's state have different lifecycles, and a lint warning
on every pull request is a warning nobody reads. A finding is located on the line of `docdag.yaml`
the rule was written on, or at the virtual path `<preset:adr>` or `<preset:spec>` where there is no
file to open, and both shipped presets lint clean, which a test holds them to.

### Periods, and an answer that names its day

`period:` names the two frontmatter keys a kind's documents write the days they are in force
between, read as a closed-open interval of ISO calendar days: in force on the day it begins and not
on the day it ends. The two values are key names rather than days, because the days are facts about
each document. From the period and the day a run is about, the engine computes `in_force` and
exposes it as an attribute that reads exactly like a projection — the one attribute it computes
rather than reads, which is what keeps the date comparison inside the engine and the vocabulary free
of date literals. An end nobody wrote is derived from the earliest day an accepted successor begins
on and never written back into the frontmatter; a successor that is withdrawn stops deriving one, so
the predecessor's period opens up again with nothing rewritten but the successor's own status. Once
a document has left force its statements lose their weight — its edges stop counting for the degree
thresholds, the one-hop clauses and the recorded exceptions — with the `supersedes` lineage exempt,
being what the ends are derived from, and `path_constraints` exempt, a statement about the shape of
the corpus not being a claim about what holds today. `period_invalid`, `period_conflict` and
`expired_deviation` are the three findings a period turns on.

`--as-of YYYY-MM-DD` names the day any read-only command answers for, `$DOCDAG_AS_OF` names it for a
whole pipeline, and `--at <rev>` reads every managed document from a revision instead of from the
working tree. The two are the valid time and the transaction time of a temporal database and are
independent, so writing both asks what the vault at that revision said was in force on that day.
`validate` and `lint --corpus` default to the day HEAD was committed on, so one commit gates the
same way however long afterwards the job runs; the listings default to the day they run, a listing
being a question about now. An expiry noticed as it happens is therefore a scheduled run that says
`--as-of $(date -I)` out loud. Every JSON report carries `as_of`, and `at` where a revision was
named, which is what makes an answer reproducible. The `spec` preset moves to revision 2 on the
strength of it: `expires` belongs to the deviation rather than to the edge, `stale_premise` reads
the day instead of a status, and `status_drift`, `pending_successor` and `premature_superseded` read
whether a successor is in force rather than whether it exists — which is how "the successor is
proposed, and the predecessor binds until it takes over" is finally sayable.

### Fixed

- A degree bound on an edge that names its endpoint kinds is read over those kinds alone: the
  outbound bounds over `from:`, the inbound ones over `to:`. Without the scoping, `min_outbound: 1`
  on an edge from one kind reported every document of every other kind, the edge's own targets
  included — a lower bound is the one bound a document with no such key at all can violate. A
  document of another kind that does hold such an edge is an `edge_kind_mismatch`, which says the
  actual mistake. An edge that names no kinds, and every corpus without `kinds:`, is bounded over
  the whole corpus exactly as before.

### Migrating from 0.2.0

- **JSON**: `validate` and `context` move to `"schema_version": 2` and are headed by
  `preset_version` — the `adr` preset is revision 1, so the field appears on every report — `as_of`,
  the day the run answered for, and `at` where `--at` named a revision. `lint --format json` is a
  report of its own kind, carrying `"kind": "lint"` and a `schema_version` of its own, so a consumer
  can never read it as a validation report. The listings (`query`, `resolve`) are arrays of records
  and carry no header; `stats --format json` carries `as_of` and `at`.
- **Text report**: unchanged for a corpus that declares no `period:`. Where one is declared the
  closing line carries `, as of <day>` and a failing run ends with `as of <day>` alone, because a
  report a person reads has to say which day it is about. A finding about a file that yields no
  identifier at all — possible only under `kinds:` — has none to name, so its line reads
  `<path>:<line>: ERROR <rule>: <detail>`.
- **Now fails where it used to pass**, each of them opted into by writing a key: with `kinds:`, a
  file in a kind's directory whose identity the kind's pattern rejects (`id_mismatch`), a `kind:`
  the directory disagrees with (`kind_mismatch`), a key nobody declared on a `closed: true` kind
  (`unknown_field`) and an edge endpoint of the wrong kind (`edge_kind_mismatch`); with `fields:`, a
  value outside a `one_of` (`unknown_field_value`), an absent `required:` key (`missing_field`) and
  a retired key (`deprecated_field`, an error after its `sunset`); with `attrs:` on an edge, an
  unknown attribute and a value the declaration rejects, and — where an attribute is
  `required: true` — every plain reference already in the corpus (`edge_attr_missing`), which is the
  migration: rewrite the entries, then require the attribute; with `target:` or `path_constraints:`,
  `stale_target` and `path_mismatch`; with a `period:`, `period_invalid`, `period_conflict` and
  `expired_deviation`.
- **Map elements**: an entry under an edge key that is a mapping is read as an attributed reference
  only where that edge declares `attrs:`. Under any other edge it names no document and stays what
  it has always been, a `dangling_ref`.
- **List replacement** now covers `projections:`, `path_constraints:` and the `kinds:` and `fields:`
  maps as well as `edges:`, `rules:` and `derived_edges:`: writing one describes every entry the
  corpus has, and writing it empty (`kinds: {}`) clears the preset's without putting anything in its
  place. `preset_version:`, `binding:` and `period:` are single values, so writing one replaces the
  preset's and leaving it out keeps it. Writing `edges:` additionally drops the inherited
  projections that read an edge type the new list does not declare, and the `binding:` that named
  one of them, since a projection over a vocabulary the corpus replaced away cannot be evaluated.
- **`new`** requires `--kind` on a corpus that declares `kinds:` — which kind to create, under which
  identity rules and from which template, has no default answer — and `--id` with it for a kind that
  declares an `id:` pattern, a pattern being a spelling rather than a sequence. `--supersedes` and
  `--depends-on` refuse an edge that declares a required attribute (exit 3): the entry would be
  incomplete and a creation has no value to put there.
- **`validate --immutable-since`** refuses a corpus that declares `kinds:`: it reads one directory
  under one identity rule.
- **`query --binding`** prints a `modality` column beside the identifier where the configuration
  declares that field, and `stats` gains its modality, topic and suppressed-conflict blocks only
  where the configuration declares what they read — so an `adr` corpus's report is what it was.
  Naming `--fields` replaces the default column set as it always did.
- **Configuration**: every new key is optional and off; an unchanged `docdag.yaml` reports what it
  reported, apart from the headers above. Pin the action or
  `go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.3.0`.

11 commits, 249 files, +26,910 / −519. Test suite 315 → 534 test functions. No new dependencies.

## [0.2.0] - 2026-08-27

v0.2.0 turns `docdag validate` from a pass/fail line into a check whose findings a reviewer, a CI
annotation and an agent can each act on, and extends the constraint layer far enough that a decision
log's own conventions — reciprocal edges, append-only history, a bounded vocabulary — are enforced
rather than described. It is a **breaking release**: the text and JSON output formats change, and a
few things that used to pass now fail. The migration section at the end lists every one.

### Every finding now has a place

- Findings carry `path`, `line` and, where it applies, `column`, taken from the YAML frontmatter
  itself. The text line is `path:line: SEVERITY rule id: detail`, so an editor or a problem matcher
  can jump to it. Findings that involve several files (`cycle`, `id_collision`, `inverse_mismatch`)
  list the others under `related`.
- `--format github` prints one workflow command per finding —
  `::error file=…,line=…,title=status_drift::…` — so a pull request shows the problem inline. GitHub
  caps inline annotations at ten per step; pair it with the text report in the step summary.
- `--format rdjson` emits reviewdog's Diagnostic Format for review comments that escape that cap.
- Findings that have a mechanical remedy say so: `fix: did you mean 0042?`,
  `fix: set status: superseded in docs/adr/0001-….md`. The text report prints it indented under the
  finding; JSON carries it as `fix`.
- The JSON report is versioned (`"schema_version": 1`).

### A larger constraint vocabulary, still without an expression language

- **`inverse:`** on an edge declares the frontmatter key the *target* must carry back
  (`amends` ↔ `amended_by`). Both directions are checked pairwise; a missing or extra entry is
  `inverse_mismatch`. The inverse key creates no edges of its own.
- **`references:`** opts the reference layer into validation. `dangling: warn|error` reports a body
  or frontmatter link that names no document; `pattern` says what an identifier-shaped target looks
  like, so `[[upstream]]`, `[[3days-recap]]` or a footnote `[[1]]` are never references;
  `scan: [body, frontmatter]` extends the walk to the wikilinks in frontmatter scalars and list
  items. Off by default: prose still cannot fail a build unless a configuration says so.
- **Cardinality** — `max_inbound`, `max_outbound`, `min_outbound` — bounds the degree of an edge
  type. The bounds are opt-in and default to unbounded; an edge type that declares `max_inbound: 1`
  reports a second document superseding the same one as a `cardinality` error.
- **`acyclic_union: true`** additionally checks for cycles across the union of the acyclic edge
  types, so `A amends B, B supersedes A` is caught.
- **List attributes**: rules can read `tags:`-style lists with `contains`, `not_contains` and
  `subset_of`; `eq`/`not` stay scalar.
- **`any_of:`** and **`not:`** nest inside `when:`, so a rule can express an alternative and a
  negation without an expression language. The vocabulary is still fixed and complete; the README
  restates it.
- **`structural:`** raises the severity of a built-in check (`missing_frontmatter: error`). Lowering
  one, or naming a check that does not exist, is a configuration error.
- **`empty_edge`**: an edge key written down and left empty (`supersedes:` with nothing under it,
  `[]`, or blank items) is an error instead of a silently absent edge.
- **`withdrawn`** joins the preset status vocabulary for a decision that was dropped rather than
  replaced. It binds nothing and raises no `superseded_orphan`.
- An explicitly empty list in `docdag.yaml` (`derived_edges: []`) now clears the preset's list, as
  the README always said it did.

### Append-only history

`docdag validate --immutable-since <rev>` treats a document that was `accepted`, `superseded` or
`withdrawn` at `<rev>` as a record. Against `git merge-base <rev> HEAD`, such a document may only
change its status value, gain entries under a configured `inverse:` key, or grow lines at the end of
its body; anything else — a rewritten paragraph, another frontmatter key, a deletion or rename — is
an `immutable_violation` naming what changed. It execs `git` from `PATH` and needs full history in
CI (`fetch-depth: 0`); a corpus outside a git repository, or a machine without `git`, exits 3. Off
unless the flag is given.

### For agents

- **`docdag context <ref>`** answers "what is decided around this?" in one call: the document, what
  it resolves to if superseded, and its typed-edge neighbourhood, each entry followed by the first
  paragraph of its `Decision` section, verbatim. `--budget` caps the prose in tokens and degrades
  whole entries rather than cutting mid-sentence; `--format md|json` for prompts and pipelines.
- **`--fields id,title,status,path`** on `resolve` and `query` prints those columns tab-separated.
  Under `--format json` these commands now return objects, not bare identifiers.
- **`validate --touching <path>…`** runs the whole corpus and reports only the findings about the
  given files and their typed-edge neighbours. The exit code and the summary still answer for the
  corpus, and the number of findings withheld goes to stderr.
- **The repository is a Claude Code plugin**: a skill teaching the vocabulary and a `PostToolUse`
  hook that runs `docdag validate --touching` on every Markdown file written under the documents
  directory. `/plugin marketplace add Kaikei-e/DocDag`, then `/plugin install docdag@docdag`.

### `new`, `export`, distribution

- **`filename:`** in `docdag.yaml` (`"{id}.md"` for bare-numeric corpora) chooses what `docdag new`
  names a document; the default `{id}-{slug}.md` is unchanged.
- **`new --dry-run`** prints the plan — identifier, path, the rewrites it would apply — as text or
  JSON and writes nothing. **`new --id <ref>`** creates under a chosen identifier and is idempotent:
  same title, exit 0 and no write; different title, exit 1.
- **`export --connected`** drops documents no typed edge touches, and **`--edge <type>`**
  (repeatable) keeps only the named types; together they draw the supersession chains alone.
- **Composite action**: `uses: Kaikei-e/DocDag@v0.2.0` downloads the release binary for the runner,
  verifies it against `checksums.txt` and runs `validate --format github`. Linux and macOS, amd64
  and arm64; Windows fails with a diagnostic rather than passing silently. No Go toolchain, no
  third-party actions.
- **pre-commit**: `.pre-commit-hooks.yaml` ships `docdag-validate` (`pass_filenames: false`, since a
  graph is validated whole). Hooks are advisory; CI is the gate.
- `make build` works again, and `make check` builds the binary.

### Fixed

- Wikilink targets are taken as references only when the whole target is identifier-shaped. A
  footnote `[[3]](url)` or a note titled `[[3days-recap]]` no longer becomes a phantom edge to
  document 0003, and links inside fenced code blocks and inline code spans are ignored.
- A wikilinked identifier inside a frontmatter edge key (`supersedes: ["[[0042]]"]`) is unwrapped; a
  value that is not an identifier at all (`see 0042`) is `invalid_ref`.
- The two `dangling_ref` wordings are one wording; `invalid_frontmatter` reports the file line rather
  than the block-relative one.
- History findings are answered relative to where the caller stands even when the repository root
  sits behind a symlink or a Windows short name.

### Migrating from 0.1.0

- **Text report**: finding lines gained a `path:line:` prefix. Anything that grepped for `^ERROR ` or
  `^WARN ` must match on ` ERROR ` / ` WARN ` or read `--format json`.
- **JSON**: `validate` output gained `schema_version`, `location`, `related`, `fix`; `resolve` and
  `query` return `[{id,title,status,path,reference?}]` instead of `["0001", …]`. Detail wordings for
  `dangling_ref`, `missing_frontmatter`, `id_collision` and `invalid_frontmatter` changed.
- **Now fails where it used to pass**: an empty edge key (`empty_edge`); a frontmatter reference that
  is not an identifier (`invalid_ref`); with `references.dangling` set, a body link to a missing
  document; with `inverse:` set, an unmirrored edge; with a cardinality bound set, an edge that
  exceeds it.
- **`derived_edges: []`** now really clears the preset's derived edges. A repository that relied on
  it being ignored will lose the `status: superseded by NNNN` derivation — declare `supersedes:` in
  frontmatter instead.
- **`withdrawn`** is a valid status; a corpus that used the word as an unknown value stops raising
  `unknown_status`.
- **`new`** paths in output are relative to the working directory.
- **Configuration**: every new key is optional and off; an unchanged `docdag.yaml` behaves as before
  apart from the items above. Pin the action or
  `go install github.com/Kaikei-e/DocDag/cmd/docdag@v0.2.0`.

87 commits, 119 files, +10,394 / −503. Test suite 155 → 315 test functions. No new dependencies.

## [0.1.0] - 2026-07-31

First release. See the [GitHub release](https://github.com/Kaikei-e/DocDag/releases/tag/v0.1.0).

[0.3.0]: https://github.com/Kaikei-e/DocDag/releases/tag/v0.3.0
[0.2.0]: https://github.com/Kaikei-e/DocDag/releases/tag/v0.2.0
[0.1.0]: https://github.com/Kaikei-e/DocDag/releases/tag/v0.1.0
