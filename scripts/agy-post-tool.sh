#!/bin/sh
# Antigravity PostToolUse hook: validate edited managed documents, lint edited
# docdag.yaml, or queue a one-time corpus hint when reading/searching inside a
# managed corpus. Queued messages are flushed by agy-post-invocation.sh.
# Always outputs {} on stdout and exits 0.
set -u

payload=$(cat)

if ! command -v jq >/dev/null 2>&1; then
    echo '{}'
    exit 0
fi

root=$(printf '%s' "$payload" | jq -r '.cwd // .workspacePaths[0] // empty' 2>/dev/null)
[ -n "$root" ] || { echo '{}'; exit 0; }
cd "$root" 2>/dev/null || { echo '{}'; exit 0; }

raw_conv_id=$(printf '%s' "$payload" | jq -r '.conversationId // empty' 2>/dev/null)
conv_id=$(printf '%s' "$raw_conv_id" | tr -cd 'A-Za-z0-9_-')
[ -n "$conv_id" ] || { echo '{}'; exit 0; }

state_dir="${PLUGIN_DATA:-${TMPDIR:-/tmp}/docdag-agy-hooks}"
mkdir -p "$state_dir" 2>/dev/null || { echo '{}'; exit 0; }

pending_file="$state_dir/${conv_id}.pending"
hinted_file="$state_dir/${conv_id}.hinted"

err_file=""
cleanup() {
    [ -n "$err_file" ] && rm -f "$err_file"
}
trap cleanup EXIT INT HUP TERM

has_corpus() {
    if [ -f docdag.yaml ]; then
        return 0
    fi
    for dir in docs/adr doc/adr docs/decisions docs/ADR adr; do
        if [ -d "$dir" ]; then
            return 0
        fi
    done
    return 1
}

append_pending() {
    printf '%s\n' "$1" >> "$pending_file"
}

append_missing_docdag() {
    msg="docdag hook: docdag is not on PATH, enforcement is OFF"
    if ! grep -qxF "$msg" "$pending_file" 2>/dev/null; then
        append_pending "$msg"
    fi
}

tool_name=$(printf '%s' "$payload" | jq -r '.toolCall.name // empty' 2>/dev/null)

case "$tool_name" in
    edit_file|write_file|replace_file_content|multi_replace_file_content|write_to_file)
        file_path=$(printf '%s' "$payload" | jq -r '.toolCall.args.Path // .toolCall.args.TargetFile // .toolCall.args.path // .toolCall.args.file_path // empty' 2>/dev/null)
        case "$file_path" in
            "")
                echo '{}'
                exit 0
                ;;
            */docdag.yaml|docdag.yaml)
                if ! command -v docdag >/dev/null 2>&1; then
                    append_missing_docdag
                    echo '{}'
                    exit 0
                fi
                err_file=$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/docdag-agy-hook-$$.err")
                report=$(docdag lint --config "$file_path" 2>"$err_file")
                status=$?
                case $status in
                    0) ;;
                    1|2)
                        [ -n "$report" ] && append_pending "$report"
                        ;;
                    *)
                        if [ -s "$err_file" ]; then
                            append_pending "$(cat "$err_file")"
                        else
                            append_pending "docdag hook: docdag exited $status, run docdag lint to see why"
                        fi
                        ;;
                esac
                ;;
            *.md)
                if ! command -v docdag >/dev/null 2>&1; then
                    if has_corpus; then
                        append_missing_docdag
                    fi
                    echo '{}'
                    exit 0
                fi
                err_file=$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/docdag-agy-hook-$$.err")
                report=$(docdag validate --touching "$file_path" --if-managed 2>"$err_file")
                status=$?
                case $status in
                    0) ;;
                    1)
                        [ -n "$report" ] && append_pending "$report"
                        ;;
                    2)
                        if grep -q 'unknown flag: --if-managed' "$err_file" 2>/dev/null; then
                            append_pending "docdag hook: docdag does not support --if-managed, enforcement is OFF"
                        elif [ -s "$err_file" ]; then
                            append_pending "$(cat "$err_file")"
                        else
                            append_pending "docdag hook: docdag exited $status, run docdag validate to see why"
                        fi
                        ;;
                    *)
                        if [ -s "$err_file" ]; then
                            append_pending "$(cat "$err_file")"
                        else
                            append_pending "docdag hook: docdag exited $status, run docdag validate to see why"
                        fi
                        ;;
                esac
                ;;
            *) ;;
        esac
        ;;
    read_file|view_file|find_by_name|grep_search)
        if [ -f "$hinted_file" ]; then
            echo '{}'
            exit 0
        fi
        if ! command -v docdag >/dev/null 2>&1 || ! has_corpus; then
            echo '{}'
            exit 0
        fi
        target=$(printf '%s' "$payload" | jq -r '.toolCall.args.Path // .toolCall.args.AbsolutePath // .toolCall.args.SearchDirectory // .toolCall.args.SearchPath // .toolCall.args.path // empty' 2>/dev/null)
        if [ -z "$target" ]; then
            echo '{}'
            exit 0
        fi
        hint=$(docdag prime --if-present --hint "$target" 2>/dev/null)
        if [ "$?" -eq 0 ] && [ -n "$hint" ]; then
            touch "$hinted_file" 2>/dev/null || true
            append_pending "$hint"
        fi
        ;;
    *) ;;
esac

echo '{}'
exit 0
