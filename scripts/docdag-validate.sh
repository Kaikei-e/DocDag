#!/bin/sh
# PostToolUse hook: validate the decision graph after a decision record is
# edited. Reads the hook payload on stdin, exits 0 for anything that is not a
# managed document, and exits 2 with the report on stderr so Claude sees it.
set -u

root=${CLAUDE_PROJECT_DIR:-$PWD}
cd "$root" 2>/dev/null || exit 0

payload=$(cat)

if ! command -v jq >/dev/null 2>&1; then
    echo "docdag hook: jq is not installed, skipping validation" >&2
    exit 0
fi

file_path=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty')
case "$file_path" in
    "") exit 0 ;;
    *.md) ;;
    *) exit 0 ;;
esac

# The configured documents directory wins; otherwise try the same well-known
# names discovery does.
docs_dirs=""
if [ -f docdag.yaml ]; then
    configured=$(sed -n 's/^dir:[[:space:]]*//p' docdag.yaml | head -n 1 | sed 's/[[:space:]]*#.*$//' | tr -d "\"'")
    [ -n "$configured" ] && docs_dirs=$configured
fi
if [ -z "$docs_dirs" ]; then
    docs_dirs="docs/adr doc/adr docs/decisions docs/ADR adr"
fi

under_docs=0
for dir in $docs_dirs; do
    [ -d "$dir" ] || continue
    absolute=$(cd "$dir" && pwd) || continue
    case "$file_path" in
        "$absolute"/*) under_docs=1 ;;
    esac
done
[ "$under_docs" -eq 1 ] || exit 0

if ! command -v docdag >/dev/null 2>&1; then
    echo "docdag hook: docdag is not on PATH, skipping validation" >&2
    exit 0
fi

report=$(docdag validate --touching "$file_path" 2>/dev/null)
status=$?
case $status in
    1)
        # docdag exits on the whole corpus; a failure with nothing to say about
        # this file is somebody else's edit, not this one.
        [ -n "$report" ] || exit 0
        printf '%s\n' "$report" >&2
        exit 2
        ;;
    0) exit 0 ;;
    *)
        echo "docdag hook: docdag exited $status, run docdag validate to see why" >&2
        exit 0
        ;;
esac
