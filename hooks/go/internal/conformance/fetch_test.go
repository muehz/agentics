package conformance

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// The dispatcher's fetch path is the one route by which a hook binary can
// appear on a machine that installed the plugin without one. These tests
// drive the real run-hook.cmd (and on Windows the real fetch-hook.ps1)
// against a local stand-in for the GitHub Release and assert at the seam the
// host sees: process streams, status, and bytes published to the user cache.
//

// fetchFixtureVersion is the plugin version the fixture claims, so the
// dispatcher asks the stand-in for /v<version>/<name>.
const fetchFixtureVersion = "9.9.9"

type fetchFixture struct {
	hooksDir string
	cacheDir string
	cacheEnv []string
	stdin    string
	// bash is the interpreter that passed its probe; it is empty on Windows, where the dispatcher runs under cmd.exe.
	bash     string
	name     string
	binary   []byte
	server   *httptest.Server
	requests atomic.Int64
}

// Serve the tracked binary against its tracked manifest when present. A source-only
// checkout uses a native fixture and records its digest in the installed copy.
func newFetchFixture(t *testing.T) *fetchFixture {
	t.Helper()
	return fetchFixtureForRuntime(t, false)
}

func fetchFixtureForRuntime(t *testing.T, buildCurrent bool) *fetchFixture {
	t.Helper()
	repoRoot := hosttest.Checkout(t)
	name := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
	if name == "" {
		t.Skipf("no packaged hook binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	binary, err := os.ReadFile(filepath.Join(repoRoot, "hooks", "bin", name))
	sourceOnly := errors.Is(err, os.ErrNotExist)
	if sourceOnly || buildCurrent {
		binary, err = sourceRuntime(repoRoot, t.TempDir())
	}
	if err != nil {
		t.Fatal(err)
	}
	var bash string
	if runtime.GOOS != "windows" {
		bash = hookutiltest.RequireInterpreter(t, "bash")
	}

	root := t.TempDir()
	hooksDir := filepath.Join(root, "installed & [é] !%PATH%", "hooks")
	if err := os.MkdirAll(filepath.Join(hooksDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(hooksDir), ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	plugin := `{"name": "agentics", "version": "` + fetchFixtureVersion + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(hooksDir), ".claude-plugin", "plugin.json"), []byte(plugin), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"run-hook.cmd", "fetch-hook.ps1", "SHA256SUMS"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, "hooks", file))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hooksDir, file), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	f := &fetchFixture{hooksDir: hooksDir, bash: bash, name: name, binary: binary}
	if sourceOnly || buildCurrent {
		recordManifest(t, hooksDir, name, binary)
	}
	cacheHome := filepath.Join(t.TempDir(), "runtime cache & [é]!%")
	f.cacheDir = cacheHome
	switch runtime.GOOS {
	case "windows":
		f.cacheEnv = []string{"LOCALAPPDATA=" + cacheHome}
	case "darwin":
		f.cacheEnv = []string{"HOME=" + cacheHome}
		f.cacheDir = filepath.Join(cacheHome, "Library", "Caches")
	default:
		f.cacheEnv = []string{"XDG_CACHE_HOME=" + cacheHome, "HOME=" + t.TempDir()}
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.URL.Path != "/v"+fetchFixtureVersion+"/"+name {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(binary)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// Reuse one native build when a source-only checkout has no packaged runtime.
var builtRuntime struct {
	sync.Once
	data []byte
	err  error
}

func sourceRuntime(repo, scratch string) ([]byte, error) {
	builtRuntime.Do(func() {
		path := filepath.Join(scratch, "fixture-runtime")
		cmd := exec.Command("go", "build", "-buildvcs=false", "-o", path, "./cmd/hook")
		cmd.Dir = filepath.Join(repo, "hooks", "go")
		if out, err := cmd.CombinedOutput(); err != nil {
			builtRuntime.err = fmt.Errorf("build fixture: %w: %s", err, out)
			return
		}
		builtRuntime.data, builtRuntime.err = os.ReadFile(path)
	})
	return builtRuntime.data, builtRuntime.err
}

func recordManifest(t *testing.T, hooksDir, name string, data []byte) {
	t.Helper()
	line := fmt.Sprintf("%x  bin/%s\n", sha256.Sum256(data), name)
	if err := os.WriteFile(filepath.Join(hooksDir, "SHA256SUMS"), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
}

func (f *fetchFixture) cached() string {
	return filepath.Join(f.cacheDir, "agentics", fetchFixtureVersion, fmt.Sprintf("%x", sha256.Sum256(f.binary)), f.name)
}

func TestDispatcherProvisionOnly(t *testing.T) {
	f := newFetchFixture(t)
	installed := filepath.Join(f.hooksDir, "bin", f.name)
	assert := func(want string, offline bool, requests int64) {
		t.Helper()
		var env []string
		if offline {
			env = []string{"AGX_NO_FETCH=0"}
		}
		status, out, diag := f.dispatch(t, env, "--provision-only")
		if want == "" {
			if status == 0 || out != "" || !strings.Contains(diag, "agx:") {
				t.Fatalf("offline miss: exit %d stdout %q stderr %q", status, out, diag)
			}
		} else if status != 0 || strings.ReplaceAll(out, "\r\n", "\n") != want+"\n" {
			t.Fatalf("provision: exit %d stdout %q want %q stderr %q", status, out, want, diag)
		}
		if n := f.requests.Load(); n != requests {
			t.Fatalf("logical asset requests = %d, want %d", n, requests)
		}
	}
	assert("", true, 0)
	if err := os.WriteFile(installed, f.binary, 0755); err != nil {
		t.Fatal(err)
	}
	assert(installed, true, 0)
	if err := os.WriteFile(installed, []byte("invalid preseed"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(installed), 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Dir(installed), 0755) })
	assert(f.cached(), false, 1)
	assert(f.cached(), true, 1)
	assert(f.cached(), false, 1)
	if b, err := os.ReadFile(installed); err != nil || string(b) != "invalid preseed" {
		t.Fatalf("installed preseed overwritten: %v", err)
	}
	if err := os.WriteFile(f.cached(), []byte("stale"), 0755); err != nil {
		t.Fatal(err)
	}
	assert("", true, 1)
	assert(f.cached(), false, 2)
}

func TestDispatcherStartupProvision(t *testing.T) {
	f := fetchFixtureForRuntime(t, true)
	original := filepath.Dir(f.hooksDir)
	root := original + " 'it’s' $(echo literal)"
	if err := os.Rename(original, root); err != nil {
		t.Fatal(err)
	}
	f.hooksDir = filepath.Join(root, "hooks")
	shell := "sh"
	name := "agx"
	if runtime.GOOS == "windows" {
		shell = "exe"
		name = "agx.exe"
	}
	installLauncherEntry(t, root, shell)
	launcher := filepath.Join(root, "bin", name)
	f.stdin = `{"source":"new","cwd":"not-the-install","initialPrompt":"untrusted text"}`
	var hint string
	run := func(offline, wantHint bool, requests int64) {
		t.Helper()
		env := []string{"AGX_SOURCE_DIGEST=1"}
		if offline {
			env = append(env, "AGX_NO_FETCH=0")
		}
		status, out, diag := f.dispatch(t, env, "copilot", "startup")
		if status != 0 || diag != "" || f.requests.Load() != requests {
			t.Fatalf("startup exit %d stdout %q stderr %q requests %d, want %d", status, out, diag, f.requests.Load(), requests)
		}
		if !wantHint {
			if out != "" {
				t.Fatalf("expected silence, got %q", out)
			}
			return
		}
		var result map[string]string
		hint = ""
		if err := json.Unmarshal([]byte(out), &result); err != nil || len(result) != 1 ||
			result["additionalContext"] == "" || strings.Contains(out, "untrusted text") {
			t.Fatalf("startup did not identify the installed launcher: %q", out)
		}
		hint = result["additionalContext"]
	}
	run(true, false, 0)
	run(false, true, 1)
	run(false, true, 1)
	run(true, true, 1)
	// Execute the literal invocation from the model context in an unrelated
	// directory. Each shell rendering must survive the install name.
	checkHelp := func(t *testing.T, cmd *exec.Cmd) {
		t.Helper()
		cmd.Dir = t.TempDir()
		cmd.Env = append(launcherEnv(f.name), f.cacheEnv...)
		cmd.Env = append(cmd.Env, "AGX_NO_FETCH=1", "AGX_RELEASE_URL="+f.server.URL)
		got := runLauncher(t, cmd)
		if got.exit != 0 || !strings.Contains(got.stdout, "agx flow") || f.requests.Load() != 1 {
			t.Fatalf("hint %q failed offline: %+v; requests %d", hint, got, f.requests.Load())
		}
	}
	invocations := strings.TrimPrefix(strings.TrimSuffix(hint, "."), "For agx commands, run PowerShell: ")
	ps, bash, ok := strings.Cut(invocations, " or Bash: ")
	if !ok {
		t.Fatalf("missing shell invocations in %q", hint)
	}
	if runtime.GOOS == "windows" {
		checkHelp(t, exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps+"; exit $LASTEXITCODE"))
		checkHelp(t, exec.Command(hookutiltest.RequireInterpreter(t, "bash"), "-c", bash))
	} else {
		checkHelp(t, exec.Command("/bin/sh", "-c", bash))
		t.Run("PowerShell", func(t *testing.T) {
			pwsh, err := exec.LookPath("pwsh")
			if err != nil {
				t.Skip("pwsh unavailable on this host")
			}
			checkHelp(t, exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", ps+"; exit $LASTEXITCODE"))
		})
	}
	f.stdin = `{"source":"resume"}`
	run(true, false, 1)
	f.stdin = `{"source":"startup"}`
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	run(true, false, 1)
	if err := os.Mkdir(launcher, 0755); err != nil {
		t.Fatal(err)
	}
	run(true, false, 1)
}

func TestDispatcherProvisionPathEncoding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX filename bytes")
	}
	for _, tc := range []struct {
		name, component string
		valid           bool
	}{
		{"valid-unicode", "¢界🙂", true},
		{"valid-ascii-control", "path\x01end", true},
		{"invalid-byte", "bad\xff", false},
		{"invalid-continuation", "bad\xc2\x01", false},
		{"truncated", "bad\xe2\x82", false},
		{"overlong", "bad\xe0\x80\x80", false},
		{"surrogate", "bad\xed\xa0\x80", false},
		{"out-of-range", "bad\xf4\x90\x80\x80", false},
		{"line-feed", "bad\nname", false},
		{"carriage-return", "bad\rname", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t)
			original := filepath.Dir(f.hooksDir)
			install := filepath.Join(filepath.Dir(original), tc.component)
			if err := os.Rename(original, install); err != nil {
				if runtime.GOOS == "darwin" && (errors.Is(err, syscall.EILSEQ) || errors.Is(err, syscall.EINVAL)) {
					t.Skipf("filesystem rejects fixture filename: %v", err)
				}
				t.Fatal(err)
			}
			f.hooksDir = filepath.Join(install, "hooks")
			runtimePath := filepath.Join(f.hooksDir, "bin", f.name)
			if err := os.WriteFile(runtimePath, f.binary, 0755); err != nil {
				t.Fatal(err)
			}
			exit, stdout, stderr := f.dispatch(t, []string{"LC_ALL=C", "AGX_NO_FETCH=0", "AGX_DEBUG="}, "--provision-only")
			if tc.valid {
				want, err := filepath.EvalSymlinks(runtimePath)
				if err != nil {
					t.Fatal(err)
				}
				if exit != 0 || stdout != want+"\n" || stderr != "" {
					t.Fatalf("valid path: exit %d stdout %q stderr %q", exit, stdout, stderr)
				}
			} else if exit == 0 || stdout != "" || !strings.Contains(stderr, "single-line UTF-8 path") {
				t.Fatalf("invalid path: exit %d stdout %q stderr %q", exit, stdout, stderr)
			}
			if f.requests.Load() != 0 {
				t.Fatalf("verified offline preseed made %d requests", f.requests.Load())
			}
		})
	}
}

func TestDispatcherProvisionClosedStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file descriptor closure")
	}
	f := newFetchFixture(t)
	if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), f.binary, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(f.bash, "-c", `exec "$1" ./run-hook.cmd --provision-only >&-`, "closed-stdout", f.bash)
	cmd.Dir = f.hooksDir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "AGX_NO_FETCH=0", "AGX_DEBUG=")
	diag, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || !bytes.Contains(diag, []byte("write error")) {
		t.Fatalf("closed stdout: %v diagnostic %q", err, diag)
	}
}

func TestDispatcherConcurrentProvision(t *testing.T) {
	f := newFetchFixture(t)
	type result struct {
		status    int
		out, diag string
	}
	runs := make(chan result, 3)
	for range cap(runs) {
		go func() {
			s, o, d := f.dispatch(t, nil, "--provision-only")
			runs <- result{s, o, d}
		}()
	}
	for range cap(runs) {
		r := <-runs
		if r.status != 0 || strings.TrimRight(r.out, "\r\n") != f.cached() {
			t.Errorf("concurrent provision: %#v", r)
		}
	}
	if b, err := os.ReadFile(f.cached()); err != nil || !bytes.Equal(b, f.binary) {
		t.Fatalf("published runtime: %v", err)
	}
	if files, err := os.ReadDir(filepath.Dir(f.cached())); err != nil || len(files) != 1 {
		t.Fatalf("published directory: %v, %v", files, err)
	}
}

