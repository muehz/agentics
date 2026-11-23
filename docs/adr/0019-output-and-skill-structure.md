# A skill states each rule once, and prints only what is true and needed

This record covers the output contract and the structure of a skill: what a `SKILL.md` must and must not contain, the
words a user sees, announce and status lines, the question format, the rules for text that leaves the session (commits,
pull request bodies, posted comments), and AI attribution. The findings come from the skill-suite audit,
`docs/designs/skill-audit-2026-11/consolidated-issues.md`, rows C9.1, C9.4, C9.5, C9.6 and C13.5.

How many confirmations a workflow asks for (C9.2) is decided in [0014](0014-approval-and-pushback.md). This record
decides only the form a question takes once it is asked. Redaction and visibility of posted text are decided in the
trust-boundaries record, and whether process files are committed at all is decided in the flow-state record.

## Rules

### A SKILL.md states each rule, index and shared paragraph once

- **Answers:** C9.1
- **Rule:** A `SKILL.md` holds a routing section, the workflow, and one reference index under `## References`. It
  carries no second copy of that index and no "Capability summary" block that restates the body; an entry-point table
  may stay when it only names modes and links to their sections. A rule is stated once in the skill that owns it, and
  later mentions link to it. Text shared by several skills lives in one file and each skill links to it in one line:
  this applies to the decision-question paragraph (13 skills today), the principles check, the todo-list instruction and
  the empirical-fork paragraph. The principles check runs only in a phase that has a diff or artifact to inspect, so
  `intent` and `challenge` drop it. The todo list is filled once per run and each later phase adds its own steps instead
  of replacing the list; a skill adds them after its mode is known. A shell command that is the same in Bash and
  PowerShell is written once; a second block appears only where the syntax differs. (verified: skills/forge/SKILL.md:48
  and skills/forge/SKILL.md:357 print the same index; .github/tools/src/conformance/domains/skills/question-format.ts:42
  requires the pasted paragraph in 13 skills; skills/ops/references/shared-principles.md:237 requires identical shell
  pairs, 2026-11-11)
- **Default / Rejected:** One copy with links; rejected: keeping self-contained skills that paste shared text, because
  the copies have already drifted (the empirical-fork paragraph differs between `routing` and `intent`) and the
  duplicated index alone is about a quarter of `skills/forge/SKILL.md`. Also rejected: keeping identical shell pairs "to
  enforce the convention", because a pair that cannot differ tests nothing and doubles the text.
- **Later work changes:** `skills/challenge/SKILL.md`, `skills/decompose/SKILL.md`, `skills/deepen/SKILL.md`,
  `skills/diagnose/SKILL.md`, `skills/docs/SKILL.md`, `skills/execute/SKILL.md`, `skills/explain/SKILL.md`,
  `skills/forge/SKILL.md`, `skills/gate/SKILL.md`, `skills/gh/SKILL.md`, `skills/git/SKILL.md`,
  `skills/handoff/SKILL.md`, `skills/hillclimb/SKILL.md`, `skills/intent/SKILL.md`, `skills/ops/SKILL.md`,
  `skills/principles/SKILL.md`, `skills/recall/SKILL.md`, `skills/review/SKILL.md`, `skills/routing/SKILL.md`,
  `skills/scaffold/SKILL.md`, `skills/spec/SKILL.md`, `skills/tdd/SKILL.md`, `skills/threat-model/SKILL.md`,
  `skills/triage/SKILL.md`, `skills/routing/references/question-format.md` (new),
  `skills/forge/references/forging/forging-skills.md`, `skills/forge/references/quality/skill-compression-playbook.md`,
  `skills/ops/references/shared-principles.md`, `.github/tools/src/conformance/domains/skills/question-format.ts`,
  `.github/tools/src/conformance/domains/skills/`

### Forge states one authoring contract and conformance checks all of it

