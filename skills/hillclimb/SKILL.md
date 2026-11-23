---
name: hillclimb
description: "Use when the goal is to improve a measurable result through repeated experiments. Establish a stable baseline and evaluation method, change one candidate at a time, and keep only improvements supported by measurements."
version: "2.0.1"
user-invocable: true
disable-model-invocation: false
argument-hint: "[metric, direction, and target, e.g. 'p95 checkout latency under 200ms']"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
---

# Hillclimb

Drive one measured metric toward an explicit target through repeated, verified attempts. The core discipline is **one change, one measurement, keep or revert.** Never stack untested changes. Never claim a win from code inspection.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Read the rows in order; the first match wins. Each row is marked with its kind, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it. A `cannot` row forwards by that protocol. A `better elsewhere` row, under an explicit invocation of this skill, gets one line naming the better route, and then this skill runs. When the model loaded this skill on its own, every row forwards.

| Order | Input signal                                                         | Use this instead | Kind             | Why                                                                            |
| ----- | -------------------------------------------------------------------- | ---------------- | ---------------- | ------------------------------------------------------------------------------ |
| 1     | A defect, error, or sudden regression whose cause is the deliverable | `diagnose`       | cannot           | That is a search for one cause. This skill produces measured changes, not one. |
| 2     | Executing an approved, decomposed task plan                          | `execute`        | cannot           | Plan execution follows a dependency graph, not a metric.                       |
| 3     | Verifying a finished change before calling it done                   | `gate`           | cannot           | Verification is a checkpoint, not an improvement loop.                         |
| 4     | Reviewing a diff or pull request for defects                         | `review`         | cannot           | Review judges changes; this skill produces them.                               |
| 5     | A one-off performance complaint with no target or loop ("X is slow") | `diagnose`       | better elsewhere | Its performance branch measures, fixes once, and verifies, with no stop rule.  |
| 6     | Otherwise                                                            | This skill runs  | —                | A metric, a direction, a stop predicate, and as many attempts as it takes.     |

