package skillcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// isRegularFile reports whether path names an existing regular file.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// repositoryAncestor returns the nearest of dir and its ancestors that holds a .git entry, file or directory, and
// whether there is one. The hook reads the settings and writes the log of the repository it finds itself in, so a
// working directory under a checkout would run under that checkout's settings.
func repositoryAncestor(dir string) (string, bool) {
	dir = filepath.Clean(dir)
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// checkOutsideRepository returns an error naming the ancestor when dir is inside a repository.
func checkOutsideRepository(dir string) error {
	ancestor, found := repositoryAncestor(dir)
	if !found {
		return nil
	}
	return fmt.Errorf("working directory %s is inside the repository at %s; run go test with the temp directory "+
		"(TMPDIR, or TEMP and TMP on Windows) outside any checkout", dir, ancestor)
}

// childEnvironment returns the whole environment of a hook process, built from nothing so that no guard switch, log
// directory, plugin path or release record of the parent reaches the hook. From parent, a slice of NAME=value, it
// copies only PATH, and on Windows the variables a process needs to start others (SystemRoot, ComSpec, PATHEXT),
// each only when set; Windows names match in any case and are written in the case shown here. The home variables
// are set to home and the temp variables to temp.
func childEnvironment(parent []string, home, temp, goos string) []string {
	copied := []string{"PATH"}
	if goos == "windows" {
		copied = append(copied, "SystemRoot", "ComSpec", "PATHEXT")
	}
	var env []string
	for _, want := range copied {
		for _, entry := range slices.Backward(parent) {
			name, value, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			if name == want || (goos == "windows" && strings.EqualFold(name, want)) {
				env = append(env, want+"="+value)
				break
			}
		}
	}
	return append(env,
		"HOME="+home, "USERPROFILE="+home,
		"TMPDIR="+temp, "TEMP="+temp, "TMP="+temp,
	)
}

// hookEvent returns the Claude Code PreToolUse event that asks to run b in dir: the body is the command of a shell
// tool, or the content of a write to block.txt in dir.
func hookEvent(b block, dir string) ([]byte, error) {
	var input map[string]string
	switch b.tool {
	case toolBash, toolPowerShell:
		input = map[string]string{"command": b.body}
	case toolWrite:
		input = map[string]string{"file_path": filepath.Join(dir, "block.txt"), "content": b.body}
	default:
		return nil, fmt.Errorf("%s:%d: no event for tool %q", b.path, b.line, b.tool)
	}
	return json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "skillcompat",
		"cwd":             dir,
		"tool_name":       b.tool,
		"tool_input":      input,
	})
}

// verdict is what the hook decided about one block.
type verdict int

const (
	// verdictAllowed: the hook let the tool call through.
	verdictAllowed verdict = iota
	// verdictDenied: a rule refused the tool call.
	verdictDenied
	// verdictFault: the hook failed or could not finish a check, so its answer says nothing about the block.
	verdictFault
)

func (v verdict) String() string {
	switch v {
	case verdictAllowed:
		return "allowed"
	case verdictDenied:
		return "denied"
	}
	return "fault"
}

// Categories of a denial that names no command-rule category.
const (
	// categoryCommands is a denial by the commands check without a blocking command-rule line, such as a release
	// rule's.
	categoryCommands = "commands"
	// categorySecrets is a denial by the secrets check.
	categorySecrets = "secrets-guardian"
)

// runResult is the classified answer of the hook to one block.
type runResult struct {
	verdict verdict
	// category is set only for verdictDenied: the command-rule category of the blocking line (such as
	// destructive_git_ops), else categoryCommands, else categorySecrets.
	category string
	// message is, for verdictDenied, the first line of the denying message with its label; for verdictFault, the
	// reason the run counts as a fault; empty for verdictAllowed.
	message string
	// exitCode is the hook's exit status.
	exitCode int
	// output is the text that was classified: standard error, or standard output when standard error is empty.
	output string
}

// The labels the hook puts before the message of each check of an event, one per check.
const (
	labelCommands = "[commands]"
	labelSecrets  = "[secrets-guardian]"
)

