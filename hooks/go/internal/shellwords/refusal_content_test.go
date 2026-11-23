package shellwords

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// contentMarker stands for argument content. A refusal that renders it has leaked what the command or script said.
const contentMarker = "leakmarkerS8"

// contentRow is a script built so that one refused child carries a given kind, with the marker in the words, strings,
// and operands around it and never in a path.
type contentRow struct {
	name      string
	command   string
	files     fakeReader
	unproven  bool
	rule      Rule
	construct construct
}

// contentNested runs body as a script called from the tool command, so each refusal it holds is a child of that script.
func contentNested(name, body string, rule Rule, c construct, files fakeReader) contentRow {
	all := fakeReader{"/ws/outer.sh": {Text: body}}
	for path, read := range files {
		all[path] = read
	}
	return contentRow{name: name, command: "bash /ws/outer.sh", files: all, rule: rule, construct: c}
}

// contentLimit is a nested call whose file the reader refuses with the given limit and a detail that holds the marker.
func contentLimit(r Rule) contentRow {
	body := "echo " + contentMarker + " \"" + contentMarker + "\"\nbash /ws/limit.sh\n"
	return contentNested("limit "+r.String(), body, r, constructNone, fakeReader{"/ws/limit.sh": {Limit: r, Detail: contentMarker}})
}

// contentConstructRows is the script text of each construct, with the marker in its words, strings, or operands.
func contentConstructRows() []contentRow {
	const m = contentMarker
	body := func(text string, c construct) contentRow {
		return contentNested("construct "+c.String(), text, RuleOpaque, c, nil)
	}
	return []contentRow{
		body("eval \"$"+m+"\" "+m+"\n", constructEval),
		body("trap 'echo "+m+"' EXIT\n", constructTrap),
		body("source lib "+m+"\n", constructSource),
		body("./report.cmd "+m+"\n", constructBatchFile),
		contentNested("construct dotnet-call", "pwsh -File /ws/report.ps1", RuleOpaque, constructDotNetCall, fakeReader{"/ws/report.ps1": {Text: "[Fixture.Report]::Describe('" + m + "')\n"}}),
		contentNested("construct dynamic-call", "pwsh -File /ws/report.ps1", RuleOpaque, constructDynamicCall, fakeReader{"/ws/report.ps1": {Text: "Import-Module ./" + m + ".psd1\n"}}),
		body("zsh -c 'echo "+m+"' "+m+"\n", constructOtherShell),
		body("$"+m+" "+m+"\n", constructCommandPositionExpansion),
		body("bash -c \"$"+m+"\" "+m+"\n", constructDynamicC),
		body("echo "+m+" | bash\n", constructStdinToShell),
		body("sudo nice nohup env time "+m+"\n", constructNestingDepth),
		body("sudo --"+m+" x\n", constructUnsupportedSyntax),
		body(m+"=/x\nPATH=/x\n", constructPinnedAssignment),
		body("echo "+m+"\nIFS=,\n", constructPersistentIFS),
		body("IFS=, echo "+m+"\n", constructIFSPrefix),
		body("declare -n r="+m+"\n", constructNameref),
		body("alias "+m+"=ls\n", constructAlias),
		body("coproc "+m+" { cat; }\n", constructCoproc),
		body("printf -v PATH "+m+"\n", constructPrintfV),
		body("git() { echo "+m+"; }\n", constructFunctionShadow),
		body("command_not_found_handle() { echo "+m+"; }\n", constructCommandNotFoundHandle),
		body("PROMPT_COMMAND="+m+"\n", constructPromptCommand),
		body("echo "+m+" $(( a[$(id "+m+")] ))\n", constructArithmeticSubstitution),
		body("cat <<E\"O\"F\n"+m+"\nEOF\n", constructHeredocDelimiter),
	}
}

