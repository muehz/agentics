# Stacked PRs (`gh stack`)

The `github/gh-stack` extension turns a chain of dependent branches into a chain of dependent PRs on GitHub. The minimum
create, link, and verify recipe is in the main skill's PR creation workflow (item 7). GitHub's own agent skill covers
further mechanics: `gh skill install github/gh-stack`.

## Prerequisites

```bash
# bash
gh extension install github/gh-stack
```

```powershell
# PowerShell equivalent (gh CLI syntax is identical across shells)
gh extension install github/gh-stack
```

- gh CLI >= 2.0
- Stacked PRs must be enabled on the repository. When they are not enabled, commands exit **9**. On exit 9, report that
  to the user and do not retry.
- Stack metadata is stored in `.git/gh-stack`. It is local only, never committed, and not shared across clones or
  worktrees.
- Recommended: `git config rerere.enabled true` (it remembers conflict resolutions across a cascade of merges or
  rebases). On multi-remote repositories, `checkout` and `trunk` use the default push remote. Resolve its configured
  name with the [base and remote resolver](../../git/references/base-and-remote.md), capture it as `$remote` in the
  config call, and run `git config -- remote.pushDefault "$remote"`.

## Command map

| Command                                                       | Purpose                                                                                                                       |
| ------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `init [branches...]`                                          | Initialize a stack; adopts existing branches or creates new ones, bottom to top                                               |
| `add [branch]`                                                | Add a new branch on top of the stack and optionally commit staged changes (`-A`/`-u`/`-m`). See the `-A` rule below the table |
| `checkout <stack# \| pr# \| url \| branch>`                   | Check out a stack; no argument opens an interactive picker                                                                    |
| `rebase`                                                      | Rebase in a cascade across the stack (trunk + inter-branch); `--continue`/`--abort` on conflict                               |
| `modify`                                                      | Interactive TUI to drop/fold/insert/reorder/rename branches in the stack                                                      |
| `sync`                                                        | Fetch, rebase, push, and sync PR state; `--prune` deletes local branches for merged PRs                                       |
| `push`                                                        | Push active branches; per-branch `--force-with-lease`, not atomic                                                             |
| `submit`                                                      | Push and create/update PRs on GitHub. See safety rules below                                                                  |
| `link <stack# \| branch-or-pr...>`                            | Create/update a stack on GitHub from existing branches, PR numbers, or PR URLs. No local tracking                             |
| `merge [stack# \| pr#]`                                       | Atomic all-or-nothing merge up through the chosen PR                                                                          |
| `view`                                                        | Show the stack (branches + PR status); `--short`, `--json`                                                                    |
| `unstack [stack#]`                                            | Remove a stack locally and on GitHub; `--local` keeps the remote stack intact                                                 |
| `up [n]` / `down [n]` / `top` / `bottom` / `trunk` / `switch` | Navigate between branches in the current stack                                                                                |

`-A` stages the whole tree, so use `add -A` only on a layer whose edits are the only ones present.

Always run the agent-safe forms: `view --json`, `submit --auto`, `init <branch>…`, `add <branch>`, and
`checkout <target>`. The bare forms open prompts or full-screen TUIs and block a non-interactive session. `modify` and
`switch` have no non-interactive path at all. Restructure with `unstack` then `init` instead.

Use `gh stack link` when branches and PRs already exist. To append to a stack that already exists, pass its number
first: `gh stack link <stack#> <branch-or-pr…>`. This adds the rest to the top. It does not need the stack's PRs listed
again.

When the CLI is unavailable, the web UI's **Create stack** and **Add to stack** finish the same job. GitHub shows a
recommendation banner on base-chained PRs. A base edit alone is still not a stack. PR bases retargeted at each other do
**not** create a stack. Until `link` (or `submit`) runs, GitHub holds no stack object and the stack UI shows nothing.

`view` cannot confirm a link-built stack because it reads local tracking and reports nothing unless the current branch
is in a stack. Confirm through GitHub, with the GraphQL `stack` field on one member PR. This is the definition-of-done
check in the main skill's Part 3.

An ordered slice list with dependency edges is already a stack shape. `/agentics:decompose`
([`skills/decompose/references/slicing.md`](../../decompose/references/slicing.md)) writes the slice sections of
`plan.md`. Each section records its dependencies, so the sections can seed `gh stack init` or `gh stack link`.

## Stack workflow: new stack

