---
name: principles
description: "Use when a design or delegation decision needs a stronger engineering rationale. Identify the principles that actually apply, read their guidance, and use them to explain the tradeoffs rather than relying on slogans or personal preference."
version: "2.0.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[optional: the task or decision the principles should inform]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
---

# Principles

This skill is the index for 22 engineering principles. It finds the principles that can change a decision in the task, reads their leaf files, and checks each one against the real artifact. It gives advice and citations. It writes no code and edits no file.

The skill has no modes. The optional argument names the task or decision. A person can invoke it directly. A caller skill can run the [How to apply](#how-to-apply) steps inside its own step, in a phase that has a diff or an artifact to inspect.

## Don't use when

Read the rows in order. The first match wins. Each row names its kind for an explicit invocation, the user naming `principles` for this request ([forwarding.md](../routing/references/forwarding.md#explicit-invocation)). A **cannot** row forwards by [forwarding.md](../routing/references/forwarding.md) under any invocation. A **better elsewhere** row, under an explicit invocation, gets one line that names the other skill, and the run continues here. When the model loads this skill on its own, every row forwards. A scan that a caller skill runs inside its own step matches no row, because the caller already owns the request.

| Order | Input signal                                                      | Kind             | Use this instead | Why                                                                                                                     |
| ----- | ----------------------------------------------------------------- | ---------------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------- |
| 1     | Capturing a problem, outcome, or scope to work on                 | cannot           | `intent`         | This skill informs decisions inside work whose direction is set. It records no direction.                               |
| 2     | A review verdict on a pull request or diff: bugs, security, merge | cannot           | `review`         | This skill gives no findings and no verdict. It can inform a review, not replace it.                                    |
| 3     | Authoring or changing a principle file under `references/`        | cannot           | `forge`          | This skill writes no file. `forge` owns principle files ([job owners](../routing/references/forwarding.md#job-owners)). |
| 4     | Adversarial pressure on one plan or design: what could go wrong   | better elsewhere | `challenge`      | `challenge` cross-examines the plan. This skill can only check the plan's decisions against the principles.             |
| 5     | Interface and seam design for one concrete module                 | better elsewhere | `deepen`         | `deepen` owns seams, interfaces, and deepening refactors. This skill can only name the principles that bear on them.    |
| 6     | Otherwise                                                         | —                | this skill       | Run [How to apply](#how-to-apply).                                                                                      |

A keyword overlap with an "Applies when" cell does not change the row. "Migration" in a plan that the user wants stress-tested is row 4, not a shortlist.

## How to apply

Apply the [principles check](../routing/references/contracts/principles-check.md) to an accessible artifact. If a direct request has no
artifact, use only the [shortlist](../routing/references/contracts/principles-check.md#shortlist) and return the provisional result below.

### How the run ends

Read the rows in order. The first match wins.

| Order | State after the scan                                         | Result                                                                                                                                                                                                                                                                                                        |
| ----- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | The shortlist is empty                                       | Say "no principle materially applies". Read no leaf, run no self-test, and cite nothing. A caller carries on. A direct request gets the advice it asks for, and a requested change follows [A requested change](#a-requested-change).                                                                         |
| 2     | No artifact is accessible: no diff, no file, nothing running | Return a provisional shortlist. Name the exact file or diff that would close the gap, and how to get it: `git diff <base>..HEAD` locally, with the base resolved as [base-and-remote.md](../git/references/base-and-remote.md#resolution-order) says, or the `gh` skill for a pull request. Cite nothing yet. |
| 3     | A caller skill ran the scan                                  | Return the shortlist, the decision constraints, and the citations. The caller carries on. Do not roll a citation into a wider refactor that it recommends.                                                                                                                                                    |
| 4     | Otherwise: a direct request with an accessible artifact      | Give the shortlist, the citations, and the advice the request asks for. When the request also asks to change code, see [A requested change](#a-requested-change).                                                                                                                                             |

### A requested change

This skill writes no code. When the request asks for a change, give the shortlist and the citations first, then pass the change on with them as constraints:

- A change to behavior, an interface, or a contract goes to `routing`, announced and run ([forwarding.md](../routing/references/forwarding.md#target-classification)). `routing` assigns the delivery path, so a direct invocation of this skill is never a way around it.
- A change with no runtime behavior, such as a typo or formatting, is normal implementation. This skill has no workflow for it; do not forward it, and let the session make the edit directly with no ceremony.

### Worked example

> Task: add a third retry mode to the export scheduler. Two booleans, `retryEnabled` and `retryImmediate`, already gate the flow in `src/export/scheduler.ts`.
> Shortlist: `model-the-domain`, `make-operations-idempotent`, `laziness-protocol`.
>
> - model-the-domain: model the retry policy as one enum. This deletes both booleans.
> - make-operations-idempotent: add a reconciliation step, so a rescheduled job converges after a crashed run leaves a stale lock.
> - laziness-protocol: drop the proposed retry-strategy plugin interface. One enum covers all three modes.

The empty result is just as valid. For "fix the typo in line 3", the shortlist is empty, and the edit goes ahead with no leaf, self-test, or citation.

### Citations that have no evidence

A citation says that a principle changed a choice in the work. Without an inspected artifact, that is a claim of status with no evidence from this session, which is a safety gate ([gates.md](../routing/references/contracts/gates.md#safety-gate)). The same holds for a reason, a measurement, or a rejected alternative that nobody recorded. When the user asks for either, hold once in the gate's shape:

- **Held:** citations or a history that no inspected evidence supports, such as a padded "Principles applied" section.
- **Harm:** reviewers and later maintainers read an invented reason as fact.
- **Instead:** inspect the diff and cite only the principles that changed a choice, or report that none did. If the diff cannot be loaded in this session, return only a provisional shortlist with no citations and name how to obtain the diff (`git diff <base>..HEAD` locally or the `gh` skill for a PR). For a missing reason, describe the change truthfully and say that its reason is not recorded, or record a new decision from evidence gathered now and date it as new.

A repeated request gets one line that points back to that alternative. Who approves the wording does not change what the evidence shows.

## References

The leaves are grouped by the kind of decision they inform. Each row names the leaf, its one-line rule, and when it applies.

**Core**

| Principle                                                                      | Rule                                                                               | Applies when                                                           |
| ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| [laziness-protocol](references/laziness-protocol.md)                           | Bias toward deletion and the smallest change that solves the problem               | Refactoring, sizing a diff, tempted to add abstractions or layers      |
| [foundational-thinking](references/foundational-thinking.md)                   | Get the data structures right first; downstream code becomes obvious               | Before writing logic: core types, scaffold-vs-feature sequencing       |
| [redesign-from-first-principles](references/redesign-from-first-principles.md) | Redesign as if the new requirement had been foundational from day one              | Integrating a new requirement into an existing design                  |
| [subtract-before-you-add](references/subtract-before-you-add.md)               | Remove dead weight first, then build on the simpler base                           | Sequencing an addition, refactor, or rewrite                           |
| [minimize-reader-load](references/minimize-reader-load.md)                     | Count layers to trace and state to hold; collapse both                             | Reviewing or shaping code that is hard to trace                        |
| [outcome-oriented-execution](references/outcome-oriented-execution.md)         | Converge on the target architecture; don't preserve throwaway compatibility states | Planned rewrites and migrations with explicit phase boundaries         |
| [experience-first](references/experience-first.md)                             | Choose user delight over implementation convenience; ship less, ship better        | Product, UX, or feature-scope trade-offs                               |
| [exhaust-the-design-space](references/exhaust-the-design-space.md)             | Build 2–3 competing prototypes and compare before committing                       | A novel interaction or architectural decision with no precedent        |
| [build-the-lever](references/build-the-lever.md)                               | Build the tool that does or proves the work; the tool is what a reviewer reruns    | Work past the small-fix test: migrations, sweeps, analyses, and checks |
| [record-the-reason](references/record-the-reason.md)                           | Record why the behavior should change and the relevant trade-off before editing    | Changing behavior, defaults, interfaces, or contracts                  |

**Architecture**

| Principle                                                                                          | Rule                                                                                      | Applies when                                                             |
| -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| [model-the-domain](references/model-the-domain.md)                                                 | Encode the domain in a structure instead of scattered conditionals                        | Writing stateful logic; code branches a lot or repeats shape assumptions |
| [boundary-discipline](references/boundary-discipline.md)                                           | Validate at system boundaries; trust internal types; keep logic pure                      | Wiring validation, error handling, or framework adapters                 |
| [type-system-discipline](references/type-system-discipline.md)                                     | Make illegal states unrepresentable; brand primitives; parse at boundaries                | Designing types or a signature in any typed language                     |
| [make-operations-idempotent](references/make-operations-idempotent.md)                             | Converge to the same end state regardless of partial prior runs                           | Commands, lifecycle steps, or loops that run amid crashes and retries    |
| [migrate-callers-then-delete-legacy-apis](references/migrate-callers-then-delete-legacy-apis.md)   | Migrate callers and delete the old API in the same wave                                   | Introducing a new internal API while old callers exist                   |
| [separate-before-serializing-shared-state](references/separate-before-serializing-shared-state.md) | Eliminate the sharing first; serialize structurally only when sharing is a real invariant | Concurrent actors might write the same file, branch, key, or object      |

**Verification**

| Principle                                                            | Rule                                                                                 | Applies when                                                          |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| [prove-it-works](references/prove-it-works.md)                       | Verify against the real artifact, not a proxy, self-report, or "it compiles"         | After completing a task, before declaring done                        |
| [fix-root-causes](references/fix-root-causes.md)                     | Trace each symptom to its root cause; reproduce first; no guard-as-fix               | Debugging                                                             |
| [sequence-verifiable-units](references/sequence-verifiable-units.md) | Small units that each end in a check; verify before advancing; order to prove itself | Multi-step work, including sweeps, migrations, and commit/PR stacking |

**Delegation**

| Principle                                                          | Rule                                                                                         | Applies when                                                     |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| [guard-the-context-window](references/guard-the-context-window.md) | Route bulk to subagents; keep summaries in the main thread                                   | Context fills up: large outputs, long files, or fan-out planning |
| [never-block-on-the-human](references/never-block-on-the-human.md) | Proceed on reversible work and present the result; reserve confirmation for the irreversible | Tempted to ask "should I do X?" on reversible work               |

**Meta**

| Principle                                                                | Rule                                                                            | Applies when                                                  |
| ------------------------------------------------------------------------ | ------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| [encode-lessons-in-structure](references/encode-lessons-in-structure.md) | Encode a recurring instruction as a lint, check, or script instead of more text | You catch yourself writing the same instruction a second time |
