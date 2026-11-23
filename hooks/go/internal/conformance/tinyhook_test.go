package conformance

// The tiny hook module is a minimal stand-in for agentics's hooks/ Go module.
// It honors the pieces of the hook contract the checks observe:
//
//   - the argv truth table (claude-code fails closed at 1, copilot open at 0,
//     codex open at 0 with a `{}` body, antigravity open at 0 with a
//     `{"decision":"allow"}` body; bad argv exits 1),
//   - a main.sourceDigest variable for the Makefile-style -X stamp,
//   - a cmd/launcher package built into bin/agx.exe, beside an LF bin/agx and
//     the .gitattributes line the bin-launchers rule requires.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// tinyGoDirective returns "go " followed by the version the REAL
// hooks/go/go.mod pins. The tiny fixture binaries are cross-compiled by the
// host toolchain, and module-rebuild-toolchain / freshness-toolchain-drift
// compare that toolchain's build info against the fixture's own `go`
// directive — a literal here that drifts from the real pin would fire those
// checks on every fixture for a reason that has nothing to do with the
// scenario under test.
func tinyGoDirective(t *testing.T) string {
	t.Helper()
	pin, err := hookprovenance.PinnedToolchain(filepath.Join(hosttest.Checkout(t), "hooks", "go"))
	if err != nil {
		t.Fatalf("resolving the real hooks/go toolchain pin: %v", err)
	}
	return "go " + strings.TrimPrefix(pin, "go")
}

// tinyGoModFor is the tiny module's go.mod, with the `go` directive kept in
// lockstep with the real hooks/go/go.mod pin (see tinyGoDirective).
func tinyGoModFor(t *testing.T) string {
	t.Helper()
	return "module example.com/tinyhook\n\n" + tinyGoDirective(t) + "\n"
}

const tinyProviderGo = `package provider
`

const tinyMainGo = `package main

import (
	"fmt"
	"io"
	"os"
)

var sourceDigest = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is main's testable core, the same split the real binary makes.
func run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	// A real read keeps the -X target alive through linker deadcode.
	if sourceDigest == "" {
		return 3
	}
	providers := map[string]bool{"claude-code": true, "copilot": true, "codex": true, "antigravity": true}
	hooks := map[string]bool{"command-guardian": true, "secrets-guardian": true, "release-guardian": true}
	if len(args) < 2 || !providers[args[0]] || !hooks[args[1]] {
		fmt.Fprintln(stderr, "usage")
		return 1
	}
	switch args[0] {
	case "claude-code":
		return 1
	case "copilot":
		return 0
	case "codex":
		fmt.Fprintln(stdout, "{}")
	case "antigravity":
		fmt.Fprintln(stdout, "{\"decision\":\"allow\"}")
	}
	return 0
}
`

// tinyLauncherGo stands in for the agx launcher. It has no test file and no
// statement inside a function body, so the tiny module's coverage total stays
// what tinyMainTestGo measures. The package-level read keeps the -X target
// alive through linker deadcode; an expression the compiler folds away, such
// as len(sourceDigest), would let the linker drop the stamp.
const tinyLauncherGo = `package main

var sourceDigest = "dev"

var stamp = []byte(sourceDigest)

func main() {}
`

// tinyLauncherScript is the LF shell launcher committed as bin/agx.
const tinyLauncherScript = "#!/bin/sh\nexec true\n"

// tinyGitAttributes marks the launcher binary the way the repository does.
const tinyGitAttributes = "* text=auto eol=lf\nbin/agx.exe binary linguist-generated\n"

// tinyMainTestGo covers run() so the tiny module clears the coverage floor
// the module gates enforce, the same obligation the real module carries.
const tinyMainTestGo = `package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	cases := []struct {
		args   []string
		exit   int
		stdout string
	}{
		{[]string{"claude-code", "command-guardian"}, 1, ""},
		{[]string{"copilot", "secrets-guardian"}, 0, ""},
		{[]string{"codex", "release-guardian"}, 0, "{}\n"},
		{[]string{"antigravity", "command-guardian"}, 0, "{\"decision\":\"allow\"}\n"},
		{[]string{"bogus"}, 1, ""},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, strings.NewReader(""), &stdout, &stderr); code != c.exit || stdout.String() != c.stdout {
			t.Errorf("run(%v) = %d %q, want %d %q", c.args, code, stdout.String(), c.exit, c.stdout)
		}
	}
	saved := sourceDigest
	sourceDigest = ""
	if code := run(nil, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 3 {
		t.Errorf("an unstamped binary must exit 3, got %d", code)
	}
	sourceDigest = saved
}
`

