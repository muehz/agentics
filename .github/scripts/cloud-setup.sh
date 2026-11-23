#!/usr/bin/env bash
# .github/scripts/cloud-setup.sh — Prepare a fresh Linux or macOS checkout (for
# example a Claude Code cloud environment) so a session can build, test, and
# commit.
#
# Usage:
#   ./.github/scripts/cloud-setup.sh                # install, set up, smoke test
#   ./.github/scripts/cloud-setup.sh --skip-tests   # install and set up only
#
# This script is only the bootstrap, because nothing else can run before Node
# exists. It parses its arguments, then, in order, stops at the first wrong tool
# or version:
#   1. Checks git, curl, tar, awk, and a SHA-256 tool.
#   2. Checks Go (hooks/go go.mod) and Node (package.json engines). If the
#      installed one is missing or older, it installs a pinned release and
#      verifies the checksum.
#   3. Runs .github/tools/src/scripts/cloud-setup.ts, which does the rest: the
#      npm floor, the optional tools, worktree setup, the smoke test, and the
#      ready summary. The exit status of that step is this script's.
#
# Environment:
#   AGX_TOOLS_DIR  where installed toolchains live (default: ~/.agentics-tools)
#   AGX_BIN_DIR    where tool links go (default: /usr/local/bin if writable,
#                    else ~/.local/bin)
#
# The script resolves both to absolute paths and exports them for cloud-setup.ts,
# together with AGX_ORIGINAL_PATH, the PATH before it prepended AGX_BIN_DIR.
# It sets AGX_ORIGINAL_PATH itself and ignores any value the caller passes.
#
# Network: proxy.golang.org, registry.npmjs.org, github.com (and its release
# asset host), go.dev and dl.google.com, nodejs.org. The pinned checksums below
# come from the official release pages. Update them when go.mod, package.json
# engines changes.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Installed only when the machine has no Node that satisfies package.json
# engines. It must satisfy engines; the script checks this.
NODE_FALLBACK_VERSION="26.12.1"

fail() {
    echo "cloud-setup: ERROR: $*" >&2
    exit 1
}

warn() {
    echo "cloud-setup: WARNING: $*" >&2
}

step() {
    echo "==> $*"
}

usage() {
    echo "Usage: $0 [--skip-tests]"
    echo ""
    echo "  --skip-tests   Install and set up, but skip the go test smoke test"
    echo "  --help         Show this help"
}

# ---------------------------------------------------------------------------
# Pinned checksums (SHA-256 of the official release archives)
# ---------------------------------------------------------------------------

# go_sha256 <archive-name> prints the checksum listed at https://go.dev/dl/
go_sha256() {
    case "$1" in
        go1.27.4.linux-amd64.tar.gz) echo 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840 ;;
        go1.27.4.linux-arm64.tar.gz) echo 3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92 ;;
        go1.27.4.darwin-amd64.tar.gz) echo 2b799377704ef12db39bd254995d5c348c9cc8d86562ca817724390e56c38f43 ;;
        go1.27.4.darwin-arm64.tar.gz) echo 91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4 ;;
        *) echo "" ;;
    esac
}

# node_sha256 <archive-name> prints the checksum from
# https://nodejs.org/dist/v26.12.1/SHASUMS256.txt
node_sha256() {
    case "$1" in
        node-v26.12.1-linux-x64.tar.xz) echo 6170b2d474e1c626f43cff3adc118df9fe3b573258a4fc3d850c8dbc6b079bbd ;;
        node-v26.12.1-linux-arm64.tar.xz) echo 42aa04922591ee5d4dbc7740d452621acf28508239daa512f5ee8c47ee224fdb ;;
        node-v26.12.1-darwin-x64.tar.gz) echo b85c2ea4420ad0bc4abf2dc6cf1f55130ed681f1cabf60cf3f526e4c9e2712ff ;;
        node-v26.12.1-darwin-arm64.tar.gz) echo 1901d539912c02afa10d1cc126943a8855a7eedebe830119463470db5e6c59e8 ;;
        *) echo "" ;;
    esac
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# version_ge <a> <b> succeeds when version a is at least version b.
version_ge() {
    # Compares numeric dot-separated parts with awk, because sort -V is not in
    # every sort. A missing part counts as 0.
    awk -v a="$1" -v b="$2" 'BEGIN {
        na = split(a, x, ".")
        nb = split(b, y, ".")
        n = (na > nb) ? na : nb
        for (i = 1; i <= n; i++) {
            xi = (i <= na) ? x[i] + 0 : 0
            yi = (i <= nb) ? y[i] + 0 : 0
            if (xi > yi) exit 0
            if (xi < yi) exit 1
        }
        exit 0
    }'
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print $1 }'
    else
        shasum -a 256 "$1" | awk '{ print $1 }'
    fi
}

# download <url> <destination>
download() {
    curl --fail --silent --show-error --location --retry 3 --retry-delay 2 \
        --output "$2" "$1" || fail "download failed: $1"
}

