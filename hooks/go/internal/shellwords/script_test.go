package shellwords

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

type fakeReader map[string]ScriptRead

func (f fakeReader) ReadScript(path string) ScriptRead {
	if r, ok := f[path]; ok {
		r.Path = path
		return r
	}
	return ScriptRead{Path: path, Limit: RuleUnreadable, NotExist: true, Detail: "no such fixture"}
}

func (f fakeReader) ProbeExists(path string) (bool, Rule) {
	read, ok := f[path]
	if ok && read.Limit == RulePathProtected {
		return false, read.Limit
	}
	return ok && !read.NotExist, RuleNone
}

var provenPOSIX = ExecutionContext{Dialect: POSIX, Proven: true}

func parseScript(command string, files fakeReader) Result {
	return ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
}

// parseScriptOn is parseScript with the path rules of host.
func parseScriptOn(host pathshape.Host, command string, files fakeReader) Result {
	return ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws", Host: host})
}

func TestTypedBashDataDoesNotBecomeExecutable(t *testing.T) {
	for _, command := range []string{
		`line=${line#ref: refs/heads/}; printf '%s' "$line"`,
		`paths+=(":(top,literal)$path"); git status -- "${paths[@]}"`,
		`urls+=("$url") hosts+=("$host"); printf '%s' "$url"`,
		`fulls[$index]=$value; printf '%s' "$value"`,
		`if [[ "$url" != *://* && $url =~ $pattern ]]; then printf yes; fi`,
	} {
		res := parseScript(command, fakeReader{})
		if got := firstScriptReason(res); got != "" {
			t.Errorf("%q: Bash data caused executable refusal %q", command, got)
		}
	}
	for _, command := range []string{
		`entries+=("$(git status)"); printf '%s' "$entry"`,
		`if [[ "$(git status)" == ready && "$mode" == valid ]]; then printf yes; fi`,
		`FIELD=value [[ yes && git status ]]`,
		`if[[ yes && git status ]]`,
		"[[\ryes && git status ]]",
		`[[ ( yes )]] && git status ]]`,
		`line=${line#$(git status)}; printf '%s' "$line"`,
	} {
		res := parseScript(command, fakeReader{})
		if !slices.ContainsFunc(res.Invocations, func(inv Invocation) bool {
			return len(inv.Words) > 0 && inv.Words[0].Text == "git"
		}) {
			t.Errorf("%q hid the Git invocation: %+v", command, res)
		}
	}
	res := parseScript(`entries[$index]=value; bash ./ok.sh`, fakeReader{"/ws/ok.sh": {Text: "echo report\n"}})
	if got := firstScriptReason(res); got != "script-predecessor-state" || len(res.Scripts) != 0 {
		t.Errorf("dynamic arithmetic index must not prove a safe predecessor, got %+v", res)
	}
}

func TestPowerShellLoopLabelsAreData(t *testing.T) {
	res := parsePowerShellCall(t, `:search foreach ($item in @('one')) { break search }; Write-Output done`, fakeReader{})
	if got := firstScriptReason(res); got != "" || len(res.Invocations) != 1 || !strings.EqualFold(res.Invocations[0].Words[0].Text, "Write-Output") {
		t.Fatalf("expected a literal break label and one executable, got %+v", res)
	}
}

func TestBashDataProofLeavesUnknownExpansionsUnresolved(t *testing.T) {
	if provenGroupData('=', `"${ value }"`, "") {
		t.Error("array proof accepted an unrecognized braced expansion")
	}
	for _, source := range []string{`${ variable }`, `${name:-${nested`} {
		if got := plainParameterWordEnd(source, 0); got >= 0 {
			t.Errorf("%q: unsupported parameter span was accepted through byte %d", source, got)
		}
	}
	if got := plainBashConditionEnd("[[ outer\n[[ inner", 0); got >= 0 {
		t.Errorf("nested openers were accepted as one condition through byte %d", got)
	}
}

