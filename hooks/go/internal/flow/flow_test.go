package flow

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandPanic(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	p := program{stdout: &stdout, stderr: &stderr}
	code := p.run([]string{"where"}, map[string]command{
		"where": func([]string) int { panic("panic sentinel") },
	})
	if code != 1 || stdout.String() != "" || stderr.String() != "flow: internal error: panic sentinel\n" {
		t.Fatalf("panic result: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestGitText(t *testing.T) {
	repo := newRepo(t)
	if got := gitText(repo, "branch", "--show-current"); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	git(t, repo, "switch", "-q", "-c", "topic")
	if got := gitText(repo, "branch", "--show-current"); got != "topic" {
		t.Fatalf("branch = %q, want topic", got)
	}
	const value = " \tkept\r\ninside\u00a0 \r\n\n"
	git(t, repo, "config", "flow.test", value)
	if got := gitText(repo, "config", "--get", "flow.test"); got != strings.TrimRight(value, "\n") {
		t.Fatalf("text = %q, want trailing LF removed only", got)
	}
	if got := gitText(repo, "rev-parse", "HEAD", "missing-ref"); got != "" {
		t.Fatalf("failed Git answer = %q", got)
	}
	t.Setenv("PATH", t.TempDir())
	if got := gitText(repo, "branch", "--show-current"); got != "" {
		t.Fatalf("absent Git answer = %q", got)
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			res := runFlow(t, t.TempDir(), nil, flag)
			mustExit(t, res, 0, "help")
			if res.stderr != "" {
				t.Fatalf("help stderr = %q", res.stderr)
			}
			for _, command := range []string{"flow log", "flow status", "flow where", "flow bind", "flow path", "flow audit"} {
				if !strings.Contains(res.stdout, command) {
					t.Errorf("usage missing %q: %s", command, res.stdout)
				}
			}
		})
	}
}

// TestHelpNamesStorageAndExitCodes keeps the usage text in step with the
// storage setting and the exit codes that flow-state.md documents: dropping a
// token or a phrase from usage must fail here.
func TestHelpNamesStorageAndExitCodes(t *testing.T) {
	t.Parallel()
	required := []string{
		"FLOW_WORK_ROOT", "FLOW_SESSION", "FLOW_STORAGE",
		"agentics.flowStorage", "committed, the default, or local",
		"usage or an unexpected failure",
		"invalid cell",
		"refused call form", "invalid storage setting", "local store", "refused bind",
		"no flow found",
		"more than one flow matched, or both stores hold the folder",
		"small fix in progress",
	}
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			res := runFlow(t, t.TempDir(), nil, flag)
			mustExit(t, res, 0, "help")
			if res.stderr != "" {
				t.Fatalf("help stderr = %q", res.stderr)
			}
			for _, want := range required {
				if !strings.Contains(res.stdout, want) {
					t.Errorf("usage missing %q: %s", want, res.stdout)
				}
			}
		})
	}
}

func TestCommandErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"missing", nil, usage},
		{"unknown", []string{"bogus"}, usage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := runFlow(t, t.TempDir(), nil, tt.args...)
			mustExit(t, res, 1, tt.name)
			if res.stdout != "" || res.stderr != tt.want {
				t.Fatalf("stdout=%q stderr=%q, want empty stdout and %q", res.stdout, res.stderr, tt.want)
			}
		})
	}
}
