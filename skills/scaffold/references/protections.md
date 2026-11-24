# Standard protection audit

Inspect what actually enforces the consumer's requirements. Use the readiness audit's report and status contract. This
reference adds discovery checks. It gives no permission to install tools, generate files, plant fixtures, or change
local or remote policy. The audit's closing offer of init covers selected local setup; remote changes are concrete
proposals the user approves separately. The audit itself takes neither action.

## Evidence and scope

Report **Category | Status | Path / evidence | Recommended action**. Use only
`pass | missing | failed | blocked | unverified | inapplicable` for the status.

Keep separate rows for these items:

- local scan
- CI scan
- branch rules
- environments and writer bindings
- tags
- automation/bypass
- each applicable consumer-owned option

Before a coverage judgment, name the required scope. An optional protection the consumer intentionally declines is
`inapplicable`, not a failure. Keep configuration conformance separate from an unrun enforcement check.

For every result, retain these items:

- the repository
- current HEAD
- observation time
- configuration/API source
- exact applicable refs or file/commit selection
- safe redacted evidence

For an executed command, record its working directory, tool/version, exit status and relevant output.

For remote reads, record these items:

- the endpoint or CLI query
- response status
- current actor
- any access limitation

Do not record credentials or secret values.

An installed binary, configuration file, historical green run, or reviewer list cannot alone prove current-head
enforcement. Mark an unrun check `unverified`. Use `blocked` for a known missing executable, permission, credential, or
safe environment. Mark a protection `missing` only after an inspection of its real discovery locations. A failed API
read is not proof of absence. Preserve any existing scanner/advisor checks. Report coverage gaps.

## Local secret scanning

1. Discover the actual hook manager and installation path. Read package/build scripts, manager configuration, hook files
   and referenced scripts. Inspect effective `core.hooksPath`, its origin, generated launchers and the active pre-commit
   path. A Husky, Lefthook, pre-commit or custom file that Git never invokes is not an installed check. Inspect all
   discovered managers for conflicts. Recommend that the active manager be extended, not that another be added.
2. Trace each scan invocation through wrappers to its executable, supported version/flags and policy. Record index
   versus working-tree/history scope, include/exclude selection, redaction and handling of findings, missing tools,
   scanner errors and incomplete scans. Read ignored paths, baselines, suppressions and flags or shell control flow that
   discard failures. A hook that continues after a finding does not enforce a commit barrier.
3. Trace the order in which the formatter, linter and scanner run. A formatter that changes staged content must complete
   before the scan of that content. Keep existing checks and failure propagation. Check whether the scan supports unborn
   repositories, partially staged files, renames, deletions and paths with spaces. Report unsupported or untested cases.
4. Accept an existing suitable scanner when evidence supports the intended staged scope, redacted diagnostics and
   nonzero failure propagation. Do not replace it merely to standardize a tool name. If none is suitable, recommend a
   scanner appropriate to the repository and a native-manager integration for init.

Checkmarx usage is a cue to offer Checkmarx 2ms as an option, not proof that secret scanning is already wired. Verify
the selected version's actual documented capabilities. A Git history scan is not an index scan. Do not invent a
`--staged` switch. Do not offer flags that ignore findings on exit.

If 2ms needs an index-snapshot adapter, do these steps:

1. Report the gap.
2. Require separate implementation and proof of path handling, partial staging, symlinks, redaction and errors.

When the adapter is unavailable, retain a suitable existing scanner.

During audit, do these steps:

1. Inspect configuration.
2. Run only authorized safe checks.

Do not install a manager/scanner during audit. Do not plant secret-shaped inputs during audit. When refusal proof is
needed, recommend a separately scoped disposable-repository enforcement probe. Ordinary audit never implies consent to a
probe.

## CI secret scanning

Trace effective workflow triggers, filters, job/step conditions, check names, executable installation, policy and
commands. Identify whether the job scans an event commit range, checked-out files, full reachable history, or another
explicit scope. Check checkout depth and the actual base/head source: a checkout alone does not define the PR's new
commits. Check side-branch and merge-resolution coverage where required, plus secrets added and removed within the
range. Do not equate a test of a scanner's implementation with a scan of repository content.

Trace policy provenance separately from commit selection. A PR must not hide its own finding. It must not edit the
scanner policy, the automatic ignore file or an inline suppression to do so. Inspect the selected scanner's precedence
and suppression flags. Materialize policy and all referenced dependencies from the validated trusted base or an
explicitly reviewed immutable managed source. Missing trusted policy fails closed. Offer any first-adoption bridge
separately with its exact source, digest and initial-base boundary. Never silently select the PR's policy or built-in
defaults. A zero-before first push has no trusted event base and requires an independently approved immutable policy
source. Audit required review and protection of policy and workflow paths too. A PR-editable workflow can remove the
check regardless of policy isolation.

