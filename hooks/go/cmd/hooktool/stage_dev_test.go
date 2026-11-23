package main

import (
	"debug/pe"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

func TestStageDevOutputAdmission(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hooks", "go"), map[string]string{"go.mod": "module fixture\n\ngo 1.27.4\n"})
	existing := t.TempDir()
	for _, tc := range []struct{ out, message string }{
		{root, "outside the checkout"},
		{filepath.Join(root, "hooks", "bin"), "outside the checkout"},
		{existing, "creating new stage-dev output"},
		{"relative", "absolute repo and out"},
	} {
		err := run([]string{"stage-dev", "--repo", root, "--out", tc.out})
		if err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Errorf("stage-dev --out %q: expected %q, got %v", tc.out, tc.message, err)
		}
	}
}

func TestStageDevCheckoutIdentity(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "LocalCheckout")
	module := filepath.Join(repo, "hooks", "go")
	writeFixture(t, module, map[string]string{
		"go.mod":               "module fixture\n\ngo 1.27.4\n",
		"cmd/hook/main.go":     "package main\nfunc main(){}\n",
		"cmd/launcher/main.go": "package main\nfunc main(){}\n",
	})
	reject := func(t *testing.T, out string) {
		t.Helper()
		if err := run([]string{"stage-dev", "--repo", repo, "--out", out}); err == nil || !strings.Contains(err.Error(), "outside the checkout") {
			t.Fatalf("output inside the checkout at %q: got %v", out, err)
		}
	}
	t.Run("nested existing parent", func(t *testing.T) {
		out := filepath.Join(module, "cmd", "hook", "output")
		reject(t, out)
		if _, err := os.Lstat(out); !os.IsNotExist(err) {
			t.Fatalf("refused output: %v", err)
		}
	})
	t.Run("checkout itself through a directory link", func(t *testing.T) {
		link := filepath.Join(base, "linked-checkout")
		if runtime.GOOS == "windows" {
			hookutiltest.JunctionOrSkip(t, repo, link)
		} else {
			hookutiltest.SymlinkOrSkip(t, repo, link)
		}
		reject(t, link)
	})
	t.Run("case alias above output parent", func(t *testing.T) {
		alias := filepath.Join(base, "localcheckout")
		aliasInfo, err := os.Stat(alias)
		if os.IsNotExist(err) {
			t.Skip("temporary volume is case-sensitive")
		}
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(repo)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(info, aliasInfo) {
			t.Fatalf("%q does not refer to the fixture checkout", alias)
		}
		out := filepath.Join(alias, "hooks", "go", "cmd", "output")
		reject(t, out)
		if _, err := os.Lstat(out); !os.IsNotExist(err) {
			t.Fatalf("refused output: %v", err)
		}
	})
	t.Run("sibling", func(t *testing.T) {
		out := filepath.Join(base, "LocalCheckout-build")
		// The standalone command sets its build environment for the process.
		cmd := exec.Command("go", "run", ".", "stage-dev", "--repo", repo, "--out", out)
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GONOPROXY=none", "GOVCS=*:off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sibling output: %v\n%s", err, output)
		}
		if _, err := os.Stat(filepath.Join(out, "hooks", "SHA256SUMS")); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStageDevRejectsDifferentMachine(t *testing.T) {
	source := t.TempDir()
	writeFixture(t, source, map[string]string{
		"go.mod":               "module fixture\n\ngo 1.27.4\n",
		"cmd/launcher/main.go": "package main\nfunc main(){}\n",
	})
	launcher := filepath.Join(t.TempDir(), filepath.Base(hookbuild.LauncherOutput))
	rebuild(t, source, filepath.Dir(launcher), nil, true)
	check := func() error { return checkDevBinary(launcher, hookbuild.LauncherTarget, runtime.Version()) }
	if err := check(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(launcher)
	if err != nil {
		t.Fatal(err)
	}
	offset := binary.LittleEndian.Uint32(data[0x3c:]) + 4
	binary.LittleEndian.PutUint16(data[offset:], pe.IMAGE_FILE_MACHINE_ARM64)
	if err := os.WriteFile(launcher, data, 0755); err != nil {
		t.Fatal(err)
	}
	if err := check(); err == nil || !strings.Contains(err.Error(), "executable header does not match windows/amd64") {
		t.Fatalf("incorrect machine with original Go metadata: got %v", err)
	}
}
