# Rollup Details, Construction, and Examples

Split from [rollup-template.md](rollup-template.md). Defines Details blocks, source sections, the Base sync block, the
body size budget, construction instructions, prior-row preservation, and rendered examples.

## Details blocks

Inside the single Details fold, every block opens with a `####` heading: `#### Base sync` first, then
`#### Detection notes`, then one `#### {Source}` per table row, in table order.

- **Under a source heading,** each non-zero disposition is its own bullet, labeled with its count: `- **Fixed (5):** …`,
  `- **Accepted (1):** …`, `- **Dismissed (1):** …`, `- **Needs you (1):** …`. Fixed lists its short SHAs with a few
  words each, separated by `·`. Every other bullet gives the location and the reason in one clause.
- **Break every non-Fixed count down by reason.** `1 dismissed` is ambiguous; `1 dismissed (pre-existing)` is not.
- **Final SHA after a push.** After a successful push, every Fixed bullet names its final pushed SHA. A duplicate lists
  that SHA even when its owner source's Details already lists it. Severity labels, locations and ownership explanations
  never replace the SHA. On blocked or skipped pushes, use the wording below.
- **Severity is presentation.** In Details, label each finding with its shared severity from
  [severity.md](../../routing/references/contracts/severity.md) (Critical, Important, Minor, Informational), and give
  the source's own label beside it when it has one, as in `**Important (P1)**`. Carry the same severity into any
  needs-you bullet for that finding. Order findings by severity within the source. Labels never change dispositions or
  counts. A finding the source gave no label stays triaged, with the severity the specialist assigned from the meanings.
- **Flag partially-correct findings explicitly.** Any single count misrepresents a finding whose underlying observation
  held but whose specific claim did not. Examples are right problem, wrong file; real deviation, wrong cause; and valid
  concern, overstated severity. Say what held and what didn't, in the bullet. A reader who only scans the table must not
  conclude that the reviewer was simply wrong.
- **Name any catch-all finding**, with its raw `disposition` value (counting rule 2).
- **Close the block with one `<sub>` line** when there are replies, thread notes, or size-budget notes to report. Put
  the line after the bullets and any sections. Example: `<sub>6 replies posted</sub>`. The line names a reply that
  failed to post after the Phase 6.2 fallback retry. It gives the reply's target (`path:line` or comment ID), so the
  developer can post it by hand. A posting failure moves no counts.
- **A source's sections** (Checks, Checkmarx) follow its bullets, inside their anchors (see "Source sections").
- **A status with no counts** (`Failed`, `Skipped`, `No new findings`, `Pending Review`) gets one plain sentence under
  its heading instead of bullets. The sentence states the reason or the prior run's summary. `Pending Review` uses the
  fixed sentence in "Needs you".
- **A `Push Blocked` or `Push Skipped` row keeps its bullets**, because its findings were triaged. Each Fixed bullet
  says the fix was not pushed, and gives no SHA, because none exists on the PR.

`#### Detection notes` renders only when the complete Phase 1 state record contains `skipped`, `unhandled`, `forwarded`,
`forward_failed`, or `unverified` results: one bullet per result, with the precise reason and `no draft produced` for
`forward_failed`. The notes are context only. They create no row, count, post, or needs-you bullet.

## Source sections

A source with no review thread to reply in delivers its detail as a collapsed section under its `#### {Source}` heading.
Checkmarx One, Checks, and any future adapter without a review thread are such sources. Such a source never gets a
top-level comment of its own. Each section is one `rollup_section` response from the source's subagent
([subagent-contract.md](subagent-contract.md)).

| Source        | Sections per run         | Key                                     |
| ------------- | ------------------------ | --------------------------------------- |
| Checkmarx One | one                      | `checkmarx`                             |
| Checks        | one per root-cause group | `checks:{lowest check_run_id in group}` |

Every section has the same frame. A follow-up finds a section to replace or carry by its open and close anchors, so
never drop them. Never put the attestation footer inside a section. The rollup closes with the only one. The summary
omits the source name, because the heading above it already carries it.

