# Prove It Works

_Apply after you complete a task, before you declare it done._

Check the real thing directly to verify every task output. Do not infer from proxies, self-reports, or "it compiles."

Unverified work has unknown correctness. Indirect verification (file mtimes, output freshness, agent self-reports,
cached screenshots) feels cheaper than direct observation. A wrong inference that you act on costs far more than a check
of the source.

Check the real thing, not a proxy:

- Check process liveness directly, not indirectly through derived state
- Read the actual value, not a cached or derived representation
- When verification fails, suspect the observation method before you suspect the system

Code and features:

1. Build it (necessary but not sufficient)
2. Run it and exercise the actual feature path
3. Check the full chain: does data flow from input to output?
4. For integrations, test the full communication path end-to-end

Delegation: trust artifacts, not self-reports. When you verify delegated work, inspect the actual output artifact (git
diff, file contents, runtime behavior), not the delegate's summary. Agents report what they intended, not always what
happened.

## Script the check when you can

Write a deterministic script that reruns the same comparison. Keep its output as an artifact that a reviewer can rerun,
not as a one-time eyeball or your word alone. A script that compares old and new compiled output catches what a glance
misses.

Keep the artifact visible for the human. Commit it only for large or complex work whose trail must be auditable later,
like a big port or migration. Most work just needs it visible, not committed.

In Agentics, `/agentics:gate check` is the checkpoint that enforces this posture mid-work. The completion gates enforce
it before "done".

## Self-test

- Did I run the feature path, or did I only build it? A clean build is not evidence.
- Is my evidence the real artifact: the running process, the file contents, or the diff? A timestamp, a cached view, or
  a delegate's summary is a proxy, and the answer is no.
- Would this check fail if the change were reverted? If no, the check proves nothing.
- Can a reviewer rerun my check and get the same result? If no, write the script.
