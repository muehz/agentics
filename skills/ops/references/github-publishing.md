# GitHub publishing

By default, runbooks live as markdown in the consuming repo, and that is the recommended pattern. This file covers the
optional paths to publish them elsewhere.

Every write follows [Writes outside the machine](../SKILL.md#writes-outside-the-machine): render the artifact and the
command in chat, run the outbound checks, and publish on any clear affirmative, or without a second question when the
user named the publication up front. A "just post it" in reply to a draft is that affirmative.

## When you don't need this file

- The runbook lives at `runbooks/<name>.md` in the consuming repo, is committed with the code, and is reviewed in a PR.
  This covers most cases and needs only the standard git and PR workflow.

## When you do need this file

The user wants the runbook to be discoverable beyond the source repo:

1. **GitHub wiki** of the same repo
2. **Separate docs repo** (e.g., a `docs-site` repo that publishes to a static site)
3. **PR-as-publish.** The runbook itself lives in the repo. You also want to open a PR to a related docs repo.

## Pattern 1: GitHub wiki

A few orgs use the GitHub wiki for runbooks because wikis live outside the source tree and add nothing to PR diffs.

### Draft

After you author the runbook, draft a wiki page:

```markdown
**Wiki page draft:** `Runbook: <name>`

<wiki page content: runbook body, possibly with extra wiki-specific links>
```

### Confirm

Present the draft and the command that will run if the user confirms:

```text
This will create a new wiki page at:
  https://github.com/<owner>/<repo>/wiki/Runbook:-<name>

Method: GitHub exposes the wiki as a separate Git remote at
https://github.com/<owner>/<repo>.wiki.git. The user clones that remote, adds the page, and pushes back.

Reply yes to publish.
Or edit the draft and it is shown again.
```

### Publish

GitHub's REST API has no endpoints for wiki pages. Push directly to the wiki remote. Provide the exact commands and the
wiki remote URL. Let the user run them. Don't try to automate wiki authentication.

## Pattern 2: Separate docs repo

Some orgs keep a central docs / runbooks repo separate from the service repo where the procedure lives.

### Draft

After you author the runbook, draft a PR to the docs repo. Before you render the draft, run the read-only steps of
`gh`'s [Pull request creation workflow](../../gh/SKILL.md#pull-request-creation-workflow) against the **docs repo**. The
steps cover the merge settings, the PR template, and the issue linkage of the docs repo. A runbook usually links no
issue. If the docs repo template has its own linkage section, record that inside it, per `gh`'s
[linkage](../../gh/references/pull-requests/linkage.md#step-3-resolve-three-ways) rule. If the template has none, add no
linkage line, per the same rule. Never put a closing keyword on an issue in the source repo, because the docs PR would
close it when it merges. The draft below shows the final body, so the Confirm step stays the only confirmation.

```markdown
**Target repo:** `<owner>/docs-repo` **Target branch:** `add-runbook-<name>` **Target path:** `docs/runbooks/<name>.md`

**PR title:** Add runbook: <name>

**PR body:** <per `gh`'s [body contract](../../gh/references/pull-requests/body.md): Summary says what the runbook
covers and where it came from; Evidence links the source repo or incident and names who reviewed the content against it,
or says it has not been reviewed yet; Impact is usually "docs only; reverting removes the page on the next site build".
The runbook lives in the file, not the body.>
```

### Confirm

```text
This will:
  1. Clone `<owner>/docs-repo` to a temporary directory and create branch `add-runbook-<name>` there
  2. Add `docs/runbooks/<name>.md` and commit it through `/agentics:git commit`
  3. Push the branch through `/agentics:git push`
  4. Open a draft PR titled "Add runbook: <name>" through `gh`

Reply yes to publish, or edit the draft and it is shown again.
```

### Publish

The clone, branch and file write are local. The commit, the push and the pull request go through their owners: `git`
writes the commit and pushes the branch under its own rules, and `gh`'s
[Pull request creation workflow](../../gh/SKILL.md#pull-request-creation-workflow) opens the draft PR. The one yes above
covers the push and the PR, because the user named both when they confirmed this sequence.

## Pattern 3: PR-as-publish (mirror)

Some teams keep the runbook in both places. The source repo keeps it close to the code. A docs repo makes it easy to
find. The skill handles both:

1. Write the runbook to `runbooks/<name>.md` in the source repo (standard).
2. After the user confirms step 1, draft a mirror PR to the docs repo per Pattern 2.
3. The docs-repo version gets a footer that points back to the source repo:

```markdown
***

_This runbook is mirrored from
[`<source-repo>/runbooks/<name>.md`](https://github.com/<owner>/<source-repo>/blob/main/runbooks/<name>.md). Keep them
in sync. Edit the source first._
```

## Provenance comments

When you publish anywhere other than the runbook's home repo, include a provenance footer:

```markdown
***

_Authored via `/agentics:ops author` on YYYY-MM-DD. Seeded from postmortem [`<owner>/<repo>#<issue>`](url) (if
applicable)._
```

The footer tells the next reader who has to update the runbook where the source of truth is.

## Authentication notes

- `gh` CLI handles GitHub-app and personal-access-token auth across repos the user can access. If the user can't access
  the target docs repo, report a clear error rather than attempt auth workarounds.
- Wiki pushes use the user's standard git auth to the wiki remote. Don't add any other auth layer on top.
- For org-internal docs repos behind SSO, confirm with `gh auth status` that SSO is satisfied before any `gh pr create`.
  If it is not, tell the user to run `gh auth refresh -s <scope>`.

## What this file does NOT cover

- Notion / Confluence / external wikis: out of scope. The user can copy the markdown draft by hand.
- Auto-discovery / runbook index pages: covered by future tooling, not this file.
- CI-driven runbook publishing: the skill is interactive, not headless.
