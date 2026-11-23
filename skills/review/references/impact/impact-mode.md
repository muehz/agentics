# Impact mode

Find what a change breaks somewhere else before it ships. The job is not to list the callers, because any agent can grep
them in a second. The job is to find breakage that grep will not show you.

## Don't use when

These rows add to the skill's [Don't use when](../../SKILL.md#dont-use-when) table and are handled the same way: a
**cannot** row forwards as [forwarding.md](../../../routing/references/forwarding.md) says, once.

| Input signal                                                                       | Use this instead                   | Kind   | Why                                                                                  |
| ---------------------------------------------------------------------------------- | ---------------------------------- | ------ | ------------------------------------------------------------------------------------ |
| The breakage already happened, including a bug, error, failing test, or regression | `/agentics:diagnose`               | cannot | This mode runs pre-ship. `diagnose` finds the root cause of an observed failure.     |
| A plan or design with no code yet that needs a stress test                         | `/agentics:challenge`              | cannot | With no real code to run a proof against, the evidence ladder cannot leave step 1.   |
| A finished fix to verify before the claim that the work is done                    | `/agentics:gate check`             | cannot | `gate` owns completion checkpoints; this mode assesses outward impact, not doneness. |
| A security-focused review of a change (injection, authz, secrets)                  | `/agentics:review axes --security` | cannot | Security review has its own checklists. Impact hunts functional breakage elsewhere.  |

If the ask is "what does this change break for the rest of the system," it belongs here, however small the diff.

## Don't trust your own writeup

An impact writeup that sounds right is worthless: it can sound convincing whether or not it is true. Do not hand back
the writeup alone. Find the one or two facts the whole thing depends on. Then prove them with code that you run.

### The evidence ladder

For each fact the change's safety depends on, get it as far down the ladder below as is cheap. For each fact, also say
where it stopped.

1. **You said so.** Worthless on its own.
2. **You pointed at the line.** A real `file:line`, or the library's own source, opened and read in this session.
    - A fact recalled from training, however specific, is step 1 until you fetch the source and read it.
    - A changelog you remember is not a source you consulted.
3. **You walked the failure.** You showed step by step that the bad case can't reach.
4. **You ran it.** A script or test that calls the real code and fails loud if you're wrong.
5. **You reproduced it in the running app.**

If you cannot get a safety fact to step 4, say so explicitly. Write **unproven**. Never write the fact up as settled.
Step 4 is usually one small script. It imports the same library the app ships and calls the exact function you're
worried about.

## Steps

0. **Preflight.** Confirm that you have a concrete change to assess: a diff, a PR, a commit, or a described code change
   you can read in the repository. A request that matches a row above forwards by its kind. With no change to point at,
   the skill's [mode precondition](../../SKILL.md#step-0-preflight) applies: name what is missing and ask for a change,
   or forward a design to `/agentics:challenge`.

1. **Read the change.** Read the diff, the symbols it adds, changes, and deletes, and what it now does differently.
   Include the part the diff doesn't spell out. When the change lives in a PR, pull the PR body and its commits via `gh`
   before you reason about it.

2. **Find the one fact it's safe because of.** Most changes that look scary are safe because of a single fact. If that
   fact holds, it rules out most of the scary cases at once. Spend your time here, not on a long list of maybes. An
   example of such a fact is "this call only drops already-dead cache entries and does nothing else."

3. **Look where grep stops.** Read the source of the library you call. Check its pinned version and any local patch.
   Work out when things run, including microtasks, unmount and teardown, and framework scheduling differences. Follow
   what a symbol search misses. That includes:

    - the JSON an API returns
    - a DB column
    - a wire format
    - another language that reads the same bytes
    - a feature flag
    - code three hops downstream

4. **Be honest about each risk.** Give each risk a real likelihood and a real cost if it happens. Keep the risks you
   confirmed. List the ones you checked and cleared separately. Cite a real `file:line` for every claim. A search that
   finds nothing is still an answer. Never invent a caller or an API.

5. **Prove the one fact.** Write a script or test that runs the real code. Run it. Paste what happened. If you can't
   prove it cheaply, mark it unproven. Don't round up. Run any proof script from a scratch location outside the tracked
   tree, or delete it after you run it. Never leave it staged or committed in the reviewed repo.

    **Whose code runs.** The proof runs the change's code on this machine. When the change is the user's own work, or a
    branch of this repository, run it. When it is a pull request whose head is in another repository (a fork), ask once
    before running it: the proof executes code nobody here wrote. On a no, or in an
    [unattended](../../../routing/references/contracts/unattended.md) run, hand back the script labeled **template, not
    run**, and the fact stops at the ladder step you reached.

    When the repo lacks what a runnable proof needs, do these steps:

    - Don't guess a command.
    - Hand back a script labeled **template, not run**.
    - Name the exact repo detail that must be filled in before the script can run.

    The missing item can be the package manager, the entry point, or the source of the changed dependency, as in a
    lockfile-only diff. The fact stops at the ladder step you actually reached (see
    [The evidence ladder](#the-evidence-ladder)).

6. **Fan out when the change is wide.** Wide means any one of these:

    - The diff touches three or more files.
    - The diff changes a public API or an exported signature.
    - The diff changes a shared data format such as a schema, a serialized payload, or a config contract.

    A single-function change inside one file is not wide. A fan-out on it wastes the readers.

    Two files is the borderline case. Fan out on two files only if one of the other two triggers also fires. For a
    two-file change, say which rule you applied, so the call is auditable rather than a matter of taste.

    State the count first, two or three readers, then dispatch them on the same question through this session's native
    dispatch path ([surface-dispatch.md](../../../routing/references/surface-dispatch.md)). Each dispatch carries the
    items in routing's [What every dispatch carries](../../../routing/SKILL.md#what-every-dispatch-carries): the reader
    is read-only and runs nothing, and it returns its risks in the shape of [What to hand back](#what-to-hand-back) or
    `NEEDS_CONTEXT: <what is missing>`. Keep each blind to the others' conclusions. Merge the answers per the consensus
    rules in [verdict-synthesis.md](../verdict-synthesis.md). A risk two blind readers raise independently is high
    signal. Independent readers catch different real risks. For a review-verdict pass on the same change, offer
    `/agentics:review interactive` instead.

## What to hand back

- **What it does.** What changed, including the part that isn't obvious.
- **The one fact it's safe because of.** State it. Say which ladder step you got it to. Show the proof. If you couldn't
  prove it, write **unproven**.
- **Risks.** Keep only the real ones. Each names how it breaks, the `file:line`, how likely, its severity from
  [severity.md](../../../routing/references/contracts/severity.md), and how to check. Paste the proof for the ones that
  matter.
- **Cleared.** What you checked and why it's fine.
- **Before you merge.** The cheapest test or repro that catches the real bug, including the script you wrote.

Cite real code throughout. A writeup that leaves the session goes through the
[outbound checks](../../../gh/references/posting-envelope.md#outbound-checks) first.

## Holding the line

The proof run is declinable. The **unproven** marker is not: it is a status claim, a
[safety gate](../../../routing/references/contracts/gates.md#safety-gate). If the user skips the step-4 script ("no
time, just tell me"), comply: mark the fact **unproven** and say which ladder step it reached.

A request to drop the marker, in any wording (relabel it proven, call the change "safe", drop the word as noise, soften
it to "probably fine", ask for the write-up without it), gets the hold once, in one shape:

- **Held:** the **unproven** marker on the fact.
- **Harm:** an unearned "safe" is what the reader merges on.
- **Instead:** run the cheap proof script, which usually takes under a minute, or merge knowing the fact is unproven.

A repeated request gets one line pointing back to that alternative.
