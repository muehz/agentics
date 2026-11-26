---
name: zealot
description: "Use when code is cluttered with comments that merely narrate it, or when tests seem to pass without proving anything. Remove low-value commentary and use mutation to identify assertions that survive the very defects they should catch."
version: "4.1.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[comments|decomment|tests] [files, directory or diff range; tests also takes all]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [comments, tests]
    mode-aliases:
        decomment: comments
---

# Zealot

Hunt one kind of low-value artifact with a maximally biased finder on a short leash. Then audit everything it reports. Each mode pairs one finder charter with this skill's audit, as the [adversarial pair pattern](../forge/references/quality/adversarial-pair-pattern.md) describes.

Authoring agents defend their own work, so the finder and the reviewer each run in a fresh context. The code in scope may be code this session wrote; the session that runs the workflow never judges it. When a mode's dispatch is refused or missing, its declared fallback runs the pass in this session and labels the run `self-read (not independent)`, as [surface-dispatch.md](../routing/references/surface-dispatch.md#any-other-surface-generic-fallback) requires.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Read the rows from the top; the first match wins. The **Kind** column says what an explicit invocation does with the row, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it: a `cannot` row forwards by that protocol. When the model loaded this skill on its own, every row forwards.

| Input signal                                                                                                     | Use this instead                                                   | Kind   | Why                                                                                     |
| ---------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------ | --------------------------------------------------------------------------------------- |
| Reviewing a diff for bugs, security, or logic errors                                                             | `review`                                                           | cannot | Zealot judges comments and whether tests prove anything, not code correctness.          |
| A failing test or a bug whose cause is unknown, including a bug a comment happens to describe                    | `diagnose`                                                         | cannot | Zealot audits passing tests and code comments. It finds no root cause.                  |
| Writing a new test, a replacement test, or a fix that makes a red test pass                                      | `tdd`                                                              | cannot | Tests mode never writes, fixes, or edits a test.                                        |
| Deleting dead code, unused files, or feature flags                                                               | `routing`, as an ordinary mutating change                          | cannot | Comments mode fixes only the code a flagged comment points at, inside the scope.        |
| Only a mutation score, kill ratio, or coverage percentage, with no tests to audit, or installing a mutation tool | No skill: Zealot has no workflow for it, and the session may do it | cannot | Tests mode reports what happened to named mutants, never a score, and installs nothing. |
| Writing, restructuring, or refreshing documentation                                                              | `docs`                                                             | cannot | Doc files are documentation, not code comments.                                         |
| Triaging pull request review comments or bot findings                                                            | `triage`                                                           | cannot | Review comments on a pull request are conversation, not code comments.                  |
| Otherwise                                                                                                        | This skill runs                                                    | —      | [Which mode?](#which-mode) picks the mode.                                              |

A scoped tests-mode request that also asks for a mutation score runs tests mode. Say in one line that Zealot reports no score.

## When an agent invokes Zealot

An agent may load Zealot without `/agentics:zealot`, inside four bounds:

1. Load it only when the user's current request matches a mode: it asks to strip or clean up low-value comments, or asks whether tests prove anything.
2. Never load it unprompted during other work, even when that work surfaces narration comments or weak tests.
3. Never chain it from another skill. Other skills may only suggest it. A route or preflight forward of the user's own request (`routing`, or `tdd` for a test audit the user asked for) is not a chain.
4. Never pick the `all` scope unless the user typed `all` or asked for the whole project in words.

However Zealot was loaded, run the selected mode's workflow whole. A result built from plain copies instead of mutant worktrees, without the finder and the reviewer (or their labelled fallback), or without the mode's report is a finder claim, not a finding. Never report it as a finding.

## Step 0: Preflight

**Read nothing else first.** Before Step 0 preflight and mode selection finish, read no mode workflow or charter file
(neither `comments/` nor `tests/`), resolve no scope, and make zero tool calls on an ambiguous or non-matching request.
Check the request against [Don't use when](#dont-use-when) before loading any mode reference, on every invocation. A
matching row forwards by [forwarding.md](../routing/references/forwarding.md) before any scope is resolved. Otherwise
select the mode below.

## Which mode?

Read the rows in order; the first match wins.

| Order | The request                                                                                                                                          | Mode                                                                                                                                                                                             |
| ----- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | starts with `comments` or `decomment`                                                                                                                | [Comments](#agenticszealot-comments)                                                                                                                                                             |
| 2     | starts with `tests`                                                                                                                                  | [Tests](#agenticszealot-tests)                                                                                                                                                                   |
| 3     | has no mode word, and it or the conversation talks of comments, narration, commented-out code, or workarounds, and not of test proof                 | Comments                                                                                                                                                                                         |
| 4     | has no mode word, and it or the conversation talks of tests that prove nothing, tautological or vacuous assertions, or mutation, and not of comments | Tests                                                                                                                                                                                            |
| 5     | Otherwise: no mode word, and the conversation supports neither mode or both                                                                          | Ask one line before any tool call, naming both modes: "Which mode: comments (strip low-value comments) or tests (find mutation-confirmed tautological tests)?" Then stop until the user answers. |

Announce an inferred mode in one line. The `all` scope applies to tests mode only. Comments mode never widens on it and asks for files or a diff range instead.

## /agentics:zealot comments

Read [references/comments/workflow.md](references/comments/workflow.md) before acting, and follow its steps in order. It resolves the scope, spawns the Comment Zealot finder from [references/comments/comment-zealot.md](references/comments/comment-zealot.md) (embedding the charter verbatim), has a fresh read-only Deep reviewer audit the finder's report (passing both the Reject list and keep rule), shows the code fixes, constraint encodings, and exact Step 5 verification command in the confirmation batch, applies confirmed items, runs baseline/post-fix verification, deletes saved copies, and reports.

## /agentics:zealot tests

Read [references/tests/workflow.md](references/tests/workflow.md) before acting, and follow its steps in order. It resolves the scope (stating nonempty balanced package groups before dispatch when `all` applies), prepares disposable mutant worktrees outside the repository, spawns the Test Zealot finder from [references/tests/test-zealot.md](references/tests/test-zealot.md), has a fresh Deep reviewer prove each candidate by mutation, audits the records, restores and diffs the caller snapshot, and emits the Step 7 handoff naming the next step for confirmed remove-or-replace work. It never deletes or edits a test, and it never reports a mutation score, kill ratio, or coverage percentage.

## References

- [`comments/workflow.md`](references/comments/workflow.md) — comments mode: scope, finder, audit, confirmed fixes and encodings, report.
- [`comments/comment-zealot.md`](references/comments/comment-zealot.md) — finder charter and exception list for comment audits.
- [`tests/workflow.md`](references/tests/workflow.md) — tests mode: scope, mutant worktrees, finder, mutation reviewer, audit, handoff, report.
- [`tests/test-zealot.md`](references/tests/test-zealot.md) — finder charter and exception list for tautological-test audits.
- [`adversarial-pair-pattern.md`](../forge/references/quality/adversarial-pair-pattern.md) — the shared finder-and-auditor pattern.
- [`surface-dispatch.md`](../routing/references/surface-dispatch.md) — dispatch permission, concurrency, the fallback label, and charter embedding.
- [`forwarding.md`](../routing/references/forwarding.md) — the route-away protocol.
- [`gates.md`](../routing/references/contracts/gates.md) — the safety gate and process gate classes the workflows use.
