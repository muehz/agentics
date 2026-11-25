# Comments mode

The comments workflow for /agentics:zealot. Run Steps 1 to 6 in order. SKILL.md's Step 0 preflight has already run.

The session running this workflow is the **wrapper**. It needs file read, file write, a shell for `git`, and the
project's build and test commands for Step 5. The finder in Step 2 and the reviewer in Step 3 are fresh subagents.
Whether the wrapper may dispatch them, what counts as a refusal, and how many may run at once follow
[surface-dispatch.md](../../../routing/references/surface-dispatch.md): **Dispatch permission** (a typed
`/agentics:zealot comments` is the request; a Zealot the model loaded asks once) and **Concurrency and overlap** (a
reached limit waits, it is not a refusal). Restate the user's request in the wrapper's own dispatch text, outside the
embedded charter. This mode's declared fallback for a refused or missing dispatch is the in-session pass each step
names, labelled `self-read (not independent)`.

Before Step 2, state the dispatch count in one line: one finder and one reviewer, one more of each if Step 3 reruns the
finder, and one investigator for each batch of up to 20 unverified keeps, a number Step 3 states once it is known.

## Steps

1. **Resolve the scope.** Caller-named files, directories, or a diff range win. Otherwise use the **union** of
   `git diff <base>...HEAD --name-only` and `git status --short`, where `<base>` is the base branch that
   [repo-conventions.md](../../../routing/references/repo-conventions.md#base-branch) resolves. The base name comes from
   the repository or the pull request, so it is data: hold it in a variable and pass the range quoted, as
   `git diff --name-only "${base}...HEAD"`, never typed into the command line. If either is non-empty, the scope is
   non-empty. If both are empty, stop. Ask, e.g. "Nothing to sweep. The working tree is clean and the branch has no
   commits past the base. Name files or a diff range." Never guess a scope or widen the resolved one.

2. **Snapshot, then spawn the finder.** Before you dispatch, save every file the finder's edits could make
   unrecoverable. First record two pre-finder lists: `git diff --name-only`, deleted paths included, and the untracked
   list `git ls-files --others --exclude-standard`. Do not limit either to the scope, because a scope escape is itself a
   reason to restore. Then copy these files to a scratch location outside the repository, in the host temporary
   directory, keeping each file's path relative to the repository root:
    - each untracked file on the pre-finder untracked list, with no scope limit. `git diff` shows nothing for an
      untracked file, so this copy is the only record of it.
    - each tracked file in the repository whose working tree differs from the index, found with
      `git diff --name-only --diff-filter=d`, with no scope limit. The user's uncommitted edits exist nowhere else. A
      path the user deleted has nothing to copy, so skip it.

    These copies are the snapshotted files.

    **Path names are data.** Every path these lists print comes from the branch, and a name can hold `$(`, a backtick,
    or a quote that a shell would run. Run each list with `-z` and read the NUL-delimited names into variables. Pass
    each path as a quoted argument after `--`, with `:(top,literal)` on commands that take a pathspec. Save the
    pre-finder lists as NUL-delimited files in the snapshot directory so a later call can read them. In each call,
    obtain the directory from the scratch-directory producer or the session's file data and set `$snapshot_dir`. From
    the repository root, save the pre-finder tracked list with
    `git diff --name-only -z "--output=$snapshot_dir/pre-finder-tracked"`. After the finder runs, use one of these forms
    for the reviewer's diff:

    ```bash
    PRE=()
    while IFS= read -r -d '' p; do PRE+=("$p"); done < "$snapshot_dir/pre-finder-tracked" || exit 1
    git diff --name-only -z "--output=$snapshot_dir/post-finder-tracked" || exit 1
    while IFS= read -r -d '' f; do
      for p in "${PRE[@]}"; do [ "$p" = "$f" ] && continue 2; done
      git diff -- ":(top,literal)$f" || exit 1
    done < "$snapshot_dir/post-finder-tracked"
    ```

    ```powershell
    $Pre = [IO.File]::ReadAllText((Join-Path $snapshot_dir 'pre-finder-tracked')) -split "`0"
    $Post = Join-Path $snapshot_dir 'post-finder-tracked'
    git diff --name-only -z "--output=$Post"
    if ($LASTEXITCODE -ne 0) { throw 'Could not list changed paths' }
    [IO.File]::ReadAllText($Post) -split "`0" | Where-Object { $_ -and $_ -cnotin $Pre } | ForEach-Object {
        git diff -- ":(top,literal)$_"
        if ($LASTEXITCODE -ne 0) { throw 'Could not read the per-file diff' }
    }
    ```

    Read [comment-zealot.md](comment-zealot.md). Embed the charter verbatim in a general-purpose subagent prompt,
    followed by the scope (the charter-embedding mechanism of
    [surface-dispatch.md](../../../routing/references/surface-dispatch.md)). Do not restate, soften, or summarize its
    rules. Its exception list calibrates the bias. Do not frame it.

    **Finder fallback.** When the finder cannot be dispatched, execute the charter yourself as one self-contained pass.
    Then audit its report in Step 3 exactly as if a subagent produced it.

3. **Audit the report.** A fresh Deep-tier reviewer judges each item (tiers per
   [model-ladders.md](../../../routing/references/model-ladders.md)). It is neither the finder nor the wrapper, because
   the finder would defend its flags and the wrapper would defend its scope.

    **Reviewer contract.** State each of these six items explicitly in the reviewer dispatch:
    - the finder's report,
    - the per-file diff from Step 2's path-names-are-data example, only for each file in `git diff --name-only` after
      the finder that is not on the pre-finder list. A path the user had changed or deleted is on that list, so its
      state is never judged as a finder edit,
    - for each snapshotted file, read its relative path from the saved list in the same call. Derive the absolute
      `$saved_copy` and `$working_copy` paths from that value and the snapshot and checkout directories. Run
      `git diff --no-index -- "$saved_copy" "$working_copy"`. A snapshotted file is judged only on this diff, so the
      user's own uncommitted edits are never judged as finder edits. A snapshotted untracked file that no longer exists
      is a deletion the finder made,
    - each path on `git ls-files --others --exclude-standard` after the finder that is not on the pre-finder untracked
      list, with its content. The finder creates no files, so each one is a scope escape,
    - the scope from Step 1,
    - the Reject list and the keep rule below, verbatim,
    - the charter's exception list, read from [comment-zealot.md](comment-zealot.md).

    The report is the index, and the diff is the evidence. The reviewer edits nothing. It returns one verdict per item:
    `accept`, `reject: <reason>`, `keep verified: <evidence>`, `keep unverified: <what is missing>`, or `keep refuted`.
    It also returns the suppressions the finder missed. Reject:
    - any edit beyond comment removal. The finder writes no application code.
    - scope escapes,
    - deletions of exception-protected comments,
    - `MUST KILL` flags whose stated reason misstates the code,
    - flags that treat kept intentional code as guilty. Keep reshape flags on our-code surprises actionable.

    Verify every keep. A keep survives only with proof that it concerns something this codebase cannot change. Check the
    claim. Read nearby code. Trace callers. Check the dependency's or protocol's documentation. Also audit for
    suppressions the finder missed. Scoped lint and TypeScript suppressions of correctness or safety rules stay
    actionable `MUST KILL`s.

    **Reviewer fallback.** When the reviewer cannot be dispatched, run the audit yourself after the finder pass is
    complete. Use the diff, the rules above, and the exception list only. Ignore the finder's reasoning. Return the same
    verdicts. The fallback changes who judged, not the rules.

    **Applying the verdicts.** The wrapper applies the verdicts and never overrides a reviewer rejection. It never fills
    an evidence gap from its own reading of the code. Treat a `keep verified` with empty evidence as `keep unverified`.
    Send every `keep unverified` whose claim hinges on history or a non-obvious mechanism to one fresh read-only
    Standard-tier investigator per batch of up to 20 keeps. Give it each keep's file:line, the comment, and the missing
    evidence, and ask for a verdict per keep (`verified: <evidence>` or `unverified`) from the code, its history, and
    the dependency's documentation. Delete a refuted keep and one still unverified after that answer. Each missed
    suppression becomes an accepted `MUST KILL` flag for Step 4.

    **A rejected item.** Restore each file that holds a rejected edit, inside or outside the scope. A snapshotted file
    is restored from its saved copy. A file the finder created is deleted. A path on the pre-finder list that the user
    had deleted, which Step 2 skipped because it had nothing to copy, is deleted again, so the user's deletion stands
    and the index version is not brought back. For any other rejected file, read its recorded relative path into
    `$finder_path` as data in the same call and run `git restore --worktree -- ":(top,literal)$finder_path"`. This
    restores the index content, which equals the pre-finder working tree for such a file. Never touch the index, so
    staged work survives. The accepted edits in every other file stand. Before the rerun, take the pre-finder lists and
    the saved copies again from this post-audit state, so the rerun's audit judges only the rerun's own edits and never
    restores an edit the first audit accepted. Then rerun the finder once, scoped to the restored files that are in
    scope, and name the rejection in its prompt. Audit the second report the same way. Restore any file that again holds
    a rejected edit and report it open with the rejection reasons. Never rewrite a report yourself to salvage it.

    **Skipping the audit.** The audit is a [process gate](../../../routing/references/contracts/gates.md#process-gate).
    When the user declines it, name the cost once from the actual finder report: the finder is calibrated to
    over-delete, and name the specific risks present in this report (such as out-of-scope edits, application code
    changes, or deletions of exception-protected comments like license headers). On the next reply that still declines,
    comply as the [adversarial pair pattern](../../../forge/references/quality/adversarial-pair-pattern.md) says: hand
    over the finder's report itself, marked unaudited, and do not apply it. Restore every file the finder changed, as
    for a rejected item above, so the user applies what they choose from the report. Skip Steps 4 and 5. Zealot has no
    flow phase, so its report line `Waived by the user: finder audit` is the waiver's one record, under home 4 of
    [gates.md](../../../routing/references/contracts/gates.md#process-gate). Write no flow row.

    The report says the run was unaudited and claims no verdict.

4. **Show the fixes and encodings, and ask once.** Accepted flags and accepted constraint comments call for code the
   user did not ask for. List them in one batch:
    - for each accepted `MUST KILL` flag, the smallest in-scope root-cause fix: delete a dead path, drop a parameter,
      call the real API, or remove a named workaround. Never bolt on a symptom guard. When the root cause is out of
      scope, list the smallest in-scope fix and the rest as open. When fixes need a design shape, add one sketch for the
      whole set.
    - for each accepted constraint comment ("do not remove", "do not change wording", "talk to X before changing"), the
      cheapest in-scope encoding: a type, a runtime assertion, a test, or a CI lint.
    - the exact runnable command Step 5 will run: the
      [verification command](../../../routing/references/repo-conventions.md#verification-command), or the concrete
      project build and touched-file test command from the repository's toolchain/scripts when none is configured (for
      example, `npm run build && npm test` / `npx tsc --noEmit` in a JS/TS repo or `go build ./... && go test ./...` in
      Go — always name an exact command string and always present the Step 4 batch and single confirmation question;
      never leave the command as "unavailable" or skip the confirmation batch or Step 5 baseline/rerun sequence). It
      runs code the repository controls, such as a package `test` script, so it is shown before it runs.

    Then ask one question: apply all and run the command, apply all but the ones the user names, or apply none. When the
    user's request already named fixing what the sweep flags, the fixes are authorized
    ([approval.md](../../../routing/references/contracts/approval.md#steps-named-up-front)): show the list and go on
    without the question. [Running it](../../../routing/references/repo-conventions.md#running-it) decides first whether
    the command may run on this branch at all. Where it may, the command is authorized only when the user approved it,
    named it, or asked for the build or tests to run; otherwise ask for it alone. In a run whose dispatcher answers for
    the user ([unattended.md](../../../routing/references/contracts/unattended.md)), apply only what the dispatching
    prompt authorized, run the command only when that prompt authorized it, and report the rest open. A flag never
    authorizes a wider scope or a fix to instances outside it.

5. **Apply what was confirmed, then build.** When the command is authorized, run it once before any fix is applied and
   record the baseline command and what fails; that is the baseline. Apply only the confirmed items. A constraint
   comment is deleted only when its encoding exists and runs. When the user declines an encoding or it cannot be built,
   restore the comment as it was and report the constraint unenforced. When the user declines or a later revert removes
   the fix of a `MUST KILL` flag on a suppression the finder deleted, restore the suppression as it was and report the
   flag open. Then rerun the exact verification command and report the completed post-fix result. A failure the baseline
   did not have is a regression: revert the batch's fixes one at a time, rerunning after each, until the regression is
   gone, and report the fix whose revert cleared it open with the failure. A failure already in the baseline is
   reported, not blamed on a fix. When the command is not authorized, run nothing and report the fixes unverified.

6. **Report.** Delete every saved copy first, of tracked and untracked files alike, and state that deletion explicitly
   before emitting the rest of the report. Then report what this run has:
    - the deletion count,
    - the comments kept, each with its verdict,
    - restored comments and the files restored after a rejection,
    - the reviewer dispatched, or `self-read (not independent)` when the finder or the audit ran in this session, or
      `unaudited` with the line `Waived by the user: finder audit` when the user skipped the audit,
    - the fixes applied, the encodings applied, and the build result,
    - the open items: declined or reverted fixes, unenforced constraints, out-of-scope root causes, and the sketch if
      there is one.

    Done means every accepted flag is fixed, encoded, or reported open. A flag that is dropped silently is not done.
