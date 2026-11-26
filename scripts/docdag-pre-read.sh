#!/bin/sh
# PreToolUse advisory hook for Read|Glob|Grep: when targeting a DocDag-managed
# path or directory, emit non-blocking additionalContext steering the agent
# toward docdag context / query commands. Always exits 0.
set -u

root=${CLAUDE_PROJECT_DIR:-$PWD}
cd "$root" 2>/dev/null || exit 0

if ! command -v jq >/dev/null 2>&1 || ! command -v docdag >/dev/null 2>&1; then
    exit 0
fi

has_corpus=0
if [ -f docdag.yaml ]; then
    has_corpus=1
else
    for dir in docs/adr doc/adr docs/decisions docs/ADR adr; do
        if [ -d "$dir" ]; then
            has_corpus=1
            break
        fi
    done
fi
[ "$has_corpus" -eq 1 ] || exit 0

payload=$(cat)

tool_name=$(printf '%s' "$payload" | jq -r '.tool_name // empty' 2>/dev/null)
target=""

case "$tool_name" in
    Read)
        target=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty' 2>/dev/null)
        ;;
    Glob)
        dir_path=$(printf '%s' "$payload" | jq -r '.tool_input.path // empty' 2>/dev/null)
        pattern=$(printf '%s' "$payload" | jq -r '.tool_input.pattern // empty' 2>/dev/null)
        if [ -n "$dir_path" ] && [ -n "$pattern" ]; then
            case "$pattern" in
                /*) target="$pattern" ;;
                *) target="${dir_path%/}/$pattern" ;;
            esac
        elif [ -n "$pattern" ]; then
            target="$pattern"
        else
            target="$dir_path"
        fi
        ;;
    Grep)
        target=$(printf '%s' "$payload" | jq -r '.tool_input.path // empty' 2>/dev/null)
        if [ -z "$target" ]; then
            target=$(printf '%s' "$payload" | jq -r '.tool_input.glob // empty' 2>/dev/null)
        fi
        ;;
    *)
        exit 0
        ;;
esac

[ -n "$target" ] || exit 0

hint=$(docdag prime --if-present --hint "$target" 2>/dev/null) || exit 0
[ -n "$hint" ] || exit 0

jq -n --arg ctx "$hint" '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$ctx}}'
exit 0
