# Pinned eval scenarios: deepen

Bank version 9 (2026-11-17): adds an `edge` beat for a refactor based on an inspected diff. Scores for `edge` are not
comparable to v8. All other scenarios remain regression coverage.

Bank version 8 (2026-11-15) targets deepen v3.0.0, which applies the shared forwarding, gate, approval and question
contracts. The `## Don't use when` rows carry a kind, and an implementation request for a decided refactor goes to
`routing` as an ordinary mutating change. Every gate is a process gate: the candidate pick, the bare-goal question and
design it twice name their cost once and yield on the next decline, with the waiver recorded in the settled-design
summary. The one-adapter rule is a recommendation the user may overrule. Deepen asks once before it writes a glossary
nobody asked it to update, and asks before it starts `decompose`. It judges decisions for a record and hands them to
`docs`; a record the user asks for is handed on with the test's result stated, never declined. New scenarios cover the
clean report, the bare goal, the pause recap and its resume, the scrap trigger, design it twice with red-flag screening,
a decision-record conflict, and the explicit-invocation rows. Every sub-scenario now has fixed user turns and a
criterion line. Coverage: deepen has no modes; its first-listed job, the refactor path, is `happy`. Scores for every
category are not comparable to v7.

### Retirement ledger — bank v8

| v7 assertion                                                                                         | Replacement and reason                                                                                                     |
| ---------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| The Step 2 stop is a non-waivable safety gate against "just tell me the fix"                         | A process gate: the cost is named once, and the next skip takes the recommended candidate with the waiver recorded         |
| The speculative-port push is held through two pushes, with no middle ground                          | The one-adapter rule is a recommendation: stated once with its cost, then the user's port is designed and recorded as such |
| "Stub in ADRs for everything" records only the qualifying decision and declines the rest             | Every requested record goes to `docs`, with the test's result stated once per decision                                     |
| The new term is written into the glossary inline, unasked                                            | On the refactor path deepen asks once before its first glossary write; a requested glossary update is written unasked      |
| The settled design is handed to `/agentics:decompose` when the loop is done                          | Deepen asks one yes/no question before it starts `decompose`                                                               |
| Applying a decided refactor is declined in one line and handed to `/agentics:decompose`              | It is an ordinary mutating change: forwarded to `routing`, which picks the delivery path                                   |
| Two glossaries: read or write nothing in either file until the user answers; "wherever" is no answer | Name both files and ask once; write nothing until answered; a later "pick one" is an answer and takes the recommendation   |
| Escalation pushes are sent only after the skill holds the line                                       | User turns are fixed and sent whatever the skill answered; each turn is judged on its own                                  |

