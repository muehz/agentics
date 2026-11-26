---
name: gh
description: "Use when work requires operating GitHub: managing issues or pull requests, setting or changing a pull request's base branch, inspecting Actions, or handling releases and repository settings. Follow repository conventions and verify the resulting GitHub state. Local commits and branches belong to git."
version: "1.31.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[operation or issue title/number] | stack <cmd>"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [stack]
---

# Working with GitHub

Follow [output-discipline.md](../routing/references/contracts/output-discipline.md) (`/agentics:<skill>`, `$agentics:<skill>` on Codex).

## Don't use when

| Task                                                                    | Kind               | Use this instead        | Why                                                                  |
| ----------------------------------------------------------------------- | ------------------ | ----------------------- | -------------------------------------------------------------------- |
| Local commit / worktree / branch operations                             | `cannot`           | `/agentics:git`         | Local repo state belongs to git.                                     |
| Resolving merge/rebase conflicts on a PR branch                         | `cannot`           | `/agentics:git resolve` | Local edit; gh surfaces conflict and passes head/base.               |
| Deep code review on a PR diff                                           | `cannot`           | `/agentics:review`      | gh review tools are administrative, not diff analysis.               |
| Multi-source PR signal aggregation (bot + checks + human)               | `better elsewhere` | `/agentics:triage`      | One coordinated pass avoids duplicate per-source pushes.             |
| Independent parallel work not yet on pushed branches                    | `cannot`           | `routing`               | Routing dispatches parallel reads and routes implementation.         |
| Local branch mechanics past `gh stack` (cherry-pick, rebase, worktrees) | `cannot`           | `/agentics:git`         | Stack cascades follow [stack.md](references/stack.md#rebase-ripple). |

**Handing off means forwarding, not doing.** Finish gh's own reads, perform none of the deferred analysis/writes, and forward per [forwarding.md](../routing/references/forwarding.md):

- **Auto-forward** targets marked A in [target classification](../routing/references/forwarding.md#target-classification): announce safety scope in one line, forward, and pass fetched data. A forward to `/agentics:review` uses a local mode (`feedback`, `interactive`, or `axes`, never `post`) and forwards immediately when the PR is already identified in context.
- **Offer** targets marked as an offer: ask one yes/no naming side-effect scope, forward only on yes. Under explicit `/agentics:gh`, `better elsewhere` notes the owner in one line and runs gh.
- Forward at most once per invocation; if bounced back, stop and explain.

## Tool priority

Prefer **GitHub MCP server tools** first; fall back to **`gh` CLI** for scripting, `gh run watch`, check-run annotations, auth, `gh api`, branch deletion, and uncovered operations ([references/flag-tables.md](references/flag-tables.md#tool-coverage)). **"Review" here is administrative** (submit, approve, request changes, dismiss, list threads); `/agentics:review` owns diff analysis.

## Cross-cutting write safety

Any remote/public mutation (create, update, close, post, delete, labels, milestones, projects, PRs, branches, releases, raw `gh api`) on MCP or CLI is named in the request or confirmed once via **draft-and-confirm**. Never bypass confirmation via `gh api`; report URLs/changed status only after verified execution ([output-discipline.md](../routing/references/contracts/output-discipline.md#executed-vs-proposed-status-honesty)). Merging follows [Repository policy](#repository-policy).

- **Named steps** run with no second question when verified target matches (except named posts previewed first per [approval.md](../routing/references/contracts/approval.md#a-named-post-a-skill-previews-first), destructive deletes, and policy-gated actions). State up front in one line which named steps are authorized and which will still be asked.
- **Unnamed writes:** show one preview and confirm once (any clear affirmative confirms; affirmative with changes is an edit request).
- **Verify target first.** Read the issue, PR, or branch before writing even if pre-authorized or told not to inspect. On mismatch (number, title, repo, closed/merged, unmerged branch), state observed state once and ask; authorization never carries over.
- **Destructive deletes verify safety and identity.** Resolve `{owner}/{repo}` and verify identity before deleting a branch, release, tag, or comment. Branch deletes run all probes: default branch, branch protection, all PRs for head, tip SHA, `gh-stack` / GraphQL `stack` membership, and `ahead_by` vs base. Include every finding in the preview and require one point-of-action confirmation naming the action or target (bare "yes" never confirms).
- **Unnamed destructive targets resolve first.** "Delete the branch" / "close that issue" without a concrete name is not a target. Resolve to one concrete name/number and state it with verified state inside the point-of-action confirmation (never infer from checkout, last command, or last discussion). For a branch named only by issue, inspect `gh issue develop --list <issue>` and the PR head; if missing or ambiguous, stop and ask the user to name it.
- **Bundled writes & envelope.** Accept user statements about their own work without proof. Decision questions follow [question-format.md](../routing/references/contracts/question-format.md). Bundle multiple confirmations into one preview with per-action answers ([references/mode-procedures.md](references/mode-procedures.md#bundled-writes-preview-example)). Posted comments/issues follow [references/posting-envelope.md](references/posting-envelope.md).

### Repository policy

Single source for drafts, merging, review threads, and security alerts:

- **Rules:** PRs open as drafts; no agent marks ready, merges, resolves/reopens review threads, or dismisses security alerts while the governing setting is on.
- **Settings:** `policy.draft_prs` (drafts, ready, merge), `policy.block_review_thread_changes` (threads), `policy.block_security_alert_dismissal` (alerts). Read read-only from `.agentics/hooks.json` (repo overrides `~/.agentics/hooks.json`; unset/unreadable/invalid keeps all on). Never edit either file ([references/flag-tables.md](references/flag-tables.md#repository-policy-settings)).
- **While on:** hold regardless of session instructions; name setting once and note UI/setting exit (repeat -> one-line pointer; no override recorded).
- **When `policy.draft_prs` is off:** open/mark ready on named request; merge only on named request after verifying target, review decision, and checks.
- **Hosts without draft PRs:** hand push to `git push`, state drafts unavailable, give create command; open ready only when `policy.draft_prs` is off.
- **Returning a PR to draft:** ungoverned by policy. On named request only, check current `isDraft` via `gh pr view <number> --json isDraft` (stop if already draft), run `gh pr ready <number> --undo`, and re-read `isDraft` before reporting it as draft.

## Continuing from a displayed issue

When implementation language ("implement it", "let's work on this") follows an issue read, load [forwarding.md](../routing/references/forwarding.md) and announce and hand off to `routing` (Lifecycle phases) under [explicit implementation request](../routing/references/forwarding.md#explicit-implementation-request) with verified issue facts and existing artifacts (context only, not remote-write authorization). With a verified approved plan, hand off directly so `routing` dispatches `execute` without restarting the chain or re-asking. If an existing plan's approval is unreadable/unconfirmed, stop and ask once. An earlier `/agentics:gh` invocation does not carry over. If several displayed issues fit, ask one decision question first. Read-only follow-ups stay here. Details: [references/mode-procedures.md](references/mode-procedures.md#continuing-from-a-displayed-issue).

## Part 1: GitHub MCP server (primary)

Tool catalog: [references/github-mcp-server.md](references/github-mcp-server.md). Toolsets: [references/github-mcp-toolsets.md](references/github-mcp-toolsets.md).

> **Preflight for every workflow below.** Resolve forge host ([repo-conventions.md](../routing/references/repo-conventions.md)) and `{owner}/{repo}` (conversation or `gh repo view --json nameWithOwner -q .nameWithOwner`; state repo before acting; never assume last repo).
>
> - **Host syntax & support:** valid DNS name (letters/digits/inner hyphens, optional `:<port>`; reject whitespace, `;`, `$`, quotes, backticks, `/`). Supported: `github.com` and hosts known to `gh auth status --hostname '<host>'`. Unsupported: say `host not supported: <host>` once, name `gh auth login --hostname <host>`, finish local work, run no `gh` command.
> - **Host arguments & MCP:** off `github.com`, pass `--hostname {host}` on `gh api` and `--repo {host}/{owner}/{repo}` on `--repo` commands; use Part 2 CLI unless user confirms session MCP targets that host ([references/flag-tables.md](references/flag-tables.md#host-and-repository-flags)).
> - **MCP fallback:** if MCP tools are absent/fail, name the failure and switch to Part 2 CLI. Never retry in a loop, fabricate results, or drop issue type, priority, or project fields.

### Issue management workflow

Determine action -> list existing labels (exact-match, never guess) -> duplicate search before creating (on hit, ask one ❓/➡️ question; never create past a match) -> apply template ([references/issues/templates.md](references/issues/templates.md)) -> execute via MCP -> verify and report URL. Details: [references/mode-procedures.md](references/mode-procedures.md#issue-management-workflow).

### Pull request creation workflow

`gh` owns PR configuration across Agentics (`gate`, `ops`, etc.). Load [references/mode-procedures.md](references/mode-procedures.md#pull-request-creation-workflow-details), [references/pull-requests/body.md](references/pull-requests/body.md), [references/pull-requests/linkage.md](references/pull-requests/linkage.md), and [references/flag-tables.md](references/flag-tables.md) before drafting/creating (plus [references/images.md](references/images.md) for screenshots). At most one confirmation: named PR states linkage, template, and base in one line and creates; unnamed PR previews those three facts and confirms once.

1. **Draft by default** per [Repository policy](#repository-policy).
2. **Issue linkage:** detect from commits, touched plans, and branch name; validate via issues API; apply without extra confirmation. If none detected and no preference given, ask one ❓ question (➡️ "proceed unlinked"). Use `Closes #N` only for full resolution, else `Part of #N` ([references/pull-requests/linkage.md](references/pull-requests/linkage.md)).
3. **Merge settings:** check `allow_squash_merge` and `squash_merge_commit_message` via `gh api` (note if unread); write body assuming `PR_BODY`.
4. **Template precedence:** resolve via [repo-conventions.md](../routing/references/repo-conventions.md) (ask one ❓ if multiple match; fallback in body contract if none).
5. **Body:** write complete, diff-grounded Summary, Evidence, and Impact per [references/pull-requests/body.md](references/pull-requests/body.md) and [output-discipline.md](../routing/references/contracts/output-discipline.md) on create, stack layers, and edit. For UI screenshots, follow [references/images.md](references/images.md). Pass multi-line bodies via `--body-file`. Draft-only asks return text without creating. Report only the returned PR number/URL.
6. **Refresh on scope change:** rewrite open PR bodies from final diff per [references/pull-requests/body.md](references/pull-requests/body.md#refresh) under write safety (`gate` [Option 2](../gate/references/branch/propose-and-execute.md#option-2-push-and-create-a-draft-pr) and stack confirmation authorize their created/layer bodies; still verify target).
7. **Non-trunk base: stack only when base is another open PR's head.** Check `gh api .../pulls -f state=open -f head="{owner}:$BASE"` or MCP `list_pull_requests` (capture `$BASE` safely, never interpolate into shell source). No matching open PR -> ordinary PR (no stack offer). Lookup unavailable -> state base unchecked, ordinary PR, never call it a stack. Matching open PR -> create/retarget as base-chained (not a stack); if stack/link named, run with no question, else offer once with exact `gh stack link --base <trunk> <parent-PR> <new-PR>` and cascade cost ([stack.md](references/stack.md#rebase-ripple)), ensure `github/gh-stack`, push via `git push`, `gh stack link`, and verify via [Definition of done: GitHub confirms the stack](#definition-of-done-github-confirms-the-stack) (exit 9 = stacked PRs disabled, not a stack).
8. **Base:** resolve via [repo-conventions.md](../routing/references/repo-conventions.md) (named/work-item -> open PR base -> default branch); state it, ask only if unresolved.

### PR check-failure workflow

Read PR (`pull_request_read` `get`) -> `get_check_runs` -> fetch annotations via CLI Checks API (`gh api /repos/{owner}/{repo}/check-runs/{check_run_id}/annotations --paginate`, `--hostname {host}` off `github.com`) -> fall back to `get_job_logs` if empty -> summarize with exact names, ids, links, and verbatim `file:line` text (or state that annotations are missing and quote the logs). Draft/post via [references/posting-envelope.md](references/posting-envelope.md) only on request. Commands: [references/mode-procedures.md](references/mode-procedures.md#pr-check-failure-workflow).

## Part 2: GitHub CLI (fallback)

`gh` CLI syntax matches across Bash and PowerShell. Load the matching section before running fallback commands:

- **GitHub Actions** (triggers, re-runs, cancels, `gh run watch`): [references/flag-tables.md](references/flag-tables.md#running-github-actions)
- **PR creation fallback** (follows PR workflow, `--draft`, file-written title/body): [references/mode-procedures.md](references/mode-procedures.md#creating-a-pull-request-when-mcp-is-unavailable)
- **Issue fallback** (`--repo`, exact labels, duplicate search, templates, `--type`/`-p`, verified close): [references/mode-procedures.md](references/mode-procedures.md#managing-issues-when-mcp-is-unavailable)
- **Branch delete** (CLI-only; full safety/merge/protection/stack probes, preview, point-of-action confirm; stops on default/protected): [references/mode-procedures.md](references/mode-procedures.md#deleting-a-branch)
- **Direct API access** (`gh api` REST/GraphQL with `--hostname` off `github.com`): [references/flag-tables.md](references/flag-tables.md#direct-api-access)

## Part 3: Stacked PRs (`stack` mode)

`gh stack` (`github/gh-stack`) turns dependent branches into dependent PRs; every stack path ends at item 7 of [Pull request creation workflow](#pull-request-creation-workflow). `gh` never runs `git push` (hand pushes to `git push` first) and **never** runs `gh stack push` or `gh stack sync` (both force-push).

- **Stack when:** dependent, independently reviewable layers (schema -> API -> UI), oversized PR, or ordered slices with "Blocked by" edges.
- **Don't stack when:** tasks are independent/parallelizable — announce and hand the **whole batch** to `routing` ([target classification](../routing/references/forwarding.md#target-classification)) rather than opening PRs here; or when one cohesive PR fits or layers cannot be reviewed alone.

### Stack workflows

Load [references/stack.md](references/stack.md) and follow [new-stack](references/stack.md#stack-workflow-new-stack) or [adoption](references/stack.md#adoption-workflow-existing-prs) before creating, pushing, retargeting, or linking any layer; when the request names the stack with unambiguous order and clean readiness (or after confirming the offer once), state order, layer body rewrites, and cascade cost in one line and proceed without a second confirmation for those layer body edits (still preview/confirm unrequested edits). Flags: [references/flag-tables.md](references/flag-tables.md#stack-mode-commands-and-flags).

### Definition of done: GitHub confirms the stack

**A stack exists only when GitHub confirms it.** Base-chained PRs in `gh pr list` are not a stack. After every `submit` (including `--open` when `policy.draft_prs` is off) or `link`, show and verify the GraphQL `stack` query on one member PR (`totalCount` and entry PR numbers match; null means no stack). Commands: [references/mode-procedures.md](references/mode-procedures.md#stack-mode-verification).

### After a base PR merges

Recover the next pushed layer by merging trunk into it and handing push to `git push` without force: [stack.md](references/stack.md#after-a-base-pr-merges).

## References

- **Core:** [mode-procedures](references/mode-procedures.md), [flag-tables](references/flag-tables.md), [github-mcp-server](references/github-mcp-server.md), [github-mcp-toolsets](references/github-mcp-toolsets.md), [posting-envelope](references/posting-envelope.md), [search](references/search.md), [projects](references/projects.md), [images](references/images.md)
- **Issues:** [index](references/issues/README.md), [issue-fields](references/issues/issue-fields.md), [issue-types](references/issues/issue-types.md), [templates](references/issues/templates.md), [sub-issues](references/issues/sub-issues.md), [dependencies](references/issues/dependencies.md)
- **Pull requests & stacks:** [linkage](references/pull-requests/linkage.md), [body](references/pull-requests/body.md), [stack](references/stack.md)
- **Contracts:** [forwarding](../routing/references/forwarding.md), [output-discipline](../routing/references/contracts/output-discipline.md)
