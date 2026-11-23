# Checks Tier Policy

Three tiers govern what the Checks specialist may do with a failed check run. The tiers keep automatic remediation
limited to deterministic, behavior-preserving changes. Anything else becomes a draft suggestion or goes to a human.

## Tiers

### Auto-fix

A change qualifies for Auto-fix only if **all** of the following hold:

- The failure has a single, deterministic remediation. For example, the formatter reports a diff, and the check passes
  when the diff is applied.
- The change is behavior-preserving in production. It does not alter API contracts, error handling flow, log output that
  callers depend on, public types, or runtime behavior.
- The fix is local (one or a handful of files) and touches no path in
  [Paths triage never pushes](../subagent-contract.md#paths-triage-never-pushes).
- The fix is verified: the failing check's own command, run locally on the changed files, now exits `0` (Phase 4 of
  [checks-triage.md](checks-triage.md#phase-4-apply-auto-fix-tier-only)). A fix that cannot be verified that way is
  Suggest.
- The dispatch says `Fork code: no` (see "Fork code" below).

Disposition: `fix`. Commit locally on the worktree branch. The orchestrator integrates it.

**Examples:**

- `prettier --check` failure → run the formatter on the files the check names. Then commit the diff.
- `eslint --fix`-able rule violations (the rule's metadata says `"fixable": "code"`) → apply the autofix to the files
  the check names.
- A missing generated artifact whose generator is committed and deterministic (`pnpm codegen`, `protoc`, `go generate`)
  → re-run the generator. Then commit the result.
- An import-sort or trailing-newline lint violation → apply the deterministic fix.

A formatter or generator runs only on the files the failing check names, never across the repository. When one of those
files is an approved planning artifact (a work item's `intent.md`, `spec.md` or `plan.md`), the finding is Suggest:
rewriting it would change text that an approval covers, so the developer decides.

### Suggest

The fix is small and the path is clear. It still requires human judgment to confirm, even for a one-line change.

A change is Suggest tier when:

- The remediation requires a judgment call about intent. For example, a failing assertion's expected value changed
  because the implementation legitimately changed. The test may be right and the implementation wrong.
- The remediation could plausibly be wrong in a way only the developer's context can disprove.
- The fix touches behavior that other tests, callers, or documentation depend on, and the specialist cannot exhaustively
  verify those dependencies.

Disposition: `defer`. Include the suggested diff in the body. Do not commit. The developer can copy the diff into the PR
if they agree.

**Examples:**

- A unit test that asserts `expected: 42, actual: 43`, where the implementation changed legitimately. The test probably
  needs `42 → 43`, but the developer should confirm the new value is correct.
- A type-error fix where the obvious change (widen the parameter type) might mask a real bug.
- A small log-message tweak to satisfy a snapshot test, where it's unclear whether the test or the log line is
  canonical.

### Intervention

Anything ambiguous, security-relevant, cross-cutting, or environmental. The specialist makes no commit and proposes no
change.

A finding is Intervention tier when:

- The failure is plausibly flaky (intermittent, no consistent stack trace, retry would likely succeed).
- The failure is environmental (network timeout, infra outage, missing secret, runner-level resource limits).
- The fix would touch CI configuration, secrets, branch protections, or external service configuration.
- The failure is in a deploy-stage check (production deploys, infrastructure provisioning).
- Multiple plausible remediations exist and none is obviously correct from the available context.
- Annotation/log retrieval failed or the available context is insufficient to tier confidently.

Disposition: `defer`. Populate `intervention_reason` with one short sentence that points the developer at what they need
to look at.

**Examples:**

- An `e2e` test fails with a screenshot diff that requires visual judgment.
- A `deploy` step fails with a 502 from the deploy target.
- A `dependabot` check fails because of a security advisory on a transitive dependency.
- A `timed_out` check run.

## Fork code

With `Fork code: yes` in the dispatch, nothing from the worktree runs on this machine: no formatter, no linter, no
generator, no build and no test. No finding is Auto-fix, because no fix can be verified. A finding whose fix is clear
from reading is Suggest, with the patch drafted from reading alone and a rationale that says nothing ran. Every other
finding is Intervention.

## Strict DO NOT list

These actions are forbidden across all tiers. The specialist never applies them automatically, never drafts them, and
never even suggests them:

- **Never change a path triage never pushes.** CI configuration, dependency manifests and lockfiles, and security and
  repository configuration are listed once, in
  [Paths triage never pushes](../subagent-contract.md#paths-triage-never-pushes). The orchestrator holds any commit that
  touches one, so a fix that needs one is Intervention, never a commit. Dependency changes go through their own PRs.
- **Never disable, skip, or delete a failing test.** No `it.skip`, `xit`, `@pytest.mark.skip`, `t.Skip()`, comment-outs,
  or test removals. If a test is wrong, that's a Suggest-tier proposal. The proposal calls out the test as the change
  target, never an automatic deletion.
- **Never change repository settings or secrets.** No branch-protection rules, security policies, audit allowlists,
  signing or notarization configs, and no `${{ secrets.X }}` additions or removals.
- **Never auto-merge across check-run categories.** Each failed check is a separate finding with its own tier. Do not
  bundle a Build/Compile fix into the same commit as a Lint/Format fix. Bundle them only when they share a single root
  cause and the change is small.
- **Never re-run a check**, in any form (see [No re-runs](checks-triage.md#no-re-runs)). If a failure looks transient,
  tier it as `intervention` so the developer decides.
- **Never push or post.** Subagents commit locally only. See [`../subagent-contract.md`](../subagent-contract.md).

## Tier-decision flow

For each failed check run, ask these questions in order. The first that answers yes decides the tier.

1. Is the failure environmental (timeout, infra error, retrieval failure, deploy step)? → **Intervention**.
2. Would the fix touch anything on the DO NOT list? → **Intervention**.
3. Does the dispatch say `Fork code: yes`? → **Suggest** when the fix is clear from reading, otherwise **Intervention**.
4. Is there a single deterministic remediation that is behavior-preserving, with a verification command the default
   branch's configuration names and this machine can run? → **Auto-fix**, provisionally. Phase 4 applies and verifies
   it, and a remediation whose verification does not pass is re-tiered **Suggest** before anything is committed or
   reported as fixed.
5. Is the remediation small and reasonably clear, but needs human judgment or cannot be verified locally? → **Suggest**.
6. Otherwise → **Intervention**.

When in doubt, demote one tier. Auto-fix is the smallest tier by design. If a finding could plausibly belong in Suggest,
put it in Suggest. Apply the same rule from Suggest to Intervention.

## Cross-references

- [`checks-categories.md`](checks-categories.md): per-category guidance on which tier each failure family typically
  lands in
- [`checks-log-parsing.md`](checks-log-parsing.md): what to retrieve to tier confidently
- [`checks-triage.md`](checks-triage.md): the workflow that consumes this policy
- [`../subagent-contract.md`](../subagent-contract.md): disposition vocabulary and `intervention_reason` semantics
