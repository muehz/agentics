# Feedback loop construction

Spend enough time to build a loop that produces useful evidence.

## Ways to construct a loop

Try them in roughly this order.

1. **Failing test** at the seam that reaches the bug. This may be unit, integration, or e2e.
2. **Curl / HTTP script** against a running dev server.
3. **CLI invocation** with a fixture input. Diff stdout against a known-good snapshot.
4. **Headless browser script** with Playwright, Puppeteer, or an equivalent the project already uses. Drive the UI.
   Assert on DOM, console, or network output.
5. **Replay a captured trace.** Save a real network request, payload, or event log to disk. Replay it through the code
   path in isolation.
6. **Throwaway harness.** Spin up a minimal subset of the system (one service, mocked deps). The subset exercises the
   bug code path with a single function call.
7. **Property / fuzz loop.** If the bug is "sometimes wrong output", run 1000 random inputs to find the failure mode.
8. **Bisection harness.** If the bug appeared between two known states (commit, dataset, version), automate "boot at
   state X, check, repeat". The automation lets you `git bisect run` it.
9. **Differential loop.** Run the same input through the old version and the new version (or two configs). Diff the
   outputs.

With a loop that reliably detects the bug, most of the work is done. Every later step reads its result from that loop.

## Improve the loop

Treat the loop as a product. Once you have one, ask:

- **Can I make it faster?** Cache setup. Skip unrelated init. Narrow the test scope.
- **Can I make the signal sharper?** Assert on the specific symptom, not "didn't crash".
- **Can I make it more deterministic?** Pin time. Seed RNG. Isolate the filesystem. Freeze the network.

A 30-second flaky loop is barely better than no loop. A 2-second deterministic loop gives you a useful signal.

## Nondeterministic bugs

The goal is not a clean repro but a **higher reproduction rate**. To raise the rate, do any of these actions:

- Loop the trigger 100×.
- Parallelise.
- Add stress.
- Narrow timing windows.
- Inject sleeps.

For example, a bug that reproduces in half of the runs is practical to debug, and one that reproduces in 1 run in 100 is
not. Raise the reproduction rate until it is practical to debug.

Tactics:

- Run the trigger N times in a tight loop. Count the failures.
- Parallelise: many copies that race each other often expose timing bugs faster than serial repetition.
- Inject artificial slowdowns at suspected race points (`sleep`, `setTimeout`, throttled network). The slowdowns widen
  the timing window where the bug occurs.
- Stress unrelated subsystems (CPU, disk, network) to perturb scheduling.

Don't proceed to hypothesise until the rate is high enough. The rate is high enough when one cycle of "change something,
run loop, observe" actually tells you something.

## When you genuinely cannot build a loop

Stop. Say so explicitly. List what you tried. Then take these steps in order.

1. **Use the user as the loop.** When the failure shows only where the user can reach (their host, their account,
   production), ask for a command or procedure that the user runs and reports back. Its output is the loop once it shows
   the failure the user described. Ask for read-only commands, and never for a command that prints a secret's value.
2. **Ask for what is missing.** When no command the user can run shows the failure, ask the user for one of:
    - **Access** to whatever environment reproduces it.
    - **A captured artifact**: HAR file, log dump, core dump, screen recording with timestamps.
    - **Permission** to add temporary production instrumentation.
3. **Take the exit.** When the user can give none of them, no loop exists and none can be built. Go to
   [When no root cause is found](../SKILL.md#when-no-root-cause-is-found). Record that no loop exists, what you asked
   for and why each item was unavailable, then report the evidence you have, the gaps and the risk of a recurrence.

Do **not** proceed to hypothesise without a loop, including on the exit. Every hypothesis you generate without a loop is
a guess you cannot test. Every fix you ship without a loop is a guess you cannot verify. A change the user still wants
goes to `execute` as [a user's change](../SKILL.md#a-users-change), with no claim that it fixes the bug.
