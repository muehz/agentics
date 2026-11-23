package shellwords

import (
	"context"
	"reflect"
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
	return ScriptRead{Path: path, Limit: RuleUnreadable, Detail: "no such fixture"}
}

var provenPOSIX = ExecutionContext{Dialect: POSIX, Proven: true}

func parseScript(command string, files fakeReader) Result {
	return ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
}

// parseScriptOn is parseScript with the path rules of host.
func parseScriptOn(host pathshape.Host, command string, files fakeReader) Result {
	return ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws", Host: host})
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

func TestScriptNonLiteralShellWordIsNotAScriptCall(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	command := "$BIN/bash /ws/ok.sh"
	with := parseScript(command, files)
	without := ParseContext(context.Background(), command, POSIX)
	if len(with.Invocations) != 1 || with.Invocations[0].Words[0].Literal || with.Invocations[0].Script != nil {
		t.Fatalf("%q: want one non-literal bash invocation, got %+v", command, with.Invocations)
	}
	if !reflect.DeepEqual(with, without) {
		t.Errorf("%q: reader changed the parse:\nwith:    %+v\nwithout: %+v", command, with, without)
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
