# Technical writing: the four layers

The goal is prose that a tired engineer understands on the first read. Four layers meet this goal. Each layer asks one
question:

- Diátaxis: what kind of document is this?
- Google developer style: how do sentences address the reader?
- STE: how much does each sentence carry?
- Global English: can a reader read any sentence in two ways?

Apply all four layers to any doc that this skill produces or maintains.

Three rules sit above the layers:

- **Cut every word that does no work.** If the sentence survives without a word, cut the word. "In order to" is "to".
  "It is important to note that" is nothing.
- **Use the short, everyday word.** "Use", not "utilize". "Help", not "facilitate". Use a long word only when it is more
  precise.
- **When a rule makes a sentence worse, fix it another way or leave it alone.** The rules serve the reader. A sentence
  that follows every rule and sounds machine-written has failed.

**Precedence in maintenance.** The layers' clarity rules apply to every edit. They cover ambiguity, one thought per
sentence, and condition before instruction. Style preferences apply only to new prose. They cover rhythm and mixed
sentence lengths. In Maintain mode, the repo's established style wins. An accuracy edit never restyles sentences that it
did not touch. Em dashes are the exception, in the other direction. The punctuation rule below governs them.

**Punctuation precedence.** This is the one rule for em dashes in every docs reference. Apply the first row that
matches:

| Where the dash is                                          | What happens to it                                                                         |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| A protected span in [ste-exclusions.md](ste-exclusions.md) | It stays, byte-exact.                                                                      |
| New prose, in any mode                                     | Never write it.                                                                            |
| Any text in Unslop mode                                    | It goes. Stripping tells is that mode's job, and a repo's house style does not protect it. |
| A sentence that a Build or Maintain edit rewrites          | It goes.                                                                                   |
| A sentence that a Maintain edit does not touch             | It stays. An accuracy edit never restyles it.                                              |

To remove a dash, end the sentence or use a comma. Do not swap in an en dash, a hyphen used as a dash, or parentheses
that only stand in for the dash. Parentheses remain fine for text that is a full grammatical unit (Layer 4).

Use the codebase as the word list. Write the real symbol, file, flag, or command name, not a synonym or a description of
it. Do not invent jargon. Use the words a developer would say out loud, such as "move" and "delete". When a named
pattern first appears, say what it means.

**Vary the rhythm.** A doc can obey every layer and still read as machine-written. This happens when every sentence is
clipped short and nothing is specific. Mix sentence lengths on purpose within the Layer 3 limits. Short sentences stress
a point. Longer sentences carry a fact with its condition or consequence. One thought per sentence does not mean one
length per sentence. Be specific instead of sterile. Write "a column rename fails the build" rather than "schema changes
can cause issues".

## Layer 1: pick the mode first (Diátaxis)

The skill body's Diátaxis table owns the four definitions. This layer explains how to write inside each mode. Choose one
mode for each document. Ask two questions: does the content inform action or understanding, and does it serve learning
or work? When you are unsure, use those questions for a whole document or for one sentence. Intuition is often wrong
here.

- **Tutorial.** The goal is that the learner learns by doing. Open with what the learner will build, not what they will
  "learn". Every step produces a visible result. The doc says what the learner should see, such as expected output, a
  prompt change, or a log line. Cut explanation to one clause and a link. Use "we" as the voice. Write in commands.
- **How-to.** Solve a problem that a person has. Assume competence. Skip the teaching. Link to background and do not
  include it. Allow forks and judgment: "If you want x, do y." Name the guide by the task, not the topic. Example: "How
  to calibrate the radar array".
- **Reference.** Describe facts for lookup. Do not add instruction, opinion, or hedges. Mirror the structure of the
  thing you describe, so that code and docs navigate together. Where possible, generate the reference from code, so that
  it stays true.
- **Explanation.** Cover one bounded topic that readers can understand away from the product. Give context about design
  decisions, history, constraints, and alternatives. You may give opinion here and nowhere else.

Do not mix modes. Do not put reference tables inside a tutorial. Do not put tutorial hand-holding inside reference. Do
not argue inside a how-to. Split and link instead.

Source: diataxis.fr.

## Layer 2: write sentences to the reader (Google developer style)

- Talk to the reader as "you", in the present tense. Use "will" only for things that happen later.
- Say who does what: "the compiler checks", not "is checked". Use the passive only when the actor is unknown or
  irrelevant.
