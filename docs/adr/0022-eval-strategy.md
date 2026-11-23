# An eval bank asserts only what its runner observes, and changes with its skill

This record covers eval strategy: what the eval harness can and cannot observe, what a pinned bank may assert, when a
bank may require a skill to hold under pressure, and when a bank changes. The findings come from the skill-suite audit
in `docs/designs/skill-audit-2026-11/consolidated-issues.md`; the bank-by-bank evidence is in
`docs/designs/skill-audit-2026-11/2026-11-09-pass-3-raw-eval-banks.md`.

The banks were written as if a runner could create fixtures, run tests, commit, post, and choose its next user turn from
the skill's answer. The harness does none of these. Its default run is one read-only evaluator that plays both sides,
and its observed run replays fixed turns into fresh processes. The banks have also drifted from the skills: two banks
give opposite verdicts on the same request, some name skills that no longer exist, and several skills have no scenario
for their main job.

This record decides the rules for banks and for the harness. It does not decide what a skill does when a user pushes
back; that is record 0014. It does not decide which skill owns a request; that is records 0013, 0015 and 0016. A bank
follows those records.

## Rules

### A bank asserts only what its execution protocol observes

- **Answers:** C13.1
- **Rule:** Each scenario is written for one of the three execution protocols the harness has, and its pass and fail
  criteria name only evidence that protocol retains.
    - **What each protocol observes.** The role-play run (`evals run`, `evals cycle`) and the observed replay
      (`evals observe`, version-1 case) retain response text, and `evals cycle` also captures the output files a
      scenario declares in an `Artifacts:` list; standalone `evals run` stages and captures none (verified:
      evals/README.md:281, 2026-11-11). A criterion names what the skill says: the question it asks, the route it names,
      the refusal, the command it proposes, the artifact text it shows. Starting state, command output, and service
      responses are fixture facts in the scenario. File content is asserted only through a declared `Artifacts:` entry
      with its fixture under `evals/fixtures/`, in a scenario run by `evals cycle`. No protocol observes a commit, a
      test run, a push, a post, or any other external effect, so no criterion requires one to have happened; "the runner
      should create it with `git init`" and "must actually be posted" are rewritten as the statement or command the
      skill must produce. (verified: evals/README.md:347, 2026-11-11) (verified: evals/README.md:1210, 2026-11-11)
    - **User turns are fixed.** Every user turn is verbatim, in a fence, and is sent whatever the skill answered. A turn
      conditioned on the skill's answer ("sent only after the skill holds this line") is not allowed, because the
      observed run sends it anyway and the role-play evaluator already knows the expected answer. The criteria say what
      is correct at each turn for a skill that reached it either way. (verified: evals/README.md:349, 2026-11-11)
    - **Assertions cover the whole heading or none of it.** A `##` category either declares an `Assertions:` inventory
      with at least one assertion for every sub-scenario under it, or declares none. A partial inventory fails
      conformance, because the grader returns exactly the declared inventory and the other sub-scenarios go unasserted.
      (verified: evals/README.md:1216, 2026-11-11)
    - **The sub-scenario is the unit of regression.** A bank has four categories, each with one or more sub-scenarios.
      Every sub-scenario has its own assertion ids, and a report lists failed assertions by sub-scenario. A category's 0
      to 5 score stays the input to a stamp, and is never the only evidence that a sub-scenario still passes.
      `evals/skills/README.md` stops saying a bank contains four scenarios. (verified: evals/skills/README.md:3,
      2026-11-11)
    - **Conformance checks content, in both implementations.** The bank rule in `evals/src/rules/banks.ts` and its Go
      counterpart in `evals/go/internal/evals/rules/banks.go` change together. Beyond headings and fence counts, they
      fail a bank when a sub-scenario has no fenced user turn, when a sub-scenario states no expected behaviour, when
      two sub-scenario bodies are byte-identical, when a named skill is not in the roster, and when an inventory is
      partial. (verified: evals/src/rules/banks.ts:217, 2026-11-11)
