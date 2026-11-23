package scriptsnap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestReadSizeBoundary(t *testing.T) {
	atLimit := strings.Repeat("a", MaxScriptBytes-1) + "\n"
	got := New().ReadScript(write(t, "exact.sh", []byte(atLimit)))
	if got.Limit != shellwords.RuleNone || got.Bytes != MaxScriptBytes || got.Raw != atLimit || got.Text != atLimit || got.TooLarge {
		t.Fatalf("a script of exactly the limit must be read: limit=%q bytes=%d tooLarge=%v", got.Limit, got.Bytes, got.TooLarge)
	}
	over := "#!/bin/sh\n" + strings.Repeat("a", MaxScriptBytes+1-len("#!/bin/sh\n"))
	got = New().ReadScript(write(t, "over.sh", []byte(over)))
	if got.Limit != shellwords.RuleTooLarge || !got.TooLarge || got.ID != "" || got.Raw != "" || got.Text != "" || got.Header.Shebang != "#!/bin/sh" {
		t.Fatalf("one byte over the limit must be refused with a header peek: %+v", got)
	}
}

func TestReadAcceptsPlainTextAndEmpty(t *testing.T) {
	for name, body := range map[string]string{"empty.sh": "", "utf8.sh": "echo \U000000e9\U00004e2d\n", "nonl.sh": "echo a"} {
		got := New().ReadScript(write(t, name, []byte(body)))
		if got.Limit != shellwords.RuleNone || got.Raw != body || got.Text != body || got.Bytes != len(body) {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// Raw script bytes with byte-order marks are cached intact so dialect-specific encoding checks can reject POSIX BOMs
// while stripping UTF-8 BOMs for PowerShell and rejecting UTF-16 BOMs everywhere.
func TestByteOrderMarksAreNamed(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantNUL bool
		wantPS  string
		psRule  shellwords.Rule
	}{
		{"utf8bom.sh", "\xef\xbb\xbfecho a\r\n", false, "echo a\n", shellwords.RuleNone},
		{"utf16le.sh", "\xff\xfee\x00c\x00", true, "", shellwords.RuleEncoding},
		{"utf16be.sh", "\xfe\xff\x00e\x00c", true, "", shellwords.RuleEncoding},
		{"utf16bom.sh", "\xff\xfe", false, "", shellwords.RuleEncoding},
	}
	for _, tc := range cases {
		got := New().ReadScript(write(t, tc.name, []byte(tc.body)))
		if got.Limit != shellwords.RuleNone || got.Raw != tc.body || got.Text != tc.body || got.Header.HasNUL != tc.wantNUL {
			t.Fatalf("%s: want raw read, got %+v", tc.name, got)
		}
		if text, rule := shellwords.CheckPOSIXEncoding(got.Raw); rule != shellwords.RuleEncoding || text != "" {
			t.Errorf("%s POSIX check: text=%q rule=%q", tc.name, text, rule)
		}
		if text, rule := shellwords.CheckPowerShellEncoding(got.Raw); rule != tc.psRule || text != tc.wantPS {
			t.Errorf("%s PowerShell check: text=%q rule=%q, want text=%q rule=%q", tc.name, text, rule, tc.wantPS, tc.psRule)
		}
	}
}

func TestRefusalDetailHoldsNoFileBytes(t *testing.T) {
	const marker = "SECRETMARKER"

	t.Run("oversize", func(t *testing.T) {
		body := "#!/bin/sh\n" + marker + strings.Repeat("a", MaxScriptBytes)
		got := New().ReadScript(write(t, "big.sh", []byte(body)))
		if got.Limit != shellwords.RuleTooLarge || got.Detail == "" || got.Raw != "" || got.Text != "" || strings.Contains(got.Detail, marker) {
			t.Fatalf("oversize refusal leaked or missing detail: %+v", got)
		}
	})

	t.Run("protected", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "secret")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := writeIn(t, dir, "cred.sh", []byte(marker+"\n"))
		got := New(WithPathPolicy(secretPolicy)).ReadScript(p)
		if got.Limit != shellwords.RulePathProtected || got.Detail == "" || got.Raw != "" || got.Text != "" || strings.Contains(got.Detail, marker) {
			t.Fatalf("protected refusal leaked: %+v", got)
		}
	})

	t.Run("changing", func(t *testing.T) {
		p := write(t, "changing.sh", []byte(marker+"\n"))
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		setHooks(t, 0, nil, func() {
			later := info.ModTime().Add(time.Hour)
			_ = os.Chtimes(p, later, later)
		}, nil)
		got := New().ReadScript(p)
		if got.Limit != shellwords.RuleChanging || got.Detail == "" || got.Raw != "" || got.Text != "" || strings.Contains(got.Detail, marker) {
			t.Fatalf("changing refusal leaked: %+v", got)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\n"+marker+"\n"))
		link := filepath.Join(dir, "link.sh")
		hookutiltest.SymlinkOrSkip(t, target, link)
		got := New().ReadScript(link)
		if got.Limit != shellwords.RuleUnreadable || got.Detail == "" || got.Raw != "" || got.Text != "" ||
			strings.Contains(got.Detail, marker) || strings.Contains(got.ResolvedHeader.Shebang, marker) {
			t.Fatalf("symlink refusal leaked: %+v", got)
		}
	})

	t.Run("encoding_checks_leak_no_bytes", func(t *testing.T) {
		bodies := map[string]string{
			"crlf.sh":    marker + "\r\n",
			"lonecr.sh":  marker + "\r",
			"nulbyte.sh": marker + "\x00\n",
			"bad.sh":     marker + "\xff\n",
			"bom_bad.sh": "\xef\xbb\xbf" + marker + "\xff\n",
			"utf16le.sh": "\xff\xfe" + marker,
			"utf16be.sh": "\xfe\xff" + marker,
		}
		for name, body := range bodies {
			got := New().ReadScript(write(t, name, []byte(body)))
			if got.Limit != shellwords.RuleNone || got.Raw != body {
				t.Fatalf("%s: want raw read, got %+v", name, got)
			}
			if text, rule := shellwords.CheckPOSIXEncoding(got.Raw); rule != shellwords.RuleEncoding || text != "" {
				t.Errorf("%s POSIX check leaked or passed: text=%q rule=%q", name, text, rule)
			}
			if name == "crlf.sh" {
				continue
			}
			if text, rule := shellwords.CheckPowerShellEncoding(got.Raw); rule != shellwords.RuleEncoding || text != "" {
				t.Errorf("%s PowerShell check leaked or passed: text=%q rule=%q", name, text, rule)
			}
		}
	})
}

func TestReadNotExistDistinguishedFromOtherUnreadable(t *testing.T) {
	dir := t.TempDir()
	missing := New().ReadScript(filepath.Join(dir, "missing.sh"))
	if missing.Limit != shellwords.RuleUnreadable || !missing.NotExist || missing.Detail != "not a readable regular file" || missing.Raw != "" || missing.Text != "" || missing.ID != "" {
		t.Fatalf("missing file: %+v", missing)
	}
	onDir := New().ReadScript(dir)
	if onDir.Limit != shellwords.RuleUnreadable || onDir.NotExist || onDir.IsSymlink {
		t.Fatalf("directory must not set NotExist: %+v", onDir)
	}
	hazard := New().ReadScript("COM0.sh")
	if hazard.Limit != shellwords.RuleUnreadable || hazard.NotExist {
		t.Fatalf("hazardous path must not set NotExist: %+v", hazard)
	}
	prot := New(WithPathPolicy(secretPolicy)).ReadScript(filepath.Join(dir, "secret", "missing.sh"))
	if prot.Limit != shellwords.RulePathProtected || prot.NotExist {
		t.Fatalf("protected missing path must fail policy before Lstat: %+v", prot)
	}
}

func TestOversizedFileHeaderPeek(t *testing.T) {
	t.Run("elf_binary", func(t *testing.T) {
		head := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00}
		body := append(append([]byte(nil), head...), make([]byte, MaxScriptBytes+64)...)
		got := New().ReadScript(write(t, "big.bin", body))
		if got.Limit != shellwords.RuleTooLarge || !got.TooLarge || got.ID != "" || got.Raw != "" || got.Text != "" || got.Bytes != 0 || got.SHA256 != "" {
			t.Fatalf("unexpected oversized read: %+v", got)
		}
		if !got.Header.BinaryMagic || !got.Header.HasNUL || got.Header.Shebang != "" {
			t.Fatalf("unexpected header on oversized ELF: %+v", got.Header)
		}
	})

	t.Run("shebang_script", func(t *testing.T) {
		prefix := "#!/usr/bin/env pwsh\r\n"
		body := prefix + strings.Repeat("Write-Host ok\r\n", (MaxScriptBytes/15)+10)
		got := New().ReadScript(write(t, "big.ps1", []byte(body)))
		if got.Limit != shellwords.RuleTooLarge || !got.TooLarge || got.ID != "" || got.Raw != "" || got.Text != "" {
			t.Fatalf("unexpected oversized script read: %+v", got)
		}
		if got.Header.Shebang != "#!/usr/bin/env pwsh" || got.Header.BinaryMagic || got.Header.HasNUL {
			t.Fatalf("unexpected header on oversized script: %+v", got.Header)
		}
	})

	t.Run("mutation_during_oversized_peek", func(t *testing.T) {
		cases := map[string]func(p string, info os.FileInfo){
			"size": func(p string, info os.FileInfo) {
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
				if err == nil {
					_, _ = f.Write([]byte("extra\n"))
					_ = f.Close()
				}
				_ = os.Chtimes(p, info.ModTime(), info.ModTime())
			},
			"mtime": func(p string, info os.FileInfo) {
				later := info.ModTime().Add(time.Hour)
				_ = os.Chtimes(p, later, later)
			},
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				body := "#!/bin/sh\n" + strings.Repeat("a", MaxScriptBytes+128)
				p := write(t, "mutated_big.sh", []byte(body))
				info, err := os.Stat(p)
				if err != nil {
					t.Fatal(err)
				}
				setHooks(t, 0, nil, func() { mutate(p, info) }, nil)
				got := New().ReadScript(p)
				if got.Limit != shellwords.RuleChanging || got.TooLarge || got.ID != "" || got.Raw != "" || got.Text != "" || got.Header != (shellwords.ScriptHeader{}) {
					t.Fatalf("mutated oversized file must fail as changing with zero header: %+v", got)
				}
			})
		}
	})
}

