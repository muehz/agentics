# Tests mode

The mutation-review workflow for `/agentics:zealot tests`. Run Steps 1 to 8 in order. SKILL.md's Step 0 preflight has
already run. This mode never deletes or edits a test. It proves which tests prove nothing and names the next step for
the remove-or-replace work.

The session running this workflow is the **wrapper**. It needs file read, file write into the mutant root, and a shell
for `git` and the project's test command. If a step cannot run for want of one of these, the affected candidates are
`unproven`. Never improvise a substitute, never substitute plain copies for the Step 2 worktrees, and never skip the
snapshot. Name the step in the report and continue with the steps that can run.

The finder in Step 4 and the reviewers in Step 5 are fresh subagents. Whether the wrapper may dispatch them, what counts
as a refusal, and how many may run at once follow
[surface-dispatch.md](../../../routing/references/surface-dispatch.md): **Dispatch permission** (a typed
`/agentics:zealot tests` is the request, and so is a test audit the user asked `tdd` for and `tdd` forwarded; a Zealot
the model loaded asks once) and **Concurrency and overlap** (a reached limit waits, it is not a refusal). Restate the
user's request in the wrapper's own dispatch text, outside the embedded charter. This mode's declared fallback for a
refused or missing dispatch is the in-session pass Steps 4 and 5 name, labelled `self-read (not independent)`. The run
never stops at an unavailable dispatch with no proof attempt.

## Steps

