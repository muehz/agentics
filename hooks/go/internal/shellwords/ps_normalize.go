package shellwords

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

var psBuiltinModulePrefixes = [...]string{
	`microsoft.powershell.utility\`,
	`microsoft.powershell.management\`,
	`microsoft.powershell.core\`,
	`microsoft.powershell.security\`,
	`microsoft.powershell.archive\`,
	`microsoft.powershell.host\`,
}

// psCanonicalAliases maps slash-less, non-.exe PowerShell built-in aliases to
// their canonical lowercase cmdlet names. External tool names curl and wget
// stay excluded so upload and transfer rules preserve their distinct identity.
var psCanonicalAliases = map[string]string{
	// Evaluators and launchers.
	"iex":   "invoke-expression",
	"icm":   "invoke-command",
	"saps":  "start-process",
	"start": "start-process",
	"ipmo":  "import-module",

	// Destructive, file writers, and movers.
	"ri":    "remove-item",
	"rm":    "remove-item",
	"del":   "remove-item",
	"erase": "remove-item",
	"rd":    "remove-item",
	"rmdir": "remove-item",
	"mi":    "move-item",
	"mv":    "move-item",
	"move":  "move-item",
	"cpi":   "copy-item",
	"cp":    "copy-item",
	"copy":  "copy-item",
	"rni":   "rename-item",
	"ren":   "rename-item",
	"ni":    "new-item",
	"sc":    "set-content",
	"ac":    "add-content",
	"clc":   "clear-content",
	"si":    "set-item",

	// Readers.
	"gc":   "get-content",
	"cat":  "get-content",
	"type": "get-content",
	"sls":  "select-string",
	"gci":  "get-childitem",
	"ls":   "get-childitem",
	"dir":  "get-childitem",
	"gi":   "get-item",

	// Directory and shell state.
	"sl":    "set-location",
	"cd":    "set-location",
	"chdir": "set-location",
	"pushd": "push-location",
	"popd":  "pop-location",
	"sal":   "set-alias",
	"nal":   "new-alias",
	"sv":    "set-variable",
	"rv":    "remove-variable",

	// Downloaders.
	"iwr": "invoke-webrequest",
	"irm": "invoke-restmethod",

	// Pipeline and iteration helpers.
	"%":       "foreach-object",
	"foreach": "foreach-object",
	"?":       "where-object",
	"where":   "where-object",
	"select":  "select-object",
	"fl":      "format-list",
	"ft":      "format-table",
	"sort":    "sort-object",
	"echo":    "write-output",
	"write":   "write-output",
}

var psStartProcessBlockedHosts = map[string]bool{
	"sh":             true,
	"bash":           true,
	"dash":           true,
	"ksh":            true,
	"zsh":            true,
	"fish":           true,
	"csh":            true,
	"tcsh":           true,
	"pwsh":           true,
	"powershell":     true,
	"powershell_ise": true,
	"cmd":            true,
	"wscript":        true,
	"cscript":        true,
	"mshta":          true,
	"rundll32":       true,
	"regsvr32":       true,
}

var psStartProcessBlockedExts = map[string]bool{
	".ps1":  true,
	".psm1": true,
	".psd1": true,
	".sh":   true,
	".bash": true,
	".dash": true,
	".ksh":  true,
	".zsh":  true,
	".fish": true,
	".csh":  true,
	".tcsh": true,
	".bat":  true,
	".cmd":  true,
	".vbs":  true,
	".vbe":  true,
	".js":   true,
	".jse":  true,
	".wsf":  true,
	".wsh":  true,
	".hta":  true,
	".scr":  true,
	".pif":  true,
	".com":  true,
	".msi":  true,
	".msp":  true,
	".cpl":  true,
	".inf":  true,
	".reg":  true,
}

func isPSDashRune(r rune) bool {
	return r == '-' || r == '\u2013' || r == '\u2014' || r == '\u2015'
}

func isAllPSDashes(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isPSDashRune(r) {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			b := []byte(s)
			for ; i < len(b); i++ {
				if b[i] >= 'A' && b[i] <= 'Z' {
					b[i] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func hasExeSuffix(s string) bool {
	return len(s) >= 4 && strings.EqualFold(s[len(s)-4:], ".exe")
}

// isUnquotedFlagHead reports whether raw starts with an unquoted PowerShell
// parameter flag head (before any colon-attached argument).
func isUnquotedFlagHead(raw string) bool {
	head, _, _ := strings.Cut(raw, ":")
	if head == "" {
		return false
	}
	first, size := utf8.DecodeRuneInString(head)
	if !isPSDashRune(first) {
		return false
	}
	i := size
	for i < len(head) {
		r, w := utf8.DecodeRuneInString(head[i:])
		if !isPSDashRune(r) {
			break
		}
		i += w
	}
	if i >= len(head) {
		return false
	}
	for _, r := range head {
		if unicode.IsSpace(r) {
			return false
		}
		switch r {
		case '\'', '"', '`', '$', '@', '(', ')', '{', '}':
			return false
		}
	}
	return true
}

