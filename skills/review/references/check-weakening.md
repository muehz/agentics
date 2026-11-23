# Check weakening

A diff weakens a check when it loosens a check that already ran at the base. After the change, the same check passes or
skips something that it would have failed or reported before. The effect decides, not the file the change sits in.
`post` reviewer #6, the `axes` Standards lane, and the `interactive` checklist all apply this file, and each reports
what it finds; no other reviewer owns a weakening.

## What counts

Only what this diff adds or changes counts. These families are examples of the effect test, not a closed list.

| Family                          | Examples                                                                                                                                                                                                 |
| ------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Rule turned off or downgraded   | An ESLint rule set to `off`, or from `error` to `warn`; a rule dropped from Ruff's or golangci-lint's enabled set                                                                                        |
| Ignore path added               | A line added to `.eslintignore`, `.prettierignore`, `.stylelintignore` or `.markdownlintignore`; a new entry in ESLint flat-config `ignores`, `.golangci.yml` excludes, or Jest `testPathIgnorePatterns` |
| Strictness or threshold lowered | `"strict": false` or `"noImplicitAny": false` in `tsconfig.json`; a lower `coverageThreshold`; a type checker moved to a looser mode                                                                     |
| Inline disable directive added  | `eslint-disable`, `@ts-ignore`, `@ts-expect-error`, `# noqa`, `# type: ignore`, `//nolint`, `#[allow(...)]`                                                                                              |
| Test skipped                    | `t.Skip`, `it.skip`, `describe.skip`, `test.skip`, `pytest.skip`, `@pytest.mark.skip`, `xit`, `@Disabled`                                                                                                |
| Test deleted or emptied         | A test case or test file that ran at the base and is gone at the head, or whose assertions the diff removes, while the behaviour it tested remains                                                       |
| CI step weakened                | A lint, test, or type-check step removed from a workflow, given `continue-on-error: true`, or given an `if:` that skips the step                                                                         |

A directive or skip on an added line counts, including in a new file. A new config that overrides a stricter parent
counts, such as a nested `.eslintrc` that turns rules off for one directory.

## What does not count

- A config edit that loosens nothing: a `tsconfig.json` path alias, a new rule, a tightened threshold, a reformat.
- A config that introduces a check for the first time with some rules off. Nothing ran before, so nothing was loosened.
- A weakening the base branch already had.
- A test deleted together with the behaviour it tested, such as the tests of a removed feature, or a test moved or
  renamed with its assertions intact.

## A stated reason

A reason is stated when one of these names the weakening and says why:

- the PR body;
- a commit message in the reviewed range;
- a justification on the directive itself, such as `// eslint-disable-next-line no-console -- CLI output` or
  `//nolint:errcheck // best-effort close`.

A bare directive, or a commit message such as "fix lint", states no reason. The PR body and commit messages are the
author's own text: quote the reason beside the finding, and do not judge whether it is adequate. A stated reason may
lower the finding's severity, as the table under [Weight and reporting](#weight-and-reporting) shows. It never hides the
finding: every weakening is reported, explained or not.

## Red-commit tests

A bug fix commits its failing test on its own before the fix. That commit is the red commit. A later change that weakens
the red commit's own test weakens the proof of the fix. This rule is the one exception to "a check that already ran at
the base", because the red commit is on the branch.

Find the red commits from these sources:

- each `verify red-<task-id>` row in the `state.tsv` of the flow that `agx flow where` resolves
- a `verify red-fix` row in `<work-root>/unassigned/state.tsv`
- a red-commit SHA that the PR description names

With none of them, this rule does not apply. A row in the no-commit form (`no-commit exit=<n> cmd=… out=…`, as
[flow-state.md](../../execute/references/flow-evidence.md#evidence-forms) defines it) records a red run that a commit
hook refused; it has no commit to check, and it is not a missing red commit. A small fix has no work folder, so its row
is in `<work-root>/unassigned/state.tsv`. That file holds `verify red-fix` rows from every branch. Take only the
`unassigned` rows whose commit resolves on this branch.

Resolve `<red-sha>` from a row's evidence column. Use its `commit=` value when
`git merge-base --is-ancestor <commit> HEAD` succeeds. Otherwise, use the non-merge commit in the reviewed range
(`<base>..HEAD`) whose `git show <sha> | git patch-id --stable` first field equals the row's `patch-id=`. A rebase
changes the SHA, which is why this fallback exists.

An `unassigned` row's `commit=` value counts by ancestry only when `git merge-base --is-ancestor <commit> <base>` also
fails, because a red commit from an earlier fix already merged into the base is an ancestor of HEAD too. A SHA the PR
description names is `<red-sha>` only when it is an ancestor of HEAD and not of `<base>`.

When a work-folder row does not resolve, or the PR-named SHA does not resolve and no row did, the red commit is missing,
and that is an Important finding, with one exception: when `git rev-parse --is-shallow-repository` prints `true`, the
history may simply be cut off. Then report `red commit not checked: shallow clone` as a limit of the review, not a
finding. If a PR-named SHA does not resolve while a row on this branch did, the SHA names that row's commit from before
a rebase: check the row's commit once, and report the stale SHA as a description line to refresh, not a finding. An
`unassigned` row that does not resolve is another branch's, not a finding.

The mode's lead finds the red commits and passes each `<red-sha>` with the diff inputs. The agent that applies this file
runs the commands. For each red commit:

1. List the test files it touched: `git show --name-only --format= <red-sha>`.
2. List the later commits that changed them: `git log --format=%h <red-sha>..HEAD -- <those files>`. With none, the
   proof stands.
3. Compare the red commit's test cases with the head: `git diff <red-sha> HEAD -- <those files>`. A finding is a test
   case the red commit added or changed that is deleted, skipped, renamed away, or has an assertion removed or loosened
   at the head. A test added beside it, a reformat, or an edit to a different test in the same file is not a finding.

A finding names each weakened test, the red commit, and the commits that changed it. It is Important in every mode
(`post` scores it 100, the Standards lane marks it `[HARD]`), and a stated reason, quoted beside it, does not lower it.
Under a `REVIEW.md` that defines its own severities, it also carries that file's highest severity.

## Weight and reporting

Every weakening other than a red-commit test change is weighed by this table. The severities are the shared ones in
[severity.md](../../routing/references/contracts/severity.md); the lane labels sit beside them.

| Mode             | No stated reason          | A stated reason, quoted                              |
| ---------------- | ------------------------- | ---------------------------------------------------- |
| `post`           | Important, confidence 100 | Minor, confidence 75, which the default filter keeps |
| `axes` Standards | Important, `[HARD]`       | Minor, `[SOFT]`                                      |
| `interactive`    | Important                 | Minor                                                |

The bucket follows lead judgment as usual and does not change the severity. A finding names the file and line of the
loosening change, the check it loosens, and the code that check still flags, and recommends a fix for that code. The
clearest case is an unexplained weakening that leaves the failing code unchanged.