func parsePowerShellCall(t *testing.T, command string, files fakeReader) Result {
	t.Helper()
	return ParseScripts(t.Context(), command, PowerShell, ScriptOptions{
		Reader: files, Context: provenPowerShell, Cwd: "/ws", Host: pathshape.For("linux"),
	})
}

func TestPowerShellScriptCallPreservesCallerArguments(t *testing.T) {
	const path = "/ws/report.ps1"
	files := fakeReader{path: {ID: "report", Text: "param($label, $pattern)\nWrite-Output $label\n"}}
	for _, tc := range []struct {
		command string
		dialect Dialect
		pattern bool
	}{
		{"powershell -NoProfile -ExecutionPolicy RemoteSigned -File ./report.ps1 'two words' 'src/*.txt'", POSIX, false},
		{"pwsh -noni -ep Default -File ./report.ps1 'two words' src/*.txt", POSIX, true},
		{"./report.ps1 'two words' 'src/*.txt'", PowerShell, false},
		{"& ./report.ps1 'two words' src/*.txt", PowerShell, true},
		{". ./report.ps1 'two words' 'src/*.txt'", PowerShell, false},
	} {
		t.Run(tc.command, func(t *testing.T) {
			ctx := ExecutionContext{Dialect: tc.dialect, Proven: true}
			res := ParseScripts(t.Context(), tc.command, tc.dialect, ScriptOptions{
				Reader: files, Context: ctx, Cwd: "/ws", Host: pathshape.For("linux"),
			})
			if reason := firstScriptReason(res); reason != "" || len(res.ScriptCalls) != 1 {
				t.Fatalf("reason %q, script calls %+v", reason, res.ScriptCalls)
			}
			call := res.ScriptCalls[0]
			if call.Path != path || !call.ConsumesParams || call.UnquotedParams || call.PatternArgs != tc.pattern ||
				!slices.Equal(call.Args, []string{"two words", "src/*.txt"}) {
				t.Errorf("got script call %+v, want the two arguments intact and pattern=%t", call, tc.pattern)
			}
			child := scriptChild(res)
			if child == nil || child.Dialect != PowerShell || child.Script.Path != path || child.Script.Line != 2 ||
				len(child.Words) != 2 || !strings.EqualFold(child.Words[0].Text, "Write-Output") {
				t.Errorf("want Write-Output from %s:2 in the PowerShell dialect, got %+v", path, child)
			}
		})
	}
}

func TestScriptCallCrossLanguageOrigins(t *testing.T) {
	const outer, middle, leaf = "/ws/outer.sh", "/ws/middle.ps1", "/ws/leaf.sh"
	const shebang = "#!/usr/bin/env pwsh"
	files := fakeReader{
		outer:  {ID: "outer", Text: "./middle.ps1\n"},
		middle: {ID: "middle", Header: ScriptHeader{Shebang: shebang}, Text: shebang + "\nWrite-Output middle\nbash ./leaf.sh\n"},
		leaf:   {ID: "leaf", Text: "# report\nprintf done\n"},
	}
	res := parseScriptOn(pathshape.For("linux"), "sh ./outer.sh", files)
	if reason := firstScriptReason(res); reason != "" || len(res.Scripts) != 3 {
		t.Fatalf("reason %q, inspected scripts %+v", reason, res.Scripts)
	}
	for _, tc := range []struct {
		word, path string
		dialect    Dialect
		chain      []string
	}{
		{"Write-Output", middle, PowerShell, []string{outer}},
		{"printf", leaf, POSIX, []string{outer, middle}},
	} {
		found := slices.ContainsFunc(res.Invocations, func(inv Invocation) bool {
			return inv.Script != nil && inv.Script.Path == tc.path && inv.Script.Line == 2 &&
				slices.Equal(inv.Script.Chain, tc.chain) && inv.Dialect == tc.dialect &&
				len(inv.Words) > 0 && strings.EqualFold(inv.Words[0].Text, tc.word)
		})
		if !found {
			t.Errorf("missing %s in %s:2 via %v, got %+v", tc.word, tc.path, tc.chain, res.Invocations)
		}
	}
}

