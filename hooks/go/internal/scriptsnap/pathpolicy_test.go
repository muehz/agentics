package scriptsnap

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// dirLink makes link a directory link to target: a junction on Windows, which needs no privilege and is the form
// EvalSymlinks cannot follow, and a symlink elsewhere.
func dirLink(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		hookutiltest.JunctionOrSkip(t, target, link)
		return
	}
	hookutiltest.SymlinkOrSkip(t, target, link)
}

func awsPolicy(p string) string {
	if strings.Contains(filepath.ToSlash(p), "/.aws/") {
		return "protected"
	}
	return ""
}

func mkAws(t *testing.T) (root, aws string) {
	t.Helper()
	root = t.TempDir()
	aws = filepath.Join(root, ".aws")
	if err := os.Mkdir(aws, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, aws
}

func TestScriptBehindADirectoryLinkIsReadNormally(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.sh"), []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	dirLink(t, real, link)
	got := New(WithPathPolicy(awsPolicy)).ReadScript(filepath.Join(link, "a.sh"))
	if got.Limit != shellwords.RuleNone || got.Text != "echo x\n" {
		t.Fatalf("%+v", got)
	}
}

// The link is followed whether or not the final file exists, and however deep the missing tail is, so the protected
// target is seen before any open. A junction is the case EvalSymlinks cannot resolve on Windows.
func TestPolicySeesThroughADirectoryLinkBeforeTheOpen(t *testing.T) {
	root, aws := mkAws(t)
	if err := os.WriteFile(filepath.Join(aws, "x.sh"), []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	dirLink(t, aws, link)
	touched := 0
	setHooks(t, 0, func() { touched++ }, func() { touched++ }, nil)
	for _, rel := range []string{"x.sh", "missing.sh", filepath.Join("sub", "missing.sh"), filepath.Join("sub", "deeper", "missing.sh")} {
		got := New(WithPathPolicy(awsPolicy)).ReadScript(filepath.Join(link, rel))
		if got.Limit != shellwords.RulePathProtected || got.Text != "" {
			t.Errorf("%s: %+v", rel, got)
		}
	}
	if touched != 0 {
		t.Fatalf("filesystem touched %d times", touched)
	}
}

func TestPolicySeesALinkChainBeforeTheOpen(t *testing.T) {
	root, aws := mkAws(t)
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	dirLink(t, aws, first)
	dirLink(t, first, second)
	got := New(WithPathPolicy(awsPolicy)).ReadScript(filepath.Join(second, "missing.sh"))
	if got.Limit != shellwords.RulePathProtected {
		t.Fatalf("%+v", got)
	}
}

// A link swapped in after the first check, to a directory holding a hardlink of the file about to be opened, is
// caught from the path the open handle ends up with.
func TestPolicySeesALinkSwappedBeforeTheOpen(t *testing.T) {
	root, aws := mkAws(t)
	plain := filepath.Join(root, "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(plain, "x.sh")
	if err := os.WriteFile(file, []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(aws, "x.sh")); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	link := filepath.Join(root, "link")
	dirLink(t, plain, link)
	setHooks(t, 0, func() {
		if err := os.Remove(link); err != nil {
			t.Error(err)
			return
		}
		dirLink(t, aws, link)
	}, nil, nil)
	got := New(WithPathPolicy(awsPolicy)).ReadScript(filepath.Join(link, "x.sh"))
	if got.Limit != shellwords.RulePathProtected || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func secretPolicy(p string) string {
	if strings.Contains(filepath.ToSlash(p), "/secret/") {
		return "protected"
	}
	return ""
}

func TestPathPolicyRunsBeforeAnyRead(t *testing.T) {
	s := New(WithPathPolicy(secretPolicy))
	got := s.ReadScript(filepath.Join(t.TempDir(), "secret", "missing.sh")) // does not exist: the policy still wins
	if got.Limit != shellwords.RulePathProtected || got.Detail != "protected or credential path" {
		t.Fatalf("%+v", got)
	}
}

func TestPathPolicyTouchesNothingOnAnExistingProtectedFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secret")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "x.sh")
	if err := os.WriteFile(p, []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touched := 0
	setHooks(t, 0, func() { touched++ }, func() { touched++ }, nil)
	got := New(WithPathPolicy(secretPolicy)).ReadScript(p)
	if got.Limit != shellwords.RulePathProtected || got.Text != "" || got.SHA256 != "" || touched != 0 {
		t.Fatalf("touched=%d %+v", touched, got)
	}
}

func TestPathPolicyIsOptional(t *testing.T) {
	p := write(t, "a.sh", []byte("echo x\n"))
	if got := New().ReadScript(p); got.Limit != shellwords.RuleNone {
		t.Fatalf("%+v", got)
	}
	if got := New(WithPathPolicy(nil)).ReadScript(p); got.Limit != shellwords.RuleNone {
		t.Fatalf("nil policy: %+v", got)
	}
}

func TestPathPolicyVerdictIsFinal(t *testing.T) {
	calls := 0
	s := New(WithPathPolicy(func(string) string { calls++; return "protected" }))
	p := filepath.Join(t.TempDir(), "a.sh")
	first := s.ReadScript(p)
	second := s.ReadScript(p)
	if first != second || first.Limit != shellwords.RulePathProtected {
		t.Fatalf("%+v %+v", first, second)
	}
	if calls != 1 {
		t.Fatalf("policy ran %d times for one path", calls)
	}
}

func TestPathPolicySeesTheResolvedPath(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "x.sh"), []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	touched := 0
	setHooks(t, 0, func() { touched++ }, nil, nil)
	got := New(WithPathPolicy(secretPolicy)).ReadScript(filepath.Join(link, "x.sh"))
	if got.Limit != shellwords.RulePathProtected || touched != 0 {
		t.Fatalf("touched=%d %+v", touched, got)
	}
}

// A directory link swapped after the first check, to a directory holding a hardlink of the same file, names the same
// file as the open handle, so only the policy run on the path as it resolves after the open can refuse it.
func TestPathPolicySeesAPathSwappedAfterTheOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the handle names the file it opened, so a later swap changes nothing; the swap before the open is pinned instead")
	}
	dir := t.TempDir()
	plain, secret := filepath.Join(dir, "plain"), filepath.Join(dir, "secret")
	for _, d := range []string{plain, secret} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(plain, "x.sh")
	if err := os.WriteFile(file, []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(secret, "x.sh")); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(plain, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	setHooks(t, 0, nil, func() {
		if err := os.Remove(link); err != nil {
			t.Error(err)
			return
		}
		if err := os.Symlink(secret, link); err != nil {
			t.Error(err)
		}
	}, nil)
	got := New(WithPathPolicy(secretPolicy)).ReadScript(filepath.Join(link, "x.sh"))
	if got.Limit != shellwords.RulePathProtected || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestResolvedPathNamingAnotherFileIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the handle names the file it opened, so there is no path to compare")
	}
	dir := t.TempDir()
	one, two := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	for _, d := range []string{one, two} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "x.sh"), []byte("echo x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(one, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	setHooks(t, 0, nil, func() {
		if err := os.Remove(link); err != nil {
			t.Error(err)
			return
		}
		if err := os.Symlink(two, link); err != nil {
			t.Error(err)
		}
	}, nil)
	got := New().ReadScript(filepath.Join(link, "x.sh"))
	if got.Limit != shellwords.RuleChanging || got.Detail != "path resolves to a different file than the one opened" || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestReaderRefusesHazardousShapesBeforeAnyFilesystemAccess(t *testing.T) {
	touched := 0
	setHooks(t, 0, func() { touched++ }, func() { touched++ }, func() { touched++ })
	for _, p := range []string{
		`\\srv\share\a.sh`, `//srv/share/a.sh`, `\\?\C:\a.sh`, `\\.\pipe\x`, `C:\a.sh:stream`, `CON.sh`, `dir\a.sh.`,
		`/proc/self/environ`, `/dev/stdin`, `/tmp/../dev/stdin`, `/./proc/self/environ`,
	} {
		got := New().ReadScript(p)
		if got.Limit != shellwords.RuleUnreadable || got.Detail != string(pathshape.LocalFileProblem(p)) || got.Detail == "" {
			t.Errorf("%q: %+v", p, got)
		}
	}
	// COM0 and LPT0 are device names on every host, so the refusal text is pinned, not derived from the function
	// under test.
	for _, p := range []string{`COM0.sh`, `dir/lpt0/a.sh`, `LPT0`, `com0.tar.sh`} {
		got := New().ReadScript(p)
		if got.Limit != shellwords.RuleUnreadable || got.Detail != "reserved device name" || got.Text != "" {
			t.Errorf("%q: %+v", p, got)
		}
	}
	if touched != 0 {
		t.Fatalf("filesystem touched %d times", touched)
	}
}