- **Answers:** C13.5
- **Rule:** Every structural rule that forge gives authors is either checked by conformance or labelled as a target, and
  no two forge files give different values for the same rule. Conformance checks every frontmatter key in forge's
  contract (`name`, `description`, `version`, `argument-hint`, `allowed-tools`, `context`, `metadata.surfaces`):
  `metadata.surfaces` against the surfaces documented in `docs/surfaces/`, and `argument-hint` against the modes and
  flags the body accepts. The size limit is one number, 500 lines per `SKILL.md`, checked by conformance, first as a
  warning and then as an error once the skill sweeps have landed; the per-tier word budgets are removed. **Superseded by
  [ADR 0025](0025-skill-files-are-capped-in-bytes.md) (2026-11-20):** the size limit is now 40,000 bytes per file under
  `skills/`, checked by `skill-file-size`, and `skill-line-limit` is retired. The 25-word description figure stays a
  compression target and is not a limit. Forge stops telling authors to keep eval keys in frontmatter, because
  conformance rejects them there. `skills/forge/SKILL.md` owns scenario counts and gates, and its references cite those
  values instead of restating them. Test-first authoring is a default that the user may override, as
  `skills/forge/references/quality/advanced-patterns.md` already says, and the forge workflow describes the order it
  actually follows. (verified: .github/tools/src/conformance/domains/skills/frontmatter.ts:163 checks three keys;
  .github/tools/src/conformance/domains/skills/frontmatter.ts:299 rejects eval keys that
  skills/forge/references/quality/best-practices.md:13 and
  skills/forge/references/quality/skill-compression-playbook.md:38 tell authors to keep;
  skills/forge/references/quality/cso-guide.md:21 budgets 300 to 500 words and
  skills/forge/references/forging/forging-skills.md:164 budgets 500 lines, 2026-11-11)
- **Default / Rejected:** One line limit that conformance enforces (**superseded by
  [ADR 0025](0025-skill-files-are-capped-in-bytes.md) (2026-11-20):** one byte limit replaces it); rejected: the word
  budgets, because the skills they name are ten times over them and forge's other rules require the sections that make
  them unreachable. Rejected: enforcing 25 words on descriptions, because every current description exceeds it and the
  descriptions carry the trigger words routing depends on. Rejected: leaving the extra frontmatter keys unchecked,
  because they decide which tools and surfaces a skill gets and are already wrong in places.
- **Later work changes:** `.github/tools/src/conformance/domains/skills/frontmatter.ts`, `skills/forge/SKILL.md`,
  `skills/forge/references/quality/cso-guide.md`, `skills/forge/references/quality/best-practices.md`,
  `skills/forge/references/quality/skill-compression-playbook.md`, `skills/forge/references/forging/forging-skills.md`,
  `skills/forge/references/evals/`, `skills/forge/references/governance/compliance-governance.md`,
  `skills/decompose/SKILL.md`

### Mandated output is true when printed, uses the user's words, and appears once

- **Answers:** C9.4
- **Rule:** A skill may require a line or field only when the skill names where its value comes from at the moment it
  prints. A field whose value is unknown, cannot be measured, or can only be zero at that point is left out, not printed
  as `0`, `unknown` or `n/a`. A forecast is labelled as an estimate, and a named test or file must exist. An announce
  line is printed at most once per invocation, after the mode is known, and says what the skill is about to do in this
  run; no fixed sentence that is false for some modes. A line that offers an override is printed only when the skill
  stops for the answer; otherwise it says what was chosen. User-visible text uses terms from one new shared vocabulary
  file, `skills/routing/references/vocabulary.md`, and an internal term (a tier, a row kind, a step number) appears only
  with its plain meaning beside it. A document is shown in chat once at most: when it is also written to a file, chat
  carries its summary and path, and the full text on request. Text that [0018](0018-trust-boundaries.md) requires the
  user to see before a write, the exact text of an instruction-file or skill change, is the exception and is shown in
  full. For an approval, showing the artifact's path and hash beside the summary counts as showing the artifact, and
  that hash is the one the approval row in [0017](0017-flow-state-v2.md) binds. Empty sections are omitted, and output
  templates carry no sentences that instruct or warn the reader about the process. The `Ask:` line is not written into
  new `intent.md` or `plan.md` files; the approval question is asked in chat. Files already approved keep the line,
  because removing it would change their hash. (verified: skills/execute/SKILL.md:18 announces "to implement this plan"
  in every mode; skills/intent/SKILL.md:252 and skills/decompose/SKILL.md:665 mandate the approval status lines;
  skills/decompose/references/skim-rules.md:46 keeps `Ask` as static text in the file; skills/execute/SKILL.md:173
  prints the provider binding "or `unknown`", 2026-11-11)