func TestPowerShellScriptCallOrdersPathAssignment(t *testing.T) {
	const call = "./report.ps1"
	const assignment = "$env:PATH = '/opt/tools'"
	files := fakeReader{"/ws/report.ps1": {ID: "report", Text: "Write-Output report\n"}}
	before := parsePowerShellCall(t, assignment+"; "+call, files)
	if got := firstScriptReason(before); (got != "script-env-pinned" && got != "script-predecessor-state") || len(before.Scripts) != 0 {
		t.Errorf("assignment before call: reason %q, inspected scripts %+v", got, before.Scripts)
	}
	after := parsePowerShellCall(t, call+"; "+assignment, files)
	if got := firstScriptReason(after); got != "" || len(after.Scripts) != 1 || after.Scripts[0].Path != "/ws/report.ps1" {
		t.Errorf("assignment after call: reason %q, inspected scripts %+v", got, after.Scripts)
	}
}

func TestPowerShellScriptCallRefusesUnsupportedForms(t *testing.T) {
	files := fakeReader{
		"/ws/report.ps1": {ID: "report", Text: "Write-Output report\n"},
		"/ws/notes.txt":  {ID: "notes", Text: "Write-Output notes\n"},
	}
	for _, tc := range []struct{ command, reason string }{
		{"pwsh -ExecutionPolicy Example -File ./report.ps1", "script-form-unsupported-option"},
		{"pwsh /File ./report.ps1", "script-form-unsupported-option"},
		{"pwsh -f ./report.ps1", "script-form-unsupported-option"},
		{"pwsh ./report.ps1", "script-form-unsupported-option"},
		{"pwsh -File ./notes.txt", "script-extension"},
		{". report.ps1", "script-opaque:source"},
	} {
		res := parsePowerShellCall(t, tc.command, files)
		if got := firstScriptReason(res); got != tc.reason || len(res.Scripts) != 0 {
			t.Errorf("%q: got %q and %d inspected scripts, want %q before inspection", tc.command, got, len(res.Scripts), tc.reason)
		}
	}
}

