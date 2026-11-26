# Push

## Overview

This file is the workflow for every push the git skill runs. It covers these requests:

| Request                  | What it does                                    |
| ------------------------ | ----------------------------------------------- |
| `push`                   | Pushes the current branch to its push remote    |
| `push <branch>`          | Pushes that one branch to its push remote       |
| `push tag <name>`        | Pushes that one tag                             |
| `push force [<branch>]`  | Force-pushes one branch, after one confirmation |
| `push delete <branch>`   | Deletes that one branch on the push remote      |
| `push delete tag <name>` | Deletes that one tag on the push remote         |

Read each input in the same shell call that uses it. Capture output in a variable or put it in a work-item scratch file
and read it back with command substitution in Bash or `Get-Content -Raw -LiteralPath` in PowerShell. Pass user-supplied
values through the file tool or structured arguments. In each block, `remote` is the resolved push remote and `push_url`
is its sole push URL. Build `target_ref` from the selected branch or tag name with the `refs/heads/` or `refs/tags/`
prefix. Before using it, verify its format with `git check-ref-format "$target_ref"`. The blocks use the same argument
forms in Bash and PowerShell 7.

For each successful `ls-remote`, keep only the row whose tab-separated ref field equals `target_ref`. Git treats the
argument as a pattern, and a different ref can end in the same text. An exact-match row supplies the remote object ID.

No command in this skill uses a bare `--force` or `-f` on a push.

Every push in this file sends the one ref the request names, and every push command passes `--no-follow-tags`. Without
it, a repository with `push.followTags` set to `true` also publishes each annotated tag that the pushed commits make
reachable, and the user named none of them. A tag goes only through `push tag <name>`, which sends that one tag, or
through an explicit request for all tags (below).

When a `git ls-remote` or a `git fetch` in this file fails, report git's message and stop. Ask for no confirmation and
run no push.

Every push names its destination ref after a colon. Without one, a `remote.<name>.push` mapping can send the branch to
another ref than the one the checks below read.

## Authorization

A push runs in one of these cases:

- The user's request names the push or the pull request.
- The user invokes `/agentics:git push`.
- A calling skill passes on such a request. Gate's draft pull request destination is one.

A commit, an approved plan or a passing gate never authorizes a push.

With no authorization, ask once whether to push, and name the branch and the remote in the question. Push on a yes.

When a request names several steps, such as "commit and push", say in one line at the start which steps you took as
authorized. Then run them in the order the request gave, with no second question about the push.