- Write instructions as commands, for example "Click Submit." Never write "should be done".
- Put the condition before the instruction: "To delete the document, click Delete." The reader skips what does not
  apply.
- Put the common case first. Put exceptions after it.
- Use no buzzwords, figurative language, or "please" in instructions. Never use "simply", "easy", or "quickly" in a
  procedure. If it were simple, the reader would not be here.
- Link with words that say where the link goes, such as the page title or a short description. Never use "click here".
- Headings carry the point, not just the topic. For example, write "Pick the mode first", not "Modes". Use sentence
  case. A task heading is a bare verb phrase. A concept heading is a noun phrase.
- Use numbered lists for sequences and bullets for everything else. Introduce a list with a complete sentence. Keep
  items parallel.
- Put code in code font. Put UI elements in bold. Use serial commas. Drop "etc." Say up front when a list is partial.

Source: developers.google.com/style.

## Layer 3: make statements load one at a time (STE)

Apply the shared [STE exclusions](ste-exclusions.md) before these checks.

Use these checks for the technical pass. When Unslop reports an edit, compare it against every row. Then include every
applicable fixed rule. An edit can fix several rules. If you name the catalog pattern of an edit, you must still report
its STE mappings.

| Check                                                                                                                                  | Rule to report when the edit fixes it                       |
| -------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| Separate multiple instructions into individual sentences or steps.                                                                     | 5.2                                                         |
| Separate multiple descriptive thoughts.                                                                                                | 4.1 and its explanation                                     |
| Shorten or split an instruction that is longer than about 20 words, so that each instruction is within that guidance.                  | 5.1                                                         |
| Reduce other technical sentences that are longer than about 25 words to sentences within that guidance.                                | 6.3                                                         |
| Move an applicable condition before its instruction.                                                                                   | 5.4                                                         |
| Open a safety instruction with a clear command or condition.                                                                           | 7.2; placement before the related step is Agentics guidance |
| Supply an article needed for clarity, such as "Remove the backup file".                                                                | 4.5                                                         |
| Replace alternate terms for the same action with one consistent term. For example, use "start" where another sentence used "initiate". | 9.4                                                         |
| Replace passive procedural wording with active wording that retains the actor.                                                         | 3.6                                                         |
| Express a procedure as direct commands.                                                                                                | 5.3                                                         |
| Correct an "-ing" verb construction, including a progressive verb. Keep permitted technical nouns and their modifiers.                 | 3.5                                                         |

Grammatical inflections such as "start", "starts", and "starting" share a lemma, so check every "-ing" verb form
separately under 3.5 and normalize every editable occurrence of the action across the passage while preserving the
sentence's existing actor—never replace a named actor with "you" (for example, change both "initiates the next batch"
and "before starting the batch" to active forms using `start`, such as "start the next batch" and "before the operator
starts the batch"). When a split fixes both action count and instruction length, report both 5.2 and 5.1. When an edit
rewrites a passive procedure as commands, report both 3.6 and 5.3. When that same edit also normalizes action terms,
include 9.4. Claim only a rule whose problem was present and whose correction appears in the delivered text.

When you split recommended actions, preserve "should" in an introduction that scopes the whole numbered group. Keep the
original actor and conditions that govern the whole group in that introduction. Keep an action-specific condition with
its step, before the action. Write each step as a direct command, so the recommendation and each condition's original
scope survive. The split does not turn the recommendation into a requirement.

For this example, the inspection condition applies to the whole procedure. The rewrite must keep the technician, the
condition, the recommendation, and the action order. The change report covers actual fixes from both the catalog and STE
passes.

Delivered revision:

```text
If the sensor reports an offset during a scheduled inspection, the technician should do these steps:

1. Record the offset from the sensor.
2. Adjust the calibration screw.
3. Confirm the new reading on the control panel.
```

Change report:

Fixed catalog pattern: **Shorten or split dense sentences** (28).

Fixed STE rules: **3.5, 5.1, 5.2, 5.3, and 5.4**. The original recommended procedure is 35 words. The replacement
introduction is 16 words. The three commands are 6, 4, and 8 words. The rewrite replaces the progressive verb and the
subordinate "-ing" actions with direct commands. It separates all three instructions. It moves the inspection condition
from the end to the group introduction. The words "before" and "then" become the same ordered sequence of numbered
steps. The technician and "should" scope every step.