func TestDirectScriptClassification(t *testing.T) {
	const note = "Write-Output report\n"
	const py = "#!/usr/bin/env python3"
	files := fakeReader{
		"/ws/report.ps1": {ID: "report", Text: note},
		"/ws/Microsoft.PowerShell.Utility/Write-Output.ps1": {ID: "local", Text: note},
		"/ws/tool":        {Limit: RuleTooLarge, TooLarge: true, Header: ScriptHeader{BinaryMagic: true}},
		"/ws/link":        {Limit: RuleUnreadable, IsSymlink: true, HasResolvedHeader: true, ResolvedHeader: ScriptHeader{Shebang: py}},
		"/ws/script-link": {Limit: RuleUnreadable, IsSymlink: true, HasResolvedHeader: true, ResolvedHeader: ScriptHeader{Shebang: "#!/bin/bash"}},
		"/ws/large.ps1":   {Limit: RuleTooLarge, TooLarge: true, Header: ScriptHeader{Shebang: py}},
		"/ws/bin/agx":     {SHA256: AgxLauncherSHA256, Text: "echo report\n"},
		"/ws/copy/agx":    {Text: "echo report\n"},
		"/ws/env":         {Text: "echo local\n"},
		"/ws/settings":    {Text: "echo ready\n"},
		"/ws/report.sh":   {Text: "echo ready\n"},
	}
	for _, tc := range []struct {
		cmd          string
		dialect      Dialect
		path, reason string
	}{
		{".\\report", PowerShell, "/ws/report.ps1", ""},
		{"Microsoft.PowerShell.Utility\\Write-Output", PowerShell, "/ws/Microsoft.PowerShell.Utility/Write-Output.ps1", ""},
		{"Microsoft.PowerShell.Utility\\Write-Host report", PowerShell, "", ""},
		{"./tool", POSIX, "", ""},
		{"./link", POSIX, "", ""},
		{"./script-link", POSIX, "", "script-unreadable"},
		{"./large.ps1", PowerShell, "", "script-too-large"},
		{"./large.ps1", POSIX, "", ""},
		{"./copy/agx", POSIX, "/ws/copy/agx", ""},
		{"./bin/agx", POSIX, "", ""},
		{"./env echo report", POSIX, "/ws/env", ""},
		{"source ./settings", POSIX, "/ws/settings", ""},
		{"dash ./report.sh", POSIX, "/ws/report.sh", ""},
		{"ksh ./report.sh", PowerShell, "/ws/report.sh", ""},
		{"zsh ./report.sh", POSIX, "", "script-opaque:other-shell"},
		{"pwsh –File ./report.ps1", PowerShell, "", "script-form-unsupported-option"},
		{"pwsh -Command -", PowerShell, "", "script-opaque:stdin-to-shell"},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			res := ParseScripts(t.Context(), tc.cmd, tc.dialect, ScriptOptions{Context: ExecutionContext{Dialect: tc.dialect, Proven: true}, Cwd: "/ws", Host: pathshape.For("linux"), Reader: files})
			if got := firstScriptReason(res); got != tc.reason {
				t.Fatalf("reason %q, want %q", got, tc.reason)
			}
			if tc.path == "" {
				if len(res.Scripts) != 0 {
					t.Errorf("unexpected scripts %+v", res.Scripts)
				}
			} else if len(res.Scripts) != 1 || res.Scripts[0].Path != tc.path {
				t.Errorf("scripts %+v, want %s", res.Scripts, tc.path)
			}
		})
	}
}

func TestDirectNonShellWithoutExpansionAuthority(t *testing.T) {
	files := fakeReader{
		"/ws/utility": {Header: ScriptHeader{BinaryMagic: true}},
		"/ws/wrapper": {Header: ScriptHeader{Shebang: "#!/usr/bin/env python3"}},
		"/ws/body.sh": {Text: "echo report\n"},
		"/ws/bin/env": {Header: ScriptHeader{BinaryMagic: true}},
	}
	for _, tc := range []struct{ command, want, leaf string }{
		{"./utility", "", "utility"},
		{"./wrapper", "", "wrapper"},
		{"./body.sh", "script-context-unqualified", "body.sh"},
		{"./bin/env echo report", "", "echo"},
	} {
		res := ParseScripts(t.Context(), tc.command, POSIX, ScriptOptions{
			Context: ExecutionContext{Dialect: POSIX}, Cwd: "/ws", Reader: files, Host: pathshape.For("linux"),
		})
		if got := firstScriptReason(res); got != tc.want || len(res.Scripts) != 0 || len(res.Invocations) != 1 ||
			ExecutableName(res.Invocations[0].Words[0].Text) != tc.leaf {
			t.Errorf("%q: reason %q, want %q; got %+v", tc.command, got, tc.want, res)
		}
	}
}

func TestPowerShellScriptArgumentsKeepFlagAndPatternSpelling(t *testing.T) {
	res := parsePowerShellCall(t, "./report.ps1 –Label 'src/'*.txt", fakeReader{"/ws/report.ps1": {Text: "Write-Output $args\n"}})
	if got := firstScriptReason(res); got != "" || len(res.ScriptCalls) != 1 {
		t.Fatalf("reason %q, calls %+v", got, res.ScriptCalls)
	}
	call := res.ScriptCalls[0]
	if !call.PatternArgs || !slices.Equal(call.Args, []string{"–Label", "src/*.txt"}) {
		t.Errorf("caller arguments %+v", call)
	}
}

