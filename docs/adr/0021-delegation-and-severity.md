# Every dispatch follows one contract, and every finding uses one severity scale

This record covers delegation and severity: what a dispatch carries and returns, how a tier is assigned, what a skill
does on a surface that cannot dispatch, how parallel and repeated dispatch is bounded, and which severity and verdict
words producers and consumers share. It answers the delegation and severity problems in the skill audit,
`docs/designs/skill-audit-2026-11/consolidated-issues.md`.

Three neighbouring questions are decided elsewhere. Who runs the final review of a laned plan belongs to the flow-state
record (0017). How many reviews a delivery path gets belongs to 0013 and 0017. How an eval bank asserts tier behaviour
belongs to 0022. The security scoring rubric and the check-weakening rules are not decided here.

## Rules

### A template and its controller state the same contract

- **Answers:** C11.1
- **Rule:** The list in `skills/routing/SKILL.md` under "What every dispatch carries" binds every skill that dispatches,
  not only `execute`, and it gains two items: the write boundary (read-only, or the paths the subagent may write) and
  the return contract (the status or verdict words and every required field). A controller requires of a return only
  what its template asked for, and every word a template can return has a branch in the controller. A template never
  tells a subagent to invoke another skill's workflow for setup; the controller does that before it dispatches. The
  known mismatches are fixed to this rule as follows.
    - The `review` post reviewers' templates carry an `exists_at_head` slot.
    - The post pipeline's test-coverage reviewer is dispatched only when repository guidance requires test coverage,
      which is the one case where scoring keeps its output.
    - The post pipeline scores on one scale, the five rubric anchors, and its default threshold is the anchor 75.
    - The implementer template names `tdd` as the method for test-first work, and the bug-fix red leg returns its own
      status word, `RED`, which the controller answers by committing the red commit.
    - A `forge` eval runner receives its scenarios in a template slot filled from the pinned bank and designs scenarios
      only when no bank exists.
    - The `explain` investigator prompt states the read-only rule.
    - The `decompose` plan-reviewer prompt names its inputs, forbids edits, and returns the two artifact verdicts in the
      severity rule below.
    - A `triage` specialist receives a worktree whose baseline the orchestrator already verified.

    (verified: skills/routing/SKILL.md:295, skills/review/references/post/post-pipeline.md:80,
    skills/review/references/post/post-scoring.md:33, skills/execute/references/implementer-prompt.md:57,
    skills/forge/references/evals/skill-eval-runner-prompt.md:10, skills/explain/references/investigator-prompt.md:139,
    skills/decompose/references/plan-document-reviewer-prompt.md:69, skills/triage/references/subagent-contract.md:15,
    2026-11-11)

- **Default / Rejected:** One contract in `routing`, with each template as its instance; rejected: a contract per skill,
  because that is the current state and it is why standing orders reach `execute` subagents and not `triage`
  specialists. Threshold 75; rejected: keeping 80, because the rubric has no anchor between 75 and 100, so only an
  "Absolutely certain" finding can post.
- **Later work changes:** `skills/routing/SKILL.md`, `skills/review/references/post/post-pipeline.md`,
  `skills/review/references/post/post-scoring.md`, `skills/review/SKILL.md`,
  `skills/execute/references/implementer-prompt.md`, `skills/forge/references/evals/`,
  `skills/explain/references/investigator-prompt.md`, `skills/decompose/references/plan-document-reviewer-prompt.md`,
  `skills/triage/references/subagent-contract.md`

### The ladder alone resolves a tier, and Max is never a skill's default for an artifact review

- **Answers:** C11.2
- **Rule:** `skills/routing/references/model-ladders.md` is the only file that maps a tier to a model or an effort. A
  skill names one tier for each dispatch role it has, helpers included, and nothing else.
    - A reviewer of a packet, plan or spec runs at Deep. It runs at Max only on an explicit user selection. The Stage 1
      default of Max for a Deep-tagged task is removed.
    - A quality reviewer may still resolve to Max when the work it reviews was implemented at Deep, because that is the
      ladder's "next position above the actual implementer" rule.
    - A helper that looks up a fact, such as a pull request's draft state, is Light.
    - The `zealot` tests finder is Standard.
    - The ladder's control table lists only parameters of the dispatch tool. A command-line flag that starts a new
      session is not a per-dispatch control; where the dispatch tool has no model or effort parameter, the binding is
      reported as unbound.
    - The Cursor rows stay, and `skills/routing/references/surface-dispatch.md` gains a Cursor section that says no
      dispatch tool is verified, so a Cursor session with none uses the generic fallback.
    - `surface-dispatch.md` names the four current tiers in place of the retired judgment, workhorse and mechanical
      names.
    - No eval bank carries a copy of a ladder table. The copies in the `execute`, `decompose`, `recall` and `explain`
      banks are removed or rewritten to cite the ladder.

    (verified: skills/execute/references/spec-reviewer-prompt.md:16, skills/routing/references/model-ladders.md:228,
    skills/routing/references/model-ladders.md:200, skills/routing/references/surface-dispatch.md:143,
    skills/zealot/references/tests/workflow.md:101, evals/skills/execute.md:563, 2026-11-11)