# verify_sha256 <file> <expected>
verify_sha256() {
    local actual
    actual="$(sha256_of "$1")"
    if [ "$actual" != "$2" ]; then
        fail "checksum mismatch for $(basename "$1"): expected $2, got $actual. The download is not trusted."
    fi
}

# module_go_version <module-dir> prints the go directive of a module.
module_go_version() {
    awk '$1 == "go" { print $2; exit }' "$REPO_ROOT/$1/go.mod"
}

# ---------------------------------------------------------------------------
# Tool checks and installs
# ---------------------------------------------------------------------------

detect_platform() {
    case "$(uname -s)" in
        Linux) OS="linux" ;;
        Darwin) OS="darwin" ;;
        *) fail "unsupported operating system $(uname -s). Use Linux or macOS." ;;
    esac
    case "$(uname -m)" in
        x86_64 | amd64)
            GO_ARCH="amd64"
            NODE_ARCH="x64"
            ;;
        aarch64 | arm64)
            GO_ARCH="arm64"
            NODE_ARCH="arm64"
            ;;
        *) fail "unsupported CPU $(uname -m). Use x86_64 or arm64." ;;
    esac
}

check_base_tools() {
    local tool
    for tool in git curl tar awk; do
        command -v "$tool" >/dev/null 2>&1 || fail "$tool is not on PATH. Install it and run this script again."
    done
    command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 ||
        fail "neither sha256sum nor shasum is on PATH. A checksum tool is required."
    git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
        fail "$REPO_ROOT is not a Git checkout. A cloud session needs a readable Git checkout."
    git -C "$REPO_ROOT" remote get-url origin >/dev/null 2>&1 ||
        warn "this checkout has no 'origin' remote. A cloud session needs a GitHub remote."
}

# installed_go_version prints the version of the Go on PATH, or nothing.
# GOTOOLCHAIN=local stops Go from downloading a different toolchain.
installed_go_version() {
    command -v go >/dev/null 2>&1 || return 0
    GOTOOLCHAIN=local go env GOVERSION 2>/dev/null | sed 's/^go//' || true
}

install_go() {
    local want="$1" archive_name sum archive
    archive_name="go${want}.${OS}-${GO_ARCH}.tar.gz"
    sum="$(go_sha256 "$archive_name")"
    [ -n "$sum" ] ||
        fail "no pinned checksum for $archive_name. Add it to go_sha256 in $0 from https://go.dev/dl/."
    step "Installing Go $want"
    archive="$WORK_DIR/$archive_name"
    download "https://go.dev/dl/$archive_name" "$archive"
    verify_sha256 "$archive" "$sum"
    rm -rf "${TOOLS_DIR:?}/go"
    tar -xzf "$archive" -C "$TOOLS_DIR"
    ln -sf "$TOOLS_DIR/go/bin/go" "$BIN_DIR/go"
    ln -sf "$TOOLS_DIR/go/bin/gofmt" "$BIN_DIR/gofmt"
}

ensure_go() {
    local hooks_version have
    hooks_version="$(module_go_version hooks/go)"
    [ -n "$hooks_version" ] || fail "cannot read the go directive from hooks/go/go.mod."
    GO_REQUIRED="$hooks_version"

    have="$(installed_go_version)"
    if [ -n "$have" ] && version_ge "$have" "$GO_REQUIRED"; then
        echo "Go $have satisfies go.mod ($GO_REQUIRED)."
    else
        [ -z "$have" ] || warn "Go $have is older than the required $GO_REQUIRED."
        install_go "$GO_REQUIRED"
    fi

    have="$(installed_go_version)"
    [ -n "$have" ] || fail "Go is not on PATH after install. Put $BIN_DIR on PATH."
    version_ge "$have" "$GO_REQUIRED" ||
        fail "Go $have is on PATH but go.mod requires $GO_REQUIRED. Remove the older Go from PATH or put $BIN_DIR first."
}

# node_range prints the engines.node range from package.json.
node_range() {
    sed -n 's/.*"node"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$REPO_ROOT/package.json" | head -n 1
}

# node_satisfies RANGE VERSION asks the installed node, through node-range.ts,
# the helper the TypeScript side shares, so both agree on what engines.node
# allows. It returns 1 when VERSION is outside RANGE and stops when RANGE is a
# form the helper does not evaluate.
node_satisfies() {
    local rc=0
    # The shared helper is a .ts file that uses import.meta.main. Only Node
    # 22.18.0 up to 23.0.0 and 24.2.0 or later both run a .ts file and have
    # import.meta.main. Any other or malformed installed version must reach
    # provisioning before we ask Node to load that helper.
    if [[ ! "$2" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        return 1
    fi
    if ! { version_ge "$2" "22.18.0" && ! version_ge "$2" "23.0.0"; } &&
        ! version_ge "$2" "24.2.0"; then
        return 1
    fi
    node "$REPO_ROOT/.github/tools/src/scripts/node-range.ts" "$1" "$2" || rc=$?
    case "$rc" in
        0) return 0 ;;
        1) return 1 ;;
        *) fail "package.json engines.node \"$1\" is a form .github/tools/src/scripts/node-range.ts cannot evaluate." ;;
    esac
}

