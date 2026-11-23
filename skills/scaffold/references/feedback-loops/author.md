# `/agentics:scaffold feedback-loops author`: generate a verification skill

Projects need a scripted way to drive the real app and prove behavior. That means three actions: launch the app,
exercise a feature the way a user would, and capture evidence. Author mode generates that as a project-local skill,
`verify-<app>`, for the repository. Write the output for the next agent, not for a human. An agent that has never seen
the app will read it cold, often in the middle of a task.

The skill goes in the repository's project skill directory: the one of `.claude/skills/`, `.github/skills/` and
`.agents/skills/` that already holds the repository's project skills, or `.claude/skills/` when none does. When more
than one holds skills, ask once which to use.

## 1. Interview the repo, not the user

**Cover every surface the user names, not just the primary one.**

A repo with both a web app and a CLI needs a Drive recipe for each surface. It also needs at least one feature file per
surface. A skill that covers only the primary surface leaves the others undriveable. The gap surfaces only when someone
later tries to verify a feature there. At the end of the interview, before you generate, name the surfaces back to the
user.

Answer the topics below from the codebase. For anything you cannot observe, ask it as one ❓ round. Give each question a
➡️ recommendation inferred from the repo. Skip any topic the codebase already settles.

- **Surface:** what a user actually touches: a web UI, a CLI/TUI, a desktop app, an API, a mobile app, a library. A repo
  can have several. Pick the primary one. Note the rest.
- **Run:** how the app starts locally. Prefer the repo's own documented dev command (package scripts, Makefile, README
  quickstart). Note ports, env vars, seed data, auth.
- **Drive:** how an agent can interact with it programmatically. Check existing harnesses first. These include
  Playwright/Cypress specs, expect scripts, PTY helpers, curl-able endpoints, and a debug port. Only then pick a generic
  recipe from [drive-recipes.md](drive-recipes.md): browser tools for web, tmux/PTY for CLI/TUI, or plain HTTP for
  services.
- **Observe:** what evidence can be captured: screenshots, terminal transcripts, response bodies, logs, exit codes, DB
  state.
- **Isolate:** whether two instances can run side by side (ports, data dirs, profiles). If not, say so in the generated
  skill. It is better to refuse to double-drive a shared instance than to corrupt the user's session.

If the checkout does not build or start as-is, report exactly what fails and stop before you generate: a skill written
against a broken base teaches wrong steps. Author mode edits no product code; the fix goes to `diagnose`, or to a direct
change outside this mode, and author runs again afterwards. The one exception is an irrelevant missing asset that blocks
startup, such as a static directory the API never serves or a sample config: the generated skill may create it, mark it
as verification scaffolding, and remove it in cleanup.

## 2. Generate the skill

Generate `verify-<app>/SKILL.md` with YAML frontmatter. The frontmatter has `name: verify-<app>` and a `description`.
The description names the app, the surface, and when to reach for it. Without frontmatter, the skill never registers.
Include these six sections. Ground each in what the interview found. Leave no placeholders.

- **Launch:** the exact command that starts the app for verification, and how to tell it's ready (a log line, a port
  that answers, a prompt). Include teardown. For a short-lived CLI or TUI there is no server to keep alive. Launch
  means: build the binary (or install deps) once. Then start each drive in its own isolated PTY or tmux session.
- **Doctor:** one read-only check that answers "is this instance worth driving?" It checks that the process is up, the
  version/build is correct, the port belongs to us, and auth is valid. An agent runs this first whenever anything looks
  off.
- **Drive:** the harness recipe with real selectors/commands from this repo, not examples. Prefer stable handles (ARIA
  labels, data attributes, prompt strings, route paths) over coordinates and tab order.
- **Evidence:** what to capture and where it goes. Proof standards:
    - Exercise the real user path, not internal setters or test-only endpoints.
    - Capture the action and the resulting state, not just the final screen.
    - Verify side effects (files written, rows inserted, messages sent) alongside what's visible.
    - Use mocks only where a production boundary already isolates the external system.

    When the safe path is a dry-run or test mode, verify what it skips. Observe files, network, and git refs. Do not
    trust the mode's name. Some dry-runs still touch the network or open a browser.

- **Cleanup:** how to tear down instances the run created. Never kill by process name. Kill what you started. Cleanup
  removes instances and scratch state, never the evidence. The evidence survives teardown at the location the skill
  names.
- **Helpers:** any script the skill ships is executable. The skill body shows its invocation. A helper the reader has to
  reverse-engineer is not a helper.

## 3. Seed the feature map

Generate `verify-<app>/features/README.md` and one file per user-facing feature you can identify. Aim for the top 3–5 to
start, from routes, commands, menus, or docs. Follow the shape in [feature-map-example/](feature-map-example/README.md):
a README index and one file per feature. Each file answers these questions from the user's point of view:

- What the feature is
- How to reach it
- How to drive it with the harness
- What observable end state proves it works

The four H2s are `Sub-features`, `How to get to it (user POV)`, `Driving it with <harness>`, and `Gotchas`. The map is
the repo's maintained verification source. A proof that drives one convenient entry point is incomplete when the map
lists others.

## 4. Show it, then write it

The generated skill tells every later agent what to run, so it is written only after the user has seen its exact text
([approval.md](../../../routing/references/contracts/approval.md#the-two-exceptions)). Show the full text of `SKILL.md`,
the feature map, and every helper script in one message, and ask once. The body of the shown `SKILL.md` starts with the
line `> Draft — generated, never executed`; say that a passing proof (step 5) removes that line and changes nothing
else, so the text the user approves covers both outcomes:

> ❓ **Q1** - **Write the verification skill**: Write `verify-<app>` as shown? (yes / edit <file> / skip)
>
> ➡️ yes, unless the text needs a change.

On **edit**, change the file and show it again with the same question. On **skip**, write nothing. On **yes**, write the
files and make the helper scripts executable.

## 5. Prove the generated skill before you hand it over

Run its own instructions end to end once, in this order:

1. Launch the app.
2. Run doctor.
3. Drive ONE mapped feature. One is enough, because the map exists so later runs can cover the rest.
4. Capture evidence.
5. Clean up.

After cleanup, confirm the evidence still exists at the named location. A cleanup that eats the proof fails this step.
Run the generated cleanup after every failed iteration too, so broken attempts don't strand processes and ports. When
the proof passes, remove the draft line and nothing else. A fix that the proof needs changes the skill's text, so show
the changed lines and ask once more before you hand it over; a declined fix leaves the draft line in place. **A
generated skill that was never executed is a draft, not a deliverable.**

Skipping the proof is a process gate ([gates.md](../../../routing/references/contracts/gates.md#process-gate)). When the
user declines it, name the cost once: an unproven skill teaches the next agent unverified steps. On the next reply that
still declines, skip it, leave the draft line from step 4 in place, and say so in the summary. Nothing new is written.
Never hand over an unexecuted skill as proven.

## 6. Offer the maintenance loop

Point the user at `/agentics:scaffold feedback-loops audit` to keep the map honest as the app changes. Suggest a cadence
only if they ask.
