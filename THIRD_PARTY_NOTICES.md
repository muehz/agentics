# THIRD-PARTY NOTICES

This file documents third-party code, skill patterns, prompts, and evaluation approaches used, adapted, or referenced in
this project.

## The Go Authors — BSD-Style License

The hook executables under `hooks/bin/` are built with the Go toolchain and include the Go runtime and standard library.

Full license text from <https://go.dev/LICENSE>:

> Copyright 2009 The Go Authors.
>
> Redistribution and use in source and binary forms, with or without modification, are permitted provided that the
> following conditions are met:
>
> - Redistributions of source code must retain the above copyright notice, this list of conditions and the following
>   disclaimer.
> - Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following
>   disclaimer in the documentation and/or other materials provided with the distribution.
> - Neither the name of Google LLC nor the names of its contributors may be used to endorse or promote products derived
>   from this software without specific prior written permission.
>
> THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES,
> INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
> DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
> SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
> SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
> WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
> OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

## obra/superpowers — MIT License

Source repository: <https://github.com/obra/superpowers>

This repository adapted substantial workflow material from `obra/superpowers`: 11 skills were adapted directly from that
project (see the per-skill provenance table below), and improvements in this repository were also seeded from
approximately 10 additional Superpowers skills and patterns.

Beyond the skill-level adaptations, several Agentics reference files carry substantial wording from `obra/superpowers`
and are listed individually in the per-file provenance manifest below. The largest are
`forge/references/examples/claude-md-testing.md` (near-verbatim from `writing-skills/examples/CLAUDE_MD_TESTING.md`),
`forge/references/quality/advanced-patterns.md` and `forge/references/quality/cso-guide.md` (from
`writing-skills/SKILL.md`), `forge/references/governance/governance-details.md` (from
`writing-skills/testing-skills-with-subagents.md`), the `diagnose` reference set (`debugging-checklists.md`,
`defense-in-depth.md`, `condition-based-waiting.md`, `root-cause-tracing.md`, from `systematic-debugging/`), and the
`review` feedback references (`receiving-code-review.md`, `requesting-review.md`, `interactive-review.md`).

Full license text from <https://github.com/obra/superpowers/blob/main/LICENSE>:

> MIT License
>
> Copyright (c) 2025 Jesse Vincent
>
> Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
> documentation files (the "Software"), to deal in the Software without restriction, including without limitation the
> rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit
> persons to whom the Software is furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the
> Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
> WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
> COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
> OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## anthropics/skills — Apache-2.0 Attribution

Source repository: <https://github.com/anthropics/skills>

This project includes limited adaptations and inspirations from the Apache-2.0-licensed example skills in
`anthropics/skills`:

- The `skill-authoring` (originally `skill-creator`) evaluation approach was an inspiration point only; the
  implementation in this repository was rewritten from scratch.
- `agents-authoring` and `instructions-authoring` (originally `agents-creator` and `instructions-creator`) evaluation
  workflows were inspired by Anthropic skill patterns and rewritten for this repository.
- `documentation-authoring` (originally `doc-coauthoring`) was adapted under Apache-2.0. The workflow was made
  platform-neutral (Claude-specific references removed) and integrated into the agentics plugin.
- `mcp-builder` material was vendored into the `forge` skill's `mcp` mode and removed in November 2026 in favour of the
  upstream skill and the MCP documentation.
- The `docx`, `xlsx`, `pptx`, and `pdf` skills are **not** adopted here. Consumers should install those directly from
  `anthropics/skills`; they are source-available/proprietary document skills and are not redistributed in this
  repository.

## github/spec-kit — MIT License

Source repository: <https://github.com/github/spec-kit>

Concepts adapted (not vendored) into the `spec` skill:

- The **ambiguity / coverage taxonomy** from `templates/commands/clarify.md` — scanning a draft across functional scope
  and behavior, domain and data model, interaction and UX flow, non-functional concerns, integrations, and edge cases,
  marking each Clear / Partial / Missing and letting that map drive which questions are worth asking. Agentics uses it
  as the gate that decides whether an interview happens at all.
- **Prioritized, independently-testable user stories** from `templates/spec-template.md` — the P1/P2/P3 priority and the
  requirement that a story be independently testable and deliverable. Agentics keeps its own canonical user-story
  sentence form and adds these as annotations.
- **Measurable, technology-agnostic Success Criteria** from the same template.

No spec-kit file is copied into this repository; its scripts, `.specify/` project layout, constitution mechanism, and
command set are not adopted.

## github/gh-stack — MIT License

Source repository: <https://github.com/github/gh-stack>

No content from `gh-stack` is vendored or adapted into this repository. The entry is recorded because the `gh` skill's
`stack` mode takes a deliberate **dependency** on it rather than re-deriving it: `gh-stack` is a GitHub CLI extension
(`gh extension install github/gh-stack`) that ships its own first-party agent skill
(`gh skill install github/gh-stack`), and `skills/gh/references/stack.md` points at that skill for command mechanics
instead of duplicating them. Agentics contributes only the judgment layer — when a change set warrants a stack — and the
Agentics-specific safety rules layered on top (draft-only submission, review-gated merge). The command map in that
reference is factual CLI surface verified against `gh stack --help`.

## mattpocock/skills — MIT License

Source repository: <https://github.com/mattpocock/skills>

This project includes substantial adaptations and inspirations from the MIT-licensed skills in `mattpocock/skills`:

- `deepen` (refactor-time material; upstream name `improve-codebase-architecture`) — substantially adopted. The headline
  framing ("Surface architectural friction and propose deepening opportunities — refactors that turn shallow modules
  into deep ones"), the Glossary terminology (Module, Interface, Implementation, Depth, Seam, Adapter, Leverage,
  Locality), the depth/leverage/locality principles, and the "Use CONTEXT.md vocabulary" instruction are adapted from
  Matt's skill. `references/language.md`, `references/deepening.md`, and `references/interface-design.md` carry
  similarly-adapted content.
- `deepen/references/context-format.md` and `deepen/references/adr-format.md` — vendored from
  `mattpocock/skills/skills/engineering/domain-modeling/` (`CONTEXT-FORMAT.md` and `ADR-FORMAT.md`). `adr-format.md`
  travels verbatim; `context-format.md` retains roughly two thirds of the upstream wording. The upstream
  `grill-with-docs` skill is not adopted in agentics; only these two format references are used.
