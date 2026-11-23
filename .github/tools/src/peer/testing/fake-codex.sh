#!/usr/bin/env bash
# Stands in for the codex CLI. FAKE_CODEX_MODE picks the shape: good (default), payload (write
# {{DIR}}/review-payload to the output file), bad-version (--version exits 7), slow (valid after 3s), hang (sleeps
# 30s), and three process-tree shapes: hang-tree (leader exits on TERM), hang-tree-ignore-leader (leader ignores
# TERM), exit-tree (leader exits 0 and leaves a TERM-immune descendant behind).
mode="${FAKE_CODEX_MODE:-good}"
record() {
    printf 'ANTHROPIC_API_KEY=%s|OPENAI_API_KEY=%s|CODEX_HOME=%s|HOST_SESSION=%s|PLUGIN_ROOT=%s|CLAUDE_PLUGIN_ROOT=%s\n' \
        "${ANTHROPIC_API_KEY-<unset>}" "${OPENAI_API_KEY-<unset>}" "${CODEX_HOME-<unset>}" \
        "${CLAUDE_CODE_SESSION_ACCESS_TOKEN-<unset>}" "${PLUGIN_ROOT-<unset>}" "${CLAUDE_PLUGIN_ROOT-<unset>}" \
        >> "{{DIR}}/codex-environment"
}
if [ "${1:-}" = "--version" ]; then
    echo version >> "{{DIR}}/codex-invocations"
    record
    [ "$mode" = bad-version ] && exit 7
    echo fake 1.0
    exit 0
fi
echo dispatch >> "{{DIR}}/codex-invocations"
record
printf '%s\n' "$*" > "{{DIR}}/codex-argv"
out=""
prev=""
for a in "$@"; do
    [ "$prev" = "--output-last-message" ] && out="$a"
    prev="$a"
done
case "$mode" in
    hang-tree | hang-tree-ignore-leader | exit-tree)
        echo "$$" > "{{DIR}}/leader.pid"
        ps -o pgid= -p "$$" | tr -d ' ' > "{{DIR}}/leader.pgid"
        if [ "$mode" = hang-tree-ignore-leader ]; then trap "" TERM; else trap 'exit 0' TERM; fi
        bash -c '
            trap "" TERM
            echo "$$" > "$1/descendant.pid"
            sleep 30 &
            sleeper=$!
            echo "$sleeper" > "$1/sleeper.pid"
            wait "$sleeper"
        ' bash "{{DIR}}" &
        if [ "$mode" = exit-tree ]; then
            while [ ! -s "{{DIR}}/sleeper.pid" ]; do sleep 0.01; done
            printf 'Fake review.\nREVIEW-COMPLETE: no-findings\n' > "$out"
            exit 0
        fi
        wait "$!"
        exit 0
        ;;
    hang) exec sleep 30 ;;
    slow) sleep 3 ;;
esac
# The prompt travels over stdin. Echoing its Range line into the output file proves it survived the watchdog, and the
# raw copy lets a test check the bytes (command substitution would strip the trailing newlines).
cat > "{{DIR}}/codex-stdin"
prompt="$(cat "{{DIR}}/codex-stdin")"
echo "fake codex transcript (progress only)"
if [ "$mode" = payload ]; then
    echo "REVIEW-COMPLETE: no-findings"
    cat "{{DIR}}/review-payload" > "$out"
    exit 0
fi
{
    printf '%s\n' "$prompt" | grep '^Range:' || true
    echo "Fake codex review. No findings."
    echo "REVIEW-COMPLETE: no-findings"
} > "$out"