func normalizeLeadingDashes(text string) string {
	if text == "" {
		return text
	}
	first, _ := utf8.DecodeRuneInString(text)
	if !isPSDashRune(first) {
		return text
	}
	var b strings.Builder
	i := 0
	changed := false
	for i < len(text) {
		r, w := utf8.DecodeRuneInString(text[i:])
		if !isPSDashRune(r) {
			break
		}
		if r != '-' {
			changed = true
		}
		b.WriteByte('-')
		i += w
	}
	if !changed {
		return text
	}
	b.WriteString(text[i:])
	return b.String()
}

func stripBuiltinPSModulePrefix(decoded string) (cmdlet string, moduleStripped bool) {
	lower := asciiLower(decoded)
	for _, prefix := range psBuiltinModulePrefixes {
		if strings.HasPrefix(lower, prefix) {
			rest := decoded[len(prefix):]
			if rest != "" && !strings.HasPrefix(rest, ".") && !strings.ContainsAny(rest, `/\:`) {
				return rest, true
			}
			return "", false
		}
	}
	return "", false
}

// normalizePSWords normalizes Words[0] command aliases/module prefixes and
// leading Unicode dashes on unquoted parameter flags in Words[1:], preserving
// Words[0].Raw and letter case on Words[1:].
func normalizePSWords(words []Word) (rawCommand string, moduleStripped bool) {
	if len(words) == 0 {
		return "", false
	}
	decoded := words[0].Text
	rawCommand = decoded
	if words[0].Literal && !hasExeSuffix(decoded) {
		cmdlet, stripped := stripBuiltinPSModulePrefix(decoded)
		moduleStripped = stripped
		if stripped {
			lower := asciiLower(cmdlet)
			if target, ok := psCanonicalAliases[lower]; ok {
				words[0].Text = target
			} else {
				words[0].Text = lower
			}
		} else if !strings.ContainsAny(decoded, `/\`) {
			if target, ok := psCanonicalAliases[asciiLower(decoded)]; ok {
				words[0].Text = target
			}
		}
	}
	for i := 1; i < len(words); i++ {
		if isUnquotedFlagHead(words[i].Raw) {
			words[i].Text = normalizeLeadingDashes(words[i].Text)
		}
	}
	return rawCommand, moduleStripped
}

// isPSScalarRaw reports whether raw represents a single scalar token (no
// unquoted comma-list, @(...)/@{...}, parenthesized expression, or scriptblock).
func isPSScalarRaw(raw string) bool {
	if raw == "" {
		return false
	}
	inSingle, inDouble := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(raw) && raw[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '`' && i+1 < len(raw) {
				i++
				continue
			}
			if c == '"' {
				if i+1 < len(raw) && raw[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch c {
		case '`':
			if i+1 < len(raw) {
				i++
			}
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case ',', '@', '{', '}', '(', ')':
			return false
		}
	}
	return !inSingle && !inDouble
}

func hasUnquotedBrace(raw string) bool {
	inSingle, inDouble := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(raw) && raw[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '`' && i+1 < len(raw) {
				i++
				continue
			}
			if c == '"' {
				if i+1 < len(raw) && raw[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch c {
		case '`':
			if i+1 < len(raw) {
				i++
			}
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '{', '}':
			return true
		}
	}
	return false
}

// isTypedStartProcessAllowed evaluates the closed parameter and target
// contract for Start-Process in typed PowerShell mode.
func isTypedStartProcessAllowed(words []Word) bool {
	if len(words) < 2 {
		return false
	}
	var (
		hasFilePath, hasArgList, hasWorkDir, hasWinStyle bool
		hasNoNewWindow, hasWait, hasPassThru             bool
		filePath                                         string
		posCount                                         int
	)
	for i := 1; i < len(words); i++ {
		w := words[i]
		if isUnquotedFlagHead(w.Raw) {
			flagName, valText, hasColon := strings.Cut(w.Text, ":")
			_, valRaw, _ := strings.Cut(w.Raw, ":")
			flagLower := asciiLower(normalizeLeadingDashes(flagName))
			readScalar := func() (string, bool) {
				if hasColon {
					if !w.Literal || valRaw == "" || valText == "" || !isPSScalarRaw(valRaw) {
						return "", false
					}
					return valText, true
				}
				if i+1 >= len(words) {
					return "", false
				}
				i++
				next := words[i]
				if isUnquotedFlagHead(next.Raw) || !next.Literal || next.Raw == "" || next.Text == "" || !isPSScalarRaw(next.Raw) {
					return "", false
				}
				return next.Text, true
			}
			switch {
			case len(flagLower) >= 2 && strings.HasPrefix("-filepath", flagLower):
				if hasFilePath {
					return false
				}
				val, ok := readScalar()
				if !ok {
					return false
				}
				hasFilePath = true
				filePath = val
			case len(flagLower) >= 2 && strings.HasPrefix("-argumentlist", flagLower):
				if hasArgList {
					return false
				}
				if hasColon {
					if !w.Literal || valRaw == "" || hasUnquotedBrace(valRaw) {
						return false
					}
				} else {
					if i+1 >= len(words) {
						return false
					}
					i++
					next := words[i]
					if !next.Literal || next.Raw == "" || hasUnquotedBrace(next.Raw) {
						return false
					}
				}
				hasArgList = true
			case len(flagLower) >= 3 && strings.HasPrefix("-workingdirectory", flagLower):
				if hasWorkDir {
					return false
				}
				if _, ok := readScalar(); !ok {
					return false
				}
				hasWorkDir = true
			case len(flagLower) >= 3 && strings.HasPrefix("-windowstyle", flagLower):
				if hasWinStyle {
					return false
				}
				val, ok := readScalar()
				if !ok {
					return false
				}
				switch asciiLower(val) {
				case "normal", "hidden", "minimized", "maximized":
				default:
					return false
				}
				hasWinStyle = true
			case flagLower == "-nnw" || (len(flagLower) >= 3 && strings.HasPrefix("-nonewwindow", flagLower)):
				if hasNoNewWindow || (hasColon && !isAllowedSwitchBoolRaw(valRaw)) {
					return false
				}
				hasNoNewWindow = true
			case len(flagLower) >= 4 && strings.HasPrefix("-wait", flagLower):
				if hasWait || (hasColon && !isAllowedSwitchBoolRaw(valRaw)) {
					return false
				}
				hasWait = true
			case len(flagLower) >= 3 && strings.HasPrefix("-passthru", flagLower):
				if hasPassThru || (hasColon && !isAllowedSwitchBoolRaw(valRaw)) {
					return false
				}
				hasPassThru = true
			default:
				return false
			}
			continue
		}
		if !w.Quoted && isAllPSDashes(w.Raw) {
			return false
		}
		posCount++
		if posCount >= 3 {
			return false
		}
		if !hasFilePath {
			if !w.Literal || w.Text == "" || !isPSScalarRaw(w.Raw) {
				return false
			}
			hasFilePath = true
			filePath = w.Text
		} else if !hasArgList {
			if !w.Literal || w.Raw == "" || hasUnquotedBrace(w.Raw) {
				return false
			}
			hasArgList = true
		} else {
			return false
		}
	}
	if !hasFilePath || filePath == "" {
		return false
	}
	return classifyStartProcessTarget(filePath)
}

func isAllowedSwitchBoolRaw(valRaw string) bool {
	return strings.EqualFold(valRaw, "true") ||
		strings.EqualFold(valRaw, "false") ||
		strings.EqualFold(valRaw, "$true") ||
		strings.EqualFold(valRaw, "$false")
}

func classifyStartProcessTarget(target string) bool {
	if target == "" {
		return false
	}
	// 1. Validated http:// or https:// URLs first.
	lower := asciiLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		rest := target[7:]
		if strings.HasPrefix(lower, "https://") {
			rest = target[8:]
		}
		if rest == "" {
			return false
		}
		hostFirst, _ := utf8.DecodeRuneInString(rest)
		if hostFirst == utf8.RuneError && len(rest) == 1 {
			return false
		}
		if unicode.IsSpace(hostFirst) || unicode.IsControl(hostFirst) || hostFirst == '/' || hostFirst == '\\' {
			return false
		}
		for _, r := range rest {
			if unicode.IsControl(r) || unicode.IsSpace(r) {
				return false
			}
		}
		return true
	}

	// 2. Other URI schemes anchored ^[A-Za-z][A-Za-z0-9+.-]+:
	if hasOtherURIScheme(target) {
		return false
	}

	// 3. UNC, device, drive-relative, provider, ADS, leading '-', trailing '.' or whitespace.
	if strings.HasPrefix(target, `\\`) ||
		strings.HasPrefix(target, `//`) ||
		strings.HasPrefix(target, `\??\`) ||
		strings.HasPrefix(target, `\/`) ||
		strings.HasPrefix(target, `/\`) ||
		strings.Contains(target, "::") ||
		strings.HasPrefix(target, "-") ||
		strings.HasSuffix(target, ".") ||
		strings.TrimRightFunc(target, unicode.IsSpace) != target {
		return false
	}
	for _, r := range target {
		if unicode.IsControl(r) {
			return false
		}
	}
	if colon := strings.IndexByte(target, ':'); colon >= 0 {
		if colon != 1 || len(target) < 3 || !isASCIILetter(target[0]) || (target[2] != '/' && target[2] != '\\') {
			return false
		}
		if strings.IndexByte(target[2:], ':') >= 0 {
			return false
		}
	}

	// 4. Basename check.
	base := target
	if slash := strings.LastIndexAny(target, `/\`); slash >= 0 {
		base = target[slash+1:]
	}
	if base == "" || strings.EqualFold(base, ".exe") {
		return false
	}

	// 5. Shell and launcher blocklist (with or without .exe).
	baseLower := asciiLower(base)
	stem := strings.TrimSuffix(baseLower, ".exe")
	if psStartProcessBlockedHosts[baseLower] || psStartProcessBlockedHosts[stem] {
		return false
	}

	// 6. Disallowed script, batch, and installer extensions.
	ext := asciiLower(path.Ext(base))
	if psStartProcessBlockedExts[ext] {
		return false
	}

	// 7. Path-bearing targets must end in .exe; bare names passing steps 1-6 pass.
	if strings.ContainsAny(target, `/\:`) {
		return hasExeSuffix(base)
	}
	return true
}

func hasOtherURIScheme(s string) bool {
	if len(s) < 3 || !isASCIILetter(s[0]) {
		return false
	}
	i := 1
	for i < len(s) {
		c := s[i]
		if isASCIILetter(c) || (c >= '0' && c <= '9') || c == '+' || c == '.' || c == '-' {
			i++
			continue
		}
		break
	}
	return i >= 2 && i < len(s) && s[i] == ':'
}