- **Default / Rejected:** Omit what cannot be filled and print the rest once; rejected: keeping fixed-shape lines for
  the sake of a uniform report, because a line that is always `0` or `unknown` teaches the reader to skip the lines that
  matter. Rejected: rendering the full document in chat and writing it, because the user reads the same text twice and
  the approval binds the file, not the chat copy. Rejected: rewriting approved artifacts to remove `Ask:`, because that
  would stale every existing approval.
- **Later work changes:** `skills/routing/references/vocabulary.md` (new), `skills/gate/SKILL.md`,
  `skills/spec/SKILL.md`, `skills/execute/references/arena-mode.md`, `skills/execute/SKILL.md`,
  `skills/intent/SKILL.md`, `skills/decompose/SKILL.md`, `skills/decompose/references/skim-rules.md`,
  `skills/spec/references/draft.md`, `skills/gate/references/auto-detect.md`,
  `skills/gate/references/branch/branch-workflow.md`, `skills/routing/references/routing-reference.md`

### The question format is one rule in one file, and one question holds one decision

- **Answers:** C9.6
- **Rule:** The decision-question format and the way answers are collected live together in one new shared reference
  file, `skills/routing/references/question-format.md`, that can be followed without opening
  `skills/challenge/SKILL.md`. Skills link to it, and conformance checks the link instead of a pasted paragraph. The
  file names what it does not govern: one-line clarifiers, fixed yes/no confirmations, and approval questions whose
  wording another rule fixes; a fixed question is never reshaped into the format. One question holds one decision; a
  bundle is split, and a question that covers two artifacts or two actions offers an answer for each. Every round,
  including a round of one question, is posted in the visible chat first as one message before any ask-tool call, with
  the whole round in one blockquote so it renders formatted and copies as one block; a fenced code block is not used,
  because rendered formatting takes priority over copying the raw markdown. The user must be able to read the questions
  after the dialog closes and copy them for someone else to answer. A host that shows text written before a tool call
  only as a summary posts the round through its send-to-user tool, or, without one, ends the turn with the round and
  collects the answers in chat. The ask tool then only collects answers, with short questions that refer to the posted
  round by number. A single "accept all" choice is worded as "use the recommended answer for Q1 to Qn", is never used
  for findings, and is not offered when any recommended answer would publish, post, or halt the work; those answers are
  given by name. (verified: skills/challenge/SKILL.md:194 holds the ask-tool procedure with the `Accept all` option, and
  skills/gate/SKILL.md:145 points to it as "shaped per challenge Step 3", 2026-11-11)
- **Default / Rejected:** One shared file that skills link to; rejected: keeping the canonical text inside `challenge`
  with a pasted pointer elsewhere, because a reader of `gate` or `ops` must open another skill to learn how to ask.
  Rejected: one ask-tool call that only collects "accept all" or "discuss", because for gate findings and for defaults
  that publish it either cannot express the answer or accepts more than the user meant. Rejected: an ask dialog alone
  that carries each question with its own choices, because the questions are gone once the dialog closes and cannot be
  copied for someone else to answer.
- **Later work changes:** `skills/routing/references/question-format.md` (new), `skills/challenge/SKILL.md`,
  `.github/tools/src/conformance/domains/skills/question-format.ts`, `skills/gate/SKILL.md`, `skills/ops/SKILL.md`,
  `skills/decompose/SKILL.md`

### Posted comments, replies and reviews carry one AI footer, and commits, pull request titles and bodies carry none

- **Answers:** C9.5
- **Rule:** Comments disclose, history does not. Every comment, reply and review that Agentics posts ends with one
  footer, defined once in `skills/gh/references/posting-envelope.md`. The footer is a repository policy in the sense of
  [0014](0014-approval-and-pushback.md): it is on by default, it is switched off per repository in `.triage.yml`, by the
  existing key renamed for the wider scope, and the settings change is its exit. Commits, pull request titles and pull
  request bodies carry no AI attribution, because the user is accountable for them. A posted comment that states a
  maintainer's decision is worded as a recommendation unless the user made the decision in this session, because the
  footer marks it as machine-written. Text that leaves the session is written for a reader who has never seen Agentics.
  A pull request body describes the change and how it was tested, and carries no workflow status lines, signal lists or
  machine-only fields. A commit message or pull request body paraphrases a user's instruction and never quotes the
  user's chat words. A commit subject describes the change to the repository, not the workflow step that produced it. A
  posted comment @-mentions a person only when it needs something from that person, once, and never tells someone
  outside the session to run an Agentics command. A posted comment makes no statement about tools that did not run and
  carries no internal counts. The posting envelope's "no emojis" rule holds for every posted body, including the triage
  rollup. Today a commit must never carry an AI trailer, every triage comment must end with
  `> _Triaged by an AI agent_`, and a reply posted through review feedback carries no footer, so the same reply to the
  same reviewer is labelled or not depending on which skill posted it. (verified: skills/git/SKILL.md:129,
  skills/triage/SKILL.md:129, skills/review/references/feedback/receiving-code-review.md:174, 2026-11-11)
