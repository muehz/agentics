#!/usr/bin/env bash
set -euo pipefail

# Run the real validator without registering or changing any installed plugin.
CACHE_ROOT=/tmp
source "$1/.github/scripts/plugin/stage.sh"
fixture_parent=$(cd "${TMPDIR:-/tmp}" && pwd -P)
fixture=$(mktemp -d "$fixture_parent/agentics-stage-test.XXXXXX")
[[ "$fixture" == "$fixture_parent"/agentics-stage-test.* ]] || exit 1
trap 'rm -rf "$fixture"' EXIT
REPO_ROOT="$fixture/repo"
STAGE_DIR="$fixture/stage"
mkdir -p "$REPO_ROOT/hooks/bin" "$STAGE_DIR/hooks/bin"
for manifest in .claude-plugin/marketplace.json .agents/plugins/marketplace.json; do
    mkdir -p "$STAGE_DIR/$(dirname "$manifest")"
    printf '{}\n' > "$STAGE_DIR/$manifest"
done
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | awk '{print $1}'; }
for file in run-hook.cmd fetch-hook.ps1; do printf 'fixture\n' > "$STAGE_DIR/hooks/$file"; done
for binary in agx-linux-amd64 agx-windows-amd64.exe; do
    printf 'fixture %s\n' "$binary" > "$REPO_ROOT/hooks/bin/$binary"
    cp "$REPO_ROOT/hooks/bin/$binary" "$STAGE_DIR/hooks/bin/$binary"
done
chmod 644 "$STAGE_DIR/hooks/bin/agx-linux-amd64"
chmod 755 "$STAGE_DIR/hooks/bin/agx-windows-amd64.exe"
zero=$(printf '0%.0s' $(seq 1 64))
{
    printf '%s  bin/agx-darwin-amd64\n' "$zero"
    printf '%s  bin/agx-darwin-arm64\n' "$zero"
    printf '%s  bin/agx-linux-amd64\n' "$(sha "$STAGE_DIR/hooks/bin/agx-linux-amd64")"
    printf '%s  bin/agx-linux-arm64\n' "$zero"
    printf '%s  bin/agx-windows-amd64.exe\n' "$(sha "$STAGE_DIR/hooks/bin/agx-windows-amd64.exe")"
    printf '%s  bin/agx-windows-arm64.exe\n' "$zero"
} > "$STAGE_DIR/hooks/SHA256SUMS"

uname() { printf 'MINGW64_NT-10.0\n'; }
verify_stage

rm "$STAGE_DIR/hooks/bin/agx-linux-amd64"
if verify_stage > "$fixture/missing.log" 2>&1; then
    echo 'Missing foreign binary was accepted' >&2
    exit 1
fi
grep -F 'hooks/bin/agx-linux-amd64 is missing' "$fixture/missing.log"
cp "$REPO_ROOT/hooks/bin/agx-linux-amd64" "$STAGE_DIR/hooks/bin/agx-linux-amd64"
chmod 644 "$STAGE_DIR/hooks/bin/agx-linux-amd64"

uname() { printf 'Linux\n'; }
if verify_stage > "$fixture/mode.log" 2>&1; then
    echo 'Non-executable Unix binary was accepted' >&2
    exit 1
fi
grep -F 'hooks/bin/agx-linux-amd64 is not executable' "$fixture/mode.log"

chmod 755 "$STAGE_DIR/hooks/bin/agx-linux-amd64"
printf 'tampered\n' >> "$STAGE_DIR/hooks/bin/agx-linux-amd64"
if verify_stage > "$fixture/hash.log" 2>&1; then
    echo 'Binary disagreeing with SHA256SUMS was accepted' >&2
    exit 1
fi
grep -F 'hooks/bin/agx-linux-amd64 does not match hooks/SHA256SUMS' "$fixture/hash.log"

rm -rf "$REPO_ROOT/hooks/bin" "$STAGE_DIR/hooks/bin"
verify_stage

cp "$STAGE_DIR/hooks/SHA256SUMS" "$fixture/SHA256SUMS"
grep -v 'linux-arm64' "$fixture/SHA256SUMS" > "$STAGE_DIR/hooks/SHA256SUMS"
if verify_stage > "$fixture/entry.log" 2>&1; then
    echo 'Manifest missing an entry was accepted' >&2
    exit 1
fi
grep -F 'has no entry for bin/agx-linux-arm64' "$fixture/entry.log"

# A binary-mode "*bin/" line is an entry run-hook.cmd and fetch-hook.ps1
# never match, so the stage check must not count it either.
sed 's#  bin/agx-linux-arm64# *bin/agx-linux-arm64#' "$fixture/SHA256SUMS" > "$STAGE_DIR/hooks/SHA256SUMS"
if verify_stage > "$fixture/binary-mode.log" 2>&1; then
    echo 'Binary-mode manifest entry was accepted' >&2
    exit 1
fi
grep -F 'has no entry for bin/agx-linux-arm64' "$fixture/binary-mode.log"

# The manifest lists exactly the six contract entries; build-all never
# writes a seventh, so one is a hand edit nothing verifies.
cp "$fixture/SHA256SUMS" "$STAGE_DIR/hooks/SHA256SUMS"
printf '%s  bin/agx-plan9-amd64\n' "$(printf '0%.0s' $(seq 64))" >> "$STAGE_DIR/hooks/SHA256SUMS"
if verify_stage > "$fixture/extra.log" 2>&1; then
    echo 'Manifest with an extra entry was accepted' >&2
    exit 1
fi
grep -F 'unexpected entry bin/agx-plan9-amd64' "$fixture/extra.log"

# A duplicate of a valid line, or a line without two fields, is also a hand
# edit: the six-entry check counts lines, not just distinct paths.
cp "$fixture/SHA256SUMS" "$STAGE_DIR/hooks/SHA256SUMS"
grep 'linux-amd64' "$fixture/SHA256SUMS" >> "$STAGE_DIR/hooks/SHA256SUMS"
if verify_stage > "$fixture/duplicate.log" 2>&1; then
    echo 'Manifest with a duplicate entry was accepted' >&2
    exit 1
fi
grep -F 'duplicate entry bin/agx-linux-amd64' "$fixture/duplicate.log"

cp "$fixture/SHA256SUMS" "$STAGE_DIR/hooks/SHA256SUMS"
printf 'not-a-manifest-line\n' >> "$STAGE_DIR/hooks/SHA256SUMS"
if verify_stage > "$fixture/malformed.log" 2>&1; then
    echo 'Manifest with a malformed line was accepted' >&2
    exit 1
fi
grep -F 'malformed line 7' "$fixture/malformed.log"

echo 'PASS stage platform validation'
