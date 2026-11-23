# Sequence Work Into Verifiable Units

_Apply to multi-step work (sweeps, migrations, runs of similar edits) and to how you stack commits and PRs._

Order work as small units that each end in a state you can check. Do not advance until the current unit is green. The
one exception is a planned migration under [Outcome-Oriented Execution](outcome-oriented-execution.md): a unit may stay
red only when the plan lists it, with the phase boundary at which it must pass again. The same discipline applies at two
levels: how you execute and how you deliver.

If you catch a break at the unit that caused it, the break is cheap to localize. When you sequence those units into a
delivery that a reviewer can replay, "trust me" becomes "watch it go red, then green."

**Execution.** In a sweep, migration, or any run of similar edits, verify each change before you start the next. Do not
batch edits and verify once at the end. Each unit is a before/after bracket: a known-good state, one change, a run of
the check, then the next unit. Start from a clean working tree on the current base so every check measures against the
real baseline. If the base has moved, bring it in with a merge, which is an ordinary commit. A rebase rewrites history,
so it needs the confirmation that `/agentics:git` asks for. When a lever does the edits, the per-unit check is nearly
free. Run it anyway.

**Delivery.** Stack commits and PRs in the order that proves the work. The canonical shape is the failing test first,
then the fix on top. The first unit shows that the bug is real (red), and the next shows that it is resolved (green). A
reviewer sees both the problem and the proof. The red commit fails only the new test. If a pre-commit hook rejects it,
do not bypass the hook. Land the test and the fix in one commit, and record the failing run (its command, exit status,
and an excerpt of its output) as the evidence of red. Other story orders are a subtraction before the reshape, a
baseline capture before the treatment, or the scaffold before the feature. Each commit builds, and every test passes
except the one new test on the red commit. The sequence reads as an argument.

Pattern:

- Pick the smallest unit that ends in a check: an edit plus its test, or a commit that stands alone.

The sequencing complement to [Prove It Works](prove-it-works.md), which keeps each check real, and
[Build the Lever](build-the-lever.md), which makes the per-unit check cheap.

## Self-test

- Did the check that ends the previous unit run before I started this one? Name the command and its exit status.
- Does each commit build? Check out each one and run the check. The count of failing tests is zero, or one on the red
  commit.
- Does the stack show the bug red before it shows it green? If a hook rejected the red commit, is the failing run
  recorded with its command, exit status, and output excerpt? A fix and its test in one commit with no recorded run hide
  the failure from the reviewer.
- Does `git status --porcelain` print nothing, and does `git merge-base HEAD "$base"` equal the fetched tip of the base?
  Put the base ref in a quoted variable. Never paste a ref name into the command line, because a ref name can contain
  shell syntax. If no, the check measures against the wrong baseline.
