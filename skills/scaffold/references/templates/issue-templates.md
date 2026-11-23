# Scaffold reference: issue templates (.github/ISSUE_TEMPLATE/)

## Overview

Issue templates are `.yml` files in `.github/ISSUE_TEMPLATE/`, one file per template a contributor sees when they open a
new issue. This workflow asks which types are needed. It generates and previews each template, and it writes the
template after the preview.

**Invocation.** Run this workflow only when the user explicitly requests it, such as "add issue templates" or "scaffold
issue templates". It is not in the standard scaffold selection menu.

**Location.** Write templates to `.github/ISSUE_TEMPLATE/` only. No other location is valid.

## ⚠️ Organization Template Override Warning

**Before you begin the workflow, display this warning and ask its yes/no once.** The question is a process gate
([gates.md](../../../routing/references/contracts/gates.md#process-gate)). Replace `{owner}` with the owner of the
repository's `origin` remote. Change nothing else in the warning.

```text
Repository-level issue templates in .github/ISSUE_TEMPLATE/ override your
organization's default templates. Once created, contributors
opening issues in this repo will only see these templates — not the org-wide
defaults.

This is acceptable when:
  • These templates are an enhancement of or addition to the org templates
  • The team is intentionally diverging for a repo-specific workflow

This can cause confusion when:
  • Templates become outdated relative to the org standard
  • New template types added to the org default are not reflected here

Recommendation: Review the org-level templates at github.com/{owner}/.github
before creating repo-level overrides, and align where possible.

Proceed with creating repo-level issue templates? (yes / no)
```

Continue on a **yes**. On a **no**, stop and note the refusal in the summary. When the user has already said not to stop
for the warning, name its cost in one line (repository templates replace the organization's defaults for this
repository) and continue on the next reply that still declines.

## Interactive workflow

### Step 1: Select template types

Ask the user as a ❓ block with exactly this wording:

> ❓ **Q1** - **Template types**: Which issue template types would you like to create?
>
> ```text
>   [ ] Bug report
>   [ ] Feature request
>   [ ] Custom (you define the name and fields)
>   [ ] All of the above
> ```
>
> ➡️ Bug report and Feature request, the two most repos need; add Custom only if the request names a third type.

### Step 2: For each selected type

1. Ask the type-specific questions below as one ❓ round, each with a ➡️ recommendation
2. Generate the `.yml` template
3. Preview the full file content
4. Ask a ❓ confirm block with exactly this wording:

    > ❓ **Q1** - **Create this template**: Create this template? (yes / edit / skip)
    >
    > ➡️ yes, unless the preview flagged something to fix.

5. Write to `.github/ISSUE_TEMPLATE/{template-name}.yml`

Repeat for each selected type.

**Step 2b: After you create all selected templates, ask as a ❓ block:**

> ❓ **Q1** - **Issue creation config**: Would you like a `config.yml` to customize the issue creation experience? This
> file can disable blank issues (forces use of a template) and add links to external trackers.
>
> - Disable blank issues? (yes/no)
> - Add external link(s)? (e.g., Jira project URL, ServiceNow queue)
>
> ➡️ Disable blank issues once at least one template exists; add external links only if the request names a tracker.

If the user says yes to either question, generate `.github/ISSUE_TEMPLATE/config.yml`:

```yaml
blank_issues_enabled: false # set to true if blank issues should be allowed
contact_links:
    - name: "{tracker name}: {project} backlog"
      url: "{tracker URL}"
      about: "{what the link is for}"
```

Set `blank_issues_enabled` from the answer: `false` when the user chose to disable blank issues, `true` otherwise. Fill
each `contact_links` entry from the link the user gave. `url` must be a full `https://` address. Write no entry for a
link the user did not give. When the user gave none, write `blank_issues_enabled` alone and omit the `contact_links`
key. Never write a placeholder URL.

---

## Bug report template

**File:** `.github/ISSUE_TEMPLATE/bug-report.yml`

**Questions to ask:**

Ask what's open as one ❓ question with a ➡️ recommendation drawn from the repo's existing labels. If the request
already settles it, skip the question.

- **Default assignees or labels,** if any.

**Standard output:**

```yaml
name: Bug Report
description: Report a bug or unexpected behavior
title: "[Bug]: "
labels: ["bug", "triage"]
body:
    - type: markdown
      attributes:
          value: |
              Thanks for taking the time to report this issue!
    - type: textarea
      id: description
      attributes:
          label: Description
          description: A clear and concise description of the bug.
          placeholder: What happened?
      validations:
          required: true
    - type: textarea
      id: reproduction
      attributes:
          label: Steps to Reproduce
          description: How can we reproduce this behavior?
          placeholder: |
              1. Go to '...'
              2. Call '...'
              3. Observe '...'
      validations:
          required: true
    - type: textarea
      id: expected
      attributes:
          label: Expected Behavior
          placeholder: What did you expect to happen?
      validations:
          required: true
    - type: textarea
      id: environment
      attributes:
          label: Environment
          description: Relevant runtime, OS, or version information.
          placeholder: |
              - OS: Windows 11
              - Runtime: .NET 8.0
              - Version: 1.2.3
      validations:
          required: false
    - type: textarea
      id: logs
      attributes:
          label: Logs / Stack Trace
          description: Paste any relevant logs or error output.
          render: shell
      validations:
          required: false
```

---

## Feature request template

**File:** `.github/ISSUE_TEMPLATE/feature-request.yml`

**Questions to ask:**

Ask what's open as one ❓ question with a ➡️ recommendation drawn from the repo's existing labels. If the request
already settles it, skip the question.

- **Default labels or assignees for feature requests,** if any.

**Standard output:**

```yaml
name: Feature Request
description: Suggest a new feature or enhancement
title: "[Feature]: "
labels: ["enhancement"]
body:
    - type: markdown
      attributes:
          value: |
              Thanks for the suggestion! Please provide as much context as possible.
    - type: textarea
      id: problem
      attributes:
          label: Problem Statement
          description: What problem does this feature solve?
          placeholder: I'm always frustrated when...
      validations:
          required: true
    - type: textarea
      id: solution
      attributes:
          label: Proposed Solution
          description: Describe the solution you'd like.
      validations:
          required: true
    - type: textarea
      id: alternatives
      attributes:
          label: Alternatives Considered
          description: What alternatives have you considered?
      validations:
          required: false
    - type: textarea
      id: context
      attributes:
          label: Additional Context
          description: Any other context, screenshots, or references.
      validations:
          required: false
```

---

## Custom template

**File:** `.github/ISSUE_TEMPLATE/{slug}.yml`

**Questions to ask:**

Ask what's open as one ❓ round, each question with a ➡️ recommendation. Skip a topic the request already settles.

- **Template name** (e.g., "Documentation Update").
- **File slug** (e.g., `docs-update` → `docs-update.yml`).
- **Fields:** each field's label, type, and whether it's required.
- **Default labels,** if any.

**Output.** Generate the template from the user's answers with the standard GitHub issue form schema. Each field uses
one of `textarea`, `input`, `dropdown`, `checkboxes`, or `markdown`.

---

## Notes

- Templates use the
  [GitHub issue forms schema](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-issue-forms)
  (`.yml` format, not legacy `.md` Markdown templates)
- The `labels` field applies labels automatically. Make sure those labels exist in the repository.

## Conformance check (audit mode)

✅ Conformant if `.github/ISSUE_TEMPLATE/` exists and contains at least one `.yml` file.
