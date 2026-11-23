# Reference-cleanup playbook

Rules for edits to `references/` files. Companion to [skill-compression-playbook.md](skill-compression-playbook.md),
which governs SKILL.md bodies. The two playbooks use opposite defaults.

## Default posture: presumed valuable

References are the detail layer. SKILL.md compression moves content _into_ them on the promise that the full version
lives there. Eval scenarios rarely load cold references, so a bad edit here can pass evals silently. Never run a
percentage-target compression pass on references. "Nothing to cut" is a valid, recordable outcome.

## Disposition before edit

Classify every file before you touch any file:

| Disposition     | Meaning                                                                                                                       |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `keep-verbatim` | Knowledge-dense or out-of-scope content. No edits.                                                                            |
| `trim-filler`   | Has intro throat-clearing, restated SKILL.md content, or section summaries to cut. Facts untouched.                           |
| `dedupe`        | Substantially duplicates another file (or its own SKILL.md). Merge toward one canonical home. Repoint the links.              |
| `delete`        | Orphaned (nothing links to it) AND obsolete. Verify orphanhood by grep before you delete it. Requires explicit user sign-off. |

Also tag heat: `hot` (loaded on the primary workflow), `warm` (mode-gated), `cold` (edge paths). Spend edit effort
proportional to heat; cold files default to `keep-verbatim`.

## In scope for cuts

Dead files, duplication, intro filler that restates the SKILL.md, superseded content.

## Out of scope: never densify

- Checklists (each line is a distinct check)
- Templates and prompt files (wording changes every downstream output)
- Code samples
- Domain-fact files (standards, API references)
- Command sequences

It is fine to trim prose _around_ these. Compression of the content itself is a hidden deletion.

## Never touch

- Model-pin tables and model/effort names (e.g., the model ladder's tier bindings). Pins move only by explicit user
  decision.
- Strings pinned by the repository's checks. Run the checks after edits. On failure, fix or revert. With no checks,
  re-read each edited link and quoted string by hand.
- Command text, flags, and arguments anywhere. A command that is the same in Bash and PowerShell is kept once; a second
  shell's block stays only where its syntax differs.
- Directory `README.md` index files. They are navigation, and convention keeps them even when nothing links to them.

## Fact-ledger verification

Evals guard the hot path only, so every edited file gets an independent diff review. In the review, enumerate each
discrete fact, check, command, or template field that the diff removes. Confirm that each one still exists in the file
or in a merged-to canonical home. Any dropped item → restore it or revert the file. Word counts are a report, not a
goal.
