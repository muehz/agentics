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

Skill commands here use `/agentics:<skill>` (Claude Code, Copilot CLI, VS Code Chat); on Codex, type `$agentics:<skill>`.

## Don't use when

| Task                                                                                              | Kind               | Use this skill instead  | Why                                                                                                                                                                                 |
| ------------------------------------------------------------------------------------------------- | ------------------ | ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local commit / worktree / branch operations                                                       | `cannot`           | `/agentics:git`         | gh is remote GitHub. git is local repo state.                                                                                                                                       |
| Resolving merge/rebase conflicts on a PR branch                                                   | `cannot`           | `/agentics:git resolve` | Conflict resolution is a local working-tree edit. gh surfaces the conflict and passes the pull request's head and base to `/agentics:git resolve`.                                  |
| Deep code review on a PR diff                                                                     | `cannot`           | `/agentics:review`      | Review owns diff analysis. The review tools of gh are administrative and do not judge the code.                                                                                     |
| Multi-source PR signal aggregation (bot + checks + human)                                         | `better elsewhere` | `/agentics:triage`      | Triage makes one coordinated pass. Serial independent gh calls risk duplicate or conflicting per-source pushes.                                                                     |
| Independent parallel work that is not yet on pushed branches                                      | `cannot`           | `routing`               | gh does not implement or commit. Routing dispatches read-only work in parallel itself. It sends implementation through the chain. Stacking imposes an order the work does not need. |
| Local branch mechanics beyond what `gh stack` covers (cherry-pick, interactive rebase, worktrees) | `cannot`           | `/agentics:git`         | Stack cascades follow [stack.md](references/stack.md#rebase-ripple). Anything past that is plain git.                                                                               |

**Handing off means forwarding, not doing.** Finish gh's own reads first, perform none of the deferred analysis or writes, and forward per [forwarding.md](../routing/references/forwarding.md):

- **Auto-forward** targets marked A in [target classification](../routing/references/forwarding.md#target-classification): announce safety scope in one line, forward, and pass fetched data. A forward to `/agentics:review` must name a local mode (`feedback`, `interactive`, or `axes`) — never `post`.
- **Offer** targets marked as an offer: ask one yes/no question naming side-effect scope, and forward only on yes. Under an explicit `/agentics:gh` invocation, a `better elsewhere` row prints a one-line note and gh runs.
- Forward at most once per invocation; if bounced back, stop and explain — never loop or silently continue the deferred work under another heading.

## Tool priority

Prefer **GitHub MCP server tools** first (structured operations without a shell); fall back to **`gh` CLI** for scripting, `gh run watch`, check-run annotations, auth, `gh api`, branch deletion, and uncovered operations. Full coverage matrix: [references/flag-tables.md](references/flag-tables.md#tool-coverage).

**"Review" in this skill means the administrative action** (submit, approve, request changes, dismiss, list threads). `/agentics:review` owns diff analysis.

## Cross-cutting write safety

Any mutation of public or remote state (create, update, close, post, delete, labels, milestones, projects, PRs, branches, releases, raw `gh api`) on either MCP or CLI is either named in the user's request or confirmed once via **draft-and-confirm**. Never use `gh api` to bypass confirmation, and report a URL or changed status only after performing and verifying the mutation. Merging follows [Repository policy](#repository-policy).

- **Named steps run with no second question** when the verified target matches what the user named (except a named post previewed first per [approval.md](../routing/references/contracts/approval.md#a-named-post-a-skill-previews-first), destructive deletes, and policy-gated actions). State up front in one line which named steps are authorized and which will still be asked.
- **Unnamed writes get one confirmation:** show a preview and confirm once (any clear affirmative confirms; an affirmative carrying a change is an edit request).
- **Verify the target first.** Always read the issue, PR, or branch before writing, even when pre-authorized or told not to inspect. If real state differs (number, title, repo, already closed/merged, unmerged branch), say what you observed once and ask; authorization never carries over to a mismatched target.
- **Destructive deletes verify safety and identity.** Before deleting a branch, release, tag, or comment, establish whether it is merged into its base, default/protected, referenced by an open PR or active stack, or someone else's unmerged work. Put facts in the preview and take one point-of-action confirmation naming the action or target (a bare "yes" never confirms a delete).
- **Unnamed destructive targets get resolved first.** "Delete the branch" or "close that issue" with no concrete name is not a target. Resolve it to one specific name or number and state it back with its verified state as part of the one confirmation at the point of action, not as a separate question. Never infer it from the current checkout, the most recent command, or the last thing discussed. For a branch named only by its issue, inspect `gh issue develop --list <issue>` and the head of its pull request; ask if ambiguous.
- **Bundled writes & envelope.** Accept the user's statements about their own work without proof. Ask decision questions in the [shared question format](../routing/references/contracts/question-format.md). Combine multiple writes requiring confirmation into one preview with per-action answers ([references/mode-procedures.md](references/mode-procedures.md#bundled-writes-preview-example)). Every posted comment or issue body follows [references/posting-envelope.md](references/posting-envelope.md) (idempotency marker, footer, outbound checks).

### Repository policy

This subsection is the single source of rules for drafts, merging, review threads, and security alerts.

- **Rules:** PRs open as drafts; no agent marks a PR ready, merges a PR, resolves/reopens a review thread, or dismisses a security alert while the governing setting is on.
- **Settings:** `policy.draft_prs` governs drafts, ready, and `gh`'s merge rule; `policy.block_review_thread_changes` governs review threads; `policy.block_security_alert_dismissal` governs security alert dismissal. Read read-only from `.agentics/hooks.json` (repo overrides `~/.agentics/hooks.json`; unset defaults on; unreadable/invalid JSON ignores both and keeps all on). Never edit either file ([references/flag-tables.md](references/flag-tables.md#repository-policy-settings)).
- **While on:** follow the rule regardless of session instructions; name the governing setting once and note the user can change it or click in the UI (repeat gets a one-line pointer; record no override).
- **When `policy.draft_prs` is off:** open ready or mark ready on named request; merge only on named request after verifying target, review decision, and check state.
- **Hosts with no draft PRs:** hand the push to `git push`, state drafts are unavailable, and give the create command; open ready only when `policy.draft_prs` is off.
- **Returning a PR to draft:** ungoverned by policy. Read `gh pr view <number> --json isDraft` first (stop if already draft); on named request only, run `gh pr ready <number> --undo`, re-read `isDraft`, and report draft only when `true`.

## Continuing from a displayed issue

If implementation language ("implement it", "let's work on this") follows an issue read, announce and hand off to `routing` ([Lifecycle phases](../routing/SKILL.md#lifecycle-phases)) under [forwarding.md](../routing/references/forwarding.md#explicit-implementation-request) with verified issue facts and existing work artifacts (context only, not remote-write authorization). If several displayed issues are plausible, ask one decision question first. Read-only follow-ups stay here. Details: [references/mode-procedures.md](references/mode-procedures.md#continuing-from-a-displayed-issue).

## Part 1: GitHub MCP server (primary)

Tool catalog: [references/github-mcp-server.md](references/github-mcp-server.md). Toolsets and mappings: [references/github-mcp-toolsets.md](references/github-mcp-toolsets.md).

> **Preflight for every workflow below.** Resolve the forge host from [repo-conventions.md](../routing/references/repo-conventions.md) together with `{owner}/{repo}` (from conversation or `gh repo view --json nameWithOwner -q .nameWithOwner`; state the resolved repo before acting; never assume the last repo discussed).
>
> - **Host syntax & support:** validate the host as a DNS name (labels of letters, digits, inner hyphens, optional `:<port>`; reject whitespace, `;`, `$`, quotes, backticks, `/`). Supported: `github.com` and any valid host known to `gh auth status --hostname '<host>'`. Unsupported: say `host not supported: <host>` once, name `gh auth login --hostname <host>`, finish local work, and run no `gh` command.
> - **Host arguments & MCP:** off `github.com`, pass `--hostname {host}` on every `gh api` call and `--repo {host}/{owner}/{repo}` on every `--repo` command, and use the Part 2 CLI path unless the user confirms the session's MCP server targets that host ([references/flag-tables.md](references/flag-tables.md#host-and-repository-flags)).
> - **MCP fallback:** if GitHub MCP tools are absent or fail (auth, permissions, missing scope, rate limit, server error), name the failure and switch to Part 2 CLI. Never retry a failing MCP call in a loop, never fabricate results, and never drop issue type, priority, or project fields.

### Issue management workflow

Determine action (create/update/query) -> list existing labels first (exact-match, never guess) -> search for duplicates before creating (on hit, surface and ask one ❓ question with ➡️ recommendation; never create silently past a match) -> apply template ([references/issues/templates.md](references/issues/templates.md)) -> execute via MCP -> verify and report URL. Full procedure: [references/mode-procedures.md](references/mode-procedures.md#issue-management-workflow).

### Pull request creation workflow

`gh` owns pull request configuration for all of Agentics (`gate`, `ops`, and other skills follow this list). Creation uses at most one confirmation: a named PR states resolved linkage, template, and base in one line and creates; an unnamed PR previews those three facts and confirms once. Load [references/mode-procedures.md](references/mode-procedures.md#pull-request-creation-workflow-details) for full mechanics.

1. **Draft by default** on both paths per [Repository policy](#repository-policy).
2. **Issue linkage is resolved, never omitted.** Detect deterministically from branch commits, touched plans, and branch name; validate via issues API; apply without separate confirmation. If none detected and user gave no preference, ask one ❓ question (➡️ "proceed unlinked"). Reserve `Closes #N` for full resolution; use `Part of #N` otherwise ([references/pull-requests/linkage.md](references/pull-requests/linkage.md)).
3. **Merge settings.** Check `allow_squash_merge` and `squash_merge_commit_message` via `gh api` before writing the body; if unread, note and continue. Write the body as though it becomes the squash commit body (`PR_BODY`).
4. **Template precedence.** Use the PR template resolved by [repo-conventions.md](../routing/references/repo-conventions.md) (ask one ❓ question if multiple match; fallback in body contract if none).
5. **Body.** Write per [references/pull-requests/body.md](references/pull-requests/body.md) on create, stack layers, and edit; pass multi-line bodies via `--body-file`.
6. **Refresh on scope change.** Rewrite open PR bodies from the final diff per [references/pull-requests/body.md](references/pull-requests/body.md#refresh) under write safety (`gate` [Option 2](../gate/references/branch/propose-and-execute.md#option-2-push-and-create-a-draft-pr) and stack confirmation authorize their created/layer bodies; still verify target).
7. **Non-trunk base: a stack only when the base is another open PR's head.** Check via `gh api .../pulls -f state=open -f head="{owner}:$BASE"` or MCP `list_pull_requests` (capture `$BASE` safely from git or file, never interpolate into shell source). No matching open PR -> ordinary PR (no stack offer). Lookup unavailable -> state base unchecked, proceed as ordinary PR, never call it a stack. Matching open PR -> create/retarget as base-chained (not a stack); if stack/link was named, run with no question, else offer once; state cascade cost ([stack.md](references/stack.md#rebase-ripple)), ensure `github/gh-stack`, push via `git push`, `gh stack link`, and verify via [Definition of done: GitHub confirms the stack](#definition-of-done-github-confirms-the-stack) (exit 9 = stacked PRs disabled, never call it a stack).
8. **Base.** Resolve per [repo-conventions.md](../routing/references/repo-conventions.md) (named/work-item -> open PR base -> default branch of base remote); state it, ask only if unresolved.

### PR check-failure workflow

Read PR (`pull_request_read` `get`) -> list check runs (`get_check_runs`) -> fetch annotations per failing run via the standing CLI Checks API exception (`gh api /repos/{owner}/{repo}/check-runs/{check_run_id}/annotations --paginate`, passing `--hostname {host}` off `github.com`) -> fall back to `get_job_logs` when annotations are empty -> summarize with exact names, ids, links, and `file:line` quotes. Draft and post a comment via [references/posting-envelope.md](references/posting-envelope.md) only on request. Full commands: [references/mode-procedures.md](references/mode-procedures.md#pr-check-failure-workflow).

## Part 2: GitHub CLI (fallback)

`gh` CLI syntax is identical in Bash and PowerShell. Load the matching section before running CLI fallback commands:

- **GitHub Actions** (triggers, re-runs, cancels, and CLI-only `gh run watch`): [references/flag-tables.md](references/flag-tables.md#running-github-actions).
- **Creating a pull request when MCP is unavailable** (follows the PR creation workflow above, `--draft`, file-written title/body): [references/mode-procedures.md](references/mode-procedures.md#creating-a-pull-request-when-mcp-is-unavailable).
- **Managing issues when MCP is unavailable** (explicit `--repo`, verified exact-match labels, duplicate search, templates, native `--type` and `-p/--project`, verified close): [references/mode-procedures.md](references/mode-procedures.md#managing-issues-when-mcp-is-unavailable).
- **Deleting a branch** (no MCP tool; requires full safety/merge/protection/stack probes, preview, and point-of-action confirmation naming action or branch before REST `DELETE`; default or protected branch stops): [references/mode-procedures.md](references/mode-procedures.md#deleting-a-branch).
- **Direct API access** (`gh api` REST/GraphQL with `--hostname` off `github.com`): [references/flag-tables.md](references/flag-tables.md#direct-api-access).

## Part 3: Stacked PRs (`stack` mode)

`gh stack` (`github/gh-stack`) turns dependent branches into dependent PRs. Every stack path ends at item 7 of [Pull request creation workflow](#pull-request-creation-workflow). `gh` runs no `git push` itself (hand branches to `git push` first so `link`/`submit --auto` only create/link PRs) and **never** runs `gh stack push` or `gh stack sync` (both force-push).

- **Stack when:** dependent, independently reviewable layers (e.g., schema -> API -> UI), oversized single PR, or an ordered slice list with "Blocked by" edges.
- **Don't stack when:** tasks are independent and parallelizable — announce and hand the **whole batch** to `routing` ([target classification](../routing/references/forwarding.md#target-classification)) rather than opening PRs here; or when the change is one cohesive PR or layers cannot be reviewed in isolation.

### Stack workflows

Load [references/stack.md](references/stack.md) and follow [new-stack](references/stack.md#stack-workflow-new-stack) or [adoption](references/stack.md#adoption-workflow-existing-prs) before creating, pushing, retargeting, or linking any layer. Command/flag rules: [references/flag-tables.md](references/flag-tables.md#stack-mode-commands-and-flags).

### Definition of done: GitHub confirms the stack

**A stack exists only when GitHub confirms it.** Base-chained PRs in `gh pr list` are not a stack. Verify the GraphQL `stack` field on one member PR (`totalCount` and entry PR numbers match the intended chain; null means no stack object). Commands: [references/mode-procedures.md](references/mode-procedures.md#stack-mode-verification).

### After a base PR merges

Recover the next pushed layer by merging trunk into it and handing the push to `git push` with no force: [stack.md](references/stack.md#after-a-base-pr-merges).

## References

- [references/mode-procedures.md](references/mode-procedures.md): workflow index, bundled-write preview example, displayed-issue handoff, issue/PR/check-failure/branch-delete procedures, and stack verification commands.
- [references/flag-tables.md](references/flag-tables.md): MCP vs CLI coverage, host/repo flags, repository policy settings table, Actions, direct API access, and stack command/flag rules.
- [references/github-mcp-server.md](references/github-mcp-server.md): MCP server overview, authentication, configuration, default toolsets, and tool catalog.
- [references/github-mcp-toolsets.md](references/github-mcp-toolsets.md): toolset definitions and common issue, pull request, and repository workflow mappings.
- [references/issues/README.md](references/issues/README.md): overview of the issue reference files.
- [references/issues/issue-fields.md](references/issues/issue-fields.md): issue fields (type, priority, dates): discover, read, set, and search.
- [references/issues/issue-types.md](references/issues/issue-types.md): GraphQL calls for issue types.
- [references/issues/templates.md](references/issues/templates.md): bug report, feature request, task, and minimal issue templates.
- [references/issues/sub-issues.md](references/issues/sub-issues.md): parent and sub-issue links via CLI, MCP, REST, and GraphQL.
- [references/issues/dependencies.md](references/issues/dependencies.md): blocked-by and blocking links, and read-only tracked issues.
- [references/posting-envelope.md](references/posting-envelope.md): idempotency marker, body shape, triage-only footer exception, title registry, and outbound checks.
- [references/search.md](references/search.md): advanced issue search syntax, patterns, and field search.
- [references/projects.md](references/projects.md): Projects V2 lookup, progress reports, OAuth scopes, and item IDs.
- [references/images.md](references/images.md): ways to embed images in issues and comments, ranked by reliability.
- [references/pull-requests/linkage.md](references/pull-requests/linkage.md): candidate detection, validation, resolutions, and closing-keyword guard.
- [references/pull-requests/body.md](references/pull-requests/body.md): PR body contract: Summary, Evidence, Impact, template placement, stack layers, and refresh.
- [references/stack.md](references/stack.md): `gh stack` prerequisites, command map, new-stack and adoption workflows, rebase ripple, recovery, and safety rules.
- [Routing lifecycle phases](../routing/SKILL.md#lifecycle-phases): start-or-continue path selecting the phase for implementation requests.
- [Forwarding](../routing/references/forwarding.md): rules preserving context and authorization in a handoff.