1. **Resolve the scope.** Explicit paths win: caller-named test files, directories, or a diff range. Otherwise use the
   default scope. Changed files are the **union** of `git diff <base>...HEAD --name-only` and `git status --short`,
   where `<base>` is the base branch that
   [repo-conventions.md](../../../routing/references/repo-conventions.md#base-branch) resolves. The base name is data:
   hold it in a variable and pass the range quoted, as `git diff --name-only "${base}...HEAD"`.
   [Comments mode](../comments/workflow.md) shares this definition. Tests in scope are these:
    - the changed test files,
    - every test in the same package or directory as a changed production file,
    - the tests of packages that import a changed production package, when the project can enumerate importers. In Go,
      run `go list -f '{{.ImportPath}}: {{join .Deps " "}}' ./...` and keep the lines whose dependency list contains the
      changed package. Other languages use their own dependency lister.

    If the project cannot enumerate importers, ask which importing packages to include. Never guess. If the default
    scope is empty, stop. Ask, e.g. "Nothing to review. The working tree is clean and the branch has no commits past the
    base. Name test files, a directory, or a diff range."

    Only two things widen the scope to the whole project: the explicit word `all`, and the user's own request for the
    whole project in words. Split it into at most eight nonempty package groups balanced by test count. Count with the
    project's test lister where one exists (Go: `go test -list . ./<package>` per package). Otherwise count the
    language's test declarations, such as `^func Test` in Go or `def test_` in Python. Approximate balance is fine. The
    cap is not. Every nonempty group gets its own Deep reviewer in Step 5.

    Before any dispatch, state the exact selection concretely to the user: list every selected test file and package by
    name, and when `all` applies, list every concrete balanced package group (<= 8 groups) with its member packages and
    test counts by name before dispatching. State the dispatch count in the same message (one finder for the whole
    selection and one Deep reviewer per group) and show the dispatch line. The wrapper owns scope: pass it explicitly to
    the finder in Step 4.

2. **Determine the mutant root, snapshot the caller, and prepare the mutant worktrees.**

    Do these in that order, because the snapshot files live under the mutant root. Create the worktrees only after the
    snapshot exists.

    - **Mutant root.** Mutant worktrees live only under the mutant root `<OS temp directory>/zealot-mutants/`. Determine
      it in this order. First, if the host granted an additional working directory whose basename is `zealot-mutants`,
      use it. Second, if the surface lets you read environment variables, use the OS temporary directory (`$TMPDIR`,
      `$TEMP`, or `$TMP`) joined with `zealot-mutants`. Third, if neither applies, ask the caller once for the mutant
      root. Stop until the caller answers. Do not work around a refused environment read. Refuse a root that lies under
      `git -C <caller> rev-parse --show-toplevel` or under `git -C <caller> rev-parse --git-common-dir` resolved to an
      absolute path. Mutants never run inside the repository tree.
    - **Snapshot.** Run these three commands in the caller checkout: `git rev-parse HEAD`,
      `git status --porcelain=v2 --untracked-files=all`, and `git worktree list --porcelain`. Write each output to its
      own file under `<mutant-root>/<run-id>/caller-snapshot-before/` with the session's file-write capability, which
      creates parents. `<run-id>` is a name that this run picks once. It is unique on the host, so concurrent runs never
      share a snapshot. An example is `snapshot-` plus a UTC timestamp and a random suffix. Step 6 compares against
      these files. If a snapshot write is refused, keep the three outputs in the session record. Then report in Step 6
      that the comparison ran from memory. A refused snapshot write never stops the run.
    - **Owner.** Between Step 4's candidates and Step 5's reviewer dispatch, the wrapper creates one worktree per
      candidate with `git -C <caller> worktree add --detach <mutant-root>/<unique-name> HEAD`. Git creates missing
      parents. Save the absolute caller and worktree paths as literal data in this run's scratch directory for Step 6.
      Step 6 removes exactly these recorded worktrees. Pass the paths to the reviewer with its candidates. Reviewers
      create no worktrees of their own. Before creating them, state the worktree count and the test runs it implies: per
      candidate, a baseline and a mutant run of the named test and one mutant run of the package and importer tests,
      plus one original-tree run of the package and importer tests per package.
    - **Carry caller state.** Each worktree must hold what the caller holds, so the reviewer runs the test the finder
      read. Apply the caller's tracked changes. Pipe the output of `git diff --binary HEAD`, run from the caller
      checkout, into `git -C <worktree> apply --binary`. Never use `--index` or `--cached`. When
      `git diff --binary HEAD` prints nothing, skip the apply. Git rejects an empty patch. Build `<scope-pathspecs>`
      from the Step 1 scope. Caller-named files and directories stay as named, so a directory root still covers every
      untracked file beneath it. Such a file can be a helper beside the selected tests. A diff range or the default
      scope is not a pathspec. Resolve it to the directory of each package or directory that Step 1 selected. Then, for
      each path from `git -C <caller> ls-files --others --exclude-standard -- <scope-pathspecs>`, read the file. Write
      it to the same relative path in the worktree with the session's file-write capability, which creates parents. A
      binary or non-text untracked file, or an untracked file outside the scope, is not carried. A baseline that fails
      for that reason makes the candidate `unproven` with the reason `uncarried file`. A refused write into the mutant
      root is `unproven`, never a retry elsewhere. Use paths exactly as the host reports them.

3. **Mutation tool.** The finder may use a mutation tool that the project already has. The tool may run only in a mode
   that lists candidate mutants and does not write to the caller's checkout. A tool that would mutate files is the
   reviewer's alone. The reviewer may use an already installed mutation tool for the project's language that can apply
   one targeted mutant. Otherwise the reviewer hand-applies the mutant inside the worktree with the file-write
   capability. Never install a mutation tool. Never let the tool choose the mutant. Apply the candidate's named edit or
   a stated correction of it.

4. **Spawn the Standard finder.** Read [test-zealot.md](test-zealot.md) at run time. Embed everything below its
   horizontal rule verbatim in a fresh Standard-tier subagent prompt (tiers per
   [model-ladders.md](../../../routing/references/model-ladders.md)). The finder reads a call path and proposes a
   production mutation, and each false candidate costs a worktree and a Deep reviewer, so it does not run at Light.
   After the charter, pass the explicit scope from Step 1 in the same prompt (the charter-embedding mechanism of
   [surface-dispatch.md](../../../routing/references/surface-dispatch.md)). Do not restate, soften, or summarize its
   rules. Its exception list calibrates the bias. Do not frame it. Its output is candidates only: test identity,
   location, claimed behavior, candidate mutant, and evidence, plus the exceptions that excused tests. It edits nothing
   and runs nothing beyond Step 3's read-only mutant listing. Nothing in the finder's report is confirmed until Step 5
   proves it.

    **Finder fallback.** When the finder cannot be dispatched, run the finder pass yourself. Apply the charter verbatim
    to the scope. Write down the complete candidate list in the charter's report shape. Finish the list before anything
    in Step 5 begins. Treat that list exactly as if a subagent produced it.

5. **Dispatch the Deep mutation reviewer.** Use one fresh Deep-tier reviewer per package group, or one in total for one
   group. It is neither the finder nor the wrapper context, because the finder would defend its candidates and the
   wrapper would defend its scope. Give it the candidates for its group, each candidate's worktree path, the module
   layout, and the rules below verbatim. The reviewer edits only inside its worktrees and never touches the caller's
   checkout.

    **Reviewer fallback.** When the reviewer cannot be dispatched, run the reviewer pass yourself only after the finder
    list is complete, whether or not the finder was dispatched. Take the candidate identities, locations, and mutants
    alone from that list. Ignore the finder's evidence and reasoning. A claim that an assertion cannot fail is not
    proof, but the mutant run is. Run every command (showing verbose `-run` subtest command output confirming exact
    subtest execution) and emit every candidate's full proof record, verdict, Step 7 handoff block, and Step 8 report
    (labelled `self-read (not independent)` in both Step 7 Assurance and Step 8), never stopping after the candidate
    list. An in-session finder with a dispatchable reviewer still gets a fresh dispatched reviewer.

    Per candidate, inside that candidate's worktree. The Go commands are examples; another language uses its test
    runner's own switches for a fresh run, an exact selection, and verbose output.

    - **Baseline.** Run the original named test from the module root that contains the test file. Force a fresh run. Go:
      `go test -C <worktree>/<module dir> ./<package> -run '^<Name>$' -count=1 -v` (`.` when the package is the module
      root). For a subtest, select every level exactly with `-run '^<Parent>$/^<Sub>$'`. Write spaces as `_`. Escape
      regular-expression metacharacters, so a sibling subtest never decides the verdict. Verify and state in the record
      that the full test name (`<Parent>/<Sub>` for a subtest) appears as executed in the verbose output. Zero tests
      run, a `no tests to run` notice, or `(cached)` output means not executed. The original must pass before any mutant
      is applied. A failing, skipped, or unexecuted baseline makes the candidate `unproven`.
    - **Package baseline.** Before the first mutant of a package is applied, run that package's tests and the tests of
      its transitive importers inside the module on the original tree, once, in the first worktree of that package.
      Every candidate of the package uses that result, because every worktree carries the same caller state. Find Go
      importers with `go list -f '{{.ImportPath}}: {{join .Deps " "}}' ./...`, keeping the lines whose dependency list
      contains the target package. Record an empty result as `importer set: empty`. Run each with a fresh run
      (`-count=1`). If a mutant was applied early, restore a clean original tree and run this baseline first.
    - **Mutant.** Apply the candidate mutant at its named file:line. When the candidate's edit misreads the code, apply
      a corrected behavior-relevant mutant instead, and state the correction in the record. Confirm the mutated tree
      builds (Go: `go build ./...` and `go vet ./<package>` from the module root, or the test command's own compile
      step). Rerun the same named test with the same command. Then run the package and importer tests again with the
      mutant applied.
    - **Verdict.** If the named candidate test itself fails under the mutant, the mutant killed it: the candidate is
      **rejected**, and the test stands. A mutant that does not build, or that does not change the behavior the test
      claims to prove, is invalid: the candidate is `unproven`. When the original named test passed, the mutant built,
      and the same named candidate test passed under it, the candidate is **confirmed**—even when a _different_ test in
      the package run fails under that mutant. In that case the mutant package run fails (because the other test killed
      the mutant), and the named candidate is **confirmed** with decision `remove` naming that other test as
      `other killing test:`. Never mark a passing candidate `rejected` because another test killed the mutant.
    - **Killing test.** A test is the other killing test only when it passed in the package baseline and fails with the
      mutant applied. Name it. A test that already failed on the original stays out of the decision. So does any failure
      unrelated to the mutant, such as a flake, a missing fixture, or an environment error. List these separately.
    - **Behavior change.** A confirmed candidate needs evidence that its mutant is not equivalent. When another test
      kills the mutant, that failure is the evidence. When none does, add a throwaway check in the worktree that calls
      the production code with one concrete input, run it on the original and on the mutant, then delete it. A mutant
      with no observable difference is equivalent: record the candidate as `unproven` with reason `equivalent mutant`.
    - **Other assertions.** Record whether the candidate test holds assertions besides the flagged one. The decision
      applies to the flagged assertion alone. Only a test with no other assertion is removed or replaced whole.
    - **Unavailable proof.** A test command that is missing, refused, or fails to start is unavailable. Record
      `unproven` for every affected candidate with the exact failure. Seek no substitute way to run tests. Substitutes
      include a hand-written runner, another tool, and reasoning about what the test would have done. Do not retry a
      refused command in any variant: no environment wrapper, no other shell, no reworded flags. Record `unproven` at
      the first refusal.

    The reviewer returns one complete record per candidate—showing the baseline, build, mutant named-test, and
    package/importer commands with their outputs for every candidate individually—with the verdict (`confirmed`,
    `rejected`, or `unproven`), any correction to the mutant, and the facts below. The lines are the requested shape,
    not a required wording: Step 6 audits the facts and their command output.
    - `Original named test: PASS; valid mutant: builds; same named test: PASS`, for a confirmed candidate,
    - `Package and transitive importer tests: <commands and results on the original tree and with the mutant>; other killing test: <identity or none>`,
      with `importer set: empty` when the importer set was empty,
    - `Behavior change: <input> -> original <output>, mutant <output>`, when no other test killed the mutant,
    - `Other assertions in test: <none, or file:line list>`.

6. **Audit the records and restore the caller.** The wrapper owns this judgment. Check each `confirmed` record for these
   facts, each backed by a command and its output in the record:
    - a green baseline with the test name shown as executed,
    - a mutant that builds and changes the claimed behavior,
    - the same named test green under the mutant,
    - the package and importer runs on the original tree and with the mutant, with their results and the importer set,
    - for a `remove` decision, the killing test passing on the original tree; for a `replace` decision, the behavior
      change,
    - the other-assertions record.

    A record that carries all six required facts stays `confirmed` regardless of phrasing (including compact alternate
    wording). Downgrade a record to `unproven` only when one of the six required facts above is genuinely absent, and
    name the missing fact. Never fill a gap from the wrapper's own reading of the code. Emit the full Step 7 block on
    every run, copying every confirmed candidate's complete reviewer record lines verbatim without abbreviation.

    **Holding the proof.** Calling a candidate confirmed without its proof is a
    [safety gate](../../../routing/references/contracts/gates.md#safety-gate): it is a claim of status with no evidence
    from this session. When the user asks to skip mutation, to take the finder's candidates as findings, or to confirm
    an incomplete record, say once what is held (the `confirmed` verdict), the harm (a test reported as proving nothing,
    on a claim no run backed, sends a working test to deletion), and what the user can do instead (the item goes to the
    Unproven list with the reason `proof declined by caller`, and the user can act on the finder's claim themselves). A
    repeated demand gets one line pointing back to both halves of that alternative (`proof declined by caller` under
    Unproven, and the user may act on the finder's claim themselves) without repeating the full hold.

    Read the recorded absolute paths for the caller and each Step 2 worktree from the run record in the same call.
    Resolve both the mutant root and the worktree path, and verify that the worktree is a descendant of that root. Pass
    the caller as `$caller_root` and the verified worktree as `$verified_worktree` to
    `git -C "$caller_root" worktree remove --force -- "$verified_worktree"`. A path outside the root is never removed.
    Report it. After every recorded worktree is removed, run the same three commands in the caller checkout:
    `git rev-parse HEAD`, `git status --porcelain=v2 --untracked-files=all`, and `git worktree list --porcelain`. Write
    each output to its own file under `<mutant-root>/<run-id>/caller-snapshot-after/`, with the `<run-id>` from Step 2.
    For each pair, derive the absolute `$before_file` and `$after_file` paths from the recorded snapshot directories and
    that output's filename in the same call. Compare them with `git diff --no-index -- "$before_file" "$after_file"`.
    Print the concrete `caller-snapshot-before/` and `caller-snapshot-after/` file paths (no placeholders), all three
    exact commands (`git rev-parse HEAD`, `git status --porcelain=v2 --untracked-files=all`, and
    `git worktree list --porcelain`) with their outputs, and the verbatim `git diff --no-index -- <before> <after>`
    result for all three files (unchanged, or the verbatim difference). When Step 2's snapshot write was refused,
    compare each output against the session record instead. Say that the comparison ran from memory. A check that only
    confirms no mutant worktree remains is not the comparison. Report any difference verbatim. Never repair it silently.
    The caller decides what to do with a changed checkout. Finally, delete this run's `<mutant-root>/<run-id>/`
    directory, if it was written, with the session's file-delete capability. This is a file deletion, distinct from
    `git worktree remove`. Delete nothing else under the mutant root. Another run's worktrees or snapshots may still be
    in use.

7. **Hand off.** Emit this fixed-shape block:

    - **Candidates.** For each confirmed candidate, give these items:
        - test identity (package or file, test function, subtest when there is one),
        - location (file:line of the assertion),
        - claimed behavior,
        - the mutant as applied (file:line and edit),
        - the reviewer's record lines, copied verbatim in full for each candidate,
        - the decision.

        The decision is `remove` when another test killed the mutant (name that test), or `replace` when none did. A
        replacement must kill this mutant. Name the mutant. The decision covers the flagged assertion, and covers the
        whole test only when the other-assertions record reads `none`. The block never paraphrases, summarizes, or
        reflows the reviewer's record lines.

    - **Rejected.** Each candidate the mutant killed, with the test that stands and the killing test command and failing
      output.
    - **Exceptions.** Each test the charter excused, with the exception that excused it. This covers a test where a
      pattern matched and an exception excused it. It also covers a test that belongs to an exception class, such as an
      Example function. A test the finder did not flag is a non-candidate and appears nowhere in this block.
    - **Unproven.** Each `unproven` item with its reason: failing or unexecuted baseline, invalid mutant,
      `equivalent mutant`, uncarried file, refused write, unavailable test command, a missing fact, or
      `proof declined by caller`.
    - **Assurance.** This run proves only that the named mutants survived or died. It proves nothing about mutants
      nobody named. When either the finder or the reviewer pass ran in this session, add `self-read (not independent)`
      here.

    Then name the next step for the confirmed work. Read the rows in order; the first match wins.

    | Order | The Candidates list holds         | Next step                                                                                                                                                                                          |
    | ----- | --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
    | 1     | at least one `replace` decision   | [`tdd`](../../../tdd/SKILL.md), with the block as input: it writes each replacement test, which must kill the named mutant, and the delivery path it attaches carries the `remove` items with them |
    | 2     | only `remove` decisions           | [`routing`](../../../routing/SKILL.md), with the block as input, as an ordinary mutating change that deletes the named assertions                                                                  |
    | 3     | Otherwise: no confirmed candidate | none; the report ends the run                                                                                                                                                                      |

    Run the next step in this session only when the user asked for it, in the request or after the block. Otherwise stop
    at the block. This workflow never deletes, edits, or rewrites a test.

8. **Report.** Report these items:
    - the scope as stated in Step 1,
    - the counts of candidates, confirmed, rejected, unproven, and exceptions,
    - the reviewer groups dispatched, or `self-read (not independent)` when either pass ran in this session,
    - the worktrees created and removed,
    - the three caller-state comparison results.

    The counts are a report of what happened to named mutants, not a score. Never derive or state a mutation score, kill
    ratio, or coverage percentage from them. Done means every candidate is confirmed with proof, rejected with the
    killing run, or `unproven` with its reason, and the caller's checkout matches the snapshot or the difference is
    reported. A candidate that is dropped silently is not done.
