# axes mode: orchestration

Use the repository guidance supplied by the review entry point. Workflow, rubric, filters, and output below are defaults
where that guidance is silent. Preserve its source paths through delegation and synthesis.

Axes mode runs independent sub-agents in parallel, then shows their findings side by side.

| Axis          | Runs                                                                                                           | When                                                                                                         |
| ------------- | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| **Standards** | One sub-agent against documented coding standards                                                              | Always                                                                                                       |
| **Spec**      | One sub-agent against the originating issue, PRD, or spec                                                      | When a spec is found; skipped and reported when not                                                          |
| **Security**  | Six parallel domain reviewers, scored and filtered, or one reviewer over 11 high-signal categories at `--fast` | On `--security`, a security ask, or a diff that touches auth, crypto, secrets, IaC, or a dependency manifest |

These are default lanes. Use repository-defined lanes and scope when supplied. Keep the user's target. Report which
lanes ran or were omitted.

Axes mode accepts `--spec-only`, which runs the Spec lane alone and ends with the same `Lanes:` line. It takes each spec
path named in the invocation. The Standards and Security states on that line are `skipped: --spec-only`, and Step 5 does
not auto-select Security. Gate uses it to produce the per-requirement table when the user skipped the review.

## Step 1: Pin the fixed point

Accept whatever the user supplied: commit SHA, branch name, tag, `main`, `HEAD~5`, etc. Do not substitute a default. If
a caller supplies a wider Security-axis target, a path or a whole repository, record it as given. Then pass it through.
If the user supplied none, ask one question for the fixed point, with a ➡️ recommendation of the base that `git`'s
[base resolver](../../../git/references/base-and-remote.md) gives for the current branch. Do not adopt that base until
the user answers, and do not proceed without a fixed point. An unattended run takes the resolver's base, and stops with
the resolver's question when the base reads `unresolved`.

Pass both commands to every sub-agent:

```bash
git diff <fixed-point>...HEAD          # three-dot: compares against merge-base
git log <fixed-point>..HEAD --oneline  # commits included
```

## Step 2: Identify the spec source

Search in this order:

1. Each spec path named in the invocation. They are read before any other source.
2. Issue and PR references in commit messages, such as `Closes #45` and `#123`. Fetch each with `gh issue view <number>`
   or `gh pr view <number>`.
3. A PRD or spec file under `docs/`, `specs/`, or `.scratch/` that matches the branch name or feature name.
4. If nothing is found, ask one question for the spec's path, with a ➡️ recommendation of the closest-named file under
   `docs/`, or "no spec" when none exists. When the user confirms no spec exists, or the run is unattended, skip the
   Spec sub-agent and note "no spec available" in the final report.

## Step 3: Collect the stated reasons for check weakenings

The Standards sub-agent needs the PR body and the full commit messages. They let it tell an explained check weakening
from an unexplained one ([../check-weakening.md](../check-weakening.md)). Step 2 does not supply them. It fetches a PR
only when a commit message names one, and `--oneline` drops commit bodies.

1. Resolve the current branch's own open PR with `gh pr view --json number,body`. Do this whether or not any commit
   names the PR. When no PR exists, say so. Then carry on without a PR body.