// dispatch runs the installed script with the fixture input and captures both streams.
func (f *fetchFixture) dispatch(t *testing.T, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Use an explicit path without interpolating the fixture directory into cmd syntax.
		cmd = exec.Command("cmd.exe", append([]string{"/d", "/c", `.\run-hook.cmd`}, args...)...)
	} else {
		cmd = exec.Command(f.bash, append([]string{"run-hook.cmd"}, args...)...)
	}
	cmd.Dir = f.hooksDir
	env := []string{}
	for _, kv := range os.Environ() {
		// The old names are dropped too, so an ambient value cannot make a
		// test pass that should prove the dispatcher ignores them.
		if strings.HasPrefix(kv, "AGX_") || strings.HasPrefix(kv, "AGX_HOOK_") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "AGX_RELEASE_URL="+f.server.URL, "AGX_DEBUG=1")
	env = append(env, f.cacheEnv...)
	cmd.Env = append(env, extraEnv...)
	cmd.Stdin = strings.NewReader(f.stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("dispatcher did not start: %v", err)
	}
	return cmd.ProcessState.ExitCode(), stdout.String(), stderr.String()
}

// cacheEntries lists files in the cache, including partials and wrongly keyed files.
func (f *fetchFixture) cacheEntries(t *testing.T) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(f.cacheDir, func(path string, entry os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == f.cacheDir {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func (f *fetchFixture) tamperManifest(t *testing.T) {
	t.Helper()
	path := filepath.Join(f.hooksDir, "SHA256SUMS")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(f.binary)
	want := hex.EncodeToString(sum[:])
	if !bytes.Contains(data, []byte(want)) {
		t.Fatalf("hooks/SHA256SUMS does not record the committed %s; the checkout is inconsistent", f.name)
	}
	tampered := bytes.Replace(data, []byte(want), []byte("0000"+want[4:]), 1)
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherFetchesMissingBinary(t *testing.T) {
	f := newFetchFixture(t)
	// Empty stdin on claude-code is the argv/dispatch truth table's fail-closed
	// exit 1: proof the fetched binary ran, not the dispatcher's posture.
	exit, _, stderr := f.dispatch(t, nil, "claude-code", "command-guardian")
	if exit != 1 {
		t.Fatalf("fetched binary should run and exit 1 on empty stdin, got %d\nstderr: %s", exit, stderr)
	}
	if !strings.Contains(stderr, "resolved binary") {
		t.Fatalf("dispatcher did not report resolving the fetched binary:\n%s", stderr)
	}
	got, err := os.ReadFile(f.cached())
	if err != nil {
		t.Fatalf("fetched binary missing: %v", err)
	}
	if !bytes.Equal(got, f.binary) {
		t.Fatal("fetched binary differs from the committed one")
	}
	if entries := f.cacheEntries(t); len(entries) != 1 {
		t.Fatalf("cache should hold only the binary, got %v", entries)
	}
	if n := f.requests.Load(); n != 1 {
		t.Fatalf("expected one download, got %d", n)
	}

	// The second dispatch must use the file in place, not the network.
	exit, _, stderr = f.dispatch(t, nil, "claude-code", "command-guardian")
	if exit != 1 || strings.Contains(stderr, "fetching") {
		t.Fatalf("second dispatch should run the cached binary without fetching: exit %d\n%s", exit, stderr)
	}
	if n := f.requests.Load(); n != 1 {
		t.Fatalf("cached binary should not be downloaded again, got %d requests", n)
	}
}

func TestDispatcherAcceptsChecksumCase(t *testing.T) {
	for _, spelling := range []string{"lower", "upper", "mixed"} {
		t.Run(spelling, func(t *testing.T) {
			f := newFetchFixture(t)
			manifest := filepath.Join(f.hooksDir, "SHA256SUMS")
			data, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			found := false
			for i, line := range lines {
				fields := strings.Fields(line)
				if len(fields) != 2 || fields[1] != "bin/"+f.name {
					continue
				}
				digest := strings.ToLower(fields[0])
				switch spelling {
				case "upper":
					digest = strings.ToUpper(digest)
				case "mixed":
					letters := 0
					var mixed strings.Builder
					for _, r := range digest {
						if r >= 'a' && r <= 'f' {
							if letters%2 == 0 {
								r -= 'a' - 'A'
							}
							letters++
						}
						mixed.WriteRune(r)
					}
					digest = mixed.String()
					if digest == strings.ToLower(digest) || digest == strings.ToUpper(digest) {
						t.Fatal("fixture digest needs at least two hexadecimal letters for mixed case")
					}
				}
				lines[i] = strings.Replace(line, fields[0], digest, 1)
				found = true
				break
			}
			if !found {
				t.Fatalf("manifest has no host entry for %s", f.name)
			}
			if err := os.WriteFile(manifest, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				t.Fatal(err)
			}
			exit, _, stderr := f.dispatch(t, nil, "claude-code", "command-guardian")
			if exit != 1 || !strings.Contains(stderr, "resolved binary") {
				t.Fatalf("valid %s checksum must fetch and run the binary: exit %d\n%s", spelling, exit, stderr)
			}
			installed, err := os.ReadFile(f.cached())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(installed, f.binary) {
				t.Fatal("installed bytes differ from the served binary")
			}
			if n := f.requests.Load(); n != 1 {
				t.Fatalf("expected one download, got %d requests", n)
			}
			if entries := f.cacheEntries(t); len(entries) != 1 || entries[0] != f.name {
				t.Fatalf("cache must contain only the binary, without partial files: %v", entries)
			}
		})
	}
}

func TestDispatcherDiscardsMismatchedDownload(t *testing.T) {
	f := newFetchFixture(t)
	f.tamperManifest(t)
	for _, guard := range []string{"pre-tool", "command-guardian", "release-guardian", "secrets-guardian"} {
		exit, _, stderr := f.dispatch(t, nil, "claude-code", guard)
		if exit != 2 || !strings.Contains(stderr, "failing closed") {
			t.Fatalf("%s: a rejected download must fail closed at exit 2, got %d\n%s", guard, exit, stderr)
		}
		if !strings.Contains(stderr, "does not match hooks/SHA256SUMS") {
			t.Fatalf("%s: the discard must be reported:\n%s", guard, stderr)
		}
		if entries := f.cacheEntries(t); len(entries) != 0 {
			t.Fatalf("%s: a rejected download must leave nothing behind, got %v", guard, entries)
		}
	}
	// A name the wrapper does not know still fetches and discards a bad
	// download, then fails open: only the four guard names fail closed.
	const hook = "unknown-hook"
	before := f.requests.Load()
	exit, stdout, stderr := f.dispatch(t, nil, "claude-code", hook)
	if exit != 0 || stdout != "" || !strings.Contains(stderr, "does not match hooks/SHA256SUMS") || f.requests.Load() != before+1 {
		t.Fatalf("%s must fetch, discard, and fail open: exit %d stdout %q stderr %q", hook, exit, stdout, stderr)
	}
	if entries := f.cacheEntries(t); len(entries) != 0 {
		t.Fatalf("rejected download left files: %v", entries)
	}
}

func TestDispatcherHonorsNoFetch(t *testing.T) {
	f := newFetchFixture(t)
	exit, _, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1"}, "claude-code", "command-guardian")
	// The cmd half names the missing binary; the Bash half reports every
	// unusable runtime, a missing binary included, with one message.
	want := "agx: guard runtime unavailable - failing closed"
	if runtime.GOOS == "windows" {
		want = "agx: guard binary missing - failing closed"
	}
	if exit != 2 || !strings.Contains(stderr, want) {
		t.Fatalf("with fetching disabled the guard must fail closed at exit 2, got %d\n%s", exit, stderr)
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("AGX_NO_FETCH must keep the dispatcher off the network, saw %d requests", n)
	}
	if entries := f.cacheEntries(t); len(entries) != 0 {
		t.Fatalf("nothing may be written, got %v", entries)
	}
}

// The renamed launcher reads only the AGX_ controls. Each old variable is set
// to the value that would change the outcome if it were still read: no-fetch
// would fail the guard closed, the release URL would send the download to a
// second server, and debug would add trace lines to stderr.
func TestDispatcherIgnoresOldNoFetchVariable(t *testing.T) {
	f := newFetchFixture(t)
	var oldRequests atomic.Int64
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldRequests.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(old.Close)

	exit, _, stderr := f.dispatch(t, []string{
		"AGX_HOOK_NO_FETCH=1",
		"AGX_HOOK_DEBUG=1",
		"AGX_HOOK_RELEASE_URL=" + old.URL,
		"AGX_DEBUG=",
	}, "claude-code", "command-guardian")
	if exit != 1 {
		t.Fatalf("the fetched binary should run and exit 1 on empty stdin, got %d\nstderr: %s", exit, stderr)
	}
	if n := f.requests.Load(); n != 1 {
		t.Fatalf("the fixture's release server should see exactly one request, got %d", n)
	}
	if n := oldRequests.Load(); n != 0 {
		t.Fatalf("the old release URL variable must not be read, its server saw %d requests", n)
	}
	got, err := os.ReadFile(f.cached())
	if err != nil {
		t.Fatalf("fetched binary missing: %v", err)
	}
	if !bytes.Equal(got, f.binary) {
		t.Fatal("fetched binary differs from the served one")
	}
	if entries := f.cacheEntries(t); len(entries) != 1 || entries[0] != f.name {
		t.Fatalf("cache should hold only the binary, got %v", entries)
	}
	for _, leak := range []string{"AGX_HOOK", "run-hook", "fetch-hook.ps1"} {
		if strings.Contains(stderr, leak) {
			t.Fatalf("stderr must not mention %q:\n%s", leak, stderr)
		}
	}

	control := newFetchFixture(t)
	controlExit, _, controlStderr := control.dispatch(t, []string{"AGX_DEBUG="}, "claude-code", "command-guardian")
	if controlExit != exit || controlStderr != stderr {
		t.Fatalf("old variables changed the dispatch: exit %d stderr %q, control exit %d stderr %q", exit, stderr, controlExit, controlStderr)
	}
}

func TestDispatcherGroupedMissingBinaryPostures(t *testing.T) {
	for _, hook := range []string{"pre-tool", "command-guardian", "release-guardian", "secrets-guardian", "unknown-hook"} {
		t.Run(hook, func(t *testing.T) {
			f := newFetchFixture(t)
			exit, stdout, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG="}, "claude-code", hook)
			closed := hook == "pre-tool" || strings.HasSuffix(hook, "guardian")
			if closed {
				if exit != 2 || !strings.Contains(stderr, "failing closed") {
					t.Fatalf("%s must fail closed: exit %d stderr %q", hook, exit, stderr)
				}
			} else if exit != 0 || stdout != "" || stderr != "" {
				t.Fatalf("%s must fail open silently: exit %d stdout %q stderr %q", hook, exit, stdout, stderr)
			}
			if n := f.requests.Load(); n != 0 {
				t.Fatalf("no-fetch made %d requests", n)
			}
		})
	}
}

func TestDispatcherUnexecutableBinaryPostures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix execute-bit posture; cmd startup failure is separately inspected")
	}
	for _, hook := range []string{"pre-tool", "command-guardian", "release-guardian", "secrets-guardian"} {
		t.Run(hook, func(t *testing.T) {
			f := newFetchFixture(t)
			if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), f.binary, 0o644); err != nil {
				t.Fatal(err)
			}
			exit, _, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG="}, "claude-code", hook)
			if exit != 2 || !strings.Contains(stderr, "failing closed") {
				t.Fatalf("unexecutable guard: exit %d stderr %q", exit, stderr)
			}
		})
	}
}

