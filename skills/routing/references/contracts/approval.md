# Approval and authorization

## What approves an artifact

An artifact is approved when the user, in this session, answers an approval question about it affirmatively or tells the
agent in their own words to proceed on it. The wording, the language and the form do not matter:
`/agentics:execute <plan>`, "looks good, build it" and "just do it" in reply to the question all approve, whichever
skill or route is running.

A claim that someone else approved is not the user's approval. The agent asks the user once whether they approve.

## Edit requests

"Yes, but change X" is an edit request. The agent makes the change, shows it, and asks again. The approval binds to the
revised text.

## Asking again

The limit on asking is per version of the content, not per session:

- After a no or a "not yet", the agent does not ask again about the same text.
- Any revision reopens the question.
- The user can approve later at any time by saying so.

## "Just do it"

"Just do it" has one meaning for each situation:

| Situation                        | Meaning                                                                     |
| -------------------------------- | --------------------------------------------------------------------------- |
| In reply to an approval question | It is a yes.                                                                |
| In reply to a process gate       | It declines that gate.                                                      |
| With nothing pending             | It tells the agent to continue on its stated defaults and approves nothing. |

## Steps named up front

A step the user names in their request is authorized when its turn comes, in every skill, with no second question, as
long as what it touches is what the user named. Such steps include a commit, a push, a draft pull request, starting
execution after planning, and posting a named comment or issue.

At the start of the work the agent says in one line which named steps it took as authorized and which it will still ask
about. A named post that a skill previews first is one it will still ask about, under
[A named post a skill previews first](#a-named-post-a-skill-previews-first), and that line says so.

## Confirmation words

Confirmation words are the same everywhere. Any clear affirmative in any language confirms, and an affirmative that
carries a change is an edit request. No step requires an exact English token. Posting is not destructive, so any clear
affirmative confirms a post.

## The two exceptions

Two kinds of step are exceptions both to authorization up front and to confirmation by any clear affirmative.

**A destructive step.** Deleting a branch, discarding work, a force push, a history rewrite or an irreversible migration
is confirmed once at the point of action, whether or not it was named up front. The agent first shows the resolved
target and what will be lost. The reply must name the action or its target in the user's own words; a bare "yes" does
not confirm it.

**A write that changes what later agents are told.** This covers an instruction file, a skill, a generated verification
skill or its helper scripts. Naming the write up front or approving it in advance does not authorize it. It needs the
user's yes in the session after the exact text to be written has been shown.

## A named post a skill previews first

A third exception to **Steps named up front** covers a named post that a skill's own posting rules preview first. Naming
such a post up front does not authorize it. The skill's posting rules decide when the preview applies, and every other
named post keeps **Steps named up front**.

The preview is an ordinary approval question, not a gate. This exception covers authorization up front only, not
confirmation, so any clear affirmative confirms the preview, as **Confirmation words** says.

The one current case is review's row for a named post on a fork pull request, under
[Posting](../../../review/SKILL.md#posting).