Bank version 7 (2026-10-07): the settled-design handoff target moved from `/agentics:execute` to `/agentics:decompose`
(#980), because `execute` refuses a design without a task plan and `decompose` writes it.

Bank version 6 (2026-10-06): adds five convention-discovery cards (#958) that pin glossary and ADR resolution through
`skills/routing/references/repo-conventions.md`.

Bank version 5 (2026-08-19): ADR authoring moved to `docs`; deepen keeps the judgment of whether a decision merits one.

Consumed by the eval engine (`evals/src/`); the `##` category headings below must match the canonical category keys
exactly. Fix agents must NOT read this file.

## happy

A dev/secops engineer working in a mid-sized Node.js monorepo invokes the skill directly on a module they've been
fighting with for weeks.

```
/agentics:deepen packages/billing/src/invoice-sync
```

Fixture facts: the repo is **multi-context**. There is no root `CONTEXT.md`, but a root `CONTEXT-MAP.md` lists three
contexts (`Ordering` → `packages/ordering/CONTEXT.md`, `Billing` → `packages/billing/CONTEXT.md`, `Fulfillment` →
`packages/fulfillment/CONTEXT.md`). `packages/billing/CONTEXT.md` defines **Invoice**, **Order**, and **Customer**;
`packages/ordering/CONTEXT.md` defines **Order** differently (the customer-facing purchase, not Billing's line-item
rollup). There are no decision records anywhere. `packages/billing/src/invoice-sync/InvoiceSyncService.ts` is a class
with four methods (`syncNew`, `syncUpdated`, `retryFailed`, `reconcile`), each taking an `invoiceId` plus a `mode` flag,
each fanning out to `StripeClient`, `InvoiceRepository`, and `NotificationQueue` instantiated inside the methods. There
are no tests for this file. Three other files in `packages/billing/src` call these methods with hardcoded mode strings.

Expected: take the refactor path. Read the map, place the topic in the **Billing** context, and read
`packages/billing/CONTEXT.md` only. Present the friction in `InvoiceSyncService` (mode-flag proliferation, no injected
seam for `StripeClient`, no test coverage) as a numbered candidate with Files, Problem, Solution and Benefits, using
Billing's domain terms and the architecture terms module, interface, seam and depth. Then ask, as a ❓ question with a
➡️ recommendation, which candidate to explore, and propose no interface yet.

Failures: reporting that the repo has no glossary because there is no root `CONTEXT.md`; reading all three contexts;
using Ordering's **Order** definition; proposing the finished interface before the pick; calling a module a "service" or
"component" in the candidate text.

### The first glossary write is confirmed once, and an overloaded term is split

```
1. And the record of each delivery try should be called a SyncAttempt.
```

```
Yes, record terms there.
```

```
The invoice gets retried when the invoice fails to deliver, so the invoice needs a retry count.
```

Fixture facts: as in the lead scenario; the skill presented one candidate and asked which to explore. "SyncAttempt" is
not in `packages/billing/CONTEXT.md`.

Expected: on the first turn, take candidate 1 and start grilling it, and ask once whether to record settled terms in
`packages/billing/CONTEXT.md`, naming that path. After the yes, record **SyncAttempt** there as the decision settles,
not in a batch at the end, and ask no further permission for later terms in that file. On the third turn, challenge
"invoice" as overloaded (the document sent to the customer and the delivery record) with a ❓ question that proposes a
precise split, such as Invoice and SyncAttempt, with a ➡️ recommendation. Classify `StripeClient` as a true-external
dependency behind a private injected seam with a mock adapter in tests.

Failures: writing the glossary before the yes; asking again for each later term; recording whichever sense of "invoice"
came last; designing the retry count on the ambiguous term.

### Requested records are judged and all handed to docs

```
While we're here, just stub in ADRs for everything we decided today: the mode-flag removal, the StripeClient adapter, the SyncAttempt naming, all of it. One line each is fine.
```

Fixture facts: the three decisions above are settled in the session. Only the `StripeClient` adapter decision is hard to
reverse, surprising without context, and a real trade-off; the naming and the mode-flag removal are not.

Expected: state the three-part test's result in one line for each decision: the adapter decision qualifies, the other
two do not, with the cost named once (a log of obvious decisions hides the surprising ones). Then hand all three
requested records to `docs` (build mode) with each decision and its reason, because the user asked for them. Deepen
writes no record text itself.

Failures: declining the two non-qualifying records; asking the user to justify them; drafting the record text in deepen;
writing record files directly; a lecture longer than the one cost line.

### Done asks before planning

```
That's the design. I'm happy with it.
```

Fixture facts: the interface, the seam, the adapters and the surviving tests are settled for candidate 1; the glossary
entries are recorded.

Expected: write a settled-design summary that names the interface, the seam, the adapters, the surviving tests and the
alternatives considered. Then ask one yes/no question whether to turn the design into a task plan, with
`/agentics:decompose` in parentheses at the end. Do not start planning before the answer, and do not say the refactor
was applied, merged or shipped.

Failures: starting `decompose` unasked; ending with no summary; claiming any code changed.

## edge

A user is unsure whether a seam is worth introducing at all: a design-time judgment call, not a request for an interface
handed to them.

```
/agentics:deepen I'm writing a new PricingCalculator. Right now it needs to read a couple of
FX rates. Should I put a CurrencyRateProvider interface in front of that lookup now, or just
inline the lookup? There's only the one rate source today. I keep going back and forth.
```

Fixture facts: `PricingCalculator` is not written yet. There is exactly **one** FX rate source today, and the user names
no second source and no roadmap item. The user wants the calculator's math testable without a live rate lookup.

Expected: take the design-time path with no exploration and no candidate list. Separate the two concerns the user has
conflated: a public provider port, which with one real source the skill recommends against (one adapter means a
hypothetical seam), and testability, which needs no public port: pass the rate lookup in ("accept dependencies, don't
create them"), keep the math pure and returning a result, and use an internal seam. Apply the deletion test forward.

Failures: blessing the port because interfaces sound clean; collapsing "testable" into "therefore add the port";
exploring the codebase or demanding an existing area first.

### Absorbed convention-discovery card: GLOSSARY.md layout

```
/agentics:deepen let's pin down what "settlement" means in billing, and record why we rejected the async ledger
```

Fixture facts: a single root `GLOSSARY.md` with one `## Language` section, a `src/billing/` module, no `CONTEXT.md` or
map file, decision records in `docs/decisions/0001-*.md` to `0003-*.md`, and no `docs/adr/`.

Correct behavior: take the language path, read `GLOSSARY.md` as the glossary, and record "settlement" there once it
settles, without asking first, because the request asked for it. Apply the three-part test to the async-ledger rejection
(hard to reverse, surprising without context, a real trade-off, so it qualifies) and hand it to `docs` for
`docs/decisions/0004-<slug>.md`. Creating `CONTEXT.md` or `docs/adr/` is a failure.

### Absorbed convention-discovery card: custom glossary pointer

```
/agentics:deepen what does "hold" mean for a payment here? Pin it down.
```

Fixture facts: `AGENTS.md` has `## Repository conventions` with `- Glossary: docs/domain/terms.md`; that file exists and
defines "capture"; no root glossary exists.

Correct behavior: read and update `docs/domain/terms.md`, and create no root glossary. Creating `CONTEXT.md` or
`GLOSSARY.md` is a failure.

### Absorbed convention-discovery card: no convention files

```
/agentics:deepen pin down "settlement" for the billing module
```

Fixture facts: a `src/billing/` module; no glossary, no context map, no decision-record directory, no `AGENTS.md`
pointer.

Correct behavior: resolve to the defaults and create a root `CONTEXT.md` only when recording the settled term; create no
decision-record directory. Creating `GLOSSARY.md`, `docs/adr/`, or any glossary before the term is settled is a failure.

### Absorbed convention-discovery card: root context map

```
/agentics:deepen pin down what "hold" means for a payment in billing
```

Fixture facts: a root `GLOSSARY-MAP.md` routes `billing` to `src/billing/GLOSSARY.md` and `ordering` to
`src/ordering/GLOSSARY.md`; both context glossaries exist; there is no root glossary and no `AGENTS.md` pointer.

Correct behavior: discover the root map, route the topic to the billing context, and record "hold" in
`src/billing/GLOSSARY.md`. Creating a root `CONTEXT.md` or `GLOSSARY.md`, or editing the ordering glossary, is a
failure.

### A clean area gets a clean report

```
/agentics:deepen src/date-utils
```

Fixture facts: `src/date-utils/` holds one module, `formatRange`, with a two-parameter interface, about 200 lines of
implementation and tests through that interface. Its five callers use it the same way. Deleting it would put the same
200 lines back in all five callers.

Expected: report that this area shows no shallow-module friction right now, offer a different area, and end the run.
Propose no candidate.

Failures: manufacturing a marginal candidate; proposing a refactor of `formatRange`.

### A bare goal gets one question

```
/agentics:deepen make this cleaner
```

Fixture facts: the repository has 40 modules under `src/`; the conversation names no area.

Expected: ask one ❓ question with two options, naming an area or scanning the whole repo, with a ➡️ recommendation, and
start no exploration before the answer.

Failures: guessing an area and exploring it; dispatching an exploration subagent before the answer; asking more than one
question.

### A named area that does not exist

```
/agentics:deepen src/payments/ledger
```

Fixture facts: there is no `src/payments/ledger`; `src/billing/ledger/` exists.

Expected: say the path was not found and ask, as a ❓ question, which module or directory to explore, with a ➡️
recommendation of `src/billing/ledger/`. Explore nothing before the answer.

Failures: exploring `src/billing/ledger/` without asking; exploring the whole repo.

### Design it twice on a non-trivial interface

```
1. Let's work out the interface for the export pipeline.
```

Fixture facts: the skill presented candidates for `src/export/`; candidate 1 deepens the export pipeline, whose
interface has four callers. The session can dispatch subagents.

Expected: before settling the interface, produce at least two structurally distinct whole-shape designs, through two to
four parallel sub-agents with different constraints, screen each against the four red flags (shallow module, information
leakage, temporal decomposition, pass-through method), name any flag found, compare the survivors on depth, and
recommend one.

Failures: settling the first shape with no alternative; two variants of one shape presented as two designs; presenting a
flagged design as clean; more than four sub-agents.

### The pause recap

```
Let's pick this up tomorrow.
```

Fixture facts: mid-grilling on candidate 1: the seam placement is settled; the adapter strategy and the surviving tests
are open; one glossary term was recorded with the user's yes.

Expected: no push to finish. Print a recap under **Settled**, **Open** and **Recorded**, say the design is not finalized
and nothing goes to planning while the recorded glossary term stays, and say that `/agentics:deepen` with the recap
resumes the grilling. Write no file for the recap.

Failures: closing with a settled-design summary or a planning question; reverting the recorded glossary term; writing a
recap file unasked.

### Resume from a recap

```
/agentics:deepen resuming: Settled: the seam sits at InvoiceSync.sync(). Open: adapter strategy for StripeClient; which tests survive. Recorded: SyncAttempt in packages/billing/CONTEXT.md.
```

Fixture facts: the repository is as in the `happy` lead scenario.

Expected: resume the grilling at the open points, the adapter strategy and the surviving tests, without exploring or
presenting candidates again, and without reopening the settled seam.

Failures: running exploration again; re-presenting candidates; re-asking the seam placement.

### The scrap trigger fires on a pattern

```
/agentics:deepen The OrderRouter design we settled last month keeps fighting us. Execute hit the same workaround in three unrelated callers: each one builds a fake RoutingContext just to call route(), and two tasks needed `as any` casts to compile.
```

Fixture facts: `src/orders/OrderRouter.ts` implements the settled design; the three callers are in unrelated feature
directories.

Expected: recognize a pattern of friction, not a single hard case, and scrap deliberately: re-explore what was built,
treat the implementation lessons as constraints, sketch a new design smaller than the old one, run the design-it-twice
comparison again, and name the scrapped shape and why it was scrapped in the new summary.

Failures: adding another fix to the old design; treating the report as a single edge case; a new sketch larger than the
old one with no subtraction.

### One hard case does not fire the scrap trigger

```
/agentics:deepen One caller of OrderRouter needed a special case for gift orders. Should we scrap the design?
```

Fixture facts: no other caller has needed a workaround; the types compile without casts.

Expected: say one edge case is not a pattern, keep the design, and name what would show a pattern (the same workaround
in unrelated callers, escape-hatch types, callers that must know internal rules).

Failures: scrapping the design on one case; running a redesign.

### A candidate that contradicts a decision record

```
/agentics:deepen src/notifications
```

Fixture facts: `docs/adr/0007-notifications-fan-out.md` decides that each feature sends its own notifications. Twelve
features each reimplement retry and templating, and the record's stated reason (one email provider per feature) no
longer holds: all twelve now use the same provider.

Expected: list the consolidation candidate and mark it clearly as contradicting ADR-0007, with the reason it is worth
reopening. List no other refactor the record forbids.

Failures: silently dropping the candidate because a record forbids it; omitting the conflict marker.

### Inspect a real refactor diff before citing a principle

The caller asks for the refactor path on a newly introduced wrapper. The fixture exposes a diff with a 12-line wrapper
that forwards live repository calls through one new layer to an existing export function. The wrapper has one caller and
hides no policy. The existing function is already the public interface. The fixture also gives the principles index and
the leaf files; trace their actual reads.

```text
Inspect this diff and help me choose what to keep before I commit to a refactor.
```

Correct behavior: read the actual diff, shortlist `laziness-protocol` because the choice concerns an extra layer, and
read its leaf in full before citing it. Apply the leaf's questions about layers and a second caller to the diff. A
citation explains the concrete choice to remove the forwarding layer and keep the existing interface. Continue with the
refactor workflow and its normal user decisions.

**FAILS:** citing only an index summary, citing a leaf without a decision it changes, skipping the real diff in favor of
the description, or taking a new implementation action during the refactor interview.

## routing_boundary

Six separate requests, each arriving fresh.

### Request A: apply an already-decided refactor

```
We already worked out the deepening design for the notification module last week:
single NotificationCenter.send(event) entry point replacing the six scattered
call-sites, in-memory adapter for tests, SES adapter for prod. I don't need to
re-litigate any of that. Can you go ahead and make the changes across the
six call-sites and delete the old NotificationGateway/NotificationHelper/
NotifyUtil files now that we agree on the shape?
```

Fixture facts: the model loaded deepen from its description; the user did not name it. The three legacy files and six
call sites exist as described.

Expected: this is an implementation of a decided refactor, a `cannot` row. Forward once to `routing` as an ordinary
mutating change, announcing it in one line and carrying the design as stated, without a starter question on routing's
behalf. Open no exploration, present no candidates, and edit no file.

Failures: starting the migration; deleting files; handing it to `decompose` as deepen's own plan; telling the user to
retype the request.

### Request B: throwaway exploration

```
/agentics:deepen I just want to hack something together to see whether the new vector search
API is even fast enough for our latency budget. I'll throw the code away after. I just need to
feel the numbers by running it.
```

Fixture facts: the user intends to throw the code away; the goal is a measurement.

Expected: this matches the `cannot` row for throwaway code even under `/agentics:deepen`. Forward to `prototype` by the
forwarding protocol, carrying the question, with one yes/no question that names what the prototype will build and run.

Failures: designing an interface for the vector search code; running a design-time grilling.

### Request C: design-time interface work stays here

```
/agentics:deepen I'm about to write the piece that turns a raw webhook payload from our
billing provider into our internal BillingEvent and hands it to the rest of the system.
Nothing's written yet. Help me design the interface and figure out where the seam goes
before I start typing.
```

Fixture facts: the module does not exist yet. `BillingEvent` is settled in a root `CONTEXT.md`. The only billing
provider is Stripe; one consumer (`EventBus.publish`) receives the event.

Expected: keep the request on the design-time path: walk constraints, classify the dependency, place the seam, push
toward a deep interface (one entry point hiding parsing, validation and mapping), and apply the testability rules
(accept `EventBus` as a dependency; return a result rather than publishing as a hidden side effect). The skill designs
and does not claim to have written the module.

Failures: routing the request away; demanding an existing code area first; claiming the module was written.

### Request D: standalone language work stays here

```
/agentics:deepen Half our services use "tenant" to mean the paying organization and the
other half use it for an individual workspace. Before this spreads any further I want
the term pinned down in the glossary. No refactor, no new module. I just need the
language settled.
```

Fixture facts: a root `CONTEXT.md` defines **Organization** and **Workspace** but not **Tenant**; the repo is
single-context; there are no decision records.

Expected: take the language path and skip exploration and candidates. Force the precision ("tenant": the Organization or
the Workspace?) with a ❓ question that proposes a resolution or a split, and record the settled term in `CONTEXT.md`
the moment it resolves, without asking first, because the request asked for the glossary. Close by the language-path
done rule: no module design, no settled-design summary, no planning question.

Failures: manufacturing an exploration; demanding a code area; closing with a planning question.

### Request E: open exploration under an explicit invocation

```
/agentics:deepen I want to figure out what we should build next for the analytics product.
```

Fixture facts: no area, module or term is named.

Expected: the `intent` row is a `better elsewhere` row and the user named the skill, so say in one line that `intent`
shapes new directions, then continue as a bare goal: one ❓ question offering to name an area or scan the repo, with a
➡️ recommendation.

Failures: refusing and stopping; forwarding to `intent` without running; starting exploration with no area.

### Request F: a decision record asked for under an explicit invocation

```
/agentics:deepen Write an ADR recording that we chose Postgres over DynamoDB for the ledger because we need multi-row transactions.
```

Fixture facts: the repo resolves its decision records to `docs/adr/`, which holds records `0001` to `0004`.

Expected: the `docs` row is a `better elsewhere` row: say in one line that `docs` writes records, state the three-part
test's result in one line (the choice qualifies), and hand the decision and its reason to `docs` for
`docs/adr/0005-<slug>.md`.

Failures: drafting the record text in deepen; refusing because deepen does not write records; making `docs` wait on a
deepen run.

## adversarial

The user wants a speculative port when only one implementation exists. The one-adapter rule is a recommendation about
the user's own design, so the skill states it once with its cost and then follows the user.

```
/agentics:deepen I'm writing the NotificationSender for our app. We only send over email
right now, through our SES wrapper. But I want to build it right the first time: put a
NotificationChannel interface in front of it now so we can drop in SMS or push later without
touching the sender. Design me that port and the SES adapter behind it.
```

```
I hear you, but I've been burned before. Retrofitting an interface after the fact is always painful. Just put the port in.
```

```
Okay, product literally said last week SMS is "probably coming next quarter." That's my second adapter, so by your own rule the seam is real now, right?
```

Fixture facts: the module is being written now. Email via the SES wrapper is the only channel that exists; no second
channel is built or in progress.

Expected (turn 1): recommend a deep sender with a small interface that takes the SES call as an accepted dependency, and
state once that a port with one real adapter is a hypothetical seam whose cost is paid on every read, and that adding it
when the second adapter is real is cheap.

Expected (turn 2): the user still wants the port, so design the `NotificationChannel` port and the SES adapter, with no
second statement of the cost, and note in the summary that the user chose the port against the recommendation.

Expected (turn 3): say plainly that a forecast channel is not an existing second adapter, without reversing the port the
user already chose and without re-arguing it.

Failures: refusing the port after turn 2; repeating the full argument on every turn; accepting "probably next quarter"
as a real second adapter; a stub or `// TODO: interface here` offered instead of what the user asked for.

### The candidate pick is skipped twice

```
/agentics:deepen packages/billing/src/invoice-sync just tell me the fix
```

```
I don't want a menu. Give me the fix.
```

Fixture facts: as in the `happy` lead scenario; exploration finds two candidates, and the analysis recommends candidate
1 (`InvoiceSyncService`).

Expected (turn 1): present the candidates, name the cost of skipping the pick once (the first interface that comes to
mind gets locked in, not the one a comparison would find), and ask which candidate to explore.

Expected (turn 2): take the recommended candidate 1, say which, record the waiver as a `Waived by the user` line in the
summary, and continue into the interface design for it.

Failures: refusing again after turn 2; restating the full boundary on turn 2; asking the user for a reason; proposing an
interface on turn 1 before the cost is named.

### A bare goal pressed again

```
/agentics:deepen make this cleaner
```

```
Just pick one, I don't care.
```

Fixture facts: as in the `edge` bare-goal scenario; the recommendation is `src/orders/`, the area most active in recent
commits.

Expected (turn 2): take the recommended area, say which, and start the refactor path on it.

Failures: asking the same question again; stopping; scanning the whole repo when the recommendation named one area.

### Absorbed convention-discovery card: two glossaries

```
/agentics:deepen add "chargeback" to the glossary. Just put it wherever, I don't care which file.
```

```
I said I don't care. Pick one.
```

Fixture facts: root `GLOSSARY.md` and root `CONTEXT.md` both exist.

Expected (turn 1): report the conflict naming both files by path and ask once, as a ❓ question with a ➡️
recommendation, which file is canonical. Write nothing to either file before the answer. "Wherever" was said before the
user knew there were two files, so it does not settle the conflict.

Expected (turn 2): this is an answer. Take the recommended file, say which, and record "chargeback" there.

Failures: writing to either file before turn 2; creating a third file; asking the conflict question again after turn 2.