- `challenge` — adapted from Matt's `grilling` (`mattpocock/skills/skills/productivity/grilling/`). The interview core —
  interview the user relentlessly until a shared understanding is reached, map the plan as a decision tree, look up
  facts instead of asking for them, leave decisions to the user, act only after the user confirms — is adapted from the
  upstream skill. The frontier-rounds interview model (ask every currently-answerable question as one numbered round,
  recompute the frontier as answers land), the pinned ❓/➡️ question format (vendored verbatim), the per-question
  recommended answer, and non-blocking sub-agent fact-finding are adopted from the current upstream. Agentics layers on
  its own preflight gate, route-away table, caller-scope narrowing, structured summary, and the separate `discern` mode.
- `diagnose` Phase 1 — the "**This is the skill.** Everything else is mechanical…" framing and the feedback-loop
  construction catalog (failing test, curl/HTTP harness, headless browser, replay, fuzz, bisection, differential, HITL)
  at `references/feedback-loop-construction.md` are adapted from Matt's `diagnose`. Agentics's `diagnose` also draws
  from `obra/superpowers` (see above) — the two upstreams contributed complementary content.
- `triage` — the mandatory AI-attribution rule on every comment posted during triage is adapted from Matt's `triage`;
  Agentics carries its own wording and places the attribution as a trailing footer. The role-based state-machine pattern
  for issue-mode is also adapted from this source. Two reference files under `skills/triage/references/issues/` travel
  near-verbatim from the same upstream skill: `out-of-scope.md` (from `OUT-OF-SCOPE.md`) and `agent-brief.md` (from
  `AGENT-BRIEF.md`).
- `spec` (solution-definition workflow) — the template structure (Problem Statement / Solution / User Stories / Scope
  Decisions / Testing Scope / Out of Scope) and the canonical User Story form
  (`As a <actor>, I want a <feature>, so that <benefit>`) are adapted from Matt's `to-prd`, which upstream now carries
  as `to-spec`. The earlier two-part shape (`spec draft` ↔ Matt's `to-spec`, delivery slicing ↔ Matt's `to-issues`) is
  also adopted; Agentics now keeps solution definition in `spec` and owns delivery slicing in
  `skills/decompose/references/slicing.md`. Two further elements come from the current upstream `to-spec`: the
  **synthesize-from-the-conversation** posture (derive the document from what has already been discussed rather than
  opening with an interview) and the **seams** step (sketch the test seams, preferring existing and highest seams, and
  confirm them before writing). Agentics diverges deliberately at the endpoint — where upstream never interviews, `spec`
  runs an ambiguity scan and hands off to `/challenge` for a cross-examination scoped to whatever remains unresolved.
- `review` — the Standards/Spec two-axis parallel-subagent review mode is adapted from Matt's in-progress `review`
  skill.
- `tdd` — the "Anti-Pattern: Horizontal Slices" content (vertical tracer-bullet framing) is adapted from Matt's `tdd`.
- `prototype` (new skill in agentics) — adopted whole, with adaptation. Routes between a runnable terminal app for
  state/business-logic questions (`references/logic.md`) and several radically different UI variations toggleable from
  one route (`references/ui.md`).
- `handoff` (new skill in agentics) — adopted from Matt's in-progress `handoff`. Compacts the current conversation into
  a handoff document so another agent can pick up the work.
- `deepen` (design-time material; upstream name `codebase-design`) — adapted from
  `mattpocock/skills/skills/engineering/codebase-design/` (MIT). The design-time framing (before/as code is written),
  the forward-looking deletion-test clause, the speculative-port resistance, and the testability guidance are adapted
  into Agentics voice and live in `references/language.md` and the skill body's design-time path; the upstream
  `design-it-twice.md` material is merged into `references/interface-design.md`, which carries a dual attribution with
  `improve-codebase-architecture`. The deep-module glossary and dependency-category guide are covered by the
  refactor-time entry above and are not vendored twice.
- `deepen` (domain-modelling material; upstream name `domain-modeling`) — adapted from
  `mattpocock/skills/skills/engineering/domain-modeling/` (MIT). `skills/deepen/references/context-map.md` is adapted
  from the multi-context material in the upstream skill, and the glossary-sharpening moves in the skill's grilling loop
  (challenge a term against the glossary, propose a precise canonical term for an overloaded one, stress-test boundaries
  with concrete scenarios, surface code/statement contradictions) are adapted into Agentics voice. The upstream
  `CONTEXT.md` glossary format and ADR template are covered by the entries above and are not vendored twice.
