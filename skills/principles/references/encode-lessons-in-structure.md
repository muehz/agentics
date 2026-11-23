# Encode Lessons in Structure

_Apply when you notice that you write the same instruction a second time, or when you notice a recurring correction._

Encode recurring fixes in mechanisms (tools, code, metadata, automation) instead of textual instructions. Every error,
human correction, and unexpected outcome is a learning signal.

Textual instructions are easy to miss: they require the reader to notice, remember, and comply. Structural mechanisms
(lint rules, metadata flags, runtime checks, automation scripts) enforce the rule without cooperation.

When you notice that you write the same instruction a second time, do these steps:

1. Ask: can this be a lint rule, a metadata flag, a runtime check, or a script?
2. If yes, encode it. Delete the instruction.
3. If no (it genuinely requires judgment), make the instruction more prominent. Add an example of the failure mode.

When more than one mechanism would work, choose the strongest one that the situation allows. From strongest to weakest:

1. An unrepresentable state that cannot compile.
2. A lint or banned API that fails CI.
3. A canonical helper.
4. A runtime check.

Agents copy whatever the surrounding code already does, so a weaker guard becomes the next template.

When the fix is structural, keep only the structural fix. A leftover instruction is a symptom, not a fix.

Feedback loop:

- **Capture every correction.** When the human intervenes or a test fails, decide whether it is a one-off or a pattern.
- **Route to the right layer.** One-off → a working note. Recurring fix → a skill, reference file, or lint rule.
  Systemic issue → a principle.
- **Close the loop.** Do not just record the lesson. Apply it now, or create a concrete todo.

Anti-patterns:

- Acknowledging without recording ("I'll keep that in mind" does not persist)
- Recording without routing (a note about a lint rule that should exist is wasted unless the lint rule gets implemented)
- Fixing without generalizing (fixing one instance while leaving the recurring pattern intact)

## Self-test

- Have I written this instruction before? Search the repo for the same rule. Two or more hits means encode it.
- Can this rule fail a build: as a type that cannot compile, a lint rule, a CI check, or a script? If yes, more text is
  not the fix.
- After I encode the rule, did I delete the prose version? A rule that lives in two places drifts.
- Does the correction I just took from the human exist anywhere outside this conversation? If no, it is lost at session
  end.
