package flow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var suite struct {
	home, tmpRoot, gitConfig string
	// template is a repository on branch main with one commit, built on first
	// use and once per run. newRepo copies it instead of spawning Git for every
	// fixture. It is lazy so the child test binaries some tests start do not
	// build one they never use.
	template     string
	templateOnce sync.Once
	templateErr  error
}

func TestMain(m *testing.M) {
	if err := setUpSuite(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(suite.tmpRoot)
	os.Exit(code)
}

func setUpSuite() error {
	tmp, err := os.MkdirTemp("", "agentics-flow-")
	if err != nil {
		return err
	}
	suite.tmpRoot = tmp
	suite.home = filepath.Join(tmp, "home")
	if err := os.MkdirAll(suite.home, 0o755); err != nil {
		return err
	}
	suite.gitConfig = filepath.Join(tmp, "gitconfig")
	if err := os.WriteFile(suite.gitConfig, nil, 0o600); err != nil {
		return err
	}
	// Production Git children inherit this environment, so they must be isolated
	// just like the fixture commands, without changing directories per test.
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		if err := os.Unsetenv(name); err != nil {
			return err
		}
	}
	for _, setting := range baseEnv() {
		name, value, _ := strings.Cut(setting, "=")
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	return nil
}

// templateRepo returns the path of suite.template, building it on first use.
func templateRepo() (string, error) {
	suite.templateOnce.Do(func() { suite.templateErr = buildTemplate() })
	return suite.template, suite.templateErr
}

// buildTemplate creates suite.template: branch main, core.autocrlf false, and
// one commit "init" holding README.md with the content "x\n".
func buildTemplate() error {
	suite.template = filepath.Join(suite.tmpRoot, "template")
	if err := os.MkdirAll(suite.template, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(suite.template, "README.md"), []byte("x\n"), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "core.autocrlf", "false"},
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = suite.template, baseEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("template git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}
