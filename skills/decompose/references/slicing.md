## Delivery slicing

This is the optional slicing step inside `/agentics:decompose`. Before decompose writes the implementation tasks, this
step turns one concrete source into delivery boundaries. It does not define a solution, add an approval gate, or publish
tracker issues.

### 1. Resolve an accepted source

Read the source artifact directly when one exists. Accept either:

- an in-flight `intent.md` that meets all of these conditions:
    - It has five sections and was written before work that starts at intent required a spec.
    - It has a concrete problem, outcome, constraints, and decisions.
    - It has no direction blocker.
    - It has a direction-review disposition: a `challenge` row of `ok` or `declined`, or a `pending` one whose open
      questions the Step 4b round carries.
- a spec in one of these states:
    - an approved `spec.md` (a current `approve spec.md` row) that includes its solution challenge disposition
    - a reviewed, unapproved spec handed over on the routine tier.

A legacy `design.md` is also a valid source when it is concrete and writes down its chosen solution and acceptance
evidence. Read it in place. Record its path in the plan. Use `derived-from: none`.

Concrete inline requirements are also a valid source when their outcomes, constraints, and acceptance evidence are
settled. With no source artifact, do these steps:

1. Record those requirements once in the plan's `## Requirements` section, directly after `## Provenance`.
2. Use `derived-from: none`.

Do not invent an intent or spec, and do not claim its approval.

With an artifact, record the actual source in `derived-from`. Never reconstruct the source from a request line or
memory.

If an intent still has a direction blocker, return it to `/agentics:intent`. A six-section intent needs an approved spec
first. Route it to `/agentics:spec`. A concrete in-flight five-section intent with a recorded direction-review
disposition proceeds directly.

If a spec is unapproved and was not handed over on the routine tier, route it to `/agentics:spec`. That route is for
solution definition and approval. Preserve all settled source decisions and deferred follow-ups. Never treat the
intent's disposition as a review verdict for a later artifact.

### 2. Apply the independent-delivery test

Ask whether two or more outcomes could each ship, be demonstrated, and be useful while sibling outcomes remain unbuilt.
Document length, architectural layers, and implementation task count do not establish separate deliveries.

If the test does not hold, write no slice sections. An authorized decompose run then writes one plan with a `## Tasks`
section. A slicing-only request reports the single-outcome decision and writes no plan.

### 3. Draft vertical slices

When the test holds, use the fewest vertical slices that preserve independently useful outcomes. Each slice must cut
through every integration layer that its outcome needs. Each slice must be verifiable on its own.

When the source has a priority and dependency order, use that order. Keep acceptance criteria in the source or the
inline-input `## Requirements` section. Each slice cites those sections or story anchors and does not copy them.

Use stable ID-only headings (`## S1`, `## S2`, …). Each active slice contains:

- **Title:** a short descriptive name.
- **Outcome:** the independently useful result and its proof boundary.
- **Scope:** source references and the boundary with sibling slices.
- **Dependencies:** sibling IDs, named external decisions, or `none`.
- **Lane:** `yes` on a slice that qualifies as a lane under
  [`chain-artifacts.md`](chain-artifacts.md#slices-inside-planmd), otherwise absent.
- **Issue:** optional; see step 4.

Dependencies must reference existing IDs. They must not cycle. A slice does not authorize implementation, publication,
or a merge.

### 4. Write or rescope the slice sections in `plan.md`

Write one `## S<n>` section per slice, with the fields above, in the work folder's `plan.md`. The sections replace
`## Tasks`. Tasks go under their own slice as `### Task N`.

When a slice has a branch of its own, put its ID in the branch name once, as `s<n>`. Bound `s<n>` by the start or end of
the name, or by `/`, `-`, `_`, or `.` (for example `feat/export-s2`). This lets `/agentics:gate branch` match the branch
to its slice. A lane branch follows the same rule.

In the plan header, `derived-from` names the source alone. Its value is one of these:

- `[spec.md]` for any work that started at intent
- `[intent.md]` only for an in-flight five-section intent
- `none` for inline requirements or a legacy design read in place.

Slices add no approval or challenge state of their own.

Reconcile an existing breakdown in place instead of a silent replacement:

- Preserve every active ID when its outcome remains.
- Preserve order only when it carries meaning.
- Keep retired IDs with `Retired: <reason>` so old anchors stay reserved.
- Never reuse an ID.
- Never leave a dependency on a retired ID.

Record the rescope and any changed source references in the plan. Fold a legacy `slices.md` beside the plan into the
plan, per [`chain-artifacts.md`](chain-artifacts.md). Remove `slices.md` with `git rm`. A copy left behind is an extra,
and gate moves it to `temp/`.

Do not create GitHub issues by default. Only when the user explicitly requests issue projection, do these steps:

1. Use `/agentics:gh` to create one issue per active slice, in dependency order.
2. Make each issue link to `plan.md#s<n>`.
3. Record the returned links in the **Issue** field when the plan has no approval yet. After the plan is approved,
   record each link as a `note slice-S<n>` row instead, because an edit to the field makes the approval stale.

The local breakdown never requires an issue.

For a slicing-only request, do these steps:

1. Write the sections with no tasks.
2. Set `**Skeleton path:** none — slicing only` and the slicing-only Provenance values in
   [plan-template.md](plan-template.md#provenance). Set `## Task index` to `none: slicing only` and `**Size:**` to
   `slicing only, no tasks`.
3. Save the plan as Step 6 does for a slicing-only run: stage it and write its `create` row, with no `challenge`,
   `review`, or `approve plan.md` row.
4. Report the path and the stable IDs in dependency order.
5. Stop.

In a slicing-only request, do not write tasks or ask for execution approval.

When slicing is part of an already-authorized decompose run, do these steps:

1. Continue in the same `plan.md`.
2. Write the tasks under their slices.

Do not make one plan or one decompose invocation per slice.
