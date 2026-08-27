# Continuous integration

`docdag validate` exits 1 on any error, so one step gates a repository. See
[commands.md](commands.md) for the output formats and [checks.md](checks.md) for what each finding
means.

## The composite action

The composite action downloads the release binary for the runner, verifies it against the release's
`checksums.txt` before running it, and needs no Go toolchain:

```yaml
name: decisions
on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.2.0
        with:
          version: v0.2.0                  # default: latest
          args: validate --format github   # this is the default
          working-directory: .             # this is the default
```

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | the release tag to install; `latest` resolves to the newest release |
| `args` | `validate --format github` | the arguments passed to `docdag` |
| `working-directory` | `.` | the directory `docdag` runs in |

The action runs on Linux and macOS runners, amd64 and arm64. On a Windows runner it fails with a
diagnostic rather than passing silently; install with `go install` there instead.

### Pinning

A tag can be moved, so pin the action by commit SHA when the supply chain matters —
`uses: Kaikei-e/DocDag@<40-char-sha> # v0.2.0`. This repository pins the actions it uses that way.

### Annotations

`--format github` writes one workflow command per finding. A workflow step renders at most ten
annotations, so a corpus with more findings than that needs a second `--format text` run written
into `$GITHUB_STEP_SUMMARY` to show the rest:

```yaml
      - uses: Kaikei-e/DocDag@v0.2.0        # annotations, and the gate
      - if: always()
        shell: bash
        run: docdag validate --format text >> "$GITHUB_STEP_SUMMARY"
```

## Append-only history

`docdag validate --immutable-since <rev>` treats a decision that was `accepted`, `superseded` or
`withdrawn` at `<rev>` as a record rather than a draft. It compares the working tree against
`git merge-base <rev> HEAD` and allows exactly three kinds of change to such a document:

- the value of the `status_field`,
- entries **added** under a configured `inverse:` key, so a later decision can record that it
  replaced this one,
- lines appended to the end of the body.

Anything else — another frontmatter key changed, added or removed; an inverse entry removed; a line
of the existing body rewritten; the file deleted — is an `immutable_violation` error naming what
changed. Renames are not followed, so moving a closed document reads as the deletion it is:

```console
$ docdag validate --immutable-since origin/main
docs/adr/0001-serve-images-from-the-application.md:11: ERROR immutable_violation 0001: the body changed at line 11, which append-only history forbids
```

A document that `<rev>` did not hold is new and is always allowed. The check runs `git` from `PATH`
and is off unless the flag is given; a corpus outside a git repository, or a machine without `git`,
exits 3.

### Running it in CI

The check needs enough history for `git merge-base <rev> HEAD` to resolve, which a shallow clone
does not have. Ask for the full history and name the branch the change is proposed against:

```yaml
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: Kaikei-e/DocDag@v0.2.0
        with:
          args: validate --immutable-since origin/${{ github.base_ref || github.event.repository.default_branch }}
```

`github.base_ref` is the branch a pull request targets and is empty on a push, where the default
branch is the useful comparison.

## The pre-commit hook

A [pre-commit](https://pre-commit.com) hook is also shipped:

```yaml
repos:
  - repo: https://github.com/Kaikei-e/DocDag
    rev: v0.2.0
    hooks:
      - id: docdag-validate
```

Hooks are advisory: `git commit --no-verify` walks straight past them, and a contributor who never
ran `pre-commit install` never had them. CI is the gate; the hook only shortens the loop.

The plugin ships a `PostToolUse` hook for agents as well — see [agents.md](agents.md).
