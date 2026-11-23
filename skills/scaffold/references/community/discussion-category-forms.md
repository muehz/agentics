# Scaffold reference: discussion category forms (.github/DISCUSSION_TEMPLATE/)

## Overview

Discussion category forms are `.yml` files in `.github/DISCUSSION_TEMPLATE/`. Each file defines a structured form for
one discussion category, such as Q&A, Ideas, or Announcements. This workflow asks which categories need forms. Then it
generates each form and previews it before it writes it.

**Invocation.** Run this workflow only when the user explicitly requests it, such as "add discussion forms" or "scaffold
discussion templates". It is not in the standard scaffold selection menu.

**Prerequisite.** GitHub Discussions must be enabled on the repository. Ask the user to confirm this before you
continue.

**Location.** Write forms to `.github/DISCUSSION_TEMPLATE/` only.

## Interactive workflow

### Step 1: Confirm Discussions is enabled

Ask:

```text
Is GitHub Discussions enabled on this repository?
If not, enable it at: Settings → General → Features → Discussions
```

Proceed only after confirmation.

### Step 2: Select discussion categories

Ask which categories should have structured forms, as a ❓ block. Keep this wording exactly:

> ❓ **Q1** - **Discussion categories**: Which discussion categories would you like to create forms for?
>
> ```text
>   [ ] Announcements (read-only category for project updates)
>   [ ] Q&A (question and answer format)
>   [ ] Ideas (feature suggestions and ideation)
>   [ ] General (open-ended discussion)
>   [ ] Custom (you define the category name and fields)
> ```
>
> ➡️ Q&A and Ideas, the two most repos use first; add others only if the request names them.

### Step 3: For each selected category

1. Ask any category-specific questions as one ❓ round, each with a ➡️ recommendation
2. Generate the `.yml` form
3. Preview the full file content
4. Ask a ❓ confirm block. Keep this wording exactly:

    > ❓ **Q1** - **Create this form**: Create this form? (yes / edit / skip)
    >
    > ➡️ yes, unless the preview flagged something to fix.

5. Write to `.github/DISCUSSION_TEMPLATE/{category-slug}.yml`. GitHub applies a form only when its file name equals the
   slug of an existing category. The slug for the category named `Q&A` is `q-a`. When the repository renamed a category,
   read the slug from the category's URL (`/discussions/categories/{slug}`).

---

## Announcements form

**File:** `.github/DISCUSSION_TEMPLATE/announcements.yml`

> Announcements is usually a write-restricted category, so its form is minimal.

```yaml
title: "[Announcement]: "
body:
    - type: textarea
      id: announcement
      attributes:
          label: Announcement
          description: Share project news, releases, or important updates.
      validations:
          required: true
```

---

## Q&A form

**File:** `.github/DISCUSSION_TEMPLATE/q-a.yml`

```yaml
title: "[Q&A]: "
body:
    - type: textarea
      id: question
      attributes:
          label: Question
          description: What would you like to know?
      validations:
          required: true
    - type: textarea
      id: context
      attributes:
          label: Context
          description: What have you already tried or researched?
      validations:
          required: false
```

---

## Ideas form

**File:** `.github/DISCUSSION_TEMPLATE/ideas.yml`

```yaml
title: "[Idea]: "
body:
    - type: textarea
      id: idea
      attributes:
          label: Idea
          description: Describe your idea or proposal.
      validations:
          required: true
    - type: textarea
      id: motivation
      attributes:
          label: Motivation
          description: What problem does this idea solve?
      validations:
          required: false
    - type: textarea
      id: alternatives
      attributes:
          label: Alternatives Considered
      validations:
          required: false
```

---

## General form

**File:** `.github/DISCUSSION_TEMPLATE/general.yml`

```yaml
title: ""
body:
    - type: textarea
      id: body
      attributes:
          label: Discussion
          description: Start a conversation about anything related to this project.
      validations:
          required: true
```

---

## Custom form

**Questions to ask:**

Ask what is open as one ❓ round. Give each question a ➡️ recommendation. Skip a topic the request already settles.

- **Category name** (e.g., "RFCs").
- **File slug:** the slug of the existing category (e.g., `rfcs` → `rfcs.yml`).
- **Fields:** each with label, type, and whether it's required.
- **Labels to apply automatically,** if any (array or comma-delimited string, e.g. `labels: ["General Introduction"]`).

**Output.** Generate the form with the standard
[GitHub discussion form schema](https://docs.github.com/en/discussions/managing-discussions-for-your-community/creating-discussion-category-forms).
Top-level keys are `title:`, `body:`, and optionally `labels:`.

---

## Notes

- Discussion category forms use the same field types as issue forms: `textarea`, `input`, `dropdown`, `checkboxes`,
  `markdown`
- The category must already exist in the repo's Discussions settings. The form only adds structure to it
- The `labels` field applies labels automatically.
    - Its value is an array or a comma-delimited string.
    - Make sure those labels exist in the repository before the form applies them.

## Conformance check (audit mode)

✅ Conformant if `.github/DISCUSSION_TEMPLATE/` exists and contains at least one `.yml` file.
