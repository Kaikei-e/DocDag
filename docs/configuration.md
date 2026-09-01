# docdag.yaml

Configuration is optional. DocDag reads `docdag.yaml` from the repository root, or from
`--config <path>`, and merges it over the preset; flags win over both. See
[checks.md](checks.md) for what each check reports and [commands.md](commands.md) for the flags.

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
    min_outbound: 1

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

### What a multi-kind corpus does not do yet

`validate --immutable-since` refuses one: it reads one directory under one identity rule.

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
      expires:   {type: date}                    # optional, and a date when written
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
    expires: 2026-01-01
```

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

## preset_version and fields

`preset_version:` is the revision of the configuration a corpus is written against. It is a plain
integer the configuration carries and DocDag never interprets: it is written into the JSON output
headers — `validate --format json` and `context --format json` — so a repository can pin the
revision its documents were checked under. The `adr` preset is revision 1, and a corpus that
overrides it says so by writing its own number.

`fields:` declares the lifecycle of the frontmatter keys the documents write. A key nobody declares
is not unknown, only undeclared, so a corpus pays for this only where it is migrating something:

```yaml
preset_version: 3
fields:
  owner:                       # the key being retired
    deprecated: true           # writing it is the deprecated_field finding
    since: 2                   # the preset revision that retired it
    migrate_to: owned-by       # where the value goes; becomes the fix line
    sunset: 2027-01-01         # the last day it is tolerated; after it, an error
  team: {}                     # declared, not retired: a known key and nothing more
```

`deprecated: true` is what makes a declaration do anything. `since`, `migrate_to` and `sunset`
describe a retirement, so writing one of them without `deprecated: true` is a configuration error
(exit 3), as is a `sunset` that is not a `YYYY-MM-DD` day, a negative `since`, a nameless field, and
a field named after an edge's `key:` or `inverse:` — that key's lifecycle belongs to the edge, and
retiring it would say the relation is over without retiring the edge.

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

## List replacement

The `edges:` and `rules:` lists above show where the new keys go; writing one of them — or
`derived_edges:`, `projections:` or the `kinds:` and `fields:` maps — replaces the preset's rather
than adding to it, and writing it as an explicit empty list (`derived_edges: []`, `kinds: {}`)
clears the preset's without putting anything in its place. `preset_version:` is a scalar, so writing
it replaces the preset's revision and leaving it out keeps it. Writing `edges:` also drops the inherited projections that
read an edge type the new list does not declare, and the `binding:` that named one of them: a
projection over a vocabulary the corpus replaced away cannot be evaluated. `binding:` is a scalar,
so writing it replaces the preset's and leaving it out keeps it.

## The rule vocabulary

A rule's `when` block ANDs its top-level clauses. The vocabulary is fixed and complete: `inbound`,
`not_inbound`, `outbound`, `not_outbound` — each naming a declared edge type — `attr: {<key>:
{eq|not: <value>}}` on a scalar, `attr: {<key>: {contains|not_contains: <value>}}` and
`attr: {<key>: {subset_of: [<value>, …]}}` on a list, `via` and `via_inbound` on a neighbour, and
the two combinators `any_of: [<condition>, …]` and `not: <condition>`, which nest. A scalar read as
a list is a one-element list; comparison is case-insensitive; a positive clause needs the attribute
to be there and a negative one is satisfied by its absence. There is no expression language: no
arithmetic, no string operations, no variables.

`inbound` and `outbound` read either as an edge name or as a degree threshold, and the name alone is
sugar for one edge or more. `via` and `via_inbound` reach exactly one hop: they hold when at least
one neighbour across the named edge type satisfies every attribute clause they carry. A neighbour
condition is attributes only — no edge clause, and no `via` inside a `via` — so a condition stays a
question about a document and its immediate neighbourhood. Transitive reach is what `resolve` is
for.

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
      - when: { attr: { level: { eq: SHOULD } } }
      - when: { attr: { level: { eq: MUST } }, not_inbound: enforces }

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

## The spec preset

`preset: spec` is the second built-in configuration: a normative standard as a graph of clauses, the
conformance tests that enforce them, the deviations recorded against them and the measurements taken
of them. It uses every key above — seven kinds, edges constrained by kind and carrying attributes,
projections and a binding of its own — and a corpus adopts the whole of it with one line:

```yaml
preset: spec
```

The file below is that preset in full:

```yaml
preset: spec
preset_version: 1
status_field: status

kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    status_values: [proposed, trial, accepted, superseded, withdrawn]
    closed: true
    fields:
      level: {}                 # MUST | SHOULD | MAY, the strength the clause claims
  conform:
    dir: spec/conform
    id: '^conform/[a-z0-9-]+$'
    fields:
      test: {}                  # path to the executable test body
  deviation:
    dir: spec/deviations
    id: '^dev-\d{4}$'
    status_values: [proposed, accepted, resolved, withdrawn]
    closed: true
  measure:
    dir: spec/measures
    id: '^interp/UZ-[A-Z]-\d{3}@\d{4}-\d{2}-\d{2}$'
  premise:
    dir: spec/premises
    id: '^premise/[a-z0-9/-]+$'
    status_values: [proposed, accepted, retired, superseded]
  principle:
    dir: spec/principles
    id: '^principle/[a-z0-9/-]+$'
    status_values: [proposed, accepted, superseded, withdrawn]
  pm:
    dir: spec/pm
    id: '^pm-\d{4}$'
    status_values: [draft, published]

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
  - name: deviates-from
    key: deviates-from
    direction: forward
    from: [deviation]
    to: [clause]
    attrs:
      expires: {required: true, type: date}
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

projections:
  - name: enforced
    when: {inbound: enforces}
  - name: effective_must
    when:
      attr: {level: {eq: MUST}, status: {eq: accepted}}
      inbound: enforces
      not_inbound: supersedes
  - name: effective_should
    any_of:
      - when:
          attr: {level: {eq: SHOULD}, status: {eq: accepted}}
          not_inbound: supersedes
      - when:
          attr: {level: {eq: MUST}, status: {eq: accepted}}
          not_inbound: enforces
          not: {inbound: supersedes}      # a condition holds one not_inbound; this is the other

binding: effective_must

rules:
  - name: orphan_must
    severity: error
    when:
      attr: {level: {eq: MUST}, status: {eq: accepted}}
      not_inbound: enforces
    message: "is MUST and accepted but nothing enforces it"
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
      via: {edge: premise, attr: {status: {eq: retired}}}
    message: "is accepted but a premise is retired"
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
and the one every other kind points at. It claims a strength in `level:` (`MUST`, `SHOULD`, `MAY`),
and it carries only the relations that are about its own content: `premise:`, `rationale:` and
`counterexample:`. Its frontmatter is `closed: true`, so a key nobody declared is a finding rather
than another tool's field; `level` is declared under the kind's `fields:`, which is what admits it.

**conform** — a conformance test, `conform/uz-v-001`, written by whoever builds the harness. It
declares `enforces:`, and that edge is what gives a `MUST` its force. The identifier carries a
slash, so no file name can hold it and the document writes `id:` in its frontmatter.

**deviation** — a recorded departure from a clause, `dev-0001`. It declares `deviates-from:` with a
required `expires:` attribute, so every departure has an end date written on the edge that departs
rather than in a field of the document. `closed: true`, like a clause: a deviation is a
hand-written record too.

**measure** — one measurement of one clause on one day, `interp/UZ-V-001@2026-08-01`, generated by
the standard's own CLI. It declares `measures:` with a required agreement rate and model name.

**premise** — something the standard assumes is true, `premise/runs-are-reproducible`, written by
whoever noticed the assumption. It is `retired` rather than superseded when the world stops making
it true, and every accepted clause still resting on a retired premise is a `stale_premise` finding.

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

**Force is derived, never written.** No document writes down whether it is binding or what its
effective level is. `enforced`, `effective_must` and `effective_should` answer both from the graph,
`query --binding` lists the clauses that actually bind, and a clause claiming `level: MUST` that no
test enforces is an `orphan_must` error rather than a binding clause.

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