func contentRuleRows() []contentRow {
	const m = contentMarker
	ok := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	prefix := "echo " + m + " \"" + m + "\"\n"
	nested := func(r Rule, line string, files fakeReader) contentRow {
		return contentNested("rule "+r.String(), prefix+line+"\n", r, constructNone, files)
	}
	// A script that the typed call feeds through a pipe passes that stream to the script it calls, so the nested call is the
	// one refused.
	stdinFed := nested(RuleStdinFed, "bash /ws/p.sh", fakeReader{"/ws/p.sh": {Text: "read x\n"}})
	stdinFed.command = "echo " + m + " | bash /ws/outer.sh"
	return []contentRow{
		contentLimit(RuleUnreadable),
		contentLimit(RuleEncoding),
		contentLimit(RuleChanging),
		contentLimit(RuleTooLarge),
		contentLimit(RuleTooManyFiles),
		contentLimit(RulePathProtected),
		contentLimit(RuleBudget),
		nested(RuleDepth, "sudo nice nohup bash /ws/ok.sh "+m, ok),
		nested(RuleCycle, "bash /ws/outer.sh "+m, nil),
		nested(RuleFormUnsupportedOption, "bash -x /ws/ok.sh "+m, ok),
		nested(RuleExtension, "bash /ws/notes.txt "+m, fakeReader{"/ws/notes.txt": {Text: m}}),
		nested(RulePathNotLiteral, "bash $HOME/ok.sh "+m, ok),
		nested(RuleCwdUnproven, "cd \"$"+m+"\" && bash ./ok.sh", ok),
		nested(RuleOperandNotLiteral, "bash /ws/inner.sh", fakeReader{"/ws/inner.sh": {Text: "echo " + m + "\nrm -rf \"$" + m + "\"\n"}}),
		nested(RuleCallerArgument, "bash /ws/p.sh \"$"+m+"\" "+m, fakeReader{"/ws/p.sh": {Text: "echo \"$1\" " + m + "\n"}}),
		stdinFed,
		nested(RuleWritesInspected, "{ echo "+m+"; bash /ws/ok.sh; } >> /ws/ok.sh", ok),

		// A typed-form refusal on a script line is relabeled, so the wrapper's option word never reaches the text.
		contentNested("typed form in a script", "env --"+m+" x\n", RuleOpaque, constructUnsupportedSyntax, nil),
		contentNested("typed command string in a script", "bash -c \"$"+m+"\"\n", RuleOpaque, constructDynamicC, nil),
		contentNested("typed wrapper depth in a script", "sudo nice nohup env time "+m+"\n", RuleOpaque, constructNestingDepth, nil),
	}
}

func parseContentRow(row contentRow) Result {
	if row.unproven {
		return ParseScripts(context.Background(), row.command, POSIX, ScriptOptions{Reader: row.files, Cwd: "/ws"})
	}
	return parseScript(row.command, row.files)
}

// TestScriptRefusalsAreContentFree parses one script per construct and per named rule that a script child can reach,
// with the marker in its words, strings, and operands. A refused child must carry a named rule, report itself content
// free, and render neither the marker in its text nor in its remedy.
func TestScriptRefusalsAreContentFree(t *testing.T) {
	t.Run("top level", checkTopLevelScriptRefusals)

	rows := append(contentConstructRows(), contentRuleRows()...)
	seenRules := map[Rule]bool{}
	seenConstructs := map[construct]bool{}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			res := parseContentRow(row)
			produced := false
			for i, inv := range res.Invocations {
				if inv.Script == nil || inv.Inspectable {
					continue
				}
				r := inv.Reason
				where := "invocation " + strconv.Itoa(i) + " (" + r.String() + ")"
				if r.Rule() == RuleNone {
					t.Errorf("%s: refused script child carries no named rule", where)
					continue
				}
				if !r.ContentFree() {
					t.Errorf("%s: ContentFree() = false", where)
				}
				if strings.Contains(r.String(), contentMarker) {
					t.Errorf("%s: String() holds the marker", where)
				}
				if remedy, ok := r.Remedy(); !ok || strings.Contains(remedy, contentMarker) {
					t.Errorf("%s: Remedy() = %q, %v; want a remedy without the marker", where, remedy, ok)
				}
				seenRules[r.Rule()] = true
				if r.Rule() == RuleOpaque {
					seenConstructs[r.construct] = true
				}
				if r.Rule() == row.rule && (row.construct == constructNone || r.construct == row.construct) {
					produced = true
				}
			}
			if !produced {
				t.Errorf("no refused child carries %s %s: %+v", row.rule, row.construct, res.Invocations)
			}
		})
	}

	// These rules judge the tool call itself: its execution context, and its typed prefix and predecessors. A script's
	// own commands never reach them, so a script child cannot carry one. The "top level" subtest covers them.
	topLevelOnly := map[Rule]bool{
		RuleContextUnqualified:  true,
		RuleEnvPinned:           true,
		RulePredecessorMayWrite: true,
		RulePredecessorState:    true,
	}
	for r := ruleFirst; r <= ruleLast; r++ {
		if !seenRules[r] && !topLevelOnly[r] {
			t.Errorf("rule %s is never seen on a refused script child", r)
		}
	}
	for c := constructFirst; c <= constructLast; c++ {
		if !seenConstructs[c] {
			t.Errorf("construct %s is never seen on a refused script child", c)
		}
	}
}

