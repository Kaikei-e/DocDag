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

## List replacement

The `edges:` and `rules:` lists above show where the new keys go; writing one of them — or
`derived_edges:` — replaces the preset's list rather than adding to it, and writing it as an
explicit empty list (`derived_edges: []`) clears the preset's without putting anything in its place.

## The rule vocabulary

A rule's `when` block ANDs its top-level clauses. The vocabulary is fixed and complete: `inbound`,
`not_inbound`, `outbound`, `not_outbound` — each naming a declared edge type — `attr: {<key>:
{eq|not: <value>}}` on a scalar, `attr: {<key>: {contains|not_contains: <value>}}` and
`attr: {<key>: {subset_of: [<value>, …]}}` on a list, and the two combinators `any_of: [<condition>,
…]` and `not: <condition>`, which nest. A scalar read as a list is a one-element list; comparison is
case-insensitive; a positive clause needs the attribute to be there and a negative one is satisfied
by its absence. There is no expression language.

## filename and template

Set `template: <path>` to replace the document template `docdag new` uses, and `filename:` to change
what it names the result — the template must carry `{id}`, may carry `{slug}`, and may not carry a
path separator.

## Structural escalation

Structural checks are not rules. `structural:` may raise one — `missing_frontmatter` and
`unstructured_supersedes` are the two that warn by default — but lowering one, or naming a check
that does not exist, is a configuration error (exit 3), and no check can be disabled.
