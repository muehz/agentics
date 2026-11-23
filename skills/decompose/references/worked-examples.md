# Worked examples

The examples below show five decompose runs, end to end. Paths, dates, and hashes are illustrative. Where an example
disagrees with `SKILL.md`, `SKILL.md` wins. Which inputs this skill accepts, and where the others go, is the
`## Don't use when` table in `SKILL.md`.

## Example 1: an approved spec, from construction to handoff

**Input.** `docs/specs/2026-09/feature-export-csv/spec.md` has a current `approve spec.md` row dated 2026-09-10. It
defines a CSV export of the orders table, capped at 10,000 rows, with RFC 4180 quoting and three success criteria. The
user says "plan this".

1. **Step 0.** The spec is approved and carries real behavior, acceptance, and scope, so row 12 accepts it. Announce:
   "I'll write the implementation plan for the CSV export."
2. **Step 1.** Read the spec's decisions and do not re-ask them. The export is one useful outcome, so the plan has no
   `## S<n>` sections. The work folder is the spec's folder.
3. **Step 2.** The context map creates `src/export/csv.ts` and `tests/export/csv.test.ts`. It modifies
   `src/export/index.ts`.
4. **Steps 3 and 4a.** No `plan.md` exists yet, so the run records that before the first write. The plan has three
   tasks. `task-1` serializes rows (outcome, covers `spec.md Success Criteria 1` and `2`). `task-2` wires the
   `/export.csv` route and depends on `task-1` (outcome, covers `spec.md Success Criteria 3`). `task-3` adds an export
   audit log line and depends on `task-2` (support, covers `none`). The draft has `**Result:** not run`.
5. **Step 4b.** The pass asks one round, Q1 and Q2. Q1: does a field that contains a comma round-trip? Answer: yes, per
   the spec's RFC 4180 decision, so `task-1` gains a quoted-comma acceptance case. Q2: should exports stream? Answer:
   no, since the spec caps exports at 10,000 rows. The pass completes, and the plan records it:

    ```markdown
    ## Plan challenge

    **Challenge record:** (1) `plan.md`, task graph, outcome proof, and verification. (2) 2026-09-12, 1 round asked and
    answered, Q1 to Q2. (3) Q1: quoted-comma case added to task-1, source: the user. Q2: no streaming, source: the user,
    citing the spec's row cap. (4) Remaining questions: none. (5) Set aside: none.
    ```

6. **Step 5.** All 10 rows run and pass. Proportionality: the Outcome proof is one command,
   `curl -s http://localhost:3000/export.csv | head -n 1`, which prints the header row `id,customer,total` and could not
   pass without the route and the serializer. The skeleton is `task-1` → `task-2`, with no task ahead of it. The plan
   has 2 outcome tasks and 1 support task, and no signal fired. The coverage check reads each task's `**Covers:**`
   field: 3 items, all covered. The repository has no plan-artifact validator, and the report says so. The plan review
   runs last and returns `READY` with no findings. Every task is marked `**Packet review:** no`: the contract is
   settled, and no security, trust, state-binding, or cross-platform seam is touched.

    ```markdown
    - **Challenged:** yes 2026-09-12
    - **Result:** pass
    - **Coverage:** 3 items, 0 uncovered, 0 partial, 0 orphan tasks, 0 contradictions; blocking: none; waived: none

    <details>
    <summary>Provenance record: written by, validated, plan review, proportionality</summary>

    - **Written by:** `decompose` v4.25.0
    - **Validated:** 2026-09-12, rows 1-10 all run
    - **Plan review:** READY — 0 critical, 0 important, 0 minor, 0 informational, rev 1, run 1
    - **Proportionality:** pass — skeleton `task-1`→`task-2`, 0 tasks ahead of it (0 support); 2 outcome / 1 support
      tasks; signals fired: none

    </details>
    ```

7. **Step 6.** The offer gate passes. Stage `plan.md` and confirm it is in the index. Log `create plan.md`, one
   `review plan` row with evidence `kind=plan dispatched=2026-09-12T10:00:00Z round=1 blocked=140 rev=1 by=independent`,
   and an `ok` `challenge plan.md` row. Stage `state.tsv`. Nothing is committed yet. The spec is approved, so the tier
   check reads only the plan: no `Deep` task, two file groups, every modified file exercised by a test, so the tier
   stays routine. Run `flow status`, note the plan's hash, show the plan by path and hash with a three-line summary, and
   ask the plan question. The user says yes. Log `approve plan.md` with `auto` and `--expect-blob`. The plan has no
   standing orders, so no `order` row follows. Stage `state.tsv`, then commit the plan and `state.tsv` together on the
   feature branch. Report that the plan is reviewed and validated, and route to `execute`.

## Example 2: a declined challenge with a failed validation row

**Input.** `docs/specs/2026-09/feature-retry-uploads/spec.md` has a current `approve spec.md` row dated 2026-09-13. It
asks for three retries with backoff on upload failure.

1. **Steps 0 to 4a.** The input fits. The draft has three tasks: `task-1` backoff helper, `task-2` retry wrapper, and
   `task-3` uploader wiring.
2. **Step 4b.** The user says "skip the challenge". State the cost once: missing dependencies or weak verification may
   survive to execution. The user's next reply still declines, so record the decline:

    ```markdown
    **Challenge record:** (1) `plan.md`; unexamined: the whole written plan (task graph, parallel groups, outcome proof,
    and verification gaps). (2) declined 2026-09-14, no round asked. (3) none. (4) Remaining questions: none. (5) none.
    ```

