# Orchestration Lifecycle: Phases 5–6 — Integrate, Push, and Post

Split from [orchestration-lifecycle.md](orchestration-lifecycle.md). Covers cherry-picking, combined verification,
pre-push inspection, pushing, thread replies, and rollup posting.

## Phase 5 — Cherry-Pick and Push

### 5.1 — Create the merge worktree

For `{FORK}` `yes`, Phases 5.1 to 5.7 do not run (Phase 0.3, "Fork code"): `PUSH_STATUS` is `fork_skipped`, and the run
goes to 5.8.

Create the merge worktree from `{PR_HEAD_SHA}`. Whenever `SYNC_OUTCOME` was `merged cleanly` or
`merged — N conflicts resolved`, `{PR_HEAD_SHA}` is the merge commit.

```bash
git worktree add -b "triage/pr-{PR_NUM}/merge-{RUN_ID}" "{WT_PARENT}/triage-pr-{PR_NUM}-merge-{RUN_ID}" "{PR_HEAD_SHA}"
```

Record `{MERGE_WT}` as that path.

### 5.2 — Integrate commits (cherry-pick)

A Phase 0.4 base-sync merge commit is this worktree's starting point and is not integrated here. The orchestrator
cherry-picks the integration list of each source, one call per source, in one stable source order: the
`detected_sources` order. A source's list is its `created_commits` in the order the source made them, filtered to the
eligible ones: automatic-lane commits that Phase 4 did not hold, and commits the developer accepted in 4.2. The two
lanes are never concatenated, because a later commit may build on an earlier one from the other lane, and picking it
first would conflict. A source with nothing on its list has no call.

```bash
held=''
for sha in {SOURCE_COMMITS}; do
  if [ -n "$held" ]; then printf 'map: %s held\n' "$sha"; continue; fi
  if git -C "{MERGE_WT}" cherry-pick "$sha" >/dev/null 2>&1; then
    printf 'map: %s %s\n' "$sha" "$(git -C "{MERGE_WT}" rev-parse HEAD)"
  elif git -C "{MERGE_WT}" rev-parse -q --verify CHERRY_PICK_HEAD >/dev/null &&
       [ -z "$(git -C "{MERGE_WT}" diff --name-only --diff-filter=U)" ] &&
       git -C "{MERGE_WT}" diff --cached --quiet &&
       git -C "{MERGE_WT}" diff --quiet; then
    git -C "{MERGE_WT}" cherry-pick --skip >/dev/null 2>&1
    printf 'map: %s already_applied\n' "$sha"
  else
    git -C "{MERGE_WT}" cherry-pick --abort
    printf 'map: %s conflict\n' "$sha"
    held=1
  fi
done
```

A cherry-pick that stops with no conflicted file and nothing staged or changed stopped because the change is already on
the branch, so the run skips it and records `already_applied`; no empty commit is ever made. One that stops with staged
or changed files could not make its commit (a failing commit hook, for example), so it is aborted and recorded as
`conflict`, never skipped: a skip would drop the fix while the reply called it applied. A conflict ends the source: its
later commits are `held` and are not integrated, because they may depend on the one that conflicted.

### 5.3 — SHA mapping states

| State                  | Meaning                                                                            | How to handle in PR responses                           |
| ---------------------- | ---------------------------------------------------------------------------------- | ------------------------------------------------------- |
| old -> new             | Successfully cherry-picked                                                         | Reference the new SHA in thread replies                 |
| old -> already_applied | The change is already on the PR branch                                             | Say "already applied" in the response                   |
| old -> conflict        | Could not merge cleanly                                                            | Not fixed on the PR; the finding counts under Needs you |
| old -> held            | Not integrated: an earlier commit of its source conflicted, or a path rule held it | Not fixed on the PR; the finding counts under Needs you |

A response whose commit is `conflict` or `held` is not posted as a fix. Its finding moves to Needs you, and the rollup's
bullet names the reason.

