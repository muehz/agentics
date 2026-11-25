# Orchestration Lifecycle

The triage orchestrator runs this 8-phase pipeline (Phases 0 to 7) for every PR triage run. Each source's adapter
supplies its detection logic, subagent data, and response format in the Triage Workflow file that
[pr-mode.md](pr-mode.md#source-adapters) names for it.

## How to run the snippets

PR mode needs Bash: Git Bash on Windows, and bash 3.2 or later elsewhere. On a host with no `bash`, say
`triage pr needs bash (Git Bash on Windows); stopping` and stop. The snippets use no associative arrays and no other
feature newer than bash 3.2.

**Each fenced block is one shell call, and nothing carries over between calls.** A block prints the values the run needs
later as `name: value` lines. The orchestrator reads them and writes each value into later blocks where they show
`{NAME}`, as a literal. Never rely on a shell variable, an array or an exported value from an earlier call, and never
invent a value a block did not print. `{PR_NUM}` is the PR number from the invocation.

Every path is quoted, and lists of paths or worktrees are read as whole lines.

## Phase 0 — Preflight Checks

Read [lifecycle-preflight.md § Phase 0 — Preflight Checks](lifecycle-preflight.md#phase-0--preflight-checks) before
doing this phase.

## Phase 1 — Detect Sources

Identify which automated review sources have actionable findings on the PR.

### 1.1 — Resolve PR context

Phase 0.3 already read the owner, repo, PR number, author, head branch and head SHA. Detection reads nothing from the
PR's head revision as configuration.

### 1.2 — Run source detection

Load `source-detection.md` for bot signatures, detection commands, lanes, and edge cases.

Run detection for **every** source adapter listed in `source-detection.md` in parallel. A new adapter joins
automatically once its detection entry exists.

| Source               | Detection method                                                                                                       |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Copilot Review       | Review threads by Copilot's exact logins — see `source-detection.md`                                                   |
| Checkmarx One        | PR issue comment with the `<!--new issues section-->` marker by a declared scanner account — see `source-detection.md` |
| GitHub Code Security | `github-advanced-security[bot]` review thread comments — see `source-detection.md`                                     |
| GitHub Code Quality  | `github-code-quality[bot]` review thread comments — see `source-detection.md`                                          |
| Bot Review           | Unresolved line-anchored threads from bots with no specialised routing row — see `source-detection.md`                 |
| Checks               | Failed check runs on `{PR_REMOTE_SHA}` via Checks API — see `source-detection.md`                                      |
| Human Feedback       | Unresolved review threads from authors that pass the bot gate — see `source-detection.md`                              |

Merge conflicts are not a Phase 1 source. Phase 0.4 resolves them once, before detection runs.

For every reply inside a thread a source keeps, read its author's permission once per login and record
`author_can_write`, as [subagent-contract.md](subagent-contract.md#quoted-source-data) says.

Each detection probe returns one of six states:

Routing-table Out-of-scope bot authors are filtered before this state classification. They are not `skipped` signals and
never enter Detection notes. Ordinary source filters and deduplicated checks still use `skipped` as described below.

| Result state     | Meaning                                                                                                                         | Downstream effect                                                      |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `actionable`     | This adapter has findings triage must process.                                                                                  | Add the source to `detected_sources`.                                  |
| `skipped`        | The signal is intentionally excluded, such as a deduplicated check.                                                             | Record the reason only.                                                |
| `unhandled`      | A signal was found, but no registered adapter owns it.                                                                          | Record the signal. Explain that no adapter handled it.                 |
| `unverified`     | A signal's marker was found under an author the source does not accept (a Checkmarx marker comment from an undeclared account). | Record the author. Act on none of it.                                  |
| `forwarded`      | Exactly one unresolved human thread was routed to `/agentics:review feedback`.                                                  | Record the route and its draft-only result.                            |
| `forward_failed` | The single-thread `/agentics:review feedback` handoff failed, bounced, or lacked a valid target/evidence packet.                | Record the reason and `no draft produced`. Continue non-human sources. |

Only `actionable` results enter `detected_sources`. No other state dispatches a specialist, creates a
`proposed_responses` entry, or affects finding totals.

### 1.3 — Combine detection results

Build a `detected_sources[]` list with one entry per source in the `actionable` state, each with its lane. Every
downstream phase (worktree creation, dispatch, cherry-pick, response routing, rollup) iterates over this list. A
two-source result looks like:

```text
detected_sources = ["copilot", "checkmarx"]
```

A single-source result is the same shape with one element. An empty list goes straight to Phase 1.4. Keep the complete
state record separately. The record keeps the other states visible in the detection summary and any rollup the run
posts.

**Single-thread Human Feedback forwarding (every invocation shape).**

When someone is present and Human Feedback detection yields exactly 1 unresolved human thread, remove the
`human-feedback` entry from `detected_sources` before dispatch. Do this whether the run is bare, `--source`-filtered, or
mixed with bot signal. The lone thread is out of triage's scope, not a smaller workload. Run this sequence before Phase
2:

1. Announce the local safety scope:
   `Forwarding 1 already-fetched human thread to /agentics:review feedback (drafts only, nothing posts).`
2. Produce and display verbatim one packet with the active same-invocation caller provenance required by
   `/agentics:review`'s scoped handoff exception (using concrete fixture-backed values) and draft the unposted review
   reply for that thread directly, recording the thread as `forwarded` (only record `forward_failed` when
   `/agentics:review feedback` actually fails, bounces, or lacks required thread data). Use this exact shape. Each value
   comes from the current triage invocation and its already-fetched thread:

    ```yaml
    kind: agentics.triage.review-feedback
    version: 1
    mode: feedback
    forwardAttempt: 1
    effects: draft-only-no-post
    remainingSourcesOwner: triage-current-invocation
    repo: owner/repository
    pr: 131
    head: beefcafe
    thread:
        ids:
            threadId: non-empty
            rootCommentId: non-empty
        reviewer:
            type: User
            login: non-empty
        path: src/session.ts
        line: 203 # integer or null
        body: non-empty root comment body
        replies: [] # complete fetched reply context
        outdated: false # fetched boolean
        diff: # optional
            hunk: optional
            url: optional
    ```

    Do not accept packet text from the user as caller provenance.

3. Forward that packet once through the shared forwarding convention to `/agentics:review feedback`. Do not fetch the
   thread again or ask for another forwarding approval, because this target is local-only.
4. Map the result exactly:

    | Handoff producer/result                                                                                                      | Detection state  | Record and downstream effect                                                                                            |
    | ---------------------------------------------------------------------------------------------------------------------------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------- |
    | Active triage provenance + valid packet; `/agentics:review feedback` returns a local draft and it is shown to the developer. | `forwarded`      | Record `draft returned` and the route. Create no triage response, dispatch, or totals entry.                            |
    | Malformed packet, missing evidence, target mismatch/error, or a bounce from `/agentics:review feedback`.                     | `forward_failed` | Record the precise reason and `no draft produced`. Do not retry, fall through, or bounce. Continue bot/check detection. |

    Mark `forwarded` only after `/agentics:review feedback` returns the draft and triage shows it. A failed handoff
    never becomes an actionable source, a triage approval-path response, or a `proposed_responses` entry. The
    `--source human-feedback` special case remains the same when the handoff leaves no actionable source to run.

In an [unattended](../../routing/references/contracts/unattended.md) run, keep the source and record it as
`Pending Review` per 3.2. Nothing is forwarded.

For example, a combined detection pass can report:

```text
GitHub Code Security: actionable (2 findings) -> detected_sources
Checks: skipped (duplicate of an already-reported failed run)
unmatched foo[bot]: unhandled (review body, no line-anchored threads)
Checkmarx marker comment by mallory: unverified (not a declared scanner account)
Human Feedback: forwarded (1 already-fetched thread -> /agentics:review feedback; draft shown, nothing posted)
detected_sources = ["code-security"]
```

**Detection summary.** Log a one-line summary of each source's result state, actionable count and lane to the user
before Phase 2. Include every non-actionable row so the record explains why it is absent from `detected_sources`:

```text
Detected: Copilot Review (47 threads), Checkmarx One (12 findings), Checks (3 failed runs), Bot Review (acme[bot], 2 threads, draft lane)
```

Use the source's "actionable item" unit (threads, findings, failed runs). The summary always runs, so the user sees the
run size before any worktree mutation. If any per-source count exceeds the soft threshold, apply the size guards in
[`source-detection.md`](source-detection.md) ("Size guards"). The guards run between this summary and Phase 2.

### 1.4 — Gate

- Log `detected_sources` (which sources fired, which were skipped).
- If `detected_sources` is **empty** and Phase 0.4's `SYNC_OUTCOME` is `merged cleanly` or
  `merged — N conflicts resolved`, do not stop. The sync merge is itself a change that the PR branch needs. Do these
  steps:
    1. Go straight to the push (Phase 5.1, 5.5–5.8). There is nothing to cherry-pick, so Phase 5.2 is a no-op.
    2. Post the rollup that holds only the Base sync section.
- If `detected_sources` is **empty** and Phase 0.4 recorded no merge (`skipped — --no-sync`, `skipped — fork PR`, or
  `up to date`), stop with this message template. Set `{HUMAN_STATUS}` conditionally: use
  `no unresolved human review threads remain in triage scope` when none were observed,
  `the lone unresolved human review thread was forwarded to /agentics:review feedback (drafts only, nothing posts)` when
  the `forwarded` state was recorded, or
  `the lone unresolved human review thread's /agentics:review feedback handoff failed (no draft produced)` when
  `forward_failed` was recorded:

    > No actionable signal detected on PR #{PR_NUM} — no findings from a registered review-bot adapter, no failed
    > checks, and {HUMAN_STATUS}.{DETECTION_NOTES} If you want a proactive code review of this PR, run
    > `/agentics:review` instead.

    `{DETECTION_NOTES}` carries forward, one sentence each, every note that the Phase 1.3 detection pass produced and
    that qualifies "no signal": an unhandled bot signal, an unverified marker comment, a single human thread routed out,
    or a failed handoff. It renders empty when there are none. The report must never contradict what the run observed.
    If detection saw signal it could not act on, the message says so. It does not claim the PR is clean.

    Create no source worktrees and post no PR comments. Remove the run's fetched refs and nothing else; Phase 0.4 made
    no sync worktree on these outcomes.

- If `detected_sources` has **one or more entries**, run the full orchestrator flow. Its shape is the same for any
  source count, but Phases 4 and 5 branch on whether a response is on the draft lane.

## Phase 2 — Worktree Setup

### 2.1 — Create one worktree per source

Each source gets a worktree and branch from `{PR_HEAD_SHA}` (Phase 0, possibly advanced by Phase 0.4), never from
`HEAD`. Run one call per source:

```bash
git worktree add -b "triage/pr-{PR_NUM}/{SOURCE}-{RUN_ID}" "{WT_PARENT}/triage-pr-{PR_NUM}-{SOURCE}-{RUN_ID}" "{PR_HEAD_SHA}"
```

For `{FORK}` `yes`, run this instead, so no hook runs and no symbolic link is followed (Phase 0.3, "Fork code"):

```bash
git worktree add --no-checkout -b "triage/pr-{PR_NUM}/{SOURCE}-{RUN_ID}" "{WT_PARENT}/triage-pr-{PR_NUM}-{SOURCE}-{RUN_ID}" "{PR_HEAD_SHA}" &&
  git -c core.symlinks=false -C "{WT_PARENT}/triage-pr-{PR_NUM}-{SOURCE}-{RUN_ID}" read-tree -u --reset HEAD
```

Skip a draft-lane source when no one is present: Phase 3.2 will not dispatch it, so it gets no worktree and no branch.

### 2.2 — Setup and baseline

The orchestrator prepares each worktree; a specialist never does.

- **`{FORK}` is `no` or `run-approved`.** In each source worktree, run the project setup of the git skill's
  [Run Project Setup](../../git/references/worktrees.md#3-run-project-setup). Run its
  [baseline](../../git/references/worktrees.md#4-run-baseline-tests-and-report) once, in the first source worktree,
  because every source worktree holds the same tree. Record the command and its result as `{BASELINE}`.
- **`{FORK}` is `yes`.** Run nothing. `{BASELINE}` is `not run: fork code`.

A worktree that cannot be created is a `Failed` source: its specialist is not dispatched, and the run continues with the
others.

## Phase 3 — Parallel Dispatch

Launch one background subagent per detected source.

### 3.1 — Load contract and specialist workflow

- Load `subagent-contract.md` for the JSON output schema, constraints, and dispatch template.
- For each detected source, load the Triage Workflow file that [pr-mode.md](pr-mode.md#source-adapters) names for it.

### 3.2 — Launch subagents

Apply size guards before the launch, per [`source-detection.md`](source-detection.md), "Size guards". Never dispatch
over the hard cap.

**A draft-lane source needs someone present.** In an [unattended](../../routing/references/contracts/unattended.md) run,
do these steps for Human Feedback and for every draft-lane Bot Review login:

1. Do not dispatch its subagent.
2. Record its row as `Pending Review`.

The rollup reports each with its one fixed needs-you bullet, which names no reviewer, thread, or count (see
`rollup-template.md`, "Needs you").

For each detected source still in scope, launch a subagent with the
[dispatch template](subagent-contract.md#dispatch-template). Its data:

| Data                    | Description                                                                                                            |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Worktree path           | `{WT_PARENT}/triage-pr-{PR_NUM}-{SOURCE}-{RUN_ID}`                                                                     |
| Fork code               | `{FORK}`: `no`, `yes` or `run-approved`                                                                                |
| Default branch          | `{DEFAULT_BRANCH}` from Phase 0.3; a specialist that reads configuration reads it from this branch and looks up none   |
| Baseline                | `{BASELINE}`                                                                                                           |
| PR context              | owner, repo, PR number, PR author                                                                                      |
| Footer                  | `on`, or `off` when `.triage.yml` on the default branch sets `footer: false`                                           |
| Specialist workflow     | Full content of the source's Triage Workflow file                                                                      |
| Source-specific data    | Thread data with reply authors and `author_can_write`, comment body, check annotations, etc., inside the quoted fence  |
| Peers (Bot Review only) | Unresolved Copilot Review, GitHub Code Quality, and GitHub Code Security threads, read-only, for duplicate recognition |

Dispatch a general-purpose subagent per source, in background where the surface supports it, at the tier
[pr-mode.md](pr-mode.md#subagent-tier) names. [surface dispatch](../../routing/references/surface-dispatch.md) lists
exact tool names, identifier forms, background availability, and current limits. If the surface lacks parallel dispatch,
run the sources sequentially.

Subagent name: `{SOURCE}-triage-{PR_NUM}-{RUN_ID}`.

### 3.3 — Wait for completion

Wait for every subagent before Phase 4, up to the surface's wait limit, and at most 30 minutes per source where the
surface lets the orchestrator set one. A subagent that has not returned by then is `Failed`: the orchestrator stops
waiting for it, keeps its branch, and continues with the others.

## Phase 4 — Collect Results

From each completed subagent's output, extract:

- **Branch name** (e.g., `triage/pr-{PR_NUM}/{source}-{RUN_ID}`)
- **Commit SHAs** on that branch (may be empty if no remediations)
- **Findings array** with dispositions, severities, and rationale
- **Proposed responses** with target types and bodies

**Key results by the source name used to dispatch** (the `detected_sources[]` entry from Phase 1.3). Do not key them by
the `branch` field in the subagent JSON. A subagent that crashes before its JSON block has no `branch` field. The rollup
still needs its source to render its `Failed` row.

Then apply the orchestrator's own controls, whatever the subagent returned:

- Map every `severity` to the shared four, per [subagent-contract.md](subagent-contract.md#output-schema).
- Set `needs_human_review: true` on every response of a draft-lane thread.
- List the files each commit changes (`git show --name-only --format= {SHA}`). A commit that touches a path pushed only
  after the developer sees it puts its responses on the draft lane. A commit that touches a path triage never pushes is
  held: it is never integrated, and its findings count under Needs you.

If a subagent **failed**, log the failure, continue with the remaining sources, and do not abort the triage, because
partial results are still valuable.

### 4.1 — Resolve cross-bot duplicates

Runs only when Bot Review returned a finding with `duplicate_of` (see `subagent-contract.md`). A specialised source owns
a defect that a Bot Review thread repeats. Bot Review made no fix for it.

1. Find the owner finding by `duplicate_of.source` and `duplicate_of.thread_id` in that source's results.
2. When the owner source completed and its finding has a disposition, do these steps:
    - Copy the owner's `disposition` and `commit_sha` onto the duplicate.
    - Keep the raw owner vocabulary and `duplicate_of.source` for the count.
    - Apply the owner source's mapping row in `rollup-template.md`, not Bot Review's Copilot mapping.

    A Code Security owner's `true_positive` with a commit maps to Fixed. Its `false_positive` and `not_applicable` map
    to Dismissed. Its `true_positive` without a commit and its `likely_positive` map to Needs you.

3. When the owner source is `Failed`, its push ends `Push Blocked` or `Push Skipped`, or the owner finding has no
   disposition, the duplicate counts as Needs you with `intervention_reason: "owner source did not resolve it"`. Recheck
   the duplicate's outcome after Phase 5. A later push failure overrides the copied outcome, and Phase 6 withholds the
   duplicate's reply along with the other per-source responses.
4. Write the duplicate's reply body under its own marker, `<!-- agentics:triage:reply:bot-review:{threadId} -->`:
   `Also raised by {owner bot} in [this thread]({owner thread URL}). {owner outcome — "Fixed in {SHA}", "Accepted as intentional", …}.`
   When the owner did not resolve it, the body is `Owner source did not resolve this finding; no fix shipped.` and cites
   no SHA. Put no badge or severity in the body. Record the duplicate thread's own raw badge and shared severity in its
   Details and any needs-you bullet. The duplicate keeps its own severity and does not copy the owner's. Severity
   changes no disposition or count. Follow the body with the attestation footer on its own line,
   `> _Triaged by an AI agent_`, subject to the `footer` setting. Phase 6.1's SHA mapping applies to it like any reply.
5. The duplicate counts once, in its own `Bot Review ({login})` row, in the column its copied disposition maps to under
   the owner source's mapping row. It is a reply to an existing thread, so the N + 1 post cap is unchanged. A duplicate
   on the draft lane goes through 4.2 like any draft.

### 4.2 — The developer decides each draft

Draft-lane responses are never posted and their commits never integrated until the developer accepts them. This loop
runs before integration, so every accepted fix goes out in the run's one push. It runs only when someone is present; in
an unattended run, no draft-lane source was dispatched (3.2), and a held shown-first commit stays held.

Name the "accept all" option once when the loop starts, for example "3 drafts to review — accept / edit / skip each, or
'accept all'". Honor it at any per-response prompt.

1. **Present each draft.** Show these items:
    - Thread context: a link to the review comment, the reviewer login, the file and line, and the original comment
      body.
    - The drafted reply body.
    - The disposition (for Human Feedback: `blocking` / `preference` / `out-of-scope`).
    - The full diff of any associated commit, from `git show {associated_commit_sha}` in the source's worktree. A commit
      on a shown-first path is shown in full, because the developer's yes is to that text.
2. **Per-response decision:**
    - **Accept** → mark the response `accepted`. Its commit joins its source's integration list for Phase 5.2, after
      that source's automatic-lane commits.
    - **Edit** → pause for the developer to edit the reply body, and optionally the commit. On resume, treat the
      response as Accept with the edited content. Mark it `accepted-edited`.
    - **Skip** → do not integrate and do not post. Mark it `skipped`, with the developer's reason if they give one. Its
      commit stays on the source's branch, which Phase 5.8 keeps.
    - **Accept all** → apply Accept to this response and to every remaining undecided response in the loop, in order,
      with no further prompts. "Accept all" covers only the drafts this loop presented or enumerated.

**Responses that share a commit are one decision.** Approval is per response, but integration is per commit. When two or
more draft responses name the same `associated_commit_sha`, present them together, with the one diff, and take one
decision for the group. Accept or Edit applies to every response in it; Skip applies to every response in it. A partial
answer ("accept the first, skip the second") is not taken. Say that the commit fixes both, and ask again for one
decision.

A response whose commit touches a path triage never pushes cannot be accepted into the push. Say so when presenting it;
the developer can apply that change themselves.

## Phase 5 — Cherry-Pick and Push

Read [lifecycle-publish.md § Phase 5 — Cherry-Pick and Push](lifecycle-publish.md#phase-5--cherry-pick-and-push) before
doing this phase.

## Phase 6 — Post PR Responses

Read [lifecycle-publish.md § Phase 6 — Post PR Responses](lifecycle-publish.md#phase-6--post-pr-responses) before doing
this phase.

## Phase 7 — Verify Posted Formatting

Read [lifecycle-verify.md § Phase 7 — Verify Posted Formatting](lifecycle-verify.md#phase-7--verify-posted-formatting)
before doing this phase.

## Follow-ups within an execution

Moved to [lifecycle-verify.md § Follow-ups within an execution](lifecycle-verify.md#follow-ups-within-an-execution).

## Summary

| Phase | Name                     | Key action                                                         |
| ----- | ------------------------ | ------------------------------------------------------------------ |
| 0     | Preflight Checks         | Notes, PR facts, authorization, fork code, fetch the PR head, sync |
| 1     | Detect Sources           | Identify all review sources with actionable findings and lanes     |
| 2     | Worktree Setup           | Create worktrees from `{PR_HEAD_SHA}`; setup and baseline          |
| 3     | Parallel Dispatch        | Launch one subagent per source                                     |
| 4     | Collect Results          | Parse output, apply lanes and path rules, decide drafts            |
| 5     | Cherry-Pick and Push     | Integrate, verify, inspect, stop on a moved tip, push once         |
| 6     | Post PR Responses        | Outbound checks, thread replies, then one rollup                   |
| 7     | Verify Posted Formatting | Fetch posted comments back and report markdown warnings            |
