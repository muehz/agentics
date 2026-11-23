# Security lane: fast depth

Apply the repository guidance the review entry point supplies
([Repository guidance](../../SKILL.md#repository-guidance)), including its relevant references. The workflow, threshold
and output below are defaults where that guidance is silent.

Every statement about checks and scanners comes from what this run executed. The examples below describe the default
reading-only run; when checks the repository asks for ran, name them and their results instead.

This file covers the `--fast` depth of the Security lane. Output is inline, and nothing is posted.

| Depth             | Invocation                                      | Runs                                        | Threshold |
| ----------------- | ----------------------------------------------- | ------------------------------------------- | --------- |
| **Fast** _(here)_ | `/agentics:review axes <ref> --security --fast` | One reviewer over 11 high-signal categories | 75        |
| Full              | `/agentics:review axes <ref> --security`        | Six domain reviewers, then batched scorers  | 75        |

Fast depth uses the same rubric and threshold as full depth and reads much less: one reviewer, eleven categories, no
domain checklists, no separate scorer. It answers "is there anything clearly wrong here before I open the PR". It does
not answer any question that starts with "are we sure"; for depth, drop `--fast` ([security-axis.md](security-axis.md)).

Also load [security-principles.md](security-principles.md) and [security-scoring.md](security-scoring.md).

## When it runs

- Pre-PR self-review; `gate` runs this depth.
- A diff whose only Security trigger is a dependency manifest or lockfile, as
  [axes mode](../axes/axes-mode.md#step-5-decide-whether-the-security-axis-runs) picks it.
- Local change inspection, or a caller that needs a fast security signal.

**Dispatch count:** one reviewer. It scores its own candidates and dispatches nothing further.

## Phases

### Phase 1: Scope

Resolve the target to a concrete set of lines:

- **A fixed point** (the usual case): reuse the one `axes` Step 1 pinned, and diff from the merge base with the
  three-dot form, `git diff <fixed-point>...HEAD`. A two-dot diff against the base tip would show the base's own later
  changes as if this branch had reverted them.
- **A path a caller named:** the whole file is in scope.

Record the files, the line ranges, and the full head SHA.

If the resolved diff is empty, do not scan and do not invent findings. Report "nothing to scan", name the resolved base
(and head, when one exists), and stop.

### Phase 2: Pattern sweep

Run one sweep across every category below. Each pattern is a hint, not a rule; confirm each match semantically before
you keep it.

If the user named a category subset (e.g. "auth or secrets"), still sweep every category, and in the report call out the
named categories with their findings or their outcome.

| Category              | High-signal patterns to look for                                                                                                                                                                             |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Hardcoded secrets     | AWS access key prefix `AKIA`/`ASIA`, GitHub token prefixes `ghp_` `gho_` `ghs_` `ghu_` `ghr_`, Stripe `sk_live_`, Slack `xox[abp]-`, JWT-shaped strings, base64 blobs near keywords (`secret`, `key`, `pwd`) |
| SQL injection         | String concatenation or template interpolation directly inside SQL keywords (`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `WHERE`)                                                                                |
| Command injection     | `exec`, `execSync`, `spawn` with `shell: true`, `os.system`, `subprocess.*` with `shell=True`, backticks in shell, `Process.Start` with concatenated arguments                                               |
| Path traversal        | `path.join`, `os.path.join`, `Path.Combine` with user input and no normalization or root containment check                                                                                                   |
| XSS                   | `innerHTML =`, `outerHTML =`, `document.write`, `dangerouslySetInnerHTML`, Vue `v-html`, Angular `bypassSecurityTrust*` with values that originate from input                                                |
| SSRF                  | Outbound HTTP (`fetch`, `axios`, `http.get`, `requests.get`, `HttpClient.GetAsync`) with URL components from user input and no allowlist                                                                     |
| Deserialization       | `pickle.loads`, `yaml.load` (without SafeLoader), `JSON.parse(reviver)` with eval-like reviver, `BinaryFormatter`, `XmlSerializer` with untrusted XML, Java `ObjectInputStream`                              |
| Auth bypass           | Comments like `// TODO auth`, `// skip auth`, recently-removed middleware, route handlers without an authentication or authorization call in the call chain                                                  |
| Crypto misuse         | `Math.random` for tokens, MD5/SHA1 for passwords, ECB mode, hardcoded IVs, deprecated TLS versions                                                                                                           |
| Open redirect         | `res.redirect(req.query.next)` without allowlist                                                                                                                                                             |
| CORS misconfiguration | `Access-Control-Allow-Origin: *` or a reflected origin, combined with `Access-Control-Allow-Credentials: true`                                                                                               |

### Phase 3: Confirm and score

For every match, settle three things:

- **In scope?** On a diff, the changed lines and what they newly reach; on a path, the whole file. Pre-existing lines a
  diff does not touch or worsen are out of scope.
- **Untrusted source?** Input an attacker controls, or a constant or environment value.
- **Controlled?** Parameterization, escaping, an allowlist, or another control on the path.

Score with [security-scoring.md](security-scoring.md) and keep candidates at its threshold (75 by default). A login or
role an attacker needs sets the severity, not the score. Count what you drop; the limits statement reports it.

A candidate that needs more than two more code files to settle (repository instructions and their references do not
count toward the two) scores 50 and is named in the limits statement as unsettled, with full depth as the way to settle
it. Do not start full depth unasked. An unsettled candidate makes the fast result incomplete: the tally line says so and
counts it, and a caller that gives a verdict treats an incomplete lane as an open question, never as a clean result. A
reader cannot tell a path split across helpers from a safe one, so a tally of zeros never stands alone.

### Phase 4: Report

Write inline output, in the finding shape of [security-findings-format.md](security-findings-format.md) shortened to one
line each:

```text
### Security scan (fast)

1. **{shared severity}** ({security severity}) {file}:{lines}: {one-line traced flow} Fix: {one-liner}

Security (fast): {C} Critical, {I} Important, {M} Minor{; incomplete: {U} unsettled, settle with full depth}.

Security limits: read by a model over {the target} for secrets, SQL and command injection, path traversal, XSS, SSRF,
deserialization, auth bypass, crypto misuse, open redirect, and CORS; no domain checklists.
Ran: {checks this run executed and their results, or "no scanner or check"}{; PR checks seen: {name and state of each scanner check `gh pr checks` reported}}.
Not read: {unsettled candidates, files not reached, or "nothing" only when that is true}.
Filter: confidence {threshold}; {K} candidates below it were not reported. For six domain reviewers, drop `--fast`.
```

When nothing clears the threshold, replace the list with "No findings cleared the confidence threshold. That is not a
finding that the target is secure." and keep the tally and the limits statement.

## Exit discipline

- Fast depth posts nothing. A post goes through `post` mode under the skill's [Posting](../../SKILL.md#posting) rule.
- Run additional checks only when repository guidance or the user asks for them and the host permits them. Report checks
  that could not run.
- "Is this safe to merge?" gets the narrower answer this scan can give: the tally, and whether it holds an open Critical
  or Important finding. Say which question was answered. No open Critical or Important means this scan found no blocker,
  not that the merge is safe.

## Composition

Under `--composed`, return the code form of the envelope in
[security-principles.md](security-principles.md#composition-contract-json-envelope) with `mode: "axes-security-fast"`,
its `tally`, and its `limits`. The `limits` string travels with the envelope, because a caller that renders an empty
`findings` array as a clean security result has changed the claim.
