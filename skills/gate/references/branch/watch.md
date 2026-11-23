# One-shot triage watch

Branch mode's Option 2 (draft PR or stack) loads this file after it pushes the gate's `state.tsv` rows, when the run
created the pull request or moved its head. A run that pushed nothing arms nothing, so a second gate run on an unchanged
pull request does not arm a second watch. Review bots and CI usually post signal within ~10 minutes of a draft PR.

## Contract

- **Exactly one check**, ~10 minutes after you arm the watch. A clean check is a terminal outcome. Report it and never
  re-arm.
- **The wait and the detection are deterministic.** The model reasons only after the detection confirms that signal
  exists.
- **Armed, announced, cancellable.** The rules are:
    - Arm it without asking once the row push lands.
    - Announce it in one line that says how to cancel.
    - A cancel said before the check fires disarms it.
    - Where the tool can cancel the mechanism, cancel it.
    - When the mechanism cannot be cancelled, the wake still happens but runs nothing. It reports one line:
        - "Check-back on PR #<N> was cancelled."
    - Posted signal still needs a yes before you act on it.
- **No writes.** The watch never posts to the PR. It either reports in-session or hands the user the callout.

## Capability test

Scan your available tools for one whose **documented contract** says all of these things:

- It fires after a delay or on completion.
- It does not block the current turn.
- It re-engages you when it fires.

Examples are a background task runner documented to re-invoke you on exit, and a scheduled-wakeup tool. The following
are disqualified regardless of harness:

- Foreground sleep, because it blocks the user's session
- A command run in the background where no completion contract is documented
- Any polling loop, because repeated checks are the runaway-inference failure mode this contract exists to prevent
- Provider cloud schedulers that check on their own cadence

If you cannot name a qualifying tool from its documented contract, you are not capable. Use the callout. Never probe the
environment to find out.

## Capable path

1. Arm the mechanism with a **deterministic payload that carries only the delay** (~10 minutes).
    - Put no detection queries and no analysis inside the payload.
    - An armed mechanism may not be cancellable. It must not be able to run anything a cancel should have stopped.
2. Announce it in one line: "Checking back once in ~10 minutes for posted signal on PR #<N> — say 'cancel the
   check-back' to stop it."
3. On wake, first check whether the user cancelled the check-back during the wait. If they did, run nothing and report
   "Check-back on PR #<N> was cancelled."
4. Otherwise, run the deterministic detection queries from triage's PR mode (see
   [pr-mode.md](../../../triage/references/pr-mode.md)) **before any reasoning**. The queries cover failed check runs,
   unresolved bot threads, and mergeable state.
5. When the hand-off recorded the PR as a stack layer, do these steps:
    - Read the GraphQL `stack` field again now, the field that gh's definition-of-done check reads. Membership may have
      changed during the wait.
    - Run the same queries on every lower layer that the `stack` field lists, because a change below cascades upward.
    - When that read fails or returns a null or unexpected `stack`, the watch is unverified for the lower layers. Report
      that in place of the clean line. Never call the stack clean.
6. Report the outcome:
    - Signal → report what was found and on which PR. Ask: "Want me to work through this posted signal (via
      `/agentics:triage pr <N>`)?" `<N>` is the PR that carries the signal.
    - Clean → report one line, "No signal on PR #<N> yet — not re-arming; run `/agentics:triage pr <N>` if signal lands
      later." Then stop.

## Callout (not capable)

> Review bots and CI usually post within ~10 minutes. Check back then and run `/agentics:triage pr <N>` — it sweeps bot
> findings, check failures, and merge conflicts in one pass.
