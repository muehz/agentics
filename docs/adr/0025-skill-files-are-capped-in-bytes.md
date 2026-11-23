# Skill files are capped in bytes, and the files already over the cap may only shrink

This record supersedes the size limit in [0019](0019-output-and-skill-structure.md): its 500-line cap per `SKILL.md`,
checked by `skill-line-limit`, is retired, and every file under `skills/` is capped in bytes instead. It answers #1605.
The rest of 0019 stands.

A line count does not track size when prose wraps at 120 columns: `skills/execute/SKILL.md` passed the 500-line cap at
422 lines and 64 KB. Size matters because a host reads a file in one tool call. Codex estimates a tool output's tokens
as `ceil(UTF-8 bytes / 4)` and cuts any single output over 10,000 tokens, dropping its middle (muehz/evals#29). In
#1145, gate's waiver sentence at byte ~53,600 of `branch-workflow.md` fell in that dropped middle.

## Rules

### Every file under skills/ is at most 40,000 bytes

- **Answers:** #1605
- **Rule:** The conformance rule `skill-file-size` caps every git-tracked file under `skills/` at 40,000 bytes: each
  `SKILL.md`, each reference, each script and any other file. The limit lives in two named constants,
  `SKILL_MD_BYTE_LIMIT` for `SKILL.md` and `SKILL_FILE_BYTE_LIMIT` for every other file, and both are 40,000 today. A
  file of at most 40,000 bytes is at most 10,000 tokens by Codex's estimate, so read whole it is never cut. The byte
  rule is forge's single size limit, and `skill-line-limit` is retired. When a step's body moves to a reference file to
  fit, the step keeps its heading and a one-line pointer to where the text went.
- **Default / Rejected:** One byte limit for every file under `skills/`; rejected: keeping the line limit, because lines
  do not track bytes at a 120-column prose wrap. Rejected: a limit on `SKILL.md` alone, because a reference is read in
  one tool call too, and the #1145 loss was in a reference.
- **Later work changes:** follow-up issues plan to lower `SKILL_MD_BYTE_LIMIT` to 30,000 and then 20,000 bytes, toward
  Anthropic's guidance of about 5,000 tokens for a skill body. Neither step is part of this record.

### The files already over the cap are allowlisted at their current size, and the list only shrinks

- **Answers:** #1605
- **Rule:** `.github/tools/src/conformance/skill-size-allowlist.json` lists each file that was over the limit when the
  rule landed, with a byte ceiling equal to its size then. A listed file may shrink but not grow past its ceiling
  (`skill-file-size`). An entry whose file is missing or within the limit fails as stale (`skill-file-size-stale`), so
  an entry leaves the list once its file fits. No pull request adds an entry or raises a ceiling. The rule and the
  allowlist run outside the skill baseline, so no `skill-baseline/` entry can silence them. Over-size files are split
  along their existing sections, one skill per pull request, with eval evidence for that skill.
- **Default / Rejected:** A dedicated shrink-only JSON allowlist with per-file ceilings; rejected: a constant map in
  rule code, because every split would then edit rule source. Rejected: extending `skill-baseline/`, because its entries
  match exact keys, hold no ceiling, and are rewritten whole when the baseline is regenerated.
- **Later work changes:** one pull request per over-size skill, each removing that skill's entries from the allowlist. A
  split moves whole sections, so links from outside `skills/` to a moved anchor must follow it; the companion rule
  `skill-inbound-anchor` checks those links.

## Existing records

- 0019: superseded in part — its size sentence, its "One line limit that conformance enforces" default, and its cost
  note that the line limit fails for several skills each carry a note pointing here; the rest of the record stands.

## What this costs

**Some files stay over the limit for a while.** Until each allowlisted file is split, a Codex agent that reads it whole
still loses its middle. The allowlist stops those files from growing; it does not make them readable.

**Every split is a skill change.** Moving a step's body into a reference adds a file read, and each split needs its own
evals, so retiring the allowlist takes one pull request per skill.

**A byte limit is coarser than a token count.** Codex's `ceil(bytes / 4)` estimate is the cut that matters today; other
hosts count tokens differently, and the limit does not track them.