```text
<!-- agentics:triage:section:{key} -->
<details><summary>{one-line subject} (run {RUN_DATE})</summary>

{section body — the adapter's own template}

</details>
<!-- agentics:triage:section-end:{key} -->
```

`{RUN_DATE}` is the run's UTC date, `YYYY-MM-DD`. It stays in the summary when a follow-up carries the section forward,
so a reader can tell which pass produced it.

A section is text the specialist wrote from logs and tool output, so the orchestrator runs the posting envelope's
redaction over it before it joins the body (Phase 6.4).

**Checks sections group by root cause.** Failed check runs that share one root cause render as one section. One root
cause means the same failed test or error signature, fixed or deferred together. Its summary names the cause and the job
count, for example `state.tsv ordering (5 jobs): fixed (run 2026-10-02)`, and its body links every job. Found still
counts one finding per failed check run, which matches the PR's list of red checks.

**Which sections a run renders:**

- A source this run triaged renders fresh sections from its `rollup_section` responses. When a follow-up re-checks a
  source, its sections in the execution's rollup are replaced, not merged: a check that passes now has no section.
- A fresh command reads no earlier rollup, so it carries no section. A source it excludes by `--source` renders as a
  `Skipped` row, and its block is the one sentence `Skipped (not in --source filter).`
- A follow-up carries every block it did not touch forward byte for byte, sections included, from each open anchor
  through its close anchor (see "Prior-row preservation").
- A source rendered `No new findings` keeps its one-sentence prior-run note and no sections.
- On `PUSH_STATUS` `push_blocked` or `fork_skipped`, render no fresh sections. The SHAs they cite do not exist on the
  remote.

## Base sync block

Base sync (Phase 0.4, not a Phase 1 source) runs on every PR-mode run, so its block always renders. The block is the
first Details block, under `#### Base sync`, keyed `base-sync`. Its body follows `SYNC_OUTCOME` (see
[`source-detection.md`](source-detection.md), "Base sync"):

- `up to date`, `merged cleanly`, `skipped — --no-sync`, `skipped — fork PR`: one plain sentence between the anchors,
  with no nested fold. A fork PR's sentence says the base was not merged because nothing from a fork is built or run on
  the maintainer's machine, so sources read the PR head unsynced.
- `merged — N conflicts resolved`: a section fold for the file table, with any version-bump note from the specialist's
  step 6. The fold holds one `| File | Disposition | Rationale |` row per conflicted file, each `fix` (a `defer` blocks
  the sync instead).
- `blocked`: the one block that is never folded. It renders directly under the header, which reads
  `@{PR_AUTHOR} — 1 needs you`. It shows a bold `**Base sync — blocked (run {RUN_DATE})**` line and a paragraph. The
  paragraph names the conflicted files and says that the merge was kept for the maintainer to finish. Phases 1-5 never
  ran, so there is no needs-you list and no Details. No table follows the block: carry-forward reads only the
  execution's own rollup, and a fresh command has none yet, so no prior row exists to list.

```text
#### Base sync

<!-- agentics:triage:section:base-sync -->
<details><summary>Merged <code>main</code> — 2 conflicts resolved (run 2026-09-24)</summary>

Version bump merged away: `package.json` carries the base's `2.4.0`.

| File | Disposition | Rationale |
| ---- | ----------- | --------- |
| `src/token-refresh.ts` | fix | Combined both sides' scheduling changes — base's backoff cap and the PR's jitter fix apply to different code paths. |
| `package.json` | fix | Took the PR's dependency bump; version field reverted to the base's, per the note above. |

</details>
<!-- agentics:triage:section-end:base-sync -->
```

## Body size budget

GitHub rejects a comment body longer than 65,536 characters. The rollup is always **one** comment. When the finished
body is longer than 65,000 characters, shrink it in this order. Stop at the first step that fits. Steps 1-3 only touch
sections this run generated fresh (its own `rollup_section` responses). They never touch carried-forward sections. A
follow-up keeps those byte for byte per "Source sections" above, so it never silently edits another source's earlier
output.

