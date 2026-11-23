package commandguard

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// hookRemoveVerbs delete, shred or empty their operands. The list is closed, as settingsWriteVerbs is, and covers the
// PowerShell spellings of the same commands.
var hookRemoveVerbs = map[string]bool{
	"rm": true, "del": true, "erase": true, "unlink": true, "shred": true, "truncate": true,
	"remove-item": true, "ri": true, "clear-content": true, "clc": true,
}

// hookMoveVerbs move their source away. A protected destination is a modification, so only a source counts.
var hookMoveVerbs = map[string]bool{"mv": true, "move": true, "move-item": true, "mi": true}

// hookNullVerbs run nothing and write nothing themselves, so only their redirect can truncate a protected file.
var hookNullVerbs = map[string]bool{":": true}

// checkHookFileEdits reports the first command that deletes, moves away or truncates a protected Git hook path.
// It reads the parsed invocations, which include the commands of every inspected script, so a script that removes a
// hook is judged like the same line typed. A redirect that writes a command's output is a modification and is not
// judged, and a script call redirected onto a hook passes for the same reason.
func checkHookFileEdits(invocations []shellwords.Invocation, report reportFunc) {
	for _, inv := range invocations {
		if !inv.Inspectable {
			continue
		}
		if target, ok := hookShellTarget(inv); ok {
			report(ruleGitHookFileEdit, target, hookEditSuggestion)
			return
		}
	}
}

func hookShellTarget(inv shellwords.Invocation) (string, bool) {
	if len(inv.Words) == 0 || !inv.Words[0].Literal {
		return "", false
	}
	name := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
	words := inv.Words[1:]
	powershell := inv.Dialect == shellwords.PowerShell
	switch {
	case hookNullVerbs[name]:
		return hookTruncatingRedirect(inv)
	case name == "git":
		return hookGitRemoveTarget(inv)
	case hookRemoveVerbs[name], hookMoveVerbs[name]:
		var operands []string
		switch {
		case powershell && hookMoveVerbs[name]:
			operands = hookPowerShellMoveSources(words)
		case powershell:
			operands = hookPowerShellOperands(words)
		case hookMoveVerbs[name]:
			operands = hookMoveSources(words)
		default:
			operands = hookOperands(words)
		}
		for _, operand := range operands {
			if isHookPath(operand) {
				return operand, true
			}
		}
	}
	return "", false
}

// hookPowerShellOperands is powershellPathOperands after dropping the abbreviated -Recurse switch (-r, -re, ...). The
// shared reader knows only the full spelling, so it would take -r for a parameter that consumes the next word and
// lose the path that follows it.
func hookPowerShellOperands(words []shellwords.Word) []string {
	kept := make([]shellwords.Word, 0, len(words))
	for _, word := range words {
		if word.Literal && !word.Quoted && len(word.Text) >= 2 && word.Text[0] == '-' &&
			strings.HasPrefix("-recurse", strings.ToLower(word.Text)) {
			continue
		}
		kept = append(kept, word)
	}
	return powershellPathOperands(kept)
}

// hookPowerShellMoveSources reads only the source of Move-Item and its aliases. A named source leaves the first
// positional word for the destination, even when that word precedes the named parameter. An unknown source keeps its
// positional slot so a literal destination cannot be mistaken for it.
func hookPowerShellMoveSources(words []shellwords.Word) []string {
	var sources, positional []string
	namedPath := false
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !word.Literal {
			positional = append(positional, "")
			continue
		}
		text := word.Text
		if word.Quoted || !strings.HasPrefix(text, "-") || text == "-" {
			positional = append(positional, text)
			continue
		}
		name, value, bound := strings.Cut(strings.TrimLeft(text, "-"), ":")
		name = strings.ToLower(name)
		isSource := name == "path" || name == "literalpath"
		if isSource {
			namedPath = true
		}
		if !bound && !powershellSwitches[name] && i+1 < len(words) {
			i++
			value, bound = words[i].Text, words[i].Literal
		}
		if isSource && bound {
			sources = append(sources, value)
		}
	}
	if namedPath {
		return sources
	}
	if len(positional) > 0 {
		return positional[:1]
	}
	return nil
}

// hookOperands returns the literal operands of a POSIX command: every word that is not an option, and every word
// after "--".
func hookOperands(words []shellwords.Word) []string {
	var operands []string
	endOptions := false
	for _, word := range words {
		if !word.Literal {
			continue
		}
		text := word.Text
		if !endOptions && text == "--" {
			endOptions = true
			continue
		}
		if !endOptions && strings.HasPrefix(text, "-") && text != "-" {
			continue
		}
		operands = append(operands, text)
	}
	return operands
}

// hookMoveSources returns the operands of mv that are moved away. With a target directory every operand is a source;
// otherwise the last operand is the destination. A word that is not a literal keeps its place as "" so the last
// operand is still the last.
func hookMoveSources(words []shellwords.Word) []string {
	var operands []string
	targetDirectory, endOptions := false, false
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !word.Literal {
			operands = append(operands, "")
			continue
		}
		text := word.Text
		if !endOptions && text == "--" {
			endOptions = true
			continue
		}
		if !endOptions && strings.HasPrefix(text, "-") && text != "-" {
			switch {
			case text == "--target-directory":
				targetDirectory = true
				i++
			case strings.HasPrefix(text, "--target-directory="):
				targetDirectory = true
			case !strings.HasPrefix(text, "--"):
				if flags, rest, found := strings.Cut(text[1:], "t"); found && strings.Trim(flags, booleanCopyFlags) == "" {
					targetDirectory = true
					if rest == "" {
						i++
					}
				}
			}
			continue
		}
		operands = append(operands, text)
	}
	if targetDirectory {
		return operands
	}
	if len(operands) < 2 {
		return nil
	}
	return operands[:len(operands)-1]
}

// hookGitRemoveTarget returns the protected path a git rm removes from the working tree and the index. A dry run
// changes nothing; --cached does not pass, because it drops the tracked hook for everyone at the next commit.
func hookGitRemoveTarget(inv shellwords.Invocation) (string, bool) {
	g, isGit := readGitInvocation(inv)
	if !isGit || !inv.Inspectable || g.Subcommand() != "rm" {
		return "", false
	}
	var operands []shellwords.Word
	endOptions := false
	for _, word := range g.Args {
		text := word.Text
		if !endOptions && text == "--" {
			endOptions = true
			continue
		}
		if !endOptions && strings.HasPrefix(text, "-") && text != "-" {
			if text == "--dry-run" || (!strings.HasPrefix(text, "--") && strings.Contains(text[1:], "n")) {
				return "", false
			}
			continue
		}
		operands = append(operands, word)
	}
	for _, operand := range operands {
		if operand.Literal && isHookPath(operand.Text) {
			return operand.Text, true
		}
	}
	return "", false
}

// hookTruncatingRedirect returns the protected file that the : command truncates. Only a truncating redirect counts;
// an append leaves the content in place.
func hookTruncatingRedirect(inv shellwords.Invocation) (string, bool) {
	return hookTruncatingTarget(inv.OutputRedirects)
}

// hookTruncatingTarget returns the first protected Git hook file that a truncating redirect opens.
func hookTruncatingTarget(redirects []shellwords.Redirection) (string, bool) {
	for _, redirect := range redirects {
		if strings.Contains(redirect.Operator, ">") && !strings.Contains(redirect.Operator, ">>") &&
			redirect.Target.Literal && isHookFilePath(redirect.Target.Text) {
			return redirect.Target.Text, true
		}
	}
	return "", false
}
