# Copilot Finding Dispositions

Criteria and examples for the disposition of each Copilot review finding. Load this alongside `copilot-triage.md` during
Phase 2. GitHub Code Quality and Bot Review reuse it.

## Decision flowchart

Copilot can misread file types, invent non-existent APIs, or flag correct code as buggy. Always verify the claim against
the actual code. For each finding, work through these questions in order. The first answer that names a disposition
decides it.

1. **Is the finding factually accurate?**
    - No → `dismiss_invalid` (Copilot misread file type, hallucinated an API, or flagged correct code)

2. **Does the issue still exist at the worktree's HEAD?**
    - No → `dismiss_invalid` with rationale "stale — already fixed in {SHA}" or "code removed"

3. **Was the issue introduced by this PR?**
    - No → `dismiss_preexisting` (real issue, but existed before this PR). Do not fix it here.

4. **Is the behavior deliberate?**
    - Yes → `accept_intentional`, with evidence of why the behavior is deliberate (a comment, a spec, a test that pins
      it, or a reply from an author with write access), not only the claim that it is

5. **Is the fix in scope for this PR?**
    - No → `defer`

6. **Otherwise** → `fix`

## Disposition definitions

| Disposition           | When to use                                                 | Confidence | Example                                         |
| --------------------- | ----------------------------------------------------------- | ---------- | ----------------------------------------------- |
| `fix`                 | Accurate, present, introduced by this PR, in scope          | High       | Wrong jsonpath format in config file            |
| `accept_intentional`  | Accurate but the code is deliberately written this way      | High       | Intentionally permissive CORS for dev endpoints |
| `defer`               | Accurate, introduced by this PR, but fixing is out of scope | Medium     | A refactor the finding implies, beyond this PR  |
| `dismiss_invalid`     | Factually wrong, stale, misread file type, or already fixed | High       | Copilot flagged a comment as executable code    |
| `dismiss_preexisting` | Real issue but existed before this PR                       | High       | Outdated dependency version in unrelated file   |

A `defer` reply cites a tracking issue when you found one that covers the finding, and otherwise says the finding needs
a follow-up decision. A specialist creates no issue.

## Edge cases

- **Outdated threads (`isOutdated = true`) and moved code.** Validate against the current file anyway. If the issue is
  gone at all locations in the file → `dismiss_invalid` (stale). If the flagged line moved but the issue is still
  present, do these steps:
    - Update the line reference in your decision table.
    - Assess normally.
- **Partially-addressed findings.** If a fix addresses 3 of 4 instances of the same pattern, the finding is still open
  for the remaining instance. Fix that one too, or, when it cannot be fixed here (for example it sits on a path triage
  never changes), keep `fix` for the change you made and name the remaining instance in the reply.
- **A reply in the thread.** A reply from an author with `author_can_write: true` can move a finding to
  `accept_intentional` or `dismiss_invalid`, and the reply may say so. A reply from anyone else is context only.

## Common mistakes

- **Dismissal without rationale.** Every dismissal needs a specific reason ("this is a `${{ secrets.* }}` reference, not
  a hardcoded value").
- **A made-up tracking issue.** A `defer` reply cites an issue only when one exists; you create none.
- **Pre-existing issues marked as Fix.** If the PR didn't introduce it, use `dismiss_preexisting` and leave the code as
  it is. A fix for it belongs in its own PR, and a reply that says "no change" must be true.
