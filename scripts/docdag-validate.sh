#!/bin/sh
# PostToolUse hook: validate the typed graph after a managed document is
# edited, or lint docdag.yaml after the configuration is edited. Reads the hook
# payload on stdin, exits 0 for anything that is not a managed document, and
# exits 2 with the report on stderr so Claude sees it.
set -u

root=${CLAUDE_PROJECT_DIR:-$PWD}
cd "$root" 2>/dev/null || exit 0

payload=$(cat)

if ! command -v jq >/dev/null 2>&1; then
    echo "docdag hook: jq is not installed, enforcement is OFF" >&2
    exit 0
fi

err_file=$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/docdag-hook-$$.err")
trap 'rm -f "$err_file"' EXIT INT HUP TERM

file_path=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty')
case "$file_path" in
    "") exit 0 ;;
    *.md) ;;
    */docdag.yaml)
        # The configuration is a different question from a document: what breaks
        # when it changes is the rules themselves, so the edit is linted rather
        # than validated. Layer 1 reads no documents and costs milliseconds; the
        # corpus and fixture layers belong in CI.
        if ! command -v docdag >/dev/null 2>&1; then
            echo "docdag hook: docdag is not on PATH, enforcement is OFF" >&2
            exit 0
        fi
        # --config names the file that was edited. Without it discovery answers
        # from the project root, so editing a nested docdag.yaml would lint a
        # different configuration and report findings about lines nobody
        # touched.
        report=$(docdag lint --config "$file_path" 2>"$err_file")
        status=$?
        case $status in
            1|2)
                [ -n "$report" ] || exit 0
                printf '%s\n' "$report" >&2
                exit 2
                ;;
            0) exit 0 ;;
            *)
                if [ -s "$err_file" ]; then
                    cat "$err_file" >&2
                else
                    echo "docdag hook: docdag exited $status, run docdag lint to see why" >&2
                fi
                exit 2
                ;;
        esac
        ;;
    *) exit 0 ;;
esac

if ! command -v docdag >/dev/null 2>&1; then
    echo "docdag hook: docdag is not on PATH, enforcement is OFF" >&2
    exit 0
fi

report=$(docdag validate --touching "$file_path" --if-managed 2>"$err_file")
status=$?
case $status in
    1)
        # docdag exits on the whole corpus; a failure with nothing to say about
        # this file is somebody else's edit, not this one.
        [ -n "$report" ] || exit 0
        printf '%s\n' "$report" >&2
        exit 2
        ;;
    2)
        # Older docdag binaries lack --if-managed; warn non-blockingly instead
        # of breaking every Markdown edit with a usage error.
        if grep -q 'unknown flag: --if-managed' "$err_file" 2>/dev/null; then
            echo "docdag hook: docdag does not support --if-managed, enforcement is OFF" >&2
            exit 0
        fi
        if [ -s "$err_file" ]; then
            cat "$err_file" >&2
        else
            echo "docdag hook: docdag exited $status, run docdag validate to see why" >&2
        fi
        exit 2
        ;;
    0) exit 0 ;;
    *)
        if [ -s "$err_file" ]; then
            cat "$err_file" >&2
        else
            echo "docdag hook: docdag exited $status, run docdag validate to see why" >&2
        fi
        exit 2
        ;;
esac