1. Replace each Checks section's log excerpt with a link to the check run's `details_url`.
2. Collapse each Checks section to one line between its anchors, with no `<details>` wrapper:
   `**{subject}** — {outcome}. [Log]({details_url})`. The `{subject}` is the section's check name or root cause.
3. Collapse every remaining freshly generated section to its summary text and a link to the source's own report. The
   link is, for example, the Checkmarx scan URL or the PR's conflict view. Keep the section between its anchors.

If the body still exceeds the limit after step 3, only then touch carried-forward sections:

4. Collapse carried-forward sections the same way (summary text and a link between their anchors). This is the one
   allowed exception to byte-for-byte preservation. It fires only when steps 1-3 on this run's own sections were not
   enough. Name the exception explicitly in that source's `<sub>` line. The line lets a reader (and a later follow-up)
   tell that the section was touched for size, not content.

If the body still exceeds the limit after step 4, apply a terminal compaction. During it, never drop a source row, a
disposition count, or a needs-you bullet to make room. Never split the rollup into a second comment. The steps are:

5. Truncate the Details blocks' free text (detection notes, bullet reasons) to fixed short forms. Then remeasure.
6. **Post the best body that fits.** Never claim success on a body that does not fit. The cases are:
    - Under 65,000 characters: post as usual.
    - Between 65,000 and GitHub's 65,536-character hard limit: post it with an added
      `**Truncated:** this run's content exceeded GitHub's comment size limit; section and detail text above was shortened to fit.`
      line as the last line inside Details.
    - Still at or over 65,536 after steps 1-5: the rollup cannot post, because GitHub hard-rejects it. Report the
      measured character count and which parts were shortened in the run's own output. Record it as a Phase 7
      verification warning (see 7.1). This case is not expected in practice: the Size guards in
      [`source-detection.md`](source-detection.md) already bound how many findings a single run can produce.

When any of steps 3-5 ran, name the shortened sections in the affected source's `<sub>` line.

## Construction instructions

1. Iterate over all subagent results collected in Phase 4.
2. Build one row per source per the rules above.
3. **Prior-row preservation.** A fresh command renders only the rows of its own `detected_sources`; it reads no earlier
   rollup. A follow-up applies "Prior-row preservation" to its execution's rollup.
4. Compute the Total row from the rollup's rows. `No new findings`, `Failed`, `Skipped`, and `Pending Review` rows
   contribute zeros.
5. Check the balance (counting rule 3). If a row is short, a disposition fell through the mapping. Route it to Needs
   you. Name it in the source's block. Do not post a table that does not add up.
6. Write the needs-you bullets. Then write the header verdict from the counts.
7. When the PR has an earlier rollup, render `{EARLIER_PASS_LINE}` directly under the header verdict, before the blocked
   base-sync block and `{SUPERSEDES_LINE}`, per "Earlier pass link". Phase 6.4 step 1 in
   [`orchestration-lifecycle.md`](orchestration-lifecycle.md) finds the earlier rollup.
8. Build the Details blocks per "Details blocks": base sync, detection notes, then one block per row with its sections.
9. Apply the SHA mapping from Phase 5 to any commit references.
10. When the PR carries a comment under a retired per-source marker, render `{SUPERSEDES_LINE}` (see Idempotency).
11. Fit the body to "Body size budget".
12. Run "Validate before posting". Correct any failure.
13. Post with `add_issue_comment` as a new top-level comment, with the `<!-- agentics:triage:rollup -->` marker and the
    execution key line. This is the run's only top-level comment. The command never updates an existing comment under
    any of the three rollup markers or a retired per-source marker. Only a follow-up edits a rollup, and only its own
    execution's.

## Prior-row preservation

Carry-forward is local to an execution: it reads only the execution's own rollup, never an earlier rollup on the PR. A
fresh command has no rollup yet, so it carries nothing. Its table holds only the sources it detected, and a source it
excludes by `--source` is a `Skipped` row (see "Source sections").