func TestPowerShellDirectScriptCallerArray(t *testing.T) {
	files := fakeReader{"/ws/report.ps1": {Text: "param([string[]]$Names)\nWrite-Output $Names\n"}}
	for _, tc := range []struct {
		command string
		args    []string
	}{
		{`./report.ps1 'one','two'`, []string{"one", "two"}},
		{`& ./report.ps1 'one','two'`, []string{"one", "two"}},
		{`. ./report.ps1 'one','two'`, []string{"one", "two"}},
		{`./report.ps1 -Names:'one','two'`, []string{"-Names", "one", "two"}},
		{`./report.ps1 'one,two'`, []string{"one,two"}},
		{`pwsh -File ./report.ps1 'one','two'`, []string{"one,two"}},
	} {
		res := parsePowerShellCall(t, tc.command, files)
		if reason := firstScriptReason(res); reason != "" || len(res.ScriptCalls) != 1 || !slices.Equal(res.ScriptCalls[0].Args, tc.args) {
			t.Errorf("%q: reason %q, calls %+v, want path arguments %v", tc.command, reason, res.ScriptCalls, tc.args)
		}
	}
}

func TestChildPathAssignmentDoesNotAffectParentScript(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo report\n"}}
	res := parseScriptOn(pathshape.For("linux"), "bash -c 'PATH=tools'; bash ./ok.sh", files)
	if reason := firstScriptReason(res); reason != "" || len(res.Scripts) != 1 || res.Scripts[0].Path != "/ws/ok.sh" {
		t.Fatalf("expected parent script lookup after a child-shell assignment, reason %q, scripts %+v", reason, res.Scripts)
	}
}

func TestScriptCallerSearchPath(t *testing.T) {
	for _, tc := range []struct {
		command string
		dialect Dialect
		want    string
	}{
		{"PATH=.:$PATH report.sh", POSIX, "script-env-pinned"},
		{"PATH=\"/usr/local/bin:$PATH\" npm test", POSIX, ""},
		{"export PATH=./tools; report", POSIX, "script-env-pinned"},
		{"PATH+=:tools report", POSIX, "script-env-pinned"},
		{"PATH+=:tools; report", POSIX, "script-env-pinned"},
		{"PATH+=:/opt/tools; report", POSIX, ""},
		{"builtin export PATH=tools; report", POSIX, "script-env-pinned"},
		{"command unset PATH; report", POSIX, "script-env-pinned"},
		{"command builtin export PATH+=:tools; report", POSIX, "script-env-pinned"},
		{"time export PATH=tools; report", POSIX, "script-env-pinned"},
		{"time command unset PATH; report", POSIX, "script-env-pinned"},
		{"command time export PATH=tools; report", POSIX, ""},
		{"bash -c 'export PATH=tools; report'", POSIX, "script-env-pinned"},
		{"bash -c 'PATH=tools'; report", POSIX, ""},
		{"pwsh -Command '$env:PATH = \"tools\"; Get-Date'", POSIX, "script-env-pinned"},
		{"pwsh -Command '$env:PATH = \"tools\"'; report", POSIX, ""},
		{"command -v export; report", POSIX, ""},
		{"$env:PATH = $env:PATH; Get-Date", PowerShell, ""},
		{"$env:PATH = './tools'; Get-Date", PowerShell, "script-env-pinned"},
		{"Get-Date; $env:PATH = './tools'", PowerShell, ""},
		{"$env:PATH = '/opt/tools'; Get-Date", PowerShell, ""},
	} {
		res := ParseScripts(t.Context(), tc.command, tc.dialect, ScriptOptions{Context: ExecutionContext{Dialect: tc.dialect, Proven: true}, Cwd: "/ws", Reader: fakeReader{}})
		if got := firstScriptReason(res); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestPowerShellRootedPathsUseCurrentDrive(t *testing.T) {
	files := fakeReader{"C:/d/report.ps1": {Text: "Write-Output current\n"}, "D:/report.ps1": {Text: "Write-Output other\n"}}
	for _, tc := range []struct{ cmd, cwd, path, refusal string }{
		{"/d/report.ps1", "C:/work", "C:/d/report.ps1", ""},
		{"\\d\\report.ps1", "C:/work", "C:/d/report.ps1", ""},
		{"Set-Location /d; ./report.ps1", "C:/work", "C:/d/report.ps1", ""},
		{"Set-Location \\d; ./report.ps1", "C:/work", "C:/d/report.ps1", ""},
		{"/d/report.ps1", "", "", "script-cwd-unproven"},
		{"C:report.cmd", "C:/work", "", "script-path-not-literal"},
		{"//files/share/report.fish", "C:/work", "", "script-path-not-literal"},
		{"./report.bat", "", "", "script-cwd-unproven"},
		{"./report.bat", "C:/work", "", "script-opaque:batch-file"},
		{"./report.fish", "C:/work", "", "script-opaque:other-shell"},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			res := ParseScripts(t.Context(), tc.cmd, PowerShell, ScriptOptions{Context: provenPowerShell, Host: pathshape.For("windows"), Cwd: tc.cwd, Reader: files})
			if got := firstScriptReason(res); got != tc.refusal {
				t.Fatalf("got %q, want %q", got, tc.refusal)
			}
			if tc.path == "" && len(res.Scripts) != 0 || tc.path != "" && (len(res.Scripts) != 1 || res.Scripts[0].Path != tc.path) {
				t.Errorf("inspected %+v, want %q", res.Scripts, tc.path)
			}
		})
	}
}

