# Security branch

Load this file when the declare-phase branch question gets "yes" or "unsure". All response work uses restricted comms
and preserves evidence until the security lead clears the branch. When nobody else holds the security lead role, the
user makes the security lead's decisions.

## Security triggers (illustrative)

- Active exploitation of a known vulnerability
- Credentials leaked in a public location (paste site, public repo, public bucket)
- Anomalous authentication: mass-failure spikes, geographic outliers, off-hours admin actions
- Outbound traffic to unfamiliar destinations, especially from privileged hosts
- New / modified accounts, IAM roles, keys, or service credentials not tied to a known change
- Logs that show access to data the affected service does not normally read
- Suspicious package, dependency, or build artifact (supply-chain compromise indicator)
- A scanner finding (Checkmarx, Dependabot, secret scanner) matched in time with anomalous behavior
- User reports that suggest account compromise (unfamiliar logins, password resets they did not request)

## The two clocks

The IC must respect both competing pressures throughout the incident.

| Clock                 | Drives toward          | Tension                                                                                          |
| --------------------- | ---------------------- | ------------------------------------------------------------------------------------------------ |
| **Containment clock** | Stop the active threat | If you push too hard, you contaminate evidence, tip off the attacker, and trip disclosure clocks |
| **Forensics clock**   | Preserve evidence      | If preservation takes too long, the threat continues to operate                                  |

Default policy: **5–15 minute snapshot-and-go.** Pull a snapshot of the impacted system. Then act. Partial forensics is
better than no forensics or an indefinite stall.

## Restricted comms (load-bearing)

The first comms decision is who knows. Default scope:

- **Initial recipients:** named security on-call, IC, comms lead (only if needed for external comms), legal/compliance
  lead (if available), exec-on-call (P1 only).
- **Not initial recipients:** general engineering, customer support, sales, public status page.
- **Channel:** a dedicated private channel or a direct-message thread. Not the general engineering channel.

Use the **security-incident restricted comms template** from
[../communication-templates.md](../communication-templates.md). Draft nothing for public engineering channels until the
security lead approves a broader announcement.

A broadcast of "we think we have a breach" can tip off attackers, contaminate evidence, and start regulatory clocks
prematurely. It can also leak to media before the company is ready.

## Where the record lives

The tracking record and the postmortem describe the weakness, so their place depends on who can read the repository.
Read the repository's visibility before drafting either. A visibility that cannot be read counts as public. Read the
rows from the top; the first match wins.

| Order | Repository                                                    | Where the record goes                                                                              |
| ----- | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| 1     | Public, internal, or unreadable                               | A draft security advisory on the repository, or a private location the user names. Never an issue. |
| 2     | Private, and the user says its readers are the right audience | A tracking issue with the `security` label, with no attacker indicators and no channel names.      |
| 3     | Otherwise: private, with the audience not settled             | Ask once whether everyone with read access may see it, and recommend the draft advisory.           |

An issue on a private repository is visible to everyone with read access, so a private repository is not a restricted
audience. GitHub has no private issue. Indicators of compromise never go into an issue on any repository.

## Evidence preservation (run before containment)

The forensics scribe captures, in this order, before any containment action:

1. **Snapshot impacted hosts / containers / volumes.** Prefer cloud-snapshot APIs, because they are write-protected and
   quick.
2. **Pull logs to a separate write-protected location.** Application logs, system logs (auth, syslog), cloud audit logs
   (CloudTrail, Audit Logs, equivalent). Choose a window of at least 7× the suspected dwell time. If dwell time is
   unknown, choose 24 hours.
3. **Capture process / network state** (`ps`, `netstat`/`ss`, `lsof`) on each impacted host before it is mutated.
4. **Hash any suspicious artifact.** Binaries, scripts, packages, build artifacts. SHA-256 minimum.
5. **Document chain of custody.** Who pulled what, when, where it is stored, who has access. The chain is part of the
   evidence.

If time pressure forces you to skip a step, log it explicitly. Example: "did not snapshot host X — IC accepted forensics
gap to contain." That note matters in the postmortem and any later legal review.

## Containment options

Use the same general principles as reliability:

- Prefer reversible changes.
- Make one change at a time.
- Log every action.

The actions differ:

| Action                                         | Reversibility                | Use when                                                                  |
| ---------------------------------------------- | ---------------------------- | ------------------------------------------------------------------------- |
| **Revoke credentials** (API keys, tokens, IAM) | Yes (re-issue)               | Suspected credential compromise. Almost always safe to do early.          |
| **Rotate shared secrets**                      | No (old value stays retired) | After containment of immediate access vector. Needs explicit IC approval. |
| **Terminate sessions**                         | Yes                          | If session hijack suspected. May log out legitimate users.                |
| **Isolate hosts** (network deny)               | Yes                          | Suspected compromised host. Quarantine before rebuild.                    |
| **Block IP / subnet at edge**                  | Yes                          | Active exploitation from a known source                                   |
| **Disable account / role**                     | Yes                          | Suspected insider or compromised internal identity                        |
| **Pull suspicious deploy artifact**            | Yes                          | Supply-chain indicator                                                    |
| **Force-deploy clean build**                   | Mostly yes                   | If a deployed artifact is itself compromised                              |
| **Take service offline**                       | Yes                          | Last resort — when isolation is incomplete and exposure continues         |
| **Wipe and rebuild**                           | No                           | Only after forensics; only with security-lead approval                    |

## "Tip-off" risk

Take care with any action that an attacker already inside your systems can observe:

- Do not post status updates that reveal what you have found
- Do not push a commit titled "fix breach"
- Do not suddenly flush all sessions if that is not consistent with normal operations. A flush forces the attacker to
  re-establish, which loses your detection signal.
- Coordinate with the security lead before any containment action visible from inside the affected system

## Disclosure clocks (legal coordination)

Several regulations have notification SLAs that start at "knowledge of breach." Interpretation varies. Treat the IC's
first reasonable belief as the start.

- **GDPR (EU):** 72 hours to supervisory authority for personal-data breaches
- **HIPAA (US, healthcare):** 60 days to affected individuals
- **State-level (US):** varies, and many states have 30–90-day notification windows
- **Contractual:** customer agreements often have explicit breach-notification clauses

The IC does not interpret regulations alone. **Bring in legal/compliance early.** The situation may not require
disclosure, but the calendar matters, and the security lead needs time.

## When the security lead clears broader comms

After triage and initial containment, the security lead decides:

- **No customer comms required:** internal wrap-up only, postmortem internal
- **Internal-only broader announcement:** engineering-channel post that explains the resolved issue, no external comms
- **Customer notification (private):** affected customers contacted directly with details
- **Public disclosure:** status-page or blog post; coordinate with comms lead and PR

The comms lead can draft the first three from [../communication-templates.md](../communication-templates.md). The fourth
almost always needs PR / legal review before it is posted.

## Hand-off to `review`'s Security lane

If containment needs a deeper read for evidence collection, invoke the `review` skill in `axes` mode with `--security`
and `--composed`. Pass the selected ref and the review target as skill inputs. The target may be a pull request, branch,
path or the full repository.

The Security lane of `review`'s `axes` mode runs six parallel domain reviewers. The reviewers cover OWASP/injection,
auth/authz, secrets, dependencies, IaC, and data flow and trust boundaries. The lane accepts a target wider than a diff,
which is what evidence collection needs. `--composed` returns findings as a JSON envelope per the
[composition contract](../../../review/references/security/security-principles.md) instead of any posted output. The
skill shows the envelope to the IC and the forensics scribe as a draft, per this skill's draft-and-confirm rule. The
findings then feed the postmortem's contributing factors and action items.

**Read the envelope's `limits` field into the incident record verbatim.** The lane reads code; it runs no scanner. An
empty `findings` array during an active incident means this reading surfaced nothing, not that the system is
uncompromised.

## Hand-off triggers (re-evaluate the branch)

Security incidents rarely downgrade. Stay on this branch until the security lead explicitly clears it. The exception is
a conclusive false positive, such as undocumented traffic from a known-good crawler. The security lead can then
downgrade to reliability with a note in the tracking issue.

Even after downgrade, the postmortem still treats the false positive as an incident. The detection-tuning gap is a real
finding.

## Postmortem implications

The postmortem-mode template handles three security-postmortem differences:

- **Restricted distribution by default.** The postmortem goes where [Where the record lives](#where-the-record-lives)
  says. A public version comes only after security-lead and legal review.
- **No IOC publication.** Indicators of compromise stay in restricted channels, because public release of them helps
  copycat attackers.
- **Action items often span multiple teams.** Detection tuning, IAM tightening, secrets rotation policy,
  incident-response process. Track owners and due dates with extra rigor.
