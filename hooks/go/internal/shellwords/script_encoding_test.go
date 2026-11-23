package shellwords

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func TestAgxLauncherSHA256MatchesCommittedFile(t *testing.T) {
	path := filepath.Join(hosttest.Checkout(t), "bin", "agx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	sum := sha256.Sum256([]byte(normalized))
	got := hex.EncodeToString(sum[:])
	if got != AgxLauncherSHA256 {
		t.Fatalf("AgxLauncherSHA256 = %q, committed LF bin/agx hashes to %q", AgxLauncherSHA256, got)
	}
}

func TestCheckPOSIXEncoding(t *testing.T) {
	passCases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"ascii lf", "#!/bin/sh\necho hi\n"},
		{"utf8 lf", "#!/bin/sh\necho caf\u00e9\n"},
		{"tab and spaces", "echo\t1 2\n"},
	}
	for _, tc := range passCases {
		t.Run("pass/"+tc.name, func(t *testing.T) {
			got, rule := CheckPOSIXEncoding(tc.raw)
			if rule != RuleNone || got != tc.raw {
				t.Fatalf("CheckPOSIXEncoding(%q) = (%q, %v), want (%q, RuleNone)", tc.raw, got, rule, tc.raw)
			}
		})
	}

	failCases := []struct {
		name string
		raw  string
	}{
		{"cr", "echo secret\recho b\n"},
		{"crlf", "echo secret\r\necho b\r\n"},
		{"utf8 bom", "\xef\xbb\xbfecho secret\n"},
		{"utf16 le bom", "\xff\xfee\x00c\x00h\x00o\x00"},
		{"utf16 be bom", "\xfe\xff\x00e\x00c\x00h\x00o"},
		{"nul byte", "echo secret\x00tail\n"},
		{"invalid utf8", "echo secret \xff\xfe\n"},
	}
	for _, tc := range failCases {
		t.Run("fail/"+tc.name, func(t *testing.T) {
			got, rule := CheckPOSIXEncoding(tc.raw)
			if rule != RuleEncoding || got != "" {
				t.Fatalf("CheckPOSIXEncoding(%q) = (%q, %v), want (\"\", RuleEncoding)", tc.raw, got, rule)
			}
		})
	}
}

func TestCheckPowerShellEncoding(t *testing.T) {
	passCases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"ascii lf", "Write-Output 'hi'\n", "Write-Output 'hi'\n"},
		{"ascii crlf", "Write-Output 'a'\r\nWrite-Output 'b'\r\n", "Write-Output 'a'\nWrite-Output 'b'\n"},
		{"utf8 bom only", "\xef\xbb\xbf", ""},
		{"utf8 bom ascii lf", "\xef\xbb\xbfWrite-Output 'hi'\n", "Write-Output 'hi'\n"},
		{"utf8 bom ascii crlf", "\xef\xbb\xbfWrite-Output 'a'\r\nWrite-Output 'b'\r\n", "Write-Output 'a'\nWrite-Output 'b'\n"},
		{"utf8 bom non-ascii lf", "\xef\xbb\xbfWrite-Output 'caf\u00e9'\n", "Write-Output 'caf\u00e9'\n"},
		{"utf8 bom non-ascii crlf", "\xef\xbb\xbfWrite-Output 'caf\u00e9'\r\nWrite-Output '\u2603'\r\n", "Write-Output 'caf\u00e9'\nWrite-Output '\u2603'\n"},
	}
	for _, tc := range passCases {
		t.Run("pass/"+tc.name, func(t *testing.T) {
			got, rule := CheckPowerShellEncoding(tc.raw)
			if rule != RuleNone || got != tc.want {
				t.Fatalf("CheckPowerShellEncoding(%q) = (%q, %v), want (%q, RuleNone)", tc.raw, got, rule, tc.want)
			}
		})
	}

	failCases := []struct {
		name string
		raw  string
	}{
		{"bomless non-ascii", "Write-Output 'secret caf\u00e9'\n"},
		{"bomless high byte", "Write-Output 'secret \x80'\n"},
		{"lone cr mid", "Write-Output 'secret'\rWrite-Output 'b'\n"},
		{"lone cr eof", "Write-Output 'secret'\r"},
		{"double cr before lf", "Write-Output 'secret'\r\r\n"},
		{"utf8 bom lone cr", "\xef\xbb\xbfWrite-Output 'secret'\rWrite-Output 'b'\n"},
		{"utf16 le bom", "\xff\xfeW\x00r\x00i\x00t\x00e\x00"},
		{"utf16 be bom", "\xfe\xff\x00W\x00r\x00i\x00t\x00e"},
		{"nul ascii", "Write-Output 'secret\x00'\n"},
		{"nul after utf8 bom", "\xef\xbb\xbfWrite-Output 'secret\x00'\n"},
		{"invalid utf8 after utf8 bom", "\xef\xbb\xbfWrite-Output 'secret \xff\xfe'\n"},
	}
	for _, tc := range failCases {
		t.Run("fail/"+tc.name, func(t *testing.T) {
			got, rule := CheckPowerShellEncoding(tc.raw)
			if rule != RuleEncoding || got != "" {
				t.Fatalf("CheckPowerShellEncoding(%q) = (%q, %v), want (\"\", RuleEncoding)", tc.raw, got, rule)
			}
		})
	}
}

