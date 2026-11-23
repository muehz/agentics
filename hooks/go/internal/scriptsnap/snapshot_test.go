package scriptsnap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func write(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSnapshotIsOnceOnly(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	s := New()
	first := s.ReadScript(p)
	if first.Limit != shellwords.RuleNone || first.Text != "echo one\n" {
		t.Fatalf("first read: %+v", first)
	}
	if err := os.WriteFile(p, []byte("rm -rf /\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := s.ReadScript(p); second != first {
		t.Fatalf("second read differs: %+v vs %+v", second, first)
	}
}

func TestSnapshotRefusalIsOnceOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "late.sh")
	s := New()
	first := s.ReadScript(p)
	if first.Limit != shellwords.RuleUnreadable {
		t.Fatalf("missing file: %+v", first)
	}
	if err := os.WriteFile(p, []byte("echo one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := s.ReadScript(p); second != first {
		t.Fatalf("a refusal must stay final: %+v vs %+v", second, first)
	}
}

func TestSnapshotKeysAreIndependent(t *testing.T) {
	a := write(t, "a.sh", []byte("echo a\n"))
	b := write(t, "b.sh", []byte("echo b\n"))
	s := New()
	ra, rb := s.ReadScript(a), s.ReadScript(b)
	if ra.Text != "echo a\n" || rb.Text != "echo b\n" {
		t.Fatalf("paths shared an entry: %+v %+v", ra, rb)
	}
}

func TestSnapshotConcurrentReadersAgree(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	s := New()
	var wg sync.WaitGroup
	results := make([]shellwords.ScriptRead, 16)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = s.ReadScript(p) }(i)
	}
	wg.Wait()
	for _, r := range results {
		if r.SHA256 == "" || r.Limit != shellwords.RuleNone || r != results[0] {
			t.Fatalf("readers disagree: %+v", results)
		}
	}
}

func TestReadRecordsHashAndBytes(t *testing.T) {
	body := []byte("echo one\n")
	p := write(t, "a.sh", body)
	sum := sha256.Sum256(body)
	got := New().ReadScript(p)
	if got.SHA256 != hex.EncodeToString(sum[:]) || got.Bytes != len(body) {
		t.Fatalf("got %+v", got)
	}
}

func TestReadRefusals(t *testing.T) {
	big := []byte("#!/bin/sh\n" + strings.Repeat("echo x\n", (MaxScriptBytes/7)+4))
	got := New().ReadScript(write(t, "big.sh", big))
	if got.Limit != shellwords.RuleTooLarge || !got.TooLarge || got.ID != "" || got.Raw != "" || got.Text != "" || got.SHA256 != "" || got.Bytes != 0 || strings.Contains(got.Detail, "echo") || got.Header.Shebang != "#!/bin/sh" {
		t.Errorf("big.sh: want %s with header peek and no text, got %+v", shellwords.RuleTooLarge, got)
	}
	dir := t.TempDir()
	if got := New().ReadScript(dir); got.Limit != shellwords.RuleUnreadable || got.NotExist || got.Raw != "" || got.Text != "" {
		t.Errorf("directory: %+v", got)
	}
	if got := New().ReadScript(filepath.Join(dir, "missing.sh")); got.Limit != shellwords.RuleUnreadable || !got.NotExist || got.Raw != "" || got.Text != "" {
		t.Errorf("missing: %+v", got)
	}
}

func TestReadCachesRawBytesWithoutContentGate(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantNUL bool
	}{
		{"crlf.sh", []byte("echo a\r\necho b\r\n"), false},
		{"cr.sh", []byte("echo a\recho b\n"), false},
		{"nulbyte.sh", []byte("echo a\x00b\n"), true},
		{"bad.sh", []byte("echo \xff\xfe\n"), false},
		{"bom.sh", []byte("\xef\xbb\xbfecho a\n"), false},
		{"utf16.sh", []byte("\xff\xfee\x00c\x00"), true},
		{"utf16be.sh", []byte("\xfe\xff\x00e\x00c"), true},
	}
	for _, tc := range cases {
		got := New().ReadScript(write(t, tc.name, tc.data))
		if got.Limit != shellwords.RuleNone || got.Raw != string(tc.data) || got.Text != string(tc.data) || got.Bytes != len(tc.data) || got.SHA256 == "" || got.Header.HasNUL != tc.wantNUL {
			t.Errorf("%s: want raw bytes cached, got %+v", tc.name, got)
		}
	}
}

