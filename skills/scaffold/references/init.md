# Init: create the selected starter files

Init runs when [mode selection](../SKILL.md#mode-selection) picks it: the user typed `init`, asked in plain words to
create, add, set up or fix starter files, or said yes to the readiness audit's offer. It offers only gaps: missing
files, non-conformant files, and contextual additions. It never regenerates an existing file wholesale. It preserves
author content and leaves unrelated sections and files intact.
[Previews and confirmation](../SKILL.md#previews-and-confirmation) decides what is shown before each write.

## Phase 1: scan and select

**Step 1: scan.** When init starts from a yes to the readiness audit's offer, take the audit's scan and gaps as this
step's result and do not scan again. Otherwise, check each [file catalog](#file-catalog) entry against the
[conformance criteria](#conformance-criteria). Never assume all files are missing. GitHub accepts `README.md`,
`CONTRIBUTING.md`, `CODEOWNERS`, `GOVERNANCE.md`, and `SUPPORT.md` at `.github/`, the repo root, or `docs/`. Check all
three locations before you mark ❌, apply the criteria to whatever is found, and report the detected path. Find the pull
request template through [repo-conventions.md](../../routing/references/repo-conventions.md) (any letter case, in
`.github/`, the root, or `docs/`, or a pointer), report the path found, and never create a second template beside an
existing one. Resolve the glossary, context map, and ADR directory through the same contract. If any of them is
ambiguous, conflicting, or in a custom location discovery cannot find, show the `AGENTS.md` row as selectable so its
pointer offer ([agents-md.md](community/agents-md.md)) can run. If an existing `AGENTS.md` lacks the Compaction,
Repeated corrections, or User-authorized exceptions subsection, show the row as selectable so that offer can run. The
row keeps its conformance status.

Also inspect the applicable protections through [protections.md](protections.md), and planning protection through
[planning-protection.md](planning-protection.md). Keep each gap list separate from the file catalog. Preserve suitable
existing scans, hooks and remote rules. Resolve the active hook manager before you offer an integration. An inaccessible
setting or unresolved manager is a blocked or unverified item with its prerequisite, not an invitation to replace it.

**Optional review skill.** Discover `code-review/SKILL.md` under `.github/skills`, `.claude/skills`, and
`.agents/skills`, in that order. Use the first existing match and report its path. Flag an unreadable match rather than
fall through. An absent skill is `Optional — not configured`, not ❌ or a compliance failure. An existing valid skill is
✅ whatever its headings or metadata. See [review skill authoring](community/code-review.md).

**Optional `REVIEW.md`.** Check the repository root for `REVIEW.md`. An absent file is `Optional — not configured`, not
❌. An existing `REVIEW.md` is ✅ whatever its headings, and init offers no replacement for it. See the
[REVIEW.md starter](community/review-md.md).

**Step 2: present one menu.** Report the scan results before the menu (`✅`, `❌`, `⚠️`, `—`; use `?` only when a file's
existence is genuinely unknown), showing existing conformant files as `✅` with no `[ ]` checkbox (kept untouched,
unselectable) and every applicable missing file as `[ ] ❌`. The menu has four sections, and the user answers them in
one reply:

1. The catalog rows, with **Select All**, which selects only ❌ and ⚠️ catalog rows.
2. The optional additions: the code-review skill and `REVIEW.md`, when absent.
3. The applicable missing protections, as [the protection selection](protections.md#explicit-init-protection-selection)
   lists them, each with the existing protection it preserves and whether its next step is a local preview or a remote
   proposal.
4. Planning protection, when it applies: parent recording, the ignore block, and native validation, each selectable on
   its own.

Select All never selects sections 2 to 4. Items the user named in the request arrive preselected. A request that names
its files ("add a CODEOWNERS") gets only those rows and their related offers, with no full menu. A declined or
unselected item changes nothing, and an unresolved prerequisite on one item never blocks the others.

```text
📋 Repository Scaffold — Select files to generate:

  Status    File                         Path              Reason
  ──────    ────                         ────              ──────
  [ ] ❌    README.md
  [ ] ❌    CONTRIBUTING.md
  [ ] ❌    CHANGELOG.md
  [ ] ❌    GOVERNANCE.md
  [ ] ❌    SUPPORT.md
  [ ] ❌    docs/CONSTITUTION.md
  [ ] ❌    docs/ARCHITECTURE.md
  [ ] ❌    AGENTS.md
  [ ] ❌    CODEOWNERS
  [ ] ❌    .editorconfig
  [ ] ❌    .markdownlint.json
  [ ] ❌    .gitignore
  [ ] ❌    .github/dependabot.yml
  [ ] ❌    .github/PULL_REQUEST_TEMPLATE.md

  [Select All]

  ❌ = missing  ⚠️ = exists, non-conformant  ✅ = exists, conformant  — = does not apply
```

**Path** carries the location the scan detected and is blank for a ❌ row. **Reason** names the conformance criterion a
⚠️ file failed, or why a — row does not apply; it is blank for ✅ and ❌ rows. Show ✅ rows for visibility; the user
cannot select them, except the `AGENTS.md` row when Step 1 marks it selectable for an offer. A catalog row whose
[Applies when](#conformance-criteria) condition is false shows — with its reason and cannot be selected.

Offer `.github/ISSUE_TEMPLATE/` and `.github/DISCUSSION_TEMPLATE/` only when the user asks for them, for example "add
issue templates". Before the issue-template questions, show the organization template override warning from
[issue-templates.md](templates/issue-templates.md) and ask its yes/no once. That question is a process gate: when the
user has already said not to stop for it, name its cost in one line (repository templates replace the organization's
defaults for this repository) and continue on the next reply that still declines.

**Step 3: ask what the selected files need, once.** Ask, in one ❓ round, only the facts the selected files need and the
repository does not already show: the project name, description, tech stack, owning team and organization, and each
selected file's own questions, grouped by file and numbered Q1..Qn across the groups with a ➡️ recommendation each.
Reuse the answers across every file. A protection-only selection asks only what its preview needs. A value with no
source is asked, never invented: an owning team for CODEOWNERS, an organization, a contact. When the user asks scaffold
to choose one, propose it from repository evidence (for example, owners from commit history) and label it as inferred in
the preview. When the user asks for a placeholder instead, name the cost once (a placeholder owner routes no review, and
GitHub flags an unknown owner as an error); on the next reply that still asks for it, use it and list it in the summary.

**Reflection hand-off.** `forge reflect` writes approved lines to an existing instruction file itself. It hands
`consumer pointer` rows to init only when the repository has no instruction file, so the rows need a starter
`AGENTS.md`. Preselect `AGENTS.md` and put each row's proposed line in the generated starter, explicitly marked inline
with the row's incident ID/source (e.g., `- <rule> <!-- incident: <incident-id> -->` — never as an unmarked plain
bullet). The user's selection in reflect is not approval of the write. A run that carries only hand-off rows shows no
menu and selects no other starter file; it asks only the Step 3 questions `AGENTS.md` needs that the scan does not
answer.

## Phase 2: generate, show, and confirm

The [file catalog](#file-catalog) links each file's questions, structure, and generation rules.
[Previews and confirmation](../SKILL.md#previews-and-confirmation) decides which files share a preview and which are
shown alone.

1. **Generate** every selected file from the Step 3 answers and repository facts. The no-invention rule applies to
   ungrounded organizational values (owning team, org, contact, check name, app ID)—it does not prevent generating
   complete previews of starter files, config updates, standalone `AGENTS.md` additions, or local protection diffs from
   discovered repository and user facts.
2. **Show the ordinary files together**, in catalog order, in one message. Print the complete verbatim text or exact
   diff of every selected file and local protection config/helper (no outlines, abbreviations, or placeholders). Ask one
   question, worded exactly:

    > ❓ **Q1** - **Create these files**: Create these <N> files? (yes / edit <file> / skip <file>)
    >
    > ➡️ yes, unless a preview flagged something to fix.

    With one ordinary file, word it `Create this file? (yes / edit / skip)`. On **edit**, take the correction,
    regenerate that file, and show it again with the same question. On **skip**, note it in the summary.

3. **Show each agent-instruction file alone**: `AGENTS.md` or its additions, `REVIEW.md`, the code-review skill. Show
   its exact text in its own message, never also inside step 2, and ask, worded exactly:

    > ❓ **Q1** - **Write this file**: Write <file> as shown? (yes / edit / skip)
    >
    > ➡️ yes, unless the text needs a change.

    Several additions to one existing file are shown in one message, each addition's exact text and placement under its
    own number, with one answer per addition. A declined addition writes nothing.

4. **Write** each confirmed file. A file whose exact content the user dictated skips step 2 and is shown by path. An
   agent-instruction file is never written without step 3.

**Selected protections** follow [protections.md](protections.md#explicit-init-protection-selection). Their local writes
join step 2, or step 3 for a consumer skill or instruction text. Their remote changes are prepared as exact proposals
and never applied by a local confirmation.

**Rules for both kinds of preview.**

- On a write failure, retry once with a safe write. If it still fails, report the error and continue with the other
  files. Never roll back files already written.
- On "stop" or "that's enough", keep the files written so far and print the summary.
- A request to revise a file already confirmed this run reopens that file: regenerate, show, and ask again. When the
  user would rather finish first, note it under "Also consider". Never edit a confirmed file without showing the change.
- After all files, scan the written files for cross-references, such as a README link to `docs/ARCHITECTURE.md`. A
  referenced file that this run did not write goes under "Also consider" with the reason.

## Phase 3: summary

```text
✅ Scaffold complete.

Created:       README.md, CONTRIBUTING.md, AGENTS.md
Skipped:       docs/CONSTITUTION.md — skipped by user
Not selected:  .github/PULL_REQUEST_TEMPLATE.md
Also consider: docs/ARCHITECTURE.md — README.md links to it, not written this run

Next: commit the files with `/agentics:git commit`.
```

Omit a line that has nothing to list. Add a line for each of these when it has entries: files written unpreviewed at the
user's request, placeholder or inferred values, local protection additions with their verification results, remote
proposals awaiting the user's approval, and blocked or unverified prerequisites with who can resolve them. Generated
source or settings are not proof of enforcement.

## Files the user decides

`LICENSE`, `SECURITY.md`, `CODE_OF_CONDUCT.md` and `FUNDING.yml` are not in the catalog. Init never offers them on its
own and never chooses their content. When the user asks for one, init asks for the decision the file records, then
writes it through step 2:

| File                 | The user decides                                                    | Content                                                                                                                                                            |
| -------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `LICENSE`            | The license, by SPDX identifier, and the copyright holder           | The license's published text, fetched from its published source or pasted by the user, with the user's year and copyright holder. Never reconstructed from memory. |
| `SECURITY.md`        | The reporting channel, the supported versions, and the response aim | Only those facts, in a short policy. No invented contact or timeline.                                                                                              |
| `CODE_OF_CONDUCT.md` | The code, by name and version, and the enforcement contact          | The code's published text, fetched or pasted, with the contact the user gave.                                                                                      |
| `FUNDING.yml`        | The funding platforms and account names                             | GitHub's `FUNDING.yml` keys with the user's values.                                                                                                                |

A request that names the file without the decision gets a Step 3 question for it, with no recommendation that makes the
decision for the user.

## File catalog

| #   | File                               | Reference                                                                                   |
| --- | ---------------------------------- | ------------------------------------------------------------------------------------------- |
| 1   | `README.md`                        | [references/templates/readme-template.md](templates/readme-template.md)                     |
| 2   | `CONTRIBUTING.md`                  | [references/community/contributing.md](community/contributing.md)                           |
| 3   | `CHANGELOG.md`                     | [references/templates/changelog.md](templates/changelog.md)                                 |
| 4   | `GOVERNANCE.md`                    | [references/governance/governance.md](governance/governance.md)                             |
| 5   | `SUPPORT.md`                       | [references/community/support.md](community/support.md)                                     |
| 6   | `docs/CONSTITUTION.md`             | [references/governance/constitution.md](governance/constitution.md)                         |
| 7   | `docs/ARCHITECTURE.md`             | [references/templates/architecture.md](templates/architecture.md)                           |
| 8   | `AGENTS.md`                        | [references/community/agents-md.md](community/agents-md.md)                                 |
| 9   | `CODEOWNERS`                       | [references/governance/codeowners.md](governance/codeowners.md)                             |
| 10  | `.editorconfig`                    | [references/config/editorconfig.md](config/editorconfig.md)                                 |
| 11  | `.markdownlint.json`               | [references/config/markdownlint.md](config/markdownlint.md)                                 |
| 12  | `.gitignore`                       | [references/config/gitignore.md](config/gitignore.md)                                       |
| 13  | `.github/dependabot.yml`           | [references/governance/dependabot.md](governance/dependabot.md)                             |
| 14  | `.github/PULL_REQUEST_TEMPLATE.md` | [references/templates/pull-request-template.md](templates/pull-request-template.md)         |
| 15  | `.github/ISSUE_TEMPLATE/`          | [references/templates/issue-templates.md](templates/issue-templates.md)                     |
| 16  | `.github/DISCUSSION_TEMPLATE/`     | [references/community/discussion-category-forms.md](community/discussion-category-forms.md) |
| 17  | Optional `code-review/SKILL.md`    | [references/community/code-review.md](community/code-review.md)                             |
| 18  | Optional `REVIEW.md`               | [references/community/review-md.md](community/review-md.md)                                 |

Row 14 names where init creates a template. The scan checks the template the contract resolves, wherever it is, and
reports that path. A resolved template is never ❌. Row 17 goes in the repository's project skill directory, which
[code-review.md](community/code-review.md) chooses.

Init creates `docs/`, `.github/`, and a skill's directory only to hold a file the user confirmed. Audit creates none.

**Excluded:** CI and release pipelines beyond an independently selected secret-scan integration. Secret-scan integration
follows the protection preview path. It is not a catalog file or part of Select All. A release or deploy writer binding
may be proposed for its owner; pipeline authoring stays outside this workflow. The four files under
[Files the user decides](#files-the-user-decides) are written only on the user's request and decisions.

## Conformance criteria

This table decides ✅, ⚠️ or — in init's menu and supplies governance evidence for the readiness audit. Audit reports a
missing file whose Applies-when condition holds as `missing`, a violated criterion as `failed`, demonstrated conformity
as `pass`, and a file whose condition is false as `inapplicable` with the reason. Where a reference file's own
conformance check differs from this table, this table decides.

| File                               | Applies when                                                                  | Conformant (✅) if...                                                                                                                       |
| ---------------------------------- | ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `README.md`                        | Always                                                                        | Has title, description, and ≥3 of: badges, prerequisites, setup, usage, architecture ref, contributing link                                 |
| `CONTRIBUTING.md`                  | Always                                                                        | Has sections for: prerequisites, local setup, PR process, commit conventions                                                                |
| `CHANGELOG.md`                     | The repository publishes versions: tags, releases, or a version in a manifest | Exists and non-empty                                                                                                                        |
| `GOVERNANCE.md`                    | More than one person maintains the repository, or the user says it is shared  | Has ≥1 section covering project roles or a decision-making process                                                                          |
| `SUPPORT.md`                       | Someone other than the maintainers uses what the repository ships             | Has ≥1 support contact, channel, or ticketing reference                                                                                     |
| `docs/CONSTITUTION.md`             | Always                                                                        | Has ≥1 core principles section and ≥1 constraints/decisions section                                                                         |
| `docs/ARCHITECTURE.md`             | The repository holds source code                                              | Exists and non-empty                                                                                                                        |
| `AGENTS.md`                        | Always                                                                        | Meets the three `AGENTS.md` criteria listed after this table                                                                                |
| `CODEOWNERS`                       | The host is GitHub or GitLab                                                  | ≥1 ownership rule; no syntax errors                                                                                                         |
| `.editorconfig`                    | The repository holds text files that people edit                              | Has `root = true` and `[*]` with `indent_style` + `indent_size`                                                                             |
| `.markdownlint.json`               | The repository holds Markdown files                                           | Valid JSON; ≥1 rule key                                                                                                                     |
| `.gitignore`                       | The directory is a Git repository                                             | Exists and non-empty                                                                                                                        |
| `.github/dependabot.yml`           | The host is GitHub and a manifest of a Dependabot ecosystem exists            | Valid YAML; `version: 2`; ≥1 `package-ecosystem` entry                                                                                      |
| `.github/PULL_REQUEST_TEMPLATE.md` | The host takes pull requests                                                  | The template [repo-conventions.md](../../routing/references/repo-conventions.md) resolves, at any of its locations, exists and is non-empty |
| `.github/ISSUE_TEMPLATE/`          | The user asked for issue templates                                            | Directory exists and contains ≥1 `.yml` file                                                                                                |
| `.github/DISCUSSION_TEMPLATE/`     | The user asked for discussion forms                                           | Directory exists and contains ≥1 `.yml` file                                                                                                |

A condition that the repository cannot show, such as how many people maintain it, is decided by what the user says. With
nothing said, the row applies, and the menu's Reason column states the assumption.

The `AGENTS.md` criteria:

- ≥1 Conventions section with ≥2 rules or behavioral instructions
- ≥1 section that states when mutating work opens with a record of why the change is made, and when that does not apply
- No `CLAUDE.md` or `.claude/CLAUDE.md` was created by init

Front matter is optional. When the file has front matter, it must parse as YAML.

A local `code-review` skill is optional. When present, check ordinary skill structure:

- Readable `SKILL.md`
- YAML frontmatter with a valid skill `name` and non-empty `description`
- Instructions in the Markdown body

Do not require Agentics evaluation metadata, specific sections, severity labels, or nit limits. Report a structural
problem with its path and reason. Keep valid author content. An absent skill is an optional offer and does not affect
compliance status.

A root `REVIEW.md` is optional too. When present, check that it is readable Markdown. Do not require the starter's
headings, severity names, or nit limit. An absent file is an optional offer and does not affect compliance status.

## Common mistakes

| Mistake                                                                         | Fix                                                                                                 |
| ------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `AGENTS.md` filled with conventions the repository does not have yet            | Write what is known: identity and commands that exist. Seed conventions as the fallback below says. |
| `docs/CONSTITUTION.md` written before any principle is decided                  | Ask whether one is decided; with none, recommend skipping it this run                               |
| `scaffold audit` run outside a Git repository                                   | The scan works on any directory, but the result cannot be committed                                 |
| `.gitignore` platform question answered with a language not on github/gitignore | Use the closest match (for example, "VisualStudio" covers all .NET) or "Other" for custom patterns  |

## When `forge` logic is unavailable

Generate `AGENTS.md` directly from [agents-md.md](community/agents-md.md). Never emit a bare `<!-- TODO -->` shell that
fails its own conformance check. Follow these steps:

1. Fill the title, description, and tech stack from the Step 3 answers. Do not ask again.
2. Add a **Conventions** section with ≥2 concrete rules. If none are known yet, seed 2 safe defaults, such as "Follow
   the existing file layout and naming" and "Run the project's formatter and linter before commit", and mark the section
   `<!-- expand as conventions emerge -->`.
3. Add these parts, worded as the template shows:
    - The Commands table with each build, test, and lint command and the one line a healthy run prints
    - The Recurring mistakes section
    - The Agent workflow's Compaction, Repeated corrections, and User-authorized exceptions subsections

    Keep the file within the template's one-page cap.

## References

- [community/agents-md.md](community/agents-md.md): AGENTS.md generation.
- [community/code-review.md](community/code-review.md): optional repository code-review skill authoring.
- [community/review-md.md](community/review-md.md): optional `REVIEW.md` starter for review passes, severities, and
  skips.
- [community/contributing.md](community/contributing.md): CONTRIBUTING.md questions and structure.
- [community/discussion-category-forms.md](community/discussion-category-forms.md): discussion template forms.
- [community/support.md](community/support.md): support policy and contact guidance.
- [config/editorconfig.md](config/editorconfig.md): editor configuration generation.
- [config/gitignore.md](config/gitignore.md): platform-specific ignore patterns.
- [config/markdownlint.md](config/markdownlint.md): markdown lint configuration.
- [governance/codeowners.md](governance/codeowners.md): CODEOWNERS questions and preview rules.
- [governance/constitution.md](governance/constitution.md): constitution structure.
- [governance/dependabot.md](governance/dependabot.md): Dependabot configuration.
- [governance/governance.md](governance/governance.md): governance file structure.
- [templates/architecture.md](templates/architecture.md): architecture document structure.
- [templates/changelog.md](templates/changelog.md): changelog generation.
- [templates/issue-templates.md](templates/issue-templates.md): issue template warning and forms.
- [templates/pull-request-template.md](templates/pull-request-template.md): pull request template generation.
- [templates/readme-template.md](templates/readme-template.md): README questions and structure.
- [community/README.md](community/README.md), [config/README.md](config/README.md),
  [governance/README.md](governance/README.md), and [templates/README.md](templates/README.md): the directory indexes.
