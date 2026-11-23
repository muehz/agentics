# Subagent-driven development: worked example

This run shows `auto` on an approved plan: Step 0 and Step 1, front-loaded Stage 1, one quality review per review group,
and the integration review at each group boundary. The tiers (Deep, Standard, Light) are surface-agnostic task
classifications; each dispatch resolves its tier with the [model ladder](../../routing/references/model-ladders.md).
Rows are shown as `phase | event | target | status | why | evidence | next`. Each `ts` and `dispatched` time comes from
`date -u +%Y-%m-%dT%H:%M:%SZ` run at that moment.

## Step 0 and Step 1

The plan is `docs/specs/2026-11/feature-install-hook/plan.md`, written by `decompose` with `**Result:** pass` and an
independent plan review. It has four tasks. Wave 1 is Task 1 and Task 2, independent by the checklist. Wave 2 is Task 3,
which depends on Task 1, and Task 4, which depends on no wave-1 task. Wave 1 completes the skeleton path. The user typed
the plan path after `/agentics:execute`.

```text
[agx flow status --folder docs/specs/2026-11/feature-install-hook]
  approvals: plan.md approved 2026-11-12 current
  orders: 1 recorded, 0 unrecorded        unaccounted: none        (no mismatch: line)
[git status --porcelain → empty. No pending verify row: a fresh run.]

You: Running docs/specs/2026-11/feature-install-hook/plan.md in auto: 4 tasks in 2 waves. You named no close-out
     steps, so I'll ask before anything leaves this machine.

[Sub-step 1: no **Coverage:** blocking items. The register holds one order: "1. No new dependencies."]
[Sub-step 2: the plan is already in the repository; nothing to persist.]
[Sub-step 3: verification preflight. Task 1: `bash tests/install-hook.test.sh` resolves; Tasks 2-4: `npm test -- <path>`
 resolves and its selection reaches each task's files. No gaps.]
[Sub-step 5: waves as above. Review groups wave-1 and wave-2, no override. The only group edge runs wave-2 -> wave-1,
 so the graph is acyclic and the groups are not independent: no final review.]
[Sub-step 6: on branch feat/install-hook already. The approval is current, so no approve row.]
  execute | enter | run | ok | auto chosen: no mode named | mode=auto branch=feat/install-hook | execute dispatch stage 1
[Sub-step 8: the plan is trusted and no task carries a **Packet review:** mark. Tasks 1 and 3 state Tier Deep, so
 Stage 1 runs for them. Tasks 2 and 4 state Tier Standard, so Stage 1 is skipped for them once each passes the
 self-containment check. Two packet reviewers go out in one batch, at Deep.]
[Sub-step 9, preview: wave 1 (2 tasks), wave 2 (2 tasks); groups wave-1, wave-2, no cycle; Stage 1 dispatched for
 task-1 and task-3, skipped for task-2 and task-4 (unmarked, Standard, self-contained); 8 dispatches in all;
 selection: Standard implementer = the driver, gpt-6.2-astra/high; Deep reviewer = gpt-6.2-astra/high at the top clamp,
 kept separate and disclosed. This host does not expose actual bindings.]
```

## Wave 1

