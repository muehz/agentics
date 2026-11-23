package commandguard

import (
	"path"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// An agent that could write .agentics/hooks.json could switch a workflow rule off
// and then do what the rule blocks, so the settings files are fixed-safety
// territory: only the user changes them. This is a guard against a mistake or
// a steered agent, not against one set on bypassing it. An interpreter run
// from a script (python -c, node -e, perl -pi), dd of=, a path built from a
// variable, an apply_patch rename onto the path (a *** Move to: header with no
// hunk records no write target), an apply_patch that only deletes the file (a
// *** Delete File: header) or updates it with removal-only hunks (neither
// records a write target), or MCP file-writing tools can still write the file,
// because the shell forms below are a closed list. The same list leaves a bare
// redirect, or a redirect on a group or subshell, when the line has no command
// before it (the parser hands such a redirect to the last command parsed before
// it, and a script call whose script runs nothing leaves none), git
// checkout, git restore, or git rm of the file, a hard or symbolic link written
// through later, and .NET file APIs.
const (
	settingsEditCategory   = "hook_settings_edit"
	settingsEditSuggestion = "Do not edit the Agentics hook settings files. Tell the user that agents cannot change hook settings and ask them to edit .agentics/hooks.json themselves."
)

// isSettingsPath reports whether p0 names an Agentics hook settings file: its
// last two segments are .agentics/hooks.json. Either separator counts, case is
// ignored, the trailing dots and spaces Windows drops from each component are
// ignored, and the path is cleaned so ./, // and .. segments cannot disguise it.
func isSettingsPath(p0 string) bool {
	p := cleanSettingsPath(p0)
	return p == ".agentics/hooks.json" || strings.HasSuffix(p, "/.agentics/hooks.json")
}

// cleanSettingsPath applies the normalization isSettingsPath compares against.
// Windows drops the trailing dots and spaces of every path component, not only
// of the last one, so each component is trimmed before the path is cleaned. The
// "." and ".." components keep their meaning.
func cleanSettingsPath(p0 string) string {
	p := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p0), `\`, "/"))
	components := strings.Split(p, "/")
	for i, component := range components {
		if component != "." && component != ".." {
			components[i] = strings.TrimRight(component, " .")
		}
	}
	return path.Clean(strings.Join(components, "/"))
}

// isSettingsDirectory reports whether p0 names a .agentics directory, the
// destination that makes a copy or move of a hooks.json file land on a
// settings file.
func isSettingsDirectory(p0 string) bool {
	p := cleanSettingsPath(p0)
	return p == ".agentics" || strings.HasSuffix(p, "/.agentics")
}

// namesHooksFile reports whether any source is a file called hooks.json, the
// only name that becomes a settings file when it is copied into .agentics.
func namesHooksFile(sources []string) bool {
	for _, source := range sources {
		if path.Base(cleanSettingsPath(source)) == "hooks.json" {
			return true
		}
	}
	return false
}

// writesIntoSettingsDirectory reports whether copying or moving sources into
// destination creates a settings file.
func writesIntoSettingsDirectory(destination string, sources []string) bool {
	return isSettingsDirectory(destination) && namesHooksFile(sources)
}

// settingsEditTarget returns the settings file a file-writing tool call names.
// Their content is prose or code, so the check reads the path and never the
// content. It reads the call's argument layers, never its proof layers, and
// does not read the write-parse-error flag: a call the adapter could not read
// is judged on the targets that can be seen.
func settingsEditTarget(fields normalizedFields) (string, bool) {
	if !toolcall.IsFileWrite(fields.toolName) {
		return "", false
	}
	for _, target := range fields.call.Writes().Targets {
		if isSettingsPath(target) {
			return target, true
		}
	}
	return "", false
}

// How the operands of a shell command that writes a file name its targets.
const (
	operandsNone = iota
	operandsAny  // every operand is changed: tee, rm, mv, Remove-Item, and the like
	operandsLast // only the last operand is written: cp, install, ln
	operandsPath // a PowerShell cmdlet: its path parameters and first positional word, never its content
	operandsSed  // every operand, but only with -i or --in-place
)

// settingsWriteVerbs is the closed list of commands checkSettingsFileWrites
// understands, keyed by lower-case executable name. PowerShell aliases are
// listed beside the cmdlets they stand for.
var settingsWriteVerbs = map[string]int{
	"tee": operandsAny, "rm": operandsAny, "del": operandsAny, "erase": operandsAny,
	"unlink": operandsAny, "truncate": operandsAny, "shred": operandsAny, "mv": operandsAny,
	"move": operandsAny, "move-item": operandsAny, "mi": operandsAny,
	"remove-item": operandsAny, "ri": operandsAny,
	"set-content": operandsPath, "sc": operandsPath, "add-content": operandsPath, "ac": operandsPath,
	"clear-content": operandsPath, "clc": operandsPath, "out-file": operandsPath,
	"rename-item": operandsAny, "rni": operandsAny, "ren": operandsAny,
	"new-item": operandsPath, "ni": operandsPath,
	"cp": operandsLast, "copy": operandsLast, "copy-item": operandsLast, "cpi": operandsLast,
	"install": operandsLast, "ln": operandsLast,
	"sed": operandsSed,
}

// settingsDirectoryVerbs copy, link, or move their sources into a destination
// directory, so a hooks.json source aimed at .agentics writes a settings file.
var settingsDirectoryVerbs = map[string]bool{
	"cp": true, "copy": true, "copy-item": true, "cpi": true, "install": true, "ln": true,
	"mv": true, "move": true, "move-item": true, "mi": true,
}

// booleanCopyFlags are the value-free short options of cp, mv, install, and ln
// on GNU and BSD systems, the only letters that may precede -t in a cluster such
// as -vt or -Rt. The value-taking letters (S, t, g, m, o) are deliberately absent.
const booleanCopyFlags = "abcCdDfFHiLlnNPpRrsTuvXxZ"

// powershellCopyVerbs take -Path and -Destination in PowerShell, where cp and
// copy are aliases of Copy-Item.
var powershellCopyVerbs = map[string]bool{"copy-item": true, "cpi": true, "copy": true, "cp": true}

// checkSettingsFileWrites reports the first command that writes a settings file
// through a redirect or one of the commands in settingsWriteVerbs. A word that
// is not a literal cannot be read, so it is not judged. The redirects of an
// expanded script call are judged on the call as well as on its commands,
// because a script that runs no command leaves no invocation to carry them
// while the shell still opens, and may truncate, the target.
func checkSettingsFileWrites(invocations []shellwords.Invocation, scriptCalls []shellwords.ScriptCall, report reportFunc) {
	for _, inv := range invocations {
		if !inv.Inspectable {
			continue
		}
		if target, ok := settingsShellTarget(inv); ok {
			report(ruleSettingsFileEdit, target, settingsEditSuggestion)
			return
		}
	}
	for _, call := range scriptCalls {
		if target, ok := settingsRedirectTarget(call.OutputRedirects); ok {
			report(ruleSettingsFileEdit, target, settingsEditSuggestion)
			return
		}
	}
}

// settingsRedirectTarget returns the settings file a write redirect opens.
func settingsRedirectTarget(redirects []shellwords.Redirection) (string, bool) {
	for _, redirect := range redirects {
		if strings.Contains(redirect.Operator, ">") && redirect.Target.Literal && isSettingsPath(redirect.Target.Text) {
			return redirect.Target.Text, true
		}
	}
	return "", false
}

func settingsShellTarget(inv shellwords.Invocation) (string, bool) {
	// OutputRedirects carries the command's own write redirects as well as those of the wrapper, script call, or group
	// it runs under.
	if target, ok := settingsRedirectTarget(inv.OutputRedirects); ok {
		return target, true
	}
	if len(inv.Words) == 0 {
		return "", false
	}
	name := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
	verb := settingsWriteVerbs[name]
	if verb == operandsNone {
		return "", false
	}
	powershell := inv.Dialect == shellwords.PowerShell
	// A PowerShell dry run changes nothing.
	if powershell && hasWhatIf(inv.Words[1:]) {
		return "", false
	}
	if powershell && powershellCopyVerbs[name] {
		return powershellCopyTarget(inv.Words[1:])
	}
	if verb == operandsPath {
		// These words are PowerShell cmdlets and aliases. Elsewhere the same
		// name is another program (sc is the Windows service controller), whose
		// first operand is not a file it writes.
		if !powershell {
			return "", false
		}
		for _, operand := range powershellPathOperands(inv.Words[1:]) {
			if isSettingsPath(operand) {
				return operand, true
			}
		}
		return "", false
	}
	inPlace := false
	var operands []string
	directoryVerb := settingsDirectoryVerbs[name]
	targetDirectory, hasTargetDirectory := "", false
	words := inv.Words[1:]
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !word.Literal {
			continue
		}
		text := word.Text
		if strings.HasPrefix(text, "-") && text != "-" {
			if verb == operandsSed && (text == "--in-place" || strings.HasPrefix(text, "--in-place=") ||
				(!strings.HasPrefix(text, "--") && strings.Contains(text, "i"))) {
				inPlace = true
			}
			if directoryVerb && !powershell {
				// -t DIR, -tDIR, --target-directory DIR, and --target-directory=DIR
				// name the destination directory and leave every operand a source.
				if value, ok := strings.CutPrefix(text, "--target-directory="); ok {
					targetDirectory, hasTargetDirectory = value, true
				} else if text == "--target-directory" && i+1 < len(words) && words[i+1].Literal {
					i++
					targetDirectory, hasTargetDirectory = words[i].Text, true
				} else if !strings.HasPrefix(text, "--") {
					// Only a cluster of boolean flags before the t makes it -t; any
					// other letter before it may take a value (-S.txt, -gstaff) that
					// merely contains a t.
					if flags, rest, found := strings.Cut(text[1:], "t"); found && strings.Trim(flags, booleanCopyFlags) == "" {
						if rest != "" {
							targetDirectory, hasTargetDirectory = rest, true
						} else if i+1 < len(words) && words[i+1].Literal {
							i++
							targetDirectory, hasTargetDirectory = words[i].Text, true
						}
					}
				}
			}
			// PowerShell binds a value to its parameter with a colon.
			if _, value, found := strings.Cut(text, ":"); found && powershell {
				operands = append(operands, value)
			}
			continue
		}
		operands = append(operands, text)
	}
	if directoryVerb {
		if hasTargetDirectory {
			if writesIntoSettingsDirectory(targetDirectory, operands) {
				return targetDirectory, true
			}
		} else if n := len(operands); n > 1 && writesIntoSettingsDirectory(operands[n-1], operands[:n-1]) {
			return operands[n-1], true
		}
	}
	switch verb {
	case operandsSed:
		if !inPlace {
			return "", false
		}
		fallthrough
	case operandsAny:
		for _, operand := range operands {
			if isSettingsPath(operand) {
				return operand, true
			}
		}
	case operandsLast:
		// With a target directory every operand is a source, so the file is
		// only read.
		if n := len(operands); n > 0 && !hasTargetDirectory && isSettingsPath(operands[n-1]) {
			return operands[n-1], true
		}
	}
	return "", false
}

// powershellPathParameters bind a file path, and powershellSwitches take no
// value. Any other parameter is assumed to consume the word after it.
var (
	powershellPathParameters = map[string]bool{"path": true, "literalpath": true, "filepath": true}
	powershellSwitches       = map[string]bool{
		"force": true, "nonewline": true, "whatif": true, "confirm": true,
		"passthru": true, "append": true, "noclobber": true, "recurse": true, "container": true,
	}
)

// hasWhatIf reports whether a PowerShell command carries the -WhatIf switch.
func hasWhatIf(words []shellwords.Word) bool {
	for _, word := range words {
		// A quoted '-WhatIf' is a value, such as the argument of -Exclude.
		if word.Literal && !word.Quoted && strings.EqualFold(word.Text, "-whatif") {
			return true
		}
	}
	return false
}

// powershellCopyTarget judges a Copy-Item style command. The destination is the
// value of -Destination (or an unambiguous abbreviation of it) when given. Else
// it is the first positional word when -Path or -LiteralPath is named, else the
// second positional word. With a single positional word and no -Destination
// there is none, because PowerShell copies to the current location. Everything
// else is a source.
func powershellCopyTarget(words []shellwords.Word) (string, bool) {
	var positional, sources []string
	destination, hasDestination, namedPath := "", false, false
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !word.Literal {
			continue
		}
		text := word.Text
		if !strings.HasPrefix(text, "-") || text == "-" {
			positional = append(positional, text)
			continue
		}
		name, value, bound := strings.Cut(strings.TrimLeft(text, "-"), ":")
		name = strings.ToLower(name)
		if !bound && !powershellSwitches[name] && i+1 < len(words) {
			i++
			value, bound = words[i].Text, words[i].Literal
		}
		if !bound {
			continue
		}
		switch {
		case len(name) >= 3 && strings.HasPrefix("destination", name):
			destination, hasDestination = value, true
		case name == "path" || name == "literalpath":
			sources = append(sources, value)
			namedPath = true
		}
	}
	switch {
	case hasDestination:
		sources = append(sources, positional...)
	case namedPath && len(positional) > 0:
		destination, hasDestination = positional[0], true
		sources = append(sources, positional[1:]...)
	case len(positional) > 1:
		destination, hasDestination = positional[1], true
		sources = append(sources, positional[0])
	}
	if !hasDestination {
		return "", false
	}
	if isSettingsPath(destination) || writesIntoSettingsDirectory(destination, sources) {
		return destination, true
	}
	return "", false
}

// powershellPathOperands returns the words of a PowerShell write cmdlet that
// name the file it writes: the value of -Path, -LiteralPath, or -FilePath, and
// the first positional word. The content word, as in Set-Content notes.txt
// ".agentics/hooks.json", is never a target.
func powershellPathOperands(words []shellwords.Word) []string {
	var paths []string
	positional := false
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !word.Literal {
			continue
		}
		text := word.Text
		if !strings.HasPrefix(text, "-") || text == "-" {
			if !positional {
				positional = true
				paths = append(paths, text)
			}
			continue
		}
		name, value, bound := strings.Cut(strings.TrimLeft(text, "-"), ":")
		name = strings.ToLower(name)
		switch {
		case bound:
			if powershellPathParameters[name] {
				paths = append(paths, value)
			}
		case powershellSwitches[name]:
		default:
			if i+1 < len(words) {
				if powershellPathParameters[name] && words[i+1].Literal {
					paths = append(paths, words[i+1].Text)
				}
				i++
			}
		}
	}
	return paths
}
