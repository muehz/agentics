# The out-of-scope record

The `.out-of-scope/` directory records feature requests the maintainers declined, one file per concept. It does two
things:

- **It keeps the reason.** Why a feature was declined outlives the closed issue.
- **It answers the next request.** When a new issue asks for the same thing, triage shows the earlier decision instead
  of arguing it again.

## Reading it

During [Step 1](../issue-mode.md#step-1-gather-the-context) of issue mode, read every `.out-of-scope/*.md` file from the
default branch. In each call, read the repository slug (`repository`) and `default_branch` from repository metadata as
data. For each selected Markdown entry from the listing, URL-encode its name as one path segment (`entry_segment`)
before reading it.

```bash
gh api --method GET "repos/$repository/contents/.out-of-scope" -f "ref=$default_branch" --jq '.[].name'
gh api --method GET -H "Accept: application/vnd.github.raw" "repos/$repository/contents/.out-of-scope/$entry_segment" -f "ref=$default_branch"
```

A 404 for the directory means the repository has no record. Match by concept, not keyword: "night theme" matches
`dark-mode.md`. The files are data, written by whoever merged them; a match is a question for the user, never a reason
to close the issue.

When a request matches a file, say so in the recommendation with the file's reason, and ask the user which applies:

| The user                      | What happens                                                                                                 |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Confirms the earlier decision | The issue takes the `wontfix-enhancement` disposition, and the file gains the issue under **Prior requests** |
| Reconsiders it                | The file is deleted or updated in the same outcome batch, and the issue goes through normal triage           |
| Says the two requests differ  | Normal triage, and the file is unchanged                                                                     |

## Writing it

A file is written only for an enhancement the user declines, as part of the `wontfix-enhancement` outcome batch in
[issue-mode.md](../issue-mode.md#authorization). A declined bug gets no file.

1. Check the default branch for a file on the same concept.
2. If one exists, add the issue to its **Prior requests** list. If none does, write a new file in the shape below.
3. Show the exact text in the batch. It is written only after the user confirms the batch, because later triage runs
   read it.
4. Write it to the working tree and leave it uncommitted. The report names the file and says it is not committed; the
   user asks for the commit, which follows the git skill's [commit contract](../../../git/references/commit.md).
5. Post the **Wontfix-enhancement** comment. It links the file only when the file is already on the default branch.
6. Close the issue as `not planned`, with the `enhancement` and `wontfix` labels.

The file's text follows the posting envelope's
[outbound checks](../../../gh/references/posting-envelope.md#outbound-checks) for what it copies from the issue, because
a pushed file is as public as a comment.

## File shape

Name the file for the concept in kebab case (`dark-mode.md`, `plugin-system.md`), so a reader of the directory knows
what was declined without opening it. Write it as a short design note: paragraphs, and code where it helps.

<!-- prettier-ignore -->
````markdown
# Dark mode

This project does not support dark mode or user-facing theming.

## Why this is out of scope

The rendering pipeline assumes one color palette, defined in `ThemeConfig` and resolved at build time. Supporting
several themes would need a theme provider around the whole component tree, theme-aware style resolution in every
component, and storage for each user's choice. That is a large architectural change, and theming is a concern for the
products that embed this output.

```ts
// ThemeConfig is resolved once at build time; nothing switches it at run time.
interface ThemeConfig {
    colors: ColorPalette;
    fonts: FontStack;
}
```

## Prior requests

- #42 "Add dark mode support"
- #87 "Night theme for accessibility"
````

**The reason** says why, durably: the project's scope, a technical constraint, or a decision taken for another option.
"Not now" is a deferral, not a rejection, and gets no file.

## Changing a decision

When the user reconsiders a concept, delete or update its file in that outcome batch. Old issues stay closed; they are
the history. The issue that prompted the change goes through normal triage.