```text
Packet reviewer (task-3): READY
  execute | review | task-3 | ok | packet cleared | kind=packet dispatched=2026-11-14T09:00:04Z round=1 blocked=95 | execute continue wave 1
Packet reviewer (task-1): REVISE
  - Important: the packet does not say whether the hook installs per user or system-wide
  - Important: no acceptance criterion covers an existing hook file
  execute | review | task-1 | failed | install scope and overwrite behavior unstated | kind=packet dispatched=2026-11-14T09:00:04Z round=1 blocked=140 | execute ask the install scope

[The plan does not decide the install scope, so the controller does not guess it.]
You: Gap: the plan does not say where the hook installs. Blocked task(s): task-1 and task-3, which depends on it.
     May proceed: task-2. Question: should the hook install per user (~/.config/...) or system-wide?
     Resolve: answer here.
User: Per user. And it should refuse to overwrite unless --force is passed.

[The answer adds an acceptance criterion to task-1, which changes what the task builds. It is not a divergence: hold
 task-1 and task-3, send the revision to decompose, and let task-2 proceed. decompose revises task-1 in plan.md, its
 plan review passes, and it asks for approval of the new text. The user says yes, so the plan approval binds the
 revised hash, and the run resumes at Step 0 on that approval.]
  decompose | approve | plan.md | ok | user approved the per-user install and --force revision | text=… | execute resume at step 0
[Re-dispatch the task-1 packet reviewer with the revised packet: its first re-review within budget.]
Packet reviewer (task-1): READY
  execute | review | task-1 | ok | refined packet cleared | kind=packet dispatched=2026-11-14T09:04:10Z round=2 blocked=0 | execute continue wave 1

User: For the reviewer role, use model=gpt-6-luna.
[A first clear reviewer pin, scope role. Every later reviewer record shows override and scope=gpt-6-luna (role).]

[Dispatch the task-1 (Deep) and task-2 (Standard) implementers in one batch.]
Implementer (task-2): DONE_WITH_CONCERNS — added verify and repair modes; concern: the progress interval is a literal.
[task-1 is still writing to this tree, so commit task-2's own files first, by name: commit 9a8b7c6.]
  execute | verify | task-2 | pending | task commit awaits the group review | commit=9a8b7c6 patch-id=7c1d2e3f40a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9 | execute verify task-2
[Then run task-2's own command: `npm test -- recovery` → 8 passed, exit 0.]
Implementer (task-1): DONE — install-hook command and tests; files scripts/install-hook.sh, scripts/common.sh, tests/install-hook.test.sh
[Commit task-1's files: commit 1a2b3c4, verify task-1 pending, then `bash tests/install-hook.test.sh` → 5 passed, exit 0.]

[The wave's last task has committed. Run the wave's integration verification and the Outcome proof now.]
[`npm test` → 22 passed, exit 0. `./bin/install-hook --dry-run` → prints the planned per-user path, the named result.]
  execute | verify | wave-1 | ok | wave 1 integration verification and Outcome proof passed | commit=1a2b3c4 | execute review group wave-1
[Divergence for wave 1: no entries. task-3 and task-4 keep their verdicts.]

[Dispatch one quality reviewer for group wave-1. task-1 ran at Deep, so the ladder would raise the reviewer to Max; the
 user's role pin wins, so it runs on gpt-6-luna. It gets the commit list
 "task-2: 9a8b7c6, task-1: 1a2b3c4", both verification outputs, both packets, and the register.]
Quality reviewer (wave-1): FINDINGS
  - task-2, important: extract the progress interval constant
  - task-2, important: no regression test covers a zero-item run
  - task-1: none
  execute | review | wave-1 | failed | two important findings on task-2 | kind=quality dispatched=2026-11-14T09:20:11Z round=1 blocked=140 | execute send the findings to task-2
  execute | verify | task-1 | ok | cleared by the group review | commit=1a2b3c4 patch-id=3f4e5d6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e | execute fix task-2
  execute | verify | task-2 | failed | progress interval was a literal; zero-item run untested | commit=9a8b7c6 patch-id=7c1d2e3f40a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9 | execute fix task-2
[task-2's ladder now reads 1: the same implementer gets both findings.]
Implementer (task-2): DONE — extracted PROGRESS_INTERVAL, added the zero-item test
[Commit the fix: 2d3e4f5, verify task-2 pending, `npm test -- recovery` → 9 passed. Continue the same reviewer with
 "task-2: 2d3e4f5" and the new output.]
Quality reviewer (wave-1): CLEAR
  execute | review | wave-1 | ok | both task-2 findings closed | kind=quality dispatched=2026-11-14T09:31:50Z round=2 blocked=95 | execute run the integration review
  execute | verify | task-2 | ok | constant extracted, zero-item test added | commit=2d3e4f5 patch-id=8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c | execute run the integration review
```

## The group boundary

