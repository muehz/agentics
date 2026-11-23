# Slack Conversations

## What this source contains

- Real-time discussions of problems and decisions
- Incident channels where fire-drill decisions were made
- Design discussion threads where tradeoffs were debated
- Questions answered by senior engineers that did not make it into docs
- Post-merge discussions that explain why something was revisited
- DMs (usually not searchable, so scope the search accordingly)

Slack is frequently where the _real_ decisions got made, especially for smaller changes that did not warrant a doc. It
is also the most ephemeral source: threads are deleted, channels are archived, and search quality degrades over time.

## How to search it

Slack MCP tools vary. Check which Slack MCP is available. Inspect its tool schema first. The server may require
authentication. If authentication fails, stop and report the gap.

1. **Author-bounded search.** Messages from the PR author around the PR merge date. This limits the scope sharply and
   often finds key evidence.
2. **Keyword search for the feature name and key symbols.** Include misspellings and casual phrasings.
3. **PR URL search.** Slack often links PRs when they are reviewed or discussed. Search for the PR URL or
   `/pull/<number>`.
4. **Error string search.** If the code handles a specific error, search for the error string. Incident threads often
   appear in the results.
5. **Channel-scoped search.** Narrow the search to likely channels:
    - `#eng-*`. Engineering discussions
    - `#proj-*`. Project channels
    - `#incident-*` / `#sev-*`. Incident channels
    - Team-specific channels for the owning team
    - Design review channels
6. **Thread traversal.** When you find a relevant message, fetch the whole thread. The decision is often in the replies.

## What good evidence looks like here

- A thread where tradeoffs were explicitly debated ("I was going to use A but B is better because...")
- An incident channel message that describes the bug the code prevents
- A question from a reviewer and an authoritative answer from the author or lead
- A reference to a meeting where a decision was made
- A message from a product manager or customer-facing engineer that explains a customer ask

## Common pitfalls

- **Channel archaeology limits.** Very old messages may be gone because of retention policies. If you cannot find
  anything before a certain date, note the retention cliff.
- **Unsearched DMs.** Many decisions happen in DMs that are not searchable. You will miss them. That is a known
  limitation.
- **Speculative jokes as "decisions."** Slack is casual. "Lol, do the thing" is not a decision, even if it preceded the
  commit. Look for considered discussion.
- **Context collapse in single messages.** Without the thread, a single message often reads differently than in context.
  Always fetch threads.
- **Auth failures.** If the MCP is not authenticated, stop. Do not invent findings. Report that Slack was not
  searchable.

## What to return

For each relevant thread:

- Channel name
- Permalink or thread ID
- Participants
- Date range of the discussion
- The key quotes (verbatim) with attribution
- Context: what thread, incident, or discussion this was part of