// An executable bit does not guarantee a usable interpreter or executable
// image. These fixtures reach the kernel launch failure after wrapper lookup.
func TestDispatcherKernelStartupFailurePostures(t *testing.T) {
	for _, args := range [][]string{{"claude-code", "pre-tool"}, {"claude-code", "command-guardian"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			f := newFetchFixture(t)
			binary := filepath.Join(f.hooksDir, "bin", f.name)
			body := "#!/" + filepath.ToSlash(filepath.Join(t.TempDir(), "missing-interpreter")) + "\n"
			if runtime.GOOS == "windows" {
				body = "fixture: not a Windows executable image\n"
			}
			recordManifest(t, f.hooksDir, f.name, []byte(body))
			if err := os.WriteFile(binary, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			control := exec.Command(binary)
			if err := control.Run(); err == nil || control.ProcessState != nil {
				t.Fatalf("fixture must fail kernel startup without executing a child: err=%v state=%v", err, control.ProcessState)
			}
			exit, stdout, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG="}, args...)
			// Bash's unchanged exec failure status. Newer Bash reports an unusable
			// interpreter as 126 (found, not executable) where older Bash says 127.
			wants := []int{127, 126}
			switch runtime.GOOS {
			case "windows":
				wants = []int{216} // cmd's unchanged incompatible-image launch status.
			case "darwin":
				wants = []int{127, 126, 1} // macOS Bash 3.2 reports an unusable interpreter as 1.
			}
			switch args[1] {
			case "pre-tool":
				wants = []int{2}
				if !strings.Contains(stderr, "failing closed") {
					t.Errorf("startup failure must report the guard posture: %q", stderr)
				}
			}
			if !slices.Contains(wants, exit) || stdout != "" || stderr == "" {
				t.Fatalf("kernel startup failure posture: exit %d want one of %v stdout %q stderr %q", exit, wants, stdout, stderr)
			}
			if n := f.requests.Load(); n != 0 {
				t.Fatalf("present executable must not fetch: %d requests", n)
			}
		})
	}
}