3. **Step 5.** All 10 rows run. Row 9 fails: `task-3`'s verification step reads "confirm uploads recover", with no run
   command and no expected result. The session cannot fix it without knowing how the user wants the recovery observed:

    ```markdown
    - **Challenged:** declined by user
    - **Result:** failed — row 9: task-3 has no run command or expected result
    ```

4. **Step 6.** The offer gate fails. Save the plan. Log `create plan.md`, the `review plan` row, and a `declined`
   `challenge plan.md` row, then a `pause plan.md` row whose `next` is `decompose fix row 9`. Write no `approve` row.
   Report row 9 and what it needs (a command that exercises recovery and its expected output). Do not ask the execution
   question. Do not call the plan validated or reviewed.
5. **Later, same session.** The user supplies `go test ./uploader -run TestRetryFakeBucket`, expected `ok`. Rebuild
   `task-3` around it. This is a change the user asked for, so it starts revision 2 of the plan, with a fresh review
   budget. Rerun all 10 rows, then the proportionality check, the coverage check, and the plan review. Every row passes,
   so `**Result:** pass`. The report says `validated; plan challenge declined`, never "reviewed". The skill asks the
   plan question.

## Example 3: an elevated plan from a routine-tier spec

**Input.** `docs/specs/2026-09/feature-rate-limit/spec.md` has no `approve spec.md` row. `spec` found no tier signal in
the spec and handed it over, with `**Spec review:** 2026-09-14, 2 findings, 2 resolved, 0 presented, run 1` and no open
policy conflict.

1. **Steps 0 to 4b.** The routine hand-off applies. The plan has two tasks. `task-1` builds the limiter store in
   `src/auth/limits/`, and `task-2` wires the check into login in `src/auth/login/` and the response in `src/http/`. The
   challenge completes on 2026-09-15.
2. **Step 5.** All 10 rows pass, proportionality passes, coverage finds both items covered, and the plan review returns
   `READY`.
3. **Step 6.** The tier check reads the final plan: three file groups, so the plan raises the tier to elevated. The
   approval file's row 3 applies. Show the spec by path and hash with its review counts, and ask the spec question. The
   user says yes. Log `approve spec.md` with `next` `decompose ask for approval`. Then show the plan by path and hash
   and ask the plan question. The user says yes. Log `approve plan.md` with `why`
   `elevated: three file groups; approved after the plan challenge and one plan review`. Commit the plan, the spec, and
   `state.tsv` together. Route to `execute`.

## Example 4: turn by turn, with a decline, a failed checklist, and "execute it anyway"

**Input.** `docs/specs/2026-09/feature-bulk-import/spec.md` is approved. The user opens with "plan this, and then hand
it to execute."

1. Step 0 fits. The announce line says the run will write the plan and take starting execution afterwards as authorized.
2. Steps 1 to 4a produce a draft. The user says "skip the challenge." The skill names the cost once, as in Example 2.
   The user declines again, so it records a `declined` `challenge plan.md` row and the record reads
   `declined 2026-09-16, no round asked`.
3. Step 5 runs all 10 rows. Row 9 fails: `task-2` says "check the imported rows look right", with no command. Nothing in
   this session can fix it, and the result reads `**Result:** failed — row 9: task-2 has no run command`.
4. The report names row 9 and what it needs. The skill writes no approval row: the upfront "hand it to execute" named
   execution of a plan that passes, and this plan does not.
5. The user has now seen row 9 and says "execute it anyway." That is their approval of the plan as it stands. The skill
   shows the plan by path and hash, logs `approve plan.md` with `why`
   `routine: no signal fired; approved by the user with row 9 failing`, commits, and routes to `execute`. The record
   stays `**Result:** failed — row 9: task-2 has no run command`, so `execute` reviews every task at Stage 1.

## Example 5: routine tier, one question for spec and plan

**Input.** `docs/specs/2026-10/feature-report-copy-link/spec.md` has no `approve spec.md` row. Its Scope Decisions
record `**Spec review:** 2026-10-12, 1 finding, 1 resolved, 0 presented, run 1`, and its Policy Conflicts reads
`n/a: no policy skills declared`. The spec adds a "Copy link" button to the existing report toolbar that copies the
current URL. `spec` found no tier signal and handed the spec over.

1. **Step 0.** The spec is unapproved, but its review is recorded, no policy conflict is open, and its challenge is not
   pending, so the routine hand-off applies. Announce the run.
2. **Steps 1 to 4a.** One task: `task-1` adds the button in `src/report/toolbar.tsx` (outcome) and extends
   `tests/report/toolbar.test.tsx`. A one-task plan is valid, and its Risk Notes table has one row.
3. **Steps 4b and 5.** The plan challenge, the checklist, the proportionality and coverage checks, and the plan review
   run as usual. The one task is marked `**Packet review:** no`.
4. **Step 6.** The tier check reads the final plan: no `Deep` task, two file groups, no ownership boundary, and
   `npm test -- tests/report/toolbar.test.tsx` exercises `src/report/toolbar.tsx`. The tier stays routine, so the
   combined question applies. Show the spec and the plan by path and hash. The spec presented no finding, so show its
   review counts only. Ask the combined question. The user says yes to both. Log `approve spec.md` first and
   `approve plan.md` second, each with `auto`, its own `--expect-blob`, and `why`
   `routine: no signal fired; approved with the plan after spec review and plan challenge`. Commit the plan, the spec,
   and `state.tsv` together. Route to `execute`.

Contrast: `src/report/share.ts` has no test or verification command in the plan that exercises it. Had `task-1` also
modified it, the tier check would find the tier elevated, and the spec question would come first, as in Example 3.
