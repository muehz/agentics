# `/agentics:triage pr`: PR triage orchestrator

This mode detects every automated review source on a PR. It dispatches subagents in isolated git worktrees to assess and
remediate each source. It integrates the results, pushes once, and posts the PR responses with correct commit
references.

## When to use

The [feedback classification](../SKILL.md#feedback-classification) decides who owns a PR's signal, and the
[Don't use when](../SKILL.md#dont-use-when) table names what goes elsewhere. In short, `/agentics:triage pr` takes:

- Review-bot findings, failed check runs, and a PR that is behind its base branch.
- 2+ unresolved human reviewer threads, as drafts the developer accepts one by one.
- Any of these together, when you don't know or care which tool posted each signal. The orchestrator detects them.

## Orchestration

Load these references for every invocation:

- [orchestration-lifecycle.md](orchestration-lifecycle.md): the 8-phase pipeline (Phases 0 to 7) with its snippets,
  controls, and SHA mapping
- [subagent-contract.md](subagent-contract.md): JSON output schema, subagent constraints, and dispatch template
- [rollup-template.md](rollup-template.md): unified rollup comment template

### Phase summary

| Phase | Name                     | Description                                                                                                                 |
| ----- | ------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| 0     | Preflight                | Note the caller's checkout. Read the PR once: host, state, fork code, authorization. Fetch the PR head.                     |
| 0.4   | Base Sync                | Merge the base into the PR in a dedicated sync worktree before detection. Merge only. Never rebase. Never for fork code.    |
| 1     | Detect Sources           | Run detection for all known sources. Record each source's lane.                                                             |
| 2     | Worktree Setup           | Create one worktree per source. Run setup and the baseline, except for fork code.                                           |
| 3     | Parallel Dispatch        | Launch one subagent per source with the specialist workflow.                                                                |
| 4     | Collect Results          | Read the subagent output. Apply lanes and path rules. The developer decides each draft.                                     |
| 5     | Integrate & Push         | Cherry-pick each source's fixes. Verify, inspect, stop on a moved tip, push once. The push carries the base-sync merge too. |
| 6     | Post PR Responses        | Run the outbound checks. Post the thread replies, then one rollup with source sections.                                     |
| 7     | Verify Posted Formatting | Fetch the posted comments back. Report non-blocking markdown/rendering issues.                                              |

### Subagent tier

Specialists that perform defined remediation, and the base-sync specialist, are implementers. Each runs at the Standard
tier of [model-ladders.md](../../routing/references/model-ladders.md), relative to the current driver; on surfaces where
that file resolves by role, they resolve as implementers. Drop to Light only for a mechanical sweep. The user's explicit
model, effort or tier for the stated scope overrides this default, as that file describes. A tier never changes a
source's classification, lane or posting rule.

## Arguments and source filtering

`/agentics:triage pr <PR-number>` runs all detected sources by default. To dispatch a single source, append
`--source <name>`:

```text
/agentics:triage pr 183 --source copilot
/agentics:triage pr 183 --source checkmarx
/agentics:triage pr 183 --source code-security
/agentics:triage pr 183 --source code-quality
/agentics:triage pr 183 --source bot-review
/agentics:triage pr 183 --source checks
/agentics:triage pr 183 --source human-feedback
/agentics:triage pr 183 --source merge-conflicts
/agentics:triage pr 183 --no-sync
```

When you supply `--source`, Phase 1 still runs detection for all sources, so the rollup can show the other sources as
`Skipped` rows per `rollup-template.md`. Only the named source proceeds through Phases 2–5. Each `Skipped` row has the
Details sentence `Skipped (not in --source filter).`

**`--source merge-conflicts` runs the base sync alone.** Base sync is Phase 0.4, not a Phase 1 source, so this filter
runs Phase 0 and 0.4 only, as [orchestration-lifecycle.md](lifecycle-preflight.md#04--sync-with-the-base-branch)
describes. `--source merge-conflicts --no-sync` is a contradiction, and the orchestrator refuses it with one line:
"`--source merge-conflicts` requires the base sync; drop `--no-sync` or drop the source filter."

**`--no-sync` skips the base sync entirely.** Sources run from the PR head as-is, and the rollup's Base sync section
records `SYNC_OUTCOME: skipped — --no-sync`. Use it when the PR is known to be behind and you want to triage its current
signal without waiting on a merge, or to debug the base sync itself.

**`--source human-feedback` with exactly 1 unresolved human thread forwards to `/agentics:review feedback`.** The
[feedback classification](../SKILL.md#feedback-classification) is required, not advisory. When Phase 1 finds exactly 1
unresolved human thread and the invocation uses `--source human-feedback`, explicitly or because the filter leaves no
other source, the orchestrator does not run triage's orchestration on the thread. It forwards it per
[the forwarding convention](../../routing/references/forwarding.md), announcing one line that names the safety scope:

_"This PR has 1 unresolved human thread — running `/agentics:review feedback` on it (drafts only, nothing posts)."_

It runs `/agentics:review feedback <PR-number>` with the thread data that Phase 1 already fetched, at most once. If
`/agentics:review feedback` routes the input back, it stops and explains the bounce. It never loops. A run with no one
present never forwards: Human Feedback is recorded as `Pending Review`, with its one needs-you bullet.

**Use `--source` when:**

- The size guard hard cap fires and you need to batch the run by source (see
  [`source-detection.md`](source-detection.md), "Size guards").
- One specialist fails and you want to triage the rest while you debug it.
- You only care about one signal type for this PR (e.g., a security-only sweep with `--source checkmarx`).

## Size guards

The orchestrator always prints the detection summary after Phase 1.3
(`Detected: Copilot Review (47 threads, 2 suppressed), …`), so you can see the run shape before any worktree exists.
Over a source's soft threshold it asks one question (proceed, batch by `--source`, or skip); over its hard cap (3× the
soft threshold) it stops and you batch by `--source`. The thresholds, and what a run with no one present does, are in
[`source-detection.md`](source-detection.md), "Size guards".

## Source adapters

Each source's reference files define that source entirely. The orchestrator loads the Triage Workflow file this table
names for each source that Phase 1 detected; it is the specialist's workflow.

| Source                           | Triage Workflow                                                                                   | Additional References                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| -------------------------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Copilot Review                   | [copilot-triage.md](pull-requests/copilot/copilot-triage.md)                                      | [copilot-finding-dispositions.md](pull-requests/copilot/copilot-finding-dispositions.md), [copilot-scope-sweep.md](pull-requests/copilot/copilot-scope-sweep.md)                                                                                                                                                                                                                                                                                                                                              |
| Checkmarx One                    | [checkmarx-triage.md](pull-requests/checkmarx/checkmarx-triage.md)                                | [checkmarx-comment-parsing.md](pull-requests/checkmarx/checkmarx-comment-parsing.md), [checkmarx-finding-categories.md](pull-requests/checkmarx/checkmarx-finding-categories.md), [checkmarx-remediation-patterns-actions-js.md](pull-requests/checkmarx/checkmarx-remediation-patterns-actions-js.md), [checkmarx-remediation-patterns-generic.md](pull-requests/checkmarx/checkmarx-remediation-patterns-generic.md). The specialist picks the right one per PR context. See `checkmarx-triage.md` Phase 3. |
| GitHub Code Security             | [code-security-triage.md](pull-requests/github-code-security/code-security-triage.md)             | Reuses [checkmarx-finding-categories.md](pull-requests/checkmarx/checkmarx-finding-categories.md), [checkmarx-remediation-patterns-generic.md](pull-requests/checkmarx/checkmarx-remediation-patterns-generic.md), and [checkmarx-remediation-patterns-actions-js.md](pull-requests/checkmarx/checkmarx-remediation-patterns-actions-js.md) (same Phase 3 heuristic as Checkmarx)                                                                                                                             |
| GitHub Code Quality              | [code-quality-triage.md](pull-requests/github-code-quality/code-quality-triage.md)                | Reuses [copilot-finding-dispositions.md](pull-requests/copilot/copilot-finding-dispositions.md) and the reply cognition in [copilot-triage.md](pull-requests/copilot/copilot-triage.md). Adapter-specific autofix-verification rules live in its own triage file.                                                                                                                                                                                                                                             |
| Bot Review                       | [bot-review-triage.md](pull-requests/bot-review/bot-review-triage.md)                             | Reuses Copilot's dispositions and reply cognition                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Checks                           | [checks-triage.md](checks/checks-triage.md)                                                       | [checks-tier-policy.md](checks/checks-tier-policy.md), [checks-categories.md](checks/checks-categories.md), [checks-log-parsing.md](checks/checks-log-parsing.md)                                                                                                                                                                                                                                                                                                                                             |
| Human Feedback                   | [human-feedback-triage.md](pull-requests/human-feedback/human-feedback-triage.md) (orchestration) | Cognitive reference: [skills/review/references/feedback/receiving-feedback.md](../../review/references/feedback/receiving-feedback.md) — the human-feedback specialist is intentionally thin and imports `/agentics:review feedback`'s anti-sycophancy + blocking-vs-preference rules                                                                                                                                                                                                                         |
| Base Sync (not a Phase 1 source) | [merge-conflicts-triage.md](pull-requests/merge-conflicts/merge-conflicts-triage.md)              | Resolution cognition: [skills/git/references/merge-conflicts.md](../../git/references/merge-conflicts.md) — the specialist is intentionally thin and imports `/agentics:git resolve`'s intent-preserving resolution discipline. Phase 0.4 calls it before detection. The orchestrator does not dispatch it alongside the sources above. Its merge commit is the base every source worktree starts from — no fast-forward, no cherry-pick. Merge only — never rebase.                                          |

Detection patterns for all sources are in [source-detection.md](source-detection.md).

### Adding a new source

1. Add a detection entry to `source-detection.md` (accepted author, lane, comment signature, detection command).
2. Create `references/pull-requests/{source}/{source}-triage.md` with the specialist triage workflow, and add its row to
   the table above.
3. Optionally create `{source}-categories.md`, `{source}-parsing.md`, or `{source}-remediation.md` beside it.
4. No changes to SKILL.md or the orchestration lifecycle are needed.

## Response routing

After the push, the orchestrator posts responses in the format appropriate to each source:

| Source               | Response Type                                                                                                                                      | Method                                                                                                                                             | Idempotency Marker                                                                  |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Copilot Review       | Thread reply per finding (auto-posted)                                                                                                             | `add_reply_to_pull_request_comment` (root commentId)                                                                                               | `<!-- agentics:triage:reply:copilot:{threadId} -->`                                 |
| Checkmarx One        | Section in the rollup (collapsed `<details>`)                                                                                                      | Rendered inside the rollup (6.4)                                                                                                                   | `<!-- agentics:triage:section:checkmarx -->`                                        |
| GitHub Code Security | Thread reply per finding (auto-posted)                                                                                                             | `add_reply_to_pull_request_comment` (root commentId)                                                                                               | `<!-- agentics:triage:reply:code-security:{threadId} -->`                           |
| GitHub Code Quality  | Thread reply per finding (auto-posted)                                                                                                             | `add_reply_to_pull_request_comment` (root commentId)                                                                                               | `<!-- agentics:triage:reply:code-quality:{threadId} -->`                            |
| Bot Review           | Thread reply per finding, auto-posted on the automatic lane and after acceptance on the draft lane; one rollup row per bot login                   | `add_reply_to_pull_request_comment` (root commentId)                                                                                               | `<!-- agentics:triage:reply:bot-review:{threadId} -->`                              |
| Checks               | One rollup section per root cause                                                                                                                  | Rendered inside the rollup (6.4)                                                                                                                   | `<!-- agentics:triage:section:checks:{check_run_id} -->`                            |
| Human Feedback       | Drafted thread reply per finding — posted only after the developer accepts it                                                                      | `add_reply_to_pull_request_comment` only after acceptance (Phase 4.2)                                                                              | `<!-- agentics:triage:reply:human-feedback:{threadId} -->`                          |
| Base Sync            | Rollup section, always rendered as the first Details block — outcome line, version-bump notes, and the per-file table when conflicts were resolved | Rendered inside the rollup, ahead of every source (6.4)                                                                                            | `<!-- agentics:triage:section:base-sync -->`                                        |
| _(new source)_       | Thread reply when the source has a thread; otherwise a rollup section, never a new top-level comment                                               | Per source adapter                                                                                                                                 | Per source adapter                                                                  |
| Rollup               | The execution's one top-level comment, created by the command and edited in place by its follow-ups                                                | `add_issue_comment` to create; the GitHub MCP issue-comment update tool, else `gh api -X PATCH` on the issue comment, to edit (see Tool reference) | `<!-- agentics:triage:rollup -->` and `<!-- agentics:triage:execution:{RUN_ID} -->` |

**Post cap.** One execution creates at most one top-level PR comment, the rollup, and makes at most N + 1 posts, where N
is the number of existing review threads it replies to. Anything with no thread to reply in is a section of the rollup.
The orchestrator leaves untouched the comments that earlier versions posted under the retired per-source markers, and
the rollup says it supersedes them. The details, including the one exception for a deleted rollup, are in
[orchestration-lifecycle.md](lifecycle-publish.md#phase-6--post-pr-responses).

**Posting the replies is part of the job.** After a successful push, every automatic-lane reply and every accepted draft
is posted, and the Base sync, Checkmarx, and Checks content goes in the rollup. Only the three cases in
[Phase 6.2](lifecycle-publish.md#62--post-the-thread-replies) withhold a reply: a push that did not land, the outbound
checks, and a posting failure that survives the fallback. Each is reported.

Thread **resolution** stays with the developer, because resolving a thread hides the reply and is
[repository policy](../../gh/SKILL.md#repository-policy). Drafts never reach the PR until the developer accepts them in
Phase 4.2, and a run with no one present drafts nothing for them.

**Shell safety: never escape the `!`.** The idempotency markers are HTML comments, literally `<!--` … `-->` with **no
backslash**. Pass comment bodies through MCP tools or `gh api -F "body=@$body_file"` with the draft file path read as
data, never through a double-quoted shell string. See [Phase 6.2](lifecycle-publish.md#62--post-the-thread-replies).

## Critical rules

**DO:**

- Report a dirty working tree, an operation in progress, and kept branches from an earlier run as notes. None of them
  stops the run.
- Detect all sources before you create any worktrees.
- Give every branch and worktree the run's `RUN_ID`, under `triage/pr-{PR_NUM}/`.
- Cherry-pick one commit at a time. Record the SHA mapping (old to new).
- Verify the combined result and inspect every commit before the push.
- When the PR branch moved during the run, push nothing and keep every branch; never merge the new tip.
- Post all PR responses after the push, so the SHAs exist on the remote.
- Verify posted responses after Phase 6. Fetch them back. Check them for markdown/rendering hazards.
- If one source fails, continue with the remaining sources.

**DON'T:**

- Rebase the PR branch, or force-push it. Triage integrates by merge, so its push is a plain fast-forward. A user who
  wants a rewrite asks `/agentics:git` for it.
- Run, build, install or test anything from a fork's head unless the user, in their own turn, named this PR and asked
  for it.
- Let subagents push, post PR comments, or invoke any GitHub write tools.
- Reference worktree branch SHAs in PR responses, because they won't exist on the PR branch.
- Push a commit on a path in [Paths triage never pushes](subagent-contract.md#paths-triage-never-pushes).
- **Post a reply or integrate a commit on the draft lane before the developer accepts it.** Human Feedback, draft-lane
  Bot Review logins, and commits on shown-first paths are draft-only. Phase 4.2 is the only path from a draft to a
  posted reply or an integrated commit.

## Tool reference

For the full GitHub MCP tool catalog, see the `gh` skill. Key triage-specific notes:

| Action                  | Tool                                                                                                                                                             | Note                                                                                                                                                             |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Reply to review thread  | `add_reply_to_pull_request_comment`                                                                                                                              | Use root commentId, not threadId. Prepend the idempotency marker.                                                                                                |
| Post or edit the rollup | `add_issue_comment` to create it; the GitHub MCP issue-comment update tool, else `gh api -X PATCH repos/{owner}/{repo}/issues/comments/{comment_id}`, to edit it | The execution's only top-level comment. The command creates it, and its follow-ups edit it in place. Start it with the rollup marker and the execution key line. |
| Launch subagent         | This session's native parallel-dispatch path — see [surface dispatch](../../routing/references/surface-dispatch.md)                                              | See `subagent-contract.md` for the full template                                                                                                                 |
| Git operations          | `bash`                                                                                                                                                           | worktree add/remove, cherry-pick, merge, push                                                                                                                    |

Pagination rule: always paginate `get_review_comments` and `get_comments` with `perPage: 100` until exhausted.

## Related skills

- `git`: worktree directory selection, fork code, setup and baseline (`worktrees.md`), and the push workflow (`push.md`)
- `execute`: parallel subagent dispatch pattern
- `review`: human-authored code review
- `diagnose`: deep root-cause investigation when a single failure needs full diagnosis instead of triage
- `gh`: GitHub MCP tool catalog, the posting envelope, and repository policy