func TestDispatcherNativeWindowsEventStatuses(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows child-status normalization; Unix successful exec preserves all child statuses")
	}
	root := t.TempDir()
	source := filepath.Join(root, "status.go")
	// This helper only emits arbitrary streams/statuses. It implements no Agentics
	// input, routing, or provider response behavior.
	program := `package main
import ("os"; "strconv"; "encoding/json")
func main() {
 if os.Getenv("AGX_TEST_CHILD_ARGV")!="" { json.NewEncoder(os.Stdout).Encode(os.Args[1:]); return }
 os.Stdout.WriteString(os.Getenv("AGX_TEST_CHILD_STDOUT"))
 os.Stderr.WriteString(os.Getenv("AGX_TEST_CHILD_STDERR"))
 status, err := strconv.Atoi(os.Getenv("AGX_TEST_CHILD_STATUS"))
 if err != nil { panic(err) }
 os.Exit(status)
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(root, "status.exe")
	if output, err := exec.Command("go", "build", "-o", helper, source).CombinedOutput(); err != nil {
		t.Fatalf("build native status fixture: %v\n%s", err, output)
	}
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("argv", func(t *testing.T) {
		f := newFetchFixture(t)
		recordManifest(t, f.hooksDir, f.name, data)
		if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), data, 0755); err != nil {
			t.Fatal(err)
		}
		args := []string{"claude-code", "command-guardian", "--mode", "block", "two words", "", "a !& b", "eight", "nine", "ten"}
		status, out, diag := f.dispatch(t, []string{"AGX_NO_FETCH=0", "AGX_DEBUG=", "AGX_TEST_CHILD_ARGV=1"}, args...)
		var got []string
		if err := json.Unmarshal([]byte(out), &got); err != nil || !slices.Equal(got, args) || status != 0 || diag != "" {
			t.Fatalf("native argv: %v %d %q %q", err, status, out, diag)
		}
	})
	check := func(t *testing.T, status, want int, childOut string, args ...string) {
		t.Helper()
		f := newFetchFixture(t)
		recordManifest(t, f.hooksDir, f.name, data)
		if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), data, 0o755); err != nil {
			t.Fatal(err)
		}
		diagnostic := "fixture stderr\n"
		exit, stdout, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG=", "AGX_TEST_CHILD_STDOUT=" + childOut, "AGX_TEST_CHILD_STDERR=" + diagnostic, "AGX_TEST_CHILD_STATUS=" + strconv.Itoa(status)}, args...)
		if exit != want {
			t.Fatalf("child status propagation: exit %d want %d stdout %q stderr %q", exit, want, stdout, stderr)
		}
		if args[0] == "antigravity" && args[1] == "pre-tool" && status != 0 {
			assertAntigravityFailClosedJSON(t, stdout, "agx: guard runtime unavailable - failing closed")
			if !strings.HasPrefix(stderr, diagnostic) || !strings.Contains(stderr, "failing closed") {
				t.Fatalf("antigravity guard normalization must retain child stderr and add posture: %q", stderr)
			}
			return
		}
		if stdout != childOut {
			t.Fatalf("child stdout propagation: stdout %q want %q stderr %q", stdout, childOut, stderr)
		}
		if args[1] == "pre-tool" && status != 0 && status != 2 {
			if !strings.HasPrefix(stderr, diagnostic) || !strings.Contains(stderr, "failing closed") {
				t.Fatalf("guard normalization must retain child stderr and add posture: %q", stderr)
			}
		} else if stderr != diagnostic {
			t.Fatalf("child stderr changed: got %q want %q", stderr, diagnostic)
		}
	}
	for _, provider := range []string{"claude-code", "codex", "copilot", "antigravity"} {
		for _, status := range []int{0, 1, 2, 7, 216} {
			t.Run(provider+"/pre-tool/"+strconv.Itoa(status), func(t *testing.T) {
				want := status
				childOut := "fixture stdout\n"
				if provider == "antigravity" {
					if status != 0 {
						want = 0
						childOut = ""
					}
				} else if status != 0 && status != 2 {
					want = 2
				}
				check(t, status, want, childOut, provider, "pre-tool")
			})
		}
	}
	for _, hook := range []string{"command-guardian", "release-guardian", "secrets-guardian"} {
		t.Run("legacy/"+hook, func(t *testing.T) { check(t, 216, 216, "fixture stdout\n", "claude-code", hook) })
		t.Run("legacy-antigravity/"+hook, func(t *testing.T) { check(t, 216, 216, "fixture stdout\n", "antigravity", hook) })
	}
}

// assertAntigravityFailClosedJSON verifies that stdout is exactly one JSON line
// carrying decision "deny" and the expected reason string.
func assertAntigravityFailClosedJSON(t *testing.T, stdout, wantReason string) {
	t.Helper()
	trimmed := strings.TrimRight(stdout, "\r\n")
	if strings.ContainsAny(trimmed, "\r\n") {
		t.Fatalf("antigravity fail-closed stdout must be single-line JSON, got %q", stdout)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		t.Fatalf("antigravity fail-closed stdout is not valid JSON: %v; stdout=%q", err, stdout)
	}
	if doc["decision"] != "deny" || doc["reason"] != wantReason || len(doc) != 2 {
		t.Fatalf("antigravity fail-closed JSON = %v, want decision=deny reason=%q", doc, wantReason)
	}
}

func TestDispatcherAntigravityMissingBinaryFailsClosedJSON(t *testing.T) {
	wantReason := "agx: guard runtime unavailable - failing closed"
	if runtime.GOOS == "windows" {
		wantReason = "agx: guard binary missing - failing closed"
	}
	for _, hook := range []string{"pre-tool", "command-guardian", "release-guardian", "secrets-guardian", "unknown-hook"} {
		t.Run(hook, func(t *testing.T) {
			f := newFetchFixture(t)
			exit, stdout, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG="}, "antigravity", hook)
			closed := hook == "pre-tool" || strings.HasSuffix(hook, "guardian")
			if closed {
				if exit != 0 {
					t.Fatalf("%s must fail closed at exit 0 for antigravity, got %d\nstdout: %q\nstderr: %q", hook, exit, stdout, stderr)
				}
				assertAntigravityFailClosedJSON(t, stdout, wantReason)
				if !strings.Contains(stderr, wantReason) {
					t.Fatalf("%s stderr must carry the fail-closed diagnostic, got %q", hook, stderr)
				}
			} else if exit != 0 || stdout != "" || stderr != "" {
				t.Fatalf("%s must fail open silently: exit %d stdout %q stderr %q", hook, exit, stdout, stderr)
			}
			if n := f.requests.Load(); n != 0 {
				t.Fatalf("no-fetch made %d requests", n)
			}
		})
	}
}

func TestDispatcherAntigravityFailedFetchFailsClosedJSON(t *testing.T) {
	wantReason := "agx: guard runtime unavailable - failing closed"
	if runtime.GOOS == "windows" {
		wantReason = "agx: guard binary missing - failing closed"
	}
	f := newFetchFixture(t)
	f.tamperManifest(t)
	for _, guard := range []string{"pre-tool", "command-guardian", "release-guardian", "secrets-guardian"} {
		t.Run(guard, func(t *testing.T) {
			exit, stdout, stderr := f.dispatch(t, nil, "antigravity", guard)
			if exit != 0 {
				t.Fatalf("%s: a rejected download must fail closed at exit 0 for antigravity, got %d\nstdout: %q\nstderr: %q", guard, exit, stdout, stderr)
			}
			assertAntigravityFailClosedJSON(t, stdout, wantReason)
			if !strings.Contains(stderr, "does not match hooks/SHA256SUMS") || !strings.Contains(stderr, "failing closed") {
				t.Fatalf("%s: stderr must report discard and fail-closed posture:\n%s", guard, stderr)
			}
			if entries := f.cacheEntries(t); len(entries) != 0 {
				t.Fatalf("%s: a rejected download must leave nothing behind, got %v", guard, entries)
			}
		})
	}
}

func TestDispatcherAntigravityKernelStartupFailureFailsClosedJSON(t *testing.T) {
	f := newFetchFixture(t)
	binary := filepath.Join(f.hooksDir, "bin", f.name)
	body := "#!/" + filepath.ToSlash(filepath.Join(t.TempDir(), "missing-interpreter")) + "\n"
	if runtime.GOOS == "windows" {
		body = "fixture: not a Windows executable image\n"
	}
	recordManifest(t, f.hooksDir, f.name, []byte(body))
	if err := os.WriteFile(binary, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	exit, stdout, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG="}, "antigravity", "pre-tool")
	if exit != 0 {
		t.Fatalf("antigravity kernel startup failure must exit 0, got %d\nstdout: %q\nstderr: %q", exit, stdout, stderr)
	}
	assertAntigravityFailClosedJSON(t, stdout, "agx: guard runtime unavailable - failing closed")
	if !strings.Contains(stderr, "agx: guard runtime unavailable - failing closed") {
		t.Fatalf("antigravity kernel startup failure must report guard posture on stderr: %q", stderr)
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("present executable must not fetch: %d requests", n)
	}
}

func TestDispatcherAntigravityUnixNonZeroChildFailsClosedJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix bash-half child-status normalization; Windows cmd half is tested in TestDispatcherNativeWindowsEventStatuses")
	}
	for _, status := range []int{0, 1, 2, 7} {
		t.Run("status-"+strconv.Itoa(status), func(t *testing.T) {
			f := newFetchFixture(t)
			body := "#!/bin/sh\n"
			if status == 0 {
				body += "printf '{\"decision\":\"allow\"}\\n'\n"
			}
			body += "printf 'child stderr\\n' >&2\nexit " + strconv.Itoa(status) + "\n"
			recordManifest(t, f.hooksDir, f.name, []byte(body))
			if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			exit, stdout, stderr := f.dispatch(t, []string{"AGX_NO_FETCH=1", "AGX_DEBUG="}, "antigravity", "pre-tool")
			if exit != 0 {
				t.Fatalf("antigravity pre-tool exit=%d want 0 (stdout=%q stderr=%q)", exit, stdout, stderr)
			}
			if status == 0 {
				if stdout != "{\"decision\":\"allow\"}\n" || stderr != "child stderr\n" {
					t.Fatalf("status 0 streams altered: stdout=%q stderr=%q", stdout, stderr)
				}
				return
			}
			assertAntigravityFailClosedJSON(t, stdout, "agx: guard runtime unavailable - failing closed")
			if !strings.HasPrefix(stderr, "child stderr\n") || !strings.Contains(stderr, "agx: guard runtime unavailable - failing closed") {
				t.Fatalf("non-zero child stderr must be retained alongside posture: %q", stderr)
			}
		})
	}
}

func TestDispatcherFailsClosedWhenReleaseIsAbsent(t *testing.T) {
	f := newFetchFixture(t)
	plugin := filepath.Join(f.hooksDir, "..", ".claude-plugin", "plugin.json")
	if err := os.WriteFile(plugin, []byte(`{"name": "agentics", "version": "8.8.8"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, _, stderr := f.dispatch(t, nil, "claude-code", "command-guardian")
	if exit != 2 || !strings.Contains(stderr, "failing closed") {
		t.Fatalf("a missing release must fail closed at exit 2, got %d\n%s", exit, stderr)
	}
	if entries := f.cacheEntries(t); len(entries) != 0 {
		t.Fatalf("a failed download must leave nothing behind, got %v", entries)
	}
}

