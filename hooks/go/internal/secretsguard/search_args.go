package secretsguard

import (
	"context"
	"os"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

type searchArguments struct {
	patterns    map[int]bool
	joinedPaths []searchPath
	reason      shellwords.GuardReason
}

type searchPath struct {
	text string
	file bool
	data bool
}

// searchReadArguments exempts proven patterns and PowerShell filter data. Unknown arity
// discards all exemptions, while already recognized file options remain reads.
func searchReadArguments(ctx context.Context, verb string, words []shellwords.Word, dialect shellwords.Dialect, configUnknown bool) (result searchArguments) {
	result.patterns = map[int]bool{}
	if verb != "grep" && verb != "rg" {
		return result
	}
	// Cancellation is an incomplete inspection, not ordinary unknown arity:
	// even harmless fallback text must not turn a canceled proof into a pass.
	defer func() {
		if ctx.Err() != nil {
			result.patterns = nil
			result.reason = shellwords.GuardSearchCanceled
		}
	}()
	valid, options, explicit, noConfig := true, true, false, false
	var operands []int
	for i := 0; i < len(words); i++ {
		if ctx.Err() != nil {
			return result
		}
		word := words[i]
		if dialect == shellwords.POSIX && searchBraceExpansion(word.Raw) {
			result.reason = shellwords.GuardSearchBraceExpansion
		}
		if !searchLiteralArgument(ctx, word, dialect) {
			valid = false
		}
		if options && word.Text == "--" {
			options = false
			continue
		}
		if !options || word.Text == "-" || !strings.HasPrefix(word.Text, "-") {
			operands = append(operands, i)
			continue
		}
		// GNU grep can stop parsing options at the first operand under
		// POSIXLY_CORRECT. No later option value has a stable data role.
		if verb == "grep" && len(operands) > 0 {
			valid = false
		}
		consume := func(kind byte, name, value string, joined bool) {
			file := searchOptionReadsFile(verb, name)
			if kind == 'e' || kind == 'f' {
				explicit = true
			}
			if joined {
				if kind == 'f' || kind == 'p' {
					result.joinedPaths = append(result.joinedPaths, searchPath{
						text: value,
						file: kind == 'f' || dialect == shellwords.PowerShell && file,
						data: dialect == shellwords.PowerShell && kind == 'p' && !file,
					})
				}
				return
			}
			if i+1 >= len(words) {
				valid = false
				return
			}
			i++
			if dialect == shellwords.POSIX && searchBraceExpansion(words[i].Raw) {
				result.reason = shellwords.GuardSearchBraceExpansion
			}
			if !searchLiteralArgument(ctx, words[i], dialect) {
				valid = false
			}
			if kind == 'e' || dialect == shellwords.PowerShell && kind == 'p' && !file {
				result.patterns[i] = true
			}
		}
		if strings.HasPrefix(word.Text, "--") {
			name, value, joined := word.Text, "", false
			for j := 0; j < len(word.Text); j++ {
				if j%1024 == 0 && ctx.Err() != nil {
					return result
				}
				if word.Text[j] == '=' {
					name, value, joined = word.Text[:j], word.Text[j+1:], true
					break
				}
			}
			// A preprocessor executes code. Retain its joined executable as
			// a read candidate without granting the command any data proof.
			if verb == "rg" && name == "--pre" && joined {
				result.joinedPaths = append(result.joinedPaths, searchPath{text: value, file: true})
			}
			kind := searchLongOption(verb, name)
			if verb == "rg" && name == "--no-config" && !joined {
				noConfig = true
			}
			if kind == 0 || kind == 'b' && joined {
				valid = false
				continue
			}
			if kind != 'b' {
				consume(kind, name, value, joined)
			}
			continue
		}
		for j := 1; j < len(word.Text); j++ {
			if j%1024 == 0 && ctx.Err() != nil {
				return result
			}
			kind := searchShortOption(verb, word.Text[j])
			if kind == 0 {
				valid = false
				break
			}
			if kind == 'b' {
				continue
			}
			consume(kind, "-"+string(word.Text[j]), word.Text[j+1:], j+1 < len(word.Text))
			break
		}
	}
	if !explicit && len(operands) > 0 {
		result.patterns[operands[0]] = true
	}
	// A config ending with a value-taking flag can consume even a visible
	// -e, so configuration uncertainty affects explicit patterns too.
	if verb == "rg" && (configUnknown || dialect == shellwords.PowerShell) && !noConfig {
		valid = false
	}
	if !valid {
		result.patterns = nil
	}
	return result
}

func searchOptionReadsFile(verb, name string) bool {
	if name == "-f" || name == "--file" {
		return true
	}
	return verb == "rg" && name == "--ignore-file" || verb == "grep" && name == "--exclude-from"
}

func searchLiteralArgument(ctx context.Context, word shellwords.Word, dialect shellwords.Dialect) bool {
	if ctx.Err() != nil {
		return false
	}
	if !word.Literal || strings.HasPrefix(word.Raw, "~") {
		return false
	}
	if dialect == shellwords.POSIX {
		return !posixExpandsPattern(word.Raw) && !searchBraceExpansion(word.Raw)
	}
	// The shared lexer preserves array/splat syntax as words. Only literal
	// scalar arguments have the external command's stable argument roles.
	var quote byte
	for i := 0; i < len(word.Raw); i++ {
		if i%1024 == 0 && ctx.Err() != nil {
			return false
		}
		c := word.Raw[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		} else if strings.ContainsRune("`$@,(){}", rune(c)) {
			return false
		}
	}
	return dialect == shellwords.PowerShell && quote == 0 && word.Text != "--%" && ctx.Err() == nil
}

// searchConfigUncertain reports whether ripgrep may load a configuration file
// that could prepend --regexp and turn the apparent implicit pattern into a
// file. It reads the process environment and the call's override containers,
// and never reads configuration contents.
//
// A null or malformed container, a null entry, and a container or name in
// another letter case cannot show that no configuration is set, since the
// executor may still apply them. Each counts as uncertain, so the exemption
// for a pattern that merely looks like a credential path is withheld.
func searchConfigUncertain(o toolcall.Overrides) bool {
	if os.Getenv("RIPGREP_CONFIG_PATH") != "" {
		return true
	}
	values, uncertain := o.Values("RIPGREP_CONFIG_PATH")
	if uncertain {
		return true
	}
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func searchConfigAssignment(word shellwords.Word) bool {
	name, value, found := strings.Cut(word.Text, "=")
	return found && name == "RIPGREP_CONFIG_PATH" && (value != "" || !word.Literal)
}

// Brace expansion changes argv before option decoding, even when the shared
// lexer can represent the original word literally.
func searchBraceExpansion(raw string) bool {
	var quote byte
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' {
				i++
			}
			continue
		}
		if c == '\\' {
			i++
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '{' || c == '}' {
			return true
		}
	}
	return false
}

