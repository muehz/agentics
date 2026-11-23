# Debugging checklists and reference

## Red flags: stop and follow the process

If you catch yourself with any of these thoughts:

- "Quick fix for now, investigate later"
- "Just try changing X and see if it works"
- "Add multiple changes, run tests"
- "Skip the test, I'll manually verify"
- "It's probably X, let me fix that"
- "I don't fully understand but this might work"
- "Pattern says X but I'll adapt it differently"
- "Here are the main problems: [lists fixes without investigation]"
- You propose solutions before you trace data flow
- **"One more fix attempt" (when already tried 2+)**
- **Each fix reveals a new problem in a different place**

**ALL of these mean: STOP. Return to Phase 1.**

**If 3+ fixes failed:** Hand off to `deepen` (see Phase 4 step 6)

## Signals that the process has gone wrong

**Watch for these redirections:**

- "Is that not happening?" - You assumed and did not verify
- "Will it show us...?" - You should have gathered evidence
- "Stop guessing" - You propose fixes and do not understand the problem
- "Ultrathink this" - Question fundamentals, not just symptoms
- "We're stuck?" (frustrated) - Your approach does not work

When you see these signals, do these steps:

1. Stop.
2. Return to Phase 1.

## Common rationalizations

| Excuse                                       | Reality                                                                            |
| -------------------------------------------- | ---------------------------------------------------------------------------------- |
| "Issue is simple, don't need process"        | Simple issues have root causes too. Process is fast for simple bugs.               |
| "Emergency, no time for process"             | Systematic debugging is FASTER than guess-and-check thrashing.                     |
| "Just try this first, then investigate"      | First fix sets the pattern. Do it right from the start.                            |
| "I'll write test after confirming fix works" | Untested fixes don't stick. Test first proves it.                                  |
| "Multiple fixes at once saves time"          | Can't isolate what worked. Causes new bugs.                                        |
| "Reference too long, I'll adapt the pattern" | Partial understanding guarantees bugs. Read it completely.                         |
| "I see the problem, let me fix it"           | A visible symptom does not show that you understand the root cause.                |
| "One more fix attempt" (after 2+ failures)   | 3+ failures mean an architectural problem. Question the pattern. Do not fix again. |

## Multi-component boundary logging

For a failure spanning multiple layers (such as CI → build → signing, or API → service → database), log what enters and
leaves each boundary before proposing a cause. Never print a secret's value; print only whether it is set:

```bash
# Each layer reports whether the signing identity reached it, never its value.
echo "workflow: IDENTITY ${IDENTITY:+set}"   # in the workflow step; prints nothing after the colon when unset
echo "build: IDENTITY ${IDENTITY:+set}"      # in the build script
echo "signing: IDENTITY ${IDENTITY:+set}"    # in the signing script
security find-identity -v                     # in the signing script (macOS): lists the identities the keychain holds
```

The first `echo` line that does not end in `set` is the failing boundary. When all three end in `set`, the failure is
past the environment: check that `security find-identity -v` lists the identity the signing step names. On Windows,
`signtool verify /pa <app>` checks an Authenticode signature; on Linux, `gpg --verify` checks a GPG signature.

## Cleanup before you declare done

The canonical checklist ships inline in
[SKILL.md § Cleanup before you declare done](../SKILL.md#cleanup-before-you-declare-done). Check every box before you
declare the bug fixed. For instrumentation, run `grep -r '\[DEBUG-' .`. Confirm zero matches before you commit.