- **Default / Rejected:** Deep for every artifact review; rejected: Max whenever the task is Deep-tagged, because Stage
  1 reviews a packet the driver wrote at Standard and no Deep work exists yet. Standard for the finder; rejected: Light,
  because the finder reads a call path and proposes a production mutation, and each false candidate costs a worktree and
  a Deep reviewer. Keep the Cursor rows; rejected: deleting them, because the owner confirmed those selectors and only
  the dispatch path is unverified.
- **Later work changes:** `skills/routing/references/model-ladders.md`, `skills/routing/references/surface-dispatch.md`,
  `skills/execute/references/spec-reviewer-prompt.md`, `skills/review/SKILL.md`,
  `skills/zealot/references/tests/workflow.md`, `evals/skills/execute.md`, `evals/skills/decompose.md`,
  `evals/skills/recall.md`, `evals/skills/explain.md`

### Each dispatching skill declares one fallback, and a self-read never counts as an independent review

- **Answers:** C11.3
- **Rule:** `skills/routing/references/surface-dispatch.md` holds one dispatch-permission rule for every skill, in place
  of the two named exceptions it has today.
    - On a surface that spawns only on a request, such as Codex, which spawns after a direct request or an applicable
      `AGENTS.md` or skill instruction, a user-typed invocation of a skill whose workflow dispatches is that request. A
      skill loaded any other way, including an `execute` that `routing` selected, asks once for permission to dispatch,
      and a yes is the request.
    - Every dispatching skill declares exactly one fallback for a refused or missing dispatch tool: a labelled
      in-session pass, or a stop that names what can still run. `review` and `forge` declare theirs; `execute` keeps its
      stop.
    - When a required independent review runs as a self-read, the record and the approval question both say
      `self-read (not independent)`. A later step that skips a check because an independent review ran does not skip it
      after a self-read, so `execute` runs Stage 1 on a plan whose plan review was a self-read.
    - Skill text uses Claude Code tool names. `routing` holds one translation table per supported surface. A surface
      with no table detects tools by presence and is never sent to another surface's table.

    (verified: skills/routing/references/surface-dispatch.md:113, skills/decompose/SKILL.md:524,
    skills/tdd/SKILL.md:135, 2026-11-11)

- **Default / Rejected:** Ask once on a non-typed load; rejected: treating any skill load as the user's request, because
  the surface's rule is an explicit request and `routing` chose the skill, not the user. Label the self-read and keep
  the later check; rejected: blocking the plan on a host that cannot dispatch, because record 0008 already accepts a
  labelled self-read for a spec.
- **Later work changes:** `skills/routing/references/surface-dispatch.md`, `skills/routing/references/copilot-tools.md`,
  `skills/decompose/SKILL.md`, `skills/execute/references/stage-1-dispatch-rule.md`, `skills/review/SKILL.md`,
  `skills/forge/SKILL.md`, `skills/tdd/SKILL.md`

### Parallel work is capped by the surface, serialised on overlap, and reviewed by commit list

- **Answers:** C11.4
- **Rule:** Three rules bound parallel dispatch.
    - A reviewer receives the list of commits it judges, each labelled with its task. The `first^..last` range form is
      removed, because in a shared worktree a range takes in other groups' commits.
    - No batch exceeds the surface's concurrency limit recorded in `skills/routing/references/surface-dispatch.md`,
      counted over every subagent still running. The rest queue. Stage 1 reviewers are batched under the same limit. A
      surface with no recorded limit uses 6, the lowest limit recorded.
    - Tasks that write the same files run one after another. Worktree isolation is for arena candidates and isolated
      packets; it does not make overlapping tasks parallel. The tool-presence row in `surface-dispatch.md` that says
      otherwise is corrected.

    (verified: skills/execute/references/code-quality-reviewer-prompt.md:56, skills/execute/SKILL.md:290,
    skills/execute/SKILL.md:316, skills/routing/references/surface-dispatch.md:20,
    skills/routing/references/surface-dispatch.md:48, skills/routing/references/surface-dispatch.md:122, 2026-11-11)

- **Default / Rejected:** Serialise overlapping tasks; rejected: isolate them in worktrees and run them in parallel,
  because the overlap returns as a merge conflict that no reviewer saw. A commit list; rejected: a range computed more
  carefully, because the worked example in `skills/execute/references/controller-example.md` already needs a separate
  capture step to keep one fix commit out of the next range.