func TestPowerShellSearchPathUsesHostSyntax(t *testing.T) {
	for _, tc := range []struct{ host, assignment, want string }{
		{"windows", "'C:/tools;D:/bin'", ""},
		{"linux", "'/opt/tools:/usr/bin'", ""},
		{"linux", "'/opt/tools:tools'", "script-env-pinned"},
		{"windows", "'C:/tools;tools'", "script-env-pinned"},
		{"linux", "$env:PATH", ""},
		{"linux", "'$env:PATH'", "script-env-pinned"},
	} {
		res := ParseScripts(t.Context(), "$env:PATH = "+tc.assignment+"; Get-Date", PowerShell, ScriptOptions{Context: provenPowerShell, Host: pathshape.For(tc.host), Reader: fakeReader{}})
		if got := firstScriptReason(res); got != tc.want {
			t.Errorf("%s %s: got %q, want %q", tc.host, tc.assignment, got, tc.want)
		}
	}
}

func TestPowerShellNativeScriptFilename(t *testing.T) {
	for _, tc := range []struct{ command, path, other string }{
		{"pwsh -File –report.ps1", "/ws/–report.ps1", "/ws/-report.ps1"},
		{"sh -- –report.sh", "/ws/–report.sh", "/ws/-report.sh"},
		{"sh –report.sh", "/ws/–report.sh", "/ws/-report.sh"},
	} {
		files := fakeReader{tc.path: {Text: "echo current\n"}, tc.other: {Text: "echo other\n"}}
		res := parsePowerShellCall(t, tc.command, files)
		if got := firstScriptReason(res); got != "" || len(res.Scripts) != 1 || res.Scripts[0].Path != tc.path {
			t.Errorf("%s: refusal %q, scripts %+v, want %s", tc.command, got, res.Scripts, tc.path)
		}
	}
}