// A manifest line that appears twice must still resolve to one hash. The Unix
// half reads the manifest with awk, which prints every match unless told to
// stop, and two hashes joined by a newline can never equal one.
func TestDispatcherToleratesDuplicateManifestEntry(t *testing.T) {
	f := newFetchFixture(t)
	path := filepath.Join(f.hooksDir, "SHA256SUMS")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var dup string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasSuffix(line, "bin/"+f.name) {
			dup = line
		}
	}
	if dup == "" {
		t.Fatalf("hooks/SHA256SUMS has no entry for %s; the checkout is inconsistent", f.name)
	}
	if err := os.WriteFile(path, append(data, []byte(dup+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, _, stderr := f.dispatch(t, nil, "claude-code", "command-guardian")
	if exit != 1 || strings.Contains(stderr, "does not match") {
		t.Fatalf("a duplicated manifest line must not reject correct bytes: exit %d\n%s", exit, stderr)
	}
	if entries := f.cacheEntries(t); len(entries) != 1 || entries[0] != f.name {
		t.Fatalf("cache should hold only the binary, got %v", entries)
	}
}

// Without a SHA-256 tool the Unix half cannot check a download, so it must
// not start one, and it must not report a mismatch it never measured.
func TestDispatcherStaysOfflineWithoutSHA256Tool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fetch-hook.ps1 hashes through .NET SHA256, which every Windows PowerShell carries")
	}
	f := newFetchFixture(t)
	// PATH holds only what the dispatcher needs besides a hash tool, so
	// `command -v` finds neither sha256sum nor shasum.
	bin := t.TempDir()
	for _, tool := range []string{"bash", "curl", "awk", "sed", "head", "mkdir", "rm", "chmod", "mv", "dirname", "basename"} {
		var p string
		if tool == "bash" {
			p = hookutiltest.RequireInterpreter(t, tool)
		} else {
			var err error
			if p, err = exec.LookPath(tool); err != nil {
				t.Skipf("%s not available", tool)
			}
		}
		if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	exit, _, stderr := f.dispatch(t, []string{"PATH=" + bin}, "claude-code", "command-guardian")
	if exit != 2 || !strings.Contains(stderr, "failing closed") {
		t.Fatalf("the guard must fail closed at exit 2, got %d\n%s", exit, stderr)
	}
	if strings.Contains(stderr, "does not match") || strings.Contains(stderr, "command not found") {
		t.Fatalf("no checksum was computed, so none may be reported:\n%s", stderr)
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("a host that cannot verify must not download, saw %d requests", n)
	}
	if entries := f.cacheEntries(t); len(entries) != 0 {
		t.Fatalf("nothing may be written, got %v", entries)
	}
}

func TestDispatcherInterruptedFetchRetries(t *testing.T) {
	f := newFetchFixture(t)
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v"+fetchFixtureVersion+"/"+f.name {
			http.NotFound(w, r)
			return
		}
		// An incomplete response cannot become the final runtime.
		w.Header().Set("Content-Length", strconv.Itoa(len(f.binary)))
		if attempts.Add(1) == 1 {
			w.Write(f.binary[:16])
			return
		}
		w.Write(f.binary)
	}))
	defer server.Close()
	env := []string{"AGX_RELEASE_URL=" + server.URL, "AGX_DEBUG="}
	s, o, d := f.dispatch(t, env, "--provision-only")
	if s == 0 || o != "" || d == "" || len(f.cacheEntries(t)) != 0 {
		t.Fatalf("partial response: %d %q %q leftovers %v", s, o, d, f.cacheEntries(t))
	}
	s, o, d = f.dispatch(t, env, "--provision-only")
	if s != 0 || strings.TrimRight(o, "\r\n") != f.cached() || attempts.Load() != 2 {
		t.Fatalf("retry: %d %q %q attempts %d", s, o, d, attempts.Load())
	}
}

