# GitHub workflows and mode procedures

Detailed procedures, command examples, and safety probes referenced by [SKILL.md](../SKILL.md).

## Workflow index

| Workflow                | Use it to                                                                                         | Takes in, produces                                                                                                          | Where                                                                                                                                                              |
| ----------------------- | ------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Issue management        | Create, update, or query issues with the MCP tools.                                               | In: the action, the repo, and the issue content. Out: the query result or the issue URL.                                    | [Issue management workflow](#issue-management-workflow)                                                                                                            |
| Pull request creation   | Create a draft PR, or edit or retarget an open PR, with linkage, template, body, and base checks. | In: the branch and its change set. Out: a draft PR, or a stack that GitHub confirms when the base is an open PR's head.     | [Pull request creation workflow details](#pull-request-creation-workflow-details)                                                                                  |
| PR check failures       | Read the failing checks of a PR and draft a comment about them.                                   | In: a PR. Out: a draft comment, posted only after the user confirms.                                                        | [PR check-failure workflow](#pr-check-failure-workflow)                                                                                                            |
| Displayed-issue handoff | Hand a displayed issue to `routing` after implementation language.                                | In: the displayed issue. Out: a handoff to `routing` with the verified issue facts.                                         | [Continuing from a displayed issue](#continuing-from-a-displayed-issue)                                                                                            |
| Actions                 | Trigger, re-run, cancel, or watch workflow runs.                                                  | In: the workflow and the ref, or a run id. Out: a triggered, re-run, cancelled, or watched run.                             | [flag-tables.md](flag-tables.md#running-github-actions)                                                                                                            |
| CLI fallback            | Create a PR or manage issues when MCP is unavailable.                                             | In: the same inputs as the MCP path. Out: a draft PR, or an issue with its type and project set.                            | [Creating a pull request (when MCP is unavailable)](#creating-a-pull-request-when-mcp-is-unavailable); [Managing issues](#managing-issues-when-mcp-is-unavailable) |
| Branch deletion         | Delete a branch through the REST ref endpoint.                                                    | In: the repo and the exact branch name. Out: a draft of the probe findings. The deletion runs only after the user confirms. | [Deleting a branch](#deleting-a-branch)                                                                                                                            |
| Direct API access       | Script issue-type changes, or query custom issue fields when no MCP server is configured.         | In: the REST or GraphQL call. Out: the API response.                                                                        | [flag-tables.md](flag-tables.md#direct-api-access)                                                                                                                 |
| Stack mode              | Plan a stack, build a new one, adopt existing PRs, or recover a stack.                            | In: a dependent chain of changes or existing PRs. Out: draft PRs in a stack that GitHub confirms.                           | [Stack mode verification](#stack-mode-verification), [stack.md](stack.md)                                                                                          |

## Bundled writes preview example

Group writes of the same kind in one preview. When a request carries several writes that need confirmation, show every
proposed action in that preview and offer an answer for each. Use the
[shared question format](../../routing/references/contracts/question-format.md) when the actions are separate decisions.
A reply authorizes only the actions it covers. For a destructive action, the reply must name the action or its target.

```text
❓ **Q1** - **Close the stale flag issue**: Close issue #482 as completed? It is open and has no linked PR.

➡️ Close #482.

❓ **Q2** - **Delete the merged branch**: Delete feat/flag-sweep? It is merged into main at a1b2c3d, with no open PR or stack entry.

➡️ Delete feat/flag-sweep.

Answer each action you want to confirm. For example, `close #482 and delete feat/flag-sweep`. A bare `both` or `yes` does not confirm the delete.
```

## Continuing from a displayed issue

- If the user follows an issue read with implementation language ("implement it", "do it", "let's work on this"), hand
  the issue to `routing` ([Lifecycle phases](../../routing/SKILL.md#lifecycle-phases)) under the
  [forwarding rule](../../routing/references/forwarding.md#explicit-implementation-request). Announce the handoff and
  proceed without another confirmation.
- If exactly one issue was displayed and the follow-up clearly refers to it, keep that target without a second question.
  If several issues are equally plausible, ask one decision question naming each candidate issue, and start no dependent
  work until the user picks one. Never pick one silently.
- Pass the verified repository, issue number, title, URL, body or scope evidence, and references to existing work
  artifacts. Preserve existing intent, spec, plan, and approval evidence instead of restarting the chain. Fetch missing
  target facts rather than inventing them. The handoff carries context, not new authorization for remote writes.
- Reading, summarizing, labeling, or organizing issues is not implementation by itself. A read-only follow-up answers
  without entering the mutating workflow; administrative writes still follow
  [Cross-cutting write safety](../SKILL.md#cross-cutting-write-safety).

## Issue management workflow

1. **Determine action.** Create, update, or query.
2. **Gather context.** Get repository info and milestones. **List the existing labels first** (`gh label list` or the
   MCP equivalent). Label names are exact-match and vary by repo, such as `priority:p1` rather than `P1`. Never guess
   them.
3. **Search for duplicates** before you create. If the search finds a match:
    - Surface the match.
    - Ask, as one ❓ question, whether to link to it, comment on it, or create anyway.
    - Give a ➡️ recommendation based on how closely the match describes the same work.

    Never create silently past a match.

4. **Structure content.** Use the appropriate issue template ([issues/templates.md](issues/templates.md)).
5. **Execute.** Use MCP tools for both reads and writes ([github-mcp-toolsets.md](github-mcp-toolsets.md)).
6. **Confirm.** Report the issue URL back to the user after the write is verified.

## Pull request creation workflow details

Detailed mechanics for items 2–8 of [Pull request creation workflow](../SKILL.md#pull-request-creation-workflow):

- **Issue linkage (item 2).** Detect candidates deterministically from commit messages, touched plan files (`*-plan.md`,
  or `plan.md` in a monthly work folder `YYYY-MM/<purpose>-<topic>/`), and the branch name, rather than conversation
  context. Validate each against the issues API. Apply detected and validated issues and state them in the one-line
  statement before creation, or in the preview (not confirmed separately). When nothing is detected and the user has not
  said how to link, ask once as one ❓ question whether to attach, create, or proceed unlinked (➡️ recommendation:
  "proceed unlinked"). When the user already said how to link ("I'll link it later", "no issue"), ask nothing and record
  the PR unlinked (in the template's own linkage section when it has one). `Closes #N` is reserved for issues the PR
  fully resolves; parents and partially-addressed issues get a plain `Part of #N`. Mechanics:
  [pull-requests/linkage.md](pull-requests/linkage.md).
- **Merge settings (item 3).** With a working `gh`, run
  `gh api repos/{owner}/{repo} --jq '{squash: .allow_squash_merge, message: .squash_merge_commit_message}'` (no GitHub
  MCP tool returns these settings). If `gh` or its auth is unavailable, say the settings are unread and continue. When
  squash is the merge method and `squash_merge_commit_message` is `PR_BODY`, the PR description becomes the squash
  commit body, so it must state what changed and why.
- **Template precedence (item 4).** Use the PR template resolved by
  [repo-conventions.md](../../routing/references/repo-conventions.md) (pointer, locations, `PULL_REQUEST_TEMPLATE/`
  directory). Ask one ❓ question with a ➡️ recommendation if several templates match; use the fallback in
  [pull-requests/body.md](pull-requests/body.md) if none exists.
- **Body (item 5).** Apply [pull-requests/body.md](pull-requests/body.md) on create (`create_pull_request` with
  `draft: true`, `gh pr create --draft`; ready only per [Repository policy](../SKILL.md#repository-policy)), stack
  layers, and edit (`update_pull_request`, `update_pull_request_body`, `gh pr edit --body-file`). Pass any body longer
  than one line through `--body-file`.
- **Refresh on scope change (item 6).** When new commits or a review decision change what an open PR does, rewrite its
  body from the final diff per [pull-requests/body.md](pull-requests/body.md#refresh). Editing a published body follows
  [Cross-cutting write safety](../SKILL.md#cross-cutting-write-safety): "Update the PR" names the edit; a `gate`
  [Option 2](../../gate/references/branch/propose-and-execute.md#option-2-push-and-create-a-draft-pr) choice authorizes
  the PR it creates; the user's stack request or one stack-offer confirmation covers its layer bodies. Target
  verification still runs.
- **Non-trunk base check (item 7).** Applies when creating or retargeting a PR onto a non-default branch
  (`--base "$BASE"` or MCP `base`). Read whether `$BASE` is the head of another open PR in `{owner}/{repo}`:
    - `gh api -X GET repos/{owner}/{repo}/pulls -f state=open -f head="{owner}:$BASE" --jq length` (PowerShell:
      `-f "head={owner}:$Base"`)
    - MCP `list_pull_requests` with `state: open` and `head: <owner>:<base>`

    Never render the branch name into shell source when setting `BASE` (`$Base`): capture it from git or from a file
    written with the file-write tool and pass only the quoted variable.

    - **No such PR:** ordinary PR against that base (draft when new). No stack, no `link`, no `gh-stack` check or
      install offer, and no "base-chained, not a stack" report. For another author's PR retarget, follow write safety
      and state it is another author's.
    - **Lookup cannot run:** say in one clause that the base was not checked against open PRs, proceed as an ordinary PR
      (draft when new), and never call the result a stack.
    - **Such a PR exists:** create or retarget as asked (draft when new), and say it is base-chained and not a stack. If
      the request named the link or stack, run it with no question; otherwise offer the link as one question. Either
      way, run the readiness read and state the cascade cost ([stack.md](stack.md#rebase-ripple)) in the offer or one
      line before a named link. Check `gh extension list` for `github/gh-stack` (offer
      `gh extension install github/gh-stack` if missing), then:
        1. **Keep the PR** as created or retargeted.
        2. **Link.** One new branch on an existing PR: `gh stack link --base <trunk> <lower-pr> <new-branch>`. Lower PR
           already in stack N: `gh stack link N <new-branch>`. Hand each branch to `git push` first so `link` only
           links. Confirm draft state with `gh pr view <n> --json isDraft` (`--open` marks PRs ready and follows
           [Repository policy](../SKILL.md#repository-policy)).
        3. **Verify** via
           [Definition of done: GitHub confirms the stack](../SKILL.md#definition-of-done-github-confirms-the-stack).
           Report a stack only after confirmation.

        On exit 9 (stacked PRs disabled on the repo): report "base-chained, not a stack: stacked PRs aren't enabled
        here", state the null `stack` result, and never call it a stack. When the CLI is unavailable, tell the user to
        use **Create stack** or **Add to stack** in the web UI.

- **Base resolution (item 8).** Resolve in the order of
  [repo-conventions.md](../../routing/references/repo-conventions.md): named/work-item base -> base of the branch's open
  PR -> default branch of the base remote. State the resolved base; ask only when none resolves.

## PR check-failure workflow

1. **Read the PR.** Call `pull_request_read` (method: `get`) for title, state, base, and head SHA.
2. **List check runs.** Call `pull_request_read` (method: `get_check_runs`). Record each failing run's exact **name**,
   **id**, and **details URL**.
3. **Fetch annotations for each failing run.** Neither `get_status` nor `get_check_runs` returns annotations, and no MCP
   tool returns them at all. Use the standing Checks API exception below, one call per failing run id (off `github.com`,
   pass `--hostname {host}`):

```bash
# bash: annotations for one failing check run
gh api /repos/{owner}/{repo}/check-runs/{check_run_id}/annotations --paginate
```

```powershell
# PowerShell equivalent (gh CLI syntax is identical across shells)
gh api /repos/{owner}/{repo}/check-runs/{check_run_id}/annotations --paginate
```

4. **Fall back to logs when a run has no annotations.** Use `get_job_logs` for the failing Actions job rather than
   guessing the cause.
5. **Summarize.** Give the summary in the session with exact check names, run ids, links, and the annotation's
   `file:line` message text. Quote what the tooling reported; never invent, merge, or paraphrase a failure.
6. **Comment only on request.** A read-only request ends at the summary. When asked for a comment, draft it from the
   summary by [posting-envelope.md](posting-envelope.md) and post via `add_issue_comment` when named or confirmed. Never
   report a session summary as posted.

`/agentics:review` or `/agentics:diagnose` analyzes _why_ the code failed; this workflow reports what CI said.

## Creating a pull request (when MCP is unavailable)

Follow [Pull request creation workflow](../SKILL.md#pull-request-creation-workflow) first. Resolve issue linkage and
write the body per [pull-requests/body.md](pull-requests/body.md). Save the title with the file-write tool. Obtain
`$title_file`, `$body_file`, and `$base_branch` from their producers or scratch records, using the same base linkage
detection scanned. `--draft` follows [Repository policy](../SKILL.md#repository-policy).

```bash
pr_title=$(cat -- "$title_file") || exit 1
gh pr create --draft --title "$pr_title" \
  --body-file "$body_file" --base "$base_branch"
```

## Managing issues (when MCP is unavailable)

A label is **not** a substitute for a real issue type or project field. Priority has no built-in field, so set it as a
label.

1. **Resolve `{owner}/{repo}`.** Pass it to every command below (from git remote via
   `gh repo view --json nameWithOwner -q .nameWithOwner` when unnamed; state before any write; off `github.com`, pass
   `--repo {host}/{owner}/{repo}`). Without `--repo`, commands silently target the current checkout.
2. **Verify label names.** Run `gh label list --repo {owner}/{repo}`. If no existing label matches, omit `--label` and
   say so in the report; never guess or invent a label.
3. **Search for duplicates before creating.** Run `gh issue list --repo {owner}/{repo} --search "<terms>"`. Handle hits
   identically to MCP step 3.
4. **Structure content with the appropriate issue template.** Use [issues/templates.md](issues/templates.md); an ad-hoc
   body is a regression. Ambiguous "missing X" defaults to the Task template with `--type Task` (or `Chore` if defined);
   pick Bug only for a clear defect, Feature only for a clear new capability.
5. **Create with native `--type` and `-p/--project`.** (`gh issue edit` uses `--type`,
   `--add-project`/`--remove-project`; see [issues/issue-types.md](issues/issue-types.md) and
   [projects.md](projects.md).) Write title and body with the file-write tool, load their locations as `$title_file` and
   `$body_file`, and pass resolved repository and verified label as separate arguments:

```bash
issue_title=$(cat -- "$title_file") || exit 1
gh issue create --repo "$target_repo" --title "$issue_title" --body-file "$body_file" --label "$priority_label" --assignee "@me" --type Bug --project "Roadmap"
```

6. **List and verify.** Run `gh issue list --repo {owner}/{repo} --assignee "@me" --label "bug" --state open`.
7. **Close when done.** On request, run `gh issue close "$number" --repo "$target_repo" --reason completed`, then read
   `gh issue view "$number" --repo "$target_repo" --json state,stateReason` and report closed only when the read shows
   `CLOSED` and `COMPLETED`.

## Deleting a branch

No MCP tool deletes a branch. The REST ref endpoint is irreversible from gh's side. Run
[Cross-cutting write safety](../SKILL.md#cross-cutting-write-safety) first:

1. Resolve `{owner}/{repo}` and the exact branch name. Unnamed destructive targets ("delete the branch" or "close that
   issue" with no concrete name) are not targets: resolve to one specific name or number, never infer from the current
   checkout, the most recent command, or the last thing discussed, and state the resolved target back with its verified
   state as part of the single point-of-action confirmation in step 4 rather than as a separate question. For a branch
   named only by its issue, inspect `gh issue develop --list <issue>` and the head of its pull request; ask if
   ambiguous.
2. Run the applicable probes below against the resolved host and repository to establish whether the branch is merged
   **into its own base** (the PR's `baseRefName`, or default branch if none). Capture `$target_repo`, `$api_repo`,
   `$forge_host`, `$branch_name`, `$owner`, `$repo_name`, numeric `$pr_number`, and URL-encoded `$base_segment` /
   `$branch_segment` from producers or scratch files.
3. Present findings as the preview (resolved target and what will be lost).
4. Take one confirmation at the point of action before `DELETE`; the reply must name the action or the branch (bare
   "yes" never confirms).

Probe order in both shells: (1) default branch (never delete it); (2) branch protection (`true` stops); (3) all PRs for
the head (`baseRefName`, `headRefOid`); (4) branch tip SHA; (5) whether `github/gh-stack` is installed; (6)
`gh stack view --json` only when installed and the branch is in the current checkout's repo; (7) GraphQL `stack` field
on the branch's open PR in every other case or when step 6 omits the branch; (8) `ahead_by` against base; (9) `DELETE`
only after confirmation.

```bash
gh repo view "$target_repo" --json defaultBranchRef -q .defaultBranchRef.name
gh api --hostname "$forge_host" "$api_repo/branches/$branch_segment" --jq .protected
gh pr list --repo "$target_repo" --head "$branch_name" --state all --json number,state,title,baseRefName,headRefOid
gh api --hostname "$forge_host" "$api_repo/git/ref/heads/$branch_segment" --jq .object.sha
gh extension list
gh stack view --json
gh api graphql --hostname "$forge_host" -f owner="$owner" -f name="$repo_name" -F number="$pr_number" -f query='query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) {
  pullRequest(number: $number) {
    stack { number entries(first: 20) { totalCount nodes { pullRequest { number } } } } } } }'
gh api --hostname "$forge_host" "$api_repo/compare/${base_segment}...${branch_segment}" --jq .ahead_by
```

A default or protected branch ends the operation: report it and do not offer `DELETE`. After confirmation in step 9,
reload the confirmed target data and run
`gh api --hostname "$forge_host" "$api_repo/git/refs/heads/$branch_segment" -X DELETE`.

- **Merged test:** merged only when a PR for that head has state `MERGED` and branch tip equals `headRefOid`; otherwise
  `ahead_by == 0` against base is the test. An open PR or non-null GraphQL `stack` is a finding in the preview
  regardless of prior merges — never delete around it.
- **Unmerged (`ahead_by > 0`):** state it in the preview, ask one ❓ confirmation (delete anyway or keep; reply must
  name the delete, not just the branch), and give a ➡️ recommendation to keep until commits land elsewhere.
- **Mismatch:** merge state is read, not assumed. If the read differs from what the user said, report once per write
  safety.

## Stack mode verification

Verify any stack through the GraphQL `stack` field on one member PR, checking `totalCount` and entry PR numbers against
the intended chain (a null `stack` means no stack object exists; return to `link`/`submit`):

```bash
# bash: a null stack means no stack object exists; go back to link/submit
gh api graphql -F owner='{owner}' -F name='{repo}' -F number=<n> -f query='query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) {
  pullRequest(number: $number) {
    stack { number entries(first: 20) { totalCount nodes { pullRequest { number } } } } } } }'
```

```powershell
# PowerShell equivalent (gh CLI syntax is identical across shells)
gh api graphql -F owner='{owner}' -F name='{repo}' -F number=<n> -f query='query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) {
  pullRequest(number: $number) {
    stack { number entries(first: 20) { totalCount nodes { pullRequest { number } } } } } } }'
```