var (
	// blockingLine is the command rules' denial; its first group is the category of the most severe threat.
	blockingLine = regexp.MustCompile(`^\d+ blocking threat\(s\) detected: (.+?) - `)
	// advisoryLine is the command rules' advice, which blocks nothing.
	advisoryLine = regexp.MustCompile(`^\d+ threat\(s\) detected: `)
	// endpointLine is the secrets check's note about network endpoints, which blocks nothing.
	endpointLine = regexp.MustCompile(`^\d+ informational network endpoint\(s\) found `)
)

// checkFaults are the notices the hook writes after a label when a check itself failed.
var checkFaults = []string{"panic", "invalid outcome", "check failed", "missing handler", "worker infrastructure panic"}

// deadlineFaults are the notices for a check that ran out of time; the hook appends one to an earlier notice with
// "; " when both apply.
var deadlineFaults = []string{"deadline exceeded; incomplete check", "deadline exceeded; cleanup unacknowledged"}

// ruleSetFailures are the paragraphs the commands check writes in place of a rule set's own message when that rule
// set failed.
var ruleSetFailures = []string{"command rules panicked", "release rules panicked", "rule set returned an invalid outcome"}

// isFailureNotice reports whether text, the whole text after a label, is one of the hook's failure notices, alone
// or compound. A rule's message that merely contains such words is not a notice.
func isFailureNotice(text string) bool {
	for _, deadline := range deadlineFaults {
		if text == deadline {
			return true
		}
		if rest, ok := strings.CutSuffix(text, "; "+deadline); ok {
			text = rest
			break
		}
	}
	return slices.Contains(checkFaults, text)
}

// hookMessage is the output of one check: the labelled line and every line up to the next labelled line.
type hookMessage struct {
	label string
	// lines holds the message's lines; the first has the label removed.
	lines []string
	// first is the labelled line as the hook wrote it.
	first string
}

// paragraphs returns the message's lines grouped at blank lines, each group joined with LF.
func (m hookMessage) paragraphs() []string {
	var paragraphs []string
	var current []string
	flush := func() {
		if len(current) > 0 {
			paragraphs = append(paragraphs, strings.Join(current, "\n"))
			current = nil
		}
	}
	for _, line := range m.lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return paragraphs
}

// cutLabel returns the label line starts with and the text after it.
func cutLabel(line string) (label, text string, ok bool) {
	for _, label := range []string{labelCommands, labelSecrets} {
		if rest, found := strings.CutPrefix(line, label); found {
			return label, strings.TrimPrefix(rest, " "), true
		}
	}
	return "", "", false
}

// isSettingsNote reports whether paragraph is the note the command rules append about the setting that governs a
// detection, which is not a verdict of its own.
func isSettingsNote(paragraph string) bool {
	return strings.HasPrefix(paragraph, "Setting: ") || strings.HasPrefix(paragraph, "Settings: ") ||
		strings.HasPrefix(paragraph, "Agentics hook settings: ignored invalid ")
}

// denies reports whether the message refuses the tool call. A commands message is advice when it opens with the
// command rules' advisory line and holds nothing after it but settings notes; any other paragraph is a rule's
// refusal, since the hook joins the release rules' message to the command rules' under the one label. A secrets
// message refuses unless it is the note about network endpoints.
func (m hookMessage) denies() bool {
	if m.label == labelSecrets {
		return !endpointLine.MatchString(m.lines[0])
	}
	paragraphs := m.paragraphs()
	if len(paragraphs) == 0 {
		return false
	}
	if !advisoryLine.MatchString(paragraphs[0]) {
		return true
	}
	for _, paragraph := range paragraphs[1:] {
		if !isSettingsNote(paragraph) {
			return true
		}
	}
	return false
}