func TestScriptLocalPowerShellVariables(t *testing.T) {
	files := fakeReader{"/ws/report.ps1": {Text: "$IFS = 'sample'\n$PATH = 'sample'\nWrite-Output done\n"}, "/ws/report.sh": {Text: "echo done\n"}}
	typed := parsePowerShellCall(t, "$IFS = 'sample'; $PATH = 'sample'; ./report.ps1", files)
	if got := firstScriptReason(typed); got != "" || len(typed.Scripts) != 1 {
		t.Errorf("local variables before call: refusal %q, scripts %+v", got, typed.Scripts)
	}
	afterLocal := parsePowerShellCall(t, "$name = 'sample'; rg sample notes.txt", files)
	if len(afterLocal.Invocations) != 1 || provenPowerShell.ForInvocation(afterLocal, 0).AllowsNewProof() {
		t.Errorf("a command after a local assignment acquired standalone identity proof: %+v", afterLocal)
	}
	files["/ws/outer.sh"] = ScriptRead{Text: "pwsh -File ./report.ps1\n"}
	nested := parseScriptOn(pathshape.For("linux"), "sh ./outer.sh; sh ./report.sh", files)
	if got := firstScriptReason(nested); got != "" || len(nested.Scripts) != 3 {
		t.Errorf("local script variables: refusal %q, scripts %+v", got, nested.Scripts)
	}
}

func TestScriptPowerShellFileSizeLimits(t *testing.T) {
	for _, tc := range []struct{ name, body, reason string }{
		{"lines", strings.Repeat("#\n", maxScriptLines+1), "script-budget"},
		{"tokens", "# " + strings.Repeat("x ", maxScriptTokens), "script-budget"},
		{"braces in data", "Write-Output '" + strings.Repeat("{", maxScriptNesting+1) + "'\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := parsePowerShellCall(t, "./report.ps1", fakeReader{"/ws/report.ps1": {Text: tc.body}})
			if got := firstScriptReason(res); got != tc.reason {
				t.Errorf("got %q, want %q", got, tc.reason)
			}
			if tc.reason == "" && len(res.Scripts) != 1 || tc.reason != "" && len(res.Scripts) != 0 {
				t.Errorf("inspected %d scripts", len(res.Scripts))
			}
		})
	}
}

func TestPowerShellCommandChildrenShareWrapperLine(t *testing.T) {
	const line = "pwsh -Command 'Write-Output one; Write-Output two'\n"
	files := fakeReader{"/ws/report.sh": {Text: line + line}}
	res := parseScriptOn(pathshape.For("linux"), "sh ./report.sh", files)
	if reason := firstScriptReason(res); reason != "" {
		t.Fatal(reason)
	}
	var lines []int
	for _, inv := range res.Invocations {
		if inv.Script != nil {
			lines = append(lines, inv.Script.Line)
		}
	}
	if !slices.Equal(lines, []int{1, 1, 2, 2}) {
		t.Errorf("lines %v, want [1 1 2 2]", lines)
	}
}

func TestPowerShellModuleFileKeepsIndependentSyntaxRefusal(t *testing.T) {
	files := fakeReader{"/ws/Microsoft.PowerShell.Utility/Invoke-Expression.ps1": {Text: "Write-Output report\n"}}
	res := parsePowerShellCall(t, "Microsoft.PowerShell.Utility\\Invoke-Expression\nWrite-Output 'unfinished", files)
	if got := firstScriptReason(res); got != "script-opaque:unsupported-syntax" || len(res.Scripts) != 1 {
		t.Errorf("reason %q, scripts %+v", got, res.Scripts)
	}
}

func TestScriptCallExpandsChildInvocations(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo hello\nrm -rf /\n"}}
	for _, exe := range []string{"bash", "sh"} {
		res := parseScript(exe+" /ws/ok.sh arg", files)
		var rm *Invocation
		for i := range res.Invocations {
			if ExecutableName(res.Invocations[i].Words[0].Text) == "rm" {
				rm = &res.Invocations[i]
			}
		}
		if rm == nil || rm.Script == nil || rm.Script.Path != "/ws/ok.sh" || !rm.Inspectable {
			t.Fatalf("%s: rm child missing or untagged: %+v", exe, res.Invocations)
		}
	}
}

