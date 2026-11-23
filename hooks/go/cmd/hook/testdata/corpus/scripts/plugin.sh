#!/usr/bin/env bash
# .github/scripts/plugin.sh — Register the local checkout as a plugin
# marketplace and install all plugins for local development.
#
# Usage:
#   ./.github/scripts/plugin.sh                    # Setup local development (CLI)
#   ./.github/scripts/plugin.sh --teardown         # Remove local, reinstall from remote
#   ./.github/scripts/plugin.sh --validate         # Run the TypeScript conformance suite and the tools tests
#   ./.github/scripts/plugin.sh --validate --fast  # Same checks without the Go and TypeScript test suites
#   ./.github/scripts/plugin.sh --status           # Show installed plugins
#   ./.github/scripts/plugin.sh --vscode           # Print the VS Code settings to add
#   ./.github/scripts/plugin.sh --vscode-insiders  # Same, for VS Code Insiders
#
# Works with: Claude Code, GitHub Copilot CLI, Codex CLI, VS Code, VS Code Insiders
#
# This file is the entrypoint only: argument parsing, help, and dispatch. Each
# area lives in its own library under plugin/, sourced below:
#
#   plugin/common.sh       repository facts and CLI detection
#   plugin/stage.sh        the clean marketplace stage
#   plugin/marketplace.sh  claude / copilot / codex setup, teardown, status
#   plugin/vscode.sh       VS Code registration instructions
#   plugin/validate.sh     the conformance suite
#
# The CLIs are registered against a clean staged copy of the checkout, not the
# checkout itself; see plugin/stage.sh for why.
#
# Only --validate needs Node 22.18 or later and a Go toolchain, and only as a
# hard requirement. Node runs the TypeScript conformance suite and the tools
# tests; Go builds only the trusted hook verifier that the hooks domain runs.
# The install path checks the manifests on Node and degrades to a warning
# without it, so a fresh clone can install the plugin with neither.

set -euo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/plugin" && pwd)"

# common.sh first: every other library uses REPO_ROOT and CACHE_ROOT.
# shellcheck source=plugin/common.sh
. "$LIB_DIR/common.sh"
# shellcheck source=plugin/stage.sh
. "$LIB_DIR/stage.sh"
# shellcheck source=plugin/validate.sh
. "$LIB_DIR/validate.sh"
# shellcheck source=plugin/vscode.sh
. "$LIB_DIR/vscode.sh"
# shellcheck source=plugin/marketplace.sh
. "$LIB_DIR/marketplace.sh"

# ---------------------------------------------------------------------------
# Argument parsing — extract VS Code variant and action
# ---------------------------------------------------------------------------

VSCODE_VARIANT=""
ACTION=""
VALIDATE_MODE="full"

for arg in "$@"; do
    case "$arg" in
        --vscode) VSCODE_VARIANT="code" ;;
        --vscode-insiders) VSCODE_VARIANT="insiders" ;;
        --teardown) ACTION="teardown" ;;
        --validate) ACTION="validate" ;;
        --fast) VALIDATE_MODE="fast" ;;
        --status) ACTION="status" ;;
        --help | -h) ACTION="help" ;;
        *)
            echo "Unknown option: $arg"
            echo "Run $0 --help for usage."
            exit 1
            ;;
    esac
done

if [ "$ACTION" = "help" ]; then
    echo "Usage: $0 [--vscode|--vscode-insiders] [--teardown|--validate [--fast]|--status|--help]"
    echo ""
    echo "  (no args)              Register local checkout as marketplace and install plugins (CLI)"
    echo "  --teardown             Remove local plugins, reinstall from remote"
    echo "  --validate             Run the TypeScript conformance suite and the tools tests (Node 22.18 or later)"
    echo "  --fast                 With --validate: run only the checkout checks; skip the Go and TypeScript test suites"
    echo "  --status               Show installed plugins and marketplaces"
    echo "  --vscode               Print the VS Code settings to add, check, and remove"
    echo "  --vscode-insiders      Same, for VS Code Insiders"
    echo "  --help                 Show this help"
    echo ""
    echo "VS Code is configured by hand. Agentics prints the three settings and the"
    echo "path to write them to; it never edits your settings.json. The printed"
    echo "block covers setup, checking, and removal, so --vscode ignores"
    echo "--teardown and --status."
    echo ""
    echo "Only --validate needs Node 22.18 or later and a Go toolchain (Go builds only the trusted hook verifier). Installing the plugin needs neither."
    exit 0
fi

# --fast modifies --validate only. Ignoring it elsewhere would let a typo run a
# full install where the caller asked for a quick check.
if [ "$VALIDATE_MODE" = "fast" ] && [ "$ACTION" != "validate" ]; then
    echo "--fast needs --validate."
    echo "Run $0 --help for usage."
    exit 1
fi

EXIT_STATUS=0

if [ -n "$VSCODE_VARIANT" ]; then
    case "$ACTION" in
        # --validate never resolves a settings path: with or without
        # --vscode it acts on the repository.
        validate) run_validate strict "$VALIDATE_MODE" || EXIT_STATUS=$? ;;
        # Setup, status, and teardown all have the same answer for VS Code —
        # the three keys — so one block serves all three.
        *) show_vscode "$VSCODE_VARIANT" || EXIT_STATUS=$? ;;
    esac
else
    case "$ACTION" in
        validate) run_validate strict "$VALIDATE_MODE" || EXIT_STATUS=$? ;;
        teardown) teardown_local || EXIT_STATUS=$? ;;
        status) show_status || EXIT_STATUS=$? ;;
        *) setup_local || EXIT_STATUS=$? ;;
    esac
fi

exit "$EXIT_STATUS"