- **Default / Rejected:** The default is to write banks down to what the three existing protocols observe; rejected:
  building an adaptive, stateful execution protocol first and keeping the criteria as written. `evals/README.md` says
  such cases "require a different reviewed execution protocol", and none exists. Until one does, a criterion that needs
  it can only be met by asserting an action that never happened. A later protocol may widen what a bank asserts; it adds
  to this list, and a scenario names the protocol it needs.
- **Later work changes:** `evals/README.md`, `evals/skills/README.md`, `evals/src/rules/banks.ts`,
  `evals/go/internal/evals/rules/banks.go`, `evals/fixtures/`, `skills/forge/references/evals/README.md`, and the 26
  scenario banks, each of which the new conformance checks apply to: `evals/skills/challenge.md`,
  `evals/skills/decompose.md`, `evals/skills/deepen.md`, `evals/skills/diagnose.md`, `evals/skills/docs.md`,
  `evals/skills/execute.md`, `evals/skills/explain.md`, `evals/skills/forge.md`, `evals/skills/gate.md`,
  `evals/skills/gh.md`, `evals/skills/git.md`, `evals/skills/handoff.md`, `evals/skills/hillclimb.md`,
  `evals/skills/intent.md`, `evals/skills/ops.md`, `evals/skills/principles.md`, `evals/skills/prototype.md`,
  `evals/skills/recall.md`, `evals/skills/review.md`, `evals/skills/routing.md`, `evals/skills/scaffold.md`,
  `evals/skills/spec.md`, `evals/skills/tdd.md`, `evals/skills/threat-model.md`, `evals/skills/triage.md`,
  `evals/skills/zealot.md`

### A bank is derived from its skill and the decision records, and two banks never disagree

- **Answers:** C13.2
- **Rule:** A bank states what its skill's current text and the decision records require, and nothing else decides a
  scenario's expected behaviour.
    - **Contradictions.** When two banks grade the same request, they give the same verdict. The verdict comes from the
      record that owns the question (0013 to 0021), never from whichever bank was written first. The fourteen
      contradictions the audit lists as B1 to B14 are the check list: each is closed in the pull request that changes
      the owning skill, and the final cross-bank check confirms none is left. This record decides none of the fourteen
      answers. (verified: evals/skills/gate.md:1170, 2026-11-11) (verified: evals/skills/zealot.md:705, 2026-11-11)
    - **Stale content.** A bank does not restate a table or list that a skill file owns. It gives the one value a
      scenario needs as a fixture fact and names the owning file. A bank anchors the judge by quoting the clause, never
      by line number. A bank names only skills and modes that exist. What the model-ladder copies must say is decided
      under C11.2 in record 0021. (verified: evals/skills/recall.md:63, 2026-11-11)
    - **Coverage.** A bank has at least one do-the-job scenario for the skill's first-listed job and for every mode in
      the skill's mode table. Where a mode has none, the bank header says so and gives the reason, as the handoff and
      recall banks already do. A mode that is missing without that header line fails the review of the pull request that
      changes the skill. Route-out and pressure scenarios do not count as coverage of a job. (verified:
      evals/skills/handoff.md:17, 2026-11-11)
- **Default / Rejected:** The default is that the skill text and the records are the source and the bank follows them;
  rejected: treating the bank as the specification and making the skill match it. The banks disagree with each other, so
  they cannot all be right, and a fix agent is not allowed to read them. Also rejected: one sweep that rewrites all 26
  banks ahead of the skill changes, because the rewritten banks would assert the behaviour the skill changes are about
  to remove.
- **Later work changes:** `evals/skills/README.md`, `evals/README.md`, and the 26 scenario banks, each of which gains
  the coverage statement in its header: `evals/skills/challenge.md`, `evals/skills/decompose.md`,
  `evals/skills/deepen.md`, `evals/skills/diagnose.md`, `evals/skills/docs.md`, `evals/skills/execute.md`,
  `evals/skills/explain.md`, `evals/skills/forge.md`, `evals/skills/gate.md`, `evals/skills/gh.md`,
  `evals/skills/git.md`, `evals/skills/handoff.md`, `evals/skills/hillclimb.md`, `evals/skills/intent.md`,
  `evals/skills/ops.md`, `evals/skills/principles.md`, `evals/skills/prototype.md`, `evals/skills/recall.md`,
  `evals/skills/review.md`, `evals/skills/routing.md`, `evals/skills/scaffold.md`, `evals/skills/spec.md`,
  `evals/skills/tdd.md`, `evals/skills/threat-model.md`, `evals/skills/triage.md`, `evals/skills/zealot.md`

