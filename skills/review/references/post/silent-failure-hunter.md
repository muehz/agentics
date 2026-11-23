# Silent Failure Hunter

Subagent #6 in the automated review pipeline. Zero tolerance for silent failures. These errors are caught, suppressed,
or swallowed without observability.

## Scope

Analyze all changed files in the PR for patterns that hide errors from logs, monitors, or callers. It also covers a diff
that weakens a check ([../check-weakening.md](../check-weakening.md)). A loosened lint, type, test, or CI check hides
failures from CI the way a swallowed error hides them from logs.

## Detection Targets

| Pattern                                            | Examples                                                                                                                            |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| Empty catch blocks                                 | `catch (e) {}` or `catch (e) { /* ignore */ }`                                                                                      |
| Broad exception handlers                           | `catch (Exception)`, `except:`, `except BaseException`, `catch (Throwable)` without filtering                                       |
| Suppressed errors                                  | `catch` blocks that return a default value without logging the error                                                                |
| Fallback without logging                           | `try { riskyCall() } catch { return safeDefault() }` with no log line                                                               |
| Promise/async swallowing                           | `.catch(() => {})`, missing `.catch()` on a chained promise, `await ...` without try/catch in a context where rejection is possible |
| Ignored error returns                              | C-style functions where the return code indicates failure but the caller discards it                                                |
| Boolean success returns that discard error details | `function doX() { try {...} catch { return false } }`: caller cannot distinguish failure modes                                      |
| Check weakening                                    | A diff that loosens a lint, format, type, test, or CI check instead of fixing the code it flags                                     |

For check weakening, apply [../check-weakening.md](../check-weakening.md), including its reason sources.

## Severity Classification

| Severity     | Criteria                                                                                                                                   |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| **CRITICAL** | Error completely invisible to operators: empty catch, swallowed promise, no logging or re-throw, no metric increment                       |
| **HIGH**     | Error partially hidden: broad handler catches more than intended, fallback loses original error details (stack trace, error code, message) |
| **MEDIUM**   | Error logged but action suppressed: catch block logs the error but returns a misleading success value to the caller                        |

A check weakening ([../check-weakening.md](../check-weakening.md)) is **HIGH** when the diff states no reason and
**MEDIUM** when a stated reason explains it. Its "User impact" line names which check no longer runs against what.

## Output Format (per finding)

For each detected pattern:

```text
**[SEVERITY]** {file_path}:{line_start}-{line_end}

exists_at_head: {yes | no | never}

Pattern: {empty_catch | broad_handler | fallback_no_log | promise_swallow | check_weakening | ...}

User impact: {what happens when this fails silently in production — concrete consequence, not vague "errors are missed"}

Fix:
{exact code change, not vague advice — show the corrected catch block, the added log line, the narrower exception type, etc.}
```

`exists_at_head` is required on every finding. The pipeline sends a finding without it back. Its values are defined in
[Step 5 of the post pipeline](post-pipeline.md#step-5-parallel-review-7-or-8-subagents). Report `never` for code that is
absent on both sides, such as a missing guard.

Include a permalink with full SHA in the final review output.

## False Positive Filters

Do not flag these. They are intentional and acceptable:

- **Documented no-op catches** with an explicit justification comment that explains why suppression is correct (e.g.,
  `// expected to fail when X; safe to ignore`)
- **Test files** where catch blocks deliberately exercise error paths in assertions.
    - This does not exempt a new test skip, which is a check weakening.
- **Cleanup or finally blocks** where failure during cleanup is genuinely non-critical and the pattern is documented
- **Optional chaining or null-safe access** that is not a try/catch (e.g., `value?.foo` is not silent failure detection
  territory)
- **Library-specific idioms** where the framework expects this pattern (e.g., specific React error boundary patterns
  that re-throw conditionally)

## Calibration

- Focus on changed lines only.
- Do not flag pre-existing silent failures unless the PR also touches them.
- Distinguish "caught but logged" from "caught and discarded". Only the latter is a finding.
- A catch that re-throws a wrapped exception is not silent (the chain is preserved)
- A catch that calls a logger and then returns a fallback IS acceptable if the fallback is appropriate.
    - The issue is when no log fires at all.

## Confidence Scoring Inputs

When the post pipeline scores findings (Step 6), this subagent's findings flow through the standard rubric in
`post-scoring.md`. Scoring guidance specific to silent failures:

- **High confidence (90+):** Empty catch with no log, no comment, and no re-throw. This is unambiguous.
- **Medium confidence (75-89):** Broad handler that probably catches too much, but the surrounding context might justify
  it
- **Low confidence (<75):** Discard. The pattern matches, but the broader function shows logging happens elsewhere.
- **Check weakening:** score by [../check-weakening.md](../check-weakening.md): 90 or above when unexplained, 50 or
  below when a stated reason explains it.
    - Quote the reason.
