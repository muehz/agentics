# 04. Build

Implement the plan. One skill runs approved plans at scale. The rest are disciplines and utilities you invoke by
describing how you want the work done.

## Run the approved plan

```text
The plan is approved. Execute it.
```

Why this phrasing works: "approved" plus "execute it" is the explicit go-ahead
[`execute`](../../skills/execute/SKILL.md) requires. It runs in `auto` mode: it reads the plan's dependency graph and
dispatches isolated subagents, in parallel waves where independence verifies and in one-task waves where it doesn't.
Only those subagents write code. Each review group gets one model quality review, group boundaries get an integration
review when its triggers fire, independent groups get a final review, and the run continues without stopping, with a
report at each wave boundary. Before the first dispatch, a plan that arrived as a paste, a session file, or an inline
list is written to the same work folder as `plan.md`, with a provenance header, so the run has a committed record. At
each wave boundary the report names any divergence between what shipped and the approved text, in a task's file scope,
acceptance criteria, or dependency edges, or in the spec or the intent, and appends it to the `## Divergences` section
of the artifact it amends, the intent, the spec, or the plan, instead of editing the approved text. The run asks for no
new approval. A divergence the user authorizes against a repository rule goes in the plan's `## Divergences` section
too, already reconciled, so `gate` does not list it. Each worker prompt that follows carries the open entries as
amendments to the approved text. `gate` folds the unambiguous open entries into the artifacts without asking. Every run,
in every mode, also records its decisions in the work folder's `state.tsv` through the `flow` helper — each task's
review verdict with its commit and patch-id, reverts, divergences, and waived reviews, each with its reason — and the
gate checks the branch's commits against those rows. A plan you changed after approving it reads as stale in
`flow status`, and the run stops until you approve it again. To build the plan with the agent in this session instead,
say "Execute it in pair mode." The agent implements in the session, the same model reviews run, and the run stops at the
end of each wave for your review. Only you can choose pair mode. For one task with competing solutions, ask for arena
mode: "run three candidates at this task and graft the best parts."

## Run independent slices in parallel sessions

```text
Execute the approved plan. Its lanes run in separate sessions; give me the launch lines.
```

Why this phrasing works: [`decompose`](../../skills/decompose/SKILL.md) marks a slice `**Lane:** yes` when its
dependencies are met and its files overlap no other open lane. For a plan with lanes, `execute` writes one launch packet
per lane to the work folder's ignored `temp/lanes/`: the slice verbatim, the standing orders, the verification commands,
a branch name carrying the slice ID (`feat/reports-s2`), and the instruction to run that slice in `auto` and stop at a
draft pull request. Each lane branch starts at the plan commit, which `execute` makes before writing the packets. It
prints the worktree command to run first and a launch line per host (a `claude --bg` session or agent view, Codex,
Copilot), and runs no lane itself. You start each lane from inside its worktree, and the home session runs `gate branch`
on each lane branch as it becomes ready. Lanes are independent, so they merge in any order, each as its own draft pull
request; gate's stack hand-off applies only to a later slice that depends on a lane. Lanes append to the same
`state.tsv`, so `execute` checks for a union merge rule and asks whether to add `<work-root>/**/state.tsv merge=union`
to `.gitattributes`. Each lane's session runs `flow bind` on its own branch, which writes a `bind` row once, so every
lane branch finds the same flow. GitHub's merge button ignores union drivers: when a lane's pull request conflicts only
in `state.tsv`, merge the base branch into the lane branch locally and push.

## Test-drive the change

```text
Use TDD: failing test first, then make it pass.
```

Why this phrasing works: [`tdd`](../../skills/tdd/SKILL.md) is the method `execute` applies to each task in an approved
plan, and to a small fix. In `auto` the dispatched implementer follows it; in `pair` you and the agent follow it
together. A TDD ask with no approved plan behind it starts the chain at intent, or takes the small-fix path when the
change meets every small-fix signal. For a bug fix, the failing test is committed on its own before the fix, and the
gate reports any later change to that test. Don't split one change into "write the tests" and "now implement" as
separate requests. The discipline owns the loop end to end within each task.

## Answer questions by running code

```text
Before we pick a data model, prototype both options and let the results decide.
```

Why this phrasing works: "let the results decide" routes the open question to
[`prototype`](../../skills/prototype/SKILL.md) instead of a debate. Anything observable, like behavior, timing, output
shape, or whether an approach works at all, is answered faster by a throwaway probe than by a discussion. Reserve
opinions for questions no experiment can settle.

## Commit as you go

```text
Commit this as a series of commits, split by concern.
```

Why this phrasing works: "split by concern" invokes [`git`](../../skills/git/SKILL.md)'s batching judgment, not just
`git commit -am`. The same skill owns worktrees for parallel work ("put this in an isolated worktree"), and its
`resolve` mode handles merge conflicts.

Two more builders match on their own vocabulary. New repo or missing governance files:
[`scaffold`](../../skills/scaffold/SKILL.md). "This module is hard to extend, design the seams":
[`deepen`](../../skills/deepen/SKILL.md). Agentics ships no UX skill. For UX flows, wireframes, interaction design, or
production frontend, use an outside UI/UX skill; Agentics records the approved problem and solution.

**Pitfall:** don't hand `execute` a bare goal. Its preflight requires a decomposed task list. "Execute: make the app
faster" has no dependency graph to read, so it routes back through `decompose` and you've spent a round trip. The chain
from [03. Plan](03-plan.md) exists so that by the time you say "execute", the only thing left to decide is the dispatch.

Next: [05. Verify](05-verify.md)
