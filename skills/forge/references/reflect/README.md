This folder holds the reviewer and synthesizer prompt templates for `forge`'s reflect mode. The mode dispatches
`judgment-reviewer.md`, `tooling-reviewer.md`, and `divergent-reviewer.md` in parallel over the session transcript. It
then feeds all three outputs into `synthesizer.md`. Pass each template verbatim. Substitute only where the template
marks a substitution. The workflow lives in `skills/forge/SKILL.md` under "Reflect mode". With earlier sessions
selected, it first runs `session-extractor.md` once per earlier session and passes the digests to the three reviewers.
Every template opens with the bounded-assignment line, is read-only, and names its return words, as routing's
[dispatch list](../../../routing/SKILL.md#what-every-dispatch-carries) requires.
