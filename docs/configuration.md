# docdag.yaml

Configuration is optional. DocDag reads `docdag.yaml` from the repository root, or from
`--config <path>`, and merges it over the preset; flags win over both. See
[checks.md](checks.md) for what each check reports and [commands.md](commands.md) for the flags.

The page reads in the order a corpus grows: the default preset, the keys a single-kind corpus adds,
then the vocabulary a corpus with several sorts of document needs, and last the two presets written
out in full.

- [The preset](#the-preset) — `adr`, the default, printed whole.
- [The optional keys](#the-optional-keys) — every key a single-kind corpus may add.
- [List replacement](#list-replacement) — what writing a list or a map does to the preset's.
- [Kinds](#kinds) — several sorts of document in one corpus, each with an identity of its own.
- [Edge attributes](#edge-attributes) — facts an edge entry carries beside the reference.
- [The rule vocabulary](#the-rule-vocabulary) — the fixed `when` vocabulary, and what it excludes.
- [Projections and binding](#projections-and-binding) — derived boolean attributes, and which one
  defines what is in force.
- [Target conditions](#target-conditions) — what an edge requires of the document it points at.
- [Path constraints](#path-constraints) — an invariant over two composed edges.
- [preset_version and fields](#preset_version-and-fields) — the revision a corpus is written
  against, and the lifecycle of a frontmatter key.
- [Periods and as-of](#periods-and-as-of) — the days a document is in force between, and the day a
  command answers for.
- [The spec preset](#the-spec-preset) — the second preset, printed whole, with its kinds and its
  operating conventions.
- [filename and template](#filename-and-template) — what `docdag new` names and writes.
- [Structural escalation](#structural-escalation) — raising a built-in check.
- [Assembling a configuration in Go](#assembling-a-configuration-in-go) — building `Config` without
  writing YAML first.

## The preset

The file below is the `adr` preset in full — what DocDag applies with no configuration:

```yaml
preset: adr
preset_version: 1              # the revision this configuration is at
dir: docs/decisions            # default: discovered, see above
id_width: 4
status_field: status
status_values: [proposed, accepted, rejected, deprecated, superseded, withdrawn]
filename: "{id}-{slug}.md"     # what `docdag new` names a document

edges:
  - name: supersedes
    key: supersedes            # frontmatter key holding the references
    acyclic: true              # subject to the cycle check
    direction: forward         # the containing document is the edge source
  - name: depends-on
    key: depends-on
    acyclic: true
    direction: forward

derived_edges:
  - field: status
    pattern: '(?i)^superseded[\s-]+by[\s-]+(\S+)'   # capture group 1 is the reference
    edge: supersedes
    direction: reverse         # the referenced document is the edge source

projections:                   # derived boolean attributes, see below
  - name: accepted_unsuperseded
    when:
      not_inbound: supersedes
      attr:
        status: { eq: accepted }

binding: accepted_unsuperseded  # the projection `query --binding` answers with

rules:
  - name: status_drift
    severity: error
    when:
      inbound: supersedes
      attr:
        status: { not: superseded }
    message: "has inbound supersedes but status is not superseded"
  - name: superseded_orphan
    severity: warn
    when:
      not_inbound: supersedes
      attr:
        status: { eq: superseded }
    message: "status is superseded but no document supersedes it"
```

`dir` defaults to whichever of `docs/adr`, `doc/adr`, `docs/decisions`, `docs/ADR`, `adr` exists
first and holds a document. Status comparison is case-insensitive, and a value outside
`status_values` is an `unknown_status` finding.

## The optional keys

Every other key is optional, and off unless the file says otherwise:

```yaml
acyclic_union: true            # also report a cycle that only the union of the acyclic types closes

references:                    # reference-layer validation; without it the layer is unvalidated
  dangling: error              # off (default) | warn | error
  pattern: '^(?i)(?:adr-?)?(\d{3,6})$'   # the default: what an identifier-shaped target looks like
  scan: [body, frontmatter]    # default [body]; frontmatter reads wikilinks in scalars and lists

structural:                    # raise a built-in check; lowering one is a configuration error
  missing_frontmatter: error

edges:
  - name: amends
    key: amends
    acyclic: true
    direction: forward
    inverse: amended_by        # the target must list the source here; the key declares no edges
    max_inbound: 1             # 0, the default, is unbounded
    max_outbound: 0
    min_outbound: 1            # read over from: where the edge names kinds; see checks.md

rules:
  - name: unexplained_retirement
    severity: warn
    when:
      not_inbound: supersedes
      any_of:                                    # holds when any alternative holds
        - attr: { status: { eq: deprecated } }
        - attr: { status: { eq: rejected } }
      not:                                       # holds when this condition does not
        attr: { tags: { contains: explained } }
    message: "is retired, nothing supersedes it, and it carries no explained tag"
```

## List replacement

The `edges:` and `rules:` lists above show where the new keys go; writing one of them — or
`derived_edges:`, `projections:`, or the `kinds:` and `fields:` maps the sections below introduce —
replaces the preset's list rather than adding to it, and writing it as an explicit empty list
(`derived_edges: []`, `kinds: {}`) clears the preset's without putting anything in its place.
Writing `edges:` also drops the inherited projections that read an edge type the new list does not
declare, and the `binding:` that named one of them: a projection over a vocabulary the corpus
replaced away cannot be evaluated. The two scalars behave as scalars do — writing `preset_version:`
or `binding:` replaces the preset's value, leaving either out keeps it — which is why every section
below that adds a key to a list prints the whole list it belongs to.

## Kinds

A corpus that declares no `kinds:` is single-kind: `dir`, `id_width` and `status_values` describe
that one kind, and identity is the digit run in a file name — which is every corpus DocDag managed
before kinds existed, the `adr` preset included. `kinds:` says instead that several sorts of
document share one corpus, each in a directory of its own and each with its own idea of an
identifier:

```yaml
kinds:
  clause:
    dir: spec/clauses                 # relative to this file
    id: '^UZ-[A-Z]-\d{3}$'            # what an identifier of this kind looks like
    status_values: [proposed, trial, accepted, superseded, withdrawn]
    closed: true                      # frontmatter keys nobody declared are findings
  conform:
    dir: spec/conform
    id: '^conform/[a-z0-9-]+$'        # carries a slash, so no file name can hold it
  deviation:
    dir: spec/deviations
    id: '^dev-\d{4}$'
    closed: true
```

Every kind needs a `dir`, no two kinds may name the same one, and the directories are read relative
to the configuration file that declares them rather than to the process's working directory: a
corpus is described from where it lives. Writing `dir:` or `id_width:` beside `kinds:` — or passing
`--dir` — is a configuration error (exit 3), because the kinds carry both. Discovery does not run:
the kinds say where the documents are.

A declared directory that is not there yet is a kind holding no documents rather than an error: the
kinds are the vocabulary a corpus may grow into, and `preset: spec` names eight directories that a
vault adopting it in one line has not all written into yet. Every other failure to read one still
exits 3.

### Identity

`id:` is a Go regular expression matched against the whole identifier, whether or not it is written
anchored, so `see UZ-V-001` never names a document. A document's identifier comes from:

1. the frontmatter `id:` key, when it writes one, and
2. otherwise the file name without its `.md`.

Either way the token has to satisfy the kind's pattern, and the token *is* the identifier: a
declared pattern is the canonical spelling, so nothing is padded, truncated or lowercased. A pattern
carrying a slash — `^conform/[a-z0-9-]+$` — is one no file name can hold, so documents of that kind
have to write `id:` in their frontmatter. A file that yields no identifier at all is the
`id_mismatch` finding of [checks.md](checks.md): unlike the single-kind reader, which skips a file
whose name does not match the pattern, a kind's directory is what declares a file one of its
documents, so a stray `README.md` in there is reported rather than passed over.

A kind that declares no `id:` keeps the digit-run rules, padded to `id_width`, so `339`, `ADR-339`
and `000339` still name one document there. Inside a multi-kind corpus those rules are held to the
same whole-reference shape the single-kind corpus holds them to — otherwise a kind declaring no
pattern would quietly claim every reference the other kinds rejected.

Wikilinks are unwrapped before any of this, so `id: "[[UZ-V-001]]"` and `enforces: ["[[UZ-V-001]]"]`
name UZ-V-001 whatever the kind.

### Which kind a document is

The directory decides: a document in `spec/clauses` is a `clause`. A document may also write
`kind: clause` in its frontmatter, and one that writes a kind its directory disagrees with is the
`kind_mismatch` finding. The directory's answer is the one that stands — it chose the identity rules
the document was read under — and it is readable from rules and projections as the attribute `kind`:

```yaml
rules:
  - name: orphan_test
    severity: error
    when:
      attr: { kind: { eq: conform } }
      not_outbound: enforces
    message: "enforces no clause"
```

### Closed kinds

`closed: true` makes the kind's frontmatter a closed set: every key the configuration does not know
is an `unknown_field` finding, one per key, on the key's own line. The known keys are `title`,
`date`, `id`, `kind`, the status field, every `key:` and `inverse:` an edge declares, every `field:`
a derived edge reads, and every name declared under `fields:` — so renaming a key in the
configuration widens the set with it, and declaring a field is how a closed kind admits a key
nothing else in the configuration mentions.
Kinds are open by default, which is what every corpus had before: unrecognized keys are ignored, and
another tool's fields raise nothing.

### Status vocabularies

A kind may carry a `status_values` of its own; one that carries none answers to the top-level
vocabulary. `unknown_status` is reported against the vocabulary of the document's own kind, so
clauses can be `trial` while deviations keep the preset's words.

### Constraining an edge by kind

`from:` and `to:` hold an edge's endpoints to a set of kinds, as the graph holds the edge — so a
`direction: reverse` edge's `from:` is the kind of the document its key names, not of the one that
wrote the key down:

```yaml
edges:
  - name: enforces
    key: enforces
    direction: forward
    from: [conform]
    to: [clause]
```

An endpoint of another kind is the `edge_kind_mismatch` finding, filed against the document that
declared the edge. Only endpoints the corpus holds are checked: a reference naming no document has
no kind to be wrong about, and is a `dangling_ref` of its own. Naming a kind nobody declares, or
writing `from:`/`to:` on a corpus without kinds, is a configuration error.

The kinds an edge points at also resolve its references first, so where two kinds' patterns accept
the same string the edge's own `to:` decides which document it means. The other kinds still follow:
a reference to a document of the wrong kind resolves and is reported as the mismatch it is, rather
than as a reference to nothing.

### Creating a document of a kind

`docdag new` needs `--kind` on a multi-kind corpus: which kind to create, under which identity rules
and from which template, has no default answer. See [commands.md](commands.md) for what it writes.

### Append-only kinds

`validate --immutable-since` reads a multi-kind corpus only under the kinds that declare
`append_only: true`. Those are the machine-generated records — the `spec` preset marks
`conform` and `measure` — so a closed document there is compared the way a single-kind ADR
corpus is. A multi-kind configuration that declares no such kind is refused:

`--immutable-since reads only kinds with append_only: true`, exit 3.

[ci.md](ci.md#append-only-history) covers what the check allows on the documents it does read.

## Edge attributes

An edge may declare attributes its entries carry. An edge that declares none — every edge of the
`adr` preset — takes plain references and nothing else:

```yaml
edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    attrs:
      reason: {required: true, one_of: [recurrence, premise-collapse, conflict, vocabulary]}
  - name: measures
    key: measures
    direction: forward
    attrs:
      agreement: {required: true, type: number}
      model:     {required: true, type: string}
      taken_on:  {type: date}                    # optional, and a date when written
```

An entry under such a key is then either a plain reference or a mapping naming one:

```yaml
supersedes:
  - ref: "0001"          # the reference; every other key of the mapping is an attribute
    reason: conflict
measures:
  - ref: "0002"
    agreement: 0.92
    model: sonnet
    taken_on: 2026-01-01
```

An attribute is a fact about the relation — the reason a clause replaced another, the model a
measurement was taken with. A *lifetime* is not one: how long a document is in force belongs to the
document, under [`period:`](#periods-and-as-of), because a record that departs from two clauses has
one lifetime and not two. That is why the `spec` preset's `deviates-from` carries no attributes and
its `deviation` kind carries `expires:`.

`type:` is `string` (the default), `number` or `date`, a date being `YYYY-MM-DD`; `one_of:` is a
closed vocabulary of strings, compared exactly rather than case-insensitively, and it implies
`string`. Declaring `one_of` on a `number` or a `date`, naming a type nothing knows, and declaring
an attribute called `ref` — the key the reference itself is written under — are configuration errors
(exit 3). An unknown attribute, a missing required one and a value the declaration rejects are the
`edge_attr_unknown`, `edge_attr_missing` and `edge_attr_invalid` findings of
[checks.md](checks.md); `required: true` reaches plain references too, since an entry that carries
no attributes carries no required one either. An `inverse:` key mirrors edges rather than declaring
them, so it takes plain references whatever its edge declares, and a derived edge comes from a field
value and carries no attributes at all.

The attributes an edge carries are part of `export --format json`, as an `attrs` object on the link,
and a link whose edge carries none is exported exactly as it was before.

The `adr` preset declares no attributes, so its edges behave as they always have. A corpus that
wants to record *why* one decision replaced another re-declares the edges itself — writing `edges:`
replaces the preset's list, so name every edge the corpus keeps:

```yaml
edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    attrs:
      reason: {one_of: [recurrence, premise-collapse, conflict, vocabulary]}
  - name: depends-on
    key: depends-on
    acyclic: true
    direction: forward
```

Leaving `required` off, as above, keeps every `supersedes: ["0001"]` already in the corpus valid and
lets a document state a reason where it has one. Adding `required: true` makes each of those entries
an `edge_attr_missing` error, which is the migration: rewrite the entries, then require the
attribute.

## The rule vocabulary

A rule's `when` block ANDs its top-level clauses. The vocabulary is fixed and complete: `inbound`,
`not_inbound`, `outbound`, `not_outbound` — each naming a declared edge type — `attr: {<key>:
{eq|not: <value>}}` on a scalar, `attr: {<key>: {contains|not_contains: <value>}}` and
`attr: {<key>: {subset_of: [<value>, …]}}` on a list, `via` and `via_inbound` on a neighbour, and
the two combinators `any_of: [<condition>, …]` and `not: <condition>`, which nest. A scalar read as
a list is a one-element list; comparison is case-insensitive; a positive clause needs the attribute
to be there and a negative one is satisfied by its absence. There is no expression language: no
arithmetic, no string operations, no variables.

An `attr:` key reads a frontmatter key, a declared projection, or `in_force` — the one attribute the
engine computes rather than reads, from the [`period:`](#periods-and-as-of) a kind declares. The
date comparison behind it stays in the engine, which is what keeps the vocabulary free of dates.

`inbound` and `outbound` read either as an edge name or as a degree threshold, and the name alone is
sugar for one edge or more. `via` and `via_inbound` reach exactly one hop: they hold when at least
one neighbour across the named edge type satisfies every attribute clause they carry. A neighbour
condition is attributes only — no edge clause, and no `via` inside a `via` — so a condition stays a
question about a document and its immediate neighbourhood. Transitive reach is what `resolve` is
for.

The same vocabulary is what an edge spec's [`target:`](#target-conditions) writes, and there it sees
only the target's own local condition: nesting `via` or another `target` inside it is not allowed,
so the modal depth stays fixed at two — one edge, then a condition. That is the bar any future
addition to the vocabulary is reviewed against: a word stays only if it keeps conditions inside the
bisimulation-invariant fragment, which is what makes a rule a question about a document's
neighbourhood rather than a query language.

```yaml
rules:
  - name: deviation_pressure
    severity: warn
    when:
      inbound: { edge: deviates-from, min: 5 }   # at least five inbound edges
    message: "has five or more deviations; reconsider the decision"
  - name: contested
    severity: warn
    when:
      outbound: { edge: depends-on, min: 1, max: 3 }   # max is unbounded unless written
    message: "depends on between one and three decisions"
  - name: stale_premise
    severity: error
    when:
      attr: { status: { eq: accepted } }
      via:                                       # via_inbound reads the other direction
        edge: premise
        attr: { status: { eq: retired } }
    message: "is accepted but one of its premises is retired"
```

A degree threshold counts the edges of that type at the document, including the ones whose other end
is a reference no document answers — that is a `dangling_ref` finding of its own. A `min` below 1
and a `max` of 0 are configuration errors: absence is `not_inbound` and `not_outbound`, which is
where the vocabulary keeps it. A neighbour the corpus does not hold carries no attributes and
satisfies no `via` clause.

## Projections and binding

A projection is a derived boolean attribute. It is named, it holds where its condition holds, and it
is readable as an attribute of that name from rules and from other projections, and as a column of
`query --fields` and `resolve --fields`:

```yaml
projections:
  - name: enforced
    when: { inbound: enforces }
  - name: effective_should
    any_of:                                      # holds when any alternative's when holds
      - when: { attr: { modality: { eq: SHOULD } } }
      - when: { attr: { modality: { eq: MUST } }, not_inbound: enforces }

binding: accepted_unsuperseded
```

A projection writes `when` or `any_of`, one of the two. Its value reads as the string `true` where
it holds and `false` where it does not, so `attr: {enforced: {eq: "true"}}` and
`attr: {enforced: {not: "true"}}` both say what they look like. A projection name shadows a
frontmatter key spelled the same way: the derived value is the configured one, and a document cannot
take it back by writing the key down.

Projections may read each other, and they are evaluated in dependency order, so the list may be
written in any order. A reference cycle among them is a configuration error (exit 3), as is a
duplicate name, a nameless projection and a `binding:` naming a projection nobody declared.

`binding:` names the projection that defines the documents in force — what `query --binding` lists,
what `stats` counts and what `context` keeps. A configuration that declares no projections at all
falls back on the built-in definition, accepted and superseded by nothing, which is what the `adr`
preset's `accepted_unsuperseded` projection writes down.

## Target conditions

Beside the attributes its entries carry, an edge may declare what it requires of the document it
points at. `target:` is the rule vocabulary above, evaluated against that document:

```yaml
edges:
  - name: enforces
    key: enforces
    direction: forward
    from: [conform]
    to: [clause]
    target:
      leaf_of: supersedes        # sugar for not_inbound: supersedes
  - name: deviates-from
    key: deviates-from
    direction: forward
    from: [deviation]
    to: [clause]
    target:
      attr: {status: {eq: accepted}}
      leaf_of: supersedes        # the two together are what "binding" means
```

`leaf_of: <edge>` says the target has to be the current leaf of that lineage rather than a document
something replaced. On a corpus without periods that is exactly `not_inbound: <edge>`; where the
target's kind declares a `period:`, the two part company, and `leaf_of` is the one to write.
"Current leaf" is then read at the day the run is about: a successor nobody has accepted, or one
whose period has not begun, leaves its predecessor the leaf, because that is the document a reader
is still bound by — which is the same fact
[`pending_successor`](checks.md#the-spec-presets-rules) reports. `not_inbound` counts a
declared successor whatever its status or its dates, so writing it where `leaf_of` is meant makes
one run say a target is stale and still binding at once. The sugar is kept rather than desugared
away for that reading, and because only that spelling earns a fix suggestion — see
[`stale_target`](checks.md#stale_target--error-structural).

The target of an edge is its head, whichever end of it the frontmatter named: on a
`direction: reverse` edge the key names the *source*, so the document the relation points at is the
one that wrote the key down. That is the same endpoint `to:` constrains, so `to:` and `target:`
speak about one document rather than two. Derived edges carry the condition too — a MADR
`status: superseded by 0003` is checked exactly as a written `supersedes:` is — and an entry naming
no document raises `dangling_ref` alone: there is no target to hold a condition against.

Projections are readable inside a target condition, like anywhere else attributes are read:
`attr: {effective_must: {eq: "true"}}` asks that the target be binding under whatever the corpus
means by binding.

What a target condition may **not** carry is `via` or `via_inbound`. The condition is already one
hop from the document that declared the edge, so a clause about the *target's* neighbours would be
two, and the depth the vocabulary stays inside is fixed at two: an edge, then a local condition.
Writing one, naming an undeclared edge under `leaf_of:`, or writing a `target:` that constrains
nothing is a configuration error (exit 3).

### Recommending it for the `adr` preset

The `adr` preset declares no target condition, deliberately: existing vaults carry `depends-on`
edges to superseded decisions, and a default error would stop their CI on documents nobody touched.
A corpus that wants the invariant re-declares the edges itself — writing `edges:` replaces the
preset's list, so name every edge the corpus keeps:

```yaml
edges:
  - {name: supersedes, key: supersedes, acyclic: true, direction: forward}
  - {name: depends-on, key: depends-on, acyclic: true, direction: forward, target: {leaf_of: supersedes}}
```

Adopt it in stages: run `docdag validate --format json` first and watch the `stale_target` count —
`jq '[.findings[] | select(.rule == "stale_target")] | length'` — while the configuration still
declares no target, clear the violations the count names, and only then write the `target:` in. The
finding is a fixed error, so the last step is the one that can fail a build.

## Path constraints

`path_constraints:` states what an edge's own `target:` cannot: an invariant over two edges
composed. Each constraint walks a path from every document and compares what it reaches against
either nothing or a second path:

```yaml
path_constraints:
  - name: amend_targets_current
    path: [amends, ^supersedes]    # d --amends--> x, and y --supersedes--> x
    equals: none                   # no such y may exist: what d amends is the current leaf
  - name: deviation_scope_consistent
    path: [deviates-from, premise]
    subset_of: [premise]           # the premises of the clause departed from are the record's own
```

For each document *d*, the `path` reaches a set P(d) — every step composed over every document the
step before it reached — and the right-hand side names Q(d): `equals: none` is the empty set, and
`subset_of:` is a second path walked the same way. P(d) ⊆ Q(d) has to hold; a document it does not
hold for is the [`path_mismatch`](checks.md#path_mismatch--error-structural) finding, listing
P(d) − Q(d).

A path element is a declared edge name, optionally prefixed with `^` to walk that edge backwards:
`supersedes` steps from a document to the ones it supersedes, and `^supersedes` to the ones that
supersede it. Both paths are **one or two steps long** — zero or three is a configuration error
(exit 3), and so is a wildcard, a regular expression or a repetition, since none of them is an edge
name. Two is the decision rather than a limit of the implementation: a longer path is a regular path
expression, whose implication problem is undecidable, and transitive reach is what `resolve` already
answers. Exactly one of `equals:` and `subset_of:` is written, `none` is the only set `equals:`
accepts, and names must be present and distinct.

Only the typed layer is walked — structured and derived edges, never the reference layer — and only
documents the corpus holds are reached, so a reference naming none is a `dangling_ref` alone rather
than a second finding here.

An invariant that an edge's `target:` can state should be written there instead: it is declared in
one place, reads locally, and carries a fix suggestion. `path_constraints:` is for what `target:`
cannot reach.

## preset_version and fields

`preset_version:` is the revision of the configuration a corpus is written against. It is a plain
integer the configuration carries and DocDag never interprets: it is written into the JSON output
headers — `validate --format json` and `context --format json` — so a repository can pin the
revision its documents were checked under. The `adr` preset is revision 1, and a corpus that
overrides it says so by writing its own number.

`fields:` declares the frontmatter keys the documents write: the vocabulary a key's value comes
from, whether a document has to write it, and the lifecycle of a key being retired. A key nobody
declares is not unknown, only undeclared, so a corpus pays for this only where it is saying
something:

```yaml
preset_version: 3
fields:
  modality:                    # a key the corpus keeps
    one_of: [MUST, SHOULD]     # a value outside it is the unknown_field_value finding
    required: true             # a document that writes none is the missing_field finding
  owner:                       # the key being retired
    deprecated: true           # writing it is the deprecated_field finding
    since: 2                   # the preset revision that retired it
    migrate_to: owned-by       # where the value goes; becomes the fix line
    sunset: 2027-01-01         # the last day it is tolerated; after it, an error
  team: {}                     # declared, not retired: a known key and nothing more
```

The two halves are alternatives rather than a set: `one_of` and `required` describe a field the
corpus keeps, `deprecated` and its three companions a field the corpus is retiring, and writing one
of each about one key is a configuration error (exit 3) — a required key that is also deprecated
would say a document must write what it is reported for writing.

`one_of` is compared exactly, case included, for the reason an edge attribute's is: a declared
vocabulary is a closed machine vocabulary a preset revision renames wholesale. An empty or repeated
value in it is a configuration error. `required` reads a **scalar**: a key written as a list or a
mapping holds no value under that key and is reported as missing. Both are read per **kind**, over
the declarations a document of that kind sees, and both answer for open kinds as well as closed
ones. See [checks.md](checks.md).

`deprecated: true` is what makes the other half of a declaration do anything. `since`, `migrate_to`
and `sunset` describe a retirement, so writing one of them without `deprecated: true` is a
configuration error, as is a `sunset` that is not a `YYYY-MM-DD` day, a negative `since`, a nameless
field, and a field named after an edge's `key:` or `inverse:` — that key's lifecycle belongs to the
edge, and retiring it would say the relation is over without retiring the edge.

A declared field is a **known** key, so a `closed: true` kind accepts it rather than reporting
`unknown_field`: a migration in progress is not a mistake. Kinds may declare `fields:` of their own,
which win over the top-level ones by name, so a key one kind still uses is not retired for it:

```yaml
kinds:
  clause:
    dir: spec/clauses
    closed: true
    fields:
      owner: {}                # clauses still write it, whatever the corpus says
```

`docdag stats --fields` counts the corpus by field — how many documents write each one, when a
document that writes it last changed, and which are retired — so a removal is decided on numbers.
A declared field nobody writes is still a row, at zero: that is what a finished migration looks
like. See [commands.md](commands.md).

### Revising a preset safely

Whether a revision is compatible is answered by running the checks twice rather than by a feature:

```console
$ docdag validate --config old.yaml --format json > old.json
$ docdag validate --config new.yaml --format json > new.json
$ diff <(jq -S '.findings' old.json) <(jq -S '.findings' new.json)
```

An unchanged findings set means the revision is compatible — a minor bump. A changed one means the
revision decides something differently about documents nobody edited, which is a major bump, and the
diff names exactly which documents moved. Bump `preset_version:` accordingly, so a repository
pinning a revision knows what it pinned.

## Periods and as-of

A `period:` declaration says which frontmatter keys a document writes the days it is in force
between. It is written per kind, or once at the top level for a corpus that declares no kinds:

```yaml
kinds:
  clause:
    period: {from: in_force_from, until: in_force_until}   # from defaults to date
  deviation:
    period: {from: date, until: expires}
  premise:
    period: {from: date, until: retired_on}
```

The two values are **key names, not days**: the days are facts about each document, so they live in
the documents. Both keys are ISO 8601 calendar dates — `YYYY-MM-DD`, no time and no timezone — and
the interval they describe is closed-open, `[from, until)`: a document is in force on the day it
begins and not on the day it ends. A kind that names no `from` reads the `date` field; a document
that does not write the named key has no beginning to compare against, so it has always begun.
Declaring the period is what makes its keys known to a `closed: true` kind, and declaring them under
`fields:` as well is what makes `stats --fields` count them.

A period whose two ends are one key, or which names no key at all, is a configuration error
(exit 3), as is one naming a key an edge already declares.

**The end can be derived.** For a document that writes none, `until` is the earliest day an
**accepted** successor begins on — over the `supersedes` lineage — and the derived day is never
written back into the frontmatter, for the reason no derived value is. A successor nobody has
accepted derives nothing, and one that is withdrawn stops deriving: the predecessor's period opens
up again with nothing rewritten but the successor's own status. A written end that disagrees with a
derived one is `period_conflict`; the written day is the one that decides.

**`in_force` is what rules read.** From the period and the day the run is about, the engine computes
`in_force(d) := from(d) ≤ as-of < until(d)` and exposes it as an attribute that reads exactly like a
projection — `attr: {in_force: {eq: "true"}}`. It is the one attribute the engine computes rather
than reads, and the date comparison stays inside it: the vocabulary gains no date literal and no
arithmetic. A projection named `in_force` is a configuration error, because it would shadow the
attribute. A kind that declares no period is always in force, which is what keeps a configuration
without periods answering as it always did. [checks.md](checks.md#periods-and-the-day-a-run-is-about)
has the three findings a period turns on, and the rule that an out-of-force document's edges stop
counting.

### The day a command answers for

| Command | Default as-of | Why |
| --- | --- | --- |
| `validate`, `lint --corpus` | the day HEAD was committed on | a gate has to answer the same way for one commit however long afterwards it runs |
| `query`, `resolve`, `context`, `stats` | the day the command runs | a listing is a question about now |

`--as-of YYYY-MM-DD` names the day explicitly on any of them, and `$DOCDAG_AS_OF` names it for a
whole pipeline; the flag wins where both are written, and a day that is not a date is a usage error
(exit 2). Outside a git repository, or where git cannot answer, `validate` falls back on the day it
runs. Detecting an expiry as it happens is a scheduled run that says so — `--as-of $(date -I)`,
which [ci.md](ci.md#periodic-runs) writes out.

`--at <rev>` is the other axis: it reads every managed document from a revision instead of from the
working tree. The two are independent — the valid time of a temporal database and its transaction
time — so combining them asks what the vault at that revision said was in force on that day:

```console
$ docdag query --binding --at v1.2.0 --as-of 2026-06-01
```

`--at` is read-only, so `new` does not take it, and a corpus outside a repository cannot answer it
(exit 3). Every JSON report carries `as_of`, and `at` where a revision was named; the text reports
carry the day only where some kind declares a period, because a corpus that answers the same on
every day has no day worth printing.

### Time-dependent status checks

The `adr` preset's `status_drift` is time-independent: the moment a successor exists, the
predecessor has to say `superseded`. A corpus that declares a period can replace it with the three
rules the `spec` preset carries, which read the day instead:

```yaml
period: {from: date}
projections:
  - name: has_inforce_successor
    when: {via_inbound: {edge: supersedes, attr: {in_force: {eq: "true"}, status: {eq: accepted}}}}
rules:
  - name: status_drift
    severity: error
    when:
      attr: {status: {not: superseded}}
      via_inbound: {edge: supersedes, attr: {status: {eq: accepted}, in_force: {eq: "true"}}}
    message: "an in-force successor supersedes it but status is not superseded"
  - name: pending_successor
    severity: warn
    when:
      attr: {status: {eq: accepted}}
      inbound: supersedes
      not: {attr: {has_inforce_successor: {eq: "true"}}}
    message: "a successor is declared but not yet in force; this clause remains binding until then"
  - name: premature_superseded
    severity: error
    when:
      attr: {status: {eq: superseded}}
      not: {attr: {has_inforce_successor: {eq: "true"}}}
    message: "status is superseded but no successor is in force yet"
```

Writing `rules:` replaces the preset's list, so the rules a corpus keeps are written out with them.
A binding projection that should follow the same reading adds `in_force: {eq: "true"}` and the
absence of `has_inforce_successor` to its own condition, which is what the `spec` preset's
`effective` does.

## The spec preset

`preset: spec` is the second built-in configuration: a normative standard as a graph of clauses, the
conformance tests that enforce them, the deviations recorded against them and the measurements taken
of them, over the subjects they speak to. It uses every key above — eight kinds, edges constrained
by kind and carrying attributes, a closed field vocabulary, projections and a binding of its own —
and a corpus adopts the whole of it with one line:

```yaml
preset: spec
```

One line is the whole adoption: the eight directories need not exist yet, and each one appears when
the corpus has something to put in it.

The file below is that preset in full:

```yaml
preset: spec
preset_version: 2
status_field: status

kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    status_values: [proposed, trial, accepted, superseded, withdrawn]
    closed: true
    fields:
      modality:                 # the strength the clause claims, and it has to claim one
        one_of: [MUST, MUST_NOT, SHOULD, SHOULD_NOT, MAY]
        required: true
      in_force_from: {}         # declared so stats --fields counts them; the period reads them
      in_force_until: {}
    period: {from: in_force_from, until: in_force_until}
  conform:
    dir: spec/conform
    id: '^conform/[a-z0-9-]+$'
    append_only: true           # harness-written; validate --immutable-since reads it
    fields:
      test: {}                  # path to the executable test body
  deviation:
    dir: spec/deviations
    id: '^dev-\d{4}$'
    status_values: [proposed, accepted, resolved, withdrawn]
    closed: true
    fields:
      expires: {}               # the day the departure stops being recorded
    period: {from: date, until: expires}
  measure:
    dir: spec/measures
    id: '^interp/UZ-[A-Z]-\d{3}@\d{4}-\d{2}-\d{2}$'
    append_only: true           # harness-written; validate --immutable-since reads it
  premise:
    dir: spec/premises
    id: '^premise/[a-z0-9/-]+$'
    status_values: [proposed, accepted, retired, superseded]
    fields:
      retired_on: {}            # the day the world stopped making it true
    period: {from: date, until: retired_on}
  principle:
    dir: spec/principles
    id: '^principle/[a-z0-9/-]+$'
    status_values: [proposed, accepted, superseded, withdrawn]
  pm:
    dir: spec/pm
    id: '^pm-\d{4}$'
    status_values: [draft, published]
  topic:
    dir: spec/topics
    id: '^topic/[a-z0-9/-]+$'

edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    from: [clause, premise]
    to: [clause, premise]
    attrs:
      reason: {required: true, one_of: [recurrence, premise-collapse, conflict, vocabulary]}
  - name: enforces
    key: enforces
    direction: forward
    from: [conform]
    to: [clause]
    target: {leaf_of: supersedes}     # the current leaf: no in-force accepted successor
  - name: deviates-from
    key: deviates-from
    direction: forward
    from: [deviation]
    to: [clause]
    target:
      attr: {status: {eq: accepted}}
      leaf_of: supersedes             # = binding: a departure departs from something in force
  - name: premise
    key: premise
    direction: forward
    from: [clause]
    to: [premise]
  - name: rationale
    key: rationale
    direction: forward
    from: [clause]
    to: [principle]
  - name: counterexample
    key: counterexample
    direction: forward
    from: [clause, principle]
    to: [pm]
  - name: measures
    key: measures
    direction: forward
    from: [measure]
    to: [clause]
    attrs:
      agreement: {required: true, type: number}
      model: {required: true, type: string}
    target: {leaf_of: supersedes}
  - name: about
    key: about
    direction: forward
    from: [clause]
    to: [topic]
    min_outbound: 1                   # a clause with no subject is invisible to modality_conflict
  - name: excepts
    key: excepts
    acyclic: true
    direction: forward
    from: [clause]
    to: [clause]
    attrs:
      scope: {required: true, type: string}
  - name: interop
    key: interop
    direction: forward
    from: [clause]
    to: [clause]

projections:
  - name: enforced
    when: {inbound: enforces}
  - name: has_inforce_successor       # in_force is the engine's, computed from period:
    when: {via_inbound: {edge: supersedes, attr: {in_force: {eq: "true"}, status: {eq: accepted}}}}
  - name: effective_must
    any_of:
      - when:
          attr: {modality: {eq: MUST}, status: {eq: accepted}, in_force: {eq: "true"}}
          inbound: enforces
          not: {attr: {has_inforce_successor: {eq: "true"}}}
      - when:
          attr: {modality: {eq: MUST_NOT}, status: {eq: accepted}, in_force: {eq: "true"}}
          inbound: enforces
          not: {attr: {has_inforce_successor: {eq: "true"}}}
  - name: effective_should
    any_of:
      - when:
          attr: {modality: {eq: SHOULD}, status: {eq: accepted}, in_force: {eq: "true"}}
          not: {attr: {has_inforce_successor: {eq: "true"}}}
      - when:
          attr: {modality: {eq: SHOULD_NOT}, status: {eq: accepted}, in_force: {eq: "true"}}
          not: {attr: {has_inforce_successor: {eq: "true"}}}
      - when:
          attr: {modality: {eq: MUST}, status: {eq: accepted}, in_force: {eq: "true"}}
          not_inbound: enforces           # a condition holds one not: block; this is the other absence
          not: {attr: {has_inforce_successor: {eq: "true"}}}
      - when:
          attr: {modality: {eq: MUST_NOT}, status: {eq: accepted}, in_force: {eq: "true"}}
          not_inbound: enforces
          not: {attr: {has_inforce_successor: {eq: "true"}}}
  - name: effective                     # the first two already read the day through the projections
    any_of:
      - when: {attr: {effective_must: {eq: "true"}}}
      - when: {attr: {effective_should: {eq: "true"}}}
      - when:
          attr: {modality: {eq: MAY}, status: {eq: accepted}, in_force: {eq: "true"}}
          not: {attr: {has_inforce_successor: {eq: "true"}}}

binding: effective

rules:
  - name: orphan_must
    severity: error
    when:
      any_of:
        - attr: {modality: {eq: MUST}, status: {eq: accepted}}
        - attr: {modality: {eq: MUST_NOT}, status: {eq: accepted}}
      not_inbound: enforces
    message: "is MUST or MUST_NOT and accepted but nothing enforces it"
  - name: orphan_test
    severity: error
    when:
      attr: {kind: {eq: conform}}
      not_outbound: enforces
    message: "enforces no clause"
  - name: stale_premise
    severity: error
    when:
      attr: {status: {eq: accepted}}
      via: {edge: premise, attr: {in_force: {eq: "false"}}}
    message: "is accepted but a premise is no longer in force"
  - name: deviation_pressure
    severity: warn
    when:
      attr: {status: {eq: accepted}}
      inbound: {edge: deviates-from, min: 5}
    message: "has 5+ deviations; reconsider the clause"
  - name: no_counterexample
    severity: warn
    when:
      attr: {kind: {eq: clause}, status: {eq: accepted}}
      not_outbound: counterexample
    message: "is accepted without a counterexample"
  - name: may_without_interop
    severity: warn
    when:
      attr: {modality: {eq: MAY}, status: {eq: accepted}}
      not_outbound: interop
    message: "is MAY but names no MUST clause that guarantees interoperation without it"
  - name: interop_not_must
    severity: error
    when:
      outbound: interop
      via: {edge: interop, attr: {modality: {not: MUST}}}
    message: "interop must point at a MUST clause"
  - name: status_drift                  # time-dependent, unlike the adr preset's rule of that name
    severity: error
    when:
      attr: {status: {not: superseded}}
      via_inbound: {edge: supersedes, attr: {status: {eq: accepted}, in_force: {eq: "true"}}}
    message: "an in-force successor supersedes it but status is not superseded"
  - name: pending_successor
    severity: warn
    when:
      attr: {status: {eq: accepted}}
      inbound: supersedes
      not: {attr: {has_inforce_successor: {eq: "true"}}}
    message: "a successor is declared but not yet in force; this clause remains binding until then"
  - name: premature_superseded
    severity: error
    when:
      attr: {status: {eq: superseded}}
      not: {attr: {has_inforce_successor: {eq: "true"}}}
    message: "status is superseded but no successor is in force yet"
```

There is no top-level `status_values`, and each kind that answers to a vocabulary carries its own.
That is deliberate: a kind inherits the top-level vocabulary wherever it declares none, so writing
one would hand `conform` and `measure` documents — which a machine writes, and which say nothing
about their own standing — a vocabulary to fall outside of. With none declared anywhere they reach,
their `status` is unchecked, which is what "this kind has no status" has to be written as.

There is no `filename:` either. Every kind of the preset declares an identifier pattern, and a kind
that does names its files after the identifier rather than from the template — see
[filename and template](#filename-and-template).

`testdata/fixtures/spec-vault/` in the repository is a small corpus under this preset.

### The kinds

**clause** — the normative statement itself, `UZ-V-001`, the one document a person writes by hand
and the one every other kind points at. It states a strength in `modality:` — one of BCP 14's five
keywords, `MUST`, `MUST_NOT`, `SHOULD`, `SHOULD_NOT` and `MAY` — and it carries only the relations
that are about its own content: the subject it speaks to under `about:`, the clause it makes an
exception of under `excepts:`, the requirement its option leans on under `interop:`, and
`premise:`, `rationale:` and `counterexample:`. Its frontmatter is `closed: true`, so a key nobody
declared is a finding rather than another tool's field; `modality` is declared under the kind's
`fields:`, which is what admits it — with the vocabulary it comes from and `required: true`, because
a clause that states no strength states nothing. Its `period:` reads `in_force_from:` and
`in_force_until:`, so a clause released next quarter can be written today and binds nothing until
then; a clause that writes neither day has always been in force and stops when its successor takes
over.

The vocabulary is five values rather than a strength and a polarity because BCP 14 has no `MAY NOT`:
a pair of fields would have an invalid combination to check for, where a closed set of five has
nothing to get wrong. `MAY` is a node like any other — a permission the standard states out loud,
which is what lets a later prohibition be seen to collide with it rather than silently overwrite it.

**topic** — the subject a clause speaks to, `topic/seed-recording`, whose body is one paragraph
defining it. A subject is a document rather than a string attribute so that a misspelling is a
`dangling_ref` instead of a second subject nobody notices, and so `context` can show the reader what
the subject is. Every clause names at least one (`min_outbound: 1` on `about`), because two clauses
can only be seen to disagree where they are known to be about one thing. Cut them at the granularity
a paragraph can define and two to five clauses hang off: too coarse is a false conflict, too fine is
a missed one, and `docdag stats` reports the per-topic counts to watch it with.

**conform** — a conformance test, `conform/uz-v-001`, written by whoever builds the harness. It
declares `enforces:`, and that edge is what gives a `MUST` its force. The identifier carries a
slash, so no file name can hold it and the document writes `id:` in its frontmatter.

**deviation** — a recorded departure from a clause, `dev-0001`. It runs from the day it was recorded
to the day it names under `expires:`, which is the deviation's own field: a record that departs from
two clauses has one lifetime and not two. Past that day it stops counting — for
`deviation_pressure`, and for anything else that reads the edges it declares — and reports
`expired_deviation` while its status still says accepted. `closed: true`, like a clause: a deviation
is a hand-written record too.

**measure** — one measurement of one clause on one day, `interp/UZ-V-001@2026-08-01`, generated by
the standard's own CLI. It declares `measures:` with a required agreement rate and model name.

**premise** — something the standard assumes is true, `premise/runs-are-reproducible`, written by
whoever noticed the assumption. It names the day the world stopped making it true under
`retired_on:`, and every accepted clause still resting on a premise past that day is a
`stale_premise` finding. `retired` stays in its status vocabulary for the person writing the
document; the rule reads the day, so a premise retired next month holds its clauses up until next
month.

**principle** — the reason behind a family of clauses, `principle/evidence-over-assertion`. Clauses
point at it through `rationale:`, and it may carry a `counterexample:` of its own.

**pm** — a post-mortem, `pm-0001`: the case a clause or a principle failed as. It is written rather
than decided, so its vocabulary is `draft` and `published` and it is never `accepted` — which keeps
the rules that read `accepted` from ever reaching one.

### Operating conventions

The preset only enforces what a graph check can. The rest of how a corpus under it is kept is a
convention, and these four are the ones the rules assume:

**Edges live on the machine-generated side.** A `conform` document declares `enforces:` and a
`measure` document declares `measures:`; a clause carries neither `enforced-by:` nor `measured-by:`.
A clause changes when someone decides something, and a test or a measurement appears every time a
machine runs — declaring the edge on the side that changes often keeps the side that changes rarely
out of the diff.

**Measure documents are generated, never hand-written.** They are the output of the standard's CLI,
one file per clause per run. Their freshness is the existence of the file and its git history — what
`stats --fields` reads — rather than an `updated:` field somebody has to remember to change: a
hand-maintained freshness field is a derived value written down, and derived values rot.

**A conformance test body lives outside Markdown.** The executable — `test.sh`, a test binary, a
CI job — is not a document, so the `conform` document is a thin frontmatter wrapper that names its
path under `test:`. The key is declared under the kind's `fields:`, so `stats --fields` counts it and
a `closed: true` conform kind would accept it, even though the preset leaves the kind open. DocDag
does not read the file it points at, and there is no `derived_edges` rule that lifts edges out of
TOML or JSON: frontmatter stays the one place an edge is declared.

**Force is derived, never written.** No document writes down whether it is binding or at what
strength it is in force. `enforced`, `effective_must`, `effective_should` and `effective` answer both
from the graph, and a clause claiming `modality: MUST` that no test enforces is an `orphan_must`
error and carries the force of a `SHOULD` in the meantime — which is what `effective_should` says. A
`MUST_NOT` is a strict rule in exactly the same way, so it needs the same test behind it and falls
to a `SHOULD_NOT` without one; `effective_must` is the pair of them.

`binding:` names `effective`, which is every accepted clause that is in force on the day being asked
about and that no in-force successor has taken over from, at whatever strength the three projections
leave it with, the explicit permission included. It is wider than `effective_must` on purpose: a
permission and a prohibition can only be seen to conflict if both are in the set, so
`query --binding` lists them all with a `modality` column saying which is which, and
`modality_conflict` compares the ones that are actually in force. It is not called `in_force`: that
name is the attribute the engine computes from `period:`, and a projection shadows an attribute
spelled the same way — so the engine rejects one, and `effective` reads it instead.

**An exception is recorded, never inferred.** Where a permission and a prohibition are meant to
stand side by side, the more specific clause declares `excepts:` on the general one with a `scope:`
saying when it applies. DocDag never reads that prose — it records it, `context` shows it, and the
conflict it answers stops being reported. What it will not do is let an exception be recorded
against a `MUST` or a `MUST_NOT`: a strict rule's consequence follows without exception, and that is
`excepts_strict`.

## filename and template

Set `template: <path>` to replace the document template `docdag new` uses, and `filename:` to change
what it names the result — the template must carry `{id}`, may carry `{slug}`, and may not carry a
path separator.

A kind that declares an `id:` pattern names its files after the identifier instead, and ignores the
template: the pattern is the canonical spelling, and a file name whose stem the pattern accepts is
what the reader turns back into identity, so a slug beside it would leave the name and the identity
disagreeing. An identifier carrying a slash — `conform/uz-v-001` — is one no file name can hold, so
its last segment names the file (`uz-v-001.md`) and the frontmatter `id:` carries the whole of it. A
kind that declares no `id:` keeps the digit-run identity, and with it the `filename:` template.

## Structural escalation

Structural checks are not rules. `structural:` may raise one — `missing_frontmatter` and
`unstructured_supersedes` are the two that warn by default — but lowering one, or naming a check
that does not exist, is a configuration error (exit 3), and no check can be disabled.

## Assembling a configuration in Go

The YAML keys on this page are also fields of `config.Config`. A program imports
`github.com/Kaikei-e/DocDag/config`, starts from `SpecPreset()` or `ADRPreset()`, mutates kinds,
edges and rules, calls `Validate()`, then `yaml.Marshal`s the value into `docdag.yaml`. The marshal
is deterministic and round-trips through `Load`. See
[ADR 0006](adr/0006-public-config-yaml-roundtrip-and-append-only.md) for the stable surface, and
`lint.Check` in package `lint` for the three lint layers without exec.
