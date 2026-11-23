package commandguard

import (
	"context"
	"runtime"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const powerShellEnvironmentMatch = "PowerShell environment mutation"
const powerShellEnvironmentSuggestion = "The guard could not inspect the PowerShell environment mutation. Use literal environment names, values and supported assignment forms so it can determine whether repository hooks are affected."

// powerShellEnvironmentSource identifies source that must not be copied into
// diagnostics after parsing fails. It examines only a fixed prefix or command
// head: malformed argument roles cannot establish which Set-Item provider is
// selected, and re-decoding them would duplicate incomplete analysis.
func powerShellEnvironmentSource(inv shellwords.Invocation) bool {
	if inv.Dialect != shellwords.PowerShell {
		return false
	}
	if inv.EnvironmentCall != nil {
		return true
	}
	if len(inv.Words) == 0 {
		return false
	}
	head := inv.Words[0]
	if head.Literal && (strings.EqualFold(head.Text, "Set-Item") || strings.EqualFold(head.Text, "Remove-Item")) {
		return true
	}
	for _, prefix := range []string{"$env:", "${env:"} {
		if len(head.Raw) >= len(prefix) && strings.EqualFold(head.Raw[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

// powerShellEnvironmentWrite decodes only writes in an established local dialect.
func powerShellEnvironmentWrite(ctx context.Context, inv shellwords.Invocation) (issue gitHookIssue) {
	if inv.Dialect != shellwords.PowerShell || len(inv.Words) == 0 {
		return gitHookIssue{}
	}
	defer func() {
		if ctx.Err() != nil {
			issue = powerShellEnvironmentIssue()
		}
	}()
	if ctx.Err() != nil {
		return powerShellEnvironmentIssue()
	}
	if call := inv.EnvironmentCall; call != nil {
		args := call.Arguments
		if len(args) < 2 || len(args) == 3 && !strings.EqualFold(args[2], "Process") && !strings.EqualFold(args[2], "User") && !strings.EqualFold(args[2], "Machine") {
			return powerShellEnvironmentIssue()
		}
		return powerShellEnvironmentValue(ctx, args[0], args[1])
	}
	if inv.Words[0].Literal && strings.EqualFold(inv.Words[0].Text, "Set-Item") {
		return powerShellSetItem(ctx, inv.Words[1:])
	}
	if inv.Words[0].Literal && strings.EqualFold(inv.Words[0].Text, "Remove-Item") {
		return powerShellRemoveEnvironment(ctx, inv.Words[1:])
	}
	raw := strings.TrimSpace(inv.Raw)
	lower := strings.ToLower(raw[:min(len(raw), len("${env:"))])
	name, rest := "", ""
	if strings.HasPrefix(lower, "${env:") {
		end := -1
		for i := len("${env:"); i < len(raw); i++ {
			if i%32768 == 0 && ctx.Err() != nil {
				break
			}
			if raw[i] == '}' {
				end = i
				break
			}
		}
		if end < 0 {
			return powerShellEnvironmentIssue()
		}
		name, rest = raw[len("${env:"):end], strings.TrimSpace(raw[end+1:])
	} else if strings.HasPrefix(lower, "$env:") {
		end := len("$env:")
		for end < len(raw) && !strings.ContainsRune(" \t\r\n=+-*/%", rune(raw[end])) {
			if end%32768 == 0 && ctx.Err() != nil {
				return powerShellEnvironmentIssue()
			}
			end++
		}
		name, rest = raw[len("$env:"):end], strings.TrimSpace(raw[end:])
	} else {
		return gitHookIssue{}
	}
	if rest == "" || !strings.ContainsRune("=+-*/%", rune(rest[0])) {
		return gitHookIssue{}
	}
	if !powerShellRelevantEnvironment(ctx, name) {
		return gitHookIssue{}
	}
	if alternateGitConfigName(name, runtime.GOOS == "windows") {
		return alternateGitConfigIssue()
	}
	if powerShellDynamicName(ctx, name) || !strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "==") {
		return powerShellEnvironmentIssue()
	}
	// Reuse the lexer's quote provenance for the right hand side; parsing is
	// lexical only, and the original invocation retains executable children.
	parsed := shellwords.ParseContext(ctx, "Write-Output "+strings.TrimSpace(rest[1:]), shellwords.PowerShell)
	if len(parsed.Invocations) != 1 || !parsed.Invocations[0].Inspectable || len(parsed.Invocations[0].Words) != 2 || !powerShellScalar(ctx, parsed.Invocations[0].Words[1]) {
		return powerShellEnvironmentIssue()
	}
	return powerShellEnvironmentValue(ctx, name, parsed.Invocations[0].Words[1].Text)
}

func powerShellEnvironmentValue(ctx context.Context, name, value string) gitHookIssue {
	if alternateGitConfigName(name, runtime.GOOS == "windows") {
		return alternateGitConfigIssue()
	}
	protected := powerShellKeyEnvironment(name) && isHooksPathKey(value)
	if powerShellEnvironmentNameEqual(name, "GIT_CONFIG_PARAMETERS") {
		const key = "core.hooksPath"
		for i := 0; i+len(key) <= len(value); i++ {
			if i%32768 == 0 && ctx.Err() != nil {
				return powerShellEnvironmentIssue()
			}
			if isHooksPathKey(value[i : i+len(key)]) {
				protected = true
				break
			}
		}
	}
	if ctx.Err() != nil {
		return powerShellEnvironmentIssue()
	}
	if protected {
		return bypassIssue(powerShellEnvironmentMatch)
	}
	return gitHookIssue{}
}

func powerShellRemoveEnvironment(ctx context.Context, words []shellwords.Word) gitHookIssue {
	var target *shellwords.Word
	uncertain, environment := false, false
	for i := 0; i < len(words); i++ {
		if ctx.Err() != nil {
			return powerShellEnvironmentIssue()
		}
		w := &words[i]
		if strings.HasPrefix(w.Text, "-") {
			if !w.Literal || !strings.EqualFold(w.Text, "-Path") && !strings.EqualFold(w.Text, "-LiteralPath") || i+1 >= len(words) {
				uncertain = true
				continue
			}
			i++
			w = &words[i]
		}
		if target != nil {
			uncertain = true
		}
		target = w
		environment = environment || strings.HasPrefix(strings.ToLower(w.Text), "env:")
	}
	if target == nil {
		return gitHookIssue{}
	}
	if environment && uncertain {
		return alternateGitConfigIssue()
	}
	if !strings.HasPrefix(strings.ToLower(target.Text), "env:") {
		return gitHookIssue{}
	}
	name := strings.TrimPrefix(target.Text[4:], `\`)
	if uncertain || !powerShellScalar(ctx, *target) || strings.ContainsAny(name, ",*?[]") || alternateGitConfigName(name, runtime.GOOS == "windows") {
		return alternateGitConfigIssue()
	}
	return gitHookIssue{}
}

func powerShellRelevantEnvironment(ctx context.Context, name string) bool {
	return powerShellDynamicName(ctx, name) || powerShellEnvironmentNameEqual(name, "GIT_CONFIG_PARAMETERS") || powerShellKeyEnvironment(name) || alternateGitConfigName(name, runtime.GOOS == "windows")
}

func powerShellEnvironmentNameEqual(name, want string) bool {
	return name == want || runtime.GOOS == "windows" && strings.EqualFold(name, want)
}

func powerShellKeyEnvironment(name string) bool {
	const prefix = "GIT_CONFIG_KEY_"
	return len(name) >= len(prefix) && powerShellEnvironmentNameEqual(name[:len(prefix)], prefix)
}

func powerShellDynamicName(ctx context.Context, name string) bool {
	for i := 0; i < len(name); i++ {
		if i%32768 == 0 && ctx.Err() != nil {
			return true
		}
		if name[i] == '$' || name[i] == '`' {
			return true
		}
	}
	return ctx.Err() != nil
}

// powerShellSetItem accepts a closed parameter grammar; competing containers
// cannot lend an unrelated path's provenance to a different target.
func powerShellSetItem(ctx context.Context, words []shellwords.Word) gitHookIssue {
	var target, value *shellwords.Word
	uncertain := false
	for i := 0; i < len(words); i++ {
		if ctx.Err() != nil {
			return powerShellEnvironmentIssue()
		}
		w := &words[i]
		if strings.HasPrefix(w.Text, "-") {
			var slot **shellwords.Word
			switch strings.ToLower(w.Text) {
			case "-path", "-literalpath":
				slot = &target
			case "-value":
				slot = &value
			default:
				uncertain = true
				continue
			}
			if !w.Literal || *slot != nil || i+1 >= len(words) {
				uncertain = true
			}
			if i+1 < len(words) {
				i++
				*slot = &words[i]
			}
		} else if target == nil {
			target = w
		} else if value == nil {
			value = w
		} else {
			uncertain = true
		}
	}
	unknown := powerShellEnvironmentIssue()
	if uncertain || target == nil || !powerShellScalar(ctx, *target) || strings.ContainsAny(target.Text, ",*?[]") {
		return unknown
	}
	if !strings.HasPrefix(strings.ToLower(target.Text), "env:") {
		return gitHookIssue{}
	}
	name := strings.TrimPrefix(target.Text[4:], `\`)
	if !powerShellRelevantEnvironment(ctx, name) {
		return gitHookIssue{}
	}
	if alternateGitConfigName(name, runtime.GOOS == "windows") {
		return alternateGitConfigIssue()
	}
	if value == nil || !powerShellScalar(ctx, *value) || powerShellDynamicName(ctx, name) {
		return unknown
	}
	return powerShellEnvironmentValue(ctx, name, value.Text)
}

// powerShellScalar excludes arrays and adjacent expressions that the general
// command lexer can retain in one word. Quote provenance is necessary but not
// sufficient to establish a scalar value in PowerShell expression syntax.
func powerShellScalar(ctx context.Context, word shellwords.Word) bool {
	if ctx.Err() != nil || !word.Literal || word.Substituted || word.Raw == "" {
		return false
	}
	raw := word.Raw
	quote := byte(0)
	if raw[0] == '\'' || raw[0] == '"' {
		quote = raw[0]
	}
	start := 0
	if quote != 0 {
		start = 1
	}
	for i := start; i < len(raw); i++ {
		if i%32768 < 2 && ctx.Err() != nil {
			return false
		}
		c := raw[i]
		if quote != 0 && c == quote {
			if i+1 < len(raw) && raw[i+1] == quote {
				i++
				continue
			}
			return i == len(raw)-1 && ctx.Err() == nil
		}
		if c == '`' && quote != '\'' {
			i++
			continue
		}
		if quote == 0 && strings.ContainsRune("'\",@(){};|&", rune(c)) {
			return false
		}
	}
	return quote == 0 && ctx.Err() == nil
}
