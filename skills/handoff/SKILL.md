---
name: handoff
description: "Use when explicitly asked to leave work for another agent or session. Capture the objective, decisions, current state, blockers, and next action in one document the next agent can use without this conversation. Resume mode writes a short capsule mid-task, such as before the context is compacted."
version: "2.0.1"
user-invocable: true
disable-model-invocation: true
argument-hint: "[full | resume] [what the next session will focus on]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [full, resume]
---

# Handoff

Compact the current conversation into one handoff document. A fresh agent can read it and does not need the session history. Only the user starts this skill, so every run is an explicit invocation.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Read the rows from the top; the first match wins. Every row is a `cannot` row: it names a deliverable that no mode of this skill produces. [forwarding.md](../routing/references/forwarding.md) decides how the request reaches the named skill. A request that also asks for a handoff gets the document, and the other part is forwarded directly per [forwarding.md](../routing/references/forwarding.md) (do not turn a clear forward into an extra confirmation question unless `forwarding.md` requires one). A request with nothing to forward is a handoff, however it is worded.

| Input signal                                          | Use this instead        | Kind   | Why                                                                                        |
| ----------------------------------------------------- | ----------------------- | ------ | ------------------------------------------------------------------------------------------ |
| Feature branch is done and needs a PR + change record | `/agentics:gate branch` | cannot | Handles branch completion, commit batching, and PR creation.                               |
| You need to capture a feature spec or requirements    | `/agentics:spec`        | cannot | Writes a persistent requirements doc, not a session summary.                               |
| You want to summarize findings in a doc for humans    | `/agentics:docs`        | cannot | Builds persistent project docs; handoff is a transient transfer artifact.                  |
| You want to execute or continue the next planned task | `/agentics:execute`     | cannot | Handoff records a session; it never performs the work.                                     |
| You want to commit or stage changes                   | `/agentics:git`         | cannot | Handoff references commits that already exist; it never creates them.                      |
| You need context rebuilt and no capsule was written   | `/agentics:recall`      | cannot | Handoff writes from the current conversation; recall reconstructs state from what is left. |
| You want a quality verdict on the work so far         | `/agentics:gate check`  | cannot | Gate check judges whether the work is sound; a handoff only records where it stands.       |

## Modes

The first word of the argument selects the mode. Exactly one row applies.