Under `/agentics:hillclimb`, row 5 runs this skill: the first deliverable is agreement on a metric ([Step 0](#step-0-preflight)).

## Gates

Each gate is stated once here, with its class from [gates.md](../routing/references/contracts/gates.md).

**Safety gates.** Each is held in the fixed shape of gates.md, said once: what is held, the harm it prevents, and what the user can do instead. A repeated demand gets a one-line pointer back to that alternative.

| Held                                                                                                                                                       | Harm it prevents                                                                                 | What the user can do instead                                                                                                                             |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A reported number, delta, or met clause comes from a log row under the current measurement contract. A target is reported as met only when a row shows it. | A claim with no evidence from this session: the next person builds on a result nobody measured.  | Measure it now, report it as unmeasured, or change the target, which the log records as a new row.                                                       |
| `decision.tsv` is append-only. No row is edited, deleted, or truncated.                                                                                    | The log is the only record of rejected attempts and unmet results. An edit erases them for good. | Append a row that supersedes the wrong one, or a `contract` row for a changed target.                                                                    |
| No attempt changes a path that holds the user's own uncommitted changes.                                                                                   | A revert restores the files an attempt touched, so it would take the user's edits with it.       | Commit the changes, set them aside through the git skill's `stash` mode, or name the paths the climb must leave untouched ([Step 0](#step-0-preflight)). |

**Process gates.** The attempt floor, the sensitivity proof, and the reproduction requirement are process gates. When the user declines one, name its cost once in one sentence. On the next reply that still declines, in any wording, do what they asked, and append one `waive` row to `decision.tsv` that names the gate. No reason is asked for. The report names every waiver. The log row is the waiver's record, as the [waiver order](../routing/references/contracts/gates.md#process-gate) sets.

**Not gates.** The metric, the target, and the decision to stop are the user's. A changed target is recorded as a `contract` row and reported as a change. A user who stops the run gets the report, with each unmet clause named. A stop before an agreed attempt floor is reached also declines the floor, so the floor's cost is named once first.

## Step 0: Preflight

1. **Route.** Check the request against [Don't use when](#dont-use-when).
2. **Metric.** If the request names a metric, a direction, and a target, use them. Otherwise, the first deliverable is agreement on them. During Step 1's exploration pass, propose a metric, a direction, and a target from the codebase, with evidence: a benchmark or a reproducible complaint. Ask it as one decision question in the [question format](../routing/references/contracts/question-format.md), with the proposal as the recommended answer. If the pass finds no evidence and the user names none, there is nothing to measure. Say so, and say what would start the climb: the operation, the metric, the direction, and the target, or the slow case. Never start the loop on an unmeasured goal. In an [unattended run](../routing/references/contracts/unattended.md), the dispatcher answers; when the dispatch prompt names no metric, return the proposal as an assumption to confirm and stop before Step 2.
3. **Tree.** Run `git status --porcelain`. Untracked files the climb does not touch are fine. Any other uncommitted change is the user's work, and the clean-tree safety gate holds: ask once whether to commit it, set it aside through the [git skill](../git/SKILL.md)'s `stash` mode, or leave named paths untouched. When the user names paths, no attempt changes them, and a hypothesis that needs one waits for the user's answer. Never stash, restore, or discard it yourself.
4. **Branch.** Say in one line which branch each accepted win is committed on. On the default branch or a detached HEAD, create a feature branch before the first commit, as [chain-phases.md](../routing/references/chain-phases.md#feature-branch-before-the-first-commit) says, and announce its name. No win is committed on the default branch.
5. **Principles.** Apply the [principles check](../routing/references/contracts/principles-check.md) when you have a baseline artifact to inspect.

## Step 1: Ground the workload, then fix the contract

Open the decision log first, as [Step 3](#step-3-open-the-decision-log) describes, so that every row from here on, a waiver included, has a place to go. Then explore the target code and architecture **before** you choose the measurement. Name the realistic workload dimensions that can move the result: data size, history, state, or concurrency. Select a case that reproduces the user's complaint. A climb on an unreproduced complaint optimizes the wrong case, so fix the reproduction first. If no reproduction can be built, say what would make one reproducible. The reproduction is a process gate: when the user declines it, climb on the closest workload and label every reported number with that workload.

Before any change, fix these in writing and log them as the first `contract` row:

- **One metric** and the **direction** that counts as better.
- **A checkable stop predicate**: the target, plus an attempt floor if the user wants one. The floor keeps a lucky early win from ending the run. If the user gave numbers, use them. If the user gave only a target, recommend a floor, for example "at least 10 iterations", in a decision question. The user may decline it, and then the predicate is the target alone.

## Step 2: Build the harness, prove it, freeze it

1. **Build** a measurement harness: one repeatable command that emits the metric. If the repository has a `verify-<app>` skill, built by `/agentics:scaffold feedback-loops`, its Launch and Drive sections can serve as the workload driver. Wrap the driven scenario in the metric command. Do not invent a second way to exercise the app.
2. **Prove its sensitivity** on contrasting realistic workloads. The target case must reproduce the symptom, and easier cases must separate from it. If the measurement tool takes no parameters and cannot produce an easier case, extend it with a minimal workload knob, such as a concurrency flag or a dataset-size flag. If the measurement cannot tell the cases apart, revise the workload or the metric: an insensitive measurement makes every later verdict noise. The proof is a process gate.
3. **Classify the metric.** Run the harness twice on the unchanged tree. Read the rows in order; the first match wins.

    | Order | The two runs                                                                                                             | N                                | Noise floor                                                                                                                       |
    | ----- | ------------------------------------------------------------------------------------------------------------------------ | -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
    | 1     | Give identical values, and the metric is computed rather than timed or sampled, as a bundle size or a test pass count is | 1, no stabilization              | None: any change in the better direction clears it.                                                                               |
    | 2     | Otherwise                                                                                                                | Stabilize, as the next item says | Twice the baseline batch's standard deviation, or 1% of its median, whichever is larger. A guard the user agrees replaces the 1%. |

4. **Stabilize N** (row 2 only). Start with N = 5. For each candidate N, collect two fresh, independent N-run batches, reusing no sample. Compare their medians against the run-to-run standard deviation pooled across the 2N observations. If the median difference exceeds that deviation, add 5 to N and repeat with two new batches. Freeze the first N that passes. N stops at 30. If 30 does not pass, the harness is too noisy to climb on: say so, and ask one decision question. The options are to reduce the noise (a quieter machine, pinned CPU, a longer warmup, a larger workload), which is the recommended answer, or to keep N = 30 with a widened noise floor: the larger of row 2's floor, taken from the baseline, and the largest batch-median difference seen. An unattended run takes the widened floor and says so.
5. **Record the baseline.** Discard the stabilization batches. Collect one fresh N-run baseline batch, and log its median and within-batch standard deviation as a `baseline` row, with N and the noise floor in the note. Run the regression gate, the tests that must keep passing, as [Running it](../routing/references/repo-conventions.md#running-it) allows, and record that it is green. A repository with no test suite names the closest executable check that proves behavior, such as a workload run whose output is compared, and uses it as the gate. Without a baseline there is no delta to claim, so the loop does not start.
6. **State the cost.** Before the loop, say what one iteration costs as an estimate: two N-run batches and one regression-gate run, with the time the baseline batch took.

The harness and N are now the **measurement contract**. Numbers from different contracts are not comparable. When the user changes the harness or N, or a change becomes unavoidable, append a `contract` row, measure a new baseline, and append its `baseline` row. Compare only rows after the latest `baseline` row with each other.

## Step 3: Open the decision log

At the start of Step 1, before its first row, create `decision.tsv` under `.hillclimb/<run-name>/` at the repository root, and report the path. The file stays untracked so that it survives reverts. Ignore the folder through the repository's local exclude file, as [session-storage.md](../routing/references/session-storage.md#worktrees-and-ignore-rules) says; never edit `.gitignore` for it. Use these tab-separated columns:

```text
id	hypothesis	change	before	after	delta	tests	verdict	note
```

| `verdict`  | The row records                                                                                   |
| ---------- | ------------------------------------------------------------------------------------------------- |
| `contract` | The metric, direction, predicate, harness, or N, as first fixed or as changed, and who changed it |
| `baseline` | A baseline batch under the latest contract: its median, standard deviation, N, and noise floor    |
| `kept`     | An attempt that cleared the noise floor with the regression gate green, and its commit            |
| `reverted` | Any other attempt, and the path of its saved diff                                                 |
| `waive`    | A process gate the user declined, by name                                                         |

A row that corrects an earlier one carries the corrected verdict, and its note says `supersedes <id>`. **Read the log before each attempt** so that the search accumulates and does not circle. A hypothesis the log already rejected is not new.

The log is this skill's own record, not the flow `state.tsv`, so these cell rules are stated here:

- Strip tabs and newlines from each cell, then trim surrounding whitespace.
- Then prefix a single quote to any cell that starts with `=`, `+`, `-`, `@`, or a single quote, so a spreadsheet reads it as text. Trim first: a leading tab hides the `=` from the check, and a spreadsheet trims it and evaluates the formula.
- Write rows with the file-editing tool. Never interpolate cell text, which comes from code and tool output, into a shell command.

## Step 4: Loop, one hypothesis per iteration

Each hypothesis names a specific mechanism from Step 1's model: "defer X off the boot path because it blocks first paint", never "try memoizing something".

Per iteration:

1. Read `decision.tsv`.
2. Implement exactly one change. A subagent with a tight scope can make the edit, so your context stays on the metric and the diff. Subagents run at the Standard tier of [model-ladders.md](../routing/references/model-ladders.md).
3. Measure before and after with the frozen harness, one fresh N-run batch each, and take each batch's median. Run the regression gate as [Running it](../routing/references/repo-conventions.md#running-it) allows.
4. **Keep the change only when the delta clears the noise floor and the gate stays green.** State the floor once when the loop starts. "Looks faster" is not a result. Otherwise revert the change in full, and a tweak that "might help" does not ride along.
5. To revert, save the attempt's diff as `attempt-<id>.diff` beside the log, then restore exactly the paths the attempt changed and delete the files it created. The tree was clean in those paths when the attempt began, so the revert discards only the attempt, and the diff file keeps it.
6. Commit each kept change on the branch from Step 0, under the [commit contract](../git/references/commit.md#the-commit-contract), naming only the files the attempt changed.
7. Append the row either way.

Each iteration ends in its measurement before the next begins, because two stacked, unmeasured changes destroy attribution for both. If changes were stacked unmeasured, revert to the last measured state and redo them one at a time.

**Parallel hypotheses.** Several genuinely independent hypotheses may go to parallel subagents, each in its own worktree from the [git skill](../git/SKILL.md)'s `worktrees` mode. Hypotheses that touch the same files run one after another.

- Before the fan-out, state the dispatch count: one subagent and one worktree per hypothesis. Batch under the concurrency limit in [surface-dispatch.md](../routing/references/surface-dispatch.md#concurrency-and-overlap).
- Subagents implement only. Measure each candidate yourself, one at a time on one machine, because concurrent runs share the machine and distort each other's numbers.
- Keep at most one candidate per round: the best one that clears the floor. Each other candidate is applied again on top of the new state and measured as a new attempt with its own row.
- Remove each worktree once its candidate is kept or dropped, as [session-storage.md](../routing/references/session-storage.md#worktrees-and-ignore-rules) says.

## Step 5: Push past the first plateau

Three `reverted` rows in a row mean pivot, not stop. Before you conclude that the hill is climbed, pivot in any of these ways:

- Change the hypothesis category.
- Combine near-misses.
- Re-read the source.
- Try something more radical.

Correctness and simplicity outrank the metric. Revert a win that breaks behavior. Keep a simplification that holds the number.

## Step 6: Stop honestly

Stop for one of three reasons, and say which:

| Reason                                                               | What the report says                                                                           |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Every clause of the predicate is met                                 | Each clause with the log value that meets it.                                                  |
| The remaining ideas are marginal against their cost, by your measure | Each unmet clause with its log value, and the ideas you judged marginal.                       |
| The user stops the run                                               | Each unmet clause with its log value. A floor stopped short of is a waiver, logged as `waive`. |

Never relax the predicate yourself to declare victory. Do not quit while cheap, untried hypotheses remain; if you are stuck, say so instead of spinning. Name every unmet clause on its own, the metric target and the attempt floor separately.

The user may change the target at any time, and their word is enough. Append a `contract` row with the old and new values, and report both: the revised target as met or unmet, and the original target as changed. A report never calls the original target met when only a revised one is. The safety gates above hold for the report and the log.

## Step 7: Deliver

Report:

- the metric and the target, with any change to the target,
- baseline to final, with the percent delta,
- iterations run, kept against reverted,
- each kept win on one line, with its commit,
- every waived gate,
- the `decision.tsv` path,
- the best idea to try next.

Every number comes from a log row under the current contract. Never use an estimate or a run the log does not carry.

The kept commits sit on the branch in the order they landed, so the climb reads top to bottom. Offer [`gate`](../gate/SKILL.md) for the branch, by [forwarding.md](../routing/references/forwarding.md#b-offer-to-forward-one-yesno-question): one yes or no question, or none when the user named the step up front. `gate` verifies the branch and proposes one destination in plain text, such as a draft pull request, which it opens with the outbound checks its body path carries. A climb has no work folder, so `gate` runs it as a no-flow run.

## References

- [forwarding.md](../routing/references/forwarding.md) is the route-away protocol the `cannot` rows and the delivery offer use.
- [gates.md](../routing/references/contracts/gates.md), [question-format.md](../routing/references/contracts/question-format.md), and [unattended.md](../routing/references/contracts/unattended.md) hold the gate classes, the decision-question format, and what an unattended run is.
- [chain-phases.md](../routing/references/chain-phases.md#feature-branch-before-the-first-commit) holds the feature-branch rule before the first commit.
- [commit.md](../git/references/commit.md#the-commit-contract) holds the commit contract.
- [session-storage.md](../routing/references/session-storage.md#worktrees-and-ignore-rules) holds the ignore and worktree-removal rules.
- [model-ladders.md](../routing/references/model-ladders.md) and [surface-dispatch.md](../routing/references/surface-dispatch.md) hold the subagent tier and the concurrency limit.
