package scriptsnap

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestInspectHeaderBinaryMagicAndNUL(t *testing.T) {
	cases := []struct {
		name      string
		head      []byte
		wantNUL   bool
		wantMagic bool
	}{
		{
			name:      "empty",
			head:      nil,
			wantNUL:   false,
			wantMagic: false,
		},
		{
			name:      "plain_text",
			head:      []byte("echo hello\n"),
			wantNUL:   false,
			wantMagic: false,
		},
		{
			name:      "elf_without_nul",
			head:      []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x01},
			wantNUL:   false,
			wantMagic: true,
		},
		{
			name:      "elf_with_nul",
			head:      []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00},
			wantNUL:   true,
			wantMagic: true,
		},
		{
			name:      "truncated_elf",
			head:      []byte{0x7f, 0x45, 0x4c},
			wantNUL:   false,
			wantMagic: false,
		},
		{
			name:      "macho_32_be",
			head:      []byte{0xfe, 0xed, 0xfa, 0xce, 0x01, 0x02},
			wantNUL:   false,
			wantMagic: true,
		},
		{
			name:      "macho_32_le",
			head:      []byte{0xce, 0xfa, 0xed, 0xfe, 0x01, 0x02},
			wantNUL:   false,
			wantMagic: true,
		},
		{
			name:      "macho_64_be",
			head:      []byte{0xfe, 0xed, 0xfa, 0xcf, 0x01, 0x02},
			wantNUL:   false,
			wantMagic: true,
		},
		{
			name:      "macho_64_le",
			head:      []byte{0xcf, 0xfa, 0xed, 0xfe, 0x01, 0x02},
			wantNUL:   false,
			wantMagic: true,
		},
		{
			name:      "fat_macho_be",
			head:      []byte{0xca, 0xfe, 0xba, 0xbe, 0x00, 0x00, 0x00, 0x02},
			wantNUL:   true,
			wantMagic: true,
		},
		{
			name:      "fat_macho_le",
			head:      []byte{0xbe, 0xba, 0xfe, 0xca, 0x02, 0x01, 0x01, 0x01},
			wantNUL:   false,
			wantMagic: true,
		},
		{
			name:      "pe_mz_with_nul",
			head:      []byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00},
			wantNUL:   true,
			wantMagic: true,
		},
		{
			name:      "pe_mz_three_bytes_with_nul",
			head:      []byte{'M', 'Z', 0x00},
			wantNUL:   true,
			wantMagic: true,
		},
		{
			name:      "mz_plain_text_without_nul",
			head:      []byte("MZ is initials in plain text\necho ok\n"),
			wantNUL:   false,
			wantMagic: false,
		},
		{
			name:      "mz_two_bytes_only",
			head:      []byte("MZ"),
			wantNUL:   false,
			wantMagic: false,
		},
		{
			name:      "text_with_nul",
			head:      []byte("echo hi\x00there\n"),
			wantNUL:   true,
			wantMagic: false,
		},
		{
			name:      "pe_nul_at_4095_inside_cap",
			head:      []byte("MZ" + strings.Repeat("a", 4093) + "\x00"),
			wantNUL:   true,
			wantMagic: true,
		},
		{
			name:      "pe_nul_at_4096_past_cap",
			head:      []byte("MZ" + strings.Repeat("a", 4094) + "\x00"),
			wantNUL:   false,
			wantMagic: false,
		},
		{
			name:      "nul_at_4096_past_cap",
			head:      []byte(strings.Repeat("x", 4096) + "\x00"),
			wantNUL:   false,
			wantMagic: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inspectHeader(tc.head)
			if got.HasNUL != tc.wantNUL || got.BinaryMagic != tc.wantMagic || got.Shebang != "" {
				t.Fatalf("inspectHeader(%s) = %+v, want HasNUL=%v BinaryMagic=%v Shebang=\"\"", tc.name, got, tc.wantNUL, tc.wantMagic)
			}
		})
	}
}