// Option kinds distinguish pattern data, file reads, path filters, other
// values, and booleans. The tools intentionally have separate arity tables.
func searchShortOption(verb string, option byte) byte {
	if option == 'e' || option == 'f' {
		return option
	}
	if verb == "grep" {
		if strings.ContainsRune("EFGPivwxzcLlnHhsoqrsRIaUbZV", rune(option)) {
			return 'b'
		}
		if strings.ContainsRune("ABCm", rune(option)) {
			return 'v'
		}
		if option == 'd' || option == 'D' {
			return 'v'
		}
	} else {
		if strings.ContainsRune("FPUivwxzcLlnoqsaHhNIuS0", rune(option)) {
			return 'b'
		}
		if strings.ContainsRune("ABCEjmrM", rune(option)) {
			return 'v'
		}
		if strings.ContainsRune("gtT", rune(option)) {
			return 'p'
		}
	}
	return 0
}

func searchLongOption(verb, option string) byte {
	switch option {
	case "--regexp":
		return 'e'
	case "--file":
		return 'f'
	case "--after-context", "--before-context", "--context", "--max-count":
		return 'v'
	case "--fixed-strings", "--ignore-case", "--invert-match", "--word-regexp", "--line-regexp", "--count", "--files-with-matches", "--files-without-match", "--line-number", "--with-filename", "--no-filename", "--only-matching", "--quiet", "--text", "--null":
		return 'b'
	}
	if verb == "grep" {
		switch option {
		case "--extended-regexp", "--basic-regexp", "--perl-regexp", "--recursive", "--dereference-recursive", "--no-messages", "--line-buffered", "--null-data", "--initial-tab", "--byte-offset", "--unix-byte-offsets", "--binary":
			return 'b'
		case "--include", "--exclude", "--exclude-dir", "--exclude-from":
			return 'p'
		case "--binary-files", "--devices", "--directories", "--label", "--group-separator":
			return 'v'
		}
	} else {
		switch option {
		case "--hidden", "--no-ignore", "--no-ignore-vcs", "--no-config", "--smart-case", "--case-sensitive", "--pcre2", "--multiline", "--multiline-dotall", "--no-messages", "--stats", "--trim", "--heading", "--no-heading", "--line-buffered", "--block-buffered", "--search-zip", "--follow":
			return 'b'
		case "--glob", "--iglob", "--type", "--type-not", "--ignore-file":
			return 'p'
		case "--encoding", "--replace", "--threads", "--max-columns", "--max-depth", "--max-filesize", "--color", "--colors", "--sort", "--sortr", "--path-separator", "--context-separator":
			return 'v'
		}
	}
	return 0
}
