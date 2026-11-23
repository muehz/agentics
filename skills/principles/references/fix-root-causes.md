# Fix Root Causes

_Apply when you debug._

When you debug, do not paper over symptoms. Trace every problem to its root cause. Fix it there.

Symptom fixes accumulate. Each workaround makes the system harder to reason about, and the real bug remains. Root-cause
fixes are slower upfront but reduce total debugging time.

Method:

- Reproduce first (if you cannot reproduce it, you cannot verify your fix)
- Ask "why" until you hit the root cause
- Resist the urge to add guards (a nil check that silences a crash is a symptom fix)
- If a workaround needs a paragraph-long comment to justify it, the code is wrong (fix the code, not the comment)
- Check for the pattern, not just the instance. Grep for the same pattern. Fix all instances
- When stuck, instrument. Do not guess (add logging or read the actual error)

Code does not change between runs, but state does. For a restart bug, suspect stale persistent state before code: config
files, caches, lock files, serialized state. If you clear a state file and behavior returns, prioritize state validation
as the fix.

In Agentics, `/agentics:diagnose` owns the full root-cause workflow. This principle is the posture that it enforces.

## Self-test

- Can I reproduce the failure on demand before the fix? If no, I cannot verify the fix.
- Does the fix change the code that produced the bad value? If no, and it only adds a guard around the symptom (a nil
  check, a catch, or a retry), it is not a fix.
- How many other places match the same pattern? Grep for it. Report the count.
- Does the failure go away when I delete a cache, lock file, or config file? If yes, the bug is stale state, not code.