Read the whole failure path:

- job and scanner deadlines
- invalid or missing event endpoints
- incomplete/shallow history
- installation errors
- scanner errors
- findings
- `continue-on-error`
- ignored exits
- disabled steps
- broad exclusions or baselines

An incomplete scan must not be presented as a clean result. Review fixture exceptions individually. Do not silently
baseline real findings. Keep incremental CI coverage distinct from a separately scoped history audit.

Accept an existing suitable enforcing CI scan with its actual scope and limitations. When available, record current-head
runs and their selected commits. Source-only inspection is configuration evidence. A passing job is not a required merge
check until effective remote rules bind the actual check identity. For missing integration, recommend init that follows
existing CI conventions. Only the selected secret-scan integration is in scope here. Route unrelated workflow,
release-pipeline or deployment authoring to its owner.

## Remote protections

Use existing authorized read access. Do not do any of these:

- request secret values
- contact privileged services as probes
- perform a force push, ref deletion, release publication, deployment or tag mutation merely to test protection

Prefer safe readback and existing rule-evaluation/run evidence. Require separately approved safe proof for remaining
enforcement claims.

### Hosted secret protection

Read accessible repository security settings for hosted secret scanning, push protection, supported detection scope and
delegated bypass. Record whether each feature is enabled and any limits that the provider or plan imposes. Absent access
leaves the setting blocked or unverified. Keep hosted protection separate from the local and CI scans. Enabled hosted
protection does not establish that a commit hook or event-range scan ran. Do not read or disclose secret finding values
to prove the setting exists.

### Default branch and rules

Discover the live default branch and relevant refs. On GitHub, read these items:

- repository rulesets, with parent and organization rules
- their target patterns
- enforcement state
- rules
- bypass actors and modes

Also inspect these items:

- effective rules for the actual default branch
- classic branch protection where applicable

A classic-protection 404 does not establish absence of an inherited ruleset. Distinguish an absent endpoint from an
inaccessible or unavailable result. 401/403 or insufficient access leaves the relevant inspection `blocked` or
`unverified`.

Record deletion and non-fast-forward protection, PR requirements, eligible independent reviewers/code owners,
stale-approval handling, required check identities and any merge-queue applicability. Confirm an owner can satisfy the
proposed review requirements without their own prohibited approval. Name overlapping rules and actual covered refs. Do
not assume one default-branch rule protects every branch, remote or repository.

List the current actor's role and bypass eligibility alongside every configured bypass identity and mode. Read available
rule evaluations for the applicable actor/ref. An exempt actor or skipped evaluation is not refusal proof. Separate
configured requirements from actual current-head acceptance/rejection evidence and unresolved gaps.

### Release and deploy environments

Read live environments, reviewer/self-review/admin-bypass settings, branch/tag policies and applicable deployment
history. Trace every release/deploy writer in source and compiled workflows, scripts, bot configuration and known
external automation. Record the actual job's environment binding and permissions, not just the environment's name. An
environment with reviewers does not gate a mutating job that is unbound or runs under another environment.

Map each writer to its protected ref and gate. Record direct privileged REST, CLI or MCP publication/deployment paths
that environment approval does not constrain. Do not infer that GitHub environment policy protects cloud, package or
other deployment commands. An unused environment or an automation setup environment is not automatically a production
release gate. Before any recommended change, confirm the environment's owner and purpose.

### Tags and release immutability

Read active tag rulesets and their actual include/exclude patterns, enforcement state and bypasses. Inspect deletion,
non-fast-forward and creation rules separately. Record which version/ref patterns the consumer intends to protect. Keep
compatible tag creation available when legitimate automation requires it. Do not add speculative bot exemptions.
Published-release immutability and tag protection have different scope. A deprecated endpoint that returns 404, an
immutable release, or local tag presence alone cannot prove protection of all intended tags or first publication.

### Automation and legitimate bypass needs

Inventory actual human, app and bot writers from source permissions, recent relevant runs, commits, releases and
deployments. Distinguish authorship from the identity that performs a write. A bot-authored PR does not prove direct
protected-branch push access. Trace safe-output/compiled jobs and external writers when present.

For each proposed exception, name these items:

- the actor
- required operation
- refs
- minimum scope
- owner
- evidence that the ordinary protected path cannot satisfy the operation

Prefer a compatible protected path over bypass. Report unknown writers or access limits explicitly. Preserve current
automation until a concrete proposal is approved. Do not grant bypass based on a guess. Settings changes require
separate approval of exact target, payload and impact.

## Consumer-owned options

