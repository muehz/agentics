// Command agx runs Agentics hooks on every provider surface and the flow CLI.
//
// Usage:
//
//	agx <provider> <hook> [hook args...]
//	agx flow <command> [command args...]
//	agx help
//
// The entry point validates argv and handles provenance, help, flow, and usage
// errors, then hands a hook invocation to internal/checkrun, which owns the
// pipeline and its fault posture.
//
// The dispatcher also uses an internal startup mode after provisioning the
// runtime. Startup context has its own provider protocol and no guard decision.
//
// Hook invocation exit codes:
//
//	0  the hook ran and the surface raised no objection
//	1  usage error or a fail-closed surface's fault code
//	2  a surface's blocking signal (the surface replays stderr to the model)
//
// For hook invocations (the first argument is not flow), exit 2 is load-bearing:
// ONLY a provider's Render mapping may produce it. Argv validation returns 1. A panic
// — which an unrecovered Go program would exit 2 with, complete with a goroutine
// dump on stderr — is recovered, turned into a contract error outcome, and
// RENDERED THROUGH THE CALLING PROVIDER, so a crashed core fails the way each
// surface expects. Exit 2 blocks Claude Code and Copilot pre-tool events, so an
// unrecovered panic would read there as a deliberate deny. Codex reads its
// decision from the response document. A
// flat exit 1 with no body would fail the other way: the two fail-open surfaces
// answer a core fault with exit 0, one of them with a response body, and exit 1
// breaks both of those protocols (see docs/surfaces/*.md, "Hook protocol").
//
// Flow mode runs no hook and uses its own exit codes, including 2 for an invalid
// cell. It reads no stdin and resolves no provider. A flow panic exits 1.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/checkrun"
	"github.com/muehz/agentics/hooks/go/internal/flow"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

const usage = "usage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help"

// commandList is what "agx help", "--help" and "-h" print to stdout.
const commandList = `agx: the Agentics command-line tool

  agx <provider> <hook> [hook args...]   run a hook (called by the hook configuration)
  agx flow <command> [args...]           record or read flow state (log, status, where, bind, path, audit)
  agx help                               print this list

Run "agx flow --help" for the flow commands.
`

// sourceDigest is stamped at link time by hooks/go/Makefile
// (-X main.sourceDigest=agentics-src-<hex>) with the digest cmd/sourcedigest prints
// of the source the binaries are built from (digest-exclude names what it
// skips). The conformance `hooks freshness` check scans each committed
// binary's bytes for the stamp to prove it was built from the checked-in
// source — including the three platforms CI cannot execute. The
// env-gated print in run() is both the human affordance for "which source
// built this binary?" and the reference that keeps the linker from
// discarding the symbol (an unreferenced var would be eliminated and the -X
// stamp silently dropped).
var sourceDigest = "agentics-src-unset"

// printDigestEnv, set to exactly "1", makes the binary print its provenance
// and exit 0 before any dispatch: the source digest on the first line, the Go
// toolchain that built it on the second. The exact match is deliberate: a
// stray empty or "0" value in a real session must never hijack a hook
// invocation.
//
// The toolchain line needs no -X stamp — the linker records the version
// itself, and runtime.Version() reads back what it wrote. The freshness check
// reads the same value out of each committed binary's build info rather than
// out of this print, so it can audit the three platforms it cannot execute.
const printDigestEnv = "AGX_SOURCE_DIGEST"

// toolchainPrefix labels the second provenance line. It is a human affordance
// only: nothing parses this output.
const toolchainPrefix = "built-by "

type hookDependencies struct {
	flow      func([]string, string, func(string) string, func() time.Time, io.Writer, io.Writer) int
	lookup    func(string) (hookspec.Handler, bool)
	dispatch  func(string, string, []string) (string, *provider.Rendered)
	normalize func(string, string, []byte) (hookspec.Event, *provider.Rendered)
	render    func(string, string, hookspec.Outcome, []byte) provider.Rendered
}

