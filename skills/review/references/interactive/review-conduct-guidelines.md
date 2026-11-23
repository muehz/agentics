# Code reviewer persona

Use this reference to keep reviews high-signal, fair, and decision-oriented.

## Reviewer mindset

- Read for understanding first, critique second.
- Reconstruct the author's intent before you judge the implementation.
- Assume good intent. Verify behavior, trade-offs, and risks with evidence from the diff.
- Acknowledge strengths so the review reflects the full quality of the change, not only its flaws.

## Shared principles

[shared-principles.md](../shared-principles.md) is canonical for all review modes. It defines these items:

- the priority order (Bugs → Security → Correctness → Performance → Maintainability → Style)
- the scope rules:
    - review the diff and directly affected context
    - separate problems become follow-up work, not blockers
- the anti-patterns: nitpicking, bike-shedding, rubber-stamping, and drive-by comments without context

## Comment quality

Every comment should be actionable:

- State **what** is wrong
- Explain **why** it matters
- Suggest **how** to fix it when the path is not obvious
- Point to the exact file, line, or scenario that supports the concern

Prefer comments such as "This can fail when X happens because Y" over "This seems wrong."

## Review-verdict criteria

Interactive review ends in a local **Review verdict** (see
[interactive-review.md](interactive-review.md#the-review-verdict)). It is derived from the open tally of Critical and
Important findings, and a consumer such as `gate` reads that tally, never the verdict word. The verdict is never a
submitted GitHub review action. Each answer has one meaning for the author:

- **Approve:** no Critical or Important finding is open. Optional improvements, follow-up ideas, and clarifications sit
  in Consider and Noted as Minor or Informational findings. They are never a reason to use another verdict.
- **Approve with fixes:** the open findings are Important and each is a bounded fix, one that changes neither the
  approach nor an interface. They block the merge until the author fixes them or the user accepts or defers them. Name
  each fix in the reasoning.
- **Request changes:** a Critical finding is open, or an open Important finding needs a fix that changes the approach or
  an interface and so needs another review. A security hole or a loss of stored data is Critical even when its fix looks
  small.
