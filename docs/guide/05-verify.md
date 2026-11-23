# 05. Verify

Prove the work is correct before calling it done. The verification skills differ in what they check and what counts as
proof. Pick by the question you're asking, and prefer the ones that run real code over the ones that read it.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Review the diff

```text
Review this branch against main for bugs, security issues, and logic errors. Interactive: I want the checklist and a review verdict.
```

Why this phrasing works: naming the base ("against main") scopes the diff, and "interactive" picks
[`review`](../../skills/review/SKILL.md)'s sit-with-me mode, a checklist plus a review verdict. On a PR, "review it and
post the findings" uses post mode instead. "Run the standards and spec axes in parallel" fans out axes mode.

## Look beyond the diff

```text
What could this change break outside the diff? Name the one fact that makes it safe, and prove it by running code.
```

Why this phrasing works: this is [`review` impact mode](../../skills/review/references/impact/impact-mode.md)'s contract
restated as a prompt: impact beyond the visible change, one load-bearing safety fact, and proof by execution rather than
write-up. Anything it cannot prove stays marked unproven, which is the honest answer you want before shipping.

## When something fails, root-cause first

```text
The integration test started failing on this branch. Diagnose it: root cause before any fix.
```

Why this phrasing works: "root cause before any fix" holds [`diagnose`](../../skills/diagnose/SKILL.md) to its own
discipline and stops the reflexive patch-the-symptom response. Any bug, regression, or unexpected behavior found during
verification takes this detour before the fix lands.

## Security has its own pass

```text
Quick security scan of this change: secrets, injection, authz.
```

Why this phrasing works: "quick scan" selects [`review`](../../skills/review/SKILL.md)'s Security lane at its fast
depth, which reports high-confidence patterns only. Ask for the deep Security lane when you want six domain reviewers
instead. For a design with no code yet, ask [`threat-model`](../../skills/threat-model/SKILL.md) for one. Naming the
classes you care about focuses the pass.

Every security result here reports what one reading found. Nothing in the suite runs a scanner, so a clean result names
its limits and never claims the code is secure. Keep your own SAST, dependency, secret, and IaC scanners in CI. This
pass does not replace them.

## Checkpoint mid-work, second-guess at the end

```text
Before the next step, run a mid-work gate check on the migration output.
```

Why this phrasing works: [`gate`](../../skills/gate/SKILL.md)'s `check` mode is the explicit mid-work checkpoint. It is
cheap, it runs inline, and it catches a flawed intermediate before it propagates. The full completion gates belong to
[06. Ship](06-ship.md).

Zealot loads when you ask for a comment or test audit in plain words, or when you type `/agentics:zealot`; Agentics
never runs it unprompted during other work. It has two modes. `/agentics:zealot comments` strips low-value code comments
from a named file set or diff and also includes current working-tree changes in its default scope; it does not require a
commit. `/agentics:zealot tests` finds tests that prove nothing and confirms each one by running a behavior-changing
mutant in a disposable worktree, then hands the remove-or-replace work to `intent`. With no mode word, it infers the
mode from the conversation or asks which one you mean.

**Pitfall:** "does this look right?" buys you an opinion, not verification. Ask for the proof: a gate that runs the
checks, a blast-radius fact proven by execution, a review with a verdict. Agentics will refuse to report unverified work
as done. Phrase your verification asks so that refusal never needs to happen.

Next: [06. Ship](06-ship.md)
