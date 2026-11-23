package shellwords

import (
	"strings"
	"unicode/utf8"
)

// AgxLauncherSHA256 is the lowercase hex SHA-256 digest of the repository's committed LF bin/agx launcher.
const AgxLauncherSHA256 = "e8d0f5fef003f62b967b71a86bea213138ca7f4a2d414990f67c149e23c1ffc6"

const (
	utf8BOM    = "\xef\xbb\xbf"
	utf16LEBOM = "\xff\xfe"
	utf16BEBOM = "\xfe\xff"

	maxShebangBytes = 256
)

// CheckPOSIXEncoding validates raw as plain LF UTF-8 text. It rejects UTF-8 and UTF-16 byte-order marks, NUL bytes,
// carriage returns, and invalid UTF-8 sequences, returning ("", RuleEncoding) without echoing input bytes on failure.
func CheckPOSIXEncoding(raw string) (string, Rule) {
	if strings.HasPrefix(raw, utf8BOM) || strings.HasPrefix(raw, utf16LEBOM) || strings.HasPrefix(raw, utf16BEBOM) {
		return "", RuleEncoding
	}
	if strings.IndexByte(raw, 0) >= 0 || strings.IndexByte(raw, '\r') >= 0 {
		return "", RuleEncoding
	}
	if !utf8.ValidString(raw) {
		return "", RuleEncoding
	}
	return raw, RuleNone
}

// CheckPowerShellEncoding validates and normalizes raw for PowerShell script inspection. It rejects NUL bytes and
// UTF-16 byte-order marks; requires valid UTF-8 after stripping an optional UTF-8 BOM, or pure ASCII (< 0x80) when no
// UTF-8 BOM is present; rejects any lone CR; and normalizes CRLF to LF. On failure it returns ("", RuleEncoding)
// without echoing input bytes.
func CheckPowerShellEncoding(raw string) (string, Rule) {
	if strings.IndexByte(raw, 0) >= 0 || strings.HasPrefix(raw, utf16LEBOM) || strings.HasPrefix(raw, utf16BEBOM) {
		return "", RuleEncoding
	}
	rest := raw
	if strings.HasPrefix(raw, utf8BOM) {
		rest = raw[len(utf8BOM):]
		if !utf8.ValidString(rest) {
			return "", RuleEncoding
		}
	} else {
		for i := 0; i < len(rest); i++ {
			if rest[i] >= 0x80 {
				return "", RuleEncoding
			}
		}
	}
	if strings.IndexByte(rest, '\r') < 0 {
		return rest, RuleNone
	}
	var b strings.Builder
	b.Grow(len(rest))
	for i := 0; i < len(rest); i++ {
		if rest[i] == '\r' {
			if i+1 >= len(rest) || rest[i+1] != '\n' {
				return "", RuleEncoding
			}
			continue
		}
		b.WriteByte(rest[i])
	}
	return b.String(), RuleNone
}

// ShebangKind classifies the interpreter declared by a script's leading "#!" line.
type ShebangKind uint8

// ShebangKind values partition first lines into non-shebangs, recognized shell families, clean non-shell interpreters,
// and malformed or untrusted shebangs that must fail closed.
const (
	// ShebangNone reports that the line does not begin with "#!".
	ShebangNone ShebangKind = iota
	// ShebangPOSIX reports a POSIX shell interpreter (sh, bash, dash, ksh).
	ShebangPOSIX
	// ShebangPowerShell reports a PowerShell interpreter (pwsh, powershell).
	ShebangPowerShell
	// ShebangBatch reports a Windows batch-file interpreter (cmd, cmd.exe).
	ShebangBatch
	// ShebangOtherShell reports another recognized shell interpreter (zsh, fish, csh, tcsh).
	ShebangOtherShell
	// ShebangNonShell reports a clean non-shell interpreter.
	ShebangNonShell
	// ShebangUnparseable reports a malformed, oversized, or untrusted shebang line.
	ShebangUnparseable
)

// ClassifyShebang classifies a script's first line without executing or consulting the filesystem. Any syntax outside
// the restricted clean-token and env grammar fails closed as ShebangUnparseable.
func ClassifyShebang(line string) ShebangKind {
	if !strings.HasPrefix(line, "#!") {
		return ShebangNone
	}
	if len(line) > maxShebangBytes || strings.ContainsAny(line, "\n\r") {
		return ShebangUnparseable
	}
	rest := line[2:]
	for i := 0; i < len(rest); i++ {
		b := rest[i]
		if b == 0 || b == '\v' || b == '\f' || b >= 0x80 || (b < 0x20 && b != '\t') || b == 0x7f {
			return ShebangUnparseable
		}
	}
	fields := strings.FieldsFunc(rest, func(r rune) bool { return r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return ShebangUnparseable
	}
	base, ok := extractInterpreterBase(fields[0])
	if !ok {
		return ShebangUnparseable
	}
	if base == "env" {
		resolvedBase, ok := resolveEnvShebang(fields[1:])
		if !ok {
			return ShebangUnparseable
		}
		base = resolvedBase
	}
	switch base {
	case "sh", "bash", "dash", "ksh":
		return ShebangPOSIX
	case "pwsh", "powershell":
		return ShebangPowerShell
	case "cmd":
		return ShebangBatch
	case "zsh", "fish", "csh", "tcsh":
		return ShebangOtherShell
	default:
		return ShebangNonShell
	}
}

func resolveEnvShebang(fields []string) (string, bool) {
	seenS := false
	endOfOpts := false
	for i := 0; i < len(fields); {
		tok := fields[i]
		if !endOfOpts {
			switch {
			case tok == "--":
				endOfOpts = true
				i++
				continue
			case tok == "-S":
				seenS = true
				i++
				continue
			case tok == "-i" || tok == "-v":
				i++
				continue
			case tok == "-u":
				if i+1 >= len(fields) || !isValidEnvName(fields[i+1]) {
					return "", false
				}
				i += 2
				continue
			case strings.HasPrefix(tok, "-"):
				return "", false
			}
		}
		if eq := strings.IndexByte(tok, '='); eq > 0 && isValidEnvName(tok[:eq]) {
			val := tok[eq+1:]
			if val != "" && !isCleanShebangToken(val) {
				return "", false
			}
			i++
			continue
		}
		interp, ok := extractInterpreterBase(tok)
		if !ok || interp == "env" {
			return "", false
		}
		if seenS {
			for _, arg := range fields[i+1:] {
				if !isCleanShebangToken(arg) {
					return "", false
				}
			}
		}
		return interp, true
	}
	return "", false
}

func isCleanShebangToken(tok string) bool {
	if tok == "" {
		return false
	}
	for i := 0; i < len(tok); i++ {
		b := tok[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			continue
		}
		switch b {
		case '.', '_', '/', '+', ':', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func extractInterpreterBase(tok string) (string, bool) {
	if !isCleanShebangToken(tok) || strings.HasSuffix(tok, "/") || strings.HasPrefix(tok, "-") {
		return "", false
	}
	base := tok
	if idx := strings.LastIndexByte(tok, '/'); idx >= 0 {
		base = tok[idx+1:]
	}
	if base == "" || base == "." || base == ".." || strings.ContainsRune(base, ':') {
		return "", false
	}
	base = strings.ToLower(base)
	base = strings.TrimSuffix(base, ".exe")
	if base == "" || base == "." || base == ".." {
		return "", false
	}
	return base, true
}

func isValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		isLetter := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
		isDigit := b >= '0' && b <= '9'
		if i == 0 {
			if !isLetter {
				return false
			}
		} else if !isLetter && !isDigit {
			return false
		}
	}
	return true
}
