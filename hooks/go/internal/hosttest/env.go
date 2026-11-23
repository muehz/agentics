package hosttest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// GuardEnv is every environment variable a shipped hook reads to decide or
// report a guard result. A test that runs a guard scrubs them all first, so
// nothing inherited from a developer's shell can change its result.
// TestGuardEnvCoversShippedReads fails when a shipped read is missing here.
var GuardEnv = []string{
	"GIT_PAGER",
	"SKIP_COMMAND_GUARD", "COMMAND_GUARD_MODE", "COMMAND_GUARD_LOG_DIR", "COMMAND_GUARD_ALLOWLIST",
	"SKIP_SECRETS_GUARD", "SECRETS_GUARD_MODE", "SECRETS_GUARD_ALLOWLIST",
	"RIPGREP_CONFIG_PATH",
	"PGPASSFILE", "KUBECONFIG", "GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_SHARED_CREDENTIALS_FILE", "SOPS_AGE_KEY_FILE",
	"AGX_RELEASE_RECORD", "AGX_RELEASE_RECORD_SHA256", "AGX_RELEASE_ENVIRONMENTS", "RELEASE_GUARD_LOG_DIR",
	"CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA", "COPILOT_PLUGIN_ROOT", "COPILOT_PLUGIN_DATA",
	"PLUGIN_ROOT", "PLUGIN_DATA", "AGX_VAULT", "AGX_SOURCE_DIGEST",
	// secrets-guardian withholds its glob and cd proofs when the shell
	// options these name are inherited.
	"CDPATH", "BASHOPTS", "GLOBIGNORE",
}

// unsetWhenEmpty names the GuardEnv variables whose empty-but-set value
// differs from absence: blanking them would itself change the result, so the
// scrub truly unsets them.
var unsetWhenEmpty = map[string]bool{
	"GIT_PAGER":                 true,
	"AGX_RELEASE_ENVIRONMENTS":  true,
	"AGX_RELEASE_RECORD_SHA256": true,
}

// ScrubGuardEnv blanks every GuardEnv variable for the rest of the test and
// truly unsets the ones whose empty value differs from absence. t.Setenv
// records each original value first, so all of them come back at cleanup;
// a test that calls it cannot run in parallel.
func ScrubGuardEnv(t testing.TB) {
	t.Helper()
	for _, name := range GuardEnv {
		t.Setenv(name, "")
		if unsetWhenEmpty[name] {
			os.Unsetenv(name)
		}
	}
}

// IsolateHome points HOME and USERPROFILE at one new empty directory for the
// rest of the test and returns it, so no test reads the developer's own home.
func IsolateHome(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// MainWithIsolatedHome runs m's tests with HOME and USERPROFILE pointing at a
// new empty directory, removes the directory, and returns the exit code for
// TestMain to pass to os.Exit. A test that needs a home of its own sets both
// variables again with t.Setenv or IsolateHome.
func MainWithIsolatedHome(m *testing.M) int {
	return mainWithIsolatedHome(m.Run)
}

func mainWithIsolatedHome(run func() int) int {
	// Child processes that tests spawn, such as go build, derive their cache,
	// config, and module paths from HOME. Pin those to the real home first so
	// a temporary HOME does not start them cold or ignore go env -w settings.
	pin := func(name string, base func() (string, error), elem ...string) {
		if _, set := os.LookupEnv(name); set {
			return
		}
		if dir, err := base(); err == nil {
			os.Setenv(name, filepath.Join(append([]string{dir}, elem...)...))
		}
	}
	pin("GOCACHE", os.UserCacheDir, "go-build")
	pin("GOENV", os.UserConfigDir, "go", "env")
	pin("GOPATH", os.UserHomeDir, "go")

	home, err := os.MkdirTemp("", "agx-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	return run()
}