func TestSymlinkFinalLeafHeaderPeek(t *testing.T) {
	t.Run("regular_shebang_target", func(t *testing.T) {
		dir := t.TempDir()
		target := writeIn(t, dir, "real.sh", []byte("#!/bin/bash\necho ok\n"))
		link := filepath.Join(dir, "link.sh")
		hookutiltest.SymlinkOrSkip(t, target, link)

		got := New().ReadScript(link)
		if got.Limit != shellwords.RuleUnreadable || got.Detail != "not a regular file" || !got.IsSymlink || got.NotExist {
			t.Fatalf("unexpected symlink refusal: %+v", got)
		}
		if got.ID != "" || got.Raw != "" || got.Text != "" || got.Bytes != 0 || got.SHA256 != "" {
			t.Fatalf("symlink read must carry no identity or content: %+v", got)
		}
		if !got.HasResolvedHeader || got.ResolvedHeader.Shebang != "#!/bin/bash" || got.ResolvedHeader.BinaryMagic || got.ResolvedHeader.HasNUL {
			t.Fatalf("unexpected ResolvedHeader: has=%v header=%+v", got.HasResolvedHeader, got.ResolvedHeader)
		}
	})

	t.Run("regular_elf_target", func(t *testing.T) {
		dir := t.TempDir()
		target := writeIn(t, dir, "app.bin", []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00})
		link := filepath.Join(dir, "app")
		hookutiltest.SymlinkOrSkip(t, target, link)

		got := New().ReadScript(link)
		if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || !got.HasResolvedHeader || !got.ResolvedHeader.BinaryMagic || !got.ResolvedHeader.HasNUL {
			t.Fatalf("unexpected ELF symlink peek: %+v", got)
		}
	})

	t.Run("dangling_symlink", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, "dangling.sh")
		hookutiltest.SymlinkOrSkip(t, filepath.Join(dir, "missing.sh"), link)

		got := New().ReadScript(link)
		if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) || got.NotExist {
			t.Fatalf("unexpected dangling symlink read: %+v", got)
		}
	})

	t.Run("symlink_to_directory", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "dirlink")
		hookutiltest.SymlinkOrSkip(t, sub, link)

		got := New().ReadScript(link)
		if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader {
			t.Fatalf("unexpected directory symlink read: %+v", got)
		}
	})

	t.Run("symlink_cycle", func(t *testing.T) {
		dir := t.TempDir()
		a := filepath.Join(dir, "a.sh")
		b := filepath.Join(dir, "b.sh")
		hookutiltest.SymlinkOrSkip(t, b, a)
		hookutiltest.SymlinkOrSkip(t, a, b)

		got := New().ReadScript(a)
		if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) {
			t.Fatalf("unexpected symlink cycle read: %+v", got)
		}
	})

	t.Run("symlink_to_protected_path_preserves_link_path", func(t *testing.T) {
		dir := t.TempDir()
		secretDir := filepath.Join(dir, "secret")
		if err := os.Mkdir(secretDir, 0o755); err != nil {
			t.Fatal(err)
		}
		target := writeIn(t, secretDir, "cred.sh", []byte("#!/bin/sh\necho secret\n"))
		link := filepath.Join(dir, "entry.sh")
		hookutiltest.SymlinkOrSkip(t, target, link)

		got := New(WithPathPolicy(secretPolicy)).ReadScript(link)
		if got.Limit != shellwords.RulePathProtected || got.Path != filepath.Clean(link) || got.HasResolvedHeader || got.Raw != "" || got.Text != "" {
			t.Fatalf("symlink to protected path must preserve link Path and refuse before peek: %+v", got)
		}
	})

	t.Run("symlink_peek_midflight_policy_refusal_preserves_link_path", func(t *testing.T) {
		dir := t.TempDir()
		target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\necho hi\n"))
		link := filepath.Join(dir, "entry.sh")
		hookutiltest.SymlinkOrSkip(t, target, link)

		// Flip policy to refuse target only after open so checkOpened inside peekSymlinkHeader returns a refusal
		// constructed with resolved; peekSymlinkHeader must normalize Path back to link.
		protectTarget := false
		setHooks(t, 0, nil, func() { protectTarget = true }, nil)
		policy := func(p string) string {
			if protectTarget && strings.HasSuffix(filepath.ToSlash(p), "/target.sh") {
				return "protected"
			}
			return ""
		}
		got := New(WithPathPolicy(policy)).ReadScript(link)
		if got.Limit != shellwords.RulePathProtected || got.Path != filepath.Clean(link) || got.HasResolvedHeader || got.Raw != "" || got.Text != "" {
			t.Fatalf("midflight protected target leaked resolved path or header: %+v", got)
		}
	})

	t.Run("concurrent_mutation_during_symlink_peek", func(t *testing.T) {
		t.Run("target_mtime_changed", func(t *testing.T) {
			dir := t.TempDir()
			target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\necho one\n"))
			link := filepath.Join(dir, "link.sh")
			hookutiltest.SymlinkOrSkip(t, target, link)
			info, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			setHooks(t, 0, nil, func() {
				later := info.ModTime().Add(time.Hour)
				_ = os.Chtimes(target, later, later)
			}, nil)
			got := New().ReadScript(link)
			if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) {
				t.Fatalf("mutated symlink target must clear ResolvedHeader: %+v", got)
			}
		})

		t.Run("target_size_changed", func(t *testing.T) {
			dir := t.TempDir()
			target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\necho one\n"))
			link := filepath.Join(dir, "link.sh")
			hookutiltest.SymlinkOrSkip(t, target, link)
			info, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			setHooks(t, 0, nil, func() {
				_ = os.WriteFile(target, []byte("#!/bin/sh\necho one\necho two\n"), 0o644)
				_ = os.Chtimes(target, info.ModTime(), info.ModTime())
			}, nil)
			got := New().ReadScript(link)
			if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) {
				t.Fatalf("resized symlink target must clear ResolvedHeader: %+v", got)
			}
		})

		t.Run("target_removed_before_open", func(t *testing.T) {
			dir := t.TempDir()
			target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\necho one\n"))
			link := filepath.Join(dir, "link.sh")
			hookutiltest.SymlinkOrSkip(t, target, link)
			setHooks(t, 0, func() { _ = os.Remove(target) }, nil, nil)
			got := New().ReadScript(link)
			if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) {
				t.Fatalf("removed symlink target must clear ResolvedHeader: %+v", got)
			}
		})

		t.Run("target_swapped_before_open", func(t *testing.T) {
			dir := t.TempDir()
			target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\necho one\n"))
			link := filepath.Join(dir, "link.sh")
			hookutiltest.SymlinkOrSkip(t, target, link)
			setHooks(t, 0, func() { replaceFile(t, target) }, nil, nil)
			got := New().ReadScript(link)
			if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) {
				t.Fatalf("swapped symlink target must clear ResolvedHeader: %+v", got)
			}
		})

		t.Run("link_retargeted_during_peek", func(t *testing.T) {
			dir := t.TempDir()
			target := writeIn(t, dir, "target.sh", []byte("#!/bin/sh\necho one\n"))
			other := writeIn(t, dir, "other.sh", []byte("#!/bin/python3\nprint(1)\n"))
			link := filepath.Join(dir, "link.sh")
			hookutiltest.SymlinkOrSkip(t, target, link)
			setHooks(t, 0, nil, func() {
				if os.Remove(link) == nil {
					_ = os.Symlink(other, link)
				}
			}, nil)
			got := New().ReadScript(link)
			if got.Limit != shellwords.RuleUnreadable || !got.IsSymlink || got.HasResolvedHeader || got.ResolvedHeader != (shellwords.ScriptHeader{}) {
				t.Fatalf("retargeted symlink must clear ResolvedHeader: %+v", got)
			}
		})
	})
}
