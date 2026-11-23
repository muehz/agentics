# Epistemics

How to reason about confidence when evidence is historical, fragmentary, and sometimes contradictory. It also covers how
to communicate that confidence so it does not become false certainty. This is the suite's canonical epistemics file.
`diagnose` (Phase 3 and its cleanup checklist), `review` (the evidence posture), and `review`'s verdict-synthesis
reference all link here. None of them carries its own copy.

Code doesn't carry its own motivation. You can read what code does; you can't read _why it exists_. That lives in
commits, PRs, tickets, docs, and conversations, all incomplete, biased, and sometimes missing entirely. If you pretend
otherwise, you produce confident-sounding guesses that mislead the user.

## Confidence Tiers

Every claim in the final output must sit in one of these tiers. The tier determines which output section the claim goes
in and how you phrase it.

### 1. Direct

An explicit, textual citation that answers the question. Not "the code does X so the author must have wanted X."
Something an author actually _wrote_ that says why.

Examples:

- A PR description that says "this fixes the bug where users with >1000 items couldn't paginate"
- A ticket that says "we're adding this because customer Acme requested it in their security review"
- A code comment that says "// clamp to 100 because the upstream API rejects larger values"
- A design doc that says "we chose option A over option B because we need persistence across restarts"
- A chat message from the author that says "switching to this approach since the old one was flaky in tests"

Phrasing: confident, present tense. "This exists because X." Cite the source.

### 2. Supported

Multiple pieces of indirect evidence converge. No single source states it explicitly, but the pattern across sources
makes it likely.

Examples:

- The PR title says "improve performance," the ticket is labeled "perf," and the surrounding commits all touch the same
  hot path
- Multiple tests were added alongside the change, and all of them exercise edge cases with very large inputs
- The author's other PRs from the same week all mention the same incident in their descriptions

Phrasing: confident but explicitly derived. "The evidence points strongly to X: [the specific pieces]." Cite multiple
sources.

### 3. Inferred

A reasonable reading of the context, but nothing explicitly supports it. The reader should understand this is _your
interpretation_, not a fact from the record.

Examples:

- The PR doesn't say why. The error happened in production (per the incident channel timing), and the fix was rushed
  (merged the same day), so it was likely a hotfix.
- The function name suggests retry logic. The retry count is 3. This matches the team's general convention of "3
  retries", which appears elsewhere in the codebase.

Phrasing: hedged. "It appears", "likely", "suggests", "is consistent with", "one reading is". Make the inference chain
explicit: "Given A and B, C seems likely because D."

### 4. Speculative

A plausible hypothesis, but the evidence is thin and other explanations fit equally well. These hypotheses can help.
Mark them as guesses.

Examples:

- "This might be a workaround for a browser bug that's since been fixed, but we found no contemporary evidence of that."
- "It's possible this threshold was chosen to match an SLA commitment, but no SLA doc references it."

Phrasing: explicitly speculative. "One possibility is X, but we have no direct evidence." Usually lives in the
"Competing Hypotheses" section alongside other possibilities.

### 5. Unknown

You looked and couldn't find out. A valid and important outcome. Document it.

Phrasing: "We searched X, Y, and Z and found no evidence of why." Be specific about _what_ you searched. "We couldn't
find out" is less useful than this example: "We searched the ticket tracker with keywords A and B. We scanned the 6 PRs
that touched this file since 2023. We grep'd the repo for string literals that match the threshold. None surfaced a
rationale."

## Phrasing Guide

### Words that carry confidence. Use carefully

These imply **Direct** or **Supported** confidence. Don't use them for inferences.

- "because". Implies a causal claim with evidence
- "the reason is". Same
- "was designed to". Claims author intent
- "fixes", "addresses", "solves". Claims the change achieved its goal
- "the team decided". Claims a group decision happened

If you use these, you should have a citation immediately adjacent.

### Words that hedge. Use for inferences

- "appears to"
- "seems to"
- "likely"
- "suggests"
- "is consistent with"
- "one reading is"
- "plausibly"
- "may have been"
- "the evidence points toward"

These signal that you interpret, not that you report. Use them liberally in the "What We Can Reasonably Infer" section.

### Words to avoid

- "obviously". If it were obvious, the user wouldn't ask
- "clearly". Almost always precedes a claim that isn't clear
- "of course". Same
- "just" (as in "it's just X for performance"). Dismissive and usually hides uncertainty
- "I think" / "I believe". You synthesize evidence, not personal opinion. Use "the evidence suggests" instead.

### Avoid rationalization

Code that "makes sense" today may have been written for reasons that no longer apply, or that were wrong when they were
written. Don't retrofit a clean rationale onto messy history.

Resist the urge to:

- Assume the author did the "right" thing, or work backward to justify it
- Assume a consistent pattern across the codebase was intentional when it might be copy-paste
- Turn an absence of evidence into evidence of absence. An example is "no one mentioned security concerns, so it must
  not have been a concern"

## The Sycophancy Trap

Users often phrase `why` questions with an embedded hypothesis: "Why do we do it this way, I assume it's for
performance?" Don't simply confirm it. Treat it as one candidate among others. Check the evidence independently. If the
evidence supports it, say so with citations. If it does not, do these steps:

1. Say so.
2. Present what the evidence _does_ support.

The user's guess is a prompt for investigation, not a conclusion to validate.

## When Evidence Contradicts

If two sources disagree (the PR description says one thing, the ticket says another), surface both. Don't pick the one
that fits a tidier narrative. A typical pattern:

- **The ticket says** "we need this for customer X's compliance requirement"
- **The PR says** "cleaning up tech debt in this area"

Both may be true (the ticket motivated the work, the PR is the author's framing of it), or one may be wrong. Present
both with their citations. Let the user make the call.

## When Evidence Is Missing

An honest "we don't know" is one of the most valuable outputs this skill can produce. The user now knows:

- The answer isn't in the obvious places
- They'll need to ask a human (the original author, the product owner, the team lead) to find out
- Or they can decide the question isn't worth further pursuit

If you fail to mark a gap and fill it with a confident guess, you actively harm the user. They will act on the guess.

When you hit a gap, name it concretely:

- What question you tried to answer
- What sources you searched
- What you searched for in each
- What you found (nothing, or only tangentially related material)

## Calibration Check Before Delivery

Before delivery, the synthesizer should review every claim in "What We Found" and "What We Can Reasonably Infer". For
each claim, it should ask:

1. Does this claim have a citation? If not, either add one or move it to "Inferred" / "Hypotheses".
2. Is the phrasing calibrated to the tier? (A Direct claim can use "because"; an Inferred claim cannot.)
3. Do I treat the code itself as evidence for its own intent? If so, that's not evidence. Remove or reclassify.
4. Does the output include a "What We Don't Know" section? If the output mentions no gaps, that's suspicious. Either the
   evidence was unusually complete or something is hidden.