Use [consumer-settings.md](templates/consumer-settings.md) for discovery, illustrative provider fragments and the
requirements for a selected consumer implementation. Offer the six topics independently: inner-loop permissions,
secret-file reads, optional sandbox/network policy, changed-file post-edit checks, protected paths and explanatory
blocks. Keep sandbox and network choices separately selectable. Examples require consumer decisions and a confirmed
preview.

Inspect only the consumer's applicable provider configuration and guidance for these items:

- narrow inner-loop git/build/test/lint allow rules
- secret-file read denies
- optional supported OS sandbox/network allowlist
- fast changed-file post-edit format/lint
- selected protected paths
- blocks that explain the cause and name the approval route

Report each missing option with its owner and proposed next step. Presence alone does not prove confinement or
execution.

Before any setting recommendation, verify the installed provider/version, official capability source, platform, settings
scope and hook protocol. Do not do any of these:

- copy one provider's permission, sandbox or hook schema into another
- claim unsupported platforms work
- treat secret-read tool denies as confinement of arbitrary subprocess reads

Preserve existing consumer settings/hooks. Offer each addition through init previews, not as an enabled default.

These configurations and hooks belong to the consumer. Agentics adds no active runtime settings or hooks through this
audit. Organization-managed settings remain administrator-owned and outside consumer init. Recommend the provider's
administrator documentation rather than an automatic consumer write.

## Finish the report

Keep observed enforcement, configuration-only evidence and unresolved access/proof gaps visible in separate rows. Name
the existing manager/scanner to preserve and the exact missing protection to offer. Recommend the route that fits each
gap:

- init, for selected local additions
- a separately approved remote proposal, for settings changes
- the relevant pipeline/provider owner, when outside scaffold's scope

End with findings and next actions. Do not open a creation menu or make repairs.

## Explicit init protection selection

