# Co-authoring workflow (build mode)

Full detail for the three-stage build workflow in the `docs` skill. Questions use the decision-question format in
[question-format.md](../../routing/references/contracts/question-format.md). Ask a question only when its answer changes
the document.

---

## Stage 1: context gathering

**Goal:** Close the gap between what the user knows and what you know so later guidance fits the work.

### Initial questions

Ask about the meta-context the request leaves open, in one round. Give each question a ➡️ recommendation drawn from the
request and the repo. Skip a topic the request already settles. For example, "I need a decision doc" settles the
document type.

- **Document type:** technical spec, decision doc, proposal, or another kind.
- **Audience:** who reads it and who approves it.
- **Desired impact:** what a reader should decide, do, or understand after reading.
- **Template:** a required template or format, or none.
- **Constraints:** deadlines, review gates, or context the draft must respect.

In the same message, tell the user they can answer in shorthand ("Q1: platform leads, Q2: none"), dump information in
any form, or end the round and take the recommended answers.

**If the user provides a template:** Read it. Then build the document on its structure.

**If you edit an existing document:** Read the current state first.

### Info dumping

Invite the user to dump all context, in any order and without organizing it first:

- Background on the project or problem
- Related team discussions or documents
- Why alternative solutions are not in use
- Organizational context (team dynamics, past incidents)
- Timeline pressures or constraints
- Technical architecture or dependencies
- Stakeholder concerns

### Clarifying questions

When the user signals that the dump is complete, ask about the gaps whose answer changes the draft, and only those. Ask
them as one round, each with a ➡️ recommendation. Let the user answer in shorthand:

- "Q1: yes, Q2: see the wiki page, Q3: no because backwards compat"

Ask a follow-up only when an answer is ambiguous. Do not open a fresh round. Any clarifying question or gap that the
user has not answered stays as an inline `[open: <what's missing>]` marker in the Stage 2 draft—do not fill it with an
assumed answer unless the user explicitly tells you to take the recommended answers (in which case the draft's review
message states which answers were assumed).

**Exit condition:** Context is sufficient when you can ask about edge cases and trade-offs, and the user does not need
to explain the basics.

**Transition:** In the same message as the questions, say that you start the draft when the user answers. The user can
add more context with those answers.

---

## Stage 2: refinement and structure

**Goal:** Draft the whole document from Stage 1 context. Review it once as a unit. Drill into sections only where the
user asks for deeper collaboration.

### Setup

If the structure is clear, use it. If it is unclear, propose 3–5 sections for the document type. Review the structure
with the full draft.

Create the initial document with all section headers.

### Step 1: draft every section

Draft each section directly from the Stage 1 context. When the context is too thin to support a claim, mark the gap
inline as `[open: <what's missing>]`. Do not invent content or pause to ask. Write each section as a targeted edit with
your surface's `str_replace` / `Edit` equivalent. Never reprint the file.

### Step 2: one full-draft review

Present the complete draft. Ask one review question: which sections need work, and what are the answers to the
`[open: …]` markers. Flag any section you drafted with low confidence and recommend it for drill-down. The user may edit
the file directly or tell you what to change. When they edit it, read it again before your next edit.

### Step 3: section drill-down (on request only)

Run this cycle only for sections the user names. It is the drill-down path, not the default. For each named section:

1. **Clarifying questions.** Ask only the questions whose answer changes what the section contains, as one round with a
   ➡️ recommendation on each. Skip this step when the context already answers them.
2. **Brainstorming.** Generate a numbered list of candidate points (5–10, scaled to section complexity). Look for
   context the user shared earlier that you or the user may have forgotten. Look for angles that nobody has mentioned
   yet.
3. **Curation.** Ask which points to keep, remove, or combine, as one question. In the ➡️ line, name the set you
   recommend to keep. Examples of answers are "Keep 1, 4, 7" and "Remove 3 (duplicates 1)". Freeform feedback also
   works. If the user rejects all options ("none of these are right"), ask the user to describe the section directly. Do
   not brainstorm again without guidance.
4. **Gap check.** List every `[open: …]` marker inside the section. Mark each marker as resolved or retained. For a
   resolved marker, give the answer from the drill-down that closes it. For a retained marker, name the missing answer.
   Ask the user for a retained answer only when the section cannot stand without it.
5. **Redraft.** Rewrite the section from the curated points. Before you present it, resolve every `[open: …]` marker
   inside the section from the drill-down. If an answer is still missing, re-flag the marker. Name the missing answer.
   Do not return an unexamined marker to the document-level review.

### Step 4: iterative refinement

Apply feedback with targeted edits. Never reprint the whole doc. Continue until the user is satisfied.

**Quality check:** After 3 consecutive iterations with no substantial changes, ask the user one question. Ask whether
you can remove anything and still keep all important information.

### Near completion (80%+ sections done)

Re-read the entire document. Then check for:

