# Drive recipes per surface

These are generic recipes for the **Drive** section of a generated verification skill. Use them only when the repo has
no harness of its own (author-mode interview, step "Drive"). Each recipe names the tools available in an agent session.
The generated skill copies the matching recipe. It fills the recipe with this repo's real commands, selectors, and
prompts. Never ship the placeholders.

## Web UI (and Electron)

Prefer the host's built-in browser tools when the session has them. Claude Code has `navigate`, `read_page`, `find`,
`form_input`, and `computer` for clicks and screenshots. For evidence, it has `read_console_messages` and
`read_network_requests`. Prefer accessibility handles over pixel coordinates. Accessibility handles include ARIA roles
and accessible names from `read_page`.

If no browser tools are available, use a scripted Playwright run via the repo's own toolchain.

One-shot scripted drive, with no test framework required. The command reads the same in Bash and PowerShell:

```text
npx --yes playwright@latest screenshot --wait-for-timeout 2000 "http://127.0.0.1:<port>/<route>" artifacts/<feature>/state.png
```

Evidence: an accessibility snapshot or page text plus a screenshot, console errors, and the relevant network responses.

## CLI

A short-lived CLI needs no live session. Run the command. Capture stdout, stderr, and the exit code.

```bash
# bash (macOS/Linux)
<cli> <subcommand> --format json > artifacts/<feature>/out.json 2> artifacts/<feature>/err.txt; echo "exit=$?"
```

```powershell
# PowerShell (Windows)
<cli> <subcommand> --format json 1> artifacts/<feature>/out.json 2> artifacts/<feature>/err.txt; "exit=$LASTEXITCODE"
```

## TUI / interactive CLI

Drive an interactive terminal program in its own isolated `tmux` session, so the agent can send keystrokes and capture
the screen. **Platform note: `tmux` is Unix-only (macOS/Linux, or WSL on Windows).** On native Windows there is no
equivalent bundled PTY driver. The generated skill should say so. It should prefer the app's non-interactive flags
(`--yes`, `--format json`) where they exist.

```bash
# bash (macOS/Linux) — isolated session per drive
tmux new-session -d -s "verify-$RUN_ID" -x 200 -y 50 '<command>'
tmux send-keys -t "verify-$RUN_ID" '<input>' Enter
sleep 1
tmux capture-pane -t "verify-$RUN_ID" -p > artifacts/<feature>/screen.txt
tmux kill-session -t "verify-$RUN_ID"   # kill what you started, never by process name
```

When the app is slow to render, wait for a specific prompt string in `capture-pane` output rather than a fixed sleep.

## HTTP service / API

Use plain HTTP against the launched instance. Read `request_url` and `body_file` as structured data in the same call.
Use the loopback base URL and port from the process you launched and the selected route. Keep the body file under the
evidence directory. Capture status, headers, and body.

```bash
# bash (macOS/Linux)
curl -sS -o "$body_file" -w '%{http_code}\n' -- "$request_url"
```

```powershell
# PowerShell (Windows)
$r = Invoke-WebRequest -Uri $request_url -UseBasicParsing
$r.Content | Out-File -LiteralPath $body_file
$r.StatusCode
```

For a mutation, follow with a read-only second request, or a DB/file check. It proves the stored state, not just the
mutation response.

## Desktop / mobile

Use this recipe only with a driver the repo already has, or with a host-provided simulator tool. An example is an
existing Appium/XCUITest/simulator harness. Without either, say so in the generated skill. Scope verification to the
app's scriptable surfaces (CLI, API) instead of promises of UI drives that the agent cannot perform.