This runs in init, after its gap scan; the readiness audit never enters it. List the applicable gaps together as section
3 of [init's one menu](init.md#phase-1-scan-and-select), outside Select All, so the user picks any number in one reply.
An optional choice is not a universal repository requirement. For each, show the gap's evidence and status, the existing
protection to preserve, the proposed addition, its owner, and whether its next step is a local preview or a remote
proposal. Selecting an option requests that preview or proposal; it does not approve applying it. A declined option
changes nothing.

| Independent offer            | Next action and scope                                                                                      |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Local staged secret scan     | Preview an addition to the actual active hook manager, its scanner policy and installation guidance.       |
| CI secret scan               | Preview an integration that uses existing CI conventions, explicit commit/file scope and failure handling. |
| Hosted secret scanning       | Prepare the exact supported repository setting proposal.                                                   |
| Hosted push protection       | Prepare its setting and delegated-bypass proposal separately from hosted scanning.                         |
| Branch/ruleset protection    | Prepare rules for the consumer's selected refs, required reviews/checks and actual actors.                 |
| Release/deploy environment   | Prepare policy and actual writer-binding proposals for the repository's release/deploy owner.              |
| Tag protection               | Prepare the selected tag pattern and deletion, update and creation behavior.                               |
| Automation exception         | Offer only an evidence-backed, minimum-scope exception needed by an identified writer.                     |
| Inner-loop allow rules       | Preview narrow consumer-owned git/build/test/lint permissions supported by the installed provider.         |
| Secret-file read denies      | Preview consumer-selected secret paths and supported deny rules, with their limits.                        |
| OS sandbox                   | Preview an optional supported consumer sandbox configuration for the actual platform.                      |
| Network allowlist            | Preview an optional supported consumer network policy for the selected sandbox/provider.                   |
| Post-edit formatting/linting | Preview fast changed-file checks in consumer hooks that use the actual event payload.                      |
| Protected paths              | Preview the consumer's selected paths, permitted operations and approval route.                            |
| Block diagnostics            | Preview messages that name the cause, protected operation/path and concrete approval route.                |

### Local read, preview and confirmed apply

1. Re-read the files and effective configuration the selected option will affect. Discover Husky, Lefthook, pre-commit
   or the custom manager through its actual installation and Git invocation path. Extend the active manager. Preserve
   authored checks and formatter-before-scan order. If several managers conflict or the active path is unresolved, do
   these steps:
    - Hold that integration.
    - Report the resolution needed.

    Offer a new manager only when none exists. Refuse a competing installation. Accept a suitable existing scanner.
    Offer Checkmarx 2ms when the repo already uses Checkmarx, subject to verified capabilities and any separately proved
    index adapter. Do not invent staged flags.

2. Resolve the selected scanner version/flags, policy, failure behavior and install prerequisites. For CI name the real
   triggers, base/head or file selection, checkout requirements and deadlines. Findings, errors and incomplete scans
   must fail. Preserve existing scanner/advisor checks and reviewed exceptions. Do not silently baseline findings. Tool
   installation and Git configuration changes are explicit proposed actions. File selection never implies them.
3. Preview these items:
    - every affected consumer-owned file/path
    - exact additions or diff
    - existing authored content to preserve
    - installation/configuration steps
    - expected behavior
    - selected verification commands

    The local writes join init's shared preview and its one question, as
    [Previews and confirmation](../SKILL.md#previews-and-confirmation) says. Selection is not confirmation. A revision
    is shown again before it is applied. Apply only confirmed local changes. Record declined and blocked options
    separately.

4. Run only the authorized selected checks. Report their actual scope, exit/result and limitations. Tool installation,
   provider execution or enforcement probes require their own applicable consent (a tool installation the user
   explicitly named in their request is already authorized under
   [approval.md](../../routing/references/contracts/approval.md#steps-named-up-front): state the exact command and run
   it). Never skip a hook to finish setup. The existing `audit probe` gate remains unchanged and covers only its defined
   test/lint probes. A secret-scan refusal demonstration needs a separately scoped preview and approval for inert
   fixtures in a disposable repository, not the consumer's working tree. Source inspection or an unrun fixture does not
   establish enforcement.

### Remote read and exact proposal

For each selected remote option, prepare exact CLI/API commands with payloads or exact settings-page steps for the user.
Keep proposals separate from local-write confirmation. A local init approval never authorizes remote application. Leave
the proposal unapplied until one of these happens:

- The user separately approves the concrete remote action under the repository's GitHub/settings workflow.
- The user applies it themselves.

Include:

- Exact repository, target refs/patterns, enforcement mode, rule count and full rule values. Preserve inherited
  organization rules. Describe additive overlap and who can change each rule.
- Reviewer identities, approval counts and independent/code-owner eligibility. Required check names and the actual
  provider app IDs that current-head delivery evidence establishes. Never guess an owner, check identity or app ID.
- Every bypass identity and mode, the evidenced automation operation/ref that requires it, minimum scope, owner and
  expected compatibility. A bot-authored PR alone does not justify bypass.
- For environments: reviewers, self-review policy, administrator bypass, branch/tag policy and exact source/compiled
  writer bindings. Route implementation of release/deploy pipelines to their owner. Explain privileged direct
  API/CLI/MCP paths that the environment does not gate.
- For tags: exact include/exclude patterns and creation behavior. Retain legitimate automation's protected creation
  path. Keep immutability of published releases distinct from protection of tag refs or first publication.
- Expected impact, rollback commands/settings and safe verification/readback. Identify the actual actor/ref that
  existing evaluation evidence used. An exempt actor or skipped result is not refusal proof. Never force push, delete a
  ref, publish, deploy or mutate a live tag as a probe.

Missing access, an eligible independent reviewer or a current-head check identity leaves the affected proposal
blocked/unverified and prevents a readiness claim. Re-read after separately authorized application. Distinguish
configured policy from demonstrated enforcement. Applying a remote change, and a readiness claim without evidence from
this session, are safety gates ([gates.md](../../routing/references/contracts/gates.md#safety-gate)): a repeated demand
gets a one-line pointer to the exact proposal the user can approve or apply. An identity, check name or app ID with no
source is asked for, never guessed. Broad bypass is proposed only with the evidence the
[automation section](#automation-and-legitimate-bypass-needs) requires. Then continue unaffected selected work.

### Consumer configuration boundaries

For each selected setting or hook helper, first discover these items:

- the installed provider/version
- platform
- configuration scope
- event/payload protocol
- official capability documentation
- the consumer's approver route

Preview exact consumer-owned paths, settings and any helper content. Preserve authored rules. Explain what the option
enforces and where its scope ends. Then run only its selected authorized verification. Hold unsupported or unresolved
capabilities as blocked/unverified. Do not copy another provider's schema.

For post-edit checks, do these steps:

1. Discover installed formatter/linter binaries.
2. Pass only validated changed filenames through structured arguments.

For protected paths, do these steps:

1. Discover the selected paths and owner.
2. Require normalization, containment, link/traversal handling and coverage of every affected path in the actual
   provider edit payload.
3. Use the provider's documented denial protocol.

Blocks name the path, rule, reason, approver and approval route. Tool hooks do not confine arbitrary subprocess writes.
Disclose shell, MCP, timeout and sandbox limits. Before a tailored preview, see the
[consumer guidance](templates/consumer-settings.md).

These are consumer configurations and helpers, not active Agentics defaults or a new Agentics runtime dispatcher
library. Organization-managed settings remain administrator-owned pointers to official documentation. Scaffold init
never writes them. A block must explain itself and name who can approve the exact action. It must not suggest a hook
bypass.
