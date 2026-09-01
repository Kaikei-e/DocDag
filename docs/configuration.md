# docdag.yaml

Configuration is optional. DocDag reads `docdag.yaml` from the repository root, or from
`--config <path>`, and merges it over the preset; flags win over both. See
[checks.md](checks.md) for what each check reports and [commands.md](commands.md) for the flags.

## The preset

The file below is the `adr` preset in full — what DocDag applies with no configuration:

```yaml
preset: adr
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
`date`, `id`, `kind`, the status field, and every `key:` and `inverse:` an edge declares plus every
`field:` a derived edge reads — so renaming a key in the configuration widens the set with it.
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

### What a multi-kind corpus does not do yet

`docdag new` refuses one — which kind to create, under which identity rules and from which template,
has no default answer — and so does `validate --immutable-since`, which reads one directory under
one identity rule. Per-kind templates arrive with the `spec` preset.

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

## List replacement

The `edges:` and `rules:` lists above show where the new keys go; writing one of them — or
`derived_edges:`, `projections:` or the `kinds:` map — replaces the preset's rather than adding to
it, and writing it as an explicit empty list (`derived_edges: []`, `kinds: {}`) clears the preset's
without putting anything in its place. Writing `edges:` also drops the inherited projections that
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

## filename and template

Set `template: <path>` to replace the document template `docdag new` uses, and `filename:` to change
what it names the result — the template must carry `{id}`, may carry `{slug}`, and may not carry a
path separator.

## Structural escalation

Structural checks are not rules. `structural:` may raise one — `missing_frontmatter` and
`unstructured_supersedes` are the two that warn by default — but lowering one, or naming a check
that does not exist, is a configuration error (exit 3), and no check can be disabled.
