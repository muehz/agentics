# Separate Before Serializing Shared State

_Apply when concurrent actors might write to the same file, branch, key, or state object._

When concurrent actors might share mutable state, first ask whether they truly need the same mutable object. If not,
eliminate the sharing. When sharing is real, enforce serialization structurally: lockfiles, sequential phases, exclusive
ownership. Instructions and conventions are not concurrency control.

Concurrent writes to shared state create race conditions that are intermittent, hard to reproduce, and expensive to
debug. An instruction to agents or goroutines to "take turns" does not work.

Pattern:

1. **Identify shared mutable state** (files both read and write, branches both push to, APIs both define and consume).
2. **Default: eliminate the shared write target.** Ask: do these actors need one canonical object, or do they publish
   independent facts? Give each actor its own owned file, key, branch, or state directory. Merge only at the
   read/reporting boundary. If two workers write their own `lastX` field into one `state.json`, that is still shared
   mutation. `indexer-state.json` + `metrics-state.json` is not shared mutation.
3. **Only when one shared write target is a real invariant, serialize access structurally.** Use lockfiles, sequential
   phases, a single-writer actor, or atomic compare-and-swap. Treat "we need a lock" as a design smell to check, not as
   the default answer.

## Self-test

- Which file, branch, key, or object do two actors both write? Name it. An empty list means this principle does not
  apply.
- Do these actors publish independent facts? If yes, each gets its own write target. If they need one canonical object,
  the sharing is real.
- If the sharing is real, does a lock, a sequential phase, a single writer, or an atomic compare-and-swap enforce the
  ordering? An instruction that tells the actors to take turns does not count, and the answer is no.
- If both actors write at the same instant, does the result stay valid? If the answer depends on timing, the design is
  not done.