Before:

```text
The technician should be recording the offset from the sensor before adjusting the calibration screw and then confirming the new reading on the control panel if the sensor reports an offset during a scheduled inspection.
```

After:

```text
If the sensor reports an offset during a scheduled inspection, the technician should do these steps:

1. Record the offset from the sensor.
2. Adjust the calibration screw.
3. Confirm the new reading on the control panel.
```

The focused STE check applies to this technical prose. The report claims no other catalog patterns or STE rules as
fixes. This example illustrates Agentics's meaning-preservation rule. It does not check dictionary approval.

Source: [ASD-STE100, Issue 9 (2025-01-15)](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf). These checks
adapt its sentence discipline. The standard sets 20- and 25-word maxima. This Agentics adaptation retains approximate
guidance. Rules 1.2 and 1.3 restrict the grammatical roles and meanings of approved words. This focused pass does not
check dictionary approval or establish full STE compliance.

## Layer 4: leave no sentence open to two readings (Global English)

- Keep words like "only" and "not" next to the word that they change. The phrases "only fails on growth" and "fails only
  on growth" say different things.
- Break up long noun strings. For example, "the proto import budget check script" becomes "the script that checks the
  proto-import budget".
- Make every "it", "they", and "this" point at one obvious thing. When in doubt, repeat the noun. Never use "this" or
  "which" to point at a whole clause.
- Do not drop verbs. The sentence "Phase 1 moves the converters and Phase 2 the runtime" leaves Phase 2 without a verb.
- Keep the small words that show structure. For example, write "Ensure **that** the switch is off". Never trade clarity
  for word count.
- When a sentence can group in two ways, say which parts "and" or "or" joins. Use "both…and", "either…or", or "if…then"
  to disambiguate.
- Prefer periods over semicolons. Do not write em dashes. Em dashes read two ways, and they are an AI tell. Pattern 13
  in [slop-patterns.md](slop-patterns.md) covers this tell, and the punctuation precedence rule above says which
  existing dashes stay.
- Make parenthetical text a full grammatical unit or its own sentence. Never form plurals with "(s)". Do not use
  slashes. Write "a, b, or both", not "a/b".
- Call each thing by one name, everywhere. A doc that says "the gate", "the ratchet", and "the budget check" for one
  thing teaches three things.
- Skip idioms, colloquialisms, Latin abbreviations, and metaphors. Non-native readers, translators, and agents parse
  plain constructions best.

Source: Kohl, _The Global English Style Guide_ (SAS Press).

## Worked example

Before:

> Configuration of the proto import ratchet budget script parameters is performed via budget.json. Note that it's
> important to remember that running with --write, which updates the committed budget to reflect the current count,
> should only be done when lowering it. If exceeded, CI fails.

After:

> `budget.mjs` reads the committed budget from `budget.json` and counts the files that import protos. If the count
> exceeds the budget, CI fails.
>
> Only when you lower the budget, you should do this step:
>
> 1. Run `budget.mjs --write`.

The fixes, by layer: "configuration is performed" becomes "`budget.mjs` reads", so the actor is clear (Google).
"Ratchet" goes away because the script's real filename names the mechanism. The five-noun string breaks into plain
clauses (Global English). The edit deletes the hedge "note that it's important to remember" because it adds no meaning.
The failure condition moves ahead of the step it explains (STE). The buried "should only be done when lowering" becomes
a recommendation that introduces a direct command. The introduction preserves "should" and the condition on when the
step applies. The words "when you lower" replace the "-ing" verb construction (3.5). "If exceeded" gets a subject: the
count (Global English).

## Review checklist

Apply this checklist to any prose that this skill covers. Item 1 applies only to document sets:

1. Is each file one Diátaxis mode, with links where modes meet?
2. Is every instruction a command, with its condition in front?
3. Does any sentence carry two instructions or two thoughts? Split it.
4. Can you cut any word and keep the meaning? Cut it.
5. Is "only" next to the word it changes? Does every "it" point at one thing? Does every clause keep its verb?
6. Does each thing have exactly one name across the docs?
7. Would a developer say these words out loud? Replace invented metaphors and fancy synonyms with the plain word or the
   real symbol name.
8. Are all symbols, paths, and counts real at this commit, with the commands that regenerate the counts?