func TestClassifyShebang(t *testing.T) {
	exact256 := "#!/" + strings.Repeat("a", 256-len("#!/")-len("/sh")) + "/sh"
	if len(exact256) != 256 {
		t.Fatalf("test setup: len(exact256) = %d, want 256", len(exact256))
	}
	over256 := exact256 + "x"

	cases := []struct {
		name string
		line string
		want ShebangKind
	}{
		// Non-shebang
		{"empty", "", ShebangNone},
		{"plain comment", "# /bin/sh", ShebangNone},
		{"leading space before shebang", " #!/bin/sh", ShebangNone},
		{"command line", "echo hi", ShebangNone},

		// POSIX shells
		{"bin sh", "#!/bin/sh", ShebangPOSIX},
		{"space after bang", "#! /bin/sh", ShebangPOSIX},
		{"tabs and spaces", "#!\t  /bin/sh  \t", ShebangPOSIX},
		{"bin bash", "#!/bin/bash", ShebangPOSIX},
		{"bin dash", "#!/bin/dash", ShebangPOSIX},
		{"bin ksh", "#!/bin/ksh", ShebangPOSIX},
		{"env bash", "#!/usr/bin/env bash", ShebangPOSIX},
		{"env -S FOO=1 sh", "#!/usr/bin/env -S FOO=1 sh", ShebangPOSIX},
		{"env -S empty value sh", "#!/usr/bin/env -S FOO= sh", ShebangPOSIX},
		{"env -u X dash", "#!/usr/bin/env -u X dash", ShebangPOSIX},
		{"env -i -v -- ksh", "#!/usr/bin/env -i -v -- ksh", ShebangPOSIX},
		{"env -- FOO=bar sh", "#!/usr/bin/env -- FOO=bar sh", ShebangPOSIX},
		{"uppercase SH.EXE", "#!/bin/SH.EXE", ShebangPOSIX},
		{"exact 256 bytes", exact256, ShebangPOSIX},
		{"env -S sh with clean flag", "#!/usr/bin/env -S sh -eu", ShebangPOSIX},

		// PowerShell
		{"pwsh direct", "#!/usr/bin/pwsh", ShebangPowerShell},
		{"powershell direct", "#!/usr/bin/powershell", ShebangPowerShell},
		{"env -S pwsh -NoProfile", "#!/usr/bin/env -S pwsh -NoProfile", ShebangPowerShell},
		{"uppercase PWSH.EXE", "#!/usr/bin/PWSH.EXE", ShebangPowerShell},
		{"env pwsh.exe", "#!/usr/bin/env pwsh.exe", ShebangPowerShell},

		// Batch
		{"env cmd", "#!/usr/bin/env cmd", ShebangBatch},
		{"cmd.exe", "#!/windows/system32/CMD.EXE", ShebangBatch},
		{"windows drive cmd.exe", "#!C:/Windows/System32/cmd.exe", ShebangBatch},

		// Other shells
		{"env zsh", "#!/usr/bin/env zsh", ShebangOtherShell},
		{"env fish", "#!/usr/bin/env fish", ShebangOtherShell},
		{"bin csh", "#!/bin/csh", ShebangOtherShell},
		{"bin tcsh", "#!/bin/tcsh", ShebangOtherShell},

		// Non-shell interpreters
		{"bin ash", "#!/bin/ash", ShebangNonShell},
		{"bin mksh", "#!/bin/mksh", ShebangNonShell},
		{"bin pdksh", "#!/bin/pdksh", ShebangNonShell},
		{"bin yash", "#!/bin/yash", ShebangNonShell},
		{"env nushell", "#!/usr/bin/env nushell", ShebangNonShell},
		{"env nu", "#!/usr/bin/env nu", ShebangNonShell},
		{"env FOO=bar_1/baz python3", "#!/usr/bin/env FOO=bar_1/baz python3", ShebangNonShell},
		{"env PATH with colons python3", "#!/usr/bin/env PATH=/usr/local/bin:/usr/bin python3", ShebangNonShell},
		{"windows drive python3.exe", "#!C:/tools/python3.exe", ShebangNonShell},
		{"python", "#!/usr/bin/python", ShebangNonShell},
		{"node", "#!/usr/bin/env node", ShebangNonShell},
		{"perl", "#!/usr/bin/perl -w", ShebangNonShell},
		{"ruby", "#!/usr/bin/env ruby", ShebangNonShell},
		{"awk", "#!/usr/bin/awk -f", ShebangNonShell},
		{"sed", "#!/bin/sed -f", ShebangNonShell},
		{"php", "#!/usr/bin/env php", ShebangNonShell},
		{"double .exe suffix", "#!/bin/sh.exe.exe", ShebangNonShell},

		// Unparseable
		{"bare shebang", "#!", ShebangUnparseable},
		{"drive-relative sh", "#!C:sh", ShebangUnparseable},
		{"drive-relative python3", "#!C:python3", ShebangUnparseable},
		{"drive-relative cmd.exe", "#!C:cmd.exe", ShebangUnparseable},
		{"whitespace only", "#!   \t ", ShebangUnparseable},
		{"over 256 bytes", over256, ShebangUnparseable},
		{"internal lf", "#!/bin/sh\necho hi", ShebangUnparseable},
		{"internal cr", "#!/bin/sh\r", ShebangUnparseable},
		{"internal vtab", "#!/bin/sh\v-e", ShebangUnparseable},
		{"internal formfeed", "#!/bin/sh\f-e", ShebangUnparseable},
		{"internal nul", "#!/bin/sh\x00", ShebangUnparseable},
		{"internal nbsp", "#!/bin/sh\u00a0-e", ShebangUnparseable},
		{"non-ascii path", "#!/bin/sh\u00e9", ShebangUnparseable},
		{"trailing slash", "#!/bin/sh/", ShebangUnparseable},
		{"dot basename", "#!/bin/.", ShebangUnparseable},
		{"dotdot basename", "#!/bin/..", ShebangUnparseable},
		{"dot exe basename", "#!/bin/..exe", ShebangUnparseable},
		{"bare .exe", "#!.exe", ShebangUnparseable},
		{"path .exe", "#!/bin/.exe", ShebangUnparseable},
		{"leading dash interpreter", "#!-sh", ShebangUnparseable},
		{"env alone", "#!/usr/bin/env", ShebangUnparseable},
		{"env -S alone", "#!/usr/bin/env -S", ShebangUnparseable},
		{"env -- alone", "#!/usr/bin/env --", ShebangUnparseable},
		{"env -u alone", "#!/usr/bin/env -u", ShebangUnparseable},
		{"env -u VAR alone", "#!/usr/bin/env -u VAR", ShebangUnparseable},
		{"env -u invalid name", "#!/usr/bin/env -u 1BAD sh", ShebangUnparseable},
		{"env FOO=1 alone", "#!/usr/bin/env FOO=1", ShebangUnparseable},
		{"env nested", "#!/usr/bin/env env sh", ShebangUnparseable},
		{"env nested with path", "#!/usr/bin/env /usr/bin/env sh", ShebangUnparseable},
		{"env unsupported flag -C", "#!/usr/bin/env -C /tmp sh", ShebangUnparseable},
		{"env unsupported flag -P", "#!/usr/bin/env -P /bin sh", ShebangUnparseable},
		{"env unsupported combined -Si", "#!/usr/bin/env -Si sh", ShebangUnparseable},
		{"env unsupported -S=sh", "#!/usr/bin/env -S=sh", ShebangUnparseable},
		{"env -S quoted arg", `#!/usr/bin/env -S "sh -x"`, ShebangUnparseable},
		{"env -S trailing quoted arg", `#!/usr/bin/env -S sh -c "id"`, ShebangUnparseable},
		{"env -S trailing single quote", `#!/usr/bin/env -S sh 'id'`, ShebangUnparseable},
		{"env -S trailing dollar", `#!/usr/bin/env -S sh $ARG`, ShebangUnparseable},
		{"env -S trailing backtick", "#!/usr/bin/env -S sh `id`", ShebangUnparseable},
		{"env -S trailing semicolon", `#!/usr/bin/env -S sh ;id`, ShebangUnparseable},
		{"env -S trailing pipe", `#!/usr/bin/env -S sh |cat`, ShebangUnparseable},
		{"env -S trailing equals", `#!/usr/bin/env -S sh FOO=1`, ShebangUnparseable},
		{"env invalid assignment value", `#!/usr/bin/env FOO=$BAR sh`, ShebangUnparseable},
		{"env invalid assignment name", `#!/usr/bin/env 1FOO=bar sh`, ShebangUnparseable},
		{"meta dollar in interpreter", "#!/bin/$SHELL", ShebangUnparseable},
		{"meta backtick in interpreter", "#!/bin/`sh`", ShebangUnparseable},
		{"meta semicolon in interpreter", "#!/bin/sh;id", ShebangUnparseable},
		{"meta pipe in interpreter", "#!/bin/sh|cat", ShebangUnparseable},
		{"meta single quote in interpreter", "#!/bin/'sh'", ShebangUnparseable},
		{"meta double quote in interpreter", `#!/bin/"sh"`, ShebangUnparseable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyShebang(tc.line); got != tc.want {
				t.Fatalf("ClassifyShebang(%q) = %d, want %d", tc.line, got, tc.want)
			}
		})
	}
}

