# Base-Sync Specialist (merge conflicts)

Runs in `orchestration-lifecycle.md` Phase 0.4, inside the dedicated sync worktree, before any source is detected. It
syncs the PR branch onto its current base. It produces the single merge commit that every source worktree then starts
from. Later specialists thus work on code already merged with the base. Otherwise they would resolve conflicts in
parallel, with fixes never tested on top of that merge. Merge only, never rebase. A rebase rewrites the PR branch's
history and requires a force-push. Triage forbids a force-push.

The orchestrator never dispatches this specialist for fork code: the sync runs the project's checks, which would run the
contributor's code (Phase 0.4 records `skipped — fork PR`).

## Boundary with `/agentics:git resolve`

This specialist is intentionally thin. The resolution cognition lives in
[skills/git/references/merge-conflicts.md](../../../../git/references/merge-conflicts.md). It covers these topics:

- How to recover the intent of both sides via `git log --merge`
- How to combine, rather than pick a winner
- How to avoid wholesale `--ours`/`--theirs`
- How to run the project's own checks before you finish

Load that file. Apply it per conflicted file. Do not redefine it here. Improvements to resolution technique land there,
not in this file.

## Inputs

From Phase 0.4, the orchestrator passes the base branch name (`{BASE_BRANCH}`), the PR branch name (`{PR_BRANCH}`),
`{PR_HEAD_SHA}`, and `{BASE_SHA}`, the base commit it fetched to `refs/remotes/triage/pr-{PR_NUM}-base`. There is no
conflicted-file list, because this specialist runs before detection and discovers conflicts itself, from the merge.

## Workflow

1. **Merge the fetched base.** In the sync worktree (already at `{PR_HEAD_SHA}`, on branch
   `triage/pr-{PR_NUM}/sync-{RUN_ID}`), merge exactly the commit the orchestrator fetched. Do not fetch again: the
   orchestrator recorded `{BASE_SHA}`, and the push inspection subtracts exactly that commit's history.

    ```bash
    git merge --no-ff --no-edit -m "Merge {BASE_BRANCH} into {PR_BRANCH}" "{BASE_SHA}"
    ```

    When the merge completes cleanly, it commits immediately with message `Merge {BASE_BRANCH} into {PR_BRANCH}`.
    **Commit a clean merge. Never abort it.** A clean merge is the sync this run exists to produce. When the merge stops
    on conflicts, resolve them (steps 2-3 below). After you resolve them, commit with
    `Merge {BASE_BRANCH} into {PR_BRANCH}; resolve N conflicts`. Whether the merge is clean or has conflicts, **the
    merge commit's first parent must be `{PR_HEAD_SHA}`.** Commit nothing before the merge commit.

