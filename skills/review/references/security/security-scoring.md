# Security confidence scoring

Every security path in this skill scores its candidates with this file: `post` mode's subagent #8, the `axes` Security
lane at full depth, and the same lane at `--fast`. Repository guidance that the review entry point supplies (see
[Repository guidance](../../SKILL.md#repository-guidance)) replaces the rubric, the threshold and the output where it
sets them. Pass its source paths to every scorer.

## What the score measures

The score is confidence that the finding is **real**: the weakness exists in the target, untrusted input or an untrusted
actor can reach it, and no effective control stops it. It is not severity.

How bad a real finding is (its impact, and the preconditions an attacker needs, such as a login, a role or a position on
the internal network) sets its severity by the taxonomy in [security-principles.md](security-principles.md). A
precondition never lowers the score. An IDOR that needs any logged-in user is as real as one that needs none; it is
rated lower, not reported less.

## Rubric (five anchors)

Pick the anchor whose description fits. A score between two anchors is not used.

- **0, Not real.** A false positive under light scrutiny, an item on the suppression list in
  [security-principles.md](security-principles.md#false-positive-suppression), a pre-existing issue the target does not
  touch, or a control the scorer confirmed stops it.
- **25, Untraced.** A pattern match whose source, path or sink is not traced, or a weakness stated without a named
  control gap.
- **50, Traced but unsettled.** The weakness is verified in isolation, but one of these is open:
    - a control may stop it and the scorer could neither confirm nor rule it out (an allowlist nearby, a framework
      default it could not read, upstream validation in a file it could not reach);
    - no line in scope connects untrusted input or an untrusted actor to it.
- **75, Traced and reachable.** The source-to-sink path or the missing control is traced through the target, the source
  is untrusted, and no effective control was found on the path. Any precondition goes into the severity.
- **100, Definitive.** One of these:
    - traced data flow from untrusted input to a dangerous sink (code execution, SQL, a shell, deserialization) with no
      control. A file-path sink with no containment scores 75, as the calibration below shows, because what it reaches
      depends on the files the process can read;
    - a value in a live credential format on a tracked line of the target;
    - a dependency version with a published advisory whose affected function the target calls.

## Threshold

**Default threshold: 75**, at every depth and in `post` mode. A candidate at 75 or 100 is reported; 0, 25 and 50 are
dropped. The rubric has no anchor between 75 and 100, so a higher bar would report only definitive findings, and traced
weaknesses that need one more hop of reading would never reach the reader. Depths differ in how much they read, not in
how sure a finding has to be.

Apply the repository's reporting criteria when the entry point supplies them, and report the threshold that was applied.
Keep each score as scored.

The filter drops real issues: a 50 is a traced weakness the scorer could not settle. Count the candidates it drops. The
count goes into the limits statement of the `axes` lane and the `--composed` envelope, and into the local report of
`post` mode, whose comment carries no internal counts.

## Scope for reachability

"In scope" depends on the target:

| Target                                                 | In scope                                                                                 |
| ------------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| A diff (a PR, a branch, a fixed point)                 | The changed lines, and unchanged code that a changed line makes newly reachable or worse |
| A path or a whole repository, which a caller asked for | Every line of the target                                                                 |

A finding on a wider target is not capped for lacking a diff line. Scope discipline in
[security-principles.md](security-principles.md#scope-discipline) decides which findings a diff target reports.

## Scoring rules

Apply them in this order; the first that fits sets the score.

1. **Suppressed, or stopped by a confirmed control:** 0.
2. **No traced source-to-sink path and no named control gap:** 25.
3. **Traced, but a control is unsettled, or nothing in scope connects untrusted input to it:** 50.
4. **Traced, reachable and uncontrolled:** 75, or 100 when it meets an anchor-100 case.

Code reachable only from tests and not shipped falls under rule 1 through suppression item 2, except a value in a live
credential format, which is never suppressed (see
[security-principles.md](security-principles.md#false-positive-suppression)).

## Calibration anchors

### 100

- `query("SELECT * FROM users WHERE id=" + req.params.id)` added in the diff with no parameterization, traced from the
  request to the driver
- `AKIA[A-Z0-9]{16}` with a 40-character secret on a tracked line, in any path, a built or vendored file included
- `exec(req.body.cmd)` or `eval(req.query.expr)` in a route handler
- A dependency pinned to a version with a published advisory whose affected function the code calls

### 75

- An IDOR: a route reads a record by an ID from the URL with no ownership check, on a route that needs a login
- Stored XSS: input written to the DOM through `innerHTML` with no sanitization, in an admin console
- Path traversal: `path.join(base, req.query.path)` read by `fs.readFile`, with no normalization or root containment
  check on the path
- CORS: `Access-Control-Allow-Origin` reflected from the request with `Access-Control-Allow-Credentials: true`, on an
  endpoint that returns account data
- A JWT verified with the `none` algorithm allowed, or with the signature check skipped on one code path

### 50

- SSRF: an outbound request to a URL built from input, with a hostname allowlist in a module the scorer could not read
- A cookie with no `SameSite` flag where the framework default could not be read
- A sink that takes a value from a configuration file, where nothing shows who can write that file

### 25

- "Consider input validation" with no sink named
- "This regex might be ReDoS-prone" with no input that shows it
- MD5 used somewhere with no use traced to a security purpose

### 0

- Placeholder credentials in `tests/` or `__fixtures__/` that are not in a live credential format
- An advisory on a transitive dependency whose affected function nothing calls
- A missing CSRF token on a stateless API that authenticates with bearer tokens
- A pattern match on commented-out code

## Dispatch

The lead scores in batches, never one scorer per candidate:

- **Full depth:** one scorer dispatch per domain reviewer that returned candidates, up to 20 candidates a dispatch; a
  domain with more gets one more dispatch per further 20. [security-axis.md](security-axis.md#dispatch-count) states the
  count.
- **`post` mode:** the post pipeline's own batches of up to 20, with this file as the rubric
  ([Step 6](../post/post-pipeline.md#step-6-confidence-scoring)).
- **`--fast`:** the one reviewer scores its own candidates with this file; no scorer is dispatched.

## Output per candidate

```json
{
    "id": "sec-001",
    "score": 75,
    "justification": "Traced req.query.path into fs.readFile at L44; no normalization or root check; route needs a login, which sets severity, not this score."
}
```