func TestProbeExistsBasicAndOnceOnly(t *testing.T) {
	dir := t.TempDir()
	existing := writeIn(t, dir, "exists.ps1", []byte("Write-Host ok\r\n"))
	missing := filepath.Join(dir, "missing.ps1")

	s := New()
	if ok, rule := s.ProbeExists(existing); !ok || rule != shellwords.RuleNone {
		t.Fatalf("existing file: ok=%v rule=%q", ok, rule)
	}
	if ok, rule := s.ProbeExists(missing); ok || rule != shellwords.RuleNone {
		t.Fatalf("missing file: ok=%v rule=%q", ok, rule)
	}
	if ok, rule := s.ProbeExists(dir); !ok || rule != shellwords.RuleNone {
		t.Fatalf("existing directory: ok=%v rule=%q", ok, rule)
	}

	// Probes are once-only per Snapshot: filesystem changes after the probe do not flip cached verdicts.
	if err := os.Remove(existing); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missing, []byte("late\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, rule := s.ProbeExists(existing); !ok || rule != shellwords.RuleNone {
		t.Fatalf("cached existing probe flipped: ok=%v rule=%q", ok, rule)
	}
	if ok, rule := s.ProbeExists(missing); ok || rule != shellwords.RuleNone {
		t.Fatalf("cached missing probe flipped: ok=%v rule=%q", ok, rule)
	}
}

func TestProbeExistsEmptyAndHazardousWithoutCachePoisoning(t *testing.T) {
	dir := t.TempDir()
	bPath := writeIn(t, dir, "b.sh", []byte("echo ok\n"))
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := New()
	if ok, rule := s.ProbeExists(""); ok || rule != shellwords.RuleUnreadable {
		t.Fatalf("empty path: ok=%v rule=%q", ok, rule)
	}
	if ok, rule := s.ProbeExists("bad\x00.sh"); ok || rule != shellwords.RuleUnreadable {
		t.Fatalf("NUL path: ok=%v rule=%q", ok, rule)
	}
	// filepath.Clean("") is "."; rejecting "" before cleaning must not poison ".".
	if ok, rule := s.ProbeExists("."); !ok || rule != shellwords.RuleNone {
		t.Fatalf("dot after empty path was poisoned: ok=%v rule=%q", ok, rule)
	}

	for _, hazard := range []string{`\\srv\share\a.sh`, `//srv/share/a.sh`, `COM0.sh`, bPath + `:stream`} {
		fresh := New()
		if ok, rule := fresh.ProbeExists(hazard); ok || rule != shellwords.RuleUnreadable {
			t.Errorf("%q: ok=%v rule=%q", hazard, ok, rule)
		}
		if len(fresh.probes) != 0 {
			t.Errorf("%q allocated a probe entry: %d", hazard, len(fresh.probes))
		}
	}

	if runtime.GOOS != "windows" {
		// On Unix filepath.Clean("//...") collapses to "/...", and filepath.Clean(".../a/../b.sh") collapses to
		// ".../b.sh". Raw checks must reject both before touching s.probes so a clean probe of bPath still succeeds.
		doubleSlash := "//" + strings.TrimPrefix(bPath, "/")
		parentAfter := filepath.Join(dir, "a") + "/../b.sh"
		for _, raw := range []string{doubleSlash, parentAfter} {
			fresh := New()
			if ok, rule := fresh.ProbeExists(raw); ok || rule != shellwords.RuleUnreadable {
				t.Fatalf("%q: ok=%v rule=%q", raw, ok, rule)
			}
			if ok, rule := fresh.ProbeExists(bPath); !ok || rule != shellwords.RuleNone {
				t.Fatalf("clean probe of %q poisoned after %q: ok=%v rule=%q", bPath, raw, ok, rule)
			}
		}
	}
}

func TestProbeExistsProtectedAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	secretDir := filepath.Join(dir, "secret")
	if err := os.Mkdir(secretDir, 0o755); err != nil {
		t.Fatal(err)
	}
	secretFile := writeIn(t, secretDir, "cred.sh", []byte("echo secret\n"))
	plainFile := writeIn(t, dir, "plain.sh", []byte("echo plain\n"))

	t.Run("protected_direct", func(t *testing.T) {
		touched := 0
		setHooks(t, 0, func() { touched++ }, func() { touched++ }, nil)
		s := New(WithPathPolicy(secretPolicy))
		for _, p := range []string{secretFile, filepath.Join(secretDir, "missing.sh")} {
			if ok, rule := s.ProbeExists(p); ok || rule != shellwords.RulePathProtected {
				t.Fatalf("%s: ok=%v rule=%q", p, ok, rule)
			}
		}
		if touched != 0 {
			t.Fatalf("protected probe touched filesystem %d times", touched)
		}
	})

	t.Run("symlink_to_existing", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link.sh")
		hookutiltest.SymlinkOrSkip(t, plainFile, link)
		if ok, rule := New().ProbeExists(link); !ok || rule != shellwords.RuleNone {
			t.Fatalf("symlink to existing file: ok=%v rule=%q", ok, rule)
		}
	})

	t.Run("symlink_to_protected_existing", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link_secret.sh")
		hookutiltest.SymlinkOrSkip(t, secretFile, link)
		if ok, rule := New(WithPathPolicy(secretPolicy)).ProbeExists(link); ok || rule != shellwords.RulePathProtected {
			t.Fatalf("symlink to protected file: ok=%v rule=%q", ok, rule)
		}
	})

	t.Run("dangling_symlink_unprotected", func(t *testing.T) {
		sub := t.TempDir()
		link := filepath.Join(sub, "dangling.sh")
		hookutiltest.SymlinkOrSkip(t, filepath.Join(sub, "nope.sh"), link)
		if ok, rule := New(WithPathPolicy(secretPolicy)).ProbeExists(link); ok || rule != shellwords.RuleNone {
			t.Fatalf("dangling unprotected symlink: ok=%v rule=%q", ok, rule)
		}
	})

	t.Run("dangling_symlink_to_protected", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "dangling_secret.sh")
		hookutiltest.SymlinkOrSkip(t, filepath.Join(secretDir, "missing.sh"), link)
		if ok, rule := New(WithPathPolicy(secretPolicy)).ProbeExists(link); ok || rule != shellwords.RulePathProtected {
			t.Fatalf("dangling symlink to protected dir: ok=%v rule=%q", ok, rule)
		}
	})

	t.Run("symlink_cycle", func(t *testing.T) {
		sub := t.TempDir()
		a := filepath.Join(sub, "a.sh")
		b := filepath.Join(sub, "b.sh")
		hookutiltest.SymlinkOrSkip(t, b, a)
		hookutiltest.SymlinkOrSkip(t, a, b)
		if ok, rule := New().ProbeExists(a); ok || rule != shellwords.RuleUnreadable {
			t.Fatalf("symlink cycle: ok=%v rule=%q", ok, rule)
		}
	})

	t.Run("symlink_retargeted_to_protected_after_lstat", func(t *testing.T) {
		sub := t.TempDir()
		link := filepath.Join(sub, "swap.sh")
		hookutiltest.SymlinkOrSkip(t, plainFile, link)
		setHooks(t, 0, func() {
			if os.Remove(link) == nil {
				_ = os.Symlink(secretFile, link)
			}
		}, nil, nil)
		if ok, rule := New(WithPathPolicy(secretPolicy)).ProbeExists(link); ok || rule != shellwords.RulePathProtected {
			t.Fatalf("retargeted symlink to protected file: ok=%v rule=%q", ok, rule)
		}
	})
}