2. **Resolve each conflicted file** per the `/agentics:git resolve` reference loaded above. Special patterns:
    - **Lockfiles and manifests** (`package-lock.json`, `yarn.lock`, `Cargo.lock`, `package.json`, etc.): automatic
      `defer`. They are in [Paths triage never pushes](../../subagent-contract.md#paths-triage-never-pushes), and the
      merge commit is not covered by the push inspection, so the specialist never writes a resolution for one. Populate
      `intervention_reason` with the regeneration command the developer can run, for example
      `npm install --package-lock-only --ignore-scripts`.
    - **Binary files**: automatic `defer`, because no textual resolution exists. Populate `intervention_reason`.
    - **Generated files**: regenerate from source rather than merge the output by hand.

3. **Classify each file** as you resolve it. The first rule that holds decides:
    - `defer`: a binary file, an incoherent result, or a judgment call. In a judgment call, both sides changed the same
      logic for different reasons, and the correct combination is not derivable from the commits. Populate
      `intervention_reason` with one sentence.
    - `fix`: both sides' intent was recoverable and the combined result is confident.

4. **Unresolvable gate.** If any file is `defer`, whether a judgment call or genuinely unresolvable, do **not** commit a
   broken merge. Run `git merge --abort`. The abort is allowed because this trial merge is the specialist's own, in a
   throwaway sync worktree. The parent skill's no-abort rule protects _user_-initiated operations. Nothing is committed,
   and the worktree is back at `{PR_HEAD_SHA}`. Report `SYNC_OUTCOME: blocked`. The orchestrator then skips Phases 1-5.
   It pushes nothing. It dispatches no source. It keeps the sync worktree and branch, so the developer can run the same
   merge there; the section's per-file table says what each conflicted file needs.

5. **Verify.** Run the project's checks per the `/agentics:git resolve` reference (build, tests, lint, discovered from
   the repo and not assumed). A resolution that breaks the build is not resolved. If possible, fix what the merge
   introduced in follow-up commits after the merge commit. A follow-up commit never touches a path in
   [Paths triage never pushes](../../subagent-contract.md#paths-triage-never-pushes); the orchestrator blocks the sync
   when one does. If a failure introduced by the merge cannot be fixed, the outcome is `SYNC_OUTCOME: blocked`, with the
   same handling as step 4 except the abort: the merge is already committed, so `git merge --abort` has nothing to
   abort. Run no abort and no reset; leave the sync branch at the merge commit so the developer can inspect it there,
   and report the merge commit's SHA in `intervention_reason`. `created_commits` stays empty, and the orchestrator
   pushes nothing. A failure that predates the merge (it also fails at `{PR_HEAD_SHA}`, the pre-sync head this
   specialist received) is not this specialist's problem. Note it in the output. Leave it to the Checks source, which
   runs later against the synced head.

6. **Version check.** For each file that meets both conditions, compare the merged value with `{BASE_SHA}`'s value for
   that field:
    - The PR's own diff (`merge-base..{PR_HEAD_SHA}`) changed a `version` field in the file.
    - The base also changed that same file.

    If the two values are equal, the merge dropped the PR's version bump. Report it as a note, not a block: "version
    bump merged away: `<file>` carries the base's `<version>`."

7. **Output.** Produce the JSON shape from [../../subagent-contract.md](../../subagent-contract.md):
    - `SYNC_OUTCOME`: one of `merged cleanly`, `merged — N conflicts resolved`, or `blocked` (steps 4-5).
    - One `findings[]` entry per conflicted file (empty when the merge was clean). All `fix` findings share the same
      `commit_sha`, which is the merge commit.
    - `created_commits`: the merge commit SHA plus any follow-up check-fix SHAs, in order. Empty when blocked.
    - One `proposed_responses[]` entry: `target_type: rollup_section`, `target_id: base-sync`. Its body holds the
      outcome line, any version-check notes from step 6, and a per-file table (file, disposition, one-line rationale)
      when conflicts were resolved or blocked the sync. The body is framed by
      `<!-- agentics:triage:section:base-sync -->` before it and `<!-- agentics:triage:section-end:base-sync -->` after
      it. The body has no attestation footer, because the section sits inside the rollup, which closes with the only one
      (see `../../rollup-template.md`, "Source sections"). Before the body goes on the PR, apply the
      [shared redaction rule](../../../../routing/references/contracts/redaction.md#copied-text) to copied check output.

Specialists dispatched after this one (Phase 1 onward) read files at the synced head, not at `{PR_HEAD_SHA}`. A review
comment's line number may point at a pre-merge line if the merge shifted that file.

## What this specialist does NOT do

- **Does not rebase.** Merge only, under any phrasing of the request.
- **Does not fetch.** It merges the base commit the orchestrator fetched.
- **Does not push and does not post.** Standard subagent constraints apply in full.
- **Does not resolve outside the sync worktree.** This specialist never touches the main repo checkout.

## Edge cases

| Scenario                                            | Handling                                                                                                                          |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Base branch moved after the orchestrator fetched it | Merge `{BASE_SHA}` anyway. The next run fetches the newer base and syncs again                                                    |
| Merge completes with no conflicts                   | Committed immediately as `merged cleanly`. A clean merge is the expected, common case, not a no-op to abort                       |
| Conflicts only in lockfiles                         | `defer`, so the sync is `blocked`. The specialist writes no lockfile; `intervention_reason` names the regeneration command        |
| Binary file conflict                                | `defer` with `intervention_reason`. Never guess a binary resolution. The `defer` blocks the sync (step 4)                         |
| Checks fail and the failure predates the merge      | Not this specialist's problem. Note it in the output rationale. Leave it to the Checks source, which runs against the synced head |
| Base moves again after this run pushes              | Expected recovery path: a re-run's `git merge-base --is-ancestor` check finds the branch behind again, so the re-run syncs again  |
