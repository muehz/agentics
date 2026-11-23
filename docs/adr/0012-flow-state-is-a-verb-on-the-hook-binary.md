# Flow state is a verb on the hook binary

## Decision

The flow commands live in `hooks/go/internal/flow`. The installed `agx` runtime accepts `flow` as its first argument and
runs `log`, `status`, `where`, `bind`, or the other commands in the current
[flow-state contract](../../skills/execute/references/flow-state.md). The implementation uses the Go standard library
and Git. The command returns before reading hook stdin or resolving a provider. Exit 2 marks an invalid cell and a
recovered flow panic exits 1. `AGX_SOURCE_DIGEST=1` prints the source digest and toolchain before dispatch.

[0017](0017-flow-state-v2.md) defines the lookup, approval hash, row, and scoping rules. The append-only `state.tsv`
format, Git attributes for approval hashing, and preservation of CR bytes in artifact text apply to the Go
implementation. [0020](0020-environment-and-platform.md) defines what to report when the host denies a flow invocation.
[0023](0023-agx-is-the-agentics-command-line-tool.md) defines the launchers and runtime provisioning.

## Publication and concurrency

Initialization writes and closes a complete header in an exclusively created temporary file, then publishes it with
`os.Link` at the absent `state.tsv` path. When hardlinks work, the header is visible before a process can append. A
competing initializer keeps the already published file.

Other link failures fall back to exclusive creation. The publisher checks that the prepared source still exists as a
regular file, creates `state.tsv` subject to the host umask, writes the retained header bytes, and checks close. A
write-only umask permits initialization and later writes because the publisher keeps the header bytes in memory.

The fallback exposes the file before its header is complete, so an append can arrive in that window and be overwritten.
Header-before-row ordering and preservation of every concurrent initial row require the hardlink path. Each ordinary row
is one append write ending in LF, and concurrent appends to an initialized file retain that behavior.

## Why one runtime

A single Go implementation keeps the behavior and in-process tests together on each supported platform. Skills and the
runtime are distributed in the same plugin, so the caller and its flow contract ship together. A separate platform
binary or script fallback would duplicate shipping and verification. The frozen script in
`hooks/go/internal/shellwords/testdata/flow.sh` is scanner fixture data with its upstream attribution.