- **Default / Rejected:** one footer on every posted comment, reply and review, on by default and switchable per
  repository, and no attribution on commits, pull request titles or pull request bodies; rejected: no attribution
  anywhere by default, with the triage footer removed and a footer a repository may turn on, because a reviewer or an
  outside contributor could then not tell that a reply, or a "we will not fix this" on an issue, was written by an
  agent, and repositories whose policy requires disclosure would have to opt in one by one. The chosen rule gives one
  test that covers every case: text Agentics posts as conversation is labelled, and work the user owns is not. It keeps
  the commit rule the git skill already enforces, keeps the per-repository switch, and removes the only real
  inconsistency, which is between triage replies and review-feedback replies.
- **Later work changes:** `skills/gh/references/posting-envelope.md`, `skills/triage/SKILL.md`,
  `skills/triage/references/rollup-template.md`, `skills/routing/references/user-authorized-divergence.md`,
  `skills/gate/references/branch/branch-workflow.md`, `skills/review/references/feedback/receiving-code-review.md`,
  `.github/tools/src/conformance/domains/skills/attribution.ts`,
  `.github/tools/src/conformance/domains/skills/triage.ts`, `.github/tools/src/conformance/domains/skills/rollup.ts`

## Existing records

- 0002: consistent — it governs reductions in ported code, not the removal of duplicated skill text decided here.
- 0005: consistent — publication stays a manual act; these rules change the wording of posted text, not who publishes.
- 0008: consistent — the approval question it adds stays, and only the stored `Ask:` line and unfillable status fields
  change.
- 0009: consistent — the small-fix path stays; its pull request body and announcement follow the public-text rules.
- 0012: consistent — the `flow unavailable: <reason>` line it mandates has a named source and prints once.

## What this costs

**Every skill is edited.** Removing the second index, the summary block and the pasted paragraphs touches all `SKILL.md`
files, and each edit invalidates that skill's eval stamp.

**A skill is no longer readable alone.** The question format, the principles check and the vocabulary sit in shared
files, so an agent loads one more file to follow them. A broken link now breaks a rule in every skill that used it.

**Conformance gets stricter before the skills comply.** The line limit and the new frontmatter checks fail for several
skills today (`skills/decompose/SKILL.md` is 778 lines). They start as warnings, and someone has to promote them.
**Superseded by [ADR 0025](0025-skill-files-are-capped-in-bytes.md) (2026-11-20)** for the line limit: it is retired,
and the byte limit that replaces it carries an allowlist for the files already over it.

**Less is printed.** Omitting unfillable fields means two runs of the same skill can print reports of different shapes.
A user who wants the full document in chat has to ask for it.

**Old and new artifacts differ.** Approved `intent.md` and `plan.md` files keep their `Ask:` line; new ones have none.

**Review-feedback replies gain a footer.** They carry none today. The footer rule moves from triage to the posting
envelope, with its conformance check, and the `.triage.yml` key is renamed. A posted comment still reads as
machine-written when it states a maintainer's decision, so its wording has to be a recommendation unless the user made
the decision in the session.

## Considered and declined

**Generating the duplicated blocks from one source at build time.** Declined. The copies would stay in every loaded
skill and cost the same context; only the drift would go away.

**A per-skill size budget based on how often the skill loads.** Declined. Load frequency is not measured anywhere, so
the tier a skill belongs to would be an opinion that conformance cannot check.

**Keeping a status line in the artifact so the file shows its approval state.** Declined. The approval is a row in flow
state, and a line in the file that changes with the state would stale the approval it reports.
