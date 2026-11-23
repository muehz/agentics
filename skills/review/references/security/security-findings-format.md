# Security finding format

Write a security finding this way in `post` mode's PR comment and in the `axes` Security lane, where repository output
guidance is silent. Only the surrounding document differs. Every statement about checks and scanners comes from what
this run actually executed.

## One finding

The shared severity leads; the security scale follows it in parentheses
([security-principles.md](security-principles.md#severity-taxonomy)).

```text
**{shared severity}** ({security severity}) {brief, specific title}
Category: {OWASP category, e.g. A05:2025: Injection} ({CWE-nnn})
Evidence: {one-line traced flow: source to sink, missing check, hardcoded value, or vulnerable version}
Fix: {one-line concrete remediation}
Risk accepted: {YYYY-MM-DD} by {accepting party}. Source: {ticket, ADR, or decision record, or `Unverified — no artifact cited`}. Annotation only; severity, confidence and the tally unchanged.

{permalink to file and line range with the full SHA}
```

Then a remediation snippet in its own language-tagged fence (see [Remediation snippets](#remediation-snippets)).

In `post` mode the finding is one numbered item of the comment's list, with the
[post output format](../post/post-output-format.md) around it.

Omit the `Risk accepted:` line when no acceptance was claimed. When one was claimed, write the line with `Source:`
filled in. Never leave `Source:` blank, because a blank field reads as an acceptance somebody traced.

## Which findings are reported

| Security severity | Shared severity | Reported                                                        |
| ----------------- | --------------- | --------------------------------------------------------------- |
| Critical          | Critical        | Yes, counted                                                    |
| High              | Important       | Yes, counted                                                    |
| Medium, Low       | Minor           | Yes, counted                                                    |
| Informational     | Informational   | Listed as context in the lane report, never counted or numbered |

Every finding that cleared the threshold is reported at its severity. Where it may not be posted (an unfixed finding on
a public repository, or an embargo), it goes to the user in the session instead
([severity.md](../../../routing/references/contracts/severity.md#rules)).

## Citations

Every finding cites an OWASP category when one applies (`A01:2025: Broken Access Control`), a CWE ID (`CWE-89`,
`CWE-79`, `CWE-798`), and for dependency findings an advisory ID (`GHSA-xxxx-xxxx-xxxx`, `CVE-2024-XXXXX`). Cite one
OWASP category per finding, the closest match.

Permalinks follow the rules for every finding this skill posts: full 40-character SHA, no shell interpolation,
`L{start}-L{end}`, one line of context either side. See [../post/post-output-format.md](../post/post-output-format.md).

## Remediation snippets

Keep each snippet to 3–8 lines. Show the safe alternative, not the vulnerable original. Language-tag the fence. Use
identifiers that match the cited file. Never paste an exploit payload.

## The tally

The `axes` lane and the `--composed` envelope close with a tally of open findings by shared severity, as
[axes mode](../axes/axes-mode.md#step-7-aggregate-and-report) prints it:

```text
Security ({fast | full}): {C} Critical, {I} Important, {M} Minor.
```

No security path gives a verdict or a merge recommendation. A consumer such as `gate` reads the tally
([severity.md](../../../routing/references/contracts/severity.md#rules)).

## The limits statement

**Every security result in the `axes` lane and the envelope carries one, a clean result most of all.** (In `post` mode,
the comment's security coverage line in [../post/post-output-format.md](../post/post-output-format.md) plays this part,
and the dropped count goes to the local report.) Build it from what this run did:

```text
Security limits: read by a model against {the domains or categories read}, over {the target}.
Ran: {the checks or scanners this run executed and their results, or "no scanner or check"}{; PR checks seen: {name and state of each scanner check `gh pr checks` reported}}.
Not read: {domains skipped, files or paths not reached, sinks not traced, or "nothing" only when that is true}.
Filter: confidence {threshold}; {K} candidates below it were not reported.
This lists what one reading found; it does not establish that the target is secure.
```

When the run executed no scanner, say that this run executed none, and add that dependencies and secrets are the weakest
of the domains read, because a scanner normally decides both. Never write that no scanner ran on the repository: CI may
run one this run did not see.

When nothing cleared the threshold, write the result as an outcome, never as a verdict:

```text
Security: no findings cleared the confidence threshold. That is not a finding that the target is secure.
```

followed by the limits statement.
