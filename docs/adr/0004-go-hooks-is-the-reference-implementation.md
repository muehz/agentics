# The Go hook engine is the reference implementation

> **Distribution update (2026-11-23).** The rebuild schedule below records the earlier committed-binary policy. The
> current source-only workflow rebuilds local outputs and tracks the runtime manifest.
> [Maintainer build](../../hooks/README.md#maintainer-build)

> **Amended (2026-11-20).** "Source", for the source digest stamped into each committed binary and for the rebuild rule
> below, means the source the binaries are built from: `hooks/go/go.mod`, the list `hooks/go/digest-exclude`, and every
> non-test `.go` file under `hooks/go` outside `bin/` and outside the directories that list names. The listed
> directories hold `hooktool`, the conformance suite, and other code that ships in no hook binary, so an edit confined
> to them, or to `_test.go` files, leaves the committed binaries current; an edit to any other non-test `.go` file, to
> `go.mod`, or to the list itself changes the digest. `TestDigestExcludeMatchesClosure` pins the list: it must equal, in
> both directions, the module's packages outside the import closure of `cmd/hook` and `cmd/launcher` over every contract
> target. The stamp format is unchanged.

`hooks/go` was written to reproduce a prior implementation line for line, and two earlier records —
[ADR-0001](0001-shared-hook-modules-keep-measured-differences.md) and
[ADR-0002](0002-unverifiable-reductions-are-behavior-changes.md) — froze it there: every measured difference was kept,
and any reduction that could not be proven against the retired oracle was declined. That oracle is gone. What remains is
a working Go binary that runs on every tool call, passes the conformance suite, and is the only implementation anyone
can point at.

This record makes that binary the reference, and says what its behavior is specified by instead.

## Decision

The Go implementation under `hooks/go` is the reference implementation of the Agentics hooks. Its behavior is specified
by two things, and only two:

1. **The hosts' published hook protocols.** Each surface doc under `docs/surfaces/` carries a **Hook protocol** section
   stating what that host does with a command hook's exit code, stdout, and stderr, citing the vendor page pinned in the
   surface ledger. What Agentics emits for each outcome is the **Provider Output Matrix** in each guardian section of
   `hooks/README.md`. Those two sections, read together, are the observable contract.
2. **The Go test suite.** The behavior corpus at `hooks/go/cmd/hook` records the binary's `(stdout, stderr, exit)` for
   enumerated inputs and is regenerated only by `go test -update`; the semantic table in
   `hooks/go/internal/commandguard` checks one core's outcome kind and reason plus diagnostic and log presence. A
   deliberate change is a reviewed diff whose reason is stated in the pull request.

Anything not derivable from those two is implementation detail. A behavior inherited from the prior implementation that
no host depends on may be changed as an ordinary change — it needs a corpus diff and a reason, not a decision record.

## The bar

"Idiomatic" is a checklist, not a taste. Code under `hooks/go` meets it when:

- `gofmt -l`, `go vet`, and `staticcheck` are clean. `staticcheck` runs through `go run` so `hooks/go/go.mod` declares
  no dependency.
- Failures are `error` values. A function never returns a message `string` with `""` meaning success. Where a caller
  branches on the failure, the error is a sentinel or a typed error it can test with `errors.Is` or `errors.As`.
- Nothing reimplements the standard library. `slices`, `maps`, `strings`, and `encoding/json` are used where they apply;
  a hand-written `contains` or `sortedKeys` is a defect.
- A helper shared by more than one package lives in one package. Two functions with the same name and different behavior
  in sibling packages is a defect.
- A repeated shape is a named type. An anonymous struct written twice in one expression is a defect.
- A `switch` over a closed set names every member and carries a `default` that fails loudly. Behavior that depends on a
  case being absent is a defect.
- A comment states why, in the present tense. A comment that restates the code below it, narrates the code's history, or
  cites a file outside the current tree is a defect. Every package has a package comment; every exported identifier has
  a doc comment.
- Tests assert at the seam a host meets — exit code, stdout, stderr, log-record fields — and never on the shape of an
  internal function.

Issues #359 through #370 are the worked examples of this bar: each names one violation in the tree at the time this
record was written, and each closes when its package is rewritten to the list above.

## When a host relies on a corrected behavior

A change made under this record may turn out to break a real host: the Live check in a guardian section of the hooks
README fails after a corpus diff shipped. The procedure is fixed and short. Revert that one behavior. Add the case to
the behavior corpus. Add the host's requirement to the surface doc's Hook protocol section with the vendor citation. Say
so in the pull request. The change is a contract discovery, and it never reopens this record.

## Rebuilding the committed binaries

`hooks/bin/` holds six committed binaries, and the conformance suite proves they were built from the checked-in source.
Two branches that both rebuild them conflict on every merge. So: the binaries are rebuilt only by the pull request being
merged, after it has been rebased on `main`, and never on a feature branch mid-flight. A branch that changes the source
the binaries are built from and is not the one being merged does not commit binaries. This is what lets sibling packages
be rewritten in parallel.

A stack of dependent pull requests is the one exception. Its layers merge one at a time and the freshness gate runs on
every head, so each layer that changes the source the binaries are built from carries its own rebuild and its own
`TestBaselineCalculatorModel` size pins, and a change low in the stack rebuilds every layer above it on the restack
(`make -B`, because a replayed binary commit carries a fresh mtime). The conflicts that rule above avoids are paid
deliberately, once per restack, inside the stack.

## Vocabulary

Comments under `hooks/go` and prose in `hooks/README.md` describe the current implementation and nothing else. Decision
records under `docs/adr/` and `docs/RENAMES.md` are history by design. This record is an example.

## Considered options

**Keep the measured baseline and only scrub comments.** Declined. A behavior with no present-tense reason is worse with
its citation removed than with it kept; the comments were the only explanation the warts had. Scrubbing without
re-deriving the contract would leave code nobody can justify.

**Resurrect the prior implementation as an oracle.** Declined. ADR-0002 already calls this a research project, and it
would specify the Go binary by a thing that was retired because the Go binary replaced it.

## Supersedes

- ADR-0001 in full. Measured per-core and per-surface differences are no longer preserved for their own sake; each is
  kept because a host requires it, kept because it is harmless, or corrected.
- ADR-0002 for `hooks/go` only. Within the hook engine, the Go test suite is the oracle and a reduction is verified
  against it. ADR-0002 still governs the TypeScript tools under `.github/tools`, the release scripts included.

## Consequences

A reviewer of a `hooks/go` change asks two questions instead of one: does the corpus diff, if any, state its reason, and
does the code meet the bar above. "This is what the old implementation did" is no longer an answer to either. The
classification of every inherited behavior is tracked in one issue while the rewrite runs and is discarded when it
finishes; the durable artifacts are the two contract homes named in the Decision and the two corpora.
