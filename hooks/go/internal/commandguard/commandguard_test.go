package commandguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestBashInspectionCancelled(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, at := range []int{1, 4, 10} {
		ctx := &hookspectest.CancelContext{Context: context.Background(), At: at}
		command := "bash" + strings.Repeat(" ", 6*64*1024) + "--help"
		out := Run(hookspec.Request{Context: ctx, Event: buildEvent(t, "Bash", commandInput(t, command), t.TempDir())})
		if ctx.Calls < at || out.Kind() != "deny" {
			t.Fatalf("cancellation granted permission: %d/%d %s", ctx.Calls, at, out.Kind())
		}
	}
}

func TestBashInspectionPrecision(t *testing.T) {
	for _, command := range []string{"bash --help", "bash --version"} {
		out, _, _ := runGo(t, tableCase{command: command})
		if out.Kind() != "pass" {
			t.Fatalf("metadata: %s %s", out.Kind(), out.Payload())
		}
	}
	for _, command := range []string{"bash --help; rm -rf .git", "alias bash=helper; bash --help", "PATH=other; bash --help", "echo public; bash --help"} {
		out, _, _ := runGo(t, tableCase{command: command})
		if out.Kind() != "deny" {
			t.Fatalf("unsafe or unsupported %q: %s", command, out.Payload())
		}
	}
	for _, field := range []string{`"shell":"bash"`, `"env":{}`, `"metadata":{"shell":"bash"}`} {
		out, _, _ := runGo(t, tableCase{input: `{"command":"bash --help",` + field + `}`})
		if out.Kind() != "deny" {
			t.Fatalf("unqualified metadata: %s", out.Payload())
		}
	}
}

func TestSyntaxReadPrecision(t *testing.T) {
	for _, command := range []string{"bash -n source.sh", "bash -n -- source.sh", `bash -n '.e[n]v'`} {
		t.Run(command, func(t *testing.T) {
			out, _, _ := runGo(t, tableCase{command: command})
			if out.Kind() != "pass" {
				t.Fatalf("syntax read = %s %s", out.Kind(), out.Payload())
			}
		})
	}
	for _, tc := range []struct{ command, category string }{
		{"bash --version; git commit --no-verify -m x", "git_hook_bypass"},
		{"bash -n source.sh; git commit --no-verify -m x", "git_hook_bypass"},
		{"bash -n source.sh; rm -rf .git", "destructive_file_ops"},
		{"bash source.sh", "command_parse_limit"},
		{`bash -n '.e'*`, "command_parse_limit"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			out, _, _ := runGo(t, tableCase{command: tc.command})
			if out.Kind() != "deny" || !strings.Contains(out.Payload(), tc.category) {
				t.Fatalf("real command lost: %s %s; want %s", out.Kind(), out.Payload(), tc.category)
			}
		})
	}
}

// defaultTestCwd is the working directory of events that name none. The
// directory does not exist, so a settings file is found only if an ancestor
// holds a .git entry, and none does in practice.
const defaultTestCwd = "/tmp/guardian-core-cwd"

// buildEvent builds a claude-code pre-tool-use neutral event with the given
// tool and input.
func buildEvent(t *testing.T, toolName, toolInputJSON, logRoot string) hookspec.Event {
	t.Helper()
	return buildEventIn(t, defaultTestCwd, toolName, toolInputJSON, logRoot)
}

