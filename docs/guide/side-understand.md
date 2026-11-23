# Understand

Build context before changing anything. This is a side page: use it from any phase of the main path when the code is
unfamiliar. Two skills serve four different questions: how something works, why it is that way, teach it to me, and what
was I doing.

## How does it work

```text
How does the payment retry pipeline work? Walk me through it from the webhook to the ledger write.
```

Why this phrasing works: "how does X work" plus the two endpoints ("from the webhook to the ledger write") triggers
[`explain`](../../skills/explain/SKILL.md)'s `how` mode and bounds the walkthrough. You get the mechanism between those
two points, not a tour of the whole codebase. Add "and critique the architecture" if you want judgment. By default the
walkthrough describes; it doesn't editorialize.

## Why is it this way

```text
Why is the retry limit 7? Find the decision history, with sources.
```

Why this phrasing works: "why" plus "with sources" triggers `explain`'s `why` mode: decision archaeology across git
history, issues, docs, and whatever evidence tools your host exposes. The answer comes back cited and
confidence-calibrated. A commit-backed reason is fact; an inferred reason is labeled inference. "With sources" makes the
desired output explicit, but the mode requires evidence either way. If no record exists, it says so.

## Teach it to me

```text
Teach me how our OAuth flow works. I want to actually understand it, not get a report.
```

Why this phrasing works: "teach me" and "actually understand" separate [`explain`](../../skills/explain/SKILL.md)'s
`teach` mode from its investigation modes. Teach is a paced lesson that checks you're following; `how`/`why` are an
investigation that returns a report. Say which one you want, because the outputs are shaped differently.

## What was I doing

```text
Catch me up on the exporter work. What was in flight, what's blocked, and what's the next move?
```

Why this phrasing works: "catch me up" triggers [`recall`](../../skills/recall/SKILL.md), which mines your session
transcripts and the shared record (git, GitHub), verifies against live state, and returns a tight brief. Naming the
topic ("the exporter work") scopes the search. Asking for "the next move" makes the brief end in an action, not a
summary.

**Pitfall:** don't skip this step because "the agent will read the code during implementation anyway." Reading-to-edit
is narrow: it finds the lines to change, not the decision history behind them. A change made without the `why` is how
deliberate behavior gets re-broken. The retry limit becomes 3 again, and the outage that made it 7 comes back.

Back to the main path: [03. Plan](03-plan.md), or the [index](README.md).