// rebuildTinyHost rebuilds only the HOST contract binary of a cloned tiny
// repo from its (possibly mutated) source, stamped with the given digest
// value; an empty stamp builds without ldflags — the pre-digest-stamping
// state.
func rebuildTinyHost(t *testing.T, root, stamp string) {
	t.Helper()
	hooksDir := filepath.Join(root, engineRel)
	name := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
	if name == "" {
		t.Skip("host has no contract binary target")
	}
	target := hookbuild.ContractTargets[name]
	dest := filepath.Join(root, "hooks", "bin", name)
	args := []string{"build", "-trimpath", "-o", dest, "./cmd/hook"}
	if stamp != "" {
		args = hookbuild.BuildArgs(stamp, dest)
	}
	env := append([]string{"CGO_ENABLED=0", "GOOS=" + target[0], "GOARCH=" + target[1]}, hookbuild.ArchTuningEnv...)
	out, err := runToolEnv(hooksDir,
		append(os.Environ(), env...),
		"go", args...)
	if err != nil {
		t.Fatalf("rebuilding tiny host binary: %v\n%s", err, out)
	}
}

// tinyRepoTemplate builds the pristine tiny repo (module + correctly stamped
// binaries) once per test binary; scenarios copy it and mutate the copy.
var tinyRepoOnce sync.Once
var tinyRepoDir string
var tinyRepoErr error

func pristineTinyRepo(t *testing.T) string {
	t.Helper()
	tinyRepoOnce.Do(func() {
		dir, err := os.MkdirTemp("", "tiny-hook-template.")
		if err != nil {
			tinyRepoErr = err
			return
		}
		tinyRepoDir = dir
		// The template lives for the whole test binary; TestMain cleans it.
		writeTinyModuleRaw(t, dir)
		digest, err := hookprovenance.SourceDigest(filepath.Join(dir, engineRel))
		if err != nil {
			tinyRepoErr = err
			return
		}
		tinyRepoErr = stampTinyBinariesRaw(dir, "agentics-src-"+digest)
	})
	if tinyRepoErr != nil {
		t.Fatalf("tiny repo template: %v", tinyRepoErr)
	}
	return tinyRepoDir
}

func writeTinyModuleRaw(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"hooks/go/go.mod":                        tinyGoModFor(t),
		"hooks/go/cmd/hook/main.go":              tinyMainGo,
		"hooks/go/cmd/hook/main_test.go":         tinyMainTestGo,
		"hooks/go/internal/provider/provider.go": tinyProviderGo,
		"hooks/go/cmd/launcher/main.go":          tinyLauncherGo,
		"bin/agx":                                tinyLauncherScript,
		".gitattributes":                         tinyGitAttributes,
		// Pure-ASCII dispatcher so the run-hook-ascii rule (part of Audit) is
		// satisfied; .go-only digest hashing means this file never disturbs
		// the stamped digest.
		"hooks/run-hook.cmd": "@echo off\nREM tiny dispatcher stand-in - ASCII only\nexit /b 0\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(content), 0o644)
	}
}

func stampTinyBinariesRaw(root, stamp string) error {
	hooksDir := filepath.Join(root, engineRel)
	binDir := filepath.Join(root, "hooks", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	for name, target := range hookbuild.ContractTargets {
		env := append([]string{"CGO_ENABLED=0", "GOOS=" + target[0], "GOARCH=" + target[1]}, hookbuild.ArchTuningEnv...)
		if _, err := runToolEnv(hooksDir,
			append(os.Environ(), env...),
			"go", hookbuild.BuildArgs(stamp, filepath.Join(binDir, name))...); err != nil {
			return err
		}
	}
	// The launcher is re-stamped with the binaries, so a re-stamp never
	// leaves it answering for an older digest.
	env := append([]string{"CGO_ENABLED=0", "GOOS=" + hookbuild.LauncherTarget[0], "GOARCH=" + hookbuild.LauncherTarget[1]}, hookbuild.ArchTuningEnv...)
	launcher := filepath.Join(root, filepath.FromSlash(hookbuild.LauncherOutput))
	if out, err := runToolEnv(hooksDir, append(os.Environ(), env...), "go", hookbuild.LauncherBuildArgs(stamp, launcher)...); err != nil {
		return fmt.Errorf("building the tiny launcher: %w\n%s", err, out)
	}
	return writeTinyManifest(root)
}

// writeTinyManifest records the tiny binaries in hooks/SHA256SUMS the way
// hooks/go/Makefile does, so a pristine clone passes the manifest verdict.
func writeTinyManifest(root string) error {
	var lines []string
	for _, name := range hookbuild.ContractBinaries {
		data, err := os.ReadFile(filepath.Join(root, "hooks", "bin", name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, hex.EncodeToString(sum[:])+"  bin/"+name)
	}
	return os.WriteFile(filepath.Join(root, "hooks", "SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// cloneTinyRepo copies the pristine template into a fresh temp dir the test
// may mutate freely.
func cloneTinyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := copyTree(pristineTinyRepo(t), root); err != nil {
		t.Fatal(err)
	}
	return root
}

// mutateTinyFile rewrites one file in a cloned tiny repo via replacement,
// failing if the target substring is absent.
func mutateTinyFile(t *testing.T, root, rel, old, new string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("mutation target %q not found in %s", old, rel)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), old, new)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	if tinyRepoDir != "" {
		os.RemoveAll(tinyRepoDir)
	}
	os.Exit(code)
}