// classifyRun turns the hook's exit status and output into a result. Exit 0 is allowed, exit 2 is the hook's deny
// status, and any other status is a fault. An exit 2 is a denial only when a check's message refuses the call; a
// failure notice anywhere in the output makes the run a fault even beside a refusal, because a check that did not
// finish gives no verdict to rely on.
func classifyRun(exitCode int, stdout, stderr string) runResult {
	output := stderr
	if strings.TrimSpace(output) == "" {
		output = stdout
	}
	result := runResult{exitCode: exitCode, output: output}
	fault := func(format string, args ...any) runResult {
		result.verdict = verdictFault
		result.message = fmt.Sprintf(format, args...)
		return result
	}
	switch exitCode {
	case 0:
		result.verdict = verdictAllowed
		return result
	case 2:
	default:
		return fault("exit status %d: %s", exitCode, firstLine(output))
	}

	var messages []hookMessage
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		label, text, labelled := cutLabel(line)
		switch {
		case labelled && isFailureNotice(text):
			return fault("the hook reported a failed check: %s", line)
		case labelled:
			messages = append(messages, hookMessage{label: label, lines: []string{text}, first: line})
		case strings.HasPrefix(line, "agx:"):
			return fault("the hook reported its own failure: %s", line)
		case len(messages) > 0:
			last := &messages[len(messages)-1]
			last.lines = append(last.lines, line)
		}
	}
	for _, message := range messages {
		for _, paragraph := range message.paragraphs() {
			if slices.Contains(ruleSetFailures, paragraph) {
				return fault("the hook reported a failed rule set: %s", paragraph)
			}
		}
	}

	denied := func(category string, message hookMessage) runResult {
		result.verdict, result.category, result.message = verdictDenied, category, message.first
		return result
	}
	for _, message := range messages {
		if message.label != labelCommands {
			continue
		}
		for _, line := range message.lines {
			if m := blockingLine.FindStringSubmatch(line); m != nil {
				return denied(m[1], message)
			}
		}
	}
	for _, label := range []string{labelCommands, labelSecrets} {
		category := categoryCommands
		if label == labelSecrets {
			category = categorySecrets
		}
		for _, message := range messages {
			if message.label == label && message.denies() {
				return denied(category, message)
			}
		}
	}
	return fault("exit status 2 without a denial: %s", firstLine(output))
}

// firstLine returns the first non-blank line of text, or a note that there is none.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "(no output)"
}

// Whether the working directory handed to runBlock is meant to be a repository.
const (
	// outsideRepository makes runBlock refuse a working directory under any checkout, so that no repository's
	// settings decide the verdict.
	outsideRepository = false
	// insideRepository says the caller built the working directory as a repository on purpose.
	insideRepository = true
)

// runTimeout bounds one hook process. The hook bounds its own checks far below this; the limit only keeps a hung
// process from holding a sweep until the test binary's own deadline.
const runTimeout = 2 * time.Minute

// pipeDelay is how long a run waits for the hook's output to close once the hook has exited or been killed.
const pipeDelay = time.Second

// sharedDirs holds the home and temp directories every hook process is given. They are shared because a sweep
// starts hundreds of processes and the hook only reads the home directory; the home directory stays empty, so no
// user settings file exists for any run.
var sharedDirs struct {
	once       sync.Once
	root       string
	home, temp string
	err        error
	hookOnce   sync.Once
	hookPath   string
	hookErr    error
}

// runnerDirs returns the shared, empty home directory and the shared temp directory of the hook processes.
func runnerDirs(t testing.TB) (home, temp string) {
	t.Helper()
	sharedDirs.once.Do(func() {
		sharedDirs.root, sharedDirs.err = os.MkdirTemp("", "skillcompat-runner-")
		if sharedDirs.err != nil {
			return
		}
		sharedDirs.home = filepath.Join(sharedDirs.root, "home")
		sharedDirs.temp = filepath.Join(sharedDirs.root, "tmp")
		sharedDirs.err = errors.Join(os.Mkdir(sharedDirs.home, 0o700), os.Mkdir(sharedDirs.temp, 0o700))
	})
	if sharedDirs.err != nil {
		t.Fatalf("create the hook's home and temp directories: %v", sharedDirs.err)
	}
	return sharedDirs.home, sharedDirs.temp
}

// runnerBinary builds the native hook once in the test process's scratch directory.
func runnerBinary(t testing.TB) string {
	t.Helper()
	name := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
	if name == "" {
		t.Skipf("no contract hook binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	runnerDirs(t)
	sharedDirs.hookOnce.Do(func() {
		repo, err := hosttest.FindCheckout()
		if err != nil {
			sharedDirs.hookErr = err
			return
		}
		result, err := hookbuild.Rebuild(hookbuild.Request{
			SourceDir: filepath.Join(repo, "hooks", "go"),
			GoBin:     "go",
			OutDir:    filepath.Join(sharedDirs.root, "bin"),
			Names:     []string{name},
		})
		if err != nil {
			sharedDirs.hookErr = err
			return
		}
		sharedDirs.hookPath, sharedDirs.hookErr = result.Targets[0].Path, result.Targets[0].Err
	})
	if sharedDirs.hookErr != nil {
		t.Fatalf("build the hook from the checkout: %v", sharedDirs.hookErr)
	}
	return sharedDirs.hookPath
}

// TestMain removes the test process's scratch directory after the last test.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedDirs.root != "" {
		os.RemoveAll(sharedDirs.root)
	}
	os.Exit(code)
}

