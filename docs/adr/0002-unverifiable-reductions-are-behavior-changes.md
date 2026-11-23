# After cutover, an unverifiable reduction is a behavior change

> **Superseded by [ADR-0004](0004-go-hooks-is-the-reference-implementation.md) for `go/hooks`.** Within the hook engine,
> the Go test suite is the oracle and a reduction is verified against it. This record still governs the TypeScript tools
> under `.github/tools`, the release scripts included. This file keeps its original wording for the reasoning at the
> time.

The bash implementation the Go hook engine ports was deleted at `b8bb573`. Two of the three cores lost their oracle with
it. `tool-guardian` kept 287 captured goldens (`go/hooks/internal/guardian/testdata/bash-oracle-goldens.json`), so its
behavior is still comparable against the measured baseline. `secrets-scanner` captured nothing. Its behavior table,
`TestRunMatchesBashCore`, had skipped since cutover because the core it shelled out to no longer existed; this change
removed it, along with `runBashCore`, `requireBashCore`, and `TestPinnedPatternsMatchLiveBashSource`. A skipping test is
not evidence, so deleting it removes no coverage — but it also leaves the scanner with no oracle of any kind.

So a change that _looks_ like a pure reduction can no longer always be proven to be one. Where that proof is
unavailable, we treat the reduction as a behavior change and decline it, rather than reasoning our way to "this is
obviously equivalent" and shipping it.

## What follows from this

**New scanner tests pin the Go port, not bash.** Any test written for `secrets-scanner` after cutover records what the
Go implementation does today. If the port carries a defect, such a test freezes it. Those files must say so in their
header, so a later reader does not mistake them for measured truth. This is accepted deliberately: reconstructing an
oracle by resurrecting the retired bash is a research project, not a refactor.

**`joinScanContinuations` stays as it is.** `go/hooks/internal/guardian/guardian.go` splits its scan text on newlines,
and that text is built from a compact JSON document, so the slice holds exactly one element unless a _tool name_
contains a newline. From that, the function's per-line loop reduces to a single `strings.ReplaceAll`. The one-element
claim is reasoning, not measurement, and the golden bank cannot be extended to test it — those goldens came from a bash
core that is gone. The reduction is therefore permanently unverifiable and must not be made.

## Consequences

This is not a licence to leave everything alone. Reductions that the compiler proves, or that the surviving 287-case
guardian bank covers, remain ordinary refactors.

**The guardian bank does not cover everything in guardian.** Measured: zero of the 287 cases are a Bash tool call whose
`tool_input` carries no `command` key, so `invocations` never takes its `strict = true` branch and the strict arm of
`segmentShellLine` is unmeasured. A change to how the tokenizer mode is threaded can therefore leave all 287 goldens
green. Tests written for that branch pin the Go port, not bash, and carry the same header the scanner tests do. The rule
binds only where the lost oracle is the _only_ thing that could have shown a difference.

Restoring an oracle for `secrets-scanner` would lift this constraint for that core. That is worthwhile work and it is
not in scope for a refactor.
