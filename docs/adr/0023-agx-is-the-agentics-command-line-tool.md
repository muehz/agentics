# agx is the Agentics command-line tool

## Decision

The Go program in `hooks/go/cmd/hook` runs the Agentics guards (`agx <provider> <hook>`) and the flow-state commands
(`agx flow <command>`). `agx help` lists the available commands. The flow arguments, output, and exit codes follow
[0012](0012-flow-state-is-a-verb-on-the-hook-binary.md) and [0017](0017-flow-state-v2.md). `hooktool` remains a separate
maintainer command.

## Launchers and runtime

- The repository tracks `bin/agx`, a POSIX `sh` script with LF line endings and executable mode.
- The [verified plugin archive](../guide/01-setup.md) also carries `bin/agx.exe`, a native Windows AMD64 launcher.
  Windows on ARM runs it under x64 emulation. It provisions and starts the native ARM64 runtime on that host.
- Both launchers locate `hooks/run-hook.cmd` from their own install directory and call its provision-only entry point.
  The dispatcher selects the host runtime and verifies it against the installed `hooks/SHA256SUMS`. It can use a
  verified package copy or cache entry and fetches a missing runtime unless `AGX_NO_FETCH` is set.

The launchers accept a single, verified absolute runtime path from provisioning. They pass original arguments and
streams to the runtime and propagate its exit status. A launcher reports provisioning or process-start failure on stderr
and exits 126. The native Windows launcher keeps argument handling out of a batch-file parser because `flow` accepts
free text that must reach the binary as data.

## Installed launcher path

Skills start with bare `agx flow ...` when the host exposes the installed `bin/` on `PATH`. On successful startup
provisioning, the startup hook can emit the verified absolute launcher path. Use it on hosts that omit `bin/` from
`PATH`. [Flow state](../../skills/execute/references/flow-state.md) defines the fallback and unavailable cases.

Provider hook configurations invoke the dispatcher by absolute plugin path. Guard launches must use the installed
runtime and manifest even when another `agx` is on `PATH`.