func TestRefusalIsOpaqueEval(t *testing.T) {
	origin := &ScriptOrigin{Path: "/ws/t.sh", Line: 2}
	if !opaqueRefusal(constructEval, nil).IsOpaqueEval() {
		t.Error("opaqueRefusal(constructEval, nil).IsOpaqueEval() = false, want true")
	}
	if !opaqueRefusal(constructEval, origin).IsOpaqueEval() {
		t.Error("opaqueRefusal(constructEval, origin).IsOpaqueEval() = false, want true")
	}
	nonEval := []struct {
		name string
		r    Refusal
	}{
		{"zero", Refusal{}},
		{"opaque trap", opaqueRefusal(constructTrap, origin)},
		{"opaque source", opaqueRefusal(constructSource, nil)},
		{"named rule encoding", ruleRefusal(RuleEncoding, origin)},
		{"named rule opaque without construct", ruleRefusal(RuleOpaque, nil)},
		{"form refusal", formRefusal(formPowerShellContract, "pwsh")},
		{"cancel refusal", cancelRefusal(POSIX)},
		{"guard refusal", GuardRefusal(GuardBashCanceled)},
	}
	for _, tc := range nonEval {
		t.Run(tc.name, func(t *testing.T) {
			if tc.r.IsOpaqueEval() {
				t.Errorf("%s (%q): IsOpaqueEval() = true, want false", tc.name, tc.r)
			}
		})
	}
}

