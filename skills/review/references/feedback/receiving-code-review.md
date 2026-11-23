# Code review reception

**Core principle:** Verify before implementing. Ask before assuming. Technical correctness over social comfort.

## The response pattern

[receiving-feedback.md](receiving-feedback.md) is canonical for the core principles and the anti-sycophancy rules. It is
also canonical for the response pattern: read fully, restate, verify, triage, respond with evidence, then implement.
[Acknowledging correct feedback](#acknowledging-correct-feedback) below lists the exact banned phrases.

## Handling unclear feedback

```text
IF any item is unclear:
  STOP - do not implement anything yet
  ASK for clarification on unclear items

WHY: Items may be related. Partial understanding = wrong implementation.
```

**Example:**

```text
User: "Fix 1-6"
You understand 1,2,3,6. Unclear on 4,5.

❌ WRONG: Implement 1,2,3,6 now, ask about 4,5 later
✅ RIGHT: "I understand items 1,2,3,6. Need clarification on 4 and 5 before proceeding."
```

## Source-specific handling

### From the user

- Trusted. Implement after you understand the feedback.
- If the scope is unclear, still ask.
- No performative agreement.
- Skip to action or a technical acknowledgment.

### From external reviewers

```text
BEFORE implementing:
  1. Check: Technically correct for THIS codebase?
  2. Check: Breaks existing functionality?
  3. Check: Reason for current implementation?
  4. Check: Works on all platforms/versions?
  5. Check: Does reviewer understand full context?

IF suggestion seems wrong:
  Push back with technical reasoning

IF can't easily verify:
  Say so as a ❓ question: "I can't verify this without [X]." Options: investigate, ask the reviewer, or proceed; the ➡️ recommends one based on the change's risk.

IF conflicts with the user's prior decisions:
  Stop and discuss with the user first
```

**Rule:** Be skeptical of external feedback, and check it carefully.

## YAGNI check for "professional" features

```text
IF reviewer suggests "implementing properly":
  grep codebase for actual usage

  IF unused: "This endpoint isn't called. Remove it (YAGNI)?"
  IF used: Then implement properly
```

**Rule:** The reviewer and you both serve the user's goals. If the feature is not needed, do not add it.

## Implementation order

Work within the canonical batch order in [receiving-feedback.md](receiving-feedback.md): clarify unclear feedback first,
then fix and validate each batch. Within a batch, fix issues in this sequence:

1. Blocking issues (breaks, security)
2. Simple fixes (typos, imports)
3. Complex fixes (refactoring, logic)

## When to push back

[receiving-feedback.md](receiving-feedback.md) defines the canonical push-back triggers and evidence-based technique.
[Source-specific handling](#source-specific-handling) above holds the source-specific checks (breaks existing
functionality, reviewer context, conflicts with the user's decisions).

## Acknowledging correct feedback

When feedback IS correct, this section governs the agent's own wording, in the chat and in a reply it drafts. Text the
user dictates for a reply is the user's: post it as written, thanks included, after the outbound checks and with the
envelope's footer ([GitHub thread replies](#github-thread-replies)). Never strip or reword a dictated reply to fit this
section.

```text
✅ "Fixed. [Brief description of what changed]"
✅ "Good catch - [specific issue]. Fixed in [location]."
✅ [Just fix it and show in the code]

❌ "You're absolutely right!"
❌ "Great point!"
❌ "Thanks for catching that!"
❌ "Thanks for [anything]"
❌ ANY gratitude expression in your own wording
```

**Why no thanks.** The code itself shows you heard the feedback. If you notice that you are about to write "Thanks" in
your own words, delete it. State the fix instead.

## Gracefully correcting your pushback

If you pushed back and were wrong:

```text
✅ "You were right - I checked [X] and it does [Y]. Implementing now."
✅ "Verified this and you're correct. My initial understanding was wrong because [reason]. Fixing."

❌ Long apology
❌ Defending why you pushed back
❌ Over-explaining
```

State the correction as a fact, then continue the work.

## Common mistakes

| Mistake                      | Fix                                 |
| ---------------------------- | ----------------------------------- |
| Performative agreement       | State requirement or just act       |
| Blind implementation         | Verify against codebase first       |
| Batch without validating     | Validate after each batch           |
| Assuming reviewer is right   | Check if breaks things              |
| Avoiding pushback            | Technical correctness > comfort     |
| Partial implementation       | Clarify all items first             |
| Can't verify, proceed anyway | State limitation, ask for direction |

## Real examples

**Performative agreement (bad):**

```text
Reviewer: "Remove legacy code"
❌ "You're absolutely right! Let me remove that..."
```

**Technical verification (good):**

```text
Reviewer: "Remove legacy code"
✅ "Checking... build target is 10.15+, this API needs 13+. Need legacy for backward compat. Current impl has wrong bundle ID - fix it or drop pre-13 support?"
```

**YAGNI (good):**

```text
Reviewer: "Implement proper metrics tracking with database, date filters, CSV export"
✅ "Grepped codebase - nothing calls this endpoint. Remove it (YAGNI)? Or is there usage I'm missing?"
```

**Unclear item (good):**

```text
User: "Fix items 1-6"
You understand 1,2,3,6. Unclear on 4,5.
✅ "Understand 1,2,3,6. Need clarification on 4 and 5 before implementing."
```

## GitHub thread replies

Reply to inline GitHub review comments in the comment thread, not as a top-level PR comment:

```bash
gh api repos/{owner}/{repo}/pulls/<pr-number>/comments/<comment-id>/replies -F body=@<reply-file>
```

`gh` fills in `{owner}` and `{repo}` from the current repository. Replace `<pr-number>` and `<comment-id>`.
`<comment-id>` is the `id` of the first comment in the thread. Write the reply to a file and pass its path as
`<reply-file>`. A reply typed inside double quotes lets the shell run any `$(...)` or backticks in it.

Every reply Agentics posts follows the shared marker, body, and footer rules in
[skills/gh/references/posting-envelope.md](../../../gh/references/posting-envelope.md), so a review reply carries the
envelope's footer too. A reply posts only under the skill's [Posting](../../SKILL.md#posting) rule: a reply the user
names (for example "reply to that thread saying …") posts with no second question, and a reply the user did not name is
shown once, exactly as it will post, and posted after one yes. A reply the agent drafts from a triage packet is
draft-only; the packet's `effects` field says so.

## The bottom line

External feedback is suggestions to evaluate, not orders to follow. The canonical bottom line is in
[receiving-feedback.md](receiving-feedback.md).
