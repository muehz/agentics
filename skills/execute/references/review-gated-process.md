# Review-gated process: visual flow

These diagrams draw the rules of `SKILL.md` Step 2. When a diagram and the text differ, the text wins.

## Per task

Stage 1 is front-loaded: Step 1 sub-step 8 dispatches every required packet reviewer for every wave, in batches within
the surface's limit. The wait at the top of this diagram is for that task's own verdict, not a fresh round trip at the
start of its wave. Every failure in this diagram counts once on the task's one failure ladder
([two-strike-stop.md](two-strike-stop.md)): two failures bring one fresh implementer, and four stop the task.

```mermaid
flowchart TD
    A[Read the plan and extract the task packet] --> A2{Stage 1 dispatch rule: skip?}
    A2 -->|no| B[Stage 1 packet reviewer, dispatched at Step 1 with every other required review]
    A2 -->|yes| A3{Packet self-contained? Checked in session}
    A3 -->|fails| B
    A3 -->|passes| E
    B --> C{READY?}
    C -->|REVISE: one failure| D[Refine the packet]
    D --> B
    C -->|READY| E[Dispatch the implementer when its wave opens]
    E --> F{Implementer status}
    F -->|NEEDS_CONTEXT; the second on one packet is a failure| G[Answer in an updated packet]
    G --> E
    F -->|BLOCKED| H[Change scope, tools, or dependencies]
    H --> E
    F -->|RED, bug-fix red leg| R[Confirm the failure reason; red commit, or the no-commit record when a hook rejects it]
    R --> E
    F -->|DONE or DONE_WITH_CONCERNS| I0[Stage and commit the task's files; write verify pending; run its verification command]
    I0 --> V{Verification passes?}
    V -->|no: one failure| K[Write the failed verify row; return the output to the implementer]
    K --> E
    V -->|yes| L[Wait for the review group's quality review; a dependent in the same group may start now]
    K -->|fourth failure| STOP[Block the task, report, and ask the user]
```

## Per review group

A review group is a slice of a sliced plan, a wave of an unsliced plan, or the tasks that share a `**Review group:**`
name. Its quality review starts once every task of the group that is not stopped has a task commit that passed the
controller's verification. A stopped task does not hold the group; the review covers the tasks that committed and names
the stopped task, whose dependents stay held.

The reviewer receives the list of the group's commits, each labelled with its task, never a range and never the live
tree. Findings name a task. Each goes to that task's implementer, and the same reviewer is continued with the fix
commits. A fix that does not close its finding counts once on the ladder of the task the finding names. The reviewer's
verdict writes each task commit's final `verify` row.

After the quality review clears, the controller hands the aggregate reviewer the group's task and fix commits, and
decides the integration review under [aggregate-wave-review.md](aggregate-wave-review.md). A group never skips while a
stop is open in it, or while the plan's proportionality line reads `override by user`.

At each wave's end the controller runs the wave's integration verification, plus the Outcome proof on the skeleton wave,
and records `verify wave-<N>`. It then computes the wave's divergence before it releases any later task. A packet the
wave's diff touched goes back through Stage 1, within its re-review budget.

```mermaid
flowchart TD
    W[Every task of the group that is not stopped has a task commit and passed verification] --> Q[One fresh quality reviewer: the commit list, verification output, and packets]
    Q --> QC{CLEAR?}
    QC -->|FINDINGS| QF[Each finding to the named task's implementer; continue the same reviewer with the fix commits]
    QF --> QC
    QC -->|CLEAR| DV[Hand the aggregate reviewer the group's task and fix commits]
    DV --> S{Relationship introduced or altered, uncertain, joins two or more groups, a yes packet, an override, or an open stop?}
    S -->|none, evidence recorded| SK[waive aggregate-review row, with the evidence]
    SK --> R
    S -->|any| AG[Fresh aggregate reviewer: quality verdicts, blocked tasks, other-group context]
    AG --> F{CLEAR?}
    F -->|FINDINGS| X[Each finding to the named task's implementer; hold only that task's dependents]
    X --> DR[Continue the same reviewer with the fix commits and re-run verification]
    DR --> F
    F -->|CLEAR| R[verify group row, boundary report, and state.tsv rows]
    R --> N[Release dependent tasks in other groups; dependents of a blocked task stay held]
```
