package scriptsnap

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// setHooks installs the test hooks and restores the defaults when the test ends.
func setHooks(t *testing.T, ceiling time.Duration, lstat, open, before func()) {
	t.Helper()
	oldCeiling, oldLstat, oldOpen, oldBefore := readCeiling, afterLstat, afterOpen, beforeRead
	t.Cleanup(func() { readCeiling, afterLstat, afterOpen, beforeRead = oldCeiling, oldLstat, oldOpen, oldBefore })
	if ceiling > 0 {
		readCeiling = ceiling
	}
	if lstat != nil {
		afterLstat = lstat
	}
	if open != nil {
		afterOpen = open
	}
	if before != nil {
		beforeRead = before
	}
}

func TestReaderRefusesDirectory(t *testing.T) {
	got := New().ReadScript(t.TempDir())
	if got.Limit != shellwords.RuleUnreadable || got.Detail != "not a regular file" {
		t.Fatalf("%+v", got)
	}
}

func TestReaderRefusesDevice(t *testing.T) {
	// The path shape check refuses the device name before the kind check can see it. A device node cannot be made
	// without privilege and every spelling of an existing one is under /dev, so the kind refusal for non-regular
	// files is pinned by the directory and FIFO tests instead.
	got := New().ReadScript(os.DevNull)
	if got.Limit != shellwords.RuleUnreadable || got.Detail != string(pathshape.LocalFileProblem(os.DevNull)) || got.Detail == "" {
		t.Fatalf("%+v", got)
	}
}