For work that has no branches or PRs yet. When the PRs already exist, use the
[adoption workflow](#adoption-workflow-existing-prs) instead.

1. **Recognize the shape.** Confirm against the [stack tests](../SKILL.md#part-3-stacked-prs-stack-mode) that the work
   is a dependent chain. Name the layers bottom to top. The bottom layer is the one nothing else depends on.
2. **Offer, don't impose.** Propose the branch order, one layer per branch. Say that, once GitHub confirms the stack,
   each layer's PR body will be written per
   [references/pull-requests/body.md](pull-requests/body.md#native-stack-layers). Disclose the cascade cost. Once the
   stack is submitted, any change to a lower layer cascades. Such a change can be review feedback, a failing check, or a
   trunk update. Every pushed layer above a change takes a merge of it, and its push is handed to `git push` with no
   force. A pushed layer is rewritten by rebase only on the user's explicit request ([Rebase ripple](#rebase-ripple)).

    The read-only readiness check always runs. A new stack has no pull requests yet, so the check has nothing to read
    here. Branches that already have open PRs go through the adoption workflow, whose step 2 holds the check. Then one
    of three cases applies:
    - **The request names the stack, the order is unambiguous, and the check finds nothing outstanding.** State the
      order and the cascade cost in one line and proceed with no question.
    - **The check finds something outstanding, or the order is ambiguous.** Ask, as one ❓ question, whether to settle
      first, stack knowingly, or stop. Give a ➡️ recommendation. Create no branch, commit, or PR before the answer.
    - **The request does not name the stack.** Offer it as one ❓ question, with the order and the cascade cost in the
      offer. Wait for the answer the same way.

3. **Create the base.** Save the approved layer names to scratch with the file-write tool. In the init call, read the
   bottom name from its record as `$bottom_branch`. Require a nonempty name that does not start with `-` and verify it
   with `git check-ref-format "refs/heads/$bottom_branch"` before running `gh stack init "$bottom_branch"`. Then commit
   **only the bottom layer's changes** to it. The commits follow the contract in
   [commit.md](../../git/references/commit.md).
4. **Add each layer above it.** In each add call, read the selected name from its record as `$layer_branch` and apply
   the same branch-name checks before running `gh stack add "$layer_branch"`, in order. Commit only that layer's changes
   to its own branch. One layer's edits never land on another layer's branch. The whole change set never lands on the
   bottom branch. A stack whose layers are not separated is one PR under several branch names.
5. **Submit as drafts.** Each branch of the stack is handed to `git push` first. Then run `gh stack submit --auto`,
   which creates the PRs as drafts, per [Repository policy](../SKILL.md#repository-policy). Bare `submit` opens an
   interactive editor, which blocks a non-interactive session, and defaults new PRs to ready for review. `--open` marks
   new and existing PRs ready, which follows [Repository policy](../SKILL.md#repository-policy). After a
   `submit --open`, read each PR's `gh pr view <number> --json isDraft` and report a PR as ready only when that read is
   `false`.
6. **Confirm the stack on GitHub.** Run the
   [definition-of-done check](../SKILL.md#definition-of-done-github-confirms-the-stack). A clean exit from `submit` is
   not that confirmation. Write each layer's body bottom to top per
   [references/pull-requests/body.md](pull-requests/body.md#native-stack-layers) only after that check passes.
7. **Merge.** The merge follows [Repository policy](../SKILL.md#repository-policy).

```bash
# bash: schema - api - ui, one layer per branch; commit each layer before adding the next
gh stack init schema-migration
git add db/migrations && git commit -m "feat(db): add search index migration"
gh stack add api-endpoints
git add src/api && git commit -m "feat(api): add search endpoint"
gh stack add search-ui
git add src/ui && git commit -m "feat(ui): add search box"
# each of the three branches is handed to git push before this line, so submit only creates the draft PRs
gh stack submit --auto
```

```powershell
# PowerShell equivalent (gh CLI syntax is identical across shells; ; separates statements)
gh stack init schema-migration
git add db/migrations; git commit -m "feat(db): add search index migration"
gh stack add api-endpoints
git add src/api; git commit -m "feat(api): add search endpoint"
gh stack add search-ui
git add src/ui; git commit -m "feat(ui): add search box"
# each of the three branches is handed to git push before this line, so submit only creates the draft PRs
gh stack submit --auto
```

The bottom layer gets its own commit right after `init`. A skipped commit leaves the schema work uncommitted for the
next commit to absorb. Stage each layer by path (`git add <paths>`), never `-A`. With all three layers in the working
tree, `gh stack add api-endpoints -A` sweeps schema, API, and UI edits into the API commit. It leaves the UI branch with
nothing to commit. The same rule applies to the `-A`/`-u` shorthand of `add`. The shorthand commits whatever is in the
tree. It is only correct when one layer is present at a time.

## Adoption workflow: existing PRs

When the PRs already exist, for example because parallel sessions opened them separately, `init`/`add` is the wrong
tool. Retarget, then **link**.

1. **Recognize and order.** Apply the same [stack tests](../SKILL.md#part-3-stacked-prs-stack-mode). Name the layers
   bottom to top. Step 3 settles an order that the request leaves ambiguous.
2. **Check readiness, read-only.** This check always runs. Inspect **every** PR that the stack adopts, top included, not
   only the lower ones: `gh pr view <n> --json state,mergeable,reviewDecision,statusCheckRollup`, plus unresolved review
   threads. Read the unresolved review threads with
   `gh api graphql -f query='query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){pullRequest(number:$n){reviewThreads(first:100){nodes{isResolved path}}}}}' -F o="$owner" -F r="$repo" -F n="$number"`
   and count the nodes whose `isResolved` is `false`. The topmost adopted PR carries review state and failing checks of
   its own. A skipped top PR hides the signal that the step 3 disclosure must carry. A branch with no PR has no review
   state to check. The link creates its PR. This step changes nothing on GitHub. Remediation waits for the user's
   decision in step 3.
3. **Offer, don't impose.** Say that, once GitHub confirms the stack, each adopted PR's body will be rewritten per
   [references/pull-requests/body.md](pull-requests/body.md#native-stack-layers). Make two disclosures. First, disclose
   whatever step 2 found outstanding. A layer that still needs fixes will change, and every change below cascades
   upward. Second, disclose the cascade cost. Once the PRs are linked, any change to a lower layer cascades. Such a
   change can be review feedback, a failing check, or a trunk update. Every pushed layer above a change takes a merge of
   it, and its push is handed to `git push` with no force. A pushed layer is rewritten by rebase only on the user's
   explicit request ([Rebase ripple](#rebase-ripple)).

    Then one of three cases applies:
    - **The request names the stack, the order is unambiguous, and step 2 found nothing outstanding.** State the order,
      the body rewrite, and the cascade cost in one line and proceed with no question.
    - **Step 2 found something outstanding, or the order is ambiguous.** Ask, as one ❓ question, whether to settle
      first, stack knowingly, or stop. Give a ➡️ recommendation: settle first via `/agentics:triage pr <N>` when signals
      are outstanding, otherwise stack. Wait for the answer before any merge, push hand-off, base retarget, `link`, or
      settling work on the PRs.
    - **The request does not name the stack.** Offer it as one ❓ question, with the order, the step 2 findings, and the
      cascade cost in the offer. Wait for the answer the same way.

4. **Chain the bases, bottom-up.** For each branch above the bottom, take the merge path in
   [Rebase ripple](#rebase-ripple). Use the lower PR's head as the base and hand the push to `git push`, with no force.
   A PR opened in another session can have no local branch here, or a local copy behind CI's pushes. To retarget the PR,
   load the lower branch's recorded name into `$lower_branch` in the same call and pass the verified PR number as
   `$pr_number` to `gh pr edit "$pr_number" --base "$lower_branch"`. The bottom PR keeps the trunk base.

5. **Link the stack.** Each branch that `link` would push is handed to `git push` first, so `link` only links. Use
   `gh stack link <bottom> <…> <top>` with PR numbers or branch names in **bottom-to-top order**. Without
   `--base <trunk>`, `link` sets the bottom PR's base to the repository's default branch. Pass `--base <trunk>` whenever
   trunk is another branch. `link` adopts existing open PRs and needs no local tracking state. **Base-chaining alone
   creates no stack.** Until `link` runs, GitHub holds no stack object, and the stack UI shows nothing. Never report a
   stack built on base edits alone.
6. **Confirm the stack on GitHub.** Use the
   [definition-of-done check](../SKILL.md#definition-of-done-github-confirms-the-stack). `gh stack view` cannot confirm
   it. `view` reads local tracking. It reports nothing for a link-built stack unless the current branch is in a stack.
   Write each layer's body bottom to top per
   [references/pull-requests/body.md](pull-requests/body.md#native-stack-layers) only after that check passes.
7. **Merge.** The merge follows [Repository policy](../SKILL.md#repository-policy).

## Rebase ripple

Any change to a lower branch leaves every branch above it behind its base. Such a change can be a review fix, a new
commit, or a trunk update. Which path brings a layer current depends on whether that layer is already pushed:

- **Pushed (branch on the remote, PR or not):** Merge.
    1. Fetch.
    2. Bring the layer to its own remote tip.
    3. Merge the updated base into it.
    4. Hand the push to `git push`, with no force.
    5. Repeat upward, one layer at a time.
- **Never pushed:**
    - Run `gh stack rebase --upstack` from the lowest never-pushed layer.
    - Do this only when every layer above it is also never pushed.
    - `--upstack` rebases its starting layer and every layer above it.
    - Use a full `rebase` only when no layer in the stack is pushed.
    - A never-pushed layer below a pushed one takes a `git merge` of its updated base instead.
    - Nothing published is rewritten.
- **Pushed, and the user explicitly asks to rewrite it:**
    - The rewrite runs from the lowest layer the user named to the top.
    - Run `gh stack rebase --upstack` from that layer. It rewrites the layers locally and pushes nothing.
    - A bare `rebase` rewrites every layer and `sync` cascade-rebases every branch, so neither runs here.
    - Hand each rewritten layer to `/agentics:git`'s force-push, which needs an explicit request that names the
      operation and the branch.

A merge keeps published history. Review in progress stays attached, and nobody who pulled the branch has to reconcile
rewritten commits. `gh stack push` and `gh stack sync` both force-push, so gh never runs either one. Each merged layer's
push is handed to `git push`:

For a pushed layer, resolve its configured remote with the
[base and remote resolver](../../git/references/base-and-remote.md). Take the layer's and lower layer's branch names
from their confirmed PR heads or the approved layer list. If the values come from an earlier call, store them with the
file-write tool and load them back as data. In each call, use `$remote`, `$layer_branch`, and `$base_branch` for those
values. Build `$layer_ref` as `refs/remotes/$remote/$layer_branch` and `$base_ref` as
`refs/remotes/$remote/$base_branch`. Run these in order in Bash or PowerShell, checking each exit status before
proceeding:

1. `git fetch -- "$remote"`.
2. Check for the local layer with `git show-ref --verify --quiet "refs/heads/$layer_branch"`. When it exists, use
   `git switch --no-guess -- "$layer_branch"`. When it is absent, use
   `git switch -c "$layer_branch" --track -- "$layer_ref"`.
3. `git merge --ff-only -- "$layer_ref"`. This picks up intervening pushes. When it refuses, stop and report the
   divergence.
4. `git merge -- "$base_ref"`. Resolve conflicts with `/agentics:git resolve`, then hand the push to `git push`, with no
   force.

Rules:

- **Check before stacking on existing work.** Do this before `link` stacks existing PRs. Inspect every PR the stack
  adopts, top included (`gh pr view <n> --json state,mergeable,reviewDecision,statusCheckRollup`, plus unresolved review
  threads). Surface what is outstanding in the stack offer. A branch with no PR has no review state to check.
  Remediation is a separate, user-confirmed step. `/agentics:triage pr <N>` owns the multi-source pass. It is never a
  silent side effect of a stack proposal. A stack built on a PR that still needs fixes guarantees a cascade.
- **Disclose the cost at proposal time.** The stack offer, or the one line before a named stack, states three things.
  Any post-creation change to a lower layer cascades. Every pushed layer above it takes a merge commit. A rebase instead
  would rewrite review in flight. Whether to stack on unsettled work remains the user's call. The user makes it
  knowingly, not by discovering the cost at the first cascade.
- **CI that pushes counts as a lower-layer change.** A CI job that pushes autofix commits can advance a lower branch
  after a cascade pass. A formatter job that pushes `style:` commits is one example. Every branch above loses ancestry
  to it, and the upper PRs flip to `CONFLICTING`. Make every branch tip formatter-clean before its push is handed to
  `git push`. After CI settles, re-verify each link. Fetch, then build the same `$base_ref` and `$layer_ref` from the
  resolved values and confirm the ancestry with `git merge-base --is-ancestor "$base_ref" "$layer_ref"`.

If a conflict occurs mid-cascade on the merge path, run `git merge --continue` or `--abort`. If it occurs on the rebase
path, run `gh stack rebase --continue` or `--abort`. The conflict edit itself is `/agentics:git resolve`'s work.

## After a base PR merges

When the bottom PR merges, recover the next layer the same way, whatever the merge method. After a squash merge, the
squash commit on trunk shares no ancestry with the branch commits it replaced. After a squash merge, GitHub may also
delete the merged head branch. A rebase of the next layer would replay commits trunk already holds. Recover with a
merge.

The steps below write to GitHub once for each step that changes remote state. These writes are the base change when
needed, one push per layer, and a re-link when needed. After step 1, preview that sequence once. Steps the user named
run when their turn comes, and the rest take one confirmation. `<trunk>` is the stack's trunk branch, usually `main`,
and `<remote>` is the branch's remote.

1. **Confirm the state, read-only.** `gh pr view <base-pr> --json state,mergedAt,headRefName` shows the merge. In the
   read call take `$merged_branch` from that PR's `headRefName` and `$remote` from the resolver. Run
   `git ls-remote --heads -- "$remote" "refs/heads/$merged_branch"` to check whether the branch still exists. After a
   successful call, count only a row whose ref field is exactly `refs/heads/$merged_branch`.
   `gh pr view <next-pr> --json state,baseRefName,isDraft` shows where the next PR points. If the base PR is not merged,
   this section does not apply. If the next PR is closed, stop and report it. Whether to reopen it is the user's call.
2. **Point the next PR at trunk.** When `baseRefName` differs from trunk, load the resolved trunk name from its producer
   or scratch record as `$trunk` and run `gh pr edit "$next_pr" --base "$trunk"` with the verified PR number.
3. **Merge trunk into the next branch.** Use the fetch, switch and merge steps in [Rebase ripple](#rebase-ripple). Set
   `$layer_branch` from the next PR's confirmed head and `$base_branch` from the resolved trunk. The next step handles
   the push.

4. **Hand the push to `git push`.** `<next-branch>` is pushed with no force. Review history on the PR stays intact.
5. **Bring the layers above current.** Each pushed layer takes the merge path from Rebase ripple, bottom-up.
6. **Check the stack, then re-link only when needed.** Merged PRs stay in their stack on GitHub. `gh stack link` only
   adds. It refuses a list that leaves out a PR already in the stack. Run the main skill's definition-of-done check on
   `<next-pr>`. Then:
    - If that stack still holds every surviving PR in order, it needs no link.
    - If that stack holds a different set, stop and report it.
    - If `<next-pr>` is in no stack and two or more PRs survive:
        - Run `gh stack link --base <trunk> <next-pr> <…> <top-pr>` in bottom-to-top order.
        - `--open` creates ready pull requests ([Repository policy](../SKILL.md#repository-policy)).
        - Then rerun the check.
    - If a stack still holds some survivors and a survivor is missing, append it with
      `gh stack link <stack#> <missing-pr>`. Use this form instead of a new list of the whole stack.

    Without `--base`, `link` sets the bottom PR's base to the repository's default branch. `link` takes at least two
    PRs, so a lone survivor needs no link. Confirm that its base is `<trunk>`.

The rebase path still fits a layer above that was never pushed. It also fits a pushed layer that the user explicitly
asks to rewrite. That rewrite follows the Rebase ripple rewrite path.

## Agentics safety rules

The rules for drafts and merges are in [Repository policy](../SKILL.md#repository-policy). This section states what each
command does. With `policy.draft_prs` off, `--open` and `gh stack merge` follow Repository policy.

- **`gh stack submit`** run interactively opens an editor where new PRs default to **ready for review**.
  `gh stack submit --auto` creates new PRs as **drafts**. `--open` marks new and existing PRs ready for review.
- **`gh stack link`** creates a PR for any branch that has none. The extension would also push its branch arguments.
    - Each branch argument is pushed through `git push` first, so `link` only links.
    - It creates new PRs as drafts unless `--open` is passed.
    - The help text (`gh stack link --help`, v0.1.1) documents only the flag, "Mark new and existing PRs as ready for
      review".
    - It does not document the default, so treat draft-by-default as inferred.
    - `--open` marks new and existing PRs ready for review.
    - After `gh stack link` runs, confirm the resulting PRs with `gh pr view <n> --json isDraft`.
- **`gh stack merge`** merges every PR in the stack up to your chosen point in a single all-or-nothing operation.
  `--yes` skips the confirmation prompt entirely.
- **`gh stack alias`** installs a wrapper (default `gs`) into `~/.local/bin/`. An alias hides the command's real name,
  so `gs merge` does not read as a merge. `gh extension exec` and the extension binary run directly hide it the same
  way. Only the spelled-out `gh stack …` form shows which command runs. Spell out `gh stack …`; do not run
  `gh stack alias`, and do not run the stack through an alias, `gh extension exec` or the extension binary.
