# Triage mode procedures

Detailed bare-number auto-detection and escape-logging procedures referenced by [SKILL.md](../SKILL.md).

## Bare-number auto-detection

For `/agentics:triage <N>` with a bare number and no `pr` / `issue` keyword, run `gh pr view <N> --json number` against
the current repo. Capture **both** the exit code and the stderr of that `gh` call. Never discard stderr with
`2>/dev/null`, because most rows below read it.

Match the exit code first. Fall through to a stderr string only when the exit code is `1`. `gh` uses `1` for every
failure it has no dedicated code for. The strings below come from measurements against `gh` 2.97.0 (`gh --version`). If
a much newer `gh` no longer matches these strings, re-probe with `gh help exit-codes` and with the failing command
itself. After the re-probe, correct this table.

| Result of `gh pr view <N>`                                                                                                          | What it means                     | Do this                                                                                                                           |
| ----------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| The shell reports `command not found`. The exact text is `gh: command not found` or `command not found: gh`, and it varies by shell | The `gh` binary is absent         | Tell the user `gh` is not installed. Point at `https://cli.github.com`. Do not fall back to a raw REST call. Do not invent a mode |
| Exit `0`                                                                                                                            | `<N>` is a PR                     | Dispatch **PR mode** with `<N>`                                                                                                   |
| Exit `4`                                                                                                                            | `gh` holds no credentials at all  | Tell the user to run `gh auth login`. Stop. Do not try the issue endpoint                                                         |
| Exit `2`                                                                                                                            | The call was cancelled            | Report that. Stop. Do not retry. Do not guess a mode                                                                              |
| Exit `1`, stderr has `Could not resolve to a PullRequest`                                                                           | `<N>` is not a PR                 | Run `gh issue view <N> --json number`. Exit `0` dispatches **issue mode**. Any other result reads as its own row in this table    |
| Exit `1`, stderr has `Could not resolve to an issue or pull request`                                                                | `<N>` is neither                  | Tell the user the number resolves to no PR and no issue in this repo. Do not invent a mode                                        |
| Exit `1`, stderr has `Bad credentials` or `HTTP 401`                                                                                | The token is present but invalid  | Say the token is bad or expired, which is a different problem from exit `4`. Point at `gh auth login`. Stop                       |
| Exit `1`, stderr has `error connecting to` or `check your internet connection`                                                      | `gh` cannot reach the host        | Say the machine is offline or the host is unreachable. Point at `https://githubstatus.com`. Stop. Do not retry the other endpoint |
| Any other exit code, or exit `1` with no row above matching                                                                         | An error this table does not name | Report the raw stderr verbatim. Stop. Never guess a mode from an error you cannot classify                                        |

The rows are exclusive: a missing binary has no exit code from `gh`, and each other row names one exit code and, for
exit `1`, one stderr string. PRs and issues share one number namespace per repo, so a bare `<N>` is never ambiguous.
Keep the failure meanings distinct: number-not-found, no credentials, bad token, unreachable host, and `gh` absent. If
you collapse any two of them, you send the user after the wrong problem.

---

## Escape logging details

Triage records what its push carries in the flow of the PR's work item, with `agx flow log`, as
[flow-state.md](../../execute/references/flow-commands.md#commands) shows, including its launcher-path fallback when
bare `agx` is not found. The record contract, the closed sets, and the exit codes are in
[flow-state.md](../../execute/references/flow-state.md). Rows are written only in `/agentics:triage pr` mode, only when
the push is authorized, and only in the merge worktree before the push, so they travel with it
([Phase 5.4](lifecycle-publish.md#54--verify-the-combined-result)).

1. Find the flow with `agx flow where`. `where` reads the current branch, and triage's own `triage/pr-…` branches match
   no `bind` row, so run it in a checkout that is on the PR head branch (`{PR_BRANCH}`), such as the caller's checkout
   when it is on that branch, and keep the folder it prints. Exit 3 (no flow), 4 (more than one flow matched), and 5 (a
   small fix is in progress) are write-nothing cases. So is a run with no checkout of the PR head branch, and a flow
   whose last row is `done`, because the flow is closed. In each, name the escape in the rollup's Details instead.
2. Write one `commit` row per commit the run integrated: target `triage`, status `ok`, evidence `auto:<sha>` with the
   commit's SHA in the merge worktree. A commit the run did not integrate gets no row.
3. Write an `escape` row only when the run fixes a Critical or Important finding, by the shared severity of
   [severity.md](../../routing/references/contracts/severity.md). A finding that is Minor, declined, or only drafted
   gets no row. The row has phase `gate`, status `failed`, and this evidence:
   `found-by=<gate|ci|copilot|person> severity=<critical|important> review=<plan|packet|quality|integration>`. Map the
   source of the fixed finding to `found-by`:
    - Checkmarx, Code Security, Code Quality, and failed checks: `ci`.
    - Copilot and Bot Review: `copilot`.
    - A human reviewer's finding the developer accepted: `person`.
4. Set the `escape` row's `target` to the task, group, or `final` whose commit introduced the defect, taken from the
   flow's `verify` rows (`agx flow status --folder <folder>`, and `state.tsv`). Set `review=` to the review that cleared
   it: `quality` when the group's quality review cleared it, `integration` when a group or final integration review (or
   its recorded skip) cleared it, and `packet` or `plan` when only a packet or plan review covered it. The helper
   rejects an unknown `found-by`, a missing `review=`, and a status other than `failed` with exit code 2.
5. Copy the `next` of the row before it, printed by `agx flow status --folder <folder>`, so no row changes the phase a
   reader selects.
6. Write no `bind` row. Triage never binds a branch to a flow.
7. Log with `--folder <folder>` in the merge worktree. Stage `state.tsv` with its own
   `git --literal-pathspecs add -- "$state_path"` using the flow path result and the `state.tsv` filename as data there
   and commit it on the merge branch. A run that pushes nothing (fork code, a blocked push, or no authorization) writes
   no row that reaches the PR, and the rollup names any escape instead.
