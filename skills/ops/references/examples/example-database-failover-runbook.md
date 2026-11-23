# Runbook: fail over the primary Postgres instance for `<service>`

> EXAMPLE: Not a live runbook. Illustrative use only. The `Last tested` date below is deliberately old, so an audit
> surfaces findings.

**Version:** 1.3 **Last tested:** 2024-08-15 **Owner:** `<github-handle>` (example placeholder. A live runbook would
name a real person.) **Severity grade:** P1

---

## Purpose

Fail over the primary Postgres instance for `<service>` to its standby when the primary is unhealthy or unreachable.

## When to use

- PagerDuty alert `<service>-db: primary unreachable` fires
- PagerDuty alert `<service>-db: write latency > 5s P99 for 3 minutes` fires
- Customer support reports clustered write failures for `<service>`

## When NOT to use

- Read-only queries are slow but writes succeed. Use the read-replica routing runbook instead.
- The primary is healthy but a downstream service fails. Diagnose the downstream first.
- A planned failover for maintenance. The primary is healthy, so step 1 stops this procedure. Use the
  planned-maintenance runbook instead.
- Replication lag is > 30 seconds. Escalate to the DBA on-call. Failover risks data loss.

## Prerequisites

- **Access:**
    - `kubectl` context `<cluster-name>` with the `sre-operator` role assumed
    - AWS IAM role `<role-name>` assumed in account `<account-id>`
    - PagerDuty access for the `<service>` schedule (to confirm on-call before you page)
- **Tools:**
    - `kubectl` v1.28 or later
    - `aws` CLI v2 authenticated against the appropriate profile
    - `psql` client (any v15+)
- **Context:**
    - You must be on the responder VPN or an authorized network
    - You must declare the incident via `/agentics:ops response` first

## Steps

### 1. Confirm the primary is genuinely down

Do not fail over on a transient blip. A failover on a transient blip doubles the downtime.

```bash
kubectl exec -n <namespace> -i <primary-pod> -- pg_isready -h localhost -p 5432
```

**Verify:** `pg_isready` exits 2 (no response), or `kubectl` cannot reach the pod. The primary is down.

**If verify fails:** Stop. Exit code 0 means the primary accepts connections, so this is not a primary-down scenario. If
write latency is high, proceed to the runaway-query runbook instead. Exit code 1 means the server is running but rejects
connections, for example during startup. That does not prove the primary is down. Page the DBA on-call before any
promotion.

### 2. Confirm the standby is healthy and replication lag is acceptable

```bash
psql -h <standby-host> -U <user> -c "SELECT pg_last_xact_replay_timestamp(), now() - pg_last_xact_replay_timestamp() AS lag;"
```

**Verify:** Lag is under 30 seconds. The standby responds to the query.

**If verify fails (lag > 30s or standby unreachable):** Stop. A failover to a lagging or unhealthy standby loses data or
extends the outage. Escalate to the DBA on-call.

### 3. Drain writes from the application layer

Avoid in-flight writes during the cutover. Record the current replica count first, because step 7 and the rollback
restore it.

```bash
kubectl get deployment/<service>-writer -n <namespace> -o jsonpath='{.spec.replicas}'
kubectl scale deployment/<service>-writer -n <namespace> --replicas=0
```

**Verify:** The first command prints the replica count. Write it down as `<original-count>`.
`kubectl get pods -n <namespace> -l app=<service>-writer` returns zero pods. Wait 30 seconds for in-flight requests to
drain.

**If verify fails:** Pods did not scale down. Check for a HorizontalPodAutoscaler that may counteract the scale. Disable
it before you retry.

### 4. Fence the old primary

Stop the old primary so it cannot return and accept writes after the standby is promoted. Two writable primaries split
the data.

```bash
kubectl scale statefulset/<primary-statefulset> -n <namespace> --replicas=0
```

**Verify:** `kubectl get pods -n <namespace> -l app=<primary-label>` returns no pods.

**If verify fails:** The old primary is still running. Do NOT promote the standby. Page the DBA on-call.

### 5. Promote the standby

> **Warning:** Step 5 is the first irreversible step. Once promoted, the standby becomes the new primary. The old
> primary cannot be re-attached unless you rebuild it.

`<pgdata-dir>` is the standby's data directory. `SHOW data_directory;` prints it.

```bash
kubectl exec -n <namespace> -i <standby-pod> -- pg_ctl promote -D <pgdata-dir>
```

**Verify:** The standby logs show `database system is ready to accept connections` (no longer "in recovery"). Run
`psql -h <standby-host> -c "SELECT pg_is_in_recovery();"`. It should return `f` (false).