func TestProbeExistsIsSlotAndByteExempt(t *testing.T) {
	dir := t.TempDir()
	s := New()

	for i := 0; i < MaxFiles+10; i++ {
		var p string
		wantExists := i%2 == 0
		if wantExists {
			p = writeIn(t, dir, fmt.Sprintf("probe_exist_%d.ps1", i), quarterMiB)
		} else {
			p = filepath.Join(dir, fmt.Sprintf("probe_missing_%d.ps1", i))
		}
		ok, rule := s.ProbeExists(p)
		if ok != wantExists || rule != shellwords.RuleNone {
			t.Fatalf("probe %d: got (%v, %q), want (%v, RuleNone)", i, ok, rule, wantExists)
		}
	}
	if len(s.entries) != 0 || s.bytes != 0 {
		t.Fatalf("ProbeExists consumed read slots or event bytes: entries=%d bytes=%d", len(s.entries), s.bytes)
	}

	for i := 0; i < MaxFiles; i++ {
		p := writeIn(t, dir, fmt.Sprintf("real_%d.sh", i), []byte("echo ok\n"))
		if got := s.ReadScript(p); got.Limit != shellwords.RuleNone {
			t.Fatalf("ReadScript %d after %d probes failed: %+v", i, MaxFiles+10, got)
		}
	}
	if got := s.ReadScript(writeIn(t, dir, "overflow.sh", []byte("echo no\n"))); got.Limit != shellwords.RuleTooManyFiles {
		t.Fatalf("ReadScript past MaxFiles: %+v", got)
	}
	// Even after MaxFiles is exhausted for ReadScript, ProbeExists remains slot-exempt.
	extra := writeIn(t, dir, "after_max.ps1", []byte("Write-Host ok\n"))
	if ok, rule := s.ProbeExists(extra); !ok || rule != shellwords.RuleNone {
		t.Fatalf("ProbeExists after MaxFiles exhausted: ok=%v rule=%q", ok, rule)
	}
}

func TestProbeExistsConcurrentCallersAgree(t *testing.T) {
	p := write(t, "shared.ps1", []byte("Write-Host hi\n"))
	var policyCalls atomic.Int32
	s := New(WithPathPolicy(func(string) string {
		policyCalls.Add(1)
		return ""
	}))
	const workers = 16
	type res struct {
		ok   bool
		rule shellwords.Rule
	}
	results := make([]res, workers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i].ok, results[i].rule = s.ProbeExists(p)
		}(i)
	}
	wg.Wait()
	for i, r := range results {
		if !r.ok || r.rule != shellwords.RuleNone {
			t.Fatalf("worker %d disagreed: %+v", i, r)
		}
	}
	if got := policyCalls.Load(); got != 1 {
		t.Fatalf("probe ran %d times, want 1", got)
	}
}

