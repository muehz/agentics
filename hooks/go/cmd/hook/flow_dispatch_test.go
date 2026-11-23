package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

type unreadFlowInput struct{ t *testing.T }

func (r unreadFlowInput) Read([]byte) (int, error) {
	r.t.Fatal("flow read hook stdin")
	return 0, io.EOF
}

func TestFlowHelpBypassesHookPipeline(t *testing.T) {
	t.Setenv(printDigestEnv, "")
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"flow", flag}, unreadFlowInput{t}, &stdout, &stderr)
		if code != 0 || !strings.Contains(stdout.String(), "flow log") || stderr.Len() != 0 {
			t.Fatalf("flow %s: exit=%d stdout=%q stderr=%q", flag, code, stdout.String(), stderr.String())
		}
	}
	deps := productionHookDependencies()
	deps.lookup = func(string) (hookspec.Handler, bool) { t.Fatal("flow resolved a hook"); return nil, false }
	deps.dispatch = func(string, string, []string) (string, *provider.Rendered) {
		t.Fatal("flow dispatched a provider")
		return "", nil
	}
	deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) {
		t.Fatal("flow normalized a payload")
		return hookspec.Event{}, nil
	}
	deps.render = func(string, string, hookspec.Outcome, []byte) provider.Rendered {
		t.Fatal("flow rendered through a provider")
		return provider.Rendered{}
	}
	var stdout, stderr bytes.Buffer
	if code := runWithDependencies(deps, []string{"flow", "-h"}, unreadFlowInput{t}, &stdout, &stderr); code != 0 {
		t.Fatalf("flow help exit=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowDigestPrecedence(t *testing.T) {
	t.Setenv(printDigestEnv, "1")
	var stdout, stderr bytes.Buffer
	code := run([]string{"flow", "-h"}, unreadFlowInput{t}, &stdout, &stderr)
	want := sourceDigest + "\n" + toolchainPrefix + runtime.Version() + "\n"
	if code != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("digest: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestFlowMissingCommand(t *testing.T) {
	t.Setenv(printDigestEnv, "")
	var help, helpErr bytes.Buffer
	if code := run([]string{"flow", "--help"}, unreadFlowInput{t}, &help, &helpErr); code != 0 || help.Len() == 0 || helpErr.Len() != 0 {
		t.Fatalf("help: exit=%d stdout=%q stderr=%q", code, help.String(), helpErr.String())
	}
	for _, args := range [][]string{{"flow"}, {"flow", "bogus"}} {
		var stdout, stderr bytes.Buffer
		code := run(args, unreadFlowInput{t}, &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || stderr.String() != help.String() {
			t.Fatalf("flow usage %q: exit=%d stdout=%q stderr=%q, want usage on stderr", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestFlowIsNotAProviderOrHook(t *testing.T) {
	t.Setenv(printDigestEnv, "")
	if _, ok := surface.Parse("flow"); ok {
		t.Fatal("flow registered as provider")
	}
	if _, ok := hooks.Lookup("flow"); ok {
		t.Fatal("flow registered as hook")
	}
	for _, id := range surface.All() {
		var stdout, stderr bytes.Buffer
		code := run([]string{string(id), "flow"}, unreadFlowInput{t}, &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "agx: unknown hook \"flow\"") {
			t.Fatalf("%s flow: exit=%d stdout=%q stderr=%q", id, code, stdout.String(), stderr.String())
		}
	}
	for _, first := range []string{"Flow", "flow-extra", " flow"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{first, "-h"}, unreadFlowInput{t}, &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), fmt.Sprintf("agx: unknown provider %q", first)) {
			t.Fatalf("%q: exit=%d stdout=%q stderr=%q", first, code, stdout.String(), stderr.String())
		}
	}
}

func TestFlowExitCodesPassThrough(t *testing.T) {
	t.Setenv(printDigestEnv, "")
	t.Setenv("FLOW_SESSION", "dispatch-sentinel")
	wantDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{0, 1, 2, 3, 4, 5} {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			deps := productionHookDependencies()
			called := false
			deps.flow = func(args []string, dir string, getenv func(string) string, now func() time.Time, stdout, stderr io.Writer) int {
				called = true
				if !reflect.DeepEqual(args, []string{"log", "sentinel"}) || dir != wantDir || getenv("FLOW_SESSION") != "dispatch-sentinel" || time.Since(now()).Abs() > time.Minute {
					t.Fatalf("flow inputs not forwarded: args=%q dir=%q", args, dir)
				}
				fmt.Fprint(stdout, "flow stdout")
				fmt.Fprint(stderr, "flow stderr")
				return want
			}
			var stdout, stderr bytes.Buffer
			code := runWithDependencies(deps, []string{"flow", "log", "sentinel"}, unreadFlowInput{t}, &stdout, &stderr)
			if !called || code != want || stdout.String() != "flow stdout" || stderr.String() != "flow stderr" {
				t.Fatalf("flow passthrough: called=%v exit=%d want=%d stdout=%q stderr=%q", called, code, want, stdout.String(), stderr.String())
			}
		})
	}
}
