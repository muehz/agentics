# Slop patterns: the AI-tell catalog

Edit text to remove AI patterns and add human voice. Preserve meaning, match the intended tone, and never alter
technical facts: file names, commands, flags, version numbers, and error text are data, not style.

Scope: this catalog detects tells in existing text; [technical-writing.md](technical-writing.md) is the authoring
standard, and the two files are aligned. A tell in this catalog is never protected as style; its
precedence-in-maintenance rule covers rhythm-level preferences only, and the four overlapping entries below (23, 28,
29, 31) defer to the authoring standard for the full rule.

## Process

1. Scan for the patterns below.
2. Rewrite. Preserve meaning, match intended tone.
3. Add soul (see next section).
4. Self-audit: "What makes this obviously AI generated?" Fix remaining tells.

## Adding soul

Removing patterns is half the job. Sterile, voiceless writing is just as obvious. Soul changes how the text's own
content reads. It never adds a claim, opinion, feeling, or outcome that the text did not already carry. Do not make
prose warmer by inventing any of them.

- **Keep the writer's opinions.** Where the text reacts to facts, keep the reaction. Do not flatten it into a neutral
  list of pros and cons, and do not add one the text lacks.
- **Vary rhythm.** Short sentences. Then longer ones that take their time. Mix it up.
- **Keep the complexity the text states.** "Impressive but also kind of unsettling" beats "impressive" when the text
  already says both. Do not add the second half.
- **Keep first person.** Where the text says "I", leave it. First person isn't unprofessional. Do not add it.
- **Don't over-polish.** Leave some irregularity in place. Perfect structure looks machine-made.
- **Be specific, from the text.** Replace "this is concerning" with the concrete fact the text already holds, such as
  "agents churn away at 3am". If the text holds no such fact, leave the sentence alone.

## Patterns to detect and fix

### Content

1. **Puffery.** "pivotal moment", "testament to", "evolving landscape", "setting the stage for", "indelible mark",
   "deeply rooted". Cut puffery, state what happened.
2. **Name-dropping.** Listing media outlets without context. Pick one, say what was said.
3. **Superficial -ing phrases.** "highlighting...", "ensuring...", "reflecting...", "showcasing...", "fostering...".
   Delete or expand with real sources.
4. **Promotional language.** "nestled", "vibrant", "breathtaking", "groundbreaking", "renowned", "stunning",
   "must-visit". Use neutral descriptions.
5. **Vague attributions.** "Experts believe", "Industry reports suggest", "Some critics argue". Name the source or
   delete.
6. **Formulaic challenges.** "Despite challenges... continues to thrive." Replace with specific facts.

### Language

7. **AI vocabulary.** Additionally, crucial, delve, enduring, enhance, fostering, garner, interplay, intricate,
   landscape (abstract), pivotal, showcase, tapestry (abstract), testament, underscore, vibrant. Replace with plain
   words.
8. **Fancy ways to say "is".** "serves as", "stands as", "boasts", "features". Just say "is" or "has".
9. **"Not just X, but Y."** State the point directly instead.
10. **Rule of three.** Forcing ideas into groups of three. Use the natural number.
11. **Synonym cycling.** Protagonist, main character, central figure, hero all in one paragraph. Pick one, repeat it.
12. **False ranges.** "from X to Y" where X and Y aren't on a meaningful scale. List topics directly.

### Style

13. **Em dash overuse.** Avoid em dashes entirely. Replace each one with a period or a comma. Do not use an en dash, a
    hyphen as a dash, or parentheses that only stand in for the dash. Em dashes are an AI tell, and the substitutes just
    trade one tell for another. The punctuation precedence rule in [technical-writing.md](technical-writing.md) says
    which dashes this pattern removes in each mode.
14. **Colon overuse.** Colons are fine before a list or example. Not as mid-sentence connectors. "If you're coming from
    traditional automation: instead of registering event handlers, you describe conditions" adds nothing with the colon.
    Rewrite to let the point stand on its own without comparison framing. "Describing when the scheduler should fire
    works best as plain English." Same meaning, no crutch punctuation.
