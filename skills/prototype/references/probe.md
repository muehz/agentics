# Probe prototype

A probe is one throwaway script that runs once and prints the observation that answers the question without a person
steering it. For a probe requested by another workflow, the
[empirical questions contract](../../routing/references/contracts/empirical-questions.md#when-to-run) determines when it
may start.

If a person needs to push a state model through cases by hand, use [logic.md](logic.md). If the question is how
something looks, use [ui.md](ui.md).

## When this is the right shape

- "How long do 10,000 inserts take inside one transaction?"
- "Does this library parse our date format at all?"
- "What shape does the API return for an empty result?"
- Any question whose answer is a number, an output, or a yes or no that one run can print.

## Process

### 1. State the question and the observation

Write the question, the observation that answers it, and any threshold in one paragraph at the top of the script. Take
the threshold from the request or the caller. When none was given, do not invent one: report the measurement and leave
the verdict to whoever asked.

### 2. Pick the language

Follow [logic.md](logic.md#2-pick-the-language): the host project's runtime, and the same question when there is none.

### 3. Write one script

- Fix every input at the top of the script and print it with the result.
- Print the facts the result depends on, such as the runtime version and whether it ran on this machine or in a
  container.
- For a timing, run a stated number of repetitions, five by default, print each one and the median, and stop after a
  stated time limit, two minutes by default.
- Keep [the rules for every mode](../SKILL.md#rules-for-every-mode): the script calls no production service and uses no
  real credential.

### 4. Run it

Run the script yourself with its one command. Its output is the evidence. A script that failed to run answers nothing:
report the failure, not a verdict.

Run it only through a runner that is already installed, as [rule 2](../SKILL.md#rules-for-every-mode) says. When the
runner is missing, ask before installing it. In an unattended run, install nothing: report the script, its command and
the missing runner, and leave the verdict open.

Importing code runs it with this session's privileges. Run the probe and any repository test as
[Running it](../../routing/references/repo-conventions.md#running-it) allows. Run the probe only against code the
session would already run, such as code the project's own tests exercise. When it would import code from an unreviewed
source, such as a fork's pull request or a dependency nobody has vetted, do not run it, even in an unattended run:
report the script and its command, and leave the verdict open.

### 5. State the verdict

Say what the output supports and what it does not, such as one machine, one data size or one run. Then record it and
close as [When done](../SKILL.md#when-done) says: remove the script only after the confirmation that section asks for,
and in an unattended run leave it and report its path.

## Anti-patterns

- **A benchmark suite.** One question, one script. A suite is code meant to survive, which is ordinary implementation.
- **A verdict from one noisy run.** Repeat timings, and state the spread.
- **A threshold you chose.** A pass or fail line that nobody asked for turns a measurement into a decision the user did
  not make.