- Flow and consistency across sections
- Redundancy or contradictions
- Generic filler ("slop")
- Whether every sentence does work

Report what you found and fixed. Then say that Reader Testing runs next. The user can stop it; the skill's **Skipping a
stage** rule covers that.

---

## Stage 3: reader testing

**Goal:** Test the document with a fresh subagent that has no context from this conversation. Verify that the document
works for readers.

**Dispatch mechanism:** Dispatch one subagent for the whole stage through the session's native dispatch path (see
[surface-dispatch.md](../../routing/references/surface-dispatch.md)). Pass only the document content, the Step 1
questions, and the Step 3 checklist in the task prompt. A re-test after fixes is one more dispatch of the same shape.

**Fallback when no isolated dispatch is available.** Do not skip Stage 3. Do not answer the questions in the authoring
conversation as if a fresh reader had answered them. Run a **simulated fresh-read**. Answer each Step 1 question and
each Step 3 check from the document text alone. Label the result `Simulated fresh-read: no isolated subagent available`
in the Stage 3 report and wherever you cite the result. State that a simulated read is weaker, because the author cannot
fully forget their context. Never report it as testing by a fresh subagent.

### Step 1: predict reader questions

Generate 5–10 questions that real readers would ask when they first find this document.

### Step 2: test with the subagent

The subagent answers every question from the document content alone. It has no context from the authoring conversation.

In the Stage 3 report (whether from a fresh subagent or a `Simulated fresh-read`), explicitly summarize **both** what
the reader got **right** and what it got **wrong** (or could not answer) across the predicted questions.

### Step 3: additional checks

The same subagent checks for:

- Ambiguity or unclear passages
- False assumptions about reader knowledge
- Internal contradictions or inconsistencies

### Step 4: report and fix

If the tests find issues:

1. Report the specific problems
2. Loop back to Stage 2 refinement for problematic sections
3. Re-test after fixes

If the same category of issue recurs after one fix-and-retest cycle, stop the loop. Show the user the failing questions.
Ask as one question whether to revise or ship as-is. Use ➡️ revise when a failing question touches the desired impact.
Otherwise use ➡️ ship.

If the tests find no issues, proceed directly to Final Review.

**Exit condition:** The fresh reader consistently answers the questions correctly and finds no new gaps.

---

## Final review

When Reader Testing passes:

1. Recommend that the user do a final read-through. They own this document.
2. Suggest that they double-check facts, links, and technical details
3. Ask them to verify that the document achieves the desired impact

Final tips:

- Use appendices to provide depth and keep the main doc short
- Update the doc as real readers send feedback

---

## Unslop procedure and change report

Detailed step rules and reporting contract for [Unslop mode](../SKILL.md#unslop-mode-ai-tell-cleanup).

### Five-step pass

1. **Prepare.** Load [slop-patterns.md](slop-patterns.md). Mark protected spans with the
   [STE exclusions](ste-exclusions.md); supplied frontmatter is one protected block, delimiters included. For technical
   prose, load [Layer 3](technical-writing.md#layer-3-make-statements-load-one-at-a-time-ste) and record an action map:
   each action, its occurrences, and the one existing term you will use for it. Keep a working ledger of the original
   editable statements, each with its role (claim, instruction, recommendation, or other).
2. **Catalog pass.** Fix every actual catalog hit and note each edit in the ledger. A grammatical form alone is not a
   hit, and a plain verb that states the writer's claim is not filler. Leave text with no hit unchanged.
3. **Focused STE pass, technical prose only.** Split sequential actions into separate commands or numbered steps, with
   conditions first. Keep instructions to about 20 words and other technical sentences to about 25. Keep "should" in an
   introduction that names the actor and conditions for a group of steps, and write each step as a command. Use the
   mapped term for every occurrence of an action. Skip this pass for brand copy and personal notes; in mixed text, check
   only the technical parts. This pass does not enforce the STE dictionary and does not establish full compliance.
4. **Self-audit.** Run the catalog's self-audit, then compare the original and final text against every applicable
   pattern and Layer 3 row. Repair what remains.
5. **Deliver.** Return the revised text, or apply the edit, with a change report. A complete replacement keeps every
   protected span byte-exact, in its original place; when you fence it, use an outer fence longer than any inner one.

### The change report

Give **one separate entry per cited catalog pattern number and per cited Layer 3 STE rule number**—each naming the
verified pattern or rule number and its short rule text from [slop-patterns.md](slop-patterns.md) or
[technical-writing.md](technical-writing.md#layer-3-make-statements-load-one-at-a-time-ste), followed by its own
`Before:` and `After:` fenced text blocks at column zero quoting the original and the delivered text exactly. Never
bundle multiple pattern numbers or multiple STE rule numbers under a single shared `Before:`/`After:` pair. A fix counts
only when the problem is in the original and gone from the final text. Patterns found only in protected text are not
reported. Do not claim full STE compliance. If you skipped the STE pass, say so.
