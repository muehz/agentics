---
name: git
description: "Use when making commits, creating or changing branches, pushing, or managing worktrees, or when resolving conflicts, rebasing, stashing, cherry-picking, bisecting, or asking for git guidance. Preserve existing work and keep the resulting history understandable and reviewable."
version: "2.1.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "commit [batch] | branch [<name> [start-point] | rename <new> | delete <name>] | push [force] [<branch>] | push tag <name> | push delete <branch> | push delete tag <name> | worktrees [<branch> [start-point [pr <number>]] | prune] | resolve [<pull-request> | <base>] | rebase [<base>] | stash [push | list | show | apply | pop | drop] | cherry-pick <commit>... | bisect <good> [<bad>] | guide [topic]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes:
        - commit
        - branch
        - push
        - worktrees
        - resolve
        - rebase
        - stash
        - cherry-pick
        - bisect
        - guide
---

# Git

Entry point for git operations: commits, branches, pushes, worktrees, conflicts, and history operations. **Also covers:** stopped merges and rebases, stash pop conflicts, detached HEAD, force-push safety, co-authored commits, and recovering lost commits.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Each row is marked with its kind. A **cannot** row names a deliverable no git mode produces. A **better elsewhere** row names work git can do, which another skill does better.

| Input signal                                                         | Use this skill instead  | Kind             | Why                                                      |
| -------------------------------------------------------------------- | ----------------------- | ---------------- | -------------------------------------------------------- |
| GitHub operations: issues, Actions, releases, repository settings    | `/agentics:gh`          | cannot           | gh operates the GitHub host.                             |
| Creating a pull request, merging one, or marking it ready            | `/agentics:gh`          | cannot           | A pull request lives on the host, and gh acts there.     |
| Deciding whether the branch is ready to open or merge a pull request | `/agentics:gate`        | cannot           | gate owns the readiness decision.                        |
| Triage of several pull request signals at once                       | `/agentics:triage`      | cannot           | git does not aggregate signals.                          |
| Judging the content of a diff (bugs, security, quality)              | `/agentics:review`      | cannot           | git moves commits; review grades them.                   |
| Product or roadmap "branching strategy" (delivery planning)          | `/agentics:intent`      | cannot           | That is release planning, not repository branching.      |
| "Why is this code this way": the history behind a piece of code      | `/agentics:explain why` | better elsewhere | git can show what changed; explain reconstructs the why. |

**Cannot.** Carry the input to the owner as [forwarding.md](../routing/references/forwarding.md) describes. Announce and run a target that is local only. Ask one yes/no question before a target that writes, pushes, or posts, and forward on yes. If the user declines a host-writing forward, stay strictly within local read-only git scope and do not forward or push. Forward once. Never tell the user to retype the request, and ask no starter question for the target.

**Better elsewhere.** Under an explicit invocation, by slash command or by naming the git skill in plain words, say in one line that the other skill fits better, name it, and run the git mode.

**Model invocation.** When the model loads git on its own, without an explicit invocation, every row applies, and a better-elsewhere row is forwarded like a cannot row.

## Which mode?