15. **Boldface overuse.** Don't bold every proper noun or acronym.
16. **Inline-header lists.** The tell is a bold label and colon that restates the line: "**Performance:** Performance
    improved...". Convert those to prose. A bold lead-in that ends in a period, names the item, and is followed by
    genuinely new detail ("**Schema in TypeScript.** Tables live in one file.") is fine, not a tell.
17. **Title case headings.** Use sentence case.
18. **Decorative emojis.** Remove from headings and bullets.
19. **Curly quotes.** Replace with straight quotes.

### Communication artifacts

20. **Chatbot phrases.** "I hope this helps!", "Let me know if...", "Of course!", "Certainly!", "Found the smoking gun!"
    Remove.
21. **Cutoff disclaimers.** "While specific details are limited..." Find sources or remove.
22. **Sycophantic tone.** "Great question! You're absolutely right!" Respond directly.

### Filler

23. **Filler phrases.** "In order to" becomes "To"; "It is important to note that" gets deleted. Same test as the
    authoring standard's cut-every-word rule ([technical-writing.md](technical-writing.md)).
24. **Excessive hedging.** "could potentially possibly be argued that it might" becomes "may".
25. **Generic conclusions.** "The future looks bright." State specific plans or facts.

### Jargon

26. **Abstract metaphor nouns.** Substrate, wedge, vector, locus, vantage, nexus, primitive (as noun), harness (as
    metaphor), surface (as in "API surface"), bedrock, scaffolding (as metaphor), modality, paradigm, gold-plating,
    ratchet (as metaphor), evacuate (for moving code), endgame, north star, flywheel. These read as technical but
    usually have a plainer concrete word. "Substrate" becomes "base". "Wedge in" becomes "add". "Vector" becomes "way"
    or "method". "Gold-plating" becomes "more than the job needs". "Ratchet" becomes the mechanism's real name or "a
    limit that only tightens". "Evacuate" becomes "move out". "Endgame" becomes "the last phase". Pick the concrete
    word.

### Plain speech

27. **Say what it does, not how it feels.** "the database stays close at hand", "SQL you can read", "types that follow
    your schema" name a feeling. The fix names the mechanism or a number: "`.toSQL()` returns the exact string sent to
    the database", "a column rename fails the build". Ask what the sentence tells the reader to do or know, then write
    that. If you can't restate it as a concrete instruction, fact, or number, cut it. One more check: if the sentence
    could appear unchanged in another project's docs, it says nothing about this one. Cut it.
28. **Shorten or split dense sentences.** One idea per sentence. Full rule: the STE layer
    ([technical-writing.md](technical-writing.md) Layer 3).
29. **Active voice.** Name the actor: "queries are validated" becomes "the compiler validates queries". Full rule: the
    Google-style layer ([technical-writing.md](technical-writing.md) Layer 2).
30. **Cut adverbs, or use a stronger verb.** "runs quickly" becomes "is fast" or the number. "significantly improves"
    becomes the measured delta. An adverb propping up a weak verb means the verb is wrong.
31. **Prefer the plain word.** "utilize" becomes "use", "leverage" becomes "use", "facilitate" becomes "help". Same rule
    as the authoring standard's short-everyday-word rule ([technical-writing.md](technical-writing.md)).

## Scope guard for repo docs

Some of the patterns above collide with conventions a repository may enforce on purpose. When cleaning a file that lives
in a repo, the repo's own documented style wins over this catalog:

- A house style that mandates title-case headings, bold-label tables, or a fixed template (ADR forms, changelog formats)
  is a convention, not a tell. Leave the structure; clean the prose inside it.
- Reserved vocabulary is data. If a project defines "surface", "primitive", or "harness" as a real term of art (a
  glossary, a CONTEXT.md, this repo's own conventions), keep the term. Pattern 26 targets decorative metaphor, not a
  project's ubiquitous language.
- House style does not protect punctuation in editable text in Unslop mode. Patterns 13 and 14 apply even where a repo's
  existing prose leans on em dashes or connector colons: stripping tells is this mode's job, and a dash-heavy house
  voice is exactly the accumulation the catalog exists to reverse. House-style protection covers structure and
  vocabulary (the two bullets above), never the tells themselves. A protected span in
  [ste-exclusions.md](ste-exclusions.md) is not editable text, so it keeps its punctuation byte-exact. The punctuation
  precedence rule in [technical-writing.md](technical-writing.md) covers every case, including the other modes.