func TestScriptCallRefusals(t *testing.T) {
	files := fakeReader{
		"/ws/ok.sh":     {Text: "echo hi\n"},
		"/ws/big.sh":    {Limit: RuleTooLarge},
		"/ws/notes.txt": {Text: "x"},
	}
	cases := []struct{ command, reason string }{
		{"bash -x /ws/ok.sh", "script-form-unsupported-option"},
		{"bash -i /ws/ok.sh", "script-form-unsupported-option"},
		{"bash --rcfile /ws/r /ws/ok.sh", "script-form-unsupported-option"},
		{"bash /ws/notes.txt", "script-extension"},
		{"bash /ws/big.sh", "script-too-large"},
		{"bash /ws/missing.sh", "script-unreadable"},
		{"bash $HOME/ok.sh", "script-path-not-literal"},
	}
	for _, tc := range cases {
		res := parseScript(tc.command, files)
		if len(res.Invocations) != 1 || res.Invocations[0].Inspectable || !strings.HasPrefix(res.Invocations[0].Reason.String(), tc.reason) {
			t.Errorf("%q: want one refused invocation with reason %q, got %+v", tc.command, tc.reason, res.Invocations)
		}
	}
}

func TestScriptClosedOptionsAreAccepted(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	res := parseScript("bash --noprofile --norc -e -u -- /ws/ok.sh", files)
	if len(res.Invocations) == 0 || !res.Invocations[0].Inspectable || res.Invocations[0].Script == nil {
		t.Fatalf("closed options were refused: %+v", res.Invocations)
	}
}

func TestScriptUnqualifiedContextKeepsTheBlock(t *testing.T) {
	opts := ScriptOptions{Reader: fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}, Cwd: "/ws"} // zero Context: not proven
	for _, command := range []string{"bash /ws/ok.sh", "bash -x /ws/ok.sh"} {
		res := ParseScripts(context.Background(), command, POSIX, opts)
		if len(res.Invocations) != 1 || res.Invocations[0].Inspectable || res.Invocations[0].Reason.Rule() != RuleContextUnqualified {
			t.Fatalf("%q: got %+v", command, res.Invocations)
		}
	}
}

func TestScriptWithoutReaderIsUnchanged(t *testing.T) {
	got := ParseContext(context.Background(), "bash /ws/ok.sh", POSIX)
	if len(got.Invocations) != 1 || got.Invocations[0].Inspectable ||
		!strings.Contains(got.Invocations[0].Reason.String(), "requires exactly -c or -lc") || got.Invocations[0].Script != nil {
		t.Fatalf("zero-option parse changed: %+v", got.Invocations)
	}
}

func TestScriptDashCAndMetadataFormsAreNotScriptCalls(t *testing.T) {
	files := fakeReader{}
	for _, command := range []string{"bash -c 'echo x'", "bash -lc 'echo x'", "bash --version"} {
		with := parseScript(command, files)
		without := ParseContext(context.Background(), command, POSIX)
		if !reflect.DeepEqual(with, without) {
			t.Errorf("%q: reader changed the parse:\nwith:    %+v\nwithout: %+v", command, with, without)
		}
	}
}

func TestScriptNonLiteralShellWordRefused(t *testing.T) {
	got := parseScript("$BIN/bash /ws/ok.sh", fakeReader{"/ws/ok.sh": {Text: "echo ok\n"}})
	if reason := firstScriptReason(got); reason != "script-opaque:command-position-expansion" || len(got.Scripts) != 0 {
		t.Fatalf("reason %q, inspected scripts %+v", reason, got.Scripts)
	}
}

func TestScriptChildrenKeepWrapperIDsAligned(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	res := parseScript("bash /ws/ok.sh", files)
	if len(res.Invocations) != 1 {
		t.Fatalf("got %+v", res.Invocations)
	}
	inv := res.Invocations[0]
	if len(inv.Wrappers) != 1 || inv.Wrappers[0].Text != "bash" || len(inv.WrapperIDs) != len(inv.Wrappers) {
		t.Fatalf("wrappers %+v ids %v", inv.Wrappers, inv.WrapperIDs)
	}
	if len(res.Scripts) != 1 || res.Scripts[0].Path != "/ws/ok.sh" {
		t.Fatalf("scripts %+v", res.Scripts)
	}
}
