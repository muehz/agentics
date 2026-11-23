# Arena mode: candidate bakeoff with base selection and grafting

Fan out N parallel attempts at the same task. Read every candidate end to end. Pick the strongest as the base. Graft the
best ideas from the others into it. Then verify the synthesized result. Use arena only when both conditions hold:

- One attempt at a non-trivial artifact could lock in the wrong shape, such as a design sketch, module interface,
  migration strategy, or hard prompt.
- N attempts cost less than it costs to live with the first shape.

Arena takes one task: a single already-decomposed task, an approved brief, or one task of an approved plan that the user
names. It needs the same approval as any other run (`SKILL.md` Step 0). It is not available on the `fix` entry.

Arena is not wave execution. The N dispatches are deliberate duplicates of **one** task. They are not independent tasks,
so the Independence Assessment Checklist does not apply between candidates. Workspace isolation still applies (Phase A,
step 4).

## Cost model

An arena multiplies one task's cost. One run dispatches N candidates, one judge, at most one graft implementer, and one
quality review, and runs one baseline of the verification command in each candidate worktree
([verification-preflight.md](verification-preflight.md)). That is up to N+3 dispatches and N baseline runs, against one
or two dispatches for a normal task.

Resolve the tier of each candidate and judge relative to the current driver, at the moment you dispatch it. Use
[model-ladders.md](../../routing/references/model-ladders.md) for this resolution. Phase C binds the judge. On Copilot
CLI and VS Code, candidates are implementers and resolve by role in that ladder instead (`gpt-6-luna` for a Light or
Standard candidate). Copilot defaults do not change with the driver. On positional surfaces, recompute unpinned defaults
and resolve persistent tier pins against the new driver when it changes. Persistent model pins stay fixed. Record
default, override and scope, requested, actual, and limitation fields for every candidate and judge dispatch, and print
the actual binding only when the provider confirmed it. This is dispatch policy only. It adds no runtime resolver.

For an unavailable unpinned default, skip every effort variant of that model and use the nearest lower available model.
If no lower default is reachable, block that dispatch. On Copilot CLI and VS Code, the unavailable-selection rule in the
canonical ladder's role resolution applies instead. An unavailable explicit model, effort, or tier pin blocks the
affected dispatch, unless the user already authorized a fallback list. If the user authorized a fallback list, try only
the listed alternatives.

Four rules keep the spend deliberate:

- **N defaults to 3, capped at 4.** More candidates need an explicit user ask. Generation-bound work (same prompt, cheap
  to judge) defaults to 2. Each candidate yields one or two grafts, so a fourth candidate adds little.
- **Candidates default to Light; the ceiling is Standard for unpinned defaults.** A Standard candidate needs the user's
  ask or approval in the cost preview. Deep is reserved for the judge when no explicit selection applies. The first
  clear explicit model, effort, or tier instruction for a candidate takes effect at once. It does so even when it
  crosses the default diversity or cost preference. It is not limited to a per-model pin. Diversity spread still points
  downward only for unpinned candidates. N, the cost preview, and the one-rerun budget stay unchanged.
- **The cost preview is part of consent.** Before fan-out, the dispatch preview states the arithmetic in one line:
  `bakeoff cost: N candidates at <tier> + 1 judge at <tier> + 1 graft + 1 review, N baseline runs ≈ <N+3>× a single task`.
  It also records the resolved default and any first-request model, effort, or tier override for each role. Only an
  increase to N or a re-run beyond the budget needs the consent described here.
- **One re-run in total.** A run gets one reframe-and-re-run on the original approval, whichever phase asks for it:
  Phase E's no-common-shape signal or a Phase F finding that Phase A was wrong. Record it as one `pivot arena` row. A
  second need stops the bakeoff and asks the user, even though arena has no other user stop. An unattended run cannot
  answer, so it reports the divergence and ends there. N stays at the default, and unpinned candidates stay at Light.
  Existing explicit model, effort, and tier selections persist within their stated scopes. A task-local selection
  expires with its task.

## Phases

Before launching any candidate, track one entry per arena phase under
[workflow steps](../../routing/references/contracts/workflow-steps.md). The arena runs without user stops.

1. Frame
2. Fan out
3. Cross-judge
4. Pick
5. Graft
6. Verify

### Phase A: frame

The N candidates receive the same prompt, so the prompt is the contract. Get it right before you spawn anything.

1. **State the artifact** each candidate produces.
2. **Derive the rubric.** State what success looks like for _this_ task. Then turn it into 3–6 concrete gradeable
   criteria. Concrete: `Adds a --dry-run flag that skips writes`. Vague: `code is correct`. The rubric is the picker's
   tool in Phase D. Candidates see only the task.
3. **Pick the runners and their diversity axis.** The host's native dispatch spawns same-family agents. On Claude Code,
   Task subagents are Claude-family only. See [surface-dispatch.md](../../routing/references/surface-dispatch.md).
   Candidate diversity comes from **prompts first, tiers second**, not model families. Give each candidate a distinct
   opinionated stance, such as smallest-diff-first, interface-first, or data-model-first. Alternatively, mix Light and
   Standard candidates within the cost model's ceiling. If the user has configured alternate-family CLIs as external
   reviewers, those CLIs can be additional candidate or judge sources. Use only the mechanisms the user already set up.
   Never invent a dispatch path. The same prompt N times is acceptable when the work is generation-bound rather than
   judgment-sensitive. Explicit selections follow the cost model.