// runBlock sends b to the built hook binary as one Claude Code PreToolUse event and returns the classified
// result. dir is both the hook process's working directory and the event's cwd; the hook writes its log under it.
// With outsideRepository the test fails when dir is under a checkout; pass insideRepository only for a directory
// built as a repository on purpose. The hook's environment is childEnvironment's, so nothing set in the test
// process changes the verdict. The body is sent as given: callers pass the block through sentBlock first. runBlock
// fails the test when the binary cannot be started, skips on a platform outside the build contract, and is safe
// to call from parallel tests.
func runBlock(t testing.TB, b block, dir string, inRepository bool) runResult {
	t.Helper()
	binary := runnerBinary(t)
	if !inRepository {
		if err := checkOutsideRepository(dir); err != nil {
			t.Fatal(err)
		}
	}
	event, err := hookEvent(b, dir)
	if err != nil {
		t.Fatal(err)
	}
	home, temp := runnerDirs(t)

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "claude-code", "pre-tool")
	cmd.Dir = dir
	cmd.Env = childEnvironment(os.Environ(), home, temp, runtime.GOOS)
	cmd.Stdin = bytes.NewReader(event)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Run waits for the output pipes to close, and a process the hook started can hold them open after the hook
	// itself has exited or been killed. The delay ends that wait, so the time bound holds.
	cmd.WaitDelay = pipeDelay

	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return runResult{
			verdict: verdictFault, exitCode: -1, output: stderr.String(),
			message: fmt.Sprintf("the hook did not exit within %s", runTimeout),
		}
	case errors.Is(err, exec.ErrWaitDelay):
		// The hook exited with status 0, but the output read so far may be cut short, so it proves nothing.
		return runResult{
			verdict: verdictFault, exitCode: 0, output: stderr.String(),
			message: fmt.Sprintf("the hook exited but its output stayed open for %s", pipeDelay),
		}
	case err == nil:
		return classifyRun(0, stdout.String(), stderr.String())
	case errors.As(err, &exit):
		return classifyRun(exit.ExitCode(), stdout.String(), stderr.String())
	}
	t.Fatalf("%s:%d: start %s: %v", b.path, b.line, binary, err)
	return runResult{}
}