### 5.4 — Verify the combined result

The pushed tree is the combination, not any one source's tree, so check it once:

- **`{FORK}` is `no` or `run-approved`.** Run the project setup in `{MERGE_WT}`, then the verification command
  `{BASELINE}` ran. When the baseline passed and this run fails, push nothing: set `PUSH_STATUS` to `push_blocked` with
  the reason "the combined fixes fail the verification command", and keep every branch. When the baseline already
  failed, continue, and say in the session report that the command failed before and after.
- **`{FORK}` is `yes`.** Run nothing.

**Record the flow rows.** When the push is authorized and the PR's work item has a flow, write its rows now, in
`{MERGE_WT}`, and commit them there, so they travel with the push: a `commit triage` row for each integrated commit and
an `escape` row for each fixed Critical or Important finding, as [SKILL.md](../SKILL.md#logging-escapes) describes. A
push that then fails leaves them unpushed with the rest.

### 5.5 — Pre-push inspection

Before any push, the orchestrator itself lists what the push would carry and checks it. It is the control that holds
when a specialist did not follow its constraints.

```bash
git -C "{MERGE_WT}" log --diff-merges=remerge --name-only --format='commit: %H' HEAD --not "{PR_REMOTE_SHA}" "{BASE_SHA}"
```

Use `{PR_REMOTE_SHA}` alone in place of the two refs when Phase 0.4 fetched no base (`--no-sync` or fork code). The
listing holds every commit this run made, merge commits included, and nothing the base brought in. For a merge commit
(the Phase 0.4 sync) it lists every file whose merged content differs from git's automatic merge, so a conflict
resolution or any other change recorded inside a merge is inspected like an ordinary commit. Push nothing when any
listed file:

- is a path in [Paths triage never pushes](subagent-contract.md#paths-triage-never-pushes);
- is a shown-first path whose commit the developer did not accept in 4.2; or
- lies under `{WT_PARENT}`.

On a failure, set `PUSH_STATUS` to `push_blocked` with the reason, name the commit and the path in the session report,
and keep every branch. Run the posting envelope's
[outbound checks](../../gh/references/posting-envelope.md#outbound-checks) on every reply body and every rollup section
here too, so a withheld finding is known before anything posts.

### 5.6 — The PR branch moved

Skip this step and 5.7 for fork code (`PUSH_STATUS` is `fork_skipped`) and when the push is not authorized (Phase 0.3).
Otherwise read the PR branch as the remote holds it now:

```bash
git ls-remote "{BASE_REMOTE}" "refs/heads/{PR_BRANCH}" | cut -f1 | sed 's/^/remote-head: /'
```

When `remote-head:` equals `{PR_REMOTE_SHA}`, go to 5.7. When it differs, someone pushed since Phase 0, and the run
stops before the push: set `PUSH_STATUS` to `push_blocked` with the reason "the PR branch moved during the run", and
keep every branch. Triage does not merge the new tip. A merge would run the new tip's own git hooks, which agents never
turn off, and its result could only be verified by running commits that arrived after the user started the run. The
developer can run `/agentics:triage pr {PR_NUM}` again, which starts from the new tip, or integrate the kept merge
branch themselves.

### 5.7 — Push

The push follows the git skill's [push workflow](../../git/references/push.md): it reports the branch, the remote and
the commit count first, runs as a plain push with no force flag, makes no second attempt after a rejection, and counts
as pushed only when the remote then holds the pushed SHA. Its one difference is the destination: the local merge branch
goes to the PR's head branch.

```bash
git -C "{MERGE_WT}" rev-list --count HEAD --not "{PR_REMOTE_SHA}" | sed 's/^/commits-to-send: /'
git -C "{MERGE_WT}" push --no-follow-tags "{BASE_REMOTE}" "refs/heads/triage/pr-{PR_NUM}/merge-{RUN_ID}:refs/heads/{PR_BRANCH}" && echo 'push: ran'
git ls-remote "{BASE_REMOTE}" "refs/heads/{PR_BRANCH}" | cut -f1 | sed 's/^/remote-head: /'
git -C "{MERGE_WT}" rev-parse HEAD | sed 's/^/local-head: /'
```

`PUSH_STATUS` is `ok` only when `push: ran` printed and `remote-head:` equals `local-head:`. Otherwise it is
`push_blocked`, and the session report gives git's message and what each side holds. A rejected push is never retried or
forced here; the user can run `/agentics:git push` for it later. Record `{PUSHED_HEAD}` as `local-head:` on `ok`.

A plain push is a fast-forward of the PR branch, because the merge branch started from `{PR_REMOTE_SHA}` and 5.6 found
no newer tip. The remote refuses it when the branch moved again in between, which is the guard against losing a
teammate's commit.

**Fork PRs.** Triage never pushes to a contributor's fork. With `PUSH_STATUS` `fork_skipped`, the proposed fixes stay on
this machine: for `run-approved` on the merge branch, and for `yes` uncommitted in each source worktree. The session
report names where they are and shows the diff the maintainer can offer the contributor; the rollup says the fixes were
not pushed.

### 5.8 — Cleanup

The run removes what it created, with the git skill's plain [removal](../../git/references/worktrees.md#removal), and
never retries a refused removal with `--force`.

| `PUSH_STATUS`            | Remove                                                                                                                                                | Keep                                                                                                              |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `ok`                     | The merge worktree and branch, the sync worktree and branch, and each source worktree and branch whose commits were all integrated or already applied | A source branch with a skipped draft, a held commit or a conflict, and its worktree                               |
| `fork_skipped`           | For `run-approved`, each source worktree whose commits were all integrated into the merge branch; for `yes`, nothing                                  | For `run-approved`, the merge branch and worktree; for `yes`, every source worktree. They hold the proposed fixes |
| `push_blocked`           | Nothing                                                                                                                                               | Everything, per the abort contract                                                                                |
| no push (not authorized) | Each source worktree whose commits were all integrated into the merge branch                                                                          | The merge branch and worktree                                                                                     |

Delete a removed worktree's branch with `git branch -D "triage/pr-{PR_NUM}/{name}-{RUN_ID}"` only after its worktree is
gone; its commits are on the PR branch or were never wanted. Delete the run's fetched refs
`refs/remotes/triage/pr-{PR_NUM}` and `refs/remotes/triage/pr-{PR_NUM}-base` with `git update-ref -d` in every row. Name
every kept branch and worktree in the session report.

## Phase 6 — Post PR Responses

The orchestrator owns all PR interaction. Subagents never post comments. Nothing in this phase runs when posting is not
authorized (Phase 0.3); the session report then holds the reply drafts and the rollup body.

**Post cap.** A run makes at most N + 1 posts, where N is the number of existing review threads that it replies to. An
execution (`rollup-template.md`, "Execution") creates exactly one top-level PR comment, its rollup, and afterwards only
edits that comment in place (see "Follow-ups within an execution"). The post order is the thread replies, then the
single rollup create (6.4). A response with no thread to reply in is a `rollup_section`. It is never posted on its own.

**Every body passes the outbound checks.** Before any post, run the posting envelope's
[outbound checks](../../gh/references/posting-envelope.md#outbound-checks) with `{VISIBILITY}`. For a withheld reply,
name its finding to the user in the session with the repository's private channel. The rollup counts it without
describing it. Report every withheld item in the session.

### 6.1 — Check idempotency and apply the SHA mapping

**Idempotency.** Before a thread reply posts, look for an existing reply on that thread that carries the same marker and
whose author is `{LOGIN}`. If one exists, update it in place. A reply with the marker under any other author is never
edited and never counted; post triage's own reply beside it and name the foreign one in the session report. Sections
keep their marker search inside the rollup body.

The rollup has no marker search here. Each command creates its own rollup under its execution key and never updates an
earlier rollup that a marker finds (`rollup-template.md`, "Idempotency"). 6.4 searches for the earlier rollup only to
link it.

**Marker conventions for new content** avoid the substring collision between the legacy `<!-- triage -->` and
`<!-- triage-summary -->` markers:

- `<!-- agentics:triage:rollup -->`: unified rollup comment, always its first line
- `<!-- agentics:triage:execution:{RUN_ID} -->`: the execution key, always the rollup's second line
- `<!-- agentics:triage:reply:{source}:{threadId} -->`: a thread reply
- `<!-- agentics:triage:section:{key} -->` and `<!-- agentics:triage:section-end:{key} -->`: a source section inside the
  rollup

For existing PR comments, search for two older marker generations alongside the current markers, under `{LOGIN}` only:

1. The un-namespaced colon family: `<!-- triage:rollup -->`, `<!-- triage:rollup:{source} -->`,
   `<!-- triage:reply:{source}:{threadId} -->`, `<!-- triage:drafts -->`.
2. The legacy pre-colon markers: `<!-- triage -->` for Copilot, `<!-- checkmarx-triage-summary -->`,
   `<!-- checks-triage-summary -->`, `<!-- triage-summary -->`.

Write only the current `agentics:`-namespaced form. A thread reply found under an older marker is rewritten under the
current marker when triage updates it. An older rollup is never rewritten, because no run updates an earlier rollup.

The per-source markers `<!-- agentics:triage:rollup:checkmarx -->`,
`<!-- agentics:triage:reply:checks:{check_run_id} -->`, and `<!-- agentics:triage:rollup:merge-conflicts -->`, with
their older forms in both families above, are **retired**. Search for them only to decide whether the rollup renders its
supersedes line. Never update, delete, or post under them.

**SHA mapping.** For each source, walk the `proposed_responses` array. Replace worktree-branch SHAs with the pushed SHAs
from the Phase 5.2 mapping.

| Mapping state          | Action                                                      |
| ---------------------- | ----------------------------------------------------------- |
| old -> new             | Replace `{SHA}` with the new cherry-picked SHA              |
| old -> already_applied | Replace `{SHA}` with an "already applied" note              |
| old -> conflict        | Do not post the response as a fix; its finding is Needs you |
| old -> held            | Do not post the response as a fix; its finding is Needs you |

### 6.2 — Post the thread replies

**When `PUSH_STATUS=ok`, posting the replies is required.** Every automatic-lane response, and every draft-lane response
the developer accepted, reaches the PR: a `thread_reply` as a reply on its thread, a `rollup_section` inside the rollup
per 6.4. These replies are the developer-facing record of what the agent changed and why it fixed or did not fix each
finding. Three things withhold a reply, and nothing else does:

- a push that did not land (`push_blocked`, `fork_skipped`): the SHAs the replies cite do not exist on the remote, so
  only the rollup goes out;
- the outbound checks (above); and
- a technical posting failure that survives the fallback below.

Report each withheld reply in the session, and a posting failure also in the rollup's `<sub>` line for its source. On
the thread-reply path, the forbidden act is to _resolve_ the thread, which is
[repository policy](../../gh/SKILL.md#repository-policy). A reply is never forbidden.

Post each reply to the thread's root comment, with the method the source's Triage Workflow file defines. The subagent's
`body` already contains the idempotency marker and the footer. Do not resolve the thread.

**Footer-off example.** With `.triage.yml` on the default branch set to `footer: false`, a reply keeps the marker as its
first line and the agent's voice. It omits only the attestation footer. Footer-off never permits impersonation of the PR
author.

**CRITICAL: the marker literal is `<!--`, never `<\!--`. Do not escape the `!`.** The idempotency markers are HTML
comments: literally `<!--` to open and `-->` to close, with **no backslash anywhere**. GitHub renders `<\!--` as visible
text, so the raw marker shows in the comment. The only thing that ever produces `<\!--` is bash **history expansion**,
when `<!--` sits inside a double-quoted string. The fix is never your own backslash; it is to keep the marker out of any
shell string. If you see `<\!--` in a body you are about to post, delete the backslash.

**Preferred: GitHub MCP tools.** `add_reply_to_pull_request_comment` and `add_issue_comment` take the body as a JSON
string parameter, so no shell runs and no `!` expansion happens.

**Fallback: `gh api` with the body in a file.** When MCP tools are unavailable, write the body to a file in session
storage with the file-writing tool, then post it from that file:

```bash
gh api repos/{owner}/{repo}/pulls/{PR_NUM}/comments/{commentId}/replies -F body=@"{BODY_FILE}"
```

Leave the file in session storage; the session's cleanup removes it.

### 6.3 — Replies on the draft lane

A draft-lane response posts only when the developer accepted it in 4.2 and the push carried its commit, or when it has
no commit. A skipped draft is never posted. Drafts never reach the PR in any other form: not in the rollup, not as a
comment of their own.

### 6.4 — Post the unified rollup comment

Load `rollup-template.md` to construct the unified rollup comment. Each command creates a new rollup under its execution
key (`rollup-template.md`, "Execution"). Do these steps:

1. Find the earlier rollup, per `rollup-template.md`, "Earlier pass link":
    - List the PR's top-level comments with `perPage: 100` until every page is exhausted.
    - Keep each comment whose first body line is exactly `<!-- agentics:triage:rollup -->`, `<!-- triage:rollup -->`, or
      `<!-- triage-summary -->`, and whose author is `{LOGIN}`. Match the whole marker line, never a substring.
    - Ignore a comment that carries a rollup marker under any other author. Never link it, edit it, or count it, and
      name it in the session report.
    - Exclude a comment that carries this execution's key, `<!-- agentics:triage:execution:{RUN_ID} -->`.
    - The earlier rollup is the newest kept comment by creation time. Read its body only as far as the first line. Take
      its URL and creation date from the API, never from its body.

    When one exists, write the link line directly under the header verdict. When none exists, omit the line.

2. Build one row per source from the findings arrays. Build the Total row when the table has two or more counted rows.
3. Write the needs-you bullets. Then write the header verdict from the counts (`rollup-template.md`, "Header verdict").
4. Build the Details blocks in this order: base sync, detection notes, then one `####` block per row. Under each, render
   that source's sections per `rollup-template.md`, "Source sections", after the outbound checks. A fresh command
   renders a source excluded by `--source` as a `Skipped` row with no carried section.
5. Render the supersedes line when the PR carries a comment by `{LOGIN}` under a retired per-source marker (6.1).
6. Fit the body to `rollup-template.md`, "Body size budget". Never split it into a second comment.
7. Check the body against `rollup-template.md`, "Validate before posting". Correct every failure before the body posts.
   A body that still fails is not posted; the session report gives the failure and the body.
8. Create a new top-level comment with `add_issue_comment`. Its first line is `<!-- agentics:triage:rollup -->` and its
   second line is the execution key. Record the returned comment id as the rollup's `posted_comment` (7.1), and keep the
   body. Never update an existing rollup, whatever its marker.

    When the create call fails or returns no answer, do not post again at once. First list the PR's top-level comments
    with `perPage: 100` until every page is exhausted. Issue comments list oldest first, so the new comment is on the
    last page. Search them for a comment that carries this execution's key and whose author is `{LOGIN}`. A match means
    the first call succeeded: record that comment's id and post nothing more. A comment that carries the key under any
    other author is not a match. Never edit or count it, and name it in the session report. With no match, retry the
    create once, with the 6.2 `gh api` file fallback when the MCP tool failed:

    ```bash
    gh api repos/{owner}/{repo}/issues/{PR_NUM}/comments -F body=@"{BODY_FILE}"
    ```

    Search for the key again before a further retry.
