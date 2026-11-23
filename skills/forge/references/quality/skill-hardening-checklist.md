# Skill hardening checklist

The two axes that skills most often fail. The 2026-07-05 library fleet eval scored 31 skills across six criteria. Two of
them were systemically weak:

- **Workflow completeness**: library mean 3.05, the weakest criterion for 20 of 31 skills.
- **Pressure resilience**: 3.23, the weakest for 10.

The other four criteria all averaged ≥3.88. So when a skill scores `needs-revision`, the fix is almost always on one of
these two axes.

Apply this checklist **conditionally**, not as blanket boilerplate:

- **Workflow-completeness items** apply to any skill with a multi-step workflow.
- **Pressure-resilience items** apply to any skill that takes ambiguous, adversarial, or authority-laden input (most
  user-facing skills). They do not apply to pure reference or lookup skills.

A skill with no routing neighbors skips the deferral items; a stateless formatter skips the pressure items. Use
judgment: close the real gap. Do not add ceremony for a score.

After you apply the checklist, re-run `/agentics:forge eval <skill>`. Report the result. Keep the evaluation record
apart from the skill or agent file; no eval key sits in frontmatter. A `passing` status comes only from a re-eval that
clears the bar.

## Introduction gate: every new or adopted asset ships lean

Before a new or adopted skill or agent first reports `passing`:

- [ ] **Token-efficiency pass on the body.** Apply [skill-compression-playbook.md](skill-compression-playbook.md) to the
      SKILL.md or charter. Its targets are one statement per rule, one example per pattern, one encoding per fact, and a
      description with every trigger keyword. About 25 words is a target for the description and not a gate item; the
      checked limit is 1,024 characters (`skill-description-length`, in
      [SKILL.md structural rules](../forging/forging-skills.md#skillmd-structural-rules)).
- [ ] **Reference disposition.** If the asset ships `references/`, classify every file per
      [reference-cleanup-playbook.md](reference-cleanup-playbook.md) (keep-verbatim / trim-filler / dedupe / delete +
      heat). A result of "nothing to cut" is valid. The point is to record the check.
- [ ] **Pinned bank + baseline eval.** Add a pinned scenario bank where
      [a bank lives](../evals/README.md#where-a-bank-and-a-trigger-set-live). Report a pass only from an actual run.

The 2026-07 campaigns swept the already-shipped assets once. This gate stops the return of bloat, one asset at a time.

---

## A. Workflow completeness: close the gaps that force guessing

- [ ] **Preflight gate (Step 0).** Before the main workflow, confirm the input actually fits this skill. If it matches a
      `## Don't use when` row, stop and route out. `## Don't use when` is the routing section that
      `skill-routing-section` checks; the name `Not for` is not used. Do not run the workflow on a mismatched input.
      (`challenge` added this as an enforced Step 0 and lifted routing/boundary scores out of the 1/5 hole.)
- [ ] **Define the abort output.** When the preflight aborts, say exactly what the user gets. It is typically a one-line
      mismatch, one starter question, and the handoff target. It is not an empty version of the normal deliverable.
- [ ] **Handoff questions read as natural English.** A next step can be a phase boundary, a consented offer, or a
      close-out. When the skill offers one, the question describes the step in plain English first. It names the
      invocation in parentheses at the end: "Ready to close this out (via `/agentics:gate branch`)?" Never lead with the
      command ("Run `/agentics:execute`?"). The parenthetical always carries the leading slash. It shows only the skill
      and mode — never argument placeholders (`/agentics:review axes`, not `/agentics:review axes <base-branch>`). The
      skill resolves its own inputs when it runs.
- [ ] **Handle thin / edge input explicitly.** Name what happens for a bare goal, a missing precondition, or
      contradictory input. Do not let the workflow assume the happy path. If a precondition is required (e.g. "an
      approved PRD", "a plan"), add a check that verifies it. The check redirects when the precondition is absent.
- [ ] **Give fallback paths their own numbered workflow.** Some skills have a primary and a fallback tool (e.g. MCP →
      CLI). In such a skill, the fallback needs its own concrete steps, not a bare heading. An operator who follows the
      fallback literally must not silently drop metadata that the primary path would have set.
- [ ] **Every command and example must be real.** No authoritative-looking literal may actually be wrong for the repo it
      runs in. Wrong literals include a hardcoded label, ID, or field value that doesn't exist. Use `<placeholders>`
      plus a one-line "verify exact values before reuse", or a command that discovers the real values first.
- [ ] **Define "done" and "sufficient context".** Open-ended skip-clauses ("skip steps if context already exists")
      invite a rushed operator to bypass verification. State the criteria and who decides.
- [ ] **No dangling references.** Every `/skill`, reference-file, and cross-link pointer must resolve to something that
      exists. A pointer to a skill that was never built breaks the moment a real task reaches it.
- [ ] **Current-state-only body.** Put no history narration ("formerly," "renamed from," "used to be," retirement
      asides) in skill or agent bodies. That content moves to the repo's change record. A short, time-boxed alias clause
      in the _description_ is the only permitted old-name mention.

## B. Pressure resilience — hold the boundary under adversarial input

Every gate belongs to one of three classes: safety gate, repository policy, or process gate. Pushback gets one of two
responses: the safety hold or the process yield. A skill links the shared gate contract in one line and does not restate
it. This section is the compact form an author checks a skill against.

**Safety gate.** A safety gate protects something the user cannot take back or accept on behalf of others. The list is
closed, and a skill cannot add to it:

- Loss of work or history that cannot be undone.
- A change outside the machine that the authorization rule below does not cover.
- A claim of status with no evidence from this session.
- The handling of secrets and of text from outside the trust boundary.
- Bypassing a commit hook, or commit or tag signing.
- The user's own approval, including the independent reviews that come before an approval question: the spec review, the
  plan review that the records require, and the review of a brief.

The authorization rule: a step the user named in their request is authorized when its turn comes, with no second
question, as long as what it touches is what the user named. Two kinds of step need one confirmation even when the user
named them. A destructive step is confirmed once at the point of action, after the agent has shown the resolved target
and what will be lost, and the reply names the action or its target in the user's own words. A write that changes what
later agents are told needs the user's yes in the session after the exact text to be written has been shown. A push or a
post to the target the user named is therefore not a safety gate; one to a target the user did not name is. A named post
that a skill's own posting rules preview first is also asked about, as an ordinary approval question and not a gate;
[approval.md](../../../routing/references/contracts/approval.md#a-named-post-a-skill-previews-first) holds that
exception.

The safety hold has one fixed shape, said once: what is held, the harm it prevents, and what the user can do instead.
Every hold names that alternative, so no stop is without an exit. A repeated demand gets a one-line pointer back to the
alternative, not the hold again.

**Repository policy.** A repository policy is a rule that a settings file switches. It is held while the setting is on,
whatever is said in the session. The agent names the settings change once as the exit, and it writes no `waive` row,
because nothing was waived.

**Process gate.** Every other gate is a process gate. It names its cost once and yields on the next reply that still
declines, in any wording. It never asks for a reason and never refuses for lack of one. The process yield is recorded as
one `waive` row, which carries the reason when the user gave one and `none given` otherwise.

A rule in a repository instruction document that the user explicitly departs from is not a gate class; it follows
[user-authorized-divergence.md](../../../routing/references/user-authorized-divergence.md).

- [ ] **Class each gate once.** State each gate once in its skill, with its class. A gate is a process gate unless it is
      on the safety list or a settings file switches it.
- [ ] **State why the boundary matters.** Give the operator the concrete failure mode, not just an unexplained rule.
      Examples are the double-push, the dropped field, and the unreviewed merge. The failure mode gives the operator
      something to cite when a user pushes back.
- [ ] **Ban verdicts and unrequested deliverables.** The ban covers a skill that only interrogates or routes. If the
      skill is meant to surface, not decide, name the ban explicitly. A per-question recommended answer is not a
      verdict. It is permitted when the skill's contract states that the decisions remain the user's (e.g. `challenge`'s
      ➡️ recommendations). The ban targets plan-level go/no-go stamps and the uninvited implementation of fixes.
- [ ] **Take the user's word about their own work.** A plain statement from the user about their own organisation,
      ownership, intent, plans or history is accepted and acted on. When evidence in the session contradicts it, the
      agent says once what it observed and then follows the user. A statement never overrides a safety gate. A role or
      title changes nothing: a safety gate holds for everyone and a process gate yields to everyone. A cheap check the
      agent can run itself needs no permission and no proof from the user. The rule against unverified claims covers
      what the agent asserts, not what the user tells it.
- [ ] **Named steps.** A skill with numbered steps or a checklist names the entries to track and links to
      [workflow steps](../../../routing/references/contracts/workflow-steps.md).
- [ ] **Shared text lives in one file.** Text that several skills share lives in one shared file, and a skill links that
      file in one line instead of carrying the text.
- [ ] **Never assert a status you didn't produce.** This rule covers "merged", "closed", "passing", and "done". Do not
      report one of them unless this run actually did it and verified it.

---

## Cross-skill contract hygiene

A hardening pass on one skill often surfaces a contradiction with another. Two recurring traps:

- **Auto-invoke contracts.** Do not instruct a skill to "automatically invoke" another skill whose frontmatter or prose
  declares it user-initiated only (`disable-model-invocation: true`). Surface a question to the user, or route, instead
  of an assertion of an internal auto-call the other skill forbids. When you change one side of such a contract, update
  every skill and agent that references it. Make those updates in the same pass. An example of such a change is to make
  a skill auto-invokable.
- **Escalation chains must match the roster.** `routing` Step 3 or a sibling skill can document a chain (e.g.
  `challenge → /agentics:challenge discern → general-purpose`). If one does, every skill that references part of the
  chain must name the same steps. A skill that skips a documented link routes work to the wrong place.

When a fix changes a routing boundary, do these steps:

1. Grep the roster and sibling skills for the other half of the contract.
2. Update both sides.