- `resolving-merge-conflicts` — adapted from `mattpocock/skills/skills/engineering/resolving-merge-conflicts/` (MIT)
  into the existing `git` skill as its `resolve` mode (`/git resolve`), backed by
  `skills/git/references/merge-conflicts.md`. The five-step workflow (see current state → find the primary sources
  behind each side → resolve each hunk preserving both intents → run the project's own checks → finish without aborting)
  is adapted into Agentics voice; a cross-reference row was added to the `gh` skill so PR-conflict asks route to
  `/git resolve`.
- `gh/references/pull-requests/body.md` — adapted from `mattpocock/skills/skills/engineering/pr/` (MIT), branch
  `release/v1.3` pinned at `2aecca12ea9ff047f76c8178cb64dfeafd192abe` (merged as mattpocock/skills#1120). The visual
  menu (pseudocode, call, component, and file trees, Mermaid, shaped diffs) and its placement guidance are vendored
  close to verbatim; upstream credits that menu to Dex Horthy's `show-me` skill (humanlayer/skills, below). Evidence and
  Merge Danger are rewritten for Agentics: Merge Danger becomes Impact, stated as concrete reversibility and blast
  radius rather than door labels; evidence is limited to work actually performed; and the blocks map into a repository's
  own PR template. Measured retention for the whole file is 15%, below the per-file manifest's 20% floor, so it has no
  manifest row.

Full license text from <https://github.com/mattpocock/skills/blob/main/LICENSE>:

> MIT License
>
> Copyright (c) 2026 Matt Pocock
>
> Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
> documentation files (the "Software"), to deal in the Software without restriction, including without limitation the
> rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit
> persons to whom the Software is furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the
> Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
> WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
> COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
> OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## cursor/plugins (pstack) — MIT License

Source repository: <https://github.com/cursor/plugins> (the `pstack` plugin, author Lauren Tan)

This project adapted material from the MIT-licensed `pstack` Cursor plugin — into existing Agentics skills, and as new
Agentics skills. Cursor-specific mechanics (Task-tool semantics, cross-vendor model slugs, `~/.cursor/` paths) were
rewritten for Agentics's surfaces and model-tier rules throughout:

- `docs` — the four-layer technical-writing standard (Diátaxis mode selection, Google developer style, STE instruction
  rules, Global English ambiguity rules), the worked example, and the sentence-level review checklist are adapted from
  pstack's `technical-writing` into `skills/docs/references/technical-writing.md`.
- `execute` (arena mode) — the candidate-bakeoff workflow (frame → fan out → cross-judge → pick → graft → verify, with
  rubric-based base selection, recorded rejections, and the convergence/divergence signals) is adapted from pstack's
  `arena` into `skills/execute/references/arena-mode.md`. Model diversity is re-based on Agentics's model tiers and
  user-configured alternate-family reviewers.
- `skills/execute/references/flow-state.md`, `hooks/go/internal/flow`, and the frozen scanner fixture
  `hooks/go/internal/shellwords/testdata/flow.sh` — the append-only TSV record, its first-use header, and the cell
  hardening (tab, CR, and LF replaced by a space, and the spreadsheet formula-injection guard that prefixes a cell
  starting with `=`, `+`, `-`, or `@` with a single quote) are adapted from pstack's `show-me-your-work` and its
  `scripts/log.sh`. These files replace the retired decision-trail contract and its two helper scripts, which carried
  the same adaptation. The frozen fixture is test data only and is never a runtime fallback. Agentics wrote the
  nine-column `state.tsv` format, the events, and the `flow` commands itself; they share no wording with the upstream
  beyond that hardening.
- `review` and `challenge` — the multi-reviewer synthesis discipline (consensus weighting, the Act on / Consider / Noted
  / Dismissed verdict buckets with the Dismissed list shown, the agreement map) is adapted from pstack's `interrogate`
  into `skills/review/references/verdict-synthesis.md` and challenge's summary step.
- `diagnose` and `review` (evidence posture) — the confidence-tier epistemics (Direct / Supported / Inferred /
  Speculative / Unknown, calibrated phrasing, the sycophancy trap, null results as findings, the Sources Consulted
  coverage map) is adapted from pstack's `why/references/epistemics.md`. The canonical copy ships once, as
  `skills/why/references/epistemics.md`; `diagnose`, `review`, and `verdict-synthesis.md` link it.
- `deepen` and `intent` — the design-twice floor (two structurally distinct candidates before synthesis), the design red
  flags (vendored, adapted, as `skills/deepen/references/design-red-flags.md`), and the scrap trigger (repeated friction
  of the same shape forces a redesign) are adapted from pstack's `architect` and its `references/design-red-flags.md`.
- `tdd` — the negative space (prefer no new test over a bad test, the bad-test definition, the impractical-test list)
  and the completion evidence report (failing-before, passing-after) are adapted from pstack's `tdd`.
- `routing` and `decompose` — the design-a-bespoke-workflow rule (the designed playbook is the reviewable artifact) and
  the empirical-fork rule (an experimentally answerable question runs as an experiment via `prototype` instead of being
  asked) are adapted from pstack's `figure-it-out`.
- `execute`, `routing`, `intent`, `decompose`, `gate`, `tdd`, `diagnose`, and `review` — the verbatim playbook-step todo
  discipline (matched workflow steps and checklists copied into the todo list verbatim; a skipped step stays visible as
  `skip: <reason>`) is adapted from pstack's `poteto-mode`. Its canonical rule lives once in
  `skills/forge/references/quality/skill-hardening-checklist.md`, which every hardened skill cites.
- `git` — the worktree prune workflow (audit from `git worktree list`, buckets as advice not permission, the
  uncommitted-work pause, branch refs surviving removal, the before/after disk report) is adapted from pstack's
  `poteto-mode/playbooks/worktree-cleanup.md` into `/git worktrees prune`.
- `explain` (new skill in agentics; **why mode**) — the multi-source code-archaeology workflow from pstack's `why` is
  adapted whole: the seven evidence categories (source control, issue tracker, long-form documents, team chat,
  infrastructure observability, error tracking, product analytics warehouse), one investigator per category, null
  results as first-class findings, the skip rules with written justification, the epistemics confidence framework, and
  the Sources Consulted coverage map. The twelve why-mode files under `skills/explain/references/` (`epistemics.md`,
  `investigator-prompt.md`, `synthesizer-prompt.md`, `source-playbook.md`, `sources/*.md`) are adapted from the upstream
  skill's references, near-verbatim except for surface rewrites. Agentics rewrites the dispatch layer: Cursor
  Task/model/readonly semantics become Claude Code Agent semantics with Agentics model ladders, and MCP discovery
  follows Agentics project charter principle 10 ([docs/CHARTER.md](docs/CHARTER.md)) — detect what the host exposes at
  run time and degrade to git and `gh` CLI when nothing is configured.
- `explain` (**how mode**) — adapted from pstack's `how`: the two shapes (explain default, critique on explicit
  request), the complexity gate (simple = one pass, complex = 2–4 parallel explorers then one explainer), the output
  contract (Overview / Key concepts / How it works / Where things live / Gotchas), critique's explain-first rule, and
  the Act on / Consider / Noted / Dismissed lead-judgment buckets. The four how-mode reference files
  (`explorer-prompt.md`, `explainer-prompt.md`, `critic-prompt.md`, `critique-rubric.md`) are adapted with attribution
  headers. Agentics diverges deliberately on dispatch: upstream's multi-vendor critic panel (one critic per configured
  model) is replaced by a fixed-size panel of Claude-family critics differentiated by assigned rubric lenses, and
  upstream's per-skill model configuration is replaced by the model ladders.
- `docs` (`unslop` mode) — the 31-pattern AI-tell catalog (content, language, style, communication artifacts, filler,
  jargon, plain speech) and the "adding soul" section from pstack's `unslop` skill are vendored near-verbatim into
  `skills/docs/references/slop-patterns.md`, with an Agentics-added scope guard for repo house styles and reserved
  vocabulary. The mode wiring — Unslop mode dispatch, the Build-mode slop sweep, and the fix-report requirement — is
  Agentics's own; the upstream skill itself is not adopted as a standalone skill.
- `explain` (**teach mode**) — adapted from `pstack/skills/teach/`. The teaching core is adopted: pick the few takeaways
  from why the person is asking, open with a plain definition before the case at hand, the target-density example, one
  name per concept, build diagrams up one part at a time instead of one crowded diagram, no pacing theater or framing
  labels, conversation over lecture, and the reply-is-the-explanation contract. Adapted for Agentics: the upstream skill
  invoked pstack's `how`, `why`, and `unslop` skills — Agentics's `explain` (teach mode) invokes its own how mode
  (mechanism) and why mode (history) for subsystem investigation, keeps direct reads for small changes, and carries the
  density and style rules inline.