```text
[Material-integration check: task-1 and task-2 both touch scripts/common.sh, so the aggregate reviewer runs.]
[Dispatch it (pinned to gpt-6-luna) with the commit list "task-2: 9a8b7c6, task-1: 1a2b3c4, task-2: 2d3e4f5", the
 quality verdicts, the integration output, and the Outcome proof.]
[At the same time task-4 is released: its only edges point at no group under review and no divergence touches it.
 task-3 depends on task-1, in the group under review, so it waits.]
Aggregate reviewer (wave-1): FINDINGS
  - task-2, important: repair mode calls a helper task-1 renamed in scripts/common.sh; the seam breaks under --force
  execute | review | wave-1 | failed | task-2 calls a helper task-1 renamed | kind=integration dispatched=2026-11-14T09:36:02Z round=1 blocked=0 | execute send the finding to task-2
Implementer (task-2): DONE
[Commit the fix: 6f7a8b9, verify task-2 pending; re-run `npm test` → 22 passed. Continue the same aggregate reviewer with
 "task-2: 6f7a8b9" only, so task-4's in-flight work is not in it.]
Aggregate reviewer (wave-1): CLEAR
  execute | review | wave-1 | ok | renamed helper finding closed | kind=integration dispatched=2026-11-14T09:44:30Z round=2 blocked=0 | execute continue wave 2
  execute | verify | task-2 | ok | repair mode calls the renamed helper | commit=6f7a8b9 patch-id=4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d | execute continue wave 2
  execute | verify | group-wave-1 | ok | integration review of wave-1 clear | commit=6f7a8b9 | execute continue wave 2

Boundary report: group wave-1 shipped; verification and Outcome proof shown; task-2 fixed twice, its ladder at 1;
no divergence entries. Continuing.
```

## Wave 2

```text
User: The session driver is now gpt-6-luna/high.
[task-4 keeps its recorded binding. task-3 is resolved again: Standard = gpt-6-luna/high. The reviewer pin stays.]
[Release task-3: its prerequisite group has cleared both reviews.]
[task-3 and task-4 each commit (3a4b5c6, 5e6f7a8), get pending rows, and pass their own commands. Wave verification:
 `npm test` → 31 passed.]
  execute | verify | wave-2 | ok | wave 2 integration verification passed | commit=5e6f7a8 | execute review group wave-2
Quality reviewer (wave-2): CLEAR
  execute | review | wave-2 | ok | task-3 and task-4 cleared | kind=quality dispatched=2026-11-14T10:02:15Z round=1 blocked=60 | execute check the integration review
  execute | verify | task-3 | ok | docs page cleared | commit=3a4b5c6 patch-id=9d8e7f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e | execute check the integration review
  execute | verify | task-4 | ok | opt-in flag cleared | commit=5e6f7a8 patch-id=1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c | execute check the integration review
[Material-integration check: task-3 adds docs/hooks.md, which names the flags task-1 shipped; each flag matches
 `./bin/install-hook --help`. task-4 adds a flag that nothing outside its task reads. No relationship is introduced or
 altered, none is uncertain, the group joins one other group, holds no `yes` packet, has no override and no open stop.]
  execute | waive | aggregate-review | ok | wave-2: docs name task-1's flags as shipped; task-4's flag has no reader | none | execute record the group verdict
  execute | verify | group-wave-2 | ok | quality verdict and integration verification; integration review skipped | commit=5e6f7a8 | gate close out the branch

You: All tasks are done and verified — ready to close this branch out with a completion check using `gate`?
```

## A trusted plan with a marked task

```text
[A second plan, also trusted: `decompose`, Result pass, an independent plan review, and approval current.]
[Its task-1 fixes a typo in the CLI's --help text: Tier Light, **Packet review:** no.]
[Stage 1 is skipped under row 4 of the dispatch rule. The self-containment check passes: every path, criterion, and
 command is in the packet. The skip comes from the mark and the trusted plan; the check only confirms the packet can
 stand alone.]
[Dispatch the implementer (Light). DONE. Commit, pending row, run the command.]
[Every task of the group is Light and marked no, so the quality reviewer runs at Standard. CLEAR → verify task-1 ok.]
[Integration check: a help string has no readers in other groups. Skip with the evidence in a waive row, then the
 group's verify row.]
```