### A skill's banks change in the same pull request as the skill

- **Answers:** C13.3
- **Rule:** A pull request that changes what a skill does also changes that skill's scenario bank and trigger set, or
  states in its description that neither asserts the changed behaviour.
    - **Scenario bank.** The author of the skill change rewrites the rows that assert the old behaviour, bumps the bank
      version, and adds the dated header line that resets comparability. The pull request's approval is the maintainer's
      approval of the bank change, so no separate issue is needed for it. The issue-first proposal in `evals/README.md`
      stays for a bank change that comes with no skill change. A bank stays frozen inside a baseline and its revision
      loop, and a fix agent still never reads it. (verified: evals/README.md:944, 2026-11-11) (verified:
      evals/skills/README.md:56, 2026-11-11)
    - **Trigger set.** A trigger positive is a request the skill's body accepts as its own job. A request that matches a
      row of the skill's `## Don't use when` table is a negative for that skill. When the description, that table, or
      the skill's jobs change, the trigger set is derived again from the new text and its version is bumped in the same
      pull request. Where a description promises a job the body routes away, that is a defect in the skill, settled
      under record 0015, and the trigger set follows the outcome. The current sets fail this rule: the tdd positives ask
      tdd to implement a change, which the tdd body sends to `routing`, and routing positives ask it to arbitrate
      between teams. (verified: evals/skills/tdd.trigger.json:7, 2026-11-11) (verified: skills/tdd/SKILL.md:26,
      2026-11-11) (verified: evals/skills/routing.trigger.json:18, 2026-11-11)
- **Default / Rejected:** The default is one pull request for the skill and its banks; rejected: changing the skill
  first and refreshing the bank later through a queued issue. That is the current practice. It left the handoff bank two
  skill versions behind and left trigger sets testing jobs the skills gave up. Every stamp in `evals/skills.yaml` is
  `not-run` today, so resetting comparability with each change loses no result.
- **Later work changes:** `evals/README.md`, `evals/skills/README.md`, `evals/skills/` (every scenario bank and every
  trigger set in it), `CONTRIBUTING.md`, `skills/forge/references/quality/trigger-eval.md`,
  `skills/forge/references/evals/README.md`

### Pressure follows the rule being pressed: a bank requires a hold only where 0014 holds the rule, and a yield everywhere else

- **Answers:** C3.3
- **Rule:** Conformance no longer requires a second pressure turn in every `adversarial` category. A scenario may
  require the skill to refuse or hold against the user's explicit instruction only when it quotes the clause being held
  and record 0014 classes that clause as one that holds. Record 0014's three classes give three cases. Pressure on a
  safety gate is tested as a hold. Pressure on a repository policy is tested as a hold with the setting stated as a
  fixture fact: the skill names the exit once and records no override, and when the fixture says the setting is off
  there is no hold. Pressure on a process gate is tested as a yield: the skill names the cost once, complies, and
  records the override. A skill with no holding rule has yield scenarios, or ambiguous and hostile input, in its
  `adversarial` category, and no hold scenario. The rubric's pressure criterion scores a correct yield as high as a
  correct hold. The `adversarial` heading stays required in every bank. No scenario requires a skill to refuse an
  instruction that record 0014 says the user may give. The scenarios the audit lists as pinning user-hostile behaviour
  are rewritten or removed in the pull request that changes the skill, according to what 0014 decides about the rule
  they press: no approval after "not yet", a refused extra spec review, a refused return of the user's own pull request
  to draft, and a refused exception that the skill's own sentence invites. The git bank's scenario that grades the skill
  on overriding the host's attribution instruction follows the attribution rule in
  [0019](0019-output-and-skill-structure.md), under which a commit carries no AI attribution, not 0014. Today the bank
  rule requires a second pressure turn of every bank, while the hardening checklist already says that safety gates hold
  and process gates yield. (verified: evals/src/rules/banks.ts:217,
  skills/forge/references/quality/skill-hardening-checklist.md:76,
  skills/forge/references/quality/skill-hardening-checklist.md:85, 2026-11-11)
- **Default / Rejected:** a hold is required only of a rule that 0014 holds, and every other pressed rule is tested as a
  yield; rejected: keeping a scripted second push in every bank that the skill must hold, with conformance still
  requiring it, because that requires a hold test of every skill, including skills that own no rule worth holding, and
  requires a yield test of none. A skill whose only gates are process gates would have to press a suite-wide safety gate
  it does not own, each softening under 0014 would fail that skill's bank until its scenario was edited, and the
  contract would stay at odds with the hardening checklist, where process gates yield. The uniform rule it offered, with
  every skill tested against an agent that gives way too easily, does not outweigh that. The chosen rule depends on
  0014, and that is the right order: the bank follows the pushback decision.
- **Later work changes:** `evals/skills/intent.md`, `evals/skills/spec.md`, `evals/skills/gh.md`, `evals/skills/git.md`,
  `evals/skills/tdd.md`, `evals/skills/` (the `adversarial` category of every bank), `evals/src/rules/banks.ts`,
  `evals/go/internal/evals/rules/banks.go`, `evals/src/packet/prompt.ts` and `evals/go/internal/evals/packet/build.go`
  (the rubric's pressure criterion)

## Existing records

- 0002: consistent — changing the bank rule in `evals/go` is a stated behaviour change with its own tests, which is what
  that record requires of a change that cannot be shown to be a pure reduction.
- 0008: consistent — this record changes no chain rule; banks that assert the approved-spec chain follow whatever
  records 0013 and 0014 decide about it.
- 0009: consistent — the small-fix signals are untouched; the bank scenarios that grade them follow record 0013.
- 0012: consistent — banks keep calling flow state through the hook binary, and that record's statement that affected
  skills remain `not-run` still holds.

## What this costs

**Banks stop testing that work was done.** Under the first rule a bank can no longer require a real commit, test run or
post. It checks that the skill says the right thing and proposes the right command. A skill that states the right
command and would run the wrong one passes. Only declared artifacts, with fixtures, test file content, and that path is
Codex-only for observed runs.

**Pressure scenarios get weaker as tests of resolve.** With fixed turns, the push arrives whether or not the skill held,
so a scenario cannot show that the skill held first and was then pushed. The criteria have to judge each turn on its
own.

**Every skill change costs a bank change.** The author of a skill change now reads and edits the bank and the trigger
set, and each edit resets comparability for the affected categories. Nothing is lost today, because no skill has a
current result, but once stamps exist a behaviour change will always invalidate one.

**The `adversarial` category of all 26 banks is reviewed.** Both conformance implementations and the rubric change with
it, and a bank cannot be rewritten until 0014 is signed off. A skill with no holding rule loses its hold test, so a
regression that makes it stubborn or evasive shows only in the yield scenarios.

**Conformance gets stricter in two places.** The TypeScript and Go bank rules must change together, and existing banks
fail the new checks until they are rewritten: the gate bank has two byte-identical sub-scenarios, four banks have
partial inventories, and two name skills that do not exist.

**Coverage is declared, not measured.** A header line that admits a missing mode is a statement by the author. Nothing
mechanical proves that a listed scenario exercises the mode it claims.

## Considered and declined

**Capping the number of sub-scenarios per bank.** Declined. The count is not the defect. The defect is that one score
hides a regression in one of sixty sub-scenarios, and reporting by sub-scenario fixes that without discarding coverage.

**Letting the bank own the answer to a cross-bank contradiction.** Declined. Each contradiction is a question about
approval, ownership or pushback that another record owns. Deciding it here would put two records in charge of one
question.

**Dropping the `adversarial` category.** Declined. Hostile, ambiguous and authority-laden input still needs scenarios;
the pressure rule above changes only whether the expected behaviour must always be to hold.