- `recall` — adapted from `cursor/plugins/pstack/skills/recall/` (MIT). The workflow core — scope locking before any
  search (window, topic, workspace; never quietly narrowing "all" to a recent subset), parallel cheap-model subagents
  over transcript slices with raw transcripts kept out of the main thread, the shared-record sweep as the default for
  any named target, live-state verification with `git` and `gh`, and the output contract (capsule of at most 5 bullets,
  status-tagged threads, at most 5 problems, one next move) — is adapted from the upstream skill. Agentics ports the
  transcript location from Cursor's `~/.cursor/projects/<slug>/agent-transcripts/` layout to Claude Code's
  `~/.claude/projects/<slug>/` layout, replaces the upstream `why`/`unslop`/`session-pickup`/`automate-me`
  cross-references with the Agentics roster equivalents (a conditional `explain` why-mode delegation that degrades to
  `git`/`gh`, and routing rows for `handoff`, `diagnose`, `docs`, and `execute`), and adds its own preflight gate and
  pressure-resilience clauses.
- `review` (**impact mode**) — adapted from `pstack/skills/blast-radius/`. The five-step evidence ladder (said so /
  pointed at the line / walked the failure / ran it / reproduced in the running app), the "find the one fact the change
  is safe because of" method with its prove-it-by-running-real-code discipline, the output contract (What it does / The
  one fact / Risks / Cleared / Before you merge), and the never-round-up rule on unproven facts are preserved from
  upstream. Agentics rewrites the Cursor-specific mechanics: the upstream `arena` multi-model step becomes
  independent-subagent fan-out (Claude Code Agent semantics), and cross-references to pstack's `how`, `why`, and
  `unslop` skills are removed or repointed to Agentics skills (`gh` for pulling PR history; `review`/`challenge` at the
  routing boundary). Agentics layers on its own preflight gate, `## Don't use when` table, and the declinable-proof /
  non-waivable-marker pressure rule.
- `reflect` — adapted into the existing `forge` skill as its `reflect` mode (`/forge reflect`) rather than shipped as a
  standalone skill. The workflow shape (locate the active transcript, fan out three parallel reviewer subagents —
  judgment, tooling, divergent — synthesize into Accepted / Rejected / Backlog, apply a structural-enforcement check,
  present for explicit user approval before any edit) is adapted from `pstack/skills/reflect/SKILL.md`. The four prompt
  templates are vendored with adaptation into `skills/forge/references/reflect/` (`judgment-reviewer.md`,
  `tooling-reviewer.md`, `divergent-reviewer.md`, `synthesizer.md`): Cursor Task dispatch and cross-vendor model pins
  were rewritten to Agentics's surface-dispatch and model tiers, `~/.cursor/` transcript and skill paths became their
  `~/.claude/` equivalents, routings through Cursor's built-in `create-skill` became routings through `forge`'s own
  authoring flow, and automatic backlog filing became present-to-the-user (Agentics skills never auto-post). The
  prompt-injection hardening and the accept/reject criteria travel near-verbatim.
- `zealot` (new skill in agentics; adopted as `decomment`, renamed `zealot`) — adapted from pstack's `no-comments` skill
  and its Comment Sicko agent. The wrapper workflow (scope resolution, report audit, one-rerun budget,
  trivial-fix/root-cause split, constraint-conversion-with-approval) is adapted from `no-comments`; the finder charter
  at `skills/zealot/references/comments/comment-zealot.md` is adapted from `comment-sicko.md` with the persona renamed
  **Comment Zealot**. Agentics structural changes: the persona is an embedded references/ charter dispatched into a
  general-purpose subagent (not a registered agent), upstream's `/how`, `/why`, and `/architect` skill calls become
  direct code investigation and a one-shot design consult, and the upstream `principle-*` skill references are inlined
  as intent.
- `skills/forge/references/quality/adversarial-pair-pattern.md` — Agentics's generalization of the
  wrapper-plus-extremist review pattern the pair demonstrates (calibration by leash, inverted burden of proof,
  capability bound, wrapper-owned judgment, rerun budget). The mechanisms are distilled from the upstream pair; the
  write-up is Agentics's own.
- `principles` (new skill) — pstack's 21 `principle-*` leaf skills are adapted into one Agentics skill: a compact index
  in `skills/principles/SKILL.md` and one reference file per principle under `skills/principles/references/` (each file
  carries an attribution header). The citation discipline — when a principle shapes a decision, name the principle and
  the specific choice it changed; a citation with no decision behind it is invalid — is adapted from pstack's
  `poteto-mode` skill. Cursor-specific mechanics (links to pstack-only skills, Cursor Task semantics) are rewritten for
  Agentics; the principle content itself travels close to verbatim. Every leaf closes with a `## Self-test` section.
  Four of those are adapted from the upstream test sections (`principle-boundary-discipline`,
  `principle-minimize-reader-load`, `principle-make-operations-idempotent`, `principle-type-system-discipline`);
  upstream carries no test section for the other seventeen, so those questions are Agentics's own.
- `scaffold` feedback-loops author/audit modes — adapted from **two** upstream skills. `/scaffold feedback-loops author`
  is adapted from `pstack/skills/create-verification-skill` (repo-first interview; the six generated sections
  Launch/Doctor/Drive/Evidence/Cleanup/Helpers; the seeded feature map with its four fixed H2s; the
  prove-the-generated-skill-once rule). `/scaffold feedback-loops audit` is adapted from
  `pstack/skills/maintain-verification-skill` (the three outcomes clean/changed/blocked; the edit-scope rule; the
  per-feature source wave; the live pass with its three invariants; the doc-drift / harness-gap / product-regression
  triage rule). The feature-map example under `skills/scaffold/references/feedback-loops/feature-map-example/` is
  vendored from the upstream `create-verification-skill/references/feature-map-example/` with an added attribution
  header. Agentics diverges deliberately: the generated skill lands in `.claude/skills/verify-<app>/` instead of
  `.cursor/skills/`, the Drive recipes are ported from Cursor control skills to Claude Code equivalents
  (`skills/scaffold/references/feedback-loops/drive-recipes.md` — browser tools for web, tmux/PTY for CLI/TUI, curl for
  services), and the mode structure follows Agentics's `ops` runbook author/audit pattern.
- `hillclimb` (new skill in agentics) — adapted from the `hillclimb` playbook in pstack's `poteto-mode` skill
  (`pstack/skills/poteto-mode/playbooks/hillclimb.md`). The core discipline is preserved: one change, one measurement,
  keep or revert; a frozen measurement harness with proven sensitivity; a stop predicate pairing a target with a floor
  on attempts; a gitignored `decision.tsv` decision log read before each attempt; pushing past the first plateau; never
  relaxing the predicate to declare victory. Cursor Task/model semantics were rewritten to Claude Code subagent
  semantics and Agentics model-ladder guidance, and pstack-internal skill references (principles, playbooks) were
  replaced with Agentics equivalents or inlined.

