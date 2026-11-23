# Drill scenarios

A starter library of three reliability and three security scenarios. Each gives the initial inject, the expected branch,
suggested mid-drill injects, and a debrief watchlist.

These are starting points, not scripts. Customize them for the participants' actual systems. A scenario that names real
services is much more useful than a generic one.

> **Naming convention:** scenarios use kebab-case so they can be invoked as `/agentics:ops drill <scenario-name>` (e.g.,
> `/agentics:ops drill leaked-aws-key`).

---

## Reliability scenarios

### `db-primary-failover`

**Branch:** Reliability

**Initial inject (T+0):**

> "PagerDuty fires: `payment-api: error rate 14% over 3 minutes`. Dashboards show write latency on the primary
> `payments` Postgres instance is 8s P99, up from 40ms baseline. Read replicas are healthy."

**Expected response:**

- IC declares (severity P1 if write path is customer-facing)
- DBA / data-tier subject expert pulled in
- Decision tree: failover to standby vs. wait
- Replication-lag check (RPO trade-off)
- Customer-facing comms while the primary is degraded

**Mid-drill injects:**

- T+10 min: "The standby is at replication lag of 12 seconds." (Forces the RPO conversation.)
- T+20 min: "A second alert fires: `payment-api: queue depth growing`." (Forces queue-shedding decision.)
- T+30 min: "After failover, write latency drops to 50ms but a few writes during the failover window are reported
  missing by customer support."
    - Forces a data-reconciliation conversation. It edges into the security branch through the "is this an attack?"
      question.

**Debrief watchlist:**

- Did the team find the failover runbook quickly?
- Did anyone ask about replication lag before the failover?
- How was the partial-data-loss conversation handled?
- Was customer comms accurate? Did it avoid speculation on cause?

---

### `runaway-query`

**Branch:** Reliability

**Initial inject (T+0):**

> "Customer support reports the dashboard endpoint is taking 30+ seconds to load. APM shows the `dashboard.load` span is
> dominated by a single database query — `select count(*) from events where org_id = $1` — running ~10× longer than the
> previous week."

**Expected response:**

- IC declares (typically P2, since it is a single endpoint and not a full outage)
- Backend / DBA subject expert pulled in
- Containment options: query timeout / circuit breaker / cache / route around
- Investigation: why now? (data growth, index bloat, plan flip)

**Mid-drill injects:**

- T+10 min: "The query plan changed two days ago — the index was dropped." (Forces hypothesis testing about deploys vs.
  data growth.)
- T+25 min: "While you're investigating, a second query — `select * from sessions where user_id = $1` — also slows
  down." (Forces "is the database under broader stress?" question.)
- T+40 min: "A backup job is running concurrently, observed in `pg_stat_activity`." (Reveals a contributing factor.)

**Debrief watchlist:**

- Did the team look at recent migrations / index changes?
- Was a containment ("kill the query, return cached data") considered before deeper investigation?
- How was the backup-job contributing factor surfaced?

---

### `deploy-induced-outage`

**Branch:** Reliability

**Initial inject (T+0):**

> "Synthetic check `homepage.load` fails. Real-user-monitoring shows ~3% of homepage loads returning 500. Last deploy
> was 8 minutes ago."

**Expected response:**

- IC declares (severity scales with traffic, P1 if homepage)
- Decision: rollback or roll forward?
- Communication while the rollback deploys
- Post-rollback verification

**Mid-drill injects:**