func productionHookDependencies() hookDependencies {
	return hookDependencies{
		flow:      flow.Run,
		lookup:    hooks.Lookup,
		dispatch:  provider.Dispatch,
		normalize: provider.Normalize,
		render:    provider.Render,
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is main's production core. It constructs a fresh dependency value for
// every invocation so tests and simultaneous callers never share mutable seams.
func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) (exit int) {
	return runWithDependencies(productionHookDependencies(), argv, stdin, stdout, stderr)
}

// runWithDependencies is the testable entry core. Argv validation,
// provenance, help, flow and the usage errors stay here; every stage after
// surface.Parse is handed to internal/checkrun.
func runWithDependencies(deps hookDependencies, argv []string, stdin io.Reader, stdout, stderr io.Writer) (exit int) {
	// checkrun.Answer catches and renders every fault once the surface is
	// known, so a panic that reaches this point has no surface to render
	// through and is reported as a usage error.
	defer func() {
		if r := recover(); r != nil {
			exit = usageError(stderr, fmt.Sprintf("agx: internal error: %v\n%s", r, debug.Stack()))
		}
	}()

	if len(argv) > 0 && argv[0] == "--startup-hint" {
		if len(argv) == 3 {
			writeStartupHint(argv[1], argv[2], stdin, stdout)
		}
		return 0
	}
	if os.Getenv(printDigestEnv) == "1" {
		fmt.Fprintln(stdout, sourceDigest)
		fmt.Fprintln(stdout, toolchainPrefix+runtime.Version())
		return 0
	}
	if len(argv) > 0 && argv[0] == "flow" {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "flow: working directory: %v\n", err)
			return 1
		}
		return deps.flow(argv[1:], dir, os.Getenv, time.Now, stdout, stderr)
	}
	// Help matches only a lone argument, so "help x" still fails provider
	// validation as it did before the command existed.
	if len(argv) == 1 && (argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h") {
		fmt.Fprint(stdout, commandList)
		return 0
	}

	if len(argv) < 2 {
		return usageError(stderr, fmt.Sprintf(
			"missing argument: expected a provider and a hook, got %d argument(s)", len(argv)))
	}

	requestedSurface, name, args := argv[0], argv[1], argv[2:]

	id, known := surface.Parse(requestedSurface)
	if !known {
		var roster []string
		for _, s := range surface.All() {
			roster = append(roster, string(s))
		}
		return usageError(stderr, fmt.Sprintf(
			"unknown provider %q (known: %s)", requestedSurface, strings.Join(roster, ", ")))
	}

	stages := checkrun.Stages{
		Lookup:    deps.lookup,
		Dispatch:  deps.dispatch,
		Normalize: deps.normalize,
		Render:    deps.render,
	}
	rendered, answered := checkrun.Answer(stages, id, name, args, stdin)
	if !answered {
		return usageError(stderr, fmt.Sprintf(
			"unknown hook %q (known: %s)", name, strings.Join(hooks.Names(), ", ")))
	}
	return emit(stdout, stderr, rendered)
}

// emit writes one rendered response and returns its exit code.
//
// Provider rendering owns response exit codes, including recovered adapter
// faults. Provenance output and usage failures bypass rendering with fixed
// non-deny statuses.
func emit(stdout, stderr io.Writer, rendered provider.Rendered) int {
	if len(rendered.Stdout) > 0 {
		_, _ = stdout.Write(rendered.Stdout)
	}
	if len(rendered.Stderr) > 0 {
		_, _ = stderr.Write(rendered.Stderr)
	}
	return rendered.Exit
}

// usageError reports a bad invocation. It always returns ExitUsage, never
// ExitDeny: an argv typo must not be mistaken for a guardrail block.
//
// It bypasses the surface deliberately. The provider is not yet known to be
// valid on this path, and a usage error is the binary's own complaint rather
// than a hook result the surface has an opinion about.
func usageError(stderr io.Writer, msg string) int {
	fmt.Fprintf(stderr, "agx: %s\n%s\n", msg, usage)
	return hookspec.ExitUsage
}