// checkTopLevelScriptRefusals covers the rules reached on the tool call itself, where the only text the
// refusal may take from the command is the script path it renders.
func checkTopLevelScriptRefusals(t *testing.T) {
	const m = contentMarker
	ok := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	rows := []struct {
		name    string
		command string
		files   fakeReader
		proven  bool
		rule    Rule
		path    string
	}{
		{"context unqualified", "bash /ws/ok.sh " + m + " \"$" + m + "\"", ok, false, RuleContextUnqualified, ""},
		{"env pinned", "PATH=" + m + " bash /ws/ok.sh " + m, ok, true, RuleEnvPinned, ""},
		{"predecessor may write", "echo " + m + " > " + m + "file; bash /ws/ok.sh", ok, true, RulePredecessorMayWrite, ""},
		{"predecessor state", "cd sub; cd ..; cd \"$" + m + "\"; bash /ws/ok.sh", ok, true, RulePredecessorState, ""},
		{"unreadable under a marked directory", "bash ./" + m + "dir/x.sh " + m, fakeReader{}, true, RuleUnreadable, "/ws/" + m + "dir/x.sh"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			opts := ScriptOptions{Reader: row.files, Cwd: "/ws"}
			if row.proven {
				opts.Context = provenPOSIX
			}
			res := ParseScripts(context.Background(), row.command, POSIX, opts)
			produced := false
			for _, inv := range res.Invocations {
				r := inv.Reason
				if inv.Inspectable || r.Rule() != row.rule {
					continue
				}
				produced = true
				if !r.ContentFree() {
					t.Errorf("%q: ContentFree() = false", r.String())
				}
				rest := r.String()
				remedy, ok := r.Remedy()
				if !ok {
					t.Errorf("%q: no remedy", r.String())
				}
				if row.path != "" {
					rest = strings.ReplaceAll(rest, row.path, "")
					remedy = strings.ReplaceAll(remedy, row.path, "")
				}
				if strings.Contains(rest, m) || strings.Contains(remedy, m) {
					t.Errorf("%q: the marker appears outside the rendered path", r.String())
				}
			}
			if !produced {
				t.Errorf("%q: no refusal carries %s: %+v", row.command, row.rule, res.Invocations)
			}
		})
	}
}

