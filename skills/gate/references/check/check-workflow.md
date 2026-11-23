# /agentics:gate check: mid-work checkpoint verification

Check mode runs lightweight checkpoints at the decision points of code work. **Workflow checkpoints** (Part 1) check
that the work is ready for its next phase. **Data integrity verification** (Part 2) checks the structured data that code
reads or produces, such as parsed config, a fixture, JSON from a command or an API, or a generated file, as soon as it
exists. The result is a pass or fail checkpoint note, not an auditable artifact, and check mode writes no `state.tsv`
row ([emission.md](../emission.md)). Check mode inspects; it never edits the artifact or the code that produced it.

## Step 0: Preflight

[SKILL.md](../../SKILL.md#step-0-preflight) has already checked the request against gate's Don't-use table. Then:

1. **Check the check-mode boundary.** When the request matches a row of [Not a checkpoint](#not-a-checkpoint) below, act
   on that row before anything else. A request for another skill never gets a missing-artifact or missing-criteria reply
   for a checkpoint it never needed.
2. **Identify the artifact.** Name the concrete artifact to check: a diff, a config or generated file, the output of a
   build or test run, a migration plan, or a plan or spec document. With only a goal or an intent in hand, stop and
   return one line: _"Nothing to check yet; produce the {diff | generated file | test run | plan} first, then run
   `/agentics:gate check` again."_ Never run an empty checkpoint on a happy-path assumption.
3. **Source the criteria.** A checkpoint needs concrete pass conditions (expected fields, ranges, counts, exit codes).
   Take them, in this order, from the user's request, the upstream contract (`decompose` acceptance criteria, the
   artifact's schema or type definitions, the spec or template it must follow), or a convention the repository states.
   Never invent a threshold. With no source, stop and return one line: _"No pass criteria for the {artifact}; give them,
   or point me at the {acceptance criteria | schema | spec}, then run `/agentics:gate check` again."_ Never pass a
   checkpoint you cannot evaluate. When a criterion is a rubric that needs judgment rather than a computed value, grade
   it with a strategy from [eval-patterns.md](eval-patterns.md#evaluation-strategies), once.

Once the preflight passes, and before the first checkpoint runs, the **Check principles before verification** rule in
[SKILL.md](../../SKILL.md#cross-cutting-rules-all-four-modes) has run.

## Not a checkpoint

These requests look like a check and are not one. Each is a **cannot** row, as gate's
[Don't use when](../../SKILL.md#dont-use-when) defines the kind: no checkpoint produces what it asks for, so it is
forwarded once, as [forwarding.md](../../../routing/references/forwarding.md) describes.

| Request                                                             | Use instead                                                                |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Mark a branch, release or change complete with a record             | `/agentics:gate branch`, `/agentics:gate release`, `/agentics:gate change` |
| Write the acceptance criteria in the first place                    | `/agentics:decompose`, or `/agentics:spec` for a whole feature             |
| Improve an output over repeated attempts against a rubric or metric | `/agentics:hillclimb`                                                      |

Gate's own Don't-use rows (fixing, diagnosing, evals, code review) were applied before check mode loaded, so no request
meets both tables.

## Skipping a checkpoint

Callers under time pressure ask to skip a checkpoint ("the output is obviously fine", "we checked this last run", "just
go on"). A checkpoint exists to stop a cascading error: a wrong input, a malformed config or a null field flows into the
next task, and every later phase has to be redone. Two rules apply, each with its class from
[gates.md](../../../routing/references/contracts/gates.md):

- **Running the checkpoint is a process gate.** Name the cost once, in one sentence that says which criteria would go
  unchecked and what they would let through. When the next reply still declines, in any wording, skip it: ask for no
  reason, and do not run part of it and wave the rest through. Check mode writes no row, so the note is the record:

    ```text
    Checkpoint status: NOT RUN — <criteria> not checked. Skipped at the user's request; reason: <the user's words | none given>.
    ```

- **Reporting a status is a safety gate.** An unrun checkpoint is never reported as passing, and no criterion is marked
  `PASS` without a check from this session or a pointer to one. When the caller says a criterion already holds ("that
  can't be null", "already validated"), that statement is about their own work and is taken as true: record it as
  `stated by the caller, not checked`, not as `PASS`. When the check is cheap, run it instead; that needs no permission
  and no proof from the caller.

A repeated demand to call the checkpoint passed gets a one-line pointer back to those two options: skip it and record
`NOT RUN`, or let it run.

In an [unattended](../../../routing/references/contracts/unattended.md) run, a failed checkpoint is reported, never
waved through, and the dispatcher decides what follows.

---

## Part 1: Workflow checkpoint gates

### Pattern

Do these steps:

1. Define the criteria.
2. Check each one.
3. Branch on the result.

The template for this pattern:

```markdown
✅ CHECKPOINT {N}: {Checkpoint Name}

**Purpose**: {what this checkpoint validates}

**Validation Criteria**:

- [ ] {Criterion 1 description}
- [ ] {Criterion n description}

**Validation Steps** (one per criterion):

1. **{Criterion}**:
    - Check: {what_to_check}
    - Expected: {expected_value_or_condition}
    - Actual: {actual_value_from_workflow}
    - Status: ✅ PASS / ❌ FAIL

**IF ANY UNCHECKED** (any criterion failed): → {Checkpoint-specific remediation: correct the input and re-run, find the
cause of the bad output, request user confirmation, log diagnostics, or prompt for missing/invalid input} → **STOP
workflow until issue resolved**

**IF ALL CHECKED** (all criteria passed): → Log checkpoint success → Proceed to {next_phase_name}
```

**A failed checkpoint stops the next phase** until the cause is fixed and the checkpoint runs again, or until the user
decides to go on. Going on is the same process gate as [skipping a checkpoint](#skipping-a-checkpoint): the cost is
named once, the next reply that still goes on is followed, and the note keeps reading `FAIL` for the criteria that
failed. The remediation the template names is the caller's to do; check mode does not perform it.

### Common checkpoint types

1. **Input Validation**, after you parse the task's inputs (arguments, config, environment, request parameters)
2. **Command Success**, after you run a build, test, migration, script, or API call
3. **Output Validation**, after you receive output (command or API results, a generated file, a test report)
4. **Transformation Validation**, after a transformation (a migration, code generation, a computed metric)
5. **Handoff Readiness**, before you produce the artifact the next phase consumes (a commit, a PR, a release, a report)

> Full per-checkpoint templates and a worked example (checking a script's JSON output) are in
> [checkpoint-templates.md](checkpoint-templates.md).

---

## Part 2: Data integrity verification

Apply these checks immediately after code reads or produces structured data: parsed config, fixtures, JSON from a
command or an API, a generated file, or the rows a migration touched. They validate quality before the next step
consumes the data. Check mode inspects the data. It does not rewrite it.

| Check             | Purpose                                                                                                         |
| ----------------- | --------------------------------------------------------------------------------------------------------------- |
| Null/Undefined    | Ensure critical fields have values. List the records with missing data.                                         |
| Range             | Verify numerical values fall within logical bounds (e.g., 0–100% for percentages)                               |
| Type              | Confirm fields have correct data types, with no NaN, Infinity, or unexpected coercions                          |
| Consistency       | Validate cross-field relationships (e.g., completed ≤ total, start < end)                                       |
| Outlier Detection | Flag values that deviate >3σ from the mean or fall outside 1.5×IQR. A flag is a warning for review, not a fail. |

After all five checks run, combine the results into a **Data Integrity Checkpoint**. The checkpoint gates the move to
the next phase.

> Full templates and a worked example (validating a pricing script's output) are in
> [validation-checks.md](validation-checks.md).
