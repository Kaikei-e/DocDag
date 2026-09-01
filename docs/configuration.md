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
`derived_edges:` or `projections:` — replaces the preset's list rather than adding to it, and
writing it as an explicit empty list (`derived_edges: []`) clears the preset's without putting
anything in its place. Writing `edges:` also drops the inherited projections that read an edge type
the new list does not declare, and the `binding:` that named one of them: a projection over a
vocabulary the corpus replaced away cannot be evaluated. `binding:` is a scalar, so writing it
replaces the preset's and leaving it out keeps it.

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