func TestProbeExistsSharesSnapshotBudgetAndRecoversPanics(t *testing.T) {
	t.Run("budget_exhausted_by_probe", func(t *testing.T) {
		dir := t.TempDir()
		a := writeIn(t, dir, "a.ps1", []byte("x\n"))
		b := writeIn(t, dir, "b.ps1", []byte("y\n"))
		c := writeIn(t, dir, "c.sh", []byte("echo z\n"))

		var starts atomic.Int32
		setHooks(t, 0, func() { time.Sleep(300 * time.Millisecond) }, nil, func() { starts.Add(1) })
		oldBudget := readBudget
		t.Cleanup(func() { readBudget = oldBudget })
		readBudget = 80 * time.Millisecond

		s := New()
		if ok, rule := s.ProbeExists(a); ok || rule != shellwords.RuleBudget {
			t.Fatalf("first probe should exceed budget: ok=%v rule=%q", ok, rule)
		}
		if ok, rule := s.ProbeExists(b); ok || rule != shellwords.RuleBudget {
			t.Fatalf("second probe after exhausted budget: ok=%v rule=%q", ok, rule)
		}
		if got := s.ReadScript(c); got.Limit != shellwords.RuleBudget {
			t.Fatalf("ReadScript after exhausted probe budget: %+v", got)
		}
		if n := starts.Load(); n != 1 {
			t.Fatalf("expected 1 started operation before budget cutoff, got %d", n)
		}
	})

	t.Run("panic_in_probe_goroutine_fails_closed", func(t *testing.T) {
		p := write(t, "panic.ps1", []byte("x\n"))
		setHooks(t, 0, func() { panic("injected probe panic") }, nil, nil)
		s := New()
		if ok, rule := s.ProbeExists(p); ok || rule != shellwords.RuleUnreadable {
			t.Fatalf("panicking probe must fail closed with RuleUnreadable: ok=%v rule=%q", ok, rule)
		}
		if ok, rule := s.ProbeExists(p); ok || rule != shellwords.RuleUnreadable {
			t.Fatalf("cached panicking probe changed: ok=%v rule=%q", ok, rule)
		}
	})

	t.Run("panic_before_probe_goroutine_leaves_unreadable", func(t *testing.T) {
		p := write(t, "panic_before.ps1", []byte("x\n"))
		setHooks(t, 0, nil, nil, func() { panic("injected beforeRead panic") })
		s := New()
		func() {
			defer func() { _ = recover() }()
			s.ProbeExists(p)
		}()
		if ok, rule := s.ProbeExists(p); ok || rule != shellwords.RuleUnreadable {
			t.Fatalf("probe interrupted before goroutine must not leave RuleNone: ok=%v rule=%q", ok, rule)
		}
	})
}
func TestDirectNonShellClassificationExemptsEventBytes(t *testing.T) {
	dir, snapshot := t.TempDir(), New()
	for i, name := range []string{"one.sh", "two.sh", "three.sh", "four.sh"} {
		path := writeIn(t, dir, name, quarterMiB)
		if got := snapshot.ReadScript(path); got.Limit != shellwords.RuleNone {
			t.Fatalf("script %d should fit the byte budget: %v", i+1, got.Limit)
		}
	}
	padding := strings.Repeat("x", 32<<10)
	for _, tc := range []struct{ name, content string }{
		{"native", "\x7fELF" + padding},
		{"utility", "#!/usr/bin/env python3\n" + padding},
	} {
		path := writeIn(t, dir, tc.name, []byte(tc.content))
		parse := func() shellwords.Result {
			return shellwords.ParseScripts(t.Context(), "./"+tc.name, shellwords.POSIX, shellwords.ScriptOptions{
				Reader: snapshot, Cwd: dir, Context: shellwords.ExecutionContext{Dialect: shellwords.POSIX, Proven: true},
			})
		}
		for _, when := range []string{"before", "after"} {
			got := parse()
			if len(got.Invocations) != 1 || !got.Invocations[0].Inspectable || len(got.Scripts) != 0 {
				t.Fatalf("%s %s a charged read: expected a non-shell invocation, got %+v", tc.name, when, got)
			}
			if read := snapshot.ReadScript(path); read.Limit != shellwords.RuleTooLarge || read.Raw != "" {
				t.Fatalf("%s must still be refused when charged as a script: limit=%v, raw bytes=%d", tc.name, read.Limit, len(read.Raw))
			}
		}
	}
	if snapshot.bytes != 4*len(quarterMiB) {
		t.Fatalf("charged %d bytes, want %d script bytes", snapshot.bytes, 4*len(quarterMiB))
	}
}

func TestSnapshotClassificationSharesScriptRead(t *testing.T) {
	const source = "Write-Output sample\n"
	path := write(t, "sample.ps1", []byte(source))
	var reads atomic.Int32
	setHooks(t, 0, nil, nil, func() { reads.Add(1) })
	snapshot := New()
	metadata := snapshot.InspectScript(path)
	if metadata.Limit != shellwords.RuleNone || metadata.SHA256 == "" || metadata.Raw != "" || metadata.Text != "" || snapshot.bytes != 0 {
		t.Fatalf("classification must expose metadata without charging script bytes: %+v, bytes=%d", metadata, snapshot.bytes)
	}
	first, next := snapshot.ReadScript(path), snapshot.ReadScript(path)
	if first.Limit != shellwords.RuleNone || first.Raw != source || first.SHA256 != metadata.SHA256 || first != next {
		t.Fatalf("script read must use the cached contents: %+v, repeated %+v", first, next)
	}
	if reads.Load() != 1 || snapshot.bytes != len(source) || len(snapshot.entries) != 1 {
		t.Fatalf("expected one open and one byte charge, got %d opens, %d bytes, %d entries", reads.Load(), snapshot.bytes, len(snapshot.entries))
	}
}