// TestTypedFormRefusalsReportContent parses typed commands with the marker in every position the kind does not quote.
// Each refusal is a form kind that reports ContentFree() false, so the flag is truthful about the three kinds that quote
// a whole dash word, and no kind renders a marker it does not quote.
func TestTypedFormRefusalsReportContent(t *testing.T) {
	const m = contentMarker
	option := func(exe, word string) string {
		return exe + " option " + word + " is outside the literal wrapper contract"
	}
	rows := []struct {
		name    string
		kind    formKind
		dialect Dialect
		command string
		want    string
		quoted  bool // the text holds the marker, inside a whole dash word
	}{
		{"shell contract", formShellContract, POSIX, `bash -x ` + m, "bash requires exactly -c or -lc and a literal command string", false},
		{"PowerShell contract", formPowerShellContract, PowerShell, `pwsh -File ` + m, "pwsh requires optional PowerShell flags, then -File <literal .ps1> or -Command and one literal token", false},
		{"env name", formEnvName, POSIX, `env -u "$` + m + `" ` + m, "env -u requires a literal name", false},
		{"wrapper option", formWrapperOption, POSIX, `sudo --bogus ` + m + ` ` + m, option("sudo", "--bogus"), false},
		{"wrapper value", formWrapperValue, POSIX, `sudo -u "$` + m + `" ` + m, "sudo -u requires a literal value", false},
		{"no child", formNoChild, POSIX, `flock "$` + m + `" ` + m, "flock has no child executable", false},
		{"xargs child", formXargsChild, POSIX, `xargs -I ` + m + ` -n ` + m, "xargs has no literal child executable", false},
		{"timeout duration", formTimeoutDuration, POSIX, `timeout ` + m + ` apply ` + m, "timeout requires a literal duration before the command", false},
		{"timeout option", formTimeoutOption, POSIX, `timeout 10 -s ` + m + ` ` + m, "timeout option -s after the duration is outside the literal wrapper contract", false},
		{"flock option", formFlockOption, POSIX, `flock /tmp/l -c ` + m, "flock option -c after the lock file is outside the literal wrapper contract", false},
		{"child not literal", formChildNotLiteral, POSIX, `nohup "$` + m + `" ` + m, "nohup child executable is not literal", false},
		{"command string", formCommandString, POSIX, `bash -c "$` + m + `" ` + m, "bash command string is not literal", false},
		{"wrapper depth", formWrapperDepth, POSIX, `sudo nice nohup env time ` + m, "wrapper depth exceeds 4 at time", false},
		{"unterminated", formUnterminated, POSIX, `echo ` + m + ` '` + m, "unterminated quoted word", false},
		{"redirection", formRedirection, POSIX, `echo ` + m + ` >`, "redirection > has no target", false},
		{"ANSI control", formANSIControl, POSIX, `$'` + m + `\cK' ` + m, "ANSI-C control escape is outside the literal parser contract", false},
		{"ANSI byte", formANSIByte, POSIX, `$'` + m + `\0' ` + m, "ANSI-C non-ASCII or NUL escape is outside the literal parser contract", false},
		{"ANSI unterminated", formANSIUnterminated, POSIX, `$'` + m + `\'`, "unterminated ANSI-C quoted word", false},

		// The three whole-word kinds quote the dash word as written, an attached value and the source of a substitution
		// included. That is why they report content.
		{"wrapper option attached value", formWrapperOption, POSIX, `sudo -u` + m + ` x`, option("sudo", "-u"+m), true},
		{"wrapper option substitution", formWrapperOption, POSIX, `sudo "-$(` + m + `)" x`, option("sudo", "-$("+m+")"), true},
		{"timeout option attached value", formTimeoutOption, POSIX, `timeout 10 -s` + m + ` x`, "timeout option -s" + m + " after the duration is outside the literal wrapper contract", true},
		{"timeout option substitution", formTimeoutOption, POSIX, `timeout 10 "-$(` + m + `)" x`, "timeout option -$(" + m + ") after the duration is outside the literal wrapper contract", true},
		{"flock option attached value", formFlockOption, POSIX, `flock /tmp/l -c` + m + ` x`, "flock option -c" + m + " after the lock file is outside the literal wrapper contract", true},
		{"flock option substitution", formFlockOption, POSIX, `flock /tmp/l "-$(` + m + `)" x`, "flock option -$(" + m + ") after the lock file is outside the literal wrapper contract", true},
	}
	seen := map[formKind]bool{}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var refused []Invocation
			for _, inv := range Parse(row.command, row.dialect).Invocations {
				if !inv.Inspectable {
					refused = append(refused, inv)
				}
			}
			if len(refused) == 0 {
				t.Fatalf("%q: no refused invocation", row.command)
			}
			r := refused[0].Reason
			if r.form != row.kind {
				t.Fatalf("%q: form kind = %q, want %q (reason %q)", row.command, r.form, row.kind, r.String())
			}
			seen[r.form] = true
			if r.ContentFree() {
				t.Errorf("%q: ContentFree() = true for a form kind", r.String())
			}
			if remedy, ok := r.Remedy(); ok {
				t.Errorf("%q: a form kind has no remedy, got %q", r.String(), remedy)
			}
			if got := r.String(); got != row.want {
				t.Errorf("%q: String() = %q, want %q", row.command, got, row.want)
			}
			if got := strings.Contains(r.String(), m); got != row.quoted {
				t.Errorf("%q: text holds the marker = %v, want %v: %q", row.command, got, row.quoted, r.String())
			}
		})
	}
	for k := formFirst; k <= formLast; k++ {
		if !seen[k] {
			t.Errorf("form kind %q is never seen", k)
		}
	}
}