**Rule.** A follow-up rebuilds its own rollup from the body it holds. It changes only the rows and Details blocks of the
sources it touched. It then recomputes the Total row, the header verdict, and the needs-you list from the resulting
rows. Every other row and block, a `--source`-excluded source's included, carries forward byte for byte. A follow-up
that re-checks a source and detects nothing sets that source's row to `No new findings`, with every count rendered as
`—`, and its block to the one-sentence prior-run note. Without that row the source would vanish from the table, and the
developer could not tell whether it was resolved, skipped, or never re-checked.

**Example.** The command renders Copilot Review and Checkmarx One with counts. A follow-up re-checks Checkmarx One,
which detects nothing, and carries the Copilot Review row forward:

```text
| Source         | Status          | Found | Fixed | Accepted | Dismissed | Needs you |
| -------------- | --------------- | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed       | 2     | 2     | 0        | 0         | 0         |
| Checkmarx One  | No new findings | —     | —     | —        | —         | —         |
| **Total**      |                 | **2** | **2** | **0**    | **0**     | **0**     |
```

The Checkmarx block after the follow-up reads
`No new findings on this run (prior run: 3 findings, 2 fixed, 1 dismissed as not applicable).`

> **Marker hygiene.** Multiple distinct markers coexist in a triage run:
>
> - The three rollup markers (see Idempotency).
> - The execution key line `<!-- agentics:triage:execution:{RUN_ID} -->`, second in this execution's rollup (see
>   "Execution").
> - Retired per-source markers like `<!-- checkmarx-triage-summary -->`. The orchestrator reads them only to render
>   `{SUPERSEDES_LINE}`.
> - The section anchors `<!-- agentics:triage:section:{key} -->` inside this comment.
> - The colon-delimited `<!-- agentics:triage:reply:{source}:{threadId} -->`.
>
> The orchestrator checks each marker independently for idempotency. Never collapse them. When you look up legacy
> markers, use exact-match detection (not substring search).

## Examples

These rendered rollups are what conformance checks. Copy their shape.

**Simple:** one source, everything fixed.

```text
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000000-12345 -->

Triage — 3 fixed

| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |
| ------ | ------ | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed | 3 | 3 | 0 | 0 | 0 |

<details><summary>Details</summary>

#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date — `main` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

#### Copilot Review

- **Fixed (3):** `61b4f677` token refresh jitter · `8f510784` retry cap · `9c1d2e3f` backoff test

<sub>3 replies posted · threads are left for you to resolve</sub>

</details>

> _Triaged by an AI agent_
```

**Later pass:** a later command on a PR that already has a rollup; the link line sits directly under the header.

```text
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790600000-24680 -->

Triage — 2 fixed

Earlier triage pass: [rollup of 2026-11-04](https://github.com/acme/payments-api/pull/230#issuecomment-123456)

| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |
| ------ | ------ | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed | 2 | 2 | 0 | 0 | 0 |

<details><summary>Details</summary>

#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date — `main` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

#### Copilot Review

- **Fixed (2):** `3e7a91c2` null guard on refresh · `b4d08f15` timeout constant

<sub>2 replies posted · threads are left for you to resolve</sub>

</details>

> _Triaged by an AI agent_
```

**Bot Review:** one row per login, with a cross-bot duplicate counted under the owner source's mapping of its
disposition.

```text
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000100-23456 -->

Triage — 3 fixed

| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |
| ------ | ------ | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed | 1 | 1 | 0 | 0 | 0 |
| Bot Review (chatgpt-codex-connector[bot]) | Completed | 2 | 2 | 0 | 0 | 0 |
| **Total** | | **3** | **3** | **0** | **0** | **0** |

<details><summary>Details</summary>

#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date — `main` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

#### Copilot Review

- **Fixed (1):** `9f07e753` nil map write

#### Bot Review (chatgpt-codex-connector[bot])

- **Fixed (2):** `9f07e753` **Important (P1)** nil map write — also raised by Copilot Review; one fix serves both · `4c1d2e3f` **Minor (P2)** missing close

<sub>2 replies posted</sub>

</details>

> _Triaged by an AI agent_
```