4. **Assign output paths.** Each candidate writes to its own git worktree (the `git` skill's `worktrees` mode). Start
   each worktree at the dispatch base, as [isolated-packets.md](isolated-packets.md) describes. Candidates that write to
   the same path share mutable state. Never allow it. A plain directory is no substitute for a worktree, because the
   controller writes code when it copies a candidate's files into the branch afterwards.

### Phase B: fan out

Dispatch the N candidates in one batch, within the surface's concurrency limit. Give each candidate these items:

- the task;
- shared grounding per the skill's dispatch rule (task contract inline, grounding as pointers, standing-orders register
  and open divergence entries verbatim);
- its own output path;
- instructions to produce **both the artifact and a short rationale**.

Include these details for each candidate:

- its default binding;
- its applicable override and scope;
- its requested binding;
- its actual provider binding, when the provider confirmed it;
- any limitation or fallback.

The rationale is mandatory. It names the alternatives the candidate considered and rejected. Without it, the parent
cannot tell whether the candidate's structure is principled or accidental, so Phase E grafting is unreliable.

If a candidate fails to produce output, do these steps:

1. Proceed with N−1.
2. Note the dropout in the synthesis record.

### Phase C: cross-judge

After all candidates complete, dispatch **one read-only cross-judge**. Bind it to the stronger of these two:

- the review tier;
- the next model position above the strongest candidate's actual ranked model.

The current driver's Deep tier is the ordinary arena default. On Copilot CLI and VS Code, the judge is code class. It
resolves by role in the canonical ladder. A candidate's explicit model, effort, or tier selection counts toward that
strongest actual binding. Do not exclude it as an unpinned candidate. At a top clamp, raise effort above the actual
candidate effort where the selected model supports it. Otherwise, disclose the shared or unobservable binding and
recommend an additional reviewer, but do not require one. Apply the first clear judge model, effort, or tier instruction
at once. Keep the judge separate from the candidate work. The judge sees the rubric, the standing-orders register and
open divergence entries verbatim, and the candidates by path label. It scores each criterion per candidate and
recommends a base with rationale.

For example, with a Luna driver, a Light candidate explicitly pinned to Astra/high makes Astra/high the strongest actual
candidate binding. At the top clamp, the judge remains Astra/high, not Astra/low. Dispatch it only after every candidate
has finished. A judge that you spawn while candidates still write sees partial outputs and reports them as dropouts. If
an alternate-family judge is available through user-configured tooling, prefer it. A judge of a different family catches
shared blind spots in same-family candidates.

### Phase D: pick a base

Read every candidate end to end before you pick. If you skim N candidates, you notice only the one that looks most
familiar. Score each candidate against the rubric criterion by criterion, not on holistic feel. Then compare with the
cross-judge. Agreement on the base confirms the pick. If you disagree, one of you is biased or the rubric was ambiguous.
Read both rationales before you decide.

Pick as the base the candidate a future maintainer can extend most easily, with the invariants kept intact. When two
feel tied, prefer the cleaner boundary or the smaller interface. Record the pick, the reason, and the cross-judge's
verdict in a short synthesis note next to the base artifact.

### Phase E: graft

Read each losing candidate once more. Find what deserves to be ported into the base, usually one or two things per
candidate. Write a graft packet. It names the base candidate's worktree, each part to port, and its source candidate.
Dispatch one fresh implementer to fold the grafts in. The controller never grafts by hand (the SKILL's **The controller
rule**). The packet tells the implementer not to paste mechanically: the result has to stay coherent under one mental
model. The graft implementer commits in the base candidate's worktree and returns its head. Phase F integrates and
records that head.

Record what was grafted, from which candidate, and **what was rejected and why**. The rejection notes are the most
useful part: future readers learn from what was considered and dropped, not only from what was kept.

Two convergence signals end the phase early:

- **N candidates converge on the same shape.** This is strong agreement. Note the convergence in the record. Ship the
  consensus shape. No graft is needed.
- **N candidates share no common shape.** Phase A was under-specified. Reframe and re-run within the cost model's one
  re-run. Do not average the divergence instead.

### Phase F: verify

The synthesized artifact clears the same quality review as any other execute output. The arena does not earn a pass. At
a top clamp, a same-model or shared-effort reviewer is still a separate review, and an additional reviewer is only a
recommendation. Verification can find a problem the arena did not catch. If Phase A was wrong, re-frame and re-run, when
the run's one re-run is unspent; otherwise stop and ask. If one candidate caught it and the graft missed it, return to
Phase E; a failed graft counts on the task's failure ladder ([two-strike-stop.md](two-strike-stop.md)). Do not hide the
problem.

Once the review clears, the base candidate's head reaches the branch the isolated way
([isolated-packets.md](isolated-packets.md)). Merge that head into the branch, so the candidate's commits keep their
SHAs and patch-ids. Never use a rebase, a cherry-pick, or a copy of its files. The task's `verify` row uses the isolated
form `base=<dispatch base> head=<graft head> integrated=<sha> patch-id=<id>`. The fields in that form mean this:

- `<dispatch base>` is the commit every candidate's worktree started from.
- `<graft head>` is the commit the graft implementer returned. When Phase E grafted nothing, it is the base candidate's
  own head.
- `integrated=` is the branch commit after the merge.
- `<id>` is the first field of `git diff <base> <head> | git patch-id --stable`. Record it before the merge.

`gate branch` audits the branch against that row. Then remove every candidate worktree as
[isolated-packets.md](isolated-packets.md#cleanup) says.

## Outputs

The arena produces one synthesized artifact and one synthesis note. The note names these items:

- the base and why;
- the grafts (with source candidate);
- the rejections and why;
- any dropouts;
- the convergence signal, if one fired;
- the verification result;
- the integrated commit.

The synthesis note's decisions also land in `state.tsv` as `pivot` rows ([flow-state.md](flow-state.md)), beside the
task's isolated-form `verify` row from Phase F.
