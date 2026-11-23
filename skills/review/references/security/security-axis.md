# Security axis: orchestration

Apply the repository guidance the review entry point supplies
([Repository guidance](../../SKILL.md#repository-guidance)), including its relevant references. The workflow, threshold
and output below are defaults where that guidance is silent. Pass it to reviewers and scorers.

This file covers the Security lane of `axes` mode, at two depths.

| Depth    | Invocation                                      | Runs                                                | Threshold |
| -------- | ----------------------------------------------- | --------------------------------------------------- | --------- |
| Fast     | `/agentics:review axes <ref> --security --fast` | One reviewer over 11 high-signal categories         | 75        |
| **Full** | `/agentics:review axes <ref> --security`        | Six parallel domain reviewers, then batched scorers | 75        |

[Axes mode](../axes/axes-mode.md#step-5-decide-whether-the-security-axis-runs) picks the depth. **Fast depth has its own
file, [security-fast-scan.md](security-fast-scan.md). Load that instead of this one and stop here.** The rest of this
page is full depth: six domain reviewers over one target, their candidates scored, filtered, and reported locally.

The depths share one rubric and one threshold; they differ in how much they read. `post` mode's single security subagent
is the proportionate pass, fast depth the pre-PR one, and full depth the deep one.

Load [security-principles.md](security-principles.md), [security-scoring.md](security-scoring.md), and
[security-findings-format.md](security-findings-format.md) alongside this file.

**This lane posts nothing.** Security findings reach a PR through `post` mode's subagent #8, or through the `--composed`
envelope a caller chooses to surface.

## Step 1: Pin the scope

`axes` Step 1 already pinned the fixed point. Reuse it:

```bash
git diff <fixed-point>...HEAD          # three-dot: compares against the merge base
git log <fixed-point>..HEAD --oneline  # commits included
```

A caller may widen the scope past a diff: `ops` collects evidence over a path or a whole repository. Accept the wider
target and record it, because the limits statement names what was read. Reachability on a wider target follows
[security-scoring.md](security-scoring.md#scope-for-reachability).

## Step 2: Context and change summary (the lead)

The lead does this itself, with no dispatch. Collect:

- **Context paths** (paths only, no contents): a root `AGENTS.md`, `CLAUDE.md` or `SECURITY.md`; any instruction file in
  a touched directory; `.security/`, `threat-models/` or ADR directories that may apply; package manifests for the
  changed languages (`package.json`, `requirements.txt`, `*.csproj`, `go.mod`, `Cargo.toml`, `pom.xml`). For a PR
  target, read these from the base revision, as [Repository guidance](../../SKILL.md#repository-guidance) says.
- **Both SHAs** in full 40-character form, recorded now so that permalinks match the revision the reviewers saw.
- **The changed files** with line counts, and a one-paragraph plain-language summary.
- **The entry points** the change introduces: new routes, public APIs, shell calls, SQL, auth flows, secrets, IaC
  resources, container images.

## Dispatch count

State the count before the first dispatch, as part of the count axes mode states:

> Security (full): 6 domain reviewers, then one scorer per domain that returns candidates (6 at most while no domain
> returns more than 20; one more per further 20 in a domain).

Once the reviewers return, state the actual scorer count before scoring.

Reviewers and scorers resolve their tier as [Step 0](../../SKILL.md#step-0-preflight) says and dispatch through this
session's native path ([surface dispatch](../../../routing/references/surface-dispatch.md)). No batch exceeds the
surface's [concurrency limit](../../../routing/references/surface-dispatch.md#concurrency-and-overlap), counted with the
other lanes still running; the rest queue. No reviewer or scorer dispatches further subagents.

## Step 3: Six parallel domain reviewers

The six run concurrently and share no context, so one domain's finding cannot bias another.

| Agent  | Domain                         | Checklist                                                                                                                            |
| ------ | ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| **#1** | OWASP Top 10 / Injection       | [domains/owasp-top10-checklist.md](domains/owasp-top10-checklist.md), [domains/injection-patterns.md](domains/injection-patterns.md) |
| **#2** | Authentication & Authorization | [domains/auth-authz-checklist.md](domains/auth-authz-checklist.md)                                                                   |
| **#3** | Secrets & Credentials          | [domains/secrets-detection-patterns.md](domains/secrets-detection-patterns.md)                                                       |
| **#4** | Dependency / Supply Chain      | [domains/dependency-review-guide.md](domains/dependency-review-guide.md)                                                             |
| **#5** | Infrastructure / IaC           | [domains/iac-security-checklist.md](domains/iac-security-checklist.md)                                                               |
| **#6** | Data Flow & Trust Boundaries   | [security-principles.md](security-principles.md)                                                                                     |

Each dispatch carries routing's [What every dispatch carries](../../../routing/SKILL.md#what-every-dispatch-carries).
The write boundary is read-only. Tell every reviewer:

- Read the target plus directly relevant callers and sinks; on a diff, the changed files.
- Apply [security-principles.md](security-principles.md), which wins over the checklist where they conflict.
- On a diff, do not flag a pre-existing issue unless the change worsens it.
- Trace the data flow: state the source, the path, and the sink for any injection or exposure finding.
- Cite each candidate with `file:line_start-line_end` and give it a security severity.
- Do not score and do not filter; the scorer does both.

The return contract: a list of candidates (possibly empty), plus **what you could not check**: a domain you could not
cover, a manifest you could not resolve, a sink you could not trace. A gap silently dropped becomes a clean result
nobody earned. A reviewer that returns neither is `failed: no result` for its domain, and the limits statement names the
domain as not read.

## Step 4: Score in batches

Send each domain's candidates to one scorer, up to 20 a dispatch, with the change summary, the context paths, the
repository criteria, and [security-scoring.md](security-scoring.md). The scorer returns one score and a one-line
justification per candidate. It verifies that the source-to-sink path is real, that the cited line is in scope, and that
no suppression item applies. Scorers merge nothing across domains; the lead merges a defect two domains both report into
one finding that keeps the higher severity and cites both.

## Step 5: Filter

Keep candidates at the threshold in [security-scoring.md](security-scoring.md#threshold) (75 by default, or the
repository's) and drop the rest. Count what was dropped; the limits statement reports it.

## Step 6: Report

Write the lane report under `## Security` in the `axes` aggregate, in the format from
[security-findings-format.md](security-findings-format.md):

1. The findings, worst shared severity first, each with its security severity beside it.
2. Informational context, if any, unnumbered.
3. The lane's tally line.
4. **The limits statement.** Mandatory: what was read, what this run ran, what was not read (every gap Step 3 returned),
   the threshold, and the number Step 5 dropped.

No verdict and no merge recommendation: axes mode's tally is what consumers read. Write a clean result as an outcome,
never as `Pass`, `Clean`, or `Secure`.

## Composition

When the caller passes `--composed`, return the code form of the envelope in
[security-principles.md](security-principles.md#composition-contract-json-envelope) with `mode: "axes-security"` instead
of the prose report, then stop. The caller surfaces it. `ops` uses this path so that evidence collected during a
response is never posted.

## Notes

- Default to code reading. Run builds, tests, typechecks, or scanners the repository asks for when they are available
  within the host's permissions; otherwise name the limitation. The limits statement describes what actually ran.
- Use `gh` for GitHub interactions, not web fetch.