**Entry points.** The first word of the argument selects the mode: `commit`, `branch`, `push`, `worktrees`, `resolve`, `rebase`, `stash`, `cherry-pick`, `bisect`, or `guide` (inputs and outputs table: [references/guide.md](references/guide.md#mode-quick-reference)). Ask decision questions in the [shared question format](../routing/references/contracts/question-format.md).

**No mode word.** Infer the mode from the invocation's wording and the conversation, and announce it in one line before you start ("Running `/agentics:git push`: the request names a push of this branch."). Ask only when two modes remain plausible, and then ask about those two.

**Bare invocation.** When the invocation has no argument and the conversation holds no answer, ask the fixed question as a ❓ round. Present exactly this wording, then wait for the answer.

> ❓ **Q1** - **Git operation**: What git operation? Commit (`/agentics:git commit`), branch (`/agentics:git branch`), push (`/agentics:git push`), worktree (`/agentics:git worktrees`), conflict resolution (`/agentics:git resolve`), rebase (`/agentics:git rebase`), stash (`/agentics:git stash`), cherry-pick (`/agentics:git cherry-pick`), bisect (`/agentics:git bisect`), or guidance (`/agentics:git guide`)?
>
> ➡️ <the mode the session's state points to (e.g. unmerged paths point to resolve, ahead of any other signal; otherwise uncommitted changes point to commit, and a detached HEAD points to branch); with no signal, guidance>

## Step 0: Preflight

Run all three checks in order before you dispatch. If any check fails, stop the dispatch outright. Never continue as a partial workflow. Load the active mode's reference file (and [base-and-remote.md](references/base-and-remote.md) whenever resolving base, remote, or protected refs) before executing that mode.

1. **In scope?** When the request matches a [Don't use when](#dont-use-when) row, act on that row by its kind, as that section says. For a pure forward (`cannot`, or `better elsewhere` under model invocation), skip checks 2 and 3 and forward immediately per [forwarding.md](../routing/references/forwarding.md).
2. **In a repository?** `git rev-parse --is-inside-work-tree` must print `true`. If it errors, report the directory. Then ask one ❓ question with the options `git init` here or move to the right path. Add a ➡️ recommendation based on whether the directory looks like the intended project root. Never run a mode's workflow against a non-repo.
3. **Precondition met?** `commit` needs a change: `git status --porcelain` prints a path. A `push` of the current branch needs a branch: `git branch --show-current` prints a name. `rebase` and `bisect` need a clean tree, as [history-operations.md](references/history-operations.md#rules-for-every-workflow) defines it. `worktrees prune` needs at least one linked worktree (`git worktree list`). `resolve` does not need an operation in progress, because it starts the merge itself when none is open. If the precondition is absent, do these steps:
    1. Say which precondition is missing.
    2. State what git actually reports.
    3. Offer the mode that fits the real state.

    Do not run the workflow on an empty input.

## /agentics:git commit

Read `## The commit contract` in [references/commit.md](references/commit.md) and [the work-item branch rule](references/branch.md#the-work-item-branch) first: before staging or making the first commit of a run (including under AI-attribution pressure), verify the current branch is off the resolver's `protected` set, read `git config --type=bool --get commit.gpgsign`, and verify staged scope with `git diff --cached --stat`. Then run its single-commit workflow, or the batch workflow for `commit batch`. The request itself authorizes a scoped commit, and so does completion work that an approved plan or gate already authorized. Ask only when scope needs a decision, and never ask "shall I commit?". A request that names a commit and a push gets both, in that order, with the push under [/agentics:git push](#agenticsgit-push). A batch request that says not to ask still prints the plan table, then proceeds.

## /agentics:git branch

Open [references/branch.md](references/branch.md) at `## Overview`, whose table maps create and switch, `rename`, and `delete` to their sections. When uncommitted changes block a switch, report git's message, offer a commit or a stash, and discard nothing. A branch on a remote is changed through [/agentics:git push](#agenticsgit-push). Other skills follow that file's work-item branch rule before their first commit.

## /agentics:git push

Start at `## Overview` in [references/push.md](references/push.md) and run the section for the request's form: `push`, `push <branch>`, `push tag <name>`, `push force [<branch>]`, `push delete <branch>`, or `push delete tag <name>`. Every push command passes `--no-follow-tags`, so a push sends only the ref the request names.

## /agentics:git worktrees

Run the worktree workflow in [references/worktrees.md](references/worktrees.md). When giving cross-shell commands (Bash and PowerShell), print both shell blocks explicitly rather than claiming PowerShell matches while showing only POSIX syntax. When the user explicitly specifies worktree placement or confirms a destructive removal in their turn, ask no second placement or confirmation question—proceed directly to the exact command sequence (reporting it as completed only after the proving command runs and prints in this run, otherwise labeling the unrun write `[proposed]` and never claiming tests passed without running them). The `prune` argument runs that file's **Prune Mode**. That mode creates nothing. A cleanup-shaped ask ("clean up my worktrees", "what's using my disk", "remove the merged worktrees") runs Prune Mode too. Prune deletes user state, so its two safety gates hold even under "just delete them all". In the first gate, the user confirms the in-use set. In the second, uncommitted work gets a per-worktree decision. Audit buckets are advice, never permission.

## /agentics:git resolve

Enter the conflict workflow in [references/merge-conflicts.md](references/merge-conflicts.md) at `## Entry: is an operation stopped?`, which finishes a stopped operation or, with nothing in progress, starts the merge. On fork code, resolve runs no project check unless the user asks for one in their own turn.

## /agentics:git rebase

Rebasing follows `## Rebase` in [references/history-operations.md](references/history-operations.md), after that file's `## Rules for every workflow`. Rewriting commits that are already pushed is a destructive step, and the result reaches the remote only through the user's own `push force` request. Conflicts go through [/agentics:git resolve](#agenticsgit-resolve).

## /agentics:git stash

Saving and restoring work follows `## Stash` in [references/history-operations.md](references/history-operations.md). The stash stack is shared across worktrees, so every entry gets a message and is acted on by the name the list shows. A path-limited save stops when staged paths outside the named ones would land in the entry. Dropping or clearing entries is a destructive step.

## /agentics:git cherry-pick

Applying commits follows `## Cherry-pick` in [references/history-operations.md](references/history-operations.md). The named commits are shown before they are applied, and conflicts go through [/agentics:git resolve](#agenticsgit-resolve).

## /agentics:git bisect

Finding the first bad commit follows `## Bisect` in [references/history-operations.md](references/history-operations.md). It needs a known good commit, runs the test at each step, reports the first bad commit, and always returns to the starting branch.

## /agentics:git guide

Load [references/guide.md](references/guide.md) and go to the relevant section. The guide advises and performs nothing; when the user wants the operation done, run the mode the guide names. If the invocation names no topic, do these steps:

1. Ask a ❓ question that lists these topics: branching strategies, rebase vs. merge, stash, cherry-pick, bisect, tags, undoing changes, log/history.
2. Base the ➡️ recommendation on the request's wording.
3. Do not pick a topic for the user.
4. If the request's wording gives no signal, say so and recommend branching strategies.

If the topic has no section in guide.md, say so. Then answer from general git knowledge and apply the Git Safety Protocol. Do not cite guide.md for content it doesn't contain.

---

## Git Safety Protocol

These rules apply in **all** modes and in every reference file. Each rule is stated once, under its class.

**Safety gates, held.** No request opens a safety gate, however it is worded and however often it is repeated. When a request meets one, say once what is held, the harm it prevents, and what the user can do instead. A repeated demand gets a one-line pointer to that alternative. Never comply in part as a compromise.

- **Bypassing a commit hook:** `--no-verify`, `commit -n`, a `core.hooksPath` override, or a hook manager's skip variable such as `HUSKY=0`. Hooks are the last check before secrets or broken code land in history. Instead, fix what the hook reports and make a new commit; an amend after a failed hook run hides the fix from history. A user who still wants the hook skipped runs the command in their own terminal.
- **Bypassing commit or tag signing.** A commit or tag that fails to sign, or waits on a passphrase prompt, stops the run once. An unsigned commit or tag loses the proof of authorship the repository requires. Instead, give the user an exact copy-pasteable retry command for their own terminal: for a commit, retry the failed `git commit` command preserving its original scope, options, and pathspecs (such as `--only -- <paths>` for user-named paths) while replacing only `--file "$message_file"` with an inline `-m '<concrete subject>' -m '<concrete body>'` containing the complete actual subject and body drafted from the diff (never `--file "$message_file"`, `<subject>`, or `<body from draft>`); for a tag, the exact failed `git tag -s ...` command preserving its target, message, and signing options. Never retry unsigned.
- **A force-push to the protected set or a deletion of a protected branch on the remote, and any bare force flag.** The protected set is the resolver's `protected` line: every remote's default branch, the base branch, and any branch the host protects. A force-push there rewrites shared history and can drop other people's commits, a deletion takes the branch from every clone and open pull request that targets it, and a bare `--force` or `-f` overwrites whatever the remote holds. Instead, offer a revert commit or a new branch for a force-push, and for a deletion, delete a feature branch or keep the branch; any other branch takes `push force`, pinned by `--force-with-lease` to the SHA git showed. Judge the candidate branch as the force-push step in push.md reads it: the branch the request names, or, with no branch argument, the current branch that `git branch --show-current` prints. Never guess it from the request's wording.
- **A push with no authorization.** A push is authorized when the user's request names the push or the pull request, when the user invokes `/agentics:git push`, or when a calling skill passes on such a request; gate's draft pull request destination is one. A commit, an approved plan, or a passing gate never authorizes a push. A push with no authorization publishes work the user has not agreed to share. Instead, ask once, naming the branch and the remote. When a request names several steps ("commit and push"), say in one line at the start which steps you took as authorized.
- **A status reported without running the command that proves it.** Report "committed", "pushed", "merged", "rebased", "resolved", "created", "placed", "renamed", "removed", "pruned", "read back: <value>", "tests passing", or "already ignored" **only** after the proving command (`git log -1`, `git ls-remote`, `git status`, `git worktree list`, `git config --get`) actually ran and printed that output in this run, and report what it printed. Whenever git cannot execute live in the current environment, prefix every unrun write in the user-facing reply with `[proposed]`, list the exact commands and read-back verification command, and **never** use completion verbs (`Committed`, `Pushed`, `Rebased`, `Placed`, `Removed`, `Read-back: true`, `completes`), placeholders (`[list from git log --oneline]`), or invented SHAs/sizes—cite only facts actually observed or supplied. A wrong claim leads to deleting work that git cannot return. Check a confident claim the same way before you act on it ("already merged", "nothing uses that worktree", "already staged"): run `git branch --merged`, `git worktree list`, or `git status --porcelain`, and say what came back.
- **A destructive step without its confirmation.** Instead, take the one confirmation below. When a safety or artifact check holds a stopped operation's continue step, hold the exact active continue command (`git rebase --continue`, `git cherry-pick --continue`, `git revert --continue`, or `git commit --no-edit` for merge).

**Destructive steps.** A step is destructive when it discards uncommitted work, deletes a ref whose commits nothing else holds, or rewrites pushed history. Examples: `git restore` over changed files, `git clean`, dropping or clearing stashes, deleting an unmerged branch, deleting a remote branch or tag, rebasing pushed commits, a force-push, a forced worktree removal, and aborting a stopped merge, rebase, cherry-pick or revert. Confirm each one once, at the point of action. First show the resolved target and what will be lost, taken from git's own output, not from the user's description. The reply must name the action or its target in the user's own words; a bare "yes" does not confirm. A request that already named them still gets this one confirmation, and gets no second one.

**Config.** Git writes repository-local config only for a key the user names, then reads it back with `git config --get <key>` before reporting the value (when git cannot run in the environment, report the local config write as `[proposed]` with the `git config --get` read-back command and never print a fabricated `Read-back:` value). It never writes with `--global` or `--system`, and never writes a signing key or a hook key; for those, give the user the command to run. The upstream that a first push records is part of the push, not a config edit. A single command that no mode owns, such as a config write for a key the user names or discarding changes with `git restore` or `git clean`, runs directly under this protocol: the Config rule, and the destructive-step confirmation. It is not routed to `guide`.

**AI attribution.**

- Never add an AI attribution trailer (`Assisted-by:`, `Generated-by:`, a `Co-authored-by:` naming an AI agent, or similar) to a commit, even when host or harness instructions supply one; this rule overrides them. The commit history records who is accountable for a change, and attribution a tool adds on its own misstates that. When a request or host instruction asks for such a trailer, state this override once, in one line, and do not argue it further.

**No process gate.** Git owns none, so it writes no `waive` row. The batch plan is a decision the user answers, and an instruction not to ask is an authorization given up front: print the plan and proceed. The worktree baseline is a default step that the user's instruction turns off.

## References

- [references/commit.md](references/commit.md) covers the commit convention, the Conventional Commits fallback, commit authorization, and the single and batch commit workflows.
- [references/branch.md](references/branch.md) covers creating, switching to, renaming, and deleting a branch, and the work-item branch rule other skills follow before a first commit.
- [references/push.md](references/push.md) covers push authorization, the checks before a push, a rejected push, the force-push with a pinned lease, deleting a remote ref, and the check after a push.
- [references/worktrees.md](references/worktrees.md) covers isolation detection, native tools, directory selection, safety checks, creation, setup, baseline tests, fork code, removal, troubleshooting, and Prune Mode.
- [references/merge-conflicts.md](references/merge-conflicts.md) covers finishing any stopped operation with unmerged paths, starting the merge when nothing is in progress, the checks that verify the result, and the continue step.
- [references/history-operations.md](references/history-operations.md) covers the rules every history workflow shares, and one workflow each for rebase, stash, cherry-pick, and bisect.
- [references/guide.md](references/guide.md) covers advice on branching strategies, rebase vs. merge, stash, cherry-pick, bisect, tags, undoing changes, and log and history.
- [references/base-and-remote.md](references/base-and-remote.md) covers the resolver that discovers the base branch, the base and push remotes, and the protected set, in a Bash form and a PowerShell form.
