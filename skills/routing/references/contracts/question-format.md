# Question format

This file holds the format of a decision question and the way its answers are collected. A skill that asks the user a
decision question uses this format.

The format does not govern one-line clarifiers, fixed yes/no confirmations, or approval questions whose wording another
rule fixes. A fixed question is never reshaped into the format.

Format each question exactly like so:

```
❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>
```

Number the questions in order. Start at Q1 unless the workflow continues numbering from an earlier round.

## One question, one decision

One question holds one decision. A bundle is split into one question per decision. A question that covers two artifacts
or two actions offers an answer for each.

## Collecting answers

Every round is posted in the visible chat first, as one message, before any ask-tool call. This holds for a round of one
question too. The whole round sits in one blockquote: every line of the round, blank lines included, starts with `>`.
The round then renders as formatted markdown, with its bold text and emoji, and can be selected and copied as one block.
Rendered formatting takes priority over copying the raw markdown, so the round is never put in a fenced code block.

The round is posted because the user must be able to read the questions after the dialog closes and copy them for
someone else to answer. An ask dialog alone does not meet this rule.

Some hosts show text written before a tool call only as a summary. On such a host, the round is posted through the
host's send-to-user tool when it has one. Without that tool, the turn ends with the round, and the answers are collected
in chat. The Claude Code, Codex and Copilot CLI terminals are expected to show text written before a tool call in full,
and none of them has a send-to-user tool, so on all three the round is an ordinary assistant message followed by the
ask-tool call. Headless runs on 2026-11-19 (Claude Code 2.1.328 `-p`, codex-cli 0.170.0 `exec`, Copilot CLI 1.0.109
`-p`) confirmed only part of this: none offered a send-to-user tool, and the Codex and Copilot event streams carried
text written before a tool call in full; the Claude Code run wrote no text before its tool calls. No ask tool was
called. In that mode Claude Code and Copilot CLI offered no ask tool, and Codex marked `request_user_input` as Plan-mode
only, so the questions ended the turn; it also offered `request_user_input_async`, which was not called. The ask-tool
step is not yet confirmed on any of the three.

The ask tool then only collects answers. Its questions are short and refer to the posted round by number, such as Q1 or
Q2.

Posting a round does not apply to the questions this file does not govern: one-line clarifiers, fixed yes/no
confirmations, and approval questions whose wording another rule fixes. Their own rules say how they are asked.

A single choice that accepts every recommendation is worded as "use the recommended answer for Q1 to Qn". It has two
limits:

- It is never used for findings.
- It is not offered when any recommended answer would publish, post, or halt the work. Those answers are given by name.

While collecting a round, settle only the questions the reply covers. An explicit choice to use recommended answers
settles the questions it covers when that choice is allowed. Other questions stay open. Follow the separate
[confirmation rules](approval.md#the-two-exceptions) for what a reply can authorize.