A force-push and the deletion of a remote ref each need their own request and their own confirmation. See
[Force-push](#force-push) and [Delete a remote ref](#delete-a-remote-ref).

## Before the push

1. Read the branch. Load a named branch from the request's data. Otherwise, capture the following command in the same
   call or retain its output in a scratch file.

    ```bash
    git branch --show-current
    ```

    A push of the current branch must run on a branch. When the command prints nothing, `HEAD` is detached: stop and say
    that a push needs a branch. Creating one is [branch.md](branch.md).

2. Resolve the push remote and the protected set. Run the resolver in [base-and-remote.md](base-and-remote.md) and take
   its `push-remote` and `protected` lines. Put a push remote the user named on the resolver's fourth input line.
    - When `push-remote` reads `unresolved`, ask the `question` line for the push remote, fill in the answer and run the
      resolver again. Leave the other `question` lines unasked.
    - When `push-remote` reads `none`, the repository has no remote. Stop and report that.

    Read the selected remote from the resolver output into `remote`, then read its push URL. A push writes to that URL.
    The fetch URL is used when the remote sets no `pushurl`.

    ```bash
    git remote get-url --all --push -- "$remote"
    ```

    `push_url` in every `ls-remote` and `fetch` command in this file is that URL, so each read sees the repository the
    push changes. When the command prints more than one URL, the push writes several repositories that one read and one
    lease cannot cover: stop and report the URLs.

3. Fetch the branch from the push remote. The resolver fetches only the base branch, so this fetch is your own. First
   read whether the remote holds the branch.

    ```bash
    git ls-remote --refs -- "$push_url" "$target_ref"
    ```

    Split the successful output into lines and each line into its tab-separated object ID and ref. Discard rows whose
    ref is not exactly `target_ref`. If there is one exact row, use its validated full object ID as `remote_oid`, fetch
    the branch, and count the commits the remote does not hold. Multiple exact rows are ambiguous: stop and report them.

    ```bash
    git fetch -- "$push_url" "$target_ref"
    git rev-list --count "$remote_oid..$target_ref" --
    ```

    When no exact row matches, the remote does not hold the branch and this is its first push. Use local tracking refs
    to estimate the preview count. The final [session artifact check](#check-session-artifacts-before-pushing)
    establishes the history at the push URL.

    ```bash
    git rev-list --count "$target_ref" --not "--remotes=$remote" --
    ```

4. Report the branch, the remote and the number of commits to be sent, before the push runs.

    ```text
    Branch:          <branch>
    Remote:          <remote>
    Commits to send: <count>
    ```

**A branch that is not the current one.** The resolver reports the push remote of the current branch. `push <branch>`,
`push force <branch>` and `push delete <branch>` can name another branch. For that branch, a push remote the user named
comes first. Otherwise read git's push configuration in the order the resolver uses, and take the first command that
prints a name. Whoever pushed a branch chose its name, and a name can hold `$(` or a backtick, so never type it into a
command. Write it to a file with the file-write tool, read the file into `branch`, and pass only the quoted variable:

```bash
branch=$(cat "$BRANCH_FILE")
git config --get "branch.${branch}.pushRemote"
git config --get remote.pushDefault
git config --get "branch.${branch}.remote"
```

A `.` from the last command is not a remote. When none prints a name, use the only remote. With several remotes and no
answer, ask the user which remote the branch pushes to. The `protected` line still comes from the resolver.

## Check session artifacts before pushing

Run this check just before every content-publishing push. It applies to a first or follow-up push, a push after a merge,
a force-push, and an authorized tag push. Deleting a remote ref sends no artifact content. Keep the exact remote, push
URL, source object IDs, and destination refs selected by the authorized operation. A push to one ref does not authorize
following tags or sending other refs. For an explicitly authorized set of tags, identify the full selected set. Resolve
annotated tag targets before deciding which commits they introduce. If an object type or target prevents you from
establishing what would go out, hold the push.

Establish the outgoing commits for the selected refs against current evidence of the repository at the actual push URL.
Use a range that covers the full outgoing set. Exclude a commit from inspection only when verified, current destination
reachability shows that the repository already holds it. The local remote-tracking refs used for a first-push count may
be stale or refer to a different URL. Do not use them as proof that a commit is already there. When exclusions are
uncertain, use a verified superset that includes every possible outgoing commit, or hold if you cannot establish and
inspect that scope.

Inspect the trees of the outgoing commits, including the full outgoing merge ancestry. Apply the
[session artifact identification rules](commit.md#check-session-artifacts-before-committing) to the paths and actual
blob bytes in each included commit. This checks earlier versions even when the branch tip changes, renames or deletes
them. A working tree, tidy tip or range diff does not represent that history. Read blobs by object ID through local Git
reads. Apply the
[shared redaction rule for session artifacts](../../routing/references/contracts/redaction.md#local-handoffs-and-resume-capsules)
and the [outbound checks](../../gh/references/posting-envelope.md#outbound-checks) to the bytes, using the visibility of
the actual destination repository and the user's applicable embargo. Unknown visibility gets the public-repository
checks.

If the scope or any relevant bytes are indeterminate, or a content check fails, hold the entire push. Report the reason
and what was withheld without including prohibited content. Keep the index and local history as they are. Offer the
existing private-channel alternative for an unfixed public security finding. Only the user lifts an embargo. A
previously approved commit or push does not approve different content or lift a hold.

Before sending, confirm that the selected refs still name the checked source objects and that your evidence still covers
the actual destination and scope. Recheck any changed inputs before proceeding, or hold if they cannot be rechecked.
Preserve the force-push lease pinned at confirmation. If the confirmed force-push scope changes, stop rather than
refreshing the lease or reusing the confirmation for different content.

## Push

A plain push sends one branch to the push remote. It is the same push whether or not the branch has a pull request.

**A branch in the protected set.** When the branch is on the `protected` line, the push runs only when the request names
that branch. When the request does not name it, ask once, name the branch in the question, and push on a yes.

Run the [session artifact check](#check-session-artifacts-before-pushing) before sending the selected branch.

**The first push of a branch.** It is the first push when `ls-remote` in step 3 of [Before the push](#before-the-push)
found no exact row, because the remote does not hold the branch. Do not decide it from the branch's upstream: a branch
created from a remote-tracking start point already tracks that start point. The first push sets the upstream to the
branch on the push remote. The upstream that a first push records is part of the push. It is not a config edit.

```bash
git push --no-follow-tags --set-upstream -- "$remote" "${target_ref}:$target_ref"
```

**A follow-up push.** When the remote already holds the branch, the push is a plain push.

```bash
git push --no-follow-tags -- "$remote" "${target_ref}:$target_ref"
```

**A tag.** `push tag <name>` pushes that one tag. Of the steps in [Before the push](#before-the-push), a tag push runs
step 2 to resolve the push remote. Report the tag and remote. It needs no branch and reports no commit count. Run the
[session artifact check](#check-session-artifacts-before-pushing) for the selected tag before sending it.

```bash
git push --no-follow-tags -- "$remote" "${target_ref}:$target_ref"
```

Git never pushes all tags unless the user asks for all tags in their own words. Only then does the push carry `--tags`:
list the tags it will send, then run the command below. `--no-follow-tags` stays on, so no tag goes that the list does
not name.

```bash
git push --no-follow-tags --tags -- "$remote"
```

The [session artifact check](#check-session-artifacts-before-pushing) covers the whole selected set before that push.

When the push fails, report git's message. Do not report a push that you did not verify. Verification is in
[After the push](#after-the-push).

## A rejected push

The remote rejects a plain push when it holds commits the local branch does not. When that happens:

1. Report the rejection, quoting git's actual rejection line verbatim (for example, `! [rejected] ... (fetch first)`)
   directly in the response to the user.
2. Make no second attempt.
3. Use no force flag. A rejected push is never forced.
4. Offer to merge the remote tip and push again. When the rejected branch is not the current branch, offer to switch to
   it first as in [branch.md](branch.md#create-and-switch), which also covers a switch that uncommitted changes block,
   then merge and push again; git never merges into a branch other than the one being pushed.

When the user accepts the offer, fetch and merge as below. The no-commit option stops a divergent merge before it
creates its commit and preserves normal fast-forward behavior.

```bash
git fetch -- "$push_url" "$target_ref"
git merge --no-commit --no-edit FETCH_HEAD
```

If Git reports a fast-forward or that the branch is already up to date, no merge commit needs to be created. For a merge
that stops on conflicts, use [merge-conflicts.md](merge-conflicts.md) and complete its resolution, staging, and check
steps. With or without conflicts, when a merge commit is pending and the index is resolved, apply the
[session artifact check before committing](commit.md#check-session-artifacts-before-committing) to the final staged
candidate before the merge commit. Honor the existing authorization and
[commit contract](commit.md#the-commit-contract). On a pass, conclude the merge with `git commit --no-edit`, the same
commit used by the conflict workflow. That commit stays on the branch being pushed. On a hold, stop. Report the reason
in sanitized form, leave the merge state and index intact, and do not retry the push. A failed merge that did not reach
a valid resolved candidate also stops the retry.

When the merge completes or Git reports a fast-forward or up-to-date result, start again at
[Before the push](#before-the-push) and run the [session artifact check](#check-session-artifacts-before-pushing) on the
resulting outgoing scope before retrying.

Rebase onto the remote tip only when the user asks for a rebase. That workflow is
[history-operations.md](history-operations.md#rebase).

## Force-push

`push force [<branch>]` is the request that names the force-push. Nothing else starts one. A rebase or another rewrite
of pushed commits ends with the user's own `push force` request, never with a force-push of its own.

A force-push is a destructive step. It gets one confirmation at the point of action, even when the request already named
the branch, and it gets no second one.

1. **Read the branch.** With a branch argument, the candidate is that branch. With no branch argument, the candidate is
   the current branch, read as in [Before the push](#before-the-push). On a detached `HEAD` with no argument, stop and
   report it.

2. **Refuse the protected set.** Resolve the push remote and the protected set as in
   [Before the push](#before-the-push). When the candidate is on the `protected` line, do not push. Say these three
   things once:
    - What is held: the force-push to that branch.
    - Why: the branch is a default branch, the base branch, or protected on the host. Other work builds on it, and a
      force-push removes commits that others already have.
    - What to do instead: a revert commit that undoes the change, or a new branch that carries the rewritten history.

    When the user repeats the demand, answer with one line that points to that alternative.

3. **Read the remote SHA.** Read the SHA the remote holds for the branch, then fetch the branch so those commits exist
   locally.

    ```bash
    git ls-remote --refs -- "$push_url" "$target_ref"
    ```

    Retain the validated full object ID from the one row whose ref field exactly equals `target_ref` as `remote_oid` for
    steps 4 and 6. If there is no exact row, use a plain push as in [Push](#push). If there are multiple exact rows,
    stop and report the ambiguity. Fetch the branch only after this check supplies `remote_oid`.

    ```bash
    git fetch -- "$push_url" "$target_ref"
    ```

4. **Show what the remote will lose.** List the commits on the remote that the push will remove.

    ```bash
    git log --oneline "$target_ref..$remote_oid" --
    ```

    Show the branch, the remote, the remote SHA and that list, taken from git's output.

5. **Wait for one reply.** The reply confirms only when it names the action or the branch in the user's own words. When
   the request gave no branch argument, the reply must name the branch. A bare "yes" does not confirm. Without a
   confirming reply, do not push, and report that the force-push did not run.

6. **Push with the pinned lease.** Run the [session artifact check](#check-session-artifacts-before-pushing). Pin the
   lease to the SHA you showed in step 4. Do not read the SHA again to refresh that lease.

    ```bash
    git push --no-follow-tags "--force-with-lease=${target_ref}:$remote_oid" -- "$remote" "${target_ref}:$target_ref"
    ```

7. **Report a failed lease.** When git rejects the push because the remote no longer holds that SHA, report the
   rejection with git's message and stop. Do not retry.

After a push that git accepted, verify it as in [After the push](#after-the-push).

## Delete a remote ref

`push delete <branch>` and `push delete tag <name>` delete that one ref on the push remote. Each is a destructive step.
It gets one confirmation at the point of action, even when the request already named the ref, and it gets no second one.

1. **Resolve the remote.** Resolve the push remote and the protected set as in [Before the push](#before-the-push).

2. **Refuse the protected set.** When the branch is on the `protected` line, do not delete it. Say these three things
   once:
    - What is held: the deletion of that branch on the remote.
    - Why: the branch is a default branch, the base branch, or protected on the host. Every clone and every open pull
      request that targets that branch loses it.
    - What to do instead: delete a feature branch, or keep the branch.

    When the user repeats the demand, answer with one line that points to that alternative.

3. **Read the remote ref.** Build `target_ref` with the appropriate branch or tag prefix.

    ```bash
    git ls-remote --refs -- "$push_url" "$target_ref"
    ```

    Retain the validated full object ID from the one row whose ref field exactly equals `target_ref` as `remote_oid`. If
    no exact row or more than one exact row matches, report that and stop.

4. **Show what will be lost.** Fetch the ref, then show what it holds. For a branch, list the commits that the
   resolver's `base-ref` does not hold. For a tag, show the tag.

    ```bash
    git fetch -- "$push_url" "$target_ref"
    ```

    For a branch, resolve the base reported by the resolver to `base_oid`. Qualify named refs and verify the commit with
    `git rev-parse --verify --end-of-options`, using the `^{commit}` suffix. Run
    `git log --oneline "$remote_oid" --not "$base_oid" --`. For a tag, run `git show --no-patch "$remote_oid" --`. Show
    the ref, the remote, the remote SHA and that output. When the resolver reports an unresolved or missing base, show
    the tip commit and say that the commits only this branch holds could not be listed.

5. **Wait for one reply.** The reply confirms only when it names the deletion or the ref in the user's own words. A bare
   "yes" does not confirm. Without a confirming reply, do not delete, and report that the deletion did not run.

6. **Delete the ref with the pinned lease.** Read the same `target_ref` and `remote_oid` you showed in step 4.

    ```bash
    git push --no-follow-tags "--force-with-lease=${target_ref}:$remote_oid" --delete -- "$remote" "$target_ref"
    ```

    When git rejects the deletion because the remote no longer holds that SHA, report the rejection with git's message
    and stop. Do not retry.

7. **Verify.** Read the remote ref again as in step 3. Report "deleted" only when the successful read has no exact row.

## After the push

Read the remote ref and the local ref with the same `target_ref` used for the push.

```bash
git ls-remote --refs -- "$push_url" "$target_ref"
git rev-parse --verify --end-of-options "$target_ref"
```

Report "pushed" only when `git ls-remote` actually ran in this run, returned one remote row whose ref field equals
`target_ref`, and its object ID equals the local SHA. Report the branch or tag, the remote and that SHA. If the push did
not run, state that explicitly and give the exact commands instead of reporting "pushed" or an invented SHA. When the
two differ, or the remote does not hold the ref, report what each side holds and do not say "pushed".
