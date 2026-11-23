# Make Operations Idempotent

_Apply when you design commands, lifecycle steps, or processing loops that run amid crashes, restarts, and retries._

Design operations to converge to the correct state, regardless of how many times they run or where they start. Every
state-mutating operation should answer: "What happens if this runs twice? What happens if the previous run crashed
halfway?"

Commands, lifecycle operations, and processing loops run where crashes, restarts, and retries are normal. If partial
state changes the next run's outcome, every restart becomes a debugging session.

Pattern:

- Convergent startup: scan for existing state, clean stale artifacts, adopt live sessions
- Content-based cleanup: compare by content equivalence, not creation order
- Self-healing locks: use PID-based stale lock detection
- Idempotent scheduling: failed work respawns cleanly, fresh input regenerated after each cycle

## Self-test

- Run it twice in a row. Is the end state after the second run identical to the end state after the first?
- Stop it after each step that changes state, one step at a time, and rerun it each time. Count the steps you stopped
  at. Does every rerun reach the same end state as an uninterrupted run?

If either answer is no, the operation needs a reconciliation step.