func TestDispatcherRejectsCacheAliases(t *testing.T) {
	for _, component := range []string{"slug", "version", "digest", "asset"} {
		t.Run(component, func(t *testing.T) {
			f := newFetchFixture(t)
			alias := f.cached()
			for _, level := range []string{"asset", "digest", "version"} {
				if component == level {
					break
				}
				alias = filepath.Dir(alias)
			}
			if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			// Even valid bytes reachable through an alias must be refused.
			rel, err := filepath.Rel(alias, f.cached())
			if err != nil {
				t.Fatal(err)
			}
			if rel != "." {
				outside := filepath.Join(target, rel)
				if err := os.MkdirAll(filepath.Dir(outside), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(outside, f.binary, 0755); err != nil {
					t.Fatal(err)
				}
			} else if runtime.GOOS != "windows" {
				target = filepath.Join(target, "runtime")
				if err := os.WriteFile(target, f.binary, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if runtime.GOOS == "windows" {
				// Junctions do not require the optional Windows symlink privilege.
				cmd := exec.Command("cmd", "/d", "/c", "mklink", "/J", "alias", target)
				cmd.Dir = filepath.Dir(alias)
				if b, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("junction: %v: %s", err, b)
				}
				if err := os.Rename(filepath.Join(filepath.Dir(alias), "alias"), alias); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(target, alias); err != nil {
				t.Fatal(err)
			}
			s, o, d := f.dispatch(t, nil, "--provision-only")
			if s == 0 || o != "" || d == "" || f.requests.Load() != 0 {
				t.Fatalf("cache alias accepted or fetched: %d %q %q requests %d", s, o, d, f.requests.Load())
			}
		})
	}
}

func TestDispatcherUnavailableCacheAndUnsafeMetadata(t *testing.T) {
	f := newFetchFixture(t)
	for _, env := range [][]string{{"LOCALAPPDATA=", "XDG_CACHE_HOME=", "HOME="}, {"LOCALAPPDATA=relative", "XDG_CACHE_HOME=relative", "HOME=relative"}} {
		s, o, d := f.dispatch(t, env, "--provision-only")
		if s == 0 || o != "" || !strings.Contains(d, "cache") || f.requests.Load() != 0 {
			t.Fatalf("unusable user cache: %d %q %q", s, o, d)
		}
	}
	plugin := filepath.Join(f.hooksDir, "..", ".claude-plugin", "plugin.json")
	if err := os.WriteFile(plugin, []byte("{\"version\":\"../../escape\"}"), 0644); err != nil {
		t.Fatal(err)
	}
	s, o, d := f.dispatch(t, nil, "--provision-only")
	if s == 0 || o != "" || d == "" || f.requests.Load() != 0 {
		t.Fatalf("unsafe version: %d %q %q", s, o, d)
	}
}

func TestDispatcherIgnoresOtherRuntimeIdentities(t *testing.T) {
	f := newFetchFixture(t)
	validDir := filepath.Dir(f.cached())
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(filepath.Dir(validDir)), "1.0.0", filepath.Base(validDir), f.name),
		filepath.Join(filepath.Dir(validDir), strings.Repeat("0", 64), f.name),
		filepath.Join(validDir, "."+f.name+".part.orphan"),
		filepath.Join(f.hooksDir, f.name),
	} {
		if err := os.MkdirAll(filepath.Dir(candidate), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(candidate, f.binary, 0755); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"PATH=" + f.hooksDir + string(os.PathListSeparator) + os.Getenv("PATH"), "AGX_NO_FETCH=0"}
	s, o, d := f.dispatch(t, env, "--provision-only")
	if s == 0 || o != "" || !strings.Contains(d, "agx:") || f.requests.Load() != 0 {
		t.Fatalf("unrelated runtime accepted: %d %q %q", s, o, d)
	}
}

func TestDispatcherVerifiedGuardReadsStdin(t *testing.T) {
	f := newFetchFixture(t)
	payloadDir := filepath.Join(hosttest.Checkout(t), "hooks", "go", "surfaces", "claude-code", "payloads")
	env := []string{"CLAUDE_PLUGIN_ROOT=" + filepath.Dir(f.hooksDir), "CLAUDE_PLUGIN_DATA=" + t.TempDir(), "AGX_DEBUG="}
	for _, tc := range []struct {
		file   string
		status int
	}{{"pre-tool-use.json", 0}, {"pre-tool-use-dangerous.json", 2}} {
		data, err := os.ReadFile(filepath.Join(payloadDir, tc.file))
		if err != nil {
			t.Fatal(err)
		}
		f.stdin = string(data)
		s, o, d := f.dispatch(t, env, "claude-code", "command-guardian", "--mode", "block")
		if s != tc.status || o != "" || (s == 0 && d != "") || (s == 2 && !strings.Contains(d, "destructive_file_ops")) {
			t.Fatalf("guard for %s: %d %q %q", tc.file, s, o, d)
		}
	}
	if f.requests.Load() != 1 {
		t.Fatalf("cold then warm guard: %d requests", f.requests.Load())
	}
}

func TestDispatcherUsesSystemVerificationTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows PowerShell hashes using .NET")
	}
	f := newFetchFixture(t)
	stub := t.TempDir()
	for _, tool := range []string{"sha256sum", "shasum", "awk", "sed", "mkdir", "mktemp", "chmod", "mv", "rm", "curl"} {
		if err := os.WriteFile(filepath.Join(stub, tool), []byte("#!/bin/sh\nprintf called > \"$AGX_TEST_TOOL_MARKER\"\nexit 1\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(stub, "marker")
	s, o, d := f.dispatch(t, []string{"PATH=" + stub + ":" + os.Getenv("PATH"), "AGX_TEST_TOOL_MARKER=" + marker}, "--provision-only")
	if s != 0 || strings.TrimRight(o, "\r\n") != f.cached() {
		t.Fatalf("provision with shadow tools: %d %q %q", s, o, d)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shadow verification tool ran: %v", err)
	}
}

func TestDispatcherLockedPreseedUsesVerifiedCache(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows exclusive file sharing")
	}
	f := newFetchFixture(t)
	if err := os.MkdirAll(filepath.Dir(f.cached()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.cached(), f.binary, 0755); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(f.hooksDir, "bin", f.name)
	if err := os.WriteFile(installed, f.binary, 0755); err != nil {
		t.Fatal(err)
	}
	ps := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	lock := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference='Stop'; $f=[IO.File]::Open($env:AGX_TEST_PRESEED,'Open','Read','None'); [Console]::WriteLine('ready'); [void][Console]::ReadLine(); $f.Dispose()")
	lock.Env = append(os.Environ(), "AGX_TEST_PRESEED="+installed)
	done, err := lock.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	ready, err := lock.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		done.Close()
		if err := lock.Wait(); err != nil {
			t.Error(err)
		}
	}()
	line, err := bufio.NewReader(ready).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("lock preseed: %q, %v", line, err)
	}
	s, o, d := f.dispatch(t, []string{"AGX_NO_FETCH=0"}, "--provision-only")
	if s != 0 || strings.TrimRight(o, "\r\n") != f.cached() || f.requests.Load() != 0 {
		t.Fatalf("locked preseed: %d %q %q", s, o, d)
	}
}