**Mixed:** two sources, one item needs the author, base sync up to date.

```text
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000200-34567 -->

@jdoe — 6 fixed · 1 needs you

### Needs you

- **Checks — Go suite (macos-15, tools-rest):** `TestRunCycleClassifiesProviderOutcomes` fails non-deterministically; no source fix is supported by the run. [Failed job](https://github.com/acme/payments-api/actions/runs/1/job/2)

| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |
| ------ | ------ | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed | 6 | 5 | 1 | 0 | 0 |
| Checks | Completed | 2 | 1 | — | — | 1 |
| **Total** | | **8** | **6** | **1** | **0** | **1** |

<details><summary>Details</summary>

#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date — `main` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

#### Copilot Review

- **Fixed (5):** `8bf374e5` transaction scope · `c8f94ee4` raw-batch replay · `8e432a58` emergency record · `5857b0b7` hand-off scope · `f9f6e0e3` fresh-read
- **Accepted (1):** `api/cors.go:14` — the permissive CORS on the dev-only endpoint is deliberate.

<sub>6 replies posted</sub>

#### Checks

- **Fixed (1):** `a1b2c3d4` Format
- **Needs you (1):** Go suite (macos-15, tools-rest) — fails non-deterministically; no source fix is supported.

<!-- agentics:triage:section:checks:108511819201 -->
<details><summary>Format: fixed (run 2026-10-02)</summary>

**Category:** Formatting · **Tier:** Auto-fix

`npx prettier --check .` failed on one file the base sync merged. Fixed in `a1b2c3d4` by running Prettier on it.

</details>
<!-- agentics:triage:section-end:checks:108511819201 -->

<!-- agentics:triage:section:checks:108511819244 -->
<details><summary>Go suite (macos-15, tools-rest): needs you (run 2026-10-02)</summary>

**Category:** Test · **Tier:** Intervention

The job failed in `TestRunCycleClassifiesProviderOutcomes/needs` (`cycle exit = 1`, empty stderr). This PR changes no Go files, and the identical shard passed on the previous run. [Failed job](https://github.com/acme/payments-api/actions/runs/1/job/2)

</details>
<!-- agentics:triage:section-end:checks:108511819244 -->

</details>

> _Triaged by an AI agent_
```

**Unattended with a draft-lane source:** Copilot fixes landed; human feedback waits for someone to approve replies.

```text
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000250-11223 -->

@jdoe — 2 fixed · 1 needs you

### Needs you

- **Human Feedback:** not handled — replies here need someone to approve them, and no one was present.

| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |
| ------ | ------ | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed | 2 | 2 | 0 | 0 | 0 |
| Human Feedback | Pending Review | — | — | — | — | — |
| **Total** | | **2** | **2** | **0** | **0** | **0** |

<details><summary>Details</summary>

#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date — `main` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

#### Copilot Review

- **Fixed (2):** `5a1c0d2e` refund rounding · `7b2e1f30` missing timeout

<sub>2 replies posted</sub>

#### Human Feedback

Not handled — no one was present to approve replies, so nothing was drafted or posted.

</details>

> _Triaged by an AI agent_
```

**Blocked:** base sync stopped before any source ran, so the rollup has no table.

```text
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000300-45678 -->

@jdoe — 1 needs you

<!-- agentics:triage:section:base-sync -->
**Base sync — blocked (run 2026-10-02)**

Merge stopped on `src/token-refresh.ts`: both sides changed the same retry logic for different reasons, and the correct combination isn't derivable from the commits. The merge is kept for the maintainer to finish. No source was triaged this pass and nothing was pushed.
<!-- agentics:triage:section-end:base-sync -->

> _Triaged by an AI agent_
```