func TestRunnerRefusesAWorkingDirectoryInsideARepository(t *testing.T) {
	root := t.TempDir()
	if err := checkOutsideRepository(root); err != nil {
		t.Skipf("the temp directory is itself inside a repository: %v", err)
	}

	for _, kind := range []string{"directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			checkout := filepath.Join(root, kind)
			work := filepath.Join(checkout, "a", "b")
			if err := os.MkdirAll(work, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := checkOutsideRepository(work); err != nil {
				t.Fatalf("before .git exists: %v", err)
			}
			git := filepath.Join(checkout, ".git")
			var err error
			if kind == "directory" {
				err = os.Mkdir(git, 0o700)
			} else {
				err = os.WriteFile(git, []byte("gitdir: elsewhere\n"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}

			for _, dir := range []string{work, checkout} {
				err := checkOutsideRepository(dir)
				if err == nil {
					t.Fatalf("%s: no error for a directory under %s", dir, git)
				}
				for _, want := range []string{checkout, "go test", "outside any checkout"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				if strings.Contains(err.Error(), "repository at "+work) {
					t.Errorf("error %q names the working directory, not the ancestor", err)
				}
			}
		})
	}
}

func TestRunnerEnvironmentIsBuiltFromNothing(t *testing.T) {
	polluted := []string{
		"SKIP_COMMAND_GUARD=true", "COMMAND_GUARD_MODE=warn", "COMMAND_GUARD_ALLOWLIST=git reset",
		"SKIP_SECRETS_GUARD=true", "SECRETS_GUARD_MODE=warn", "SECRETS_GUARD_ALLOWLIST=.env",
		"CLAUDE_PLUGIN_DATA=/plugin/data", "CLAUDE_PLUGIN_ROOT=/plugin/root", "COMMAND_GUARD_LOG_DIR=/logs",
		"AGX_SOURCE_DIGEST=abc", "AGX_RELEASE_RECORD=/record.json", "GIT_PAGER=less", "CDPATH=/somewhere",
		"HOME=/parent/home", "USERPROFILE=/parent/profile", "TMPDIR=/parent/tmpdir", "TEMP=/parent/temp",
		"TMP=/parent/tmp", "malformed",
	}

	t.Run("windows", func(t *testing.T) {
		parent := append([]string{
			`Path=C:\Windows\system32;C:\Git\cmd`, `SYSTEMROOT=C:\Windows`,
			`ComSpec=C:\Windows\system32\cmd.exe`, "PATHEXT=.COM;.EXE",
		}, polluted...)
		want := []string{
			`PATH=C:\Windows\system32;C:\Git\cmd`, `SystemRoot=C:\Windows`,
			`ComSpec=C:\Windows\system32\cmd.exe`, "PATHEXT=.COM;.EXE",
			`HOME=C:\run\home`, `USERPROFILE=C:\run\home`,
			`TMPDIR=C:\run\tmp`, `TEMP=C:\run\tmp`, `TMP=C:\run\tmp`,
		}
		if got := childEnvironment(parent, `C:\run\home`, `C:\run\tmp`, "windows"); !slices.Equal(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("linux", func(t *testing.T) {
		// The Windows-only names and a differently cased Path are ordinary variables here and are dropped.
		parent := append([]string{
			"PATH=/usr/bin:/bin", "Path=/not/the/path", "SystemRoot=/x", "ComSpec=/y", "PATHEXT=.z",
		}, polluted...)
		want := []string{
			"PATH=/usr/bin:/bin",
			"HOME=/run/home", "USERPROFILE=/run/home",
			"TMPDIR=/run/tmp", "TEMP=/run/tmp", "TMP=/run/tmp",
		}
		if got := childEnvironment(parent, "/run/home", "/run/tmp", "linux"); !slices.Equal(got, want) {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("unset in the parent", func(t *testing.T) {
		want := []string{"HOME=h", "USERPROFILE=h", "TMPDIR=t", "TEMP=t", "TMP=t"}
		for _, goos := range []string{"windows", "linux"} {
			if got := childEnvironment(polluted, "h", "t", goos); !slices.Equal(got, want) {
				t.Errorf("%s: got %q, want %q", goos, got, want)
			}
		}
	})
}

func TestRunnerEvent(t *testing.T) {
	dir := t.TempDir()
	decode := func(t *testing.T, b block) map[string]any {
		t.Helper()
		data, err := hookEvent(b, dir)
		if err != nil {
			t.Fatal(err)
		}
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"hook_event_name": "PreToolUse", "cwd": dir, "tool_name": b.tool} {
			if event[name] != want {
				t.Errorf("%s = %v, want %q", name, event[name], want)
			}
		}
		if id, _ := event["session_id"].(string); id == "" {
			t.Errorf("session_id = %v, want a non-empty string", event["session_id"])
		}
		if len(event) != 5 {
			t.Errorf("event has %d fields, want 5: %v", len(event), event)
		}
		return event
	}

	const command = "printf '%s\\n' \"a\tb\" <in >out\n"
	for _, tool := range []string{toolBash, toolPowerShell} {
		event := decode(t, block{tool: tool, body: command})
		input, _ := event["tool_input"].(map[string]any)
		if len(input) != 1 || input["command"] != command {
			t.Errorf("%s tool_input = %v, want only the command %q", tool, input, command)
		}
	}

	event := decode(t, block{tool: toolWrite, body: "{ \"name\": \"fixture\" }\n"})
	input, _ := event["tool_input"].(map[string]any)
	path, _ := input["file_path"].(string)
	if len(input) != 2 || input["content"] != "{ \"name\": \"fixture\" }\n" || path != filepath.Join(dir, "block.txt") {
		t.Errorf("Write tool_input = %v, want the content and block.txt in %s", input, dir)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("file_path %q is not absolute", path)
	}

	if _, err := hookEvent(block{path: "x.md", line: 3, tool: "Read"}, dir); err == nil {
		t.Error("no error for a tool the runner has no event for")
	}
}

func TestRunnerCategoryParsing(t *testing.T) {
	const (
		advisory = "[commands] 1 threat(s) detected: command_reformulation - Run the command from the repository root."
		note     = "Setting: policy.command_reformulation. To allow this, ask the user to set it to false in " +
			".agentics/hooks.json in the repository root or ~/.agentics/hooks.json; an agent must not edit those files."
		release  = "kubectl scale is a production-shaped deploy command (kubernetes_deploy); run /agentics:gate change ($agentics:gate change on Codex)"
		blocking = "[commands] 1 blocking threat(s) detected: destructive_git_ops - Use git stash instead."
		endpoint = "[secrets-guardian] 2 informational network endpoint(s) found before writing to block.txt:"
		secret   = "[secrets-guardian] 1 potential secret(s) found before writing to block.txt:"
	)
	for _, tc := range []struct {
		name     string
		exit     int
		stderr   string
		verdict  verdict
		category string
		message  string
	}{
		{name: "exit 0", exit: 0, verdict: verdictAllowed},
		{name: "exit 0 with advice", exit: 0, stderr: advisory + "\n\n" + note + "\n", verdict: verdictAllowed},
		{
			name: "blocking git rule", exit: 2, stderr: blocking + "\n",
			verdict: verdictDenied, category: "destructive_git_ops", message: blocking,
		},
		{
			name: "blocking parse limit", exit: 2,
			stderr: "[commands] 1 blocking threat(s) detected: command_parse_limit - The guard could not inspect " +
				"alternate Git configuration. Use the established sources - not others.\n",
			verdict: verdictDenied, category: "command_parse_limit",
		},
		{
			name: "blocking line with CRLF", exit: 2, stderr: blocking + "\r\n\r\n" + note + "\r\n",
			verdict: verdictDenied, category: "destructive_git_ops", message: blocking,
		},
		{
			name: "bare release denial", exit: 2, stderr: "[commands] " + release + "\n",
			verdict: verdictDenied, category: categoryCommands, message: "[commands] " + release,
		},
		{
			name: "advisory, settings note, release denial", exit: 2,
			stderr:  advisory + "\n\n" + note + "\n\n" + release + "\n",
			verdict: verdictDenied, category: categoryCommands, message: advisory,
		},
		{
			name: "advisory, note with ignored settings file, release denial", exit: 2,
			stderr: advisory + "\n\n" + note + "\nAgentics hook settings: ignored invalid /r/.agentics/hooks.json " +
				"(syntax); every policy default applies.\n\n" + release + "\n",
			verdict: verdictDenied, category: categoryCommands, message: advisory,
		},
		{
			name: "advisory and settings note only", exit: 2, stderr: advisory + "\n\n" + note + "\n",
			verdict: verdictFault,
		},
		{
			name: "advisory with two settings", exit: 2,
			stderr:  advisory + "\n\nSettings: policy.a, policy.b. To allow this, ask the user.\n",
			verdict: verdictFault,
		},
		{
			name: "release denial, then endpoint advisory", exit: 2,
			stderr:  "[commands] " + release + "\n\n" + endpoint + "\n  https://example.invalid\n",
			verdict: verdictDenied, category: categoryCommands, message: "[commands] " + release,
		},
		{
			name: "advisory, then secret finding", exit: 2,
			stderr:  advisory + "\n\n" + note + "\n\n" + secret + "\n  line 1: generic assignment\n",
			verdict: verdictDenied, category: categorySecrets, message: secret,
		},
		{
			name: "blocking line, then secret finding", exit: 2,
			stderr:  blocking + "\n\n" + secret + "\n",
			verdict: verdictDenied, category: "destructive_git_ops", message: blocking,
		},
		{
			name: "release rules could not inspect", exit: 2,
			stderr: "[commands] release-guardian could not inspect a recognized execution path (env child " +
				"executable is not literal); run /agentics:gate change ($agentics:gate change on Codex) and set the release record in the host environment\n",
			verdict: verdictDenied, category: categoryCommands,
		},
		{
			name: "secrets check could not inspect", exit: 2,
			stderr: "[secrets-guardian] secrets-guardian could not inspect a recognized execution path (env " +
				"child executable is not literal)\n",
			verdict: verdictDenied, category: categorySecrets,
		},
		{
			name: "credential file read", exit: 2,
			stderr: "[secrets-guardian] 1 credential file read(s) blocked — this content would enter the " +
				"transcript:\n  .env\n",
			verdict: verdictDenied, category: categorySecrets,
		},
		{
			name: "endpoint advisory only", exit: 2, stderr: endpoint + "\n", verdict: verdictFault,
		},
		{
			name: "a rule message that mentions a failure word", exit: 2,
			stderr:  "[commands] " + release + "; the last check failed\n",
			verdict: verdictDenied, category: categoryCommands,
		},
		{name: "unlabelled hook failure", exit: 2, stderr: "agx: event selection failed\n", verdict: verdictFault},
		{
			name: "unlabelled hook failure after a denial", exit: 2,
			stderr:  blocking + "\nagx: grouped event infrastructure panicked\n",
			verdict: verdictFault,
		},
		{name: "panic", exit: 2, stderr: "[commands] check panicked\n\n[commands] panic\n", verdict: verdictFault},
		{name: "invalid outcome", exit: 2, stderr: "[secrets-guardian] invalid outcome\n", verdict: verdictFault},
		{
			name: "check failed", exit: 2,
			stderr:  "[secrets-guardian] could not validate\n\n[secrets-guardian] check failed\n",
			verdict: verdictFault,
		},
		{name: "missing handler", exit: 2, stderr: "[commands] missing handler\n", verdict: verdictFault},
		{
			name: "deadline", exit: 2, stderr: "[commands] deadline exceeded; incomplete check\n",
			verdict: verdictFault,
		},
		{
			name: "deadline without cleanup", exit: 2, stderr: "[commands] deadline exceeded; cleanup unacknowledged\n",
			verdict: verdictFault,
		},
		{
			name: "compound notice", exit: 2, stderr: "[commands] panic; deadline exceeded; incomplete check\n",
			verdict: verdictFault,
		},
		{
			name: "deadline after a denial", exit: 2,
			stderr:  blocking + "\n\n[commands] deadline exceeded; incomplete check\n",
			verdict: verdictFault,
		},
		{
			name: "worker infrastructure panic", exit: 2, stderr: "[secrets-guardian] worker infrastructure panic\n",
			verdict: verdictFault,
		},
		{
			name: "command rules panicked", exit: 2, stderr: "[commands] command rules panicked\n\n" + release + "\n",
			verdict: verdictFault,
		},
		{
			name: "release rules panicked", exit: 2, stderr: blocking + "\n\nrelease rules panicked\n",
			verdict: verdictFault,
		},
		{
			name: "invalid rule set outcome", exit: 2, stderr: "[commands] rule set returned an invalid outcome\n",
			verdict: verdictFault,
		},
		{name: "exit 1", exit: 1, stderr: "agx: unknown provider\nusage\n", verdict: verdictFault},
		{name: "exit 1 with a denial text", exit: 1, stderr: blocking + "\n", verdict: verdictFault},
		{name: "exit 3", exit: 3, verdict: verdictFault},
		{name: "empty output at exit 2", exit: 2, verdict: verdictFault},
		{name: "unlabelled output at exit 2", exit: 2, stderr: release + "\n", verdict: verdictFault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyRun(tc.exit, "", tc.stderr)
			if got.verdict != tc.verdict || got.category != tc.category {
				t.Fatalf("got %s %q (%s), want %s %q", got.verdict, got.category, got.message, tc.verdict, tc.category)
			}
			if tc.message != "" && got.message != tc.message {
				t.Errorf("message %q, want %q", got.message, tc.message)
			}
			if got.verdict == verdictFault && got.message == "" {
				t.Error("a fault carries no reason")
			}
			if got.exitCode != tc.exit || got.output != tc.stderr {
				t.Errorf("exit %d output %q, want %d %q", got.exitCode, got.output, tc.exit, tc.stderr)
			}
		})
	}

	t.Run("standard output is read when standard error is empty", func(t *testing.T) {
		got := classifyRun(2, blocking+"\n", " \n")
		if got.verdict != verdictDenied || got.category != "destructive_git_ops" {
			t.Errorf("got %s %q, want a destructive_git_ops denial", got.verdict, got.category)
		}
		got = classifyRun(2, blocking+"\n", "agx: event selection failed\n")
		if got.verdict != verdictFault {
			t.Errorf("got %s, want a fault: standard error is not empty, so standard output is not read", got.verdict)
		}
	})
}

// resetHard is a command the command rules deny in every mode a clean environment leaves them in.
var resetHard = block{path: "runner_test.go", tool: toolBash, language: "bash", body: "git reset --hard HEAD~1"}

func requireDenied(t *testing.T, got runResult, category string) {
	t.Helper()
	if got.verdict != verdictDenied || got.category != category {
		t.Fatalf("got %s %q, want denied %q: exit %d, %s\n%s",
			got.verdict, got.category, category, got.exitCode, got.message, got.output)
	}
}

func TestRunnerAllowsAPlainCommand(t *testing.T) {
	t.Parallel()
	got := runBlock(t, block{path: "runner_test.go", tool: toolBash, body: "git status"}, t.TempDir(), outsideRepository)
	if got.verdict != verdictAllowed {
		t.Fatalf("got %s, want allowed: exit %d, %s\n%s", got.verdict, got.exitCode, got.message, got.output)
	}
	if got.category != "" || got.message != "" {
		t.Errorf("an allowed run carries category %q and message %q", got.category, got.message)
	}
}

func TestRunnerDeniesAndNamesTheCategory(t *testing.T) {
	t.Parallel()
	got := runBlock(t, resetHard, t.TempDir(), outsideRepository)
	requireDenied(t, got, "destructive_git_ops")
	if !strings.HasPrefix(got.message, labelCommands+" 1 blocking threat(s) detected: destructive_git_ops - ") {
		t.Errorf("message %q is not the blocking line", got.message)
	}
	if got.exitCode != 2 {
		t.Errorf("exit %d, want 2", got.exitCode)
	}
}

func TestRunnerIgnoresGuardSwitchesInTheParent(t *testing.T) {
	t.Setenv("COMMAND_GUARD_MODE", "warn")
	t.Setenv("SKIP_COMMAND_GUARD", "true")
	requireDenied(t, runBlock(t, resetHard, t.TempDir(), outsideRepository), "destructive_git_ops")
}

func TestRunnerLeavesNoLogInTheCheckout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	requireDenied(t, runBlock(t, resetHard, dir, outsideRepository), "destructive_git_ops")

	log := filepath.Join(dir, ".agentics", "logs", "command-guardian", "guard.log")
	if !isRegularFile(log) {
		t.Errorf("the hook wrote no %s", log)
	}
	// The test binary runs in the package directory, which is where the hook would log if it were not given its own.
	if _, err := os.Lstat(".agentics"); !errors.Is(err, os.ErrNotExist) {
		pkg, _ := os.Getwd()
		t.Errorf("%s holds .agentics after a run (%v)", pkg, err)
	}

	home, temp := runnerDirs(t)
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Errorf("the shared home directory %s is not empty: %v %v", home, entries, err)
	}
	if temp == dir || home == dir {
		t.Errorf("the hook's home %s or temp %s is the working directory", home, temp)
	}
}

func TestRunnerRefusesToRunInsideARepository(t *testing.T) {
	runnerBinary(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := runBlock(t, resetHard, dir, insideRepository); got.verdict != verdictDenied {
		t.Errorf("a deliberate repository: got %s, want denied: %s\n%s", got.verdict, got.message, got.output)
	}

	recorder := &fatalRecorder{TB: t}
	func() {
		defer func() { _ = recover() }()
		runBlock(recorder, resetHard, dir, outsideRepository)
	}()
	if !strings.Contains(recorder.message, "inside the repository at "+dir) {
		t.Errorf("runBlock did not refuse %s: %q", dir, recorder.message)
	}
}

// fatalRecorder records the first fatal message instead of failing the test, and stops the caller as Fatal does.
type fatalRecorder struct {
	testing.TB
	message string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatal(args ...any) {
	r.message = fmt.Sprint(args...)
	panic(r)
}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	panic(r)
}

func TestRunnerIsSafeToCallInParallel(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			if i%2 == 0 {
				requireDenied(t, runBlock(t, resetHard, t.TempDir(), outsideRepository), "destructive_git_ops")
				return
			}
			b := block{path: "runner_test.go", tool: toolWrite, body: "plain text\n"}
			if got := runBlock(t, b, t.TempDir(), outsideRepository); got.verdict != verdictAllowed {
				t.Errorf("got %s, want allowed: %s\n%s", got.verdict, got.message, got.output)
			}
		})
	}
}
