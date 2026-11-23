# 06. Ship

Close out the work: run the completion gate, do the GitHub mechanics, settle the docs, and leave a capsule for the next
session.

## Run the completion gate

```text
I'm done with this feature branch. Run the gate.
```

Why this phrasing works: "done with this branch" is the completion claim, and [`gate`](../../skills/gate/SKILL.md) is
what makes it true. Its default mode auto-detects from git state (branch completion here), `release` is the pre-deploy
ceremony, and `change` prepares an auditable change record. The gate produces an artifact, not a vibe: what was
verified, what was skipped, and why. A completion-mode run also leaves a verdict row in `state.tsv` saying what it rests
on, plus one row per skipped or not-applicable item carrying that item's reason. The gate writes them through the `flow`
helper to the branch's flow: the work folder that has a `bind` row for the branch or that the branch changed. When no
flow matches, as in a `release` run or any other run on the default branch, the rows go to `unassigned/state.tsv` under
the work root. So do a discard's rows when the flow's folder existed only on the discarded branch. A local merge or a
discard ends with a `done` row, the last row of the run. After a local merge the gate still finds the merged branch's
work folder, because it resolved the folder before the merge and captured the branch name before deleting it. `check`
mode leaves none. Branch mode commits those rows itself when you merge, open a draft PR, or keep the branch, and pushes
them with a draft PR; a discard, `release`, or `change` run leaves them staged for your next commit. Before merging, the
checklist also confirms that `flow status` lists the plan's approval as current and that each task in a plan which ran
through `execute` has a `verify` row.

In branch mode, before the review, the gate folds the open divergences that `execute` appended to the intent, the spec,
and the plan into those artifacts, without asking. It edits the artifacts for every entry whose edit is unambiguous. For
each artifact it edited whose approval was current, it writes a new approval that cites the earlier one. It runs no
reviewer on those edits. An entry whose edit is ambiguous stays open and is reported. Then the gate runs the docs
refresh and an axes review on its own, once tests pass, and shows their findings before it offers merge, draft PR, keep,
or discard. Each finding gets a severity. Important covers a Critical or High security issue, a hard standards
violation, a missing or wrong requirement, a controller-audit finding, and a red-commit finding: a bug fix's committed
failing test that is missing from the branch or was changed afterwards. Everything else is a Nit or informational. The
gate reports the tally, such as `1 Important, 2 Nit open`, in chat only; it never enters the PR body. Nothing halts on a
finding alone: for each open Important finding that is not record drift the gate asks one question, fix it, accept it
with a reason, or defer it with an issue link, and it proposes a destination once you have answered. Record drift is a
Standards or Spec finding in documentation and work-folder files, or a controller-audit finding whose commit changed
only documentation or work-folder files: the gate resolves it itself and reports what it did. Skill text and eval banks
count as documentation. A file documenting a hook's safety rules, exceptions, or limits, a workflow source, an agent
definition, and test fixtures do not. Security and red-commit findings are never record drift. A controller-audit
finding whose commit changed code, a hook, or a test still gets the question. The gate edits a documentation file only
when the branch changed it and you did not skip the docs refresh; otherwise it records the finding as accepted. An
accepted or deferred finding you decided stays in the tally, and so does a finding the gate fixed or accepted as record
drift, until a review that saw the fix no longer reports it. Both the docs refresh and the review are process gates: say
`--no-docs` or `--no-review` to skip either, and the gate accepts it in one line that names what goes unverified; the
skip is recorded, not hidden. On a security-sensitive diff, `--no-review` still runs the fast security scan. After it
opens a draft PR, the gate checks back once about 10 minutes later for CI and review-bot signal, and says so in one line
you can cancel.

## GitHub mechanics

```text
Open a draft PR for this branch: proper description, linked issue.
```

```text
Triage the PR in one pass: failing checks, review comments, bot findings.
```

Why this phrasing works: any GitHub operation routes to [`gh`](../../skills/gh/SKILL.md), MCP-first with `gh` CLI
fallback; it detects what your host exposes. "One pass" is [`triage`](../../skills/triage/SKILL.md)'s shape: it
orchestrates everything the PR needs in a single sweep. Know its posting posture going in. Replies to bot findings post
automatically; replies to human reviewers are drafted for your approval, never auto-posted.

The draft PR's description follows `gh`'s [PR body contract](../../skills/gh/references/pull-requests/body.md): a
Summary with the smallest useful visual when the change has a shape to show, Evidence from what actually ran (the gate's
test run when the gate opens it; its audit status lines stay in chat), and Impact stating what a revert undoes and what
could break. If your repository has a PR template, its headings win; the contract fills them and adds none. When a
review changes the PR's scope, ask for the body to be refreshed so a squash merge's commit message describes the final
diff.

## Settle the docs

```text
Update every doc this change touched, and flag any claim you couldn't verify against the code.
```

Why this phrasing works: "every doc this change touched" gives [`docs`](../../skills/docs/SKILL.md) a reconcile target
rather than a writing prompt, and "flag any claim you couldn't verify" invokes its evidence-backed posture. Drifted docs
get fixed against the source of truth, not rewritten from memory. Bonus mode: "unslop this README" strips AI tells from
prose without touching the facts.

## Leave a capsule

Invoke the user-only `handoff` skill with your host's explicit syntax:

| Surface                                   | Invocation          |
| ----------------------------------------- | ------------------- |
| Claude Code, Copilot CLI, or VS Code Chat | `/agentics:handoff` |
| Codex                                     | `$agentics:handoff` |

Why this works: [`handoff`](../../skills/handoff/SKILL.md) compacts the session into a handoff document: what happened,
what's in flight, what's next. It's user-invoked by design. Agentics suggests it at session end but never runs it for
you, because you decide when a session is over.

A long session does not always reach the end intact. When the harness says the context is about to be summarized, the
conversation loses detail while the work keeps going:

| Surface                                   | Invocation                 |
| ----------------------------------------- | -------------------------- |
| Claude Code, Copilot CLI, or VS Code Chat | `/agentics:handoff resume` |
| Codex                                     | `$agentics:handoff resume` |

Why this phrasing works: `resume` mode writes a short capsule to a file — intent, what is in flight, what is verified,
the next step — because a note kept in the conversation is summarized along with everything else. It takes seconds, it
does not end the session, and `recall` reads it as authoritative afterwards.

**Pitfall:** don't end a working session without `handoff`. The `recall` skill can rebuild context from transcripts and
the shared record, but that's the expensive fallback, mining and verification every time. The capsule is cheap, exact,
and written while the context is still loaded. Thirty seconds now, or several minutes at every future session start.

That ends the main path. Once the change is live, incidents, runbooks, and metric work are on the
[Operate](side-operate.md) side page.

Next: the [reference pages](README.md#reference)