installed_node_version() {
    command -v node >/dev/null 2>&1 || return 0
    node --version 2>/dev/null | sed 's/^v//' || true
}

install_node() {
    local want="$1" ext archive_name sum archive
    case "$OS" in
        linux) ext="tar.xz" ;;
        *) ext="tar.gz" ;;
    esac
    archive_name="node-v${want}-${OS}-${NODE_ARCH}.${ext}"
    sum="$(node_sha256 "$archive_name")"
    [ -n "$sum" ] ||
        fail "no pinned checksum for $archive_name. Add it to node_sha256 in $0 from https://nodejs.org/dist/."
    step "Installing Node $want"
    archive="$WORK_DIR/$archive_name"
    download "https://nodejs.org/dist/v$want/$archive_name" "$archive"
    verify_sha256 "$archive" "$sum"
    rm -rf "${TOOLS_DIR:?}/node" "${TOOLS_DIR:?}/node-v${want}-${OS}-${NODE_ARCH}"
    tar -xf "$archive" -C "$TOOLS_DIR" ||
        fail "cannot unpack $archive_name. Install xz-utils and run this script again."
    mv "$TOOLS_DIR/node-v${want}-${OS}-${NODE_ARCH}" "$TOOLS_DIR/node"
    ln -sf "$TOOLS_DIR/node/bin/node" "$BIN_DIR/node"
    ln -sf "$TOOLS_DIR/node/bin/npm" "$BIN_DIR/npm"
    ln -sf "$TOOLS_DIR/node/bin/npx" "$BIN_DIR/npx"
}

ensure_node() {
    local range have
    range="$(node_range)"
    [ -n "$range" ] || fail "package.json has no engines.node."

    have="$(installed_node_version)"
    if [ -n "$have" ] && node_satisfies "$range" "$have"; then
        echo "Node $have satisfies engines.node ($range)."
    else
        [ -z "$have" ] || warn "Node $have does not satisfy the required $range."
        install_node "$NODE_FALLBACK_VERSION"
    fi

    have="$(installed_node_version)"
    [ -n "$have" ] || fail "Node is not on PATH after install. Put $BIN_DIR on PATH."
    if ! node_satisfies "$range" "$have"; then
        fail "Node $have is on PATH but package.json requires $range. Put $BIN_DIR first on PATH, and keep NODE_FALLBACK_VERSION in $0 inside that range."
    fi
    command -v npm >/dev/null 2>&1 || fail "npm is not on PATH. Install npm with Node."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

RUN_TESTS=1
for arg in "$@"; do
    case "$arg" in
        --skip-tests) RUN_TESTS=0 ;;
        --help | -h)
            usage
            exit 0
            ;;
        *)
            usage >&2
            fail "unknown option: $arg"
            ;;
    esac
done

detect_platform

TOOLS_DIR="${AGX_TOOLS_DIR:-${HOME:?HOME is not set}/.agentics-tools}"
if [ -n "${AGX_BIN_DIR:-}" ]; then
    BIN_DIR="$AGX_BIN_DIR"
elif [ -w /usr/local/bin ]; then
    BIN_DIR="/usr/local/bin"
else
    BIN_DIR="$HOME/.local/bin"
fi
mkdir -p "$TOOLS_DIR" "$BIN_DIR" || fail "cannot create $TOOLS_DIR or $BIN_DIR."
# Links and PATH need absolute paths, because the script changes directory.
TOOLS_DIR="$(cd "$TOOLS_DIR" && pwd)"
BIN_DIR="$(cd "$BIN_DIR" && pwd)"
ORIGINAL_PATH="$PATH"
export PATH="$BIN_DIR:$PATH"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/agentics-cloud-setup.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT

# An old GOROOT would point the new Go at the wrong tree. GOTOOLCHAIN=local
# keeps Go on the toolchain this script checked, as CI does.
unset GOROOT
export GOTOOLCHAIN=local

cd "$REPO_ROOT"

step "Checking base tools"
check_base_tools

step "Checking Go"
ensure_go

step "Checking Node"
ensure_node

# Absolute paths, always overwritten. The child is not exec'd so the EXIT trap
# still removes WORK_DIR, and its status is returned unchanged because
# cloud-setup.ts prints its own error.
export AGX_TOOLS_DIR="$TOOLS_DIR"
export AGX_BIN_DIR="$BIN_DIR"
export AGX_ORIGINAL_PATH="$ORIGINAL_PATH"

# The argument loop is done, so the positional parameters carry the hand-off.
if [ "$RUN_TESTS" -eq 0 ]; then
    set -- --skip-tests
else
    set --
fi
status=0
node .github/tools/src/scripts/cloud-setup.ts "$@" || status=$?
exit "$status"
