# Forging hooks

A hook is a command the host runs on a tool call or a session event. The host runs it whatever the model decides, so a
hook can enforce a rule that an instruction can only ask for. A hook also runs code on the user's behalf and can block
the agent's work, so it carries more risk than prose does.

The host's own hook documentation is the authority for event names and payload fields, and a host changes them, so read
it before you rely on this guide. This guide holds what a host-neutral author needs: how a hook's exit code and output
behave on each supported host, and the rules that hold on all of them. It covers Claude Code
([hooks reference](https://code.claude.com/docs/en/hooks)), Copilot CLI
([hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference)), Codex
([hooks](https://learn.chatgpt.com/docs/hooks)), and VS Code
([agent plugins](https://code.visualstudio.com/docs/agent-customization/agent-plugins)).

## Hook, instruction, or check

| The rule                                                              | Use                                                                      |
| --------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| Must hold on every tool call, whatever the model decides              | A hook                                                                   |
| Is a preference, a convention, or context the agent needs             | An instruction line ([forging-instructions.md](forging-instructions.md)) |
| Is a procedure the agent follows on request                           | A skill ([forging-skills.md](forging-skills.md))                         |
| Can be checked after the change exists, with no live agent in the way | A test, a lint, or a CI check                                            |

Prefer the check when it can catch the same mistake later: it runs for every contributor and every tool, and a hook runs
only on the hosts where it is installed and trusted. A hook earns its place when the mistake is costly or cannot be
undone, such as a force push, a read of a credential file, or a write outside the workspace.

## What every hook states

Before it writes anything, an author states five facts. The asset is not ready until all five are known.

1. **The event.** The one event it runs on, and the tool names it matches. Tool names differ by host (see the table
   below), so read the name from the host's payload; do not assume it.
2. **The input.** The payload fields the script reads, taken from a recorded event, not from memory.
3. **The outcome.** What the exit code, stdout, and stderr do on each host the hook must run on.
4. **The failure posture.** What happens when the script crashes, receives malformed input, or runs past its time limit:
   the call proceeds (fail-open) or is blocked (fail-closed).
5. **The test.** The recorded events the script runs against, and the host check that shows the host honors its answer.

## The events and what a hook's answer does

The rows come from each host's published hook documentation, read in 2026-10. Recheck the host's page before you rely on
a row for a hook that must block.

| Surface     | Event and config shape                                                                  | Exit 0                                                 | Exit 2                                  | Any other exit                                                                           | Timeout                                                      |
| ----------- | --------------------------------------------------------------------------------------- | ------------------------------------------------------ | --------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| Claude Code | `PreToolUse`, with a regex matcher over tool names                                      | No decision unless stdout JSON sets one                | Blocks the call; stderr is the reason   | A non-blocking error: the call proceeds. A hook with `onFailure: "block"` blocks instead | `timeout` in seconds; at the limit the call continues        |
| Copilot CLI | `preToolUse`, lower-camel names; `version: 1`, `bash` and `powershell` command fields   | stdout JSON is the answer; empty stdout is the default | Denies, even when the JSON says `allow` | Denies, reported as a hook error                                                         | `timeoutSec`, default 30; the host skips the hook, fail-open |
| Codex       | `PreToolUse`; the user trusts the hook through `/hooks`, and a change needs fresh trust | Success; stdout JSON, if any, is read                  | Blocks; stderr is the reason            | A reported failure: the call proceeds                                                    | `timeout` in seconds, default 600; the call proceeds         |

VS Code runs plugin hooks, and their configuration and payloads depend on the harness of the session, so it has no row.
Author for that harness and use its hook schema; do not assume it reads the Claude Code or Copilot CLI configuration.

Two consequences follow from the table:

- **Exit 2 with the reason on stderr is the one blocking answer all three documented hosts share.** Stdout JSON differs:
  Claude Code and Codex read `hookSpecificOutput.permissionDecision`, Copilot reads a flat `permissionDecision`, and
  Copilot requires a reason on `deny`. A hook that must work on more than one host uses exit 2, or writes one adapter
  per host.
- **Tool names differ.** Copilot names its shell tool `bash` or `powershell`, Codex names it `exec_command` and its edit
  tool `apply_patch`, and Claude Code names them `Bash` and `Edit`. A matcher or a script that tests one spelling misses
  the same call on another host.

## Blocking or advisory

- **Blocking.** The hook denies the call. Use it only for a mistake that is costly or cannot be undone, and give the
  agent the reason and the next step: "force push blocked; push a new commit, or ask the user to run it".
- **Advisory.** The hook lets the call proceed and adds context. Claude Code reads `additionalContext` on `PreToolUse`.
  Copilot's `preToolUse` has no documented advisory channel, so an advisory hook there says nothing the agent sees. When
  a rule must reach the agent on every host, make it blocking or put it in an instruction line as well.

A blocking hook that is wrong stops legitimate work. Keep the deny set narrow, name the safe alternative in the reason,
and keep an escape the user controls, such as an environment variable the user sets for one command.

## Fail-open or fail-closed

The hosts do not agree, so a hook chooses its own posture and does not rely on the host's default.

- Claude Code and Codex proceed when a script crashes or exits with a code other than 0 or 2. Copilot CLI denies on any
  code other than 0, so one crash blocks every call on that host.
- All three proceed when a hook runs past its timeout (Claude Code blocks only for a hook set to `onFailure: "block"`).
  A hook cannot enforce anything inside a call that outlives the limit, so it must finish well before it.
- To fail closed on a script fault, catch the fault in the script and exit 2 with the reason on stderr. Decide this per
  rule: a rule that guards something irreversible fails closed, and a convenience check fails open.
- Keep the work bounded. Read stdin once, run no network call, start no process that can hang, and set an internal
  deadline below the host's timeout so the script answers before the host gives up.

## Untrusted input in the payload

The payload carries text the model or a file wrote: a command line, a file path, file content. Treat every field as
untrusted data.

- Parse stdin with a JSON parser. Do not `grep` the raw payload, and never interpolate a field into a shell string,
  `eval` it, or pass it to a shell. Pass it as an argument in array form.
- Match on the parsed command, not a substring. A force-push rule has to see `--force`, `-f`, a combined short flag,
  `--force-with-lease`, a refspec that starts with `+`, and a command that arrives through a wrapper, an alias, or a
  quoted segment. Decide which of these the rule blocks and test each.
- Normalize a path before you compare it: resolve `..` segments and symbolic links, and compare against the resolved
  root. A path check and the later write are two steps, so a hook that approves a path says that a link can change
  between them ([mcp-security-checklist.md](../mcp/mcp-security-checklist.md#safe-writes-inside-a-root)).
- Quote with care on every host the hook runs on. A command that differs between Bash and PowerShell has a block for
  each, and a command that is the same is written once.
- Log the decision and the rule that fired. Do not log the payload: it can hold a secret.

## Test with recorded events

A hook is tested with an event, not by reading the script.

1. **Record the event.** Capture a real payload from the host, redact secrets, and keep it as a fixture. A payload
   written from memory tests your memory.
2. **Run the script against it.** Pipe the fixture to the script's stdin and assert the exit code, stdout, and stderr.
3. **Cover four events.** A call the hook must block, a call it must allow, a malformed payload, and a script fault,
   such as a missing dependency. The last two show the failure posture.
4. **Make the test fail without the check.** Remove the check and run the test. A test that passes without the check
   proves nothing. Assert the distinctive reason text, not only a nonzero exit.
5. **Check the host once.** A golden test cannot show that a live host honors a decision. Run the surface document's
   Live check, or in a session on the host trigger the blocked call and record what happened and the host version.

A hook runs on every matching call, so measure its latency on the host and keep it well under the timeout.

## Registering the hook

The registration is configuration the host reads, so showing it is part of showing the asset. A script and its
registration are written together, only after the user says yes to the exact text. Say where the registration lives (the
project's host configuration, or a plugin's hook file), which hosts read it, and what the user does to trust it: Codex
needs the user to review the hook through `/hooks` before it runs. Say what removes it.

When a repository ships hooks of its own, follow its hook guide and its registration pattern in preference to this one.
