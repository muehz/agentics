# Build the Lever

_Apply to any non-trivial work, not just bulk work: edits, migrations, analyses, checks._

When the work is not trivial, build the tool that does it. Do not do it by hand.

A codemod, generator, or script does the work the same way every time and reruns for free. It gives the reviewer an
artifact to read and rerun. You can re-verify a hand-done change only if you redo it. A script turns "trust me" into
"run this".

Build the lever by default. Skip it only when the work is trivial. An edit is trivial when it passes the small-fix test
that `/agentics:routing` applies. The lever is itself a new non-test file, and a new non-test file fails that test's
size limit, so requiring a lever on such an edit would take it off the small-fix path. Work with no diff, such as an
analysis or a check, is trivial when no lever is needed to make it checkable. Do not cite this principle for trivial
work.

- Do the first unit by hand to learn the recipe. Then build the tool. Rerun the tool on that unit. Diff the result
  against your hand-done version, so the tool is safe when a reviewer reruns it.
- Use a codemod or script for edits and a generator for repetitive files. Use a dump-to-sqlite query for analysis and a
  rerunnable check for verification.
- A deterministic lever beats fan-out. If the tool can process every unit in one pass, run it yourself. Do not fan out
  delegates to hand-apply what a script can do.
- When you fan work out to subagents, write the lever as one shared reference file that they all read. Put the recipe,
  the verification contract, and the do-not-touch fences in that one artifact. Every delegate inherits the same hardened
  version, so you do not re-explain it per prompt and no delegate drifts. Keep the file outside the delegates' write
  scope, so they cannot quietly edit the contract.
- This principle produces a file. If you cited it and the diff has no codemod, script, generator, or delegate contract,
  you did not apply it.
- When the work outlives the session, commit the lever. The next run then reruns it and does not redo the work.

**Balance:** The bar is triviality, not repetition. A one-off still earns a lever when the lever is what makes the work
checkable. Per the [Laziness Protocol](laziness-protocol.md), build the smallest script that does or proves the job,
never a framework.

This principle differs from [Encode Lessons in Structure](encode-lessons-in-structure.md), which makes a recurring
instruction a durable guardrail. This one covers throughput and reviewability on the work in front of you. To script the
verification itself, see [Prove It Works](prove-it-works.md).

## Self-test

- Does the change pass the small-fix test? If yes, the lever is not required. If no, it is. For work with no diff: does
  the result need a rerunnable tool before a reviewer can check it? If yes, build it.
- If the lever is required, does the diff contain it: a script, codemod, generator, or shared delegate contract file? If
  no, the principle was not applied.
- Run the lever twice against the same input. Does the second run produce the same result and leave the tree unchanged?
  If no, it is not safe to rerun.
- Will this work outlive the session? If yes, is the lever committed?