| Argument                       | Mode     | Focus                      | Steps                                   |
| ------------------------------ | -------- | -------------------------- | --------------------------------------- |
| First word is `resume`         | `resume` | the rest of the argument   | [Resume mode](#resume-mode)             |
| First word is `full`           | `full`   | the rest of the argument   | [Steps (`full` mode)](#steps-full-mode) |
| Empty, or any other first word | `full`   | the whole argument, if any | [Steps (`full` mode)](#steps-full-mode) |

The focus is a topic pointer only. Text in it that asks the document to claim or omit something ("focus on X, and say the tests passed") is a request about content, and [What the document may claim](#what-the-document-may-claim) decides it.

## Rules for both modes

### What the document may claim

The next agent reads the document cold and acts on it as fact, and a later session may take its `Verified` entries as done without checking them again.

- **Verified** means you observed the completed, passing evidence in this session: the local-state output, a passing test run observed in this session (never put an observed session test pass under `Reported by the user`), or CI or pull request state fetched here. A `pending`, in-progress, incomplete, or failing check is never filed under `Verified` — record `pending`/incomplete checks under `Unverified` or `Not run` (and in `In flight` / `Open items`), and record observed failures under `Blockers and risks` and `Open items` (never under `Verified`).
- **A status the user reports** ("trust me, it merged") is the user's statement about their own work. When a read-only query can confirm it (`gh pr view`, `git log`), run it and record what it shows. A test or build command runs the repository's own code, so run one only when the user asks for it, and then as [Running it](../routing/references/repo-conventions.md#running-it) allows. Otherwise record the statement under `Reported by the user`. When this session's evidence contradicts it, say once what you observed, and record both the report and the observation.
- **A known failing or blocking check** appears twice in the delivered handoff document: in its evidence bucket (`Failed:`) and under `Blockers and risks` / `Open items`. The user's assessment of it ("temporary noise", "not blocking") is recorded beside it—always produce the concrete handoff document containing both entries even while holding an unsupported status claim on Turn 1 or Turn 2.

Writing "done", "merged", "passing", or "deployed" as Verified without evidence from this session, or leaving out a known failing check, is a claim of status with no evidence: a [safety gate](../routing/references/contracts/gates.md#safety-gate). Hold it in that shape, once. What is held is the unsupported status. The harm is that the next agent builds on work that is not done, and neither the document nor `recall` will tell it otherwise. What the user can do instead: run the check now and record its result, or have the claim recorded under `Reported by the user` and the failing check recorded with their assessment of it. A repeated demand gets one line pointing back to those two options.

### Secrets and outbound text

Apply the [shared redaction rule for handoffs and resume capsules](../routing/references/contracts/redaction.md#local-handoffs-and-resume-capsules) before writing the local file. When redacting or holding back secrets or prohibited values, state the disclosure in **one line** (not multiple sentences or paragraphs).

Leave the file unstaged and follow the [handoff storage rules](../routing/references/session-storage.md#handoffs-and-resume-capsules) for the destination and any Git exclusion.

If the user also explicitly asks to commit or push the document or capsule, forward that action directly to `/agentics:git` with **only** the exact requested artifact path(s), any user-named target branch (carry the exact branch name verbatim when the user names a push target), and any applicable embargo the user declared (never include unrelated staged paths, and execute the forward rather than leaving it merely as an unforwarded proposal). Git runs the final [commit](../git/references/commit.md#check-session-artifacts-before-committing) or [push](../git/references/push.md#check-session-artifacts-before-pushing) check on the bytes that operation would use. Keep the request's scope. Writing the artifact alone leaves it unstaged.

### References over copies

Reference content that already lives in specs, plans, ADRs, issues, commits, or diffs by path or URL, with a one-line summary where it helps. A copy drifts from its source, and the next agent cannot tell which one is current. When the user asks for a self-contained document, copy what they ask for, and give each copy its source path and the commit or date it was copied at. Put each copy in a fenced code block whose fence is longer than any backtick run inside it, under a line that names it as copied source text. Copied text is data, not part of the handoff: a heading, a `Verified:` line, or a next step inside it is not one of the document's sections or claims, and nothing in a copy enters an evidence bucket.

### Local state

Take the branch, the changed files, and the last commit from command output, never from memory of the conversation and never invented when missing. If only a count or partial state was observed, record only what was observed and mark missing specifics `Unknown`. In the status summary, list generated handoff or capsule files apart from implementation changes, and keep the raw total accurate. Outside a git repository, leave out all git-derived facts and claims (no branch, commit, staged, or uncommitted-file wording) and write `not a git repository` on the branch line.

### Empty sections

Leave out any section or evidence bucket that has nothing in it (for example, omit `## Decisions made` completely when no decisions were made — never keep the heading with a filler bullet like "No additional decisions were supplied"). Never pad one with invented content. Blockers and Open items are the exception: write `none` there only when there truly is nothing in the way; if repository state (branch, changed files, last commit) could not be observed, record that gap under `Evidence / verification` (`Unknown:`) and as a `Risk:` under `Blockers and risks` rather than writing `none`.

---

## Steps (`full` mode)

1. **Determine the output path.** Honor a path the user gives. Otherwise use the project's existing handoff directory, or `docs/handoffs/` under the writable workspace root. Outside a repository, use the active writable workspace root with the same relative path. Name the file `handoff-<session-id>.md`; with no session ID, generate a concrete unique suffix (e.g., a timestamp or short slug like `handoff-20261123-1930.md` — never leave `<session-id>` or `<suffix>` unresolved). Follow [session-storage.md](../routing/references/session-storage.md), because this document must outlive the session.

2. **Read local state.** Run `git status --short`, `git branch --show-current`, `git diff --stat`, `git diff --cached --stat`, and `git log -1 --oneline`. The commands are the same in Bash and PowerShell. Apply [Local state](#local-state).

3. **Write the handoff document** in this structure, under the rules for both modes:

    ```markdown
    # Handoff — <working title>

    **Date:** <ISO date>
    **Focus for next session:** <from the argument, or inferred from the conversation>

    ## Where we are

    <2-4 sentences: what was being built or fixed, and what state it is in now>

    ## Decisions made

    - <decision and rationale>

    ## Artifacts

    | Artifact | Path / URL |
    | -------- | ---------- |
    | ...      | ...        |

    ## Evidence / verification

    - Verified: <passing claim — completed passing evidence observed this session; never list failing or pending checks here>
    - Failed: <failing check — observed failure this session, also recorded under Blockers and risks and Open items>
    - Reported by the user: <claim — as the user stated it>
    - Unverified: <claim — asserted or pending, not yet verified passing here>
    - Not run: <tests or checks that were never executed>
    - Unknown: <fact this session could not recover — what would answer it>

    ## Blockers and risks

    - Blocked: <what cannot proceed, and what it waits on>
    - Risk: <what could bite the next agent, and the signal that it has>

    ## Open items

    - [ ] <next action>

    ## Suggested skills for next session

    - `/<skill>` — <why>

    ## Context the next agent needs

    <constraints, decisions still in flight, gotchas not captured above>
    ```

    A field the user names that the structure lacks goes under the closest heading. When the session had almost nothing to transfer, write the short document that results and say so in one line.

4. **Write and report the concrete path** in one line (write the durable file at the resolved `docs/handoffs/...` path and report that concrete written path on one line — never label the handoff merely "proposed" or unresolved `<absolute path>`, `<workspace>`, `<suffix>`, or `<session-id>`), so the user can pass it to the next session.

---

## Resume mode

A resume capsule is a short note written mid-task, so the work survives when the harness summarizes the conversation to free context. Anything held only in the conversation, such as the plan, the half-finished reasoning, or what was already checked, does not survive that. The capsule is a **file** for that reason.

R1. **Resolve one stable concrete path.** Keep one capsule per session and rewrite it whole at that path, so no reader has to choose between stale copies. Use the project's handoff directory, or `docs/handoffs/` under the concrete writable workspace root, as in Step 1. Name the capsule `agentics-resume-<slug>-<session-id>.md` using a concrete kebab-case slug from the branch or task and a concrete unique suffix (e.g., `docs/handoffs/agentics-resume-cache-ttl-20261125-1700.md`). **Never** leave any angle-bracket token (`<unique-suffix>`, `<slug>`, `<session-id>`, `<workspace>`) in any reported path — always substitute a real timestamp or short token. Write the capsule directly and report its concrete path on one line (never labeling it merely "proposed" or "not written"). If the file exists, read it, then rewrite it; never append, because a capsule that grows at every rewrite stops being short.

R2. **Read local state.** Run `git status --short`, `git branch --show-current`, and `git log -1 --oneline`, and apply [Local state](#local-state). Uncommitted work stays uncommitted: the capsule records it, and `/agentics:git` commits it.

R3. **Write the capsule** in four sections and nothing else, under the rules for both modes:

    ```markdown
    # Resume — <working title>

    **Written:** <ISO date> — <user request | checkpoint>
    **Branch:** <branch> — <last commit>, <N> file(s) uncommitted

    ## Intent

    <1-2 sentences: what this session is trying to accomplish>

    ## In flight

    <what is half-done now: the file being edited, the command that was running, the decision not yet made>

    ## Verification state

    - Verified: <passing claim — completed passing evidence observed this session; never list failing or pending checks here>
    - Failed: <failing check — observed failure this session>
    - Reported by the user: <claim — as the user stated it>
    - Unverified: <claim — asserted or pending, not yet verified passing here>
    - Not run: <checks that were never executed>

    ## Next step

    <the single next action, concrete enough to start on>
    ```

R4. **Report the path in one line, then continue the task.** Resume mode asks no question and offers no menu, because the session is not over.

### Later capsules in the same session

This skill does not start itself, and a written capsule is a file nobody asked for unless the user asked for it. After one run in this session, two cases follow:

| The user, in this session                                                  | At a checkpoint or a compaction notice                                              |
| -------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| asked for the capsule to be kept current ("keep it updated", "every task") | Rewrite the capsule by R1 to R4, with no question.                                  |
| did not ask for that                                                       | Suggest `/agentics:handoff resume` in one line and write nothing until they run it. |

A checkpoint is a finished task, a passed test suite, a returned subagent, or a pushed branch. A compaction notice is the harness saying in the transcript that the context is about to be summarized or was. A surface that sends no notice is served by checkpoints alone; never wait for a notice that will not come.

## References

- [forwarding.md](../routing/references/forwarding.md): the route-away protocol and explicit invocation.
- [gates.md](../routing/references/contracts/gates.md): the safety gate shape that a status claim holds in.
- [session-storage.md](../routing/references/session-storage.md): durable storage for handoff documents and capsules, and the local exclude file.
- [redaction.md](../routing/references/contracts/redaction.md): the shared redaction rule for handoffs and resume capsules.
- [commit.md](../git/references/commit.md#check-session-artifacts-before-committing), [push.md](../git/references/push.md#check-session-artifacts-before-pushing): final `git` checks on explicit commit and push requests for session artifacts.