func TestReaderRefusesAShapeThePolicyWouldAlsoMatch(t *testing.T) {
	got := New(WithPathPolicy(func(string) string { return "protected" })).ReadScript(`//srv/share/secret/a.sh`)
	if got.Limit != shellwords.RuleUnreadable {
		t.Fatalf("a hazardous shape is refused before the policy: %+v", got)
	}
}

func TestReaderRefusesAProcessObjectBehindADirectoryLink(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("no /proc")
	}
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Skip("no /proc")
	}
	link := filepath.Join(t.TempDir(), "proc")
	if err := os.Symlink("/proc/self", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got := New().ReadScript(filepath.Join(link, "environ"))
	if got.Limit != shellwords.RuleUnreadable || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestReaderRefusesADrivePathOnANonWindowsHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a drive path is native here")
	}
	touched := 0
	setHooks(t, 0, func() { touched++ }, nil, nil)
	got := New().ReadScript(`C:/Users/x/a.sh`)
	if got.Limit != shellwords.RuleUnreadable || got.Detail != "drive path on a non-Windows host" || touched != 0 {
		t.Fatalf("touched=%d %+v", touched, got)
	}
}

// Off Windows the shell resolves .. against the real directory, so a path with a .. after a name may name another
// file than its cleaned form. A caller asking for one is refused whatever the files hold.
func TestReaderRefusesAParentReferenceAfterANameOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows resolves .. by spelling")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.sh"), []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := New().ReadScript(filepath.Join(dir, "a") + "/../b.sh")
	if got.Limit != shellwords.RuleUnreadable || got.Detail != "parent reference after a path component" || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestReaderLetsALegitimateConsoleNamedScriptThrough(t *testing.T) {
	for _, name := range []string{"console.sh", "network.sh", "COM10.sh", "ünï.sh"} {
		p := write(t, name, []byte("echo x\n"))
		if got := New().ReadScript(p); got.Limit != shellwords.RuleNone {
			t.Errorf("%s: %+v", name, got)
		}
	}
}