// buildEventIn is buildEvent with an explicit event working directory.
func buildEventIn(t *testing.T, dir, toolName, toolInputJSON, logRoot string) hookspec.Event {
	t.Helper()
	var input map[string]any
	decoder := json.NewDecoder(strings.NewReader(toolInputJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		t.Fatalf("invalid tool_input fixture %q: %v", toolInputJSON, err)
	}
	session := "guardian-core-session"
	event, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.ClaudeCode,
		SessionID: &session,
		Cwd:       &dir,
		Runtime:   contract.NeutralRuntime{LogRoot: &logRoot},
		ToolName:  toolName,
		ToolInput: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func commandInput(t *testing.T, command string) string {
	t.Helper()
	return mustEncodeCompact(t, map[string]string{"command": command})
}

// tableCase is one behavioral case. TestGuardianSemantics asserts the handler
// outcome kind and reason, plus diagnostic and audit-log presence.
type tableCase struct {
	name    string
	tool    string // defaults to Bash
	input   string // tool_input JSON; built from command when empty
	command string
	env     map[string]string
	cwd     string // event working directory; defaults to defaultTestCwd

	settings *settings.Source // workflow settings the request carries; nil reads the files

	want       string // pass | deny | context | error
	wantReason string // exact deny reason / context message when set
}

func (c tableCase) workingDir() string {
	if c.cwd == "" {
		return defaultTestCwd
	}
	return c.cwd
}

func (c tableCase) toolName() string {
	if c.tool == "" {
		return "Bash"
	}
	return c.tool
}

func (c tableCase) toolInput(t *testing.T) string {
	if c.input != "" {
		return c.input
	}
	return commandInput(t, c.command)
}

// wantLog reports whether the case must leave a guard.log record: threats
// only — never a happy-path record.
func (c tableCase) wantLog() bool {
	return c.want == "deny" || c.want == "context"
}

const (
	denyRootReason  = "1 blocking threat(s) detected: destructive_file_ops - Remove only specific, verified paths instead of the filesystem root. If the user still wants to delete the root, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal."
	warnRootMessage = "1 threat(s) detected: destructive_file_ops - Remove only specific, verified paths instead of the filesystem root. If the user still wants to delete the root, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal."
)

// tableCases builds the behavioral bank: branch-coverage cases, the key
// behavioral classes, the Windows drive-root boundary suites, and the
// Semantic case bank.
func tableCases(t *testing.T) []tableCase {
	t.Helper()
	var cases []tableCase
	add := func(c tableCase) { cases = append(cases, c) }

	// --- branch coverage: paths no earlier case reached ----------------------
	// A Bash payload with no command key falls back to scanning the whole
	// input with strict segmentation, so a gated verb in another field is
	// still caught (fail closed).
	add(tableCase{name: "strict_segmentation_without_command_key", tool: "Bash",
		input: `{"description":"gh pr create --title live"}`, want: "deny"})
	add(tableCase{name: "unclassified_task_input_denies", tool: "Bash",
		input: `{"command":"print(1)\n","unclassified_task_input":true}`, want: "deny"})
	add(tableCase{name: "control_character_inside_match", command: "rm -rf /\x01", want: "deny"})
	add(tableCase{name: "git_clean_force_long_flag", command: "git clean --force", want: "deny"})
	add(tableCase{name: "git_clean_dry_run_long_flag", command: "git clean --dry-run -f .", want: "pass"})
	add(tableCase{name: "git_clean_exclude_consumes_its_pattern", command: "git clean -e '*.log' -f src", want: "pass"})
	add(tableCase{name: "git_clean_bare", command: "git clean", want: "pass"})
	add(tableCase{name: "git_clean_double_dash_then_dot", command: "git clean -f -- .", want: "deny"})

	// --- key behavioral classes ---------------------------------------------
	add(tableCase{name: "safe", command: "printf safe",
		env: map[string]string{"COMMAND_GUARD_MODE": "warn"}, want: "pass"})
	add(tableCase{name: "env_mode_block_denies", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_MODE": "block"}, want: "deny", wantReason: denyRootReason})
	add(tableCase{name: "env_mode_warn_gives_context", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_MODE": "warn"}, want: "context", wantReason: warnRootMessage})
	add(tableCase{name: "env_mode_unknown_value_blocks", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_MODE": "blcok"}, want: "deny", wantReason: unknownModeWarning + "\n\n" + denyRootReason})
	add(tableCase{name: "block", command: "rm -rf /", want: "deny", wantReason: denyRootReason})
	add(tableCase{name: "disabled", command: "rm -rf /",
		env: map[string]string{"SKIP_COMMAND_GUARD": "true"}, want: "pass"})
	add(tableCase{name: "allowlisted", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_ALLOWLIST": "rm -rf /"}, want: "pass"})
	add(tableCase{name: "allowlist_trims_and_splits", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_ALLOWLIST": "no-match, rm -rf / ,"}, want: "pass"})
	add(tableCase{name: "allowlist_miss_still_blocks", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_ALLOWLIST": "printf safe"}, want: "deny", wantReason: denyRootReason})
	add(tableCase{name: "execution_self_documentation_text_does_not_bypass",
		command: "rm -rf /; printf hooks/command-guardian/", want: "deny"})
	add(tableCase{name: "unknown_execution_self_documentation_text_does_not_bypass",
		tool: "UnknownExecutor", input: `{"command":"rm -rf /; printf hooks/command-guardian/"}`, want: "deny"})
	// The three Edit cases pass because Edit is a non-execution tool; there
	// is no path-based exemption.
	add(tableCase{name: "guardian_documentation_edit", tool: "Edit",
		input: `{"file_path":"/tmp/agentics/hooks/command-guardian/README.md","old_string":"safe","new_string":"rm -rf /"}`,
		want:  "pass"})
	add(tableCase{name: "guardian_documentation_edit_windows", tool: "Edit",
		input: `{"file_path":"C:\\src\\agentics\\hooks\\command-guardian\\README.md","new_string":"rm -rf /"}`,
		want:  "pass"})
	add(tableCase{name: "similar_non_guardian_path_is_not_self_documentation", tool: "Edit",
		input: `{"file_path":"/tmp/foohooks/command-guardian/README.md","new_string":"rm -rf /"}`,
		want:  "pass"})
	add(tableCase{name: "sibling_field_cannot_vouch",
		input: `{"command":"gh pr create --title live","description":"the docs mention --draft"}`, want: "deny"})
	add(tableCase{name: "known_execution_sibling_cannot_trigger",
		input: `{"command":"printf safe","query":"DROP DATABASE production"}`, want: "pass"})
	add(tableCase{name: "nested_structured_fields",
		input: `{"command":"printf safe","metadata":{"quote":"a\\\"b","items":[1,true,null],"path":"C:\\\\tmp"}}`,
		want:  "pass"})
	add(tableCase{name: "mcp_empty_command_scans_structured_sibling", tool: "mcp__db__query",
		input: `{"command":"","value":{"query":"DROP DATABASE production"}}`, want: "deny"})
	add(tableCase{name: "mcp_safe_command_scans_dangerous_query", tool: "mcp__db__query",
		input: `{"command":"SELECT 1","query":"DROP DATABASE production"}`, want: "deny"})
	add(tableCase{name: "mcp_dangerous_command_scans_safe_query", tool: "mcp__db__query",
		input: `{"command":"DROP DATABASE production","query":"SELECT 1"}`, want: "deny"})
	add(tableCase{name: "mcp_safe_command_and_query_pass", tool: "mcp__db__query",
		input: `{"command":"SELECT 1","query":"SELECT 2"}`, want: "pass"})
	add(tableCase{name: "mcp_drop_newline_database", tool: "mcp__db__query",
		input: `{"command":"SELECT 1","query":"DROP\nDATABASE production"}`, want: "deny"})
	add(tableCase{name: "mcp_drop_tab_table", tool: "mcp__db__query",
		input: `{"command":"SELECT 1","query":"DROP\tTABLE production"}`, want: "deny"})
	add(tableCase{name: "mcp_truncate_crlf_table", tool: "mcp__db__query",
		input: `{"command":"SELECT 1","query":"TRUNCATE\r\nTABLE production"}`, want: "deny"})
	add(tableCase{name: "mcp_delete_newline_from", tool: "mcp__db__query",
		input: `{"command":"SELECT 1","query":"DELETE\nFROM users;"}`, want: "deny"})
	add(tableCase{name: "mcp_database_destruction", tool: "mcp__db__query",
		input: `{"value":{"query":"DROP DATABASE production"}}`, want: "deny"})
	add(tableCase{name: "inert_heredoc",
		command: "cat > /tmp/guardian-note <<'EOF'\nrm -rf /\nEOF", want: "pass"})
	add(tableCase{name: "active_heredoc",
		command: "bash <<'EOF'\nrm -rf /\nEOF", want: "deny"})
	add(tableCase{name: "hostile_string_is_data",
		command: "printf '%s' '$(touch /tmp/guardian-hostile-marker)'", want: "pass"})

	// --- Windows drive-root delete boundary suites --------------------------
	winBoundary := func(label, root string) {
		child := root + "tmp"
		pairs := []struct {
			suffix string
			kind   string
		}{
			{" ", "space"}, {"\t", "tab"}, {"\n", "newline"},
			{" && echo safe", "and"}, {" & echo safe", "background"},
			{" | echo safe", "pipe"}, {"; echo safe", "semicolon"},
			{">nul", "redirect"}, {">>log.txt", "redirect_append"},
			{"<nul", "input_redirect"},
		}
		for _, p := range pairs {
			add(tableCase{name: "windows_" + label + "_root_" + p.kind, command: root + p.suffix, want: "deny"})
			add(tableCase{name: "windows_" + label + "_child_" + p.kind, command: child + p.suffix, want: "pass"})
		}
		add(tableCase{name: "windows_" + label + "_root_group_close", command: "(" + root + ")& echo safe", want: "deny"})
		add(tableCase{name: "windows_" + label + "_child_group_close", command: "(" + child + ")& echo safe", want: "pass"})
	}
	winBoundary("rd", `rd /s /q C:\`)
	winBoundary("del", `del /f /s /q C:\`)

	winCase := func(name, want, command string) {
		add(tableCase{name: name, command: command, want: want})
	}
	winCase("windows_rd_reorder_root", "deny", `rd /q /s C:\`)
	winCase("windows_rd_reorder_child", "pass", `rd /q /s C:\Temp`)
	winCase("windows_rd_no_space_flags_root", "deny", `rd /s/q C:\`)
	winCase("windows_rd_no_space_flags_child", "pass", `rd /s/q C:\Temp`)
	winCase("windows_rd_double_space_root", "deny", `rd  /s  /q  C:\`)
	winCase("windows_rd_double_space_child", "pass", `rd  /s  /q  C:\Temp`)
	winCase("windows_rd_tab_flags_root", "deny", "rd\t/s\t/q\tC:\\")
	winCase("windows_rd_tab_flags_child", "pass", "rd\t/s\t/q\tC:\\Temp")
	winCase("windows_rd_newline_flags_root", "deny", "rd\n/s\n/q\nC:\\")
	winCase("windows_rd_newline_flags_child", "pass", "rd\n/s\n/q\nC:\\Temp")
	winCase("windows_rd_cr_flags_root", "deny", "rd\r/s\r/q\rC:\\")
	winCase("windows_rd_cr_flags_child", "pass", "rd\r/s\r/q\rC:\\Temp")
	winCase("windows_del_reorder_root", "deny", `del /q /f /s C:\`)
	winCase("windows_del_reorder_child", "pass", `del /q /f /s C:\Users\me\build`)
	winCase("windows_del_no_space_flags_root", "deny", `del /f/s/q C:\`)
	winCase("windows_del_no_space_flags_child", "pass", `del /f/s/q C:\Users\me\build`)
	winCase("windows_del_double_space_root", "deny", `del  /f  /s  /q  C:\`)
	winCase("windows_del_double_space_child", "pass", `del  /f  /s  /q  C:\Users\me\build`)
	winCase("windows_del_tab_flags_root", "deny", "del\t/f\t/s\t/q\tC:\\")
	winCase("windows_del_tab_flags_child", "pass", "del\t/f\t/s\t/q\tC:\\Users\\me\\build")
	winCase("windows_del_newline_flags_root", "deny", "del\n/f\n/s\n/q\nC:\\")
	winCase("windows_del_newline_flags_child", "pass", "del\n/f\n/s\n/q\nC:\\Users\\me\\build")
	winCase("windows_del_cr_flags_root", "deny", "del\r/f\r/s\r/q\rC:\\")
	winCase("windows_del_cr_flags_child", "pass", "del\r/f\r/s\r/q\rC:\\Users\\me\\build")
	winCase("windows_rd_percent_boundary_root", "deny", `rd /s /q C:\%comspec:~0,0%&calc`)
	winCase("windows_rd_percent_bare_root", "deny", `rd /s /q C:\%`)
	winCase("windows_rd_percent_child", "pass", `rd /s /q C:\Temp%`)
	winCase("windows_rd_percent_child_full", "pass", `rd /s /q C:\Temp%comspec:~0,0%&calc`)
	winCase("windows_del_percent_boundary_root", "deny", `del /f /s /q C:\%comspec:~0,0%&calc`)
	winCase("windows_del_percent_child", "pass", `del /f /s /q C:\Users\me\build%foo%`)
	winCase("windows_rd_comma_boundary_root", "deny", `rd /s /q C:\,calc`)
	winCase("windows_rd_equals_boundary_root", "deny", `rd /s /q C:\=calc`)
	winCase("windows_rd_caret_not_boundary", "pass", `rd /s /q C:\^calc`)
	winCase("windows_rd_bang_not_boundary", "pass", `rd /s /q C:\!calc`)
	winCase("windows_rd_bare_no_flags_allowed", "pass", `rd C:\`)
	winCase("windows_rd_missing_s_allowed", "pass", `rd /q C:\`)
	winCase("windows_del_missing_s_allowed", "pass", `del /f /q C:\`)
	winCase("windows_del_missing_s_bare_allowed", "pass", `del /q C:\`)
	winCase("windows_rd_bare_s_root", "deny", `rd /s C:\`)
	winCase("windows_rd_bare_s_child", "pass", `rd /s C:\Temp`)
	winCase("windows_rd_bare_s_redirect_root", "deny", `rd /s C:\>nul`)
	winCase("windows_rd_bare_s_redirect_child", "pass", `rd /s C:\Temp>nul`)
	winCase("windows_del_bare_s_root", "deny", `del /s C:\`)
	winCase("windows_del_bare_s_child", "pass", `del /s C:\Temp`)
	winCase("windows_del_s_q_root", "deny", `del /s /q C:\`)
	winCase("windows_del_s_q_child", "pass", `del /s /q C:\Temp`)
	winCase("windows_del_f_s_root", "deny", `del /f /s C:\`)
	winCase("windows_del_f_s_child", "pass", `del /f /s C:\Users\me\build`)
	winCase("windows_rd_flags_after_root", "deny", `rd C:\ /s`)
	winCase("windows_rd_flags_after_child", "pass", `rd C:\Temp /s`)
	winCase("windows_rd_flags_after_reorder_root", "deny", `rd C:\ /q /s`)
	winCase("windows_del_flags_after_root", "deny", `del C:\ /s /q`)
	winCase("windows_del_flags_after_child", "pass", `del C:\Users\me\build /s /q`)
	winCase("windows_del_flags_after_reorder_root", "deny", `del C:\ /q /s /f`)

	// --- the 165-case semantic bank ------------------------------------------
	semanticCount := 0
	kindOf := map[string]string{"allow": "pass", "block": "deny", "context": "context"}
	tuning := func(expect, tool, input string) {
		semanticCount++
		want, ok := kindOf[expect]
		if !ok {
			t.Fatalf("unknown semantic expectation: %s", expect)
		}
		add(tableCase{
			name:  fmt.Sprintf("semantic-%03d", semanticCount),
			tool:  tool,
			input: input,
			want:  want,
		})
	}
	shellCase := func(expect, command string) {
		tuning(expect, "Bash", mustCommandInput(t, command))
	}

	// Claude Code's native PowerShell tool carries a shell command too. It is
	// promoted and scanned exactly as Bash is, so the gh publication gates and
	// the git checks reach it. Copilot's adapter already normalizes its
	// powershell tool to Bash; this keeps the two surfaces consistent.
	powershellCase := func(expect, command string) {
		tuning(expect, "PowerShell", mustCommandInput(t, command))
	}

	shellCase("allow", "git clean -n")
	shellCase("allow", "git clean -nd dist/")
	shellCase("allow", "git clean -fd dist/ build/")
	shellCase("allow", "git clean -fdq packages/app/tmp")
	shellCase("allow", "git clean -fd -- dist/")
	shellCase("block", "git clean -fd")
	shellCase("block", "git clean --force")
	shellCase("block", "git clean -f .")
	shellCase("block", "git clean -fd *")
	shellCase("block", "git clean -fdx dist/")
	shellCase("block", "git clean -fd :")
	shellCase("block", "git clean -fd :^cache")
	shellCase("block", "git clean -f src :")
	shellCase("block", `git clean -fd src \*`)
	shellCase("allow", "git reset --hard HEAD")
	shellCase("block", "git reset --hard")
	shellCase("block", "git reset --hard HEAD~1")
	shellCase("block", `git reset --hard HEAD\~1`)
	shellCase("block", "git reset --hard HEAD'~1'")
	shellCase("block", "git reset --hard origin/main")
	shellCase("block", "git reset --hard HEAD; git reset --hard origin/main")
	shellCase("allow", "git restore .")
	shellCase("context", "rm -rf node_modules")
	tuning("allow", "AskUserQuestion", `{"questions":[{"question":"Delete the .git directory or run rm -rf / to start over?","options":[{"label":"Never","description":"git push --force origin main is also bad"}]}]}`)
	tuning("allow", "Grep", `{"pattern":"rm -rf /|DROP TABLE","path":"skills"}`)
	tuning("allow", "Glob", `{"pattern":"**/.git/**"}`)

	// Windows drive-root deletion must block without overmatching a child path.
	shellCase("block", `rd /s /q C:\`)
	shellCase("allow", `rd /s /q C:\tmp`)
	shellCase("block", `del /f /s /q C:\`)
	shellCase("allow", `del /f /s /q C:\tmp\artifact.txt`)

	// Help probes of publish-shaped gh commands.
	shellCase("allow", "gh pr create --help")
	shellCase("allow", "gh pr create -h")
	shellCase("allow", "gh pr ready --help")
	shellCase("block", "gh pr create --title x")
	shellCase("block", "gh pr create -H feature-branch --title x")
	shellCase("block", "gh pr ready 42")
	shellCase("allow", "gh pr create --draft --title x")
	shellCase("allow", "gh pr create -d --title x")
	shellCase("block", "gh pr create --help; gh pr create --title live")
	shellCase("block", "gh pr create --draft --title a; gh pr create --title live")
	shellCase("block", "gh pr ready --help; gh pr ready 42")
	shellCase("block", "gh pr create --title x\ngh pr create --draft --title y")
	shellCase("block", "gh pr create --title \"require --draft here\"")
	shellCase("block", "gh pr create --title 'require --draft here'")
	shellCase("allow", "gh pr create --title \"x\" --draft")

	// Multi-path targeted rm.
	shellCase("context", "rm -rf /tmp/dbg.out /tmp/dbg.err /tmp/dbg-test")
	shellCase("context", "rm -rf /tmp/agentics-roottest-vault")
	shellCase("block", "rm -rf /")
	shellCase("block", "rm -rf / ; echo done")
	shellCase("block", "rm -rf /\necho next line")
	shellCase("block", "cat <<EOF; rm -rf /\nbody\nEOF")
	shellCase("context", "rm -rf /tmp/scratch\necho next line")

	// Inert heredoc bodies.
	shellCase("allow", "cat > /tmp/pr-reply.md <<EOF\nPlease avoid git reset --hard origin/main here.\nEOF")
	shellCase("allow", "cat <<'EOF' > /tmp/note.md\nThe old advice was rm -rf / which is wrong.\nEOF")
	shellCase("allow", "tee /tmp/doc.md <<EOF\nNever run git push --force origin main.\nEOF")
	shellCase("block", "cat > /tmp/x <<EOF\nharmless\nEOF\ngit reset --hard origin/main")
	shellCase("block", "cat <<EOF | bash\nrm -rf /\nEOF")
	shellCase("block", "bash <<EOF\nrm -rf /\nEOF")
	shellCase("block", "cat <<-EOF > /tmp/x\ntext\n\tEOF\ngit reset --hard origin/main")
	shellCase("block", "cat <<-EOF > /tmp/x\ntext\n\tEOF\nrm -rf /")
	shellCase("block", "cat > >(bash) <<EOF\nrm -rf /\nEOF")
	shellCase("block", "cat <<EOF > >(bash)\nrm -rf /\nEOF")
	shellCase("block", "tee >(bash) <<EOF\nrm -rf /\nEOF")
	shellCase("block", "bash -c \"$( cat <<EOF\nrm -rf /\nEOF\n)\"")
	shellCase("block", "cat <<EOF > /tmp/x; bash /tmp/x\nrm -rf /\nEOF")
	shellCase("block", "cat > /tmp/x <<EOF\n$(rm -rf /)\nEOF")
	shellCase("block", "cat > /tmp/x <<EOF\n`rm -rf /`\nEOF")
	shellCase("block", "cat > /tmp/x <<EOF\n$(git reset --hard origin/main)\nEOF")
	shellCase("block", "cat > /tmp/x <<EOF\n$(rm -rf /)")
	shellCase("allow", "cat > /tmp/x <<'EOF'\n$(rm -rf /)\nEOF")
	shellCase("allow", "cat > /tmp/x <<EOF\nprose naming rm -rf / with no substitution\nEOF")
	shellCase("block", "echo $(rm -rf /)")
	shellCase("block", "echo `rm -rf ~`")

	// Quote-aware gh segmentation; sibling fields never vouch.
	tuning("allow", "Bash", `{"command":"gh pr create --title \"a;b\" --draft"}`)
	tuning("allow", "Bash", `{"command":"gh pr create --title \"fix: a|b\" --draft"}`)
	tuning("block", "Bash", `{"command":"gh pr create --title \"a;b\""}`)
	tuning("block", "Bash", `{"command":"gh pr create --title live","description":"open a PR with --draft set"}`)
	tuning("block", "Bash", `{"command":"gh pr create --title \"say \\\"--draft\\\" here\""}`)

	// Boolean assignment forms.
	shellCase("block", "gh pr create --help=false --title live --body x")
	shellCase("block", "gh pr create --draft=false --title live")
	shellCase("block", "gh pr ready 42 --help=false")
	shellCase("block", "gh pr create --draft=0 --title live")
	shellCase("block", "gh pr create --draft= --title live")
	shellCase("allow", "gh pr create --draft=true --title live")
	shellCase("allow", "gh pr create --draft=1 --title live")
	shellCase("allow", "gh pr create --help=true")

	// Shell comments.
	shellCase("block", "gh pr create --title live --body x # --draft")
	shellCase("block", "gh pr create --title live # --help")
	shellCase("allow", "gh pr create --title \"fix #42\" --draft")
	shellCase("block", "gh pr create --title \"fix #42 --draft\"")

	// gh stack submit.
	shellCase("allow", "gh stack submit --help")
	shellCase("block", "gh stack submit")
	shellCase("allow", "gh stack submit --auto")
	shellCase("block", "gh stack submit --auto --open")
	shellCase("block", "gh stack submit --open")
	shellCase("allow", "gh stack submit --auto --open=false")
	shellCase("block", "gh stack submit --auto; gh stack submit")

	// gh stack link.
	shellCase("allow", "gh stack link --help")
	shellCase("allow", "gh stack link a b")
	shellCase("block", "gh stack link --open a b")
	shellCase("allow", "gh stack link --open=false a b")

	// Redirects do not end the command.
	shellCase("block", "gh stack submit --auto 2>/dev/null --open")
	shellCase("block", "gh stack submit --auto >/tmp/o --open")
	shellCase("block", "gh stack submit --auto < /dev/null --open")
	shellCase("block", "gh stack submit --auto &> /tmp/x --open")
	shellCase("block", "gh stack link a 2>/dev/null --open b")
	shellCase("block", "gh stack submit --auto 2>&1 --open")
	shellCase("allow", "gh stack submit 2>/dev/null --auto")
	shellCase("allow", "gh stack submit --auto 2>/dev/null")
	shellCase("allow", "gh pr create -t x 2>/dev/null --draft")
	shellCase("allow", "gh stack view | grep foo")
	shellCase("block", "gh stack view | gh stack submit")
	shellCase("block", "cat <(gh stack submit)")
	shellCase("block", "tee >(gh stack submit)")
	shellCase("block", "gh stack view > $(gh stack submit)")
	shellCase("block", "cat <(gh pr create --title x)")
	shellCase("block", "cat <(gh stack submit --auto --open)")
	shellCase("allow", "cat <(gh pr create --draft --title x)")
	shellCase("allow", "cat <(gh stack submit --auto)")
	shellCase("allow", "gh stack submit --auto > out.txt")

	// Backslash line continuations.
	shellCase("block", "gh stack \\\n  submit")
	shellCase("block", "gh \\\n  stack submit")
	shellCase("block", "gh stack submit --auto \\\n  --open")
	shellCase("block", "gh pr \\\n  create -t x")
	shellCase("allow", "gh stack submit \\\n  --auto")
	shellCase("allow", "gh pr create \\\n  --title t \\\n  --draft")

	// Everything after a bare -- is positional.
	shellCase("block", "gh stack submit -- --auto")
	shellCase("block", "gh pr create -- --draft")
	shellCase("block", "gh pr create --title x -- --draft")

	// Unsafe finding disposal.
	shellCase("block", `gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "RT_abc"}) { thread { id } } }'`)
	shellCase("block", `gh api graphql -f query='mutation { unresolveReviewThread(input: {threadId: "RT_abc"}) { thread { id } } }'`)
	tuning("block", "mcp__github__graphql", `{"query":"mutation { resolveReviewThread(input:{threadId:\"RT_abc\"}){thread{id}} }"}`)
	shellCase("allow", `gh api graphql -f query='mutation { addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: "RT_abc", body: "done"}) { comment { id } } }'`)
	shellCase("block", "gh api -X PATCH repos/o/r/code-scanning/alerts/42 -f state=dismissed -f dismissed_reason='false positive'")
	shellCase("block", "gh api --method PATCH repos/o/r/code-scanning/alerts/42 -f state=dismissed")
	shellCase("block", `gh api -X PATCH repos/o/r/code-scanning/alerts/42 -f state=\"dismissed\"`)
	shellCase("block", `curl -X PATCH https://api.github.com/repos/o/r/code-scanning/alerts/7 -d '{"state":"dismissed","dismissed_reason":"wont fix"}'`)
	shellCase("block", "gh api -X PATCH repos/o/r/dependabot/alerts/3 -f state=dismissed -f dismissed_reason=tolerable_risk")
	shellCase("allow", "gh api repos/o/r/code-scanning/alerts")
	shellCase("allow", "gh api 'repos/o/r/code-scanning/alerts?state=dismissed'")
	shellCase("allow", "gh api 'repos/o/r/code-scanning/alerts?state=dismissed&per_page=100'")
	shellCase("allow", "gh api repos/o/r/code-scanning/alerts/42")
	shellCase("allow", "gh api repos/o/r/code-scanning/alerts/42 --jq .dismissed_reason")
	shellCase("allow", `gh api repos/o/r/code-scanning/alerts --jq '.[] | select(.state == "dismissed")'`)

	// Shell line continuations are joined before pattern scanning.
	shellCase("block", "gh api graphql -f query=\"mutation { resolveReview\\\nThread(input: {threadId: \\\"RT_abc\\\"}) { thread { id } } }\"")
	shellCase("block", "gh api -X PATCH repos/o/r/code-scanning/alerts/42 -f state=dis\\\nmissed")
	shellCase("block", "gh api -X PATCH repos/o/r/code-scanning/alerts/42 -f state=dismissed -f dismissed_rea\\\nson=false_positive")

	// Dismissal rules are scoped to GitHub alert surfaces.
	shellCase("allow", "curl -X PATCH https://example.test/jobs/1 -d state=dismissed")
	shellCase("allow", `curl -X PATCH https://example.test/jobs/1 -d '{"state":"dismissed","dismissed_reason":"duplicate"}'`)
	shellCase("allow", "./notify-cli update --id 4 -f state=dismissed")
	shellCase("block", `curl -X PATCH -d '{"state":"dismissed","dismissed_reason":"x"}' https://api.github.com/repos/o/r/code-scanning/alerts/7`)
	shellCase("block", "X=repos/o/r/code-scanning/alerts/1; gh api -X PATCH $X -f state=dismissed")

	// Drafted prose naming the gated calls rides the inert-heredoc exemption.
	shellCase("allow", "cat > /tmp/reply.md <<EOF\nDismissal is your call — this run did not set state=dismissed or run resolveReviewThread.\nEOF")

	// PowerShell cases sit last so the recorded semantic-NNN names above them
	// keep their numbers.
	powershellCase("block", "gh pr create --title live")
	powershellCase("allow", "gh pr create --title live --draft")
	powershellCase("block", "git reset --hard HEAD~1")
	powershellCase("allow", "Get-ChildItem -Recurse -Filter *.go")

	if semanticCount != 157 {
		t.Fatalf("semantic bank expected 157 cases, got %d", semanticCount)
	}

	// Publication hardening. An npm publish is a preview only when a complete
	// invocation is effectively help or a dry run, and a comment suffix grants
	// neither. Every denial pins its category and a count of exactly one.
	npmDeny := "1 blocking threat(s) detected: system_danger - " + npmPublishSuggestion + "\n\n" +
		settings.Note(settings.Result{}, []settings.Key{settings.BlockPackagePublish})
	ghDeny := "1 blocking threat(s) detected: unsafe_publication - " + ghPrReadySuggestion + "\n\n" +
		settings.Note(settings.Result{}, []settings.Key{settings.DraftPRs})
	for _, h := range []struct {
		name, command, wantReason string
	}{
		{"npm preview", "npm publish --dry-run", ""},
		{"npm false dry run denied", "npm publish --dry-run=false", npmDeny},
		{"npm end marker selects command but cannot grant preview", "npm -- publish --dry-run", npmDeny},
		{"npm version after end marker", "npm -- version", ""},
		{"an incomplete invocation cannot grant preview", "xargs -- npm publish --dry-run", npmDeny},
		{"true", "npm publish --dry-run true", ""},
		{"false", "npm publish --dry-run false", npmDeny},
		{"dynamic", "npm publish --dry-run $DRY_RUN", npmDeny},
		{"global false", "npm --dry-run false publish", npmDeny},
		{"boolean config before publish", "npm --commit-hooks publish", npmDeny},
		{"second boolean config before publish", "npm --git-tag-version publish", npmDeny},
		{"audit boolean before publish", "npm --audit false publish", npmDeny},
		{"bin links boolean before publish", "npm --bin-links false publish", npmDeny},
		{"separated boolean config before publish", "npm --ignore-scripts false publish", npmDeny},
		{"global config value before publish", "npm --globalconfig /tmp/npmrc publish", npmDeny},
		{"color value before publish", "npm --color always publish", npmDeny},
		{"proxy value before publish", "npm --proxy http://proxy.test publish", npmDeny},
		{"HTTPS proxy value before publish", "npm --https-proxy http://proxy.test publish", npmDeny},
		{"script shell value before publish", "npm --script-shell /bin/sh publish", npmDeny},
		{"call shorthand value before publish", "npm -c echo publish", npmDeny},
		{"location shorthand value before publish", "npm -L global publish", npmDeny},
		{"prefix shorthand value before publish", "npm -C /tmp publish", npmDeny},
		{"message shorthand value before publish", "npm -m note publish", npmDeny},
		{"call shorthand consumes postverb preview data", "npm publish -c --dry-run", npmDeny},
		{"global shorthand boolean before publish", "npm -g false publish", npmDeny},
		{"all shorthand boolean before publish", "npm -a false publish", npmDeny},
		{"help shorthand boolean before publish", "npm -h false publish", npmDeny},
		{"help long boolean before publish", "npm --help false publish", npmDeny},
		{"browser optional value before publish", "npm --browser chrome publish", npmDeny},
		{"compact boolean cluster before publish", "npm -gf false publish", npmDeny},
		{"compact value cluster before publish", "npm -gc echo publish", npmDeny},
		{"unknown attached call shorthand before publish", "npm -cfoo publish", npmDeny},
		{"unknown attached prefix shorthand before publish", "npm -C/tmp publish", npmDeny},
		{"unknown attached workspace shorthand before publish", "npm -wworkspace publish", npmDeny},
		{"unknown attached location shorthand before publish", "npm -Lglobal publish", npmDeny},
		{"unknown attached message shorthand before publish", "npm -mnote publish", npmDeny},
		{"registry shorthand before publish", "npm --reg https://registry.test publish", npmDeny},
		{"enjoy-by shorthand before publish", "npm --enjoy-by 2025-01-01 publish", npmDeny},
		{"workspace shorthand before publish", "npm --ws false publish", npmDeny},
		{"include workspace shorthand before publish", "npm --iwr false publish", npmDeny},
		{"readonly shorthand before publish", "npm --readonly false publish", npmDeny},
		{"description shorthand before publish", "npm --desc false publish", npmDeny},
		{"local shorthand before publish", "npm --local false publish", npmDeny},
		{"bare help shorthand before publish", "npm help false publish", npmDeny},
		{"negated audit before publish", "npm --no-audit false publish", npmDeny},
		{"negated bin links before publish", "npm --no-bin-links false publish", npmDeny},
		{"negated audit preserves preview", "npm --no-audit publish --dry-run", ""},
		{"unique value abbreviation before publish", "npm --userconf /tmp/npmrc publish", npmDeny},
		{"negated dry run conflicts with preview", "npm publish --dry-run --no-dry-run", npmDeny},
		{"unknown long boolean before publish", "npm --mystery false publish", npmDeny},
		{"unknown short boolean before publish", "npm -z false publish", npmDeny},
		{"unknown cluster boolean before publish", "npm -zf false publish", npmDeny},
		{"abbreviated negated dry run conflicts with preview", "npm publish --dry-run --no-dry=true", npmDeny},
		{"attached negated help conflicts with help", "npm publish --help --no-usage=true", npmDeny},
		{"comment suffix cannot grant undo", "gh pr ready 42 #review --undo", ghDeny},
		{"comment suffix cannot grant ready help", "gh pr ready 42 #review --help", ghDeny},
		{"comment suffix cannot grant preview", "npm publish #review --dry-run", npmDeny},
		{"comment suffix cannot grant help", "npm publish #review --help", npmDeny},
		{"mixed quote comment cannot grant preview", "npm publish #\"review\" --dry-run", npmDeny},
	} {
		c := tableCase{name: "publication hardening " + h.name, command: h.command, want: "pass"}
		if h.wantReason != "" {
			c.want, c.wantReason = "deny", h.wantReason
		}
		add(c)
	}

	return cases
}

func mustCommandInput(t *testing.T, command string) string {
	t.Helper()
	return mustEncodeCompact(t, map[string]string{"command": command})
}

// runGo executes one case through the Go handler with a scrubbed environment,
// returning the typed outcome, the diagnostics, and the log root.
func runGo(t *testing.T, c tableCase) (hookspec.Outcome, string, string) {
	t.Helper()
	logRoot := t.TempDir()
	hosttest.ScrubGuardEnv(t)
	for k, v := range c.env {
		t.Setenv(k, v)
	}
	payload := buildEventIn(t, c.workingDir(), c.toolName(), c.toolInput(t), logRoot)
	var diag bytes.Buffer
	outcome := Run(hookspec.Request{
		Provider: "claude-code",
		Hook:     "command-guardian",
		Event:    payload,
		Diag:     &diag,
		Settings: c.settings,
	})
	return outcome, diag.String(), logRoot
}

func guardLogPath(logRoot string) string {
	return filepath.Join(logRoot, "agentics", "logs", "command-guardian", "guard.log")
}

func TestExecutionContextPrecisionDeletionAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, tool, input string
		deny              bool
	}{
		{"canonical", "PowerShell", `{"command":"rm -LiteralPath '.git-commit-msg.txt'"}`, false},
		{"unbound selector", "PowerShell", `{"command":"rm -LiteralPath '.git-commit-msg.txt'","shell":"pwsh"}`, true},
		{"null selector", "PowerShell", `{"command":"rm -LiteralPath '.git-commit-msg.txt'","shell":null}`, true},
		{"namespace", "mcp__host__PowerShell", `{"command":"rm -LiteralPath '.git-commit-msg.txt'"}`, true},
		{"opaque predecessor", "PowerShell", `{"command":"opaque-helper; rm -LiteralPath '.git-commit-msg.txt'"}`, true},
		{"case variant", "powershell", `{"command":"rm -LiteralPath '.git-commit-msg.txt'"}`, true},
		{"alias replacement", "PowerShell", `{"command":"Set-Alias rm custom-remove; rm -LiteralPath '.git-commit-msg.txt'"}`, true},
		{"function replacement", "PowerShell", `{"command":"function rm { custom-remove }; rm -LiteralPath '.git-commit-msg.txt'"}`, true},
		{"protected operand", "PowerShell", `{"command":"rm -LiteralPath '.git/config'","shell":null}`, true},
		{"no blanket denial", "PowerShell", `{"command":"Remove-Item -LiteralPath cache","shell":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: tc.tool, input: tc.input})
			if (got.Kind() == "deny") != tc.deny {
				t.Fatalf("kind = %s; want deny=%v", got.Kind(), tc.deny)
			}
		})
	}
}

func TestExecutionContextPrecisionRetainsProtection(t *testing.T) {
	for _, prefix := range []string{"opaque-helper; ", "Set-Alias rg helper; ", "function rg { helper }; "} {
		command := prefix + `rg --no-config '.env' source.txt; rm -LiteralPath '.git/config'`
		got, _, _ := runGo(t, tableCase{tool: "PowerShell", command: command})
		if got.Kind() != "deny" || !strings.Contains(got.Payload(), "destructive_file_ops") {
			t.Fatalf("protected command lost: %s %s", got.Kind(), got.Payload())
		}
	}
}

type semanticRecord struct {
	Outcome   hookspec.Outcome
	Diag      string
	GuardLog  string
	LogExists bool
}

func TestGuardianSemantics(t *testing.T) {
	for _, c := range tableCases(t) {
		t.Run(c.name, func(t *testing.T) {
			got := recordCase(t, c)
			assertCaseSemantics(t, c, got)
		})
	}
}

func TestDestructivePipelineContinuesAfterAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name, command, category string
	}{
		{"targeted rm then broad deletion", "rm -rf node_modules; rm --force --recursive /", "destructive_file_ops"},
		{"targeted find then world write", "find build/cache -delete; chmod 1777 build", "permission_abuse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _, _ := runGo(t, tableCase{command: tc.command})
			outcome := raw
			if outcome.Kind() != "deny" {
				t.Fatalf("outcome = %s, want deny", raw)
			}
			reason := outcome.Payload()
			if !strings.Contains(reason, tc.category) {
				t.Fatalf("reason = %q, want %s", reason, tc.category)
			}
		})
	}
}

// TestQuotedSubstitutionHeredocBlocksAsTheUnquotedSpelling: a heredoc body an
// interpreter consumes through a quoted substitution blocks exactly as the
// unquoted spelling does — on the uninspectable wrapper's parse limit — and
// never as an unterminated quoted word.
func TestQuotedSubstitutionHeredocBlocksAsTheUnquotedSpelling(t *testing.T) {
	quoted, quotedDiag, _ := runGo(t, tableCase{command: "bash -c \"$( cat <<EOF\nrm -rf /\nEOF\n)\""})
	unquoted, _, _ := runGo(t, tableCase{command: "bash -c $( cat <<EOF\nrm -rf /\nEOF\n)"})
	if quoted.Kind() != "deny" || unquoted.Kind() != "deny" {
		t.Fatalf("kinds = %s / %s, want deny / deny (diag %q)", quoted.Kind(), unquoted.Kind(), quotedDiag)
	}
	if !strings.Contains(quoted.Payload(), "command_parse_limit") {
		t.Fatalf("quoted reason %q, want command_parse_limit", quoted.Payload())
	}
	if !strings.Contains(unquoted.Payload(), "command_parse_limit") {
		t.Fatalf("unquoted reason %q, want command_parse_limit", unquoted.Payload())
	}
	for _, spelling := range []string{"unterminated quoted word", "could not inspect"} {
		if strings.Contains(quoted.Payload(), spelling) || strings.Contains(quotedDiag, spelling) {
			t.Fatalf("still reported as a parse failure (%q):\npayload %q\ndiag %s", spelling, quoted.Payload(), quotedDiag)
		}
	}
}

func TestStructuredShellSuggestions(t *testing.T) {
	cases := []struct {
		name       string
		command    string
		suggestion string
	}{
		{"pull request creation", "gh pr create --title live", "Add --draft (or -d). If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."},
		{"pull request promotion", "gh pr ready 42", "Do not mark the pull request ready for review. Tell the user that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."},
		{"stack submission", "gh stack submit", "Add --auto without --open to create draft pull requests. If the user wants the pull requests ready for review, tell them that agents cannot publish them as ready and ask them to mark each one ready in the pull request UI or with 'gh pr ready'."},
		{"stack link promotion", "gh stack link --open", "Drop --open so the linked pull requests remain drafts. If the user wants them ready for review, tell them that agents cannot publish them as ready and ask them to mark each one ready in the pull request UI or with 'gh pr ready'."},
		{"npm publication", "npm publish", "Use 'npm publish --dry-run' to inspect the package contents without publishing. If the user still wants to publish the package, tell the user that agents cannot publish packages and ask them to run 'npm publish' in their own terminal."},
		{"whole-tree clean", "git clean -fd", "Scope it with an explicit pathspec — 'git clean -f <path>' is allowed; preview with 'git clean -n <path>' first. Whole-tree cleans and -x/-X (ignored files) stay blocked."},
		{"reset away from HEAD", "git reset --hard HEAD~1", "Use 'git restore <path>' (or 'git restore .') to discard working-tree changes, 'git stash' to keep them; 'git reset --hard HEAD' (exactly HEAD) is allowed."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, _, _ := runGo(t, tableCase{command: c.command})
			if got := suggestionFromReason(t, raw); got != c.suggestion {
				t.Fatalf("suggestion = %q, want %q", got, c.suggestion)
			}
		})
	}
}

func assertCaseSemantics(t *testing.T, c tableCase, got semanticRecord) {
	t.Helper()
	outcome := got.Outcome
	kind := outcome.Kind()
	if kind != c.want {
		t.Fatalf("expected %s, got %s (outcome %s, diag %q)", c.want, kind, got.Outcome, got.Diag)
	}
	message := ""
	if c.want == "deny" {
		message = outcome.Payload()
	} else if c.want == "context" {
		message = outcome.Payload()
	}
	if (c.want == "deny" || c.want == "context") && message == "" {
		t.Fatalf("%s outcome has no semantic reason", c.want)
	}
	if c.wantReason != "" {
		if message != c.wantReason {
			t.Fatalf("expected reason %q, got %q", c.wantReason, message)
		}
	}
	if got.LogExists != c.wantLog() {
		t.Fatalf("guard.log existence = %v, want %v (failures and findings only)", got.LogExists, c.wantLog())
	}
	if c.want == "pass" && got.Diag != "" {
		t.Fatalf("pass outcome wrote diagnostics: %q", got.Diag)
	}
	if (c.want == "deny" || c.want == "context") && got.Diag == "" {
		t.Fatalf("threat outcome wrote no diagnostic table")
	}
	if c.want == "deny" || c.want == "context" {
		var log guardLogRecord
		if err := json.Unmarshal([]byte(got.GuardLog), &log); err != nil {
			t.Fatalf("guard.log is not a semantic record: %v", err)
		}
		if log.ThreatCount == 0 || log.ThreatCount != len(log.Threats) {
			t.Fatalf("guard.log threat count = %d for %d threats", log.ThreatCount, len(log.Threats))
		}
		// The threat summary follows the unusable-mode warning, which the
		// exact reason above already pins.
		message = strings.TrimPrefix(message, unknownModeWarning+"\n\n")
		reasonThreats := log.Threats
		countLabel := "threat(s)"
		if c.want == "deny" {
			reasonThreats = nil
			countLabel = "blocking threat(s)"
			for _, threat := range log.Threats {
				if threat.Severity != severityInfo {
					reasonThreats = append(reasonThreats, threat)
				}
			}
		}
		wantPrefix := fmt.Sprintf("%d %s detected: ", len(reasonThreats), countLabel)
		if !strings.HasPrefix(message, wantPrefix) {
			t.Fatalf("reason %q does not report %q", message, wantPrefix)
		}
		if len(reasonThreats) == 0 || !strings.Contains(message, reasonThreats[0].Category) {
			t.Fatalf("reason %q does not identify the leading threat category", message)
		}
		for _, threat := range log.Threats {
			if threat.Category == "" || !strings.Contains(got.Diag, threat.Category) {
				t.Fatalf("diagnostics do not identify logged threat category %q", threat.Category)
			}
		}
	}
}

func recordCase(t *testing.T, c tableCase) semanticRecord {
	t.Helper()
	outcome, diag, logRoot := runGo(t, c)
	record := ""
	logExists := false
	data, err := os.ReadFile(guardLogPath(logRoot))
	switch {
	case err == nil:
		logExists = true
		record = timestampField.ReplaceAllString(string(data), `"timestamp":""`)
	case !os.IsNotExist(err):
		t.Fatalf("reading guard.log: %v", err)
	}
	return semanticRecord{Outcome: outcome, Diag: diag, GuardLog: record, LogExists: logExists}
}

// timestampField blanks the only nondeterministic byte range of a guard.log
// record so a recording can be compared whole.
var timestampField = regexp.MustCompile(`"timestamp":"[^"]*"`)

// ---------------------------------------------------------------------------
// Read-half regression pins
// ---------------------------------------------------------------------------

// hugeIntegerLiteral is a 401-digit integer. The contract preserves integer
// literals of any length, so it survives into the canonical document; a plain
// encoding/json decode of that document then fails, because the literal does
// not fit a float64.
func hugeIntegerLiteral() string { return "1" + strings.Repeat("0", 400) }

// outOfRangeIntegerEvent is a canonical event whose tool.input carries the
// 401-digit literal alongside an otherwise harmless command.
func outOfRangeIntegerEvent(t *testing.T) hookspec.Event {
	return buildEvent(t, "Bash", `{"command":"printf safe","n":`+hugeIntegerLiteral()+`}`, "")
}

// TestOutOfRangeToolInputInteger pins which of the two read-stage messages the
// guard reports for an event the contract accepts but the tool.input decode
// rejects. The event validates, so it is the extract message. The guard denies
// input it cannot read because an error outcome fails open on Copilot and
// Codex. The tool.input decode stays separate from event validation and on
// plain encoding/json semantics; UseNumber here would make this event succeed.
func TestOutOfRangeToolInputInteger(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	raw := Run(hookspec.Request{
		Event: outOfRangeIntegerEvent(t),
	})
	outcome := raw
	if outcome.Kind() != "deny" ||
		outcome.Payload() != "command-guardian could not extract normalized input" {
		t.Fatalf("expected the extract-class deny, got %s", raw)
	}
}

// TestInputRawIsNotReEncoded pins trap 1 at the core level.
//
// The guardian scans tool.input AS TEXT. That text must be the contract
// package own bytes, which keep & < > raw because contract encodes with
// SetEscapeHTML(false). The allowlist is a plain substring match over the
// same text, so it hits only while the ampersand stays raw. Re-encode
// tool.input anywhere in the read path and the allowlist misses, the
// destruction rule fires, and this case flips from pass to deny.
//
// It lives outside tableCases on purpose so this focused regression test can
// state the raw-text invariant directly instead of duplicating the case in the
// broader handler corpus.
func TestInputRawIsNotReEncoded(t *testing.T) {
	raw, _, _ := runGo(t, tableCase{
		name:  "input_raw_is_not_re_encoded",
		tool:  "mcp__db__query",
		input: `{"query":"DROP DATABASE production","note":"keep a&b"}`,
		env:   map[string]string{"COMMAND_GUARD_ALLOWLIST": "keep a&b"},
	})
	if kind := raw.Kind(); kind != "pass" {
		t.Fatalf("the allowlist must match the unescaped ampersand in tool.input; got kind=%v\n%s", kind, raw)
	}
}

// An absent event passes; a constructed event with a NUL fails at handler access.
func TestEmptyAndNULInput(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	out := Run(hookspec.Request{})
	if out.Kind() != "pass" {
		t.Fatalf("absent event: %s", out)
	}
	event := buildEvent(t, "Bash", commandInput(t, "printf \x00safe"), "")
	out = Run(hookspec.Request{Event: event})
	if out.Kind() != "deny" || out.Payload() != "command-guardian could not validate normalized input" {
		t.Fatalf("NUL event: %s", out)
	}
}

// TestSeverityNamesTheReasonEndToEnd fires two threats in one command: an
// uninspectable segment (command_parse_limit, high) precedes a critical
// destructive_file_ops rule. The deny reason must name the critical threat,
// not the parse-limit threat rule order reported first.
func TestSeverityNamesTheReasonEndToEnd(t *testing.T) {
	out, _, _ := runGo(t, tableCase{
		name:    "parse_limit_then_critical_destructive",
		command: "sudo -X true; rm -rf /",
		want:    "deny",
	})
	if out.Kind() != "deny" {
		t.Fatalf("kind = %s, want deny\n%s", out.Kind(), out)
	}
	if !strings.Contains(out.Payload(), "destructive_file_ops") {
		t.Fatalf("reason does not name destructive_file_ops: %s", out.Payload())
	}
	if strings.Contains(out.Payload(), "command_parse_limit") {
		t.Fatalf("reason names the lower-severity command_parse_limit ahead of the critical threat: %s", out.Payload())
	}
}

// TestTieBrokenCriticalThreatsNameTheFirstByRuleOrder fires two distinct
// critical-tier threats in one command: a broad `rm -rf /` (destructive_file_ops,
// reported by checkDestructiveCommands) and `DROP TABLE users;`
// (database_destruction, reported later). mostSevere breaks a tie between
// equal severities by rule order, so the reason must name the
// destructive_file_ops threat, not database_destruction.
func TestTieBrokenCriticalThreatsNameTheFirstByRuleOrder(t *testing.T) {
	out, _, _ := runGo(t, tableCase{
		name:    "two_criticals_tie_break_by_rule_order",
		command: "rm -rf / && DROP TABLE users;",
		want:    "deny",
	})
	if out.Kind() != "deny" {
		t.Fatalf("kind = %s, want deny\n%s", out.Kind(), out)
	}
	if !strings.Contains(out.Payload(), "destructive_file_ops") {
		t.Fatalf("reason does not name destructive_file_ops: %s", out.Payload())
	}
	if strings.Contains(out.Payload(), "database_destruction") {
		t.Fatalf("reason names the later-reported database_destruction threat ahead of the earlier destructive_file_ops one: %s", out.Payload())
	}
}

// TestMergeAndReleaseCommandsPassWhileDraftAndPublishRulesStay pins the
// boundary of the removed merge and release-publication rules: those commands
// and the MCP merge call are no longer gated, and the neighbouring draft pull
// request, package publication and repository deletion rules still deny.
func TestMergeAndReleaseCommandsPassWhileDraftAndPublishRulesStay(t *testing.T) {
	for _, c := range []tableCase{
		{name: "pr merge", command: "gh pr merge 1"},
		{name: "pr merge auto", command: "gh pr merge 1 --auto --squash"},
		{name: "stack merge", command: "gh stack merge"},
		{name: "release create", command: "gh release create v1"},
		{name: "release publish", command: "gh release edit v1 --draft=false"},
		{name: "mcp merge", tool: "mcp__github__merge_pull_request", input: `{"owner":"o","repo":"r","pullNumber":1}`},
	} {
		t.Run("passes/"+c.name, func(t *testing.T) {
			out, _, _ := runGo(t, c)
			if out.Kind() != "pass" {
				t.Fatalf("kind = %s, want pass: %s", out.Kind(), out.Payload())
			}
		})
	}
	for _, c := range []tableCase{
		{name: "pr create", command: "gh pr create --title x"},
		{name: "pr ready", command: "gh pr ready 1"},
		{name: "stack submit", command: "gh stack submit"},
		{name: "npm publish", command: "npm publish"},
		{name: "mcp delete repository", tool: "mcp__github__delete_repository", input: `{"owner":"o","repo":"r"}`},
	} {
		t.Run("denies/"+c.name, func(t *testing.T) {
			out, _, _ := runGo(t, c)
			if out.Kind() != "deny" {
				t.Fatalf("kind = %s, want deny: %s", out.Kind(), out.Payload())
			}
		})
	}
}

// mustEncodeCompact encodes v as the compact JSON the guard scans.
func mustEncodeCompact(t *testing.T, v any) string {
	t.Helper()
	out, err := encodeCompact(v)
	if err != nil {
		t.Fatalf("encoding %#v: %v", v, err)
	}
	return out
}