- **Later work changes:** `skills/execute/SKILL.md`, `skills/execute/references/code-quality-reviewer-prompt.md`,
  `skills/execute/references/aggregate-reviewer-prompt.md`, `skills/execute/references/controller-example.md`,
  `skills/routing/references/surface-dispatch.md`

### A dispatch count is stated before it is spent, and per-item dispatch is batched

- **Answers:** C11.5
- **Rule:** A workflow that dispatches more than once states its full dispatch count before it starts, counting nested
  dispatches, judges, quality reviews and one baseline run per worktree. A loop that dispatches once per item batches
  the items instead, at most 20 to a dispatch. A subagent fans out only when its controller's count included that
  fan-out.
    - `zealot` comments sends every unverified keep of a run to one read-only Standard investigator per batch and no
      longer runs `explain` once per keep.
    - The full Security lane scores each domain reviewer's candidates in one scorer dispatch, not one per candidate.
    - A diff whose only Security trigger is a dependency manifest or lockfile gets the fast depth. An explicit security
      request and every other trigger keep the full depth.
    - The arena preview counts every dispatch and every baseline suite.
    - `forge` reflect's "5+ tool calls" trigger offers the reflection in one line and dispatches its reviewers only
      after a yes.

    This rule changes how many scorers run. It does not change how a security finding is scored.

    (verified: skills/zealot/references/comments/workflow.md:70, skills/review/references/axes/axes-mode.md:107,
    skills/execute/references/arena-mode.md:16, skills/forge/SKILL.md:290, 2026-11-11)

- **Default / Rejected:** Batch and state the count; rejected: a fixed ceiling on dispatches per run, because the right
  number depends on the plan's size and the user can judge a stated count. Fast depth for a lockfile-only trigger;
  rejected: no automatic Security run for it, because a dependency change is a real trigger and the fast scan covers it.
- **Later work changes:** `skills/zealot/references/comments/workflow.md`, `skills/review/references/axes/axes-mode.md`,
  `skills/review/references/security/security-axis.md`, `skills/execute/references/arena-mode.md`,
  `skills/forge/SKILL.md`

### Findings use four severities, and consumers read the tally