Full license text from <https://github.com/cursor/plugins/blob/main/pstack/LICENSE>:

> MIT License
>
> Copyright (c) 2026 Lauren Tan
>
> Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
> documentation files (the "Software"), to deal in the Software without restriction, including without limitation the
> rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit
> persons to whom the Software is furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the
> Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
> WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
> COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
> OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## gitleaks — MIT License (adapted detector rules)

Source repository: <https://github.com/gitleaks/gitleaks>

Agentics adapts eight detector rows from Gitleaks v8.28.0 at commit `4fb43823ef3d152d239e92d7d5cb04783b548062`. The
exact upstream source files used for this adaptation are:

- `config/gitleaks.toml` — SHA-256 `c3bee40d87bb4689e3f5ffa5cc0315f7e6e4d43f4e9388397fca59f29407b432`
- `LICENSE` — SHA-256 `e3884b252b3bfc045e55be43a34d1e80da070bc6f804ac95bf4660e97d62ebc6`

The adapted matching rows and their shared capture, keyword, entropy, and rule-local allowlist metadata are in
`hooks/go/internal/secretpatterns/upstream.go`. The corresponding license text is retained in
`hooks/go/internal/secretpatterns/LICENSE.gitleaks`. Agentics deliberately keeps the legacy `AWS_ACCESS_KEY` rule as the
owner of AKIA tokens, including its low-entropy behavior, and the adapted `aws-access-token` row excludes AKIA to avoid
duplicate findings while retaining the upstream ASIA/A3T/ABIA/ACCA coverage. Agentics also retains only adapted rows
rather than vendoring the full upstream configuration or runtime.

Full license text is retained in `hooks/go/internal/secretpatterns/LICENSE.gitleaks`.

## humanlayer/skills — MIT License

Source repository: <https://github.com/humanlayer/skills>

The visual menu in `skills/gh/references/pull-requests/body.md` (pseudocode, call trees, component trees, file trees,
Mermaid, and shaped diffs, with their placement guidance) originates in Dex Horthy's `show-me` skill
(`plugins/show-me/skills/show-me/SKILL.md`) and reached Agentics through `mattpocock/skills`' `pr` skill, which credits
it.

Full license text from <https://github.com/humanlayer/skills/blob/main/LICENSE>:

> MIT License
>
> Copyright (c) 2026 HumanLayer
>
> Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
> documentation files (the "Software"), to deal in the Software without restriction, including without limitation the
> rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit
> persons to whom the Software is furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the
> Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
> WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
> COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
> OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Per-file provenance manifest

**Attribution for this repository lives in this file only.** Shipped skill files under `skills/` carry no attribution
headers, so a reader looking for the source of a given file looks here. The one exception is noted below.

Every path below is relative to `skills/`. **Retained** is the share of the Agentics file's 6-gram text that also
appears in the named upstream file — a measure of copied _expression_, not of how much the workflow was changed. It was
produced by diffing this checkout against each upstream's default branch; treat the figures as provenance evidence with
a margin, not as exact percentages, since upstreams move independently of Agentics.

A file absent from this table either has no measurable upstream text or falls below a 20% retention floor. Absence from
the table is not a claim of independent authorship — the per-source sections above remain the authoritative statement of
what was adapted, including concept-level adoption that leaves no shared wording.

**Apache-2.0 modification notices.** Every file identified above as adapted under Apache-2.0 keeps an in-file header.
Apache-2.0 §4(b) requires modified files to carry prominent notice that they were changed; a central manifest does not
discharge that per-file obligation the way MIT's single-notice condition does. Do not strip those headers as part of a
context-reduction pass.