**If verify fails:** The standby did not promote. Do NOT retry. A partial state may exist. Page the DBA on-call
immediately.

### 6. Update the application's DB endpoint

Point `<service>` at the new primary.

```bash
kubectl set env deployment/<service>-writer -n <namespace> DB_HOST=<new-primary-host>
```

**Verify:** A new deployment revision rolls out. `kubectl rollout status deployment/<service>-writer -n <namespace>`
returns success.

**If verify fails:** The rollout did not complete. Check for a deployment-blocking signal (resource quota, image pull
failure). Page the on-call SRE.

### 7. Scale writes back up

```bash
kubectl scale deployment/<service>-writer -n <namespace> --replicas=<original-count>
```

**Verify:** The pods come up healthy. The synthetic write check passes 3 consecutive runs.

**If verify fails:** Writers cannot connect. Re-check `DB_HOST` env var. Test the connection from a debug pod.

## Verification (end-to-end)

After all steps:

- Dashboard `<service>-writes-per-second` returns to baseline (within 20% of pre-incident rate) for 10 consecutive
  minutes
- Dashboard `<service>-write-latency-p99` is under 200 ms for 10 consecutive minutes
- Synthetic check `<service>.write` passes 5 consecutive runs
- No new alerts on `<service>` in the 10 minutes after step 7

If any verification check fails, proceed to **Rollback**.

## Rollback

Steps 1–4 are reversible. **Step 5 (promotion) is irreversible.** Once promoted, the old primary cannot be re-attached
unless you rebuild it from a base backup. After step 5, "rollback" means rebuild-from-backup, not undo. Steps 6 and 7
run after that point, so correct them going forward (re-check `DB_HOST`). Do not undo them.

### Reverse steps 1–4 (before step 5)

If a step fails before step 5, undo only the steps that ran. Steps 1 and 2 change nothing, so a stop there needs no
undo.

1. **Undo step 4, only if step 4 ran:** scale the old primary back up with
   `kubectl scale statefulset/<primary-statefulset> -n <namespace> --replicas=1`.
2. **Undo step 3, only if step 3 ran:** scale writes back to `<original-count>`, the number you wrote down in step 3.
3. **Re-check primary health:** if the primary has recovered, the failover was a false alarm. Close out.

### After step 5: rebuild-from-backup

If verification fails after step 5:

1. **Declare a fresh P1 via `/agentics:ops response`** if you have not already declared one.
2. Page the DBA on-call.
3. Do not attempt to re-promote the old primary.
4. The recovery path is restore-from-backup, which is its own procedure. See [rebuild-postgres-from-backup.md](#)
   (placeholder. Link to the real runbook in production.)

## Escalation

| Trigger                                       | Action                                                                                      |
| --------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Step 4 (fence) fails                          | Stop. Do NOT promote. Page DBA on-call immediately.                                         |
| Step 5 (promote) fails or partially completes | Stop. Page DBA on-call immediately. Do NOT retry promote.                                   |
| Verification fails AND rollback fails         | Declare P1 via `/agentics:ops response`. Open a war room. DBA on-call and SRE on-call join. |
| Procedure takes longer than 45 minutes        | Page secondary DBA on-call. Reassess severity and customer impact.                          |
| Operator unsure at any step                   | Stop. Page DBA on-call. Ambiguity in production is worth a page.                            |

## Related runbooks

- [`rebuild-postgres-from-backup.md`](#), when failover failed and the only recovery is restore-from-backup
  (placeholder)
- [`runaway-query-mitigation.md`](#), when writes are slow but the primary is up (placeholder)
- [`promote-standby-planned-maintenance.md`](#), variant of this procedure for planned maintenance windows (placeholder)

## Last incident this was used in

| Date       | Incident                    | Outcome   | Drift observed |
| ---------- | --------------------------- | --------- | -------------- |
| 2024-08-15 | [Example incident #1234](#) | Succeeded | None           |

## Change history

| Date       | Author     | Change                                                                                       |
| ---------- | ---------- | -------------------------------------------------------------------------------------------- |
| 2023-04-02 | `<handle>` | Initial version                                                                              |
| 2023-11-10 | `<handle>` | Added the irreversibility warning to the promote step                                        |
| 2024-08-15 | `<handle>` | Last drilled; minor wording fixes from drill findings                                        |
| 2026-11-11 | `<handle>` | Added step 4 (fence the old primary); removed the planned-failover trigger; renumbered steps |
