# Scaffold reference: REVIEW.md

## Overview

`REVIEW.md` at the repository root tells a reviewer, human or agent, which passes to run, how to grade findings, and
which paths to skip. A review agent reads it before any repository code-review skill. It wins where the two disagree. A
code-review skill can still hold the review workflow. `REVIEW.md` holds the passes, severities, and skips.

Offer this optional starter only through init, as a selectable row in init's menu, and write it only after its exact
text is shown alone and the user says yes ([init.md](../init.md#phase-2-generate-show-and-confirm) step 3). Select All
never selects it. Audit inspects and reports. It never writes the file. Preserve an existing `REVIEW.md` as authored.
Offer no replacement for it.

## Questions to ask

Ask these in the same ❓ round as the other selected files. Give each a ➡️ recommendation drawn from the repository.
Skip a question the repository already answers.

- **Generated paths:** directories and files a tool generates. Recommend what the repository's generator configuration,
  build output, and ignore files show.
- **Lockfiles:** the dependency lockfiles the repository commits.
- **Checks CI enforces:** the formatters, linters, and type checkers CI runs, so review leaves their rules to CI.

## Starter

```markdown
# Review guidance

Read this file before reviewing a change in this repository. Where it disagrees with other review instructions, this
file wins.

## Passes

Run each pass over the whole change.

1. **Bugs:** logic errors, unhandled failures, missed edge cases, and regressions in behavior the change touches.
2. **Security:** injection, authentication and authorization gaps, exposed secrets, unsafe input handling, and risky
   dependency changes.
3. **Compliance:** the change does what its `spec.md` and `plan.md` say, when it has them, and follows the conventions
   in `AGENTS.md`.

## Severity

- **Important:** a correctness defect, a security problem, a requirement gap (something the spec or plan requires that
  the change does not do), or a contract break (a change to an interface, data format, or documented behavior that
  callers rely on). Report every Important finding.
- **Nit:** style, naming, and optional refactors. Show at most five, most useful first, and give the count of the rest.

## Skip

Do not review these paths or rules.

- Generated paths: {generated directories and files, for example `dist/`}
- Lockfiles: {committed lockfiles, for example `package-lock.json`}
- Anything CI enforces: {the checks CI runs, for example formatting and lint rules}
```

Keep the Passes and Severity sections as written unless the owner changes them. Fill every Skip placeholder from the
answers. Drop a Skip line the repository has nothing for. Do not invent paths. The file's content appears in full in the
preview before any write. A declined preview writes nothing.

## Conformance check (init selection and audit evidence)

`REVIEW.md` is optional. An absent file is `inapplicable` with `Optional — not configured` evidence in audit and an
optional offer in init, never a compliance failure. An existing file conforms when it is readable Markdown at the
repository root. Do not require the starter's headings, severity names, or nit limit. Report a structural problem with
its path and reason.