2. Read the full messages for the range: `git log <fixed-point>..HEAD --format=%B`.
3. Find each red commit `<red-sha>` exactly as [../check-weakening.md](../check-weakening.md#red-commit-tests)
   describes, with `<fixed-point>` as `<base>`. A PR body from Step 1 is one of its sources.
4. When a plan exists, read its `## Divergences` section and `agx flow status --folder <folder>`. The Standards
   sub-agent skips a deviation only when a recorded
   [user-authorized divergence](../../../routing/references/user-authorized-divergence.md) covers it, as the
   [recorded divergence filter](../verdict-synthesis.md#dismissal-filters) defines a record. Pass both, so the lane can
   tell a recorded entry from one that reads `unrecorded`.

This does not change which issue, PR, or file the Spec sub-agent reads.

## Step 4: Identify the standards sources

Collect every file in the repo that documents how code should be written. Resolve the glossary, context map, ADR
directory, and standards documents through [repo-conventions.md](../../../routing/references/repo-conventions.md). In a
multi-context repo, read the glossary and ADR directory of each context that the diff touches. Common sources:

- `CLAUDE.md`, `AGENTS.md`, and every standards document the contract resolves
- the resolved glossary, because architectural vocabulary is a standard
- the resolved ADR directory, because architectural decisions are standards
- `.editorconfig`, `eslint.config.*`, `biome.json`, `prettier.config.*`, `tsconfig.json`: machine-enforced, so the
  sub-agent should flag violations but not re-run the tools

## Step 5: Decide whether the Security axis runs

It runs when the user passed `--security`, or when the diff touches any of these:

- authentication or authorization code
- cryptography
- secrets or credential handling
- infrastructure-as-code
- a dependency manifest

Otherwise it does not run, and the report says `Security: not run`. Do not run it silently. Do not skip it silently.
Which one happened is part of the result.

**Then pick the depth.** A diff whose only Security trigger is a dependency manifest or lockfile gets the fast depth,
unless the user passed `--security` or asked for a security review. The eleven fast categories hold no dependency check,
so that one reviewer also loads [dependency-review-guide.md](../security/domains/dependency-review-guide.md) and reviews
the changed manifest and lockfile entries against it: new or renamed packages, install scripts, Git, URL or tarball
sources, and version downgrades. The report names that checklist in its `Read:` line. `--fast` selects one reviewer over
eleven high-signal categories ([security-fast-scan.md](../security/security-fast-scan.md)). Without it, six parallel
domain reviewers run ([security-axis.md](../security/security-axis.md)). Each file sets its own confidence filter. A
plain-language security request with no depth or "fast" wording gets the full depth, the same as no `--fast` flag. An
example is "do a security pass before we cut the release". State the depth, the chosen filter's confidence threshold,
and what it discards in one sentence of the report. A reader cannot tell the depths apart from the findings alone, and
they miss very different amounts.

## Step 6: Spawn the sub-agents in parallel

Before the first dispatch, state the count: one Standards sub-agent, one Spec sub-agent when its source exists, and the
Security lane's own count, which its reference states (one reviewer at the fast depth; six domain reviewers and their
scorers at the full depth). Dispatch through this session's native dispatch path (see
[surface dispatch](../../../routing/references/surface-dispatch.md)). Each dispatch carries the items in routing's
[What every dispatch carries](../../../routing/SKILL.md#what-every-dispatch-carries); every lane is read-only, and its
return contract is the output format of its brief. A lane that returns nothing usable is `failed: no result`. Load the
per-axis prompts from:

- Standards sub-agent: `references/axes/standards-axis.md`
- Spec sub-agent: `references/axes/spec-axis.md`
- Security axis: [`references/security/security-axis.md`](../security/security-axis.md) at full depth, or
  [`references/security/security-fast-scan.md`](../security/security-fast-scan.md) at `--fast`. Each orchestrates its
  own reviewers, scoring, and filter

Include in each sub-agent call:

- The full diff command and commit list (from Step 1).
- Standards sub-agent:
    - The list of standards-source files (from Step 4).
    - The PR body, the full commit messages, and each `<red-sha>` when present (from Step 3), plus the plan's
      `## Divergences` section and the flow status output when a plan exists. With these, the sub-agent can quote a
      stated reason for a check weakening ([../check-weakening.md](../check-weakening.md)) and tell a recorded
      divergence from an unrecorded one.
- Spec sub-agent: the path or fetched contents of each spec (from Step 2).
- Security axis: the target scope. The scope may be wider than a diff when the caller asked for one. For example, `ops`
  collects evidence over a path or a whole repository.

The sub-agents run concurrently and do not share context, so a finding in one axis cannot bias the reviewers in another.

## Step 7: Aggregate and report

Present findings under one heading per axis:

```
## Standards
<standards sub-agent report, verbatim or lightly cleaned>

## Spec
<spec sub-agent report, verbatim or lightly cleaned>

## Security
<security lane report at the chosen depth, including its limits statement>
```

Do **not** merge, rerank, or reconcile findings across axes. The user reads them independently.

When repository guidance defines a per-finding field schema, such as Rule / Evidence / Source, the schema replaces the
lane's own default finding shape. Carry each finding in that schema verbatim, field for field.

Each finding keeps its lane label and carries the shared severity that
[severity.md](../../../routing/references/contracts/severity.md#producer-mapping) maps it to: a Standards `[HARD]` is
Important and `[SOFT]` is Minor; a Spec `[MISSING]` or `[WRONG]` is Important and `[ADDED]` is Informational; a Security
Critical is Critical, High is Important, and Medium and Low are Minor. Axes mode gives no verdict and no merge
recommendation: a consumer such as `gate` reads the tallies. When the Security lane report carries its own
`Merge recommendation` line, drop that line from the `## Security` section; the Security tally replaces it.

Close with a one-line tally per lane, counting open findings by shared severity. Informational findings are listed and
not counted:

> Standards: <I> Important, <M> Minor. Spec: <I> Important. Security ({fast | full}): <C> Critical, <I> Important, <M>
> Minor.

Use these lines for an omitted lane:

- If the Standards sub-agent failed: "Standards: failed: {reason}."
- If the spec sub-agent was skipped: "Spec: skipped — no spec available."
- If the Security axis did not run: "Security: not run. {reason}"

End the aggregate with exactly one `Lanes:` line, after the one-line summary. It is the last line of the report:

    Lanes: standards <state> | spec <state> | security <state>

Each `<state>` is `run`, `skipped: <reason>`, or `failed: <reason>`. The security state adds `(fast)` or `(full)` after
`run`. A lane that was dispatched and returned nothing is `failed: no result`, never `run`. The forms for an omitted
lane are `Standards: failed: <reason>.`, `Spec: skipped — no spec available.`, and `Security: not run. <reason>`. A
report without the `Lanes:` line is incomplete, and no consumer reads it as a full review.

**The security limits statement.** Carry it into the report verbatim. It is the one part of the aggregate that you must
never summarize away. It matters most when the Security axis found nothing. A `## Security` heading with no findings and
no limits statement reads as a clean result that nothing here produced.

## Why the axes stay separate

A change can pass one axis and fail another:

- Code that follows every standard but implements the wrong thing → **Standards pass / Spec fail.**
- Code that does exactly what the issue asked but violates project conventions → **Spec pass / Standards fail.**
- Code that is correct, conventional, and on-spec, and still ships an IDOR → **Standards and Spec pass / Security
  fail.**

Separate reports stop one axis from masking another. They also keep the security result attached to its own limits,
because one aggregate verdict would claim more than any axis measured.