func TestReaderRefusesFinalSymlink(t *testing.T) {
	target := write(t, "real.sh", []byte("echo x\n"))
	link := filepath.Join(t.TempDir(), "link.sh")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got := New().ReadScript(link)
	if got.Limit != shellwords.RuleUnreadable || got.Detail != "not a regular file" || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestHardlinkAliasSharesTheFirstRead(t *testing.T) {
	a := write(t, "a.sh", []byte("echo one\n"))
	b := filepath.Join(filepath.Dir(a), "b.sh")
	if err := os.Link(a, b); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	opens := 0
	setHooks(t, 0, nil, func() { opens++ }, nil)
	s := New()
	first := s.ReadScript(a)
	if first.Limit != shellwords.RuleNone || first.ID == "" {
		t.Fatalf("first read: %+v", first)
	}
	second := s.ReadScript(b)
	if second != first || opens != 2 {
		t.Fatalf("alias read differs or re-read: opens=%d %+v vs %+v", opens, second, first)
	}
	if s.ReadScript(a) != first || s.ReadScript(b) != first || opens != 2 {
		t.Fatalf("a repeated path was read again: opens=%d", opens)
	}
}

// An alias whose bytes differ from the first read of the same identity is not answered with the first text.
func TestAliasModifiedBetweenReadsIsRefused(t *testing.T) {
	a := write(t, "a.sh", []byte("echo one\n"))
	b := filepath.Join(filepath.Dir(a), "b.sh")
	if err := os.Link(a, b); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	s := New()
	first := s.ReadScript(a)
	if err := os.WriteFile(a, []byte("echo two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := s.ReadScript(b)
	if got.Limit != shellwords.RuleChanging || got.Text != "" || got.SHA256 != "" {
		t.Fatalf("%+v", got)
	}
	if again := s.ReadScript(a); again != first {
		t.Fatalf("the first read changed: %+v", again)
	}
}

func setFileID(t *testing.T, f func(*os.File, os.FileInfo) (string, bool)) {
	t.Helper()
	old := fileIDFunc
	t.Cleanup(func() { fileIDFunc = old })
	fileIDFunc = f
}

// A filesystem that reports one identity for every file must not merge distinct scripts.
func TestConstantIdentityDoesNotMergeDistinctScripts(t *testing.T) {
	setFileID(t, func(*os.File, os.FileInfo) (string, bool) { return "same", true })
	a := write(t, "a.sh", []byte("echo one\n"))
	b := write(t, "b.sh", []byte("echo two\n"))
	s := New()
	first := s.ReadScript(a)
	got := s.ReadScript(b)
	if first.Limit != shellwords.RuleNone || got.Limit != shellwords.RuleChanging || got.Text != "" {
		t.Fatalf("first=%+v second=%+v", first, got)
	}
}

// An empty identity means the filesystem gave none: the file is read on its own and never merged.
func TestEmptyIdentitySkipsDedupe(t *testing.T) {
	setFileID(t, func(*os.File, os.FileInfo) (string, bool) { return "", true })
	a := write(t, "a.sh", []byte("echo one\n"))
	b := write(t, "b.sh", []byte("echo two\n"))
	s := New()
	ra, rb := s.ReadScript(a), s.ReadScript(b)
	if ra.Limit != shellwords.RuleNone || rb.Limit != shellwords.RuleNone || ra.Text != "echo one\n" || rb.Text != "echo two\n" || ra.ID != "" || rb.ID != "" {
		t.Fatalf("%+v %+v", ra, rb)
	}
}

// Two distinct files never share an identity, so the identity map cannot merge unrelated scripts.
func TestDistinctFilesHaveDistinctIDs(t *testing.T) {
	a := write(t, "a.sh", []byte("echo a\n"))
	b := write(t, "b.sh", []byte("echo a\n"))
	s := New()
	ra, rb := s.ReadScript(a), s.ReadScript(b)
	if ra.ID == "" || rb.ID == "" || ra.ID == rb.ID {
		t.Fatalf("ids: %q %q", ra.ID, rb.ID)
	}
	if filepath.ToSlash(rb.Path) != filepath.ToSlash(filepath.Clean(b)) {
		t.Fatalf("second file answered with the wrong read: %+v", rb)
	}
}

func TestLexicalAliasesShareOneEntry(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	dir := filepath.Dir(p)
	reads := 0
	setHooks(t, 0, nil, func() { reads++ }, nil)
	s := New()
	first := s.ReadScript(p)
	// Join would clean the path itself, so the unclean spelling is built by hand.
	sep := string(filepath.Separator)
	second := s.ReadScript(dir + sep + "." + sep + "a.sh")
	if first != second || reads != 1 {
		t.Fatalf("reads=%d first=%+v second=%+v", reads, first, second)
	}
}

func TestChangeDuringReadIsDetected(t *testing.T) {
	cases := map[string]func(p string, info os.FileInfo){
		"size": func(p string, info os.FileInfo) {
			_ = os.WriteFile(p, []byte("echo one\necho two\n"), 0o644)
			_ = os.Chtimes(p, info.ModTime(), info.ModTime())
		},
		"mtime": func(p string, info os.FileInfo) {
			later := info.ModTime().Add(time.Hour)
			_ = os.Chtimes(p, later, later)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := write(t, "a.sh", []byte("echo one\n"))
			info, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			setHooks(t, 0, nil, func() { mutate(p, info) }, nil)
			got := New().ReadScript(p)
			if got.Limit != shellwords.RuleChanging || got.Text != "" || got.SHA256 != "" {
				t.Fatalf("%+v", got)
			}
		})
	}
}

// replaceFile puts a different file at p, with the size and modification time of the one it replaces, so only the file
// identity tells them apart. Hosts that refuse to replace a path whose file is open report false.
func replaceFile(t *testing.T, p string) bool {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(p), "other.sh")
	if err := os.WriteFile(other, []byte("#"+strings.Repeat("o", int(info.Size())-2)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		return false
	}
	return os.Rename(other, p) == nil
}

func TestReplacementBetweenOpenAndRestatIsDetected(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	replaced := false
	setHooks(t, 0, nil, func() { replaced = replaceFile(t, p) }, nil)
	got := New().ReadScript(p)
	if !replaced {
		t.Skip("the host does not allow replacing an open file")
	}
	if got.Limit != shellwords.RuleChanging {
		t.Fatalf("%+v", got)
	}
}

func TestSwapBetweenLstatAndOpenIsDetected(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	setHooks(t, 0, func() { replaceFile(t, p) }, nil, nil)
	got := New().ReadScript(p)
	if got.Limit != shellwords.RuleChanging || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestSymlinkSwapBetweenLstatAndOpenIsDetected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.sh")
	target := filepath.Join(dir, "target.sh")
	if err := os.WriteFile(p, []byte("echo one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("echo two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	swapped := false
	setHooks(t, 0, func() {
		if os.Remove(p) == nil {
			swapped = os.Symlink(target, p) == nil
		}
	}, nil, nil)
	got := New().ReadScript(p)
	if !swapped {
		t.Skip("symlinks unavailable")
	}
	// Hosts that do not follow a final symlink at open refuse it as unreadable instead.
	if (got.Limit != shellwords.RuleChanging && got.Limit != shellwords.RuleUnreadable) || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

func TestReadCeilingIsEnforced(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	release := make(chan struct{})
	hookDone := make(chan struct{})
	var once sync.Once
	setHooks(t, 50*time.Millisecond, nil, func() {
		defer once.Do(func() { close(hookDone) })
		<-release
	}, nil)
	// The snapshot-wide budget is far longer than the wait below, so only the per-read ceiling can end the read in time.
	oldBudget := readBudget
	t.Cleanup(func() { readBudget = oldBudget })
	readBudget = time.Minute
	s := New()
	var wg sync.WaitGroup
	results := make([]shellwords.ScriptRead, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = s.ReadScript(p) }(i)
	}
	waiters := make(chan struct{})
	go func() { wg.Wait(); close(waiters) }()
	select {
	case <-waiters:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("waiters were held by a read that never finished")
	}
	for _, r := range results {
		if r.Limit != shellwords.RuleBudget || r != results[0] {
			close(release)
			t.Fatalf("results: %+v", results)
		}
	}
	// Let the abandoned read run to completion; its late success must not replace the stored budget refusal.
	close(release)
	<-hookDone
	if again := s.ReadScript(p); again != results[0] {
		t.Fatalf("definitive result was overwritten: %+v", again)
	}
}

func TestPanickingReadIsRefusedNotTrusted(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	setHooks(t, 0, nil, func() { panic("injected") }, nil)
	s := New()
	got := s.ReadScript(p)
	if got.Limit != shellwords.RuleUnreadable || got.Text != "" {
		t.Fatalf("%+v", got)
	}
	if again := s.ReadScript(p); again != got {
		t.Fatalf("second read differs: %+v", again)
	}
}

// A panic on the calling goroutine marks the once as done, so the pre-seeded refusal is all later callers see.
func TestPanicBeforeTheReadLeavesARefusal(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	setHooks(t, 0, nil, nil, func() { panic("injected") })
	s := New()
	func() {
		defer func() { _ = recover() }()
		s.ReadScript(p)
	}()
	got := s.ReadScript(p)
	if got.Limit != shellwords.RuleUnreadable || got.Text != "" || got.SHA256 != "" {
		t.Fatalf("a read that never completed must not look like success: %+v", got)
	}
}

func TestSparseOversizeFileIsRefusedCheaply(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sparse.sh")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 28); err != nil {
		f.Close()
		t.Skipf("sparse files unavailable: %v", err)
	}
	f.Close()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got := New().ReadScript(p)
	runtime.ReadMemStats(&after)
	if got.Limit != shellwords.RuleTooLarge {
		t.Fatalf("%+v", got)
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 8<<20 {
		t.Fatalf("the oversize file was buffered: %d bytes allocated", grown)
	}
}

func TestRefusalDetailsStayFreeOfContent(t *testing.T) {
	p := write(t, "a.sh", []byte("SECRETMARKER\n"))
	setHooks(t, 0, nil, func() { panic("SECRETMARKER") }, nil)
	got := New().ReadScript(p)
	if got.Limit == shellwords.RuleNone || strings.Contains(got.Detail, "SECRETMARKER") {
		t.Fatalf("%+v", got)
	}
}