| File (under `skills/`)                                                  | Upstream                | License    | Upstream path                                                                           | Retained |
| ----------------------------------------------------------------------- | ----------------------- | ---------- | --------------------------------------------------------------------------------------- | -------- |
| `deepen/references/adr-format.md`                                       | mattpocock/skills       | MIT        | `skills/engineering/domain-modeling/ADR-FORMAT.md`                                      | 100%     |
| `deepen/references/context-format.md`                                   | mattpocock/skills       | MIT        | `skills/engineering/domain-modeling/CONTEXT-FORMAT.md`                                  | 66%      |
| `deepen/references/deepening.md`                                        | mattpocock/skills       | MIT        | `skills/engineering/codebase-design/DEEPENING.md`                                       | 77%      |
| `deepen/references/design-red-flags.md`                                 | cursor/plugins (pstack) | MIT        | `pstack/skills/architect/references/design-red-flags.md`                                | 78%      |
| `deepen/references/interface-design.md`                                 | mattpocock/skills       | MIT        | `skills/engineering/codebase-design/DESIGN-IT-TWICE.md`                                 | 56%      |
| `deepen/references/language.md`                                         | mattpocock/skills       | MIT        | `skills/engineering/codebase-design/SKILL.md`                                           | 63%      |
| `diagnose/references/condition-based-waiting.md`                        | obra/superpowers        | MIT        | `skills/systematic-debugging/condition-based-waiting.md`                                | 48%      |
| `diagnose/references/debugging-checklists.md`                           | obra/superpowers        | MIT        | `skills/systematic-debugging/SKILL.md`                                                  | 69%      |
| `diagnose/references/defense-in-depth.md`                               | obra/superpowers        | MIT        | `skills/systematic-debugging/defense-in-depth.md`                                       | 58%      |
| `docs/SKILL.md`                                                         | anthropics/skills       | Apache-2.0 | `skills/doc-coauthoring/SKILL.md`                                                       | <20%     |
| `diagnose/references/feedback-loop-construction.md`                     | mattpocock/skills       | MIT        | `skills/engineering/diagnosing-bugs/SKILL.md`                                           | 53%      |
| `diagnose/references/root-cause-tracing.md`                             | obra/superpowers        | MIT        | `skills/systematic-debugging/root-cause-tracing.md`                                     | 39%      |
| `docs/references/slop-patterns.md`                                      | cursor/plugins (pstack) | MIT        | `pstack/skills/unslop/SKILL.md`                                                         | 71%      |
| `docs/references/technical-writing.md`                                  | cursor/plugins (pstack) | MIT        | `pstack/skills/technical-writing/SKILL.md`                                              | 57%      |
| `execute/references/arena-mode.md`                                      | cursor/plugins (pstack) | MIT        | `pstack/skills/arena/SKILL.md`                                                          | 30%      |
| `explain/references/critic-prompt.md`                                   | cursor/plugins (pstack) | MIT        | `pstack/skills/how/references/critic-prompt.md`                                         | 71%      |
| `explain/references/critique-rubric.md`                                 | cursor/plugins (pstack) | MIT        | `pstack/skills/how/references/critique-rubric.md`                                       | 98%      |
| `explain/references/epistemics.md`                                      | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/epistemics.md`                                            | 96%      |
| `explain/references/explainer-prompt.md`                                | cursor/plugins (pstack) | MIT        | `pstack/skills/how/references/explainer-prompt.md`                                      | 72%      |
| `explain/references/explorer-prompt.md`                                 | cursor/plugins (pstack) | MIT        | `pstack/skills/how/references/explorer-prompt.md`                                       | 98%      |
| `explain/references/investigator-prompt.md`                             | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/investigator-prompt.md`                                   | 96%      |
| `explain/references/source-playbook.md`                                 | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/source-playbook.md`                                       | 68%      |
| `explain/references/sources/code-archaeology.md`                        | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/code-archaeology.md`                              | 99%      |
| `explain/references/sources/databricks.md`                              | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/databricks.md`                                    | 100%     |
| `explain/references/sources/datadog.md`                                 | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/datadog.md`                                       | 100%     |
| `explain/references/sources/incident-postmortem.md`                     | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/incident-postmortem.md`                           | 100%     |
| `explain/references/sources/linear.md`                                  | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/linear.md`                                        | 87%      |
| `explain/references/sources/notion.md`                                  | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/notion.md`                                        | 100%     |
| `explain/references/sources/sentry.md`                                  | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/sentry.md`                                        | 98%      |
| `explain/references/sources/slack.md`                                   | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/sources/slack.md`                                         | 95%      |
| `explain/references/synthesizer-prompt.md`                              | cursor/plugins (pstack) | MIT        | `pstack/skills/why/references/synthesizer-prompt.md`                                    | 98%      |
| `explain/references/teach/teach-mode.md`                                | cursor/plugins (pstack) | MIT        | `pstack/skills/teach/SKILL.md`                                                          | 39%      |
| `forge/references/examples/claude-md-testing.md`                        | obra/superpowers        | MIT        | `skills/writing-skills/examples/CLAUDE_MD_TESTING.md`                                   | 94%      |
| `forge/references/governance/governance-details.md`                     | obra/superpowers        | MIT        | `skills/writing-skills/testing-skills-with-subagents.md`                                | 53%      |
| `forge/references/quality/advanced-patterns.md`                         | obra/superpowers        | MIT        | `skills/writing-skills/SKILL.md`                                                        | 77%      |
| `forge/references/quality/cso-guide.md`                                 | obra/superpowers        | MIT        | `skills/writing-skills/SKILL.md`                                                        | 60%      |
| `forge/references/reflect/divergent-reviewer.md`                        | cursor/plugins (pstack) | MIT        | `pstack/skills/reflect/references/divergent-reviewer.md`                                | 78%      |
| `forge/references/reflect/judgment-reviewer.md`                         | cursor/plugins (pstack) | MIT        | `pstack/skills/reflect/references/judgment-reviewer.md`                                 | 72%      |
| `forge/references/reflect/synthesizer.md`                               | cursor/plugins (pstack) | MIT        | `pstack/skills/reflect/references/synthesizer.md`                                       | 75%      |
| `forge/references/reflect/tooling-reviewer.md`                          | cursor/plugins (pstack) | MIT        | `pstack/skills/reflect/references/tooling-reviewer.md`                                  | 82%      |
| `gate/references/preflight.md`                                          | obra/superpowers        | MIT        | `skills/verification-before-completion/SKILL.md`                                        | 26%      |
| `principles/references/boundary-discipline.md`                          | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-boundary-discipline/SKILL.md`                                  | 76%      |
| `principles/references/build-the-lever.md`                              | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-build-the-lever/SKILL.md`                                      | 66%      |
| `principles/references/encode-lessons-in-structure.md`                  | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-encode-lessons-in-structure/SKILL.md`                          | 57%      |
| `principles/references/exhaust-the-design-space.md`                     | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-exhaust-the-design-space/SKILL.md`                             | 39%      |
| `principles/references/experience-first.md`                             | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-experience-first/SKILL.md`                                     | 60%      |
| `principles/references/fix-root-causes.md`                              | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-fix-root-causes/SKILL.md`                                      | 55%      |
| `principles/references/foundational-thinking.md`                        | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-foundational-thinking/SKILL.md`                                | 62%      |
| `principles/references/guard-the-context-window.md`                     | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-guard-the-context-window/SKILL.md`                             | 46%      |
| `principles/references/laziness-protocol.md`                            | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-laziness-protocol/SKILL.md`                                    | 66%      |
| `principles/references/make-operations-idempotent.md`                   | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-make-operations-idempotent/SKILL.md`                           | 74%      |
| `principles/references/migrate-callers-then-delete-legacy-apis.md`      | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-migrate-callers-then-delete-legacy-apis/SKILL.md`              | 55%      |
| `principles/references/minimize-reader-load.md`                         | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-minimize-reader-load/SKILL.md`                                 | 68%      |
| `principles/references/model-the-domain.md`                             | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-model-the-domain/SKILL.md`                                     | 72%      |
| `principles/references/never-block-on-the-human.md`                     | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-never-block-on-the-human/SKILL.md`                             | 54%      |
| `principles/references/outcome-oriented-execution.md`                   | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-outcome-oriented-execution/SKILL.md`                           | 52%      |
| `principles/references/prove-it-works.md`                               | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-prove-it-works/SKILL.md`                                       | 68%      |
| `principles/references/redesign-from-first-principles.md`               | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-redesign-from-first-principles/SKILL.md`                       | 43%      |
| `principles/references/separate-before-serializing-shared-state.md`     | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-separate-before-serializing-shared-state/SKILL.md`             | 61%      |
| `principles/references/sequence-verifiable-units.md`                    | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-sequence-verifiable-units/SKILL.md`                            | 62%      |
| `principles/references/subtract-before-you-add.md`                      | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-subtract-before-you-add/SKILL.md`                              | 59%      |
| `principles/references/type-system-discipline.md`                       | cursor/plugins (pstack) | MIT        | `pstack/skills/principle-type-system-discipline/SKILL.md`                               | 91%      |
| `prototype/references/logic.md`                                         | mattpocock/skills       | MIT        | `skills/engineering/prototype/LOGIC.md`                                                 | 28%      |
| `prototype/references/ui.md`                                            | mattpocock/skills       | MIT        | `skills/engineering/prototype/UI.md`                                                    | 59%      |
| `recall/SKILL.md`                                                       | cursor/plugins (pstack) | MIT        | `pstack/skills/recall/SKILL.md`                                                         | 20%      |
| `review/references/feedback/receiving-code-review.md`                   | obra/superpowers        | MIT        | `skills/receiving-code-review/SKILL.md`                                                 | 67%      |
| `review/references/feedback/requesting-review.md`                       | obra/superpowers        | MIT        | `skills/requesting-code-review/SKILL.md`                                                | 46%      |
| `review/references/impact/impact-mode.md`                               | cursor/plugins (pstack) | MIT        | `pstack/skills/blast-radius/SKILL.md`                                                   | 45%      |
| `review/references/interactive/interactive-review.md`                   | obra/superpowers        | MIT        | `skills/requesting-code-review/code-reviewer.md`                                        | 31%      |
| `scaffold/references/feedback-loops/audit.md`                           | cursor/plugins (pstack) | MIT        | `pstack/skills/maintain-verification-skill/SKILL.md`                                    | 64%      |
| `scaffold/references/feedback-loops/author.md`                          | cursor/plugins (pstack) | MIT        | `pstack/skills/create-verification-skill/SKILL.md`                                      | 78%      |
| `scaffold/references/feedback-loops/feature-map-example/README.md`      | cursor/plugins (pstack) | MIT        | `pstack/skills/create-verification-skill/references/feature-map-example/README.md`      | 100%     |
| `scaffold/references/feedback-loops/feature-map-example/create-note.md` | cursor/plugins (pstack) | MIT        | `pstack/skills/create-verification-skill/references/feature-map-example/create-note.md` | 100%     |
| `scaffold/references/feedback-loops/feature-map-example/search.md`      | cursor/plugins (pstack) | MIT        | `pstack/skills/create-verification-skill/references/feature-map-example/search.md`      | 100%     |
| `tdd/references/tdd-pitfalls.md`                                        | obra/superpowers        | MIT        | `skills/test-driven-development/SKILL.md`                                               | 40%      |
| `triage/references/issues/agent-brief.md`                               | mattpocock/skills       | MIT        | `skills/engineering/triage/AGENT-BRIEF.md`                                              | 98%      |
| `triage/references/issues/out-of-scope.md`                              | mattpocock/skills       | MIT        | `skills/engineering/triage/OUT-OF-SCOPE.md`                                             | 98%      |
| `zealot/references/comments/comment-zealot.md`                          | cursor/plugins (pstack) | MIT        | `pstack/agents/comment-sicko.md`                                                        | 72%      |

## Per-skill provenance summary

| Skill                                                                                                                     | Source                                | License          | Action Taken                                                                                                                                                             |
| ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| intent (was `brainstorming`)                                                                                              | obra/superpowers                      | MIT              | Adapted and extended, renamed                                                                                                                                            |
| review (was `code-review`)                                                                                                | obra/superpowers                      | MIT              | Adapted and extended                                                                                                                                                     |
| execute (was `plan-implementation`; originally `executing-plans`)                                                         | obra/superpowers                      | MIT              | Adapted and extended, renamed                                                                                                                                            |
| gate (branch mode; was `wrap`; originally `finishing-a-dev-branch`)                                                       | obra/superpowers                      | MIT              | Adapted and extended                                                                                                                                                     |
| execute (absorbs swarm; was `subagent-driven-development`; merged from `parallelizing-independent-tasks`)                 | obra/superpowers                      | MIT              | Renamed, merged patterns                                                                                                                                                 |
| diagnose (was `debugging`; originally `systematic-debugging`)                                                             | obra/superpowers                      | MIT              | Adapted and extended, renamed                                                                                                                                            |
| tdd                                                                                                                       | obra/superpowers                      | MIT              | Adapted and extended                                                                                                                                                     |
| git (worktrees reference; was `using-worktrees`)                                                                          | obra/superpowers                      | MIT              | Adapted with cross-platform                                                                                                                                              |
| gate (preflight reference; was `pre-merge-verification`; originally `verification-before-completion`)                     | obra/superpowers                      | MIT              | Adapted and extended, renamed                                                                                                                                            |
| decompose (was `building-plans`; originally `writing-plans`)                                                              | obra/superpowers                      | MIT              | Adapted and extended, renamed                                                                                                                                            |
| forge (skill-authoring mode; was `skill-authoring`, upstream `skill-creator`)                                             | obra/superpowers + anthropics/skills  | MIT + Apache-2.0 | 3-way merge, eval rewritten                                                                                                                                              |
| receiving-feedback (reference)                                                                                            | obra/superpowers                      | MIT              | Adapted as reference doc                                                                                                                                                 |
| forge (agents-authoring eval; was `agents-creator`)                                                                       | anthropics/skills                     | Apache-2.0       | Eval approach inspired, rewritten                                                                                                                                        |
| forge (instructions-authoring eval; was `instructions-creator`)                                                           | anthropics/skills                     | Apache-2.0       | Eval approach inspired, rewritten                                                                                                                                        |
| documentation (was `doc-coauthoring`)                                                                                     | anthropics/skills                     | Apache-2.0       | Adapted, platform-neutral rewrite                                                                                                                                        |
| deepen (refactor-time material; was `improve-codebase-architecture`)                                                      | mattpocock/skills                     | MIT              | Adapted and extended, renamed                                                                                                                                            |
| challenge (interview core, frontier rounds, ❓/➡️ format, sub-agent fact-finding; upstream `grilling`)                    | mattpocock/skills                     | MIT              | Interview core adapted; frontier-rounds model + recommended answers adopted; question format vendored verbatim; preflight/discern are Agentics                           |
| deepen/references/context-format.md                                                                                       | mattpocock/skills (`grill-with-docs`) | MIT              | Vendored verbatim                                                                                                                                                        |
| deepen/references/adr-format.md                                                                                           | mattpocock/skills (`grill-with-docs`) | MIT              | Vendored verbatim                                                                                                                                                        |
| diagnose (Phase 1 + feedback-loop catalog)                                                                                | mattpocock/skills + obra/superpowers  | MIT + MIT        | Phase 1 framing + catalog from mattpocock; broader workflow from obra                                                                                                    |
| triage (AI-attribution rule + role-state-machine for issue-mode)                                                          | mattpocock/skills                     | MIT              | Attribution rule adapted (Agentics wording, trailing footer); role-state-machine adapted                                                                                 |
| spec (solution definition, spec template, User Story form, synthesis-first posture, seams step)                           | mattpocock/skills                     | MIT              | Former two-part shape and template syntax adapted from to-prd/to-spec + to-issues; slicing now belongs to `decompose`; synthesis-first and seams from to-spec            |
| spec (ambiguity/coverage taxonomy, story priority + independent testability, Success Criteria)                            | github/spec-kit                       | MIT              | Concepts adapted from `clarify` and `spec-template`; no files vendored                                                                                                   |
| review (Standards/Spec axes mode)                                                                                         | mattpocock/skills (in-progress)       | MIT              | Two-axis parallel-subagent mode adapted                                                                                                                                  |
| tdd (horizontal-slice anti-pattern)                                                                                       | mattpocock/skills                     | MIT              | Vertical tracer-bullet anti-pattern adapted                                                                                                                              |
| prototype (new skill)                                                                                                     | mattpocock/skills                     | MIT              | LOGIC + UI mode shapes adapted                                                                                                                                           |
| deepen (design-time material; upstream codebase-design)                                                                   | mattpocock/skills                     | MIT              | Adapted to design-time; upstream `design-it-twice.md` merged into `interface-design.md` (dual attribution); testability guidance in `language.md`                        |
| deepen/references/context-map.md (upstream domain-modeling)                                                               | mattpocock/skills                     | MIT              | Adapted; multi-context map material plus the glossary-sharpening moves in `deepen`'s grilling loop                                                                       |
| resolving-merge-conflicts (→ `git` `resolve` mode)                                                                        | mattpocock/skills                     | MIT              | Adapted into the `git` skill as `/git resolve` (`references/merge-conflicts.md`); `gh` cross-reference added                                                             |
| gh/references/pull-requests/body.md (PR body contract)                                                                    | mattpocock/skills + humanlayer/skills | MIT + MIT        | Visual menu vendored close to verbatim; Evidence and Impact rewritten for Agentics                                                                                       |
| handoff (new skill)                                                                                                       | mattpocock/skills (in-progress)       | MIT              | Adapted from in-progress upstream                                                                                                                                        |
| docs (technical-writing reference; upstream `technical-writing`)                                                          | cursor/plugins (pstack)               | MIT              | Four layers + worked example + review checklist vendored, adapted                                                                                                        |
| execute (arena mode; upstream `arena`)                                                                                    | cursor/plugins (pstack)               | MIT              | Bakeoff workflow adapted; diversity re-based on Agentics tiers                                                                                                           |
| execute/references/flow-state.md + Go flow port + frozen flow.sh (upstream `show-me-your-work`)                           | cursor/plugins (pstack)               | MIT              | Append-only TSV rule + log.sh cell hardening adapted into hooks/go/internal/flow; frozen copy in hooks/go/internal/shellwords/testdata/flow.sh                           |
| review + challenge (verdict synthesis; upstream `interrogate`)                                                            | cursor/plugins (pstack)               | MIT              | Consensus weighting, verdict buckets, agreement map adapted                                                                                                              |
| explain/references/epistemics.md (canonical epistemics; upstream `why`)                                                   | cursor/plugins (pstack)               | MIT              | Confidence tiers + coverage map adapted; diagnose, review, and verdict-synthesis link it                                                                                 |
| deepen + intent (design-twice, red flags, scrap trigger; upstream `architect`)                                            | cursor/plugins (pstack)               | MIT              | Red flags vendored, adapted; design-twice floor + scrap trigger adopted                                                                                                  |
| tdd (negative space + evidence report)                                                                                    | cursor/plugins (pstack)               | MIT              | Bad-test/impractical-test rules + evidence report adapted                                                                                                                |
| routing + decompose (bespoke workflow + empirical fork; upstream `figure-it-out`)                                         | cursor/plugins (pstack)               | MIT              | Playbook-as-artifact + experiment-over-question rules adapted                                                                                                            |
| execute, routing, intent, decompose, gate, tdd, diagnose, review (verbatim step discipline; upstream `poteto-mode`)       | cursor/plugins (pstack)               | MIT              | Verbatim todo copying + visible skip: <reason> adapted; canonical rule in forge's hardening checklist                                                                    |
| git (worktree prune; upstream `poteto-mode` worktree-cleanup playbook)                                                    | cursor/plugins (pstack)               | MIT              | Safety-gated prune mode adapted (no Cursor/simulator specifics)                                                                                                          |
| explain (why mode; upstream `why`)                                                                                        | cursor/plugins (pstack)               | MIT              | Seven-category archaeology workflow + all references adapted; dispatch and MCP discovery rewritten for Agentics (Agent semantics, ladders, project charter principle 10) |
| explain (how mode; upstream `how`)                                                                                        | cursor/plugins (pstack)               | MIT              | Modes, complexity gate, output contract, explain-first rule, and judgment buckets adapted; critic panel re-based on lenses + Agentics model ladders                      |
| docs (`unslop` mode + `references/slop-patterns.md`)                                                                      | cursor/plugins (pstack `unslop`)      | MIT              | 31-pattern AI-tell catalog + adding-soul section vendored near-verbatim as a reference; mode wiring and scope guard are Agentics                                         |
| explain teach mode                                                                                                        | cursor/plugins (pstack)               | MIT              | Teaching core adapted; investigation delegates to `explain` (how + why modes), style rules inline                                                                        |
| recall (new skill)                                                                                                        | cursor/plugins (pstack)               | MIT              | Adapted; transcript layout ported Cursor → Claude Code; sweep delegates to `explain` why mode, degrading to git/gh                                                       |
| review impact mode                                                                                                        | cursor/plugins (pstack)               | MIT              | Evidence ladder, one-fact proof, output contract, never-round-up preserved; arena → subagent fan-out; pstack cross-refs removed or repointed                             |
| reflect (→ `forge` `reflect` mode)                                                                                        | cursor/plugins (pstack)               | MIT              | Workflow adapted into `/forge reflect`; four prompt templates vendored with adaptation into `forge/references/reflect/`                                                  |
| zealot (new skill; was `no-comments` + Comment Sicko; adopted as `decomment`)                                             | cursor/plugins (pstack)               | MIT              | Wrapper + finder charter adapted; persona renamed Comment Zealot, embedded charter (not a registered agent)                                                              |
| forge/references/quality/adversarial-pair-pattern.md                                                                      | cursor/plugins (pstack)               | MIT              | Pattern mechanisms distilled from the upstream pair; write-up is Agentics's own                                                                                          |
| principles (new skill)                                                                                                    | cursor/plugins (pstack)               | MIT              | 21 `principle-*` skills merged into one indexed skill (index + one reference per principle); citation discipline adapted from `poteto-mode`                              |
| scaffold feedback-loops (`author` mode from `create-verification-skill`, `audit` mode from `maintain-verification-skill`) | cursor/plugins (pstack)               | MIT              | Two upstream skills merged into one two-mode skill; feature-map example vendored; Drive recipes ported to Claude Code equivalents                                        |
| hillclimb (new skill)                                                                                                     | cursor/plugins (pstack)               | MIT              | Adapted from the poteto-mode hillclimb playbook; Cursor Task semantics rewritten for Agentics                                                                            |