- **Answers:** C12.1
- **Rule:** Every finding that leaves the skill that produced it carries one of four severities, defined once in
  `skills/review/references/verdict-synthesis.md`. A producer may print its own scale beside the shared one. A consumer
  reads only the shared one.

    | Severity      | Meaning                                                                        | Effect                                 |
    | ------------- | ------------------------------------------------------------------------------ | -------------------------------------- |
    | Critical      | Data loss, a security compromise, or the changed behaviour broken for everyone | Blocks. Judgment alone never lowers it |
    | Important     | A defect in required behaviour, a broken contract, or a failed required check  | Blocks until fixed, accepted, deferred |
    | Minor         | A real defect or deviation that does not block                                 | Counted and listed                     |
    | Informational | Context                                                                        | Listed, never counted                  |

    The word Nit is retired in favour of Minor. Each producer maps as follows.

    | Producer                                                           | Critical   | Important          | Minor           | Informational |
    | ------------------------------------------------------------------ | ---------- | ------------------ | --------------- | ------------- |
    | Security lane                                                      | Critical   | High               | Medium, Low     | context       |
    | Standards lane                                                     |            | `HARD`             | `SOFT`          |               |
    | Spec lane (its status decides; it has no separate severity column) |            | `MISSING`, `WRONG` |                 | `ADDED`       |
    | Check-weakening, controller audit, red-commit check                |            | Important          | Minor           |               |
    | `execute` quality and aggregate reviewers                          | critical   | important          | minor           |               |
    | A source with `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, as in `triage`  | `CRITICAL` | `HIGH`             | `MEDIUM`, `LOW` |               |
    | CI, Copilot or a person, which state no severity                   | by meaning | by meaning         | by meaning      |               |

    For the last row, the skill that records the finding assigns the severity from the meanings above, so an `escape`
    row can always be written. The rest of the rule is this.
    - A review bucket says what the lead recommends; it does not rename the severity. A Standards `HARD` finding moved
      to Consider is still Important for `review` and for `gate`.
    - A Minor finding from the Security lane goes into the pull-request body where [0018](0018-trust-boundaries.md)
      allows it to be posted; where that record withholds it (an unfixed finding on a public repository, or an embargo)
      it goes to the user in the session. Other Minor findings stay in the report.
    - A blocking finding of a spec or plan review is the exception to "accepted, deferred" in the Important row: record
      0017 keeps it non-waivable, so it is closed only by a fix or a revision of the artifact.
    - The `gate` change record reads "no Critical or Important findings" in place of "no findings above LOW".
    - `escape` rows keep `severity=<critical|important>`.
    - `P1` to `P4` name only an `ops` incident priority. No other skill uses those labels, and the "plan challenge
      P1-P4" example in `skills/decompose/SKILL.md` is replaced with what `challenge` returned.
    - A consumer reads the open tally, never a verdict word. Verdicts are derived from it:

    | Producer                                     | No open Critical or Important | Open Important, each a bounded fix | Anything else   |
    | -------------------------------------------- | ----------------------------- | ---------------------------------- | --------------- |
    | Packet, plan and spec reviewers              | `READY`                       | `REVISE`                           | `REVISE`        |
    | Quality and aggregate reviewers              | `CLEAR`                       | `FINDINGS`                         | `FINDINGS`      |
    | `review` interactive                         | Approve                       | Approve with fixes                 | Request changes |
    | `review` axes (tallies per lane, no verdict) | none                          | none                               | none            |
    | `gate`                                       | READY                         | NOT READY                          | NOT READY       |

    A bounded fix changes neither the approach nor an interface, so a fix that changes an interface is Request changes.
    The plan reviewer's third verdict, "Major rework", is removed; a plan that needs it returns `REVISE` with a Critical
    finding. (verified: skills/review/references/verdict-synthesis.md:86,
    skills/gate/references/branch/branch-workflow.md:1271, skills/gate/references/change/change-record-template.md:81,
    skills/gate/SKILL.md:127, skills/review/references/axes/axes-mode.md:192,
    skills/execute/references/flow-state.md:81, skills/review/references/interactive/interactive-review.md:47,
    skills/review/references/check-weakening.md:105, skills/decompose/SKILL.md:635, 2026-11-11)

- **Default / Rejected:** Four severities with Security Medium as Minor and listed in the pull-request body; rejected:
  the current collapse of Security Medium to Nit, which keeps a real vulnerability out of the pull request, and mapping
  Medium to Important, which would put a question to the user for every Medium. Consumers read the tally; rejected:
  making axes mode print an aggregate verdict for `gate`, because axes mode declines one on the ground that it would
  claim more than any lane measured.
- **Later work changes:** `skills/review/references/verdict-synthesis.md`, `skills/review/SKILL.md`,
  `skills/review/references/check-weakening.md`, `skills/review/references/interactive/interactive-review.md`,
  `skills/gate/SKILL.md`, `skills/gate/references/branch/branch-workflow.md`,
  `skills/gate/references/change/change-record-template.md`, `skills/execute/SKILL.md`,
  `skills/execute/references/flow-state.md`, `skills/decompose/SKILL.md`,
  `skills/decompose/references/plan-document-reviewer-prompt.md`, `skills/triage/references/subagent-contract.md`,
  `evals/skills/review.md`, `evals/skills/gate.md`, `evals/skills/triage.md`

## Existing records

- 0002: consistent — it governs reductions in Go tooling and release scripts, which this record does not touch.
- 0008: amended — its cost note says the spec review runs on the most capable model rung every time, and the reviewer
  now runs at Deep unless the user selects Max; its labelled self-read on a host that cannot dispatch is kept and
  extended to the plan review.
- 0009: consistent — the small fix still gets its dispatched quality review; who writes it is decided in 0013.
- 0012: consistent — `escape` rows keep the `severity=<critical|important>` evidence that the flow code validates.

## What this costs

**More findings post and more reach the pull request.** A threshold of 75 posts "Highly confident" findings that 80 held
back, and Security Medium and Low findings now appear in the pull-request body. Reviews get longer.

**Renaming Nit to Minor touches every tally.** The `Review:` status line, the gate's severity table, the review buckets
and three eval banks all change in step. Until they do, two words for one level are in use.

**Batches are slower than free fan-out.** Queuing behind the surface limit and running overlapping tasks one at a time
lengthens a wide wave. The limit of 6 on a surface with no recorded limit may be lower than the host allows.

**The batch size of 20 items is chosen, not measured.** No run or source sets it. A batch that large may be judged less
carefully per item than a smaller one, and the number may need to change once a batched run is evaluated.

**A routing-selected `execute` asks one more question** on a surface that spawns only on request.

**Batched scoring and a Standard finder are untested trades.** One scorer per domain may judge a candidate less
carefully than one scorer per candidate, and a Standard finder costs more per run than a Light one. Neither has an eval
result behind it yet.

**The contract is enforced by reading.** Nothing checks that a template names every field its controller requires. A
conformance rule for that is later work and is not promised here.

## Considered and declined

**Keep each lane's scale and translate only at the gate.** Declined. That is the current design, and the translation
table is where Security Medium became Nit and where `escape` rows lost their producers.

**Put the severity scale in `routing`.** Declined. `review` produces most findings and already defines the severity
names; the consumers link to it.

**Remove the post pipeline's test-coverage reviewer.** Declined. Scoring keeps its output when repository guidance
requires coverage, so the dispatch is made conditional on that.