- T+8 min: "Rollback executed. Synthetic check passes again. RUM error rate dropping but not to zero."
- T+20 min: "~1% of homepage loads still returning 500 even after rollback." (Forces re-evaluation. Maybe the deploy
  wasn't the cause. Maybe it just exposed a latent issue.)
- T+30 min: "A CDN config change went out at the same time as the deploy." (Reveals a co-deploy that wasn't tracked.)

**Debrief watchlist:**

- Was the rollback decision made quickly?
- Did the team consider the rollback target was known-good?
- Did the team catch the co-deploy of CDN config, or did it take the inject?
- How was the post-rollback "still broken" surprise handled?

---

## Security scenarios

### `leaked-aws-key`

**Branch:** Security

**Initial inject (T+0):**

> "GitGuardian (or your secret scanner) alerts: an AWS access key was committed to a public repo 27 minutes ago. The key
> has access to the `s3-prod-customer-data` bucket."

**Expected response:**

- Security branch declared immediately
- Security lead and forensics scribe paged
- Restricted comms, with no posts to the general engineering channel
- Evidence first, on a clock: the forensics scribe starts the export of the CloudTrail / equivalent audit log events for
  the key. The IC sets the snapshot-and-go clock (5–15 minutes).
- Containment: revoke the key when the export has started, and no later than the clock, then rotate IAM credentials. The
  key is public and in use, so the revocation does not wait for the log review or for a complete export.
- CloudTrail / equivalent audit log review for what the key was used for in the dwell window
- Disclosure decision based on what the audit reveals

**Mid-drill injects:**

- T+5 min: "CloudTrail shows the key was used 14 minutes after commit, from an IP not on your allowlist, to issue a
  `s3:ListBucket` on `s3-prod-customer-data`." (Forces escalation. It may be only reconnaissance, but it is suspicious.)
- T+15 min: "A `s3:GetObject` was issued 12 minutes ago against an object containing customer email addresses." (Forces
  disclosure conversation.)
- T+30 min: "Legal counsel asks: 'how do we determine GDPR notification scope?'" (Forces concrete legal-coordination
  work.)

**Debrief watchlist:**

- Did the team page security on-call before anything else?
- Was the audit log export started before the key was revoked?
- Was the key revoked inside the snapshot-and-go clock, or did evidence work hold it past the clock?
- How was the comms restriction enforced?
- Did the team correctly identify GDPR's 72-hour clock?
- Were detection-tuning gaps noted? (How did the key end up committed in the first place?)

---

### `suspicious-dependency`

**Branch:** Security (supply-chain)

**Initial inject (T+0):**

> "Dependabot alerts on `colors-extra@1.4.7`, a transitive dependency added in last week's deploy. The package was
> released 5 days ago by a maintainer who took over from the original author 6 weeks ago. The package contains an
> obfuscated `postinstall` script that exfiltrates environment variables to an unfamiliar domain."

**Expected response:**

- Security branch declared
- DevSecOps subject expert pulled in for supply-chain expertise
- Forensics: what built artifacts contain the package? Were they deployed? Did they execute the postinstall?
- Containment: pull the build artifacts, rotate any environment secrets that may have been exfiltrated, rebuild with the
  package pinned-removed
- Investigation: did the postinstall actually fire in production?

**Mid-drill injects:**

- T+10 min: "The package shipped to production 4 days ago. Build logs show the postinstall ran during build but the
  container itself does not install dependencies at startup." (Forces "build-time exposure vs. runtime exposure"
  distinction.)
- T+20 min: "Network logs from the build environment show outbound connections to `cdn-static-pkg.io` from the build
  hour." (Confirms exfiltration.)
- T+35 min: "Build environment has access to: AWS deployment role, GitHub deploy token, package-registry credentials."
  (Forces the rotation list.)

**Debrief watchlist:**

- Did the team distinguish build-time from runtime exposure?
- Was the rotation list complete? (All build-environment secrets, not just the obvious ones.)
- How was the SBOM / dependency review used?
- Were detection gaps noted? (Why did this package make it through review?)

---

### `sso-idp-outage-as-access-vector`

**Branch:** Security (potentially reliability)

**Initial inject (T+0):**

> "SSO provider Okta (or your IdP) is down for 22 minutes per their status page. During the outage, your application's
> auth fallback kicks in — username/password against your local user store. After the outage ends, you observe ~40
> successful logins from new IPs to admin accounts."

**Expected response:**

- Branch question: is this opportunistic credential reuse (security incident), or just lots of admins who logged back in
  after the outage (reliability artifact)?
- Forensics on the 40 logins: IPs, geographic distribution, time-correlation with normal admin patterns
- Containment options: force re-authentication, terminate sessions, rotate admin secrets, audit the auth fallback
- Decision: was the auth fallback the right design? Should it be removed?

**Mid-drill injects:**

- T+15 min: "10 of the 40 logins are from IPs in geographic regions where the admin has never logged in before."
- T+25 min: "One of those admins reports they did not log in." (Confirms compromise.)
- T+40 min: "Audit logs show the compromised admin issued an IAM policy change while logged in." (Forces blast-radius
  investigation.)

**Debrief watchlist:**

- How long did it take to flip from "reliability artifact" to "security incident"?
- Was the auth fallback identified as a contributing factor early?
- Was session termination considered as a containment step?
- What detection-tuning gap allowed the legitimate admin's compromise to go unflagged?
- Did anyone surface the design question: "should the auth fallback exist at all?"

---

## Authoring new scenarios

A new scenario follows the structure above. A good scenario:

- Has a specific initial inject that points at real systems the team owns
- Names an expected branch (reliability / security / both)
- Has 2–4 mid-drill injects that escalate or complicate
- Lists debrief watchlist items that point at specific gaps the scenario can surface
- Fits the time-box: 60–90 min for tabletop, 2–4 hours for game day
