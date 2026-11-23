# Adversarial pair pattern

A two-part contract for destructive or judgment-heavy sweeps pairs a **maximally biased finder** with a **wrapper skill
that owns all judgment**. Use it when both of these are true:

- A balanced single agent would defer to justifications already present in the code or its author's prose.
- An unchecked extremist would do damage.

The pair gets the extremist's recall and the audit's precision.

First shipped instance: `skills/zealot` (finder charter: `comment-zealot.md` in `skills/zealot/references/comments/`).

## When to use it

- The target defends itself in prose ("do not remove", "IMPORTANT", long justifications), so a neutral reviewer inherits
  the author's framing.
- Findings are cheap to verify but expensive to miss. Over-collection plus audit beats polite under-collection.
- The action is destructive enough that raw extremist output must never land unreviewed.

If findings are expensive to verify or the sweep is advisory-only, do not build a pair. A single balanced reviewer is
cheaper.

## The five mechanisms

Every pair implements all five. A pair that drops one mechanism breaks the contract:

- No leash → noise.
- No burden inversion → sycophancy returns.
- No capability bound → unauditable damage.
- No wrapper audit → the extremist ships.
- No rerun budget → infinite bad-report loops.

1. **Calibration by leash.** The finder is maximally biased in one direction. Its only counterweight is a tiny,
   explicit, closed exception list. For zealot, the list holds legal headers, proven external constraints,
   `prettier-ignore`, public API doc contracts, and issue links that code cannot express. Uncertainty resolves toward
   the finding, never toward mercy. The audit exists to catch overreach, so the finder never self-moderates. Write the
   leash as a closed list; "use judgment" is not a leash.

2. **Inverted burden of proof.** An exception claim survives only with proof, verified against the code, that it
   concerns something the codebase cannot change. To verify the proof:
    - Read the site.
    - Trace the callers.
    - Check the external dependency's documentation.

    The finder's own assertion is a lead to check, not proof. A refuted or still-ambiguous exception claim resolves
    toward the finding.

3. **Capability bound.** The finder gets the narrowest capability that keeps its report checkable. For zealot, the
   finder may delete comments and flag symbols `MUST KILL`, nothing else, and it is read-only on application code. The
   audit auto-rejects anything beyond the bound regardless of merit, because an out-of-bound edit can't be verified as a
   finding.

4. **The wrapper owns judgment.** Findings are claims, not facts. The wrapper audits every report, or delegates the
   audit to a fresh reviewer that is neither the finder nor the wrapper. In the audit, reject scope escapes,
   out-of-bound edits, exception-protected hits, and findings whose stated reason misstates the code. The audit is a
   process gate. When the caller wants the raw output or declines the audit, name its cost once: an unaudited report can
   carry scope escapes and findings that misstate the code. When the next reply still declines, hand over the report
   itself, marked unaudited, and record that the audit did not run. Handing it over gives the caller a report to read;
   it does not apply the report's findings to the code. Never describe an unaudited report as audited. A delegated audit
   keeps the wrapper in charge of scope, the record of a skipped audit, the rerun decision, and the application of the
   verdicts. The wrapper never overrides a reviewer rejection. It runs the audit itself only when the reviewer cannot be
   dispatched. Zealot's comments mode uses this three-role form.

5. **Rerun budget.** A rejected item earns exactly one rerun, with the failure named in the finder's prompt. Restore
   only the work that holds a rejected item, keep the accepted work everywhere else, and rerun once on what was
   restored. Audit the second report the same way. What again holds a rejected item is restored and reported open with
   the rejection reasons; one wrong item never reverts the whole run. The wrapper never edits a report into
   acceptability; that silently converts the extremist's bias into the wrapper's own.

## Constraint conversion (when findings claim permanence)

Where a finding meets a claim of protected status ("do not remove", "talk to X first"), the pair's final step has two
parts:

1. With the user's approval, convert the claim into an enforced artifact. The artifact is a type, a runtime assertion, a
   test, or a CI check.
2. Then delete the prose, once the artifact exists and runs.

A constraint that matters is worth a check. When the user declines the encoding, or it cannot be built, keep the prose
as it was and report the constraint unenforced: the prose is the only record of the constraint, and deleting it loses
that record.

## Authoring checklist for a new pair

- [ ] Finder charter is a self-contained prompt, embedded verbatim at spawn (the `surface-dispatch.md` charter-embedding
      mechanism). The finder is **not** a roster agent, and the roster stays domain-typed.
- [ ] The leash is a closed exception list, each entry externally verifiable.
- [ ] The capability bound is stated in the charter and re-checked at audit.
- [ ] The wrapper's audit rules name what gets auto-rejected.
- [ ] The rerun budget is one, a rejected report restores and reruns only what holds a rejected item, and the second
      failure's output shape is defined: those items reported open with the rejection reasons.
- [ ] The conversion step keeps the prose and reports the constraint unenforced when its encoding is declined or cannot
      be built.