type stubScriptProber struct{}

func (stubScriptProber) ProbeExists(string) (bool, Rule) { return true, RuleNone }

func TestScriptReadAndHeaderAreValueComparable(t *testing.T) {
	var prober ScriptProber = stubScriptProber{}
	if ok, rule := prober.ProbeExists("/ws/a.sh"); !ok || rule != RuleNone {
		t.Fatalf("ProbeExists = (%v, %v), want (true, RuleNone)", ok, rule)
	}
	h := ScriptHeader{HasNUL: false, BinaryMagic: false, Shebang: "#!/bin/sh"}
	r1 := ScriptRead{
		Path:              "/ws/a.sh",
		ID:                "id-1",
		Text:              "echo hi\n",
		Raw:               "echo hi\n",
		SHA256:            "abc",
		Bytes:             8,
		Limit:             RuleNone,
		NotExist:          false,
		IsSymlink:         true,
		TooLarge:          false,
		Header:            h,
		ResolvedHeader:    h,
		HasResolvedHeader: true,
	}
	r2 := r1
	if r1 != r2 {
		t.Fatalf("identical ScriptRead values compared unequal: %+v vs %+v", r1, r2)
	}
	r2.TooLarge = true
	if r1 == r2 {
		t.Fatal("distinct ScriptRead values compared equal")
	}
}