func TestInspectHeaderShebang(t *testing.T) {
	exact256 := "#!/bin/" + strings.Repeat("a", 256-len("#!/bin/"))
	over257 := exact256 + "b"
	over300 := "#!/bin/" + strings.Repeat("a", 300)

	cases := []struct {
		name     string
		head     []byte
		wantLine string
		wantKind shellwords.ShebangKind
	}{
		{
			name:     "not_shebang",
			head:     []byte("# comment\necho hi\n"),
			wantLine: "",
			wantKind: shellwords.ShebangNone,
		},
		{
			name:     "leading_space_before_hashbang",
			head:     []byte(" #!/bin/sh\n"),
			wantLine: "",
			wantKind: shellwords.ShebangNone,
		},
		{
			name:     "single_hash",
			head:     []byte("#"),
			wantLine: "",
			wantKind: shellwords.ShebangNone,
		},
		{
			name:     "bare_hashbang_eof",
			head:     []byte("#!"),
			wantLine: "#!",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "bare_hashbang_lf",
			head:     []byte("#!\n"),
			wantLine: "#!",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "bare_hashbang_crlf",
			head:     []byte("#!\r\n"),
			wantLine: "#!",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "lf_posix",
			head:     []byte("#!/bin/sh\necho hi\n"),
			wantLine: "#!/bin/sh",
			wantKind: shellwords.ShebangPOSIX,
		},
		{
			name:     "crlf_stripped_cleanly",
			head:     []byte("#!/usr/bin/env pwsh\r\nWrite-Host hi\r\n"),
			wantLine: "#!/usr/bin/env pwsh",
			wantKind: shellwords.ShebangPowerShell,
		},
		{
			name:     "double_cr_before_lf_preserves_earlier_cr",
			head:     []byte("#!/bin/sh\r\r\necho hi\n"),
			wantLine: "#!/bin/sh\r",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "lone_cr_at_eof_preserved",
			head:     []byte("#!/bin/sh\r"),
			wantLine: "#!/bin/sh\r",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "internal_cr_before_lf_preserved",
			head:     []byte("#!/bin/sh\recho hi\n"),
			wantLine: "#!/bin/sh\recho hi",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "internal_cr_without_lf_preserved",
			head:     []byte("#!/bin/sh\recho hi"),
			wantLine: "#!/bin/sh\recho hi",
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "no_newline_at_eof",
			head:     []byte("#!/bin/bash"),
			wantLine: "#!/bin/bash",
			wantKind: shellwords.ShebangPOSIX,
		},
		{
			name:     "exact_256_lf",
			head:     []byte(exact256 + "\necho hi\n"),
			wantLine: exact256,
			wantKind: shellwords.ShebangNonShell,
		},
		{
			name:     "exact_256_crlf",
			head:     []byte(exact256 + "\r\necho hi\n"),
			wantLine: exact256,
			wantKind: shellwords.ShebangNonShell,
		},
		{
			name:     "exact_256_eof",
			head:     []byte(exact256),
			wantLine: exact256,
			wantKind: shellwords.ShebangNonShell,
		},
		{
			name:     "exact_257_lf_rejected",
			head:     []byte(over257 + "\necho hi\n"),
			wantLine: over257,
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "over_257_capped_at_257",
			head:     []byte(over300 + "\necho hi\n"),
			wantLine: over300[:257],
			wantKind: shellwords.ShebangUnparseable,
		},
		{
			name:     "over_4096_without_newline_capped_at_257",
			head:     []byte("#!/bin/" + strings.Repeat("x", 5000)),
			wantLine: ("#!/bin/" + strings.Repeat("x", 5000))[:257],
			wantKind: shellwords.ShebangUnparseable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inspectHeader(tc.head)
			if got.Shebang != tc.wantLine {
				t.Fatalf("Shebang = %q (len %d), want %q (len %d)", got.Shebang, len(got.Shebang), tc.wantLine, len(tc.wantLine))
			}
			if kind := shellwords.ClassifyShebang(got.Shebang); kind != tc.wantKind {
				t.Fatalf("ClassifyShebang(%q) = %v, want %v", got.Shebang, kind, tc.wantKind)
			}
		})
	}
}
