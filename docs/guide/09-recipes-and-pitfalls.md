# 09. Recipes and pitfalls

Three end-to-end prompt sequences, then every pitfall from the guide in one table.

## Recipe: a feature, idea to shipped

```text
1. I want CSV export on the reports page. Capture and challenge the intent before any implementation.
2. I approve the intent. Write the solution spec.
3. Decompose the spec into delivery slices when they can ship independently, then write an ordered plan with dependencies.
4. The plan is approved. Execute it.
5. Optional: review the branch against main interactively, with a review verdict.
6. I'm done with this feature branch. Run the gate.
7. Invoke the user-only `handoff` skill with your host's explicit syntax.
```

Each line is one phase boundary from pages [03. Plan](03-plan.md) through [06. Ship](06-ship.md). The sequence works
because each downstream phase receives the artifact and approval it requires. Intent asks for your approval, spec gets
an independent review, and decompose reviews the written task graph before validation and execution approval. On routine
work step 3 asks one question that approves the spec and the plan together. Elevated work, such as a public contract,
asks you to approve the spec at step 2 and the plan at step 3. Step 4 runs in `auto`: subagents implement, model quality
reviews run per review group, integration reviews run at group boundaries when their triggers fire, and nothing pauses.
Say "use pair mode" at step 4 to review each wave yourself. Nothing asks `execute` for judgment or `intent` for code.
Step 5 is the one optional line: the gate's own review runs either way, and the interactive pass adds only the review
verdict, which gate's readiness answer reads as one input.

## Recipe: a bug, report to merged

```text
1. Exports started failing for large workspaces yesterday. Diagnose it: root cause before any fix.
2. Fix it. Take the small-fix path if it qualifies; otherwise start the chain.
3. What could this fix break outside the diff? Prove the safety fact by running code.
4. I'm done with this branch. Run the gate, then open a draft PR: description, linked issue.
5. Triage the PR in one pass: checks, comments, bot findings.
```

Root cause comes first ([05. Verify](05-verify.md)). Once the cause is known, routing classifies the fix. When every
small-fix signal holds (a one-sentence diff, one concern, no interface, contract, trust-boundary, or data-handling
change, a covering test, and at most 3 non-test files and 50 changed lines with no non-test file more than half
rewritten), it announces the signals, and `execute fix` commits the failing test on its own, then the fix, with no work
folder. Otherwise the fix enters the chain at `intent`, as in the feature recipe. Say "full chain" at any point to move
a small fix into the chain. Either way, `gate` closes the branch. For a small fix it checks the record's `skip chain`
row, the verification and review, the controller audit, the red commit, and the PR description's reason, signals, and
red commit SHA.

## Recipe: an outage, page to drill

```text
1. Checkout is down in production, customers affected. Run incident response.
2. Write the blameless postmortem for the incident.
3. Author a runbook from the postmortem's action items.
4. Drill me on that runbook.
```

The chain from [Operate](side-operate.md): each artifact seeds the next, and the drill is what proves the runbook would
survive contact with 3 a.m.

## The pitfalls, consolidated

| Page                                                         | Pitfall                                                                                                                                  |
| ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| [01. Setup](01-setup.md)                                     | A zero exit code is not a verified install. Send the verify prompt and expect the routed answer                                          |
| [02. Routing](02-routing.md)                                 | A skill-sounding noun is not a route. State the outcome; routing classifies by what's produced                                           |
| [03. Plan](03-plan.md)                                       | Don't ask for a plan mid-discussion. Planning decomposes reviewed specs, never a bare intent; unpressured assumptions ride into the plan |
| [04. Build](04-build.md)                                     | Don't hand `execute` a bare goal. It needs a decomposed task list, or you buy a round trip through `decompose`                           |
| [05. Verify](05-verify.md)                                   | "Does this look right?" buys an opinion. Ask for proof that runs: gates, blast-radius facts, verdicts                                    |
| [06. Ship](06-ship.md)                                       | Don't end a session without `handoff`. `recall` is the expensive fallback; the capsule is cheap and exact                                |
| [Understand](side-understand.md)                             | "The agent reads the code anyway" finds the lines, not the decision history. Skipping the `why` re-breaks deliberate behavior            |
| [Operate](side-operate.md)                                   | During an outage, state impact plainly and don't split the report. Technical vocabulary invites bug-routing mid-incident                 |
| [07. Hooks](07-hooks.md)                                     | Don't answer a block by disabling the hook. Narrow allowlist entries and suggested safe forms exist for exactly that                     |
| [08. Extend](08-extend.md)                                   | Don't forge prose for what structure enforces. Hooks, lint, and CI fire every time; instructions drift                                   |
| [10. Update and troubleshoot](10-update-and-troubleshoot.md) | A refresh on one coding tool does not update every install. Check the installation path and reload or start a new session                |

If one page earns a re-read after some time with Agentics, it's [02. Routing](02-routing.md). Nearly every pitfall above
is a routing signal you control with phrasing.

Next: [10. Update and troubleshoot](10-update-and-troubleshoot.md)

Back to the [index](README.md).
