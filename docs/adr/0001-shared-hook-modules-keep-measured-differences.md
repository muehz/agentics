# Shared hook modules keep measured per-core and per-surface differences

> **Superseded by [ADR-0004](0004-go-hooks-is-the-reference-implementation.md).** The Go hook engine is now the
> reference implementation, and the measured differences this record preserved are each kept because a host requires
> them, kept because they are harmless, or corrected as ordinary changes under that record. This file keeps its original
> wording for the reasoning at the time.

> **Amended (1.35.0).** The `prettier` hook was retired; formatting moved to a pull-request workflow. This record keeps
> its original wording, including the `prettier` examples, because it documents the reasoning at the time the decision
> was made. The principle is unchanged and still governs the two remaining cores. Read every `prettier` mention below as
> historical.

The Go hook engine under `hooks/` is a port of a retired bash implementation, and its diagnostics, exit codes, and
field-extraction rules are compared byte for byte by the conformance suite (`go/tools/internal/conformance/hooks`). When
we pull duplicated code into a shared module, we keep every difference that a measurement pinned, even where the
duplicate copies look like an obvious cleanup.

## Three instances this decision was made for

**The neutral-event reader offers no file path.** `contract.ReadEvent` gives callers `tool.input` and nothing more. The
three cores find a file path in three different ways, all measured: `guardian` falls back through `file_path`, `path`,
`value.file_path`, `value.path`; `prettier` reads `tool.input.file_path` alone; `scanner` reads none. A shared
`FilePath` field would invite all three to use it and would silently change `prettier`.

**The surface argv table keeps empty rows.** `claude-code` performs no argv validation for `tool-guardian` and
`prettier`, while `copilot` and `codex` require an exact `--mode` value for both. The table makes those empty rows
visible, and filling them in "for consistency" would add checks bash never had.

**A shared decode forces work onto a core that never did it.** `rejectNonFinite` in
`go/hooks/internal/contract/contract.go` returns early for any integer literal, so `EventJSON` accepts an
arbitrary-precision integer inside `tool.input`. A later plain `encoding/json` decode of that same document into
`map[string]any` fails, because the value does not fit a `float64`. The three cores measure that failure three different
ways, verified on a 401-digit integer:

| Core       | Outcome                                                    |
| ---------- | ---------------------------------------------------------- |
| `guardian` | `tool-guardian could not extract normalized input`, exit 1 |
| `prettier` | `prettier could not read normalized input`, exit 1         |
| `scanner`  | pass, exit 0 — it never decodes `tool.input`               |

So `contract.ReadEvent` must not decode `tool.input` eagerly, and must not decode it with `UseNumber`: eager decoding
makes `scanner` start failing, and `UseNumber` makes `guardian` and `prettier` stop failing. The decode stays
caller-invoked, with plain `encoding/json` semantics.

## Consequences

A reviewer who proposes unifying either of these is proposing a behavior change, not a refactor. Any such change needs a
conformance-suite run and a deliberate decision to move the measured baseline — not a cleanup commit.
