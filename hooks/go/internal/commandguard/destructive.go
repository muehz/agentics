package commandguard

import (
	"strconv"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const (
	destructiveFileSuggestion = "Remove only specific, verified paths. Broad recursive deletion is blocked; target one known child path instead."
	findDeleteSuggestion      = "Use find against one specific child path, inspect the matches, and delete only that bounded result."
)

func protectedDeletionComponent(component string) bool {
	component = strings.ToLower(component)
	return component == ".git" || component == ".env" || strings.HasPrefix(component, ".env.")
}

// checkProtectedDeletion returns true only when the legacy protected-path
// rules have no remaining executable context to inspect.
func checkProtectedDeletion(invocations []shellwords.Invocation, report reportFunc) bool {
	complete := true
	for _, inv := range invocations {
		// Assignments may configure executable consumers. This classifier
		// establishes operand roles only, not the meaning of environment data.
		if len(inv.Assignments) != 0 {
			complete = false
		}
		if !inv.Inspectable || !inv.Complete || len(inv.Words) == 0 {
			complete = false
			continue
		}
		exe := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
		if inv.Dialect == shellwords.PowerShell && (exe == "remove-item" || exe == "rm" || exe == "ri" || exe == "del") {
			removal := parsePowerShellRemoval(inv.Words)
			for _, target := range removal.targets {
				if target.environment {
					// Literal Env: removal already has an environment-policy
					// consumer; it cannot prove a filesystem deletion context.
					complete = false
					continue
				}
				if !target.word.Literal {
					continue
				}
				parts := strings.Split(normalizeDotSegments(target.word.Text), "/")
				for i, part := range parts {
					if !protectedDeletionComponent(part) || (i != len(parts)-1 && !strings.EqualFold(part, ".git")) {
						continue
					}
					suggestion := "Move the .env file to a backup location instead of removing it."
					if strings.EqualFold(part, ".git") {
						suggestion = "Do not delete the .git directory. Use Git commands to manage repository state."
					}
					report(ruleProtectedDeletion, exe+" "+target.word.Raw, suggestion)
					break
				}
			}
			_, broad := broadRemoveItem(inv.Words)
			if !removal.understood && !broad {
				report(ruleUninspectableDeletion, inv.Raw, "Use literal deletion operands and supported options so the guard can inspect every target.")
			}
			redirects, ok := inv.OwnRedirects()
			complete = complete && removal.understood && ok && len(redirects) == 0 && len(inv.Wrappers) == 0 && inv.Parent < 0 && !inv.PipelineInput && !inv.PipelineOutput
			continue
		}
		if exe != "rm" && exe != "unlink" && exe != "del" {
			complete = protectedDeletionInertContext(inv) && complete
			continue
		}
		if inv.Dialect != shellwords.POSIX {
			complete = false
			continue
		}
		options, understood, targets := true, true, 0
		for _, word := range inv.Words[1:] {
			if !word.Literal || hasActiveDataExpansion(word.Raw) {
				understood = false
			}
			if options && word.Text == "--" {
				options = false
				continue
			}
			if options && strings.HasPrefix(word.Text, "-") && word.Text != "-" {
				known := false
				if exe == "rm" {
					switch word.Text {
					case "--force", "--recursive", "--interactive", "--verbose":
						known = true
					default:
						known = !strings.HasPrefix(word.Text, "--") && strings.Trim(word.Text[1:], "frRiv") == ""
					}
				}
				understood = understood && known
				continue
			}
			if exe == "del" && strings.HasPrefix(word.Text, "/") {
				understood = false
			}
			targets++
			if !word.Literal {
				continue
			}
			parts := strings.Split(normalizeDotSegments(word.Text), "/")
			for i, part := range parts {
				if !protectedDeletionComponent(part) || (i != len(parts)-1 && !strings.EqualFold(part, ".git")) {
					continue
				}
				suggestion := "Move the .env file to a backup location instead of removing it."
				if strings.EqualFold(part, ".git") {
					suggestion = "Do not delete the .git directory. Use Git commands to manage repository state."
				}
				report(ruleProtectedDeletion, exe+" "+word.Raw, suggestion)
				break
			}
		}
		if targets == 0 || (exe == "unlink" && targets != 1) {
			understood = false
		}
		if !understood {
			complete = false
			// CMD-style del options remain under the original raw evidence
			// rules; POSIX parsing cannot establish their operand roles.
			_, broad, active := classifyRM(inv.Words)
			if exe != "del" && !(exe == "rm" && active && broad) {
				report(ruleUninspectableDeletion, inv.Raw, "Use literal deletion operands and supported options so the guard can inspect every target.")
			}
		}
	}
	return complete
}

type powerShellRemovalTarget struct {
	word        shellwords.Word
	literalPath bool
	environment bool
}

type powerShellRemoval struct {
	targets    []powerShellRemovalTarget
	recursive  bool
	understood bool
}

func parsePowerShellRemoval(words []shellwords.Word) powerShellRemoval {
	result := powerShellRemoval{understood: true}
	for i := 1; i < len(words); i++ {
		word := words[i]
		literalPath := false
		option := ""
		if !word.Quoted && word.Literal && word.Raw == word.Text {
			option = strings.ToLower(word.Text)
		}
		switch option {
		case "-recurse", "-r":
			result.recursive = true
			continue
		case "-force":
			continue
		case "-path", "-literalpath":
			literalPath = strings.EqualFold(word.Text, "-literalpath")
			if i+1 == len(words) {
				result.understood = false
				continue
			}
			i++
			word = words[i]
		case "-filter", "-include", "-exclude", "-credential", "-stream":
			if i+1 == len(words) {
				result.understood = false
				continue
			}
			i++
			if !powerShellRemovalLiteral(words[i]) {
				result.understood = false
			}
			continue
		default:
			if !word.Quoted && strings.HasPrefix(word.Text, "-") {
				result.understood = false
				continue
			}
		}
		environment := false
		if strings.EqualFold(words[0].Text, "Remove-Item") && len(word.Text) > len("Env:") && strings.EqualFold(word.Text[:len("Env:")], "Env:") {
			name := strings.TrimPrefix(word.Text[len("Env:"):], `\`)
			environment = name != "" && !strings.ContainsAny(name, `:/\`)
		}
		if !powerShellRemovalLiteral(word) || (!environment && !powerShellRemovalPathKnown(word.Text)) || (!literalPath && strings.ContainsAny(word.Text, "*?[]")) {
			result.understood = false
		}
		result.targets = append(result.targets, powerShellRemovalTarget{word: word, literalPath: literalPath, environment: environment})
	}
	if len(result.targets) == 0 {
		result.understood = false
	}
	return result
}

func powerShellRemovalPathKnown(path string) bool {
	path = strings.ReplaceAll(path, `\`, "/")
	// Provider namespaces, PSDrives, UNC/device paths and drive-relative paths
	// require resolution beyond ordinary lexical filesystem components. Keep
	// their evidence instead of treating a familiar suffix as a complete path.
	if strings.HasPrefix(path, "//") {
		return false
	}
	if colon := strings.IndexByte(path, ':'); colon >= 0 {
		return colon == 1 && len(path) >= 3 && path[2] == '/' &&
			(path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z') &&
			!strings.ContainsRune(path[2:], ':')
	}
	return true
}

func powerShellRemovalLiteral(word shellwords.Word) bool {
	if !word.Literal {
		return false
	}
	if !strings.ContainsAny(word.Raw, "@,") {
		return true
	}
	// Arrays and splats have argument-mode meaning outside a complete quoted
	// scalar. Mixed quoting remains unproven rather than flattening that role.
	return len(word.Raw) >= 2 && (word.Raw[0] == '\'' || word.Raw[0] == '"') &&
		word.Raw[len(word.Raw)-1] == word.Raw[0] && !strings.ContainsRune(word.Raw[1:len(word.Raw)-1], rune(word.Raw[0]))
}

// protectedDeletionInertContext proves a closed consumer contract. Arbitrary
// executable arguments remain evidence even when deletion names are embedded
// inside another language's punctuation or an option value.
func protectedDeletionInertContext(inv shellwords.Invocation) bool {
	if inv.Dialect != shellwords.POSIX || len(inv.Words) == 0 || len(inv.Wrappers) != 0 || len(inv.Assignments) != 0 ||
		inv.Parent >= 0 || inv.Before == shellwords.Group || inv.Before == shellwords.Substitution || inv.Before == shellwords.Background {
		return false
	}
	for _, word := range inv.Words {
		if !word.Literal || hasActiveDataExpansion(word.Raw) {
			return false
		}
	}
	redirects, ok := inv.OwnRedirects()
	if !ok || len(redirects) != 0 {
		return false
	}
	words := inv.Words
	if words[0].Text == "docker" {
		// Only this bounded run form has known option roles and an explicit
		// inert child. An image's default command or entrypoint overrides
		// cannot borrow the --rm option's harmlessness.
		if len(words) < 4 || words[1].Text != "run" {
			return false
		}
		i := 2
		for i < len(words) && strings.HasPrefix(words[i].Text, "-") {
			switch words[i].Text {
			case "--rm":
				i++
			case "--mount":
				i += 2
			default:
				return false
			}
		}
		if i+1 >= len(words) {
			return false
		}
		words = words[i+1:]
		inv.Words = words
	}
	if words[0].Text == "ls" {
		flags := []string{"--all", "--almost-all", "--directory", "--human-readable", "--recursive"}
		for _, word := range words[1:] {
			if word.Text == "--" {
				break
			}
			// Each accepted short flag is boolean; unknown or value-taking
			// letters cannot borrow another member's understood role.
			if len(word.Text) > 1 && word.Text[0] == '-' && strings.Trim(word.Text[1:], "aAldhR") == "" {
				flags = append(flags, word.Text)
			}
		}
		ok, _ := decodeDataScan(words, 1, argv.NewSpec(dataOptions(flags, nil)...), nil, false, false)
		return ok
	}
	return fullInertInvocationProof(inv.Raw, shellwords.Result{Invocations: []shellwords.Invocation{inv}}, false)
}

// checkDestructiveCommands applies C01 and C08 to literal shell invocations.
// The shared parser has already removed supported wrappers and kept quote and
// expansion provenance on every word.
func checkDestructiveCommands(invocations []shellwords.Invocation, report reportFunc) {
	for _, inv := range invocations {
		if !inv.Inspectable || len(inv.Words) == 0 {
			continue
		}
		exe := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
		if inv.Dialect == shellwords.PowerShell && (exe == "remove-item" || exe == "rm" || exe == "ri" || exe == "del") {
			if match, ok := broadRemoveItem(inv.Words); ok {
				report(ruleBroadRemoveItem, match, destructiveFileSuggestion)
			}
			continue
		}
		switch exe {
		case "rm":
			if match, broad, active := classifyRM(inv.Words); active {
				if broad {
					report(ruleBroadRm, match, rmBroadSuggestion(match))
				} else {
					report(ruleTargetedRmAdvice, match,
						"In a git repo, prefer 'git clean -f <path>' for non-ignored untracked files; targeted rm remains appropriate for ignored build artifacts.")
				}
			}
		case "remove-item":
			if match, ok := broadRemoveItem(inv.Words); ok {
				report(ruleBroadRemoveItem, match, destructiveFileSuggestion)
			}
		case "find":
			if match, broad, active := classifyFindDelete(inv.Words); active {
				if broad {
					report(ruleBroadFindDelete, match, findDeleteSuggestion)
				} else {
					report(ruleFindDeleteAdvice, match,
						"Use targeted removal on specific known paths instead of 'find -delete'.")
				}
			}
		case "chmod":
			if match, ok := worldWritableChmod(inv.Words); ok {
				report(ruleWorldWritableChmod, match, chmodSuggestion(inv.Words))
			}
		}
	}
}

func rmBroadSuggestion(match string) string {
	target := strings.TrimSpace(strings.TrimPrefix(match, "rm"))
	target = strings.Trim(target, `"'`)
	target = normalizeDotSegments(target)
	switch {
	case strings.HasPrefix(target, "/"):
		return "Remove only specific, verified paths instead of the filesystem root. If the user still wants to delete the root, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal."
	case target == "." || strings.HasPrefix(target, "./"):
		return "Remove only specific, verified paths instead of the current directory. If the user still wants to delete the entire current directory, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal."
	case target == ".." || strings.HasPrefix(target, "../"):
		return "Remove only specific, verified paths instead of a parent directory. If the user still wants to delete the parent directory recursively, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal."
	default:
		return "Remove only specific, verified paths instead of the home directory. If the user still wants to delete the entire home directory, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal."
	}
}

func chmodSuggestion(words []shellwords.Word) string {
	for _, word := range words[1:] {
		if word.Text == "-R" || len(word.Text) > 1 && word.Text[0] == '-' && strings.ContainsRune(word.Text[1:], 'R') {
			return "Use specific permissions such as 'chmod -R 755' and limit the operation to verified paths."
		}
	}
	return "Use 'chmod 755' for directories or 'chmod 644' for files instead."
}

func classifyRM(words []shellwords.Word) (match string, broad, active bool) {
	recursive, force, options := false, false, true
	var targets []shellwords.Word
	for _, word := range words[1:] {
		text := word.Text
		if options && text == "--" {
			options = false
			continue
		}
		if options {
			switch text {
			case "--recursive":
				recursive = true
				continue
			case "--force":
				force = true
				continue
			}
			if len(text) > 1 && text[0] == '-' && text[1] != '-' {
				for _, flag := range text[1:] {
					switch flag {
					case 'r', 'R':
						recursive = true
					case 'f':
						force = true
					}
				}
				continue
			}
		}
		targets = append(targets, word)
	}
	if !recursive || !force {
		return "", false, false
	}
	for _, target := range targets {
		if isBroadTarget(target) {
			return "rm " + target.Raw, true, true
		}
	}
	if len(targets) > 0 {
		return "rm " + targets[0].Raw, false, true
	}
	return "", false, false
}

func broadRemoveItem(words []shellwords.Word) (string, bool) {
	removal := parsePowerShellRemoval(words)
	if !removal.recursive {
		return "", false
	}
	for _, target := range removal.targets {
		if target.literalPath && strings.ContainsAny(target.word.Text, "*?[]") {
			continue
		}
		if isBroadTarget(target.word) {
			return "Remove-Item " + target.word.Raw, true
		}
	}
	return "", false
}

func classifyFindDelete(words []shellwords.Word) (match string, broad, active bool) {
	deleteSeen := false
	for _, word := range words[1:] {
		if word.Text == "-delete" {
			deleteSeen = true
			break
		}
	}
	if !deleteSeen {
		return "", false, false
	}
	var roots []shellwords.Word
	options := true
	for i := 1; i < len(words); i++ {
		text := words[i].Text
		if options && len(roots) == 0 {
			switch text {
			case "-H", "-L", "-P":
				continue
			case "--":
				options = false
				continue
			case "-D", "-O":
				if i+1 < len(words) {
					i++
					continue
				}
				return "find -delete", true, true
			}
			if strings.HasPrefix(text, "-D") || strings.HasPrefix(text, "-O") {
				continue
			}
			if strings.HasPrefix(text, "-") {
				break
			}
		}
		if text == "!" || text == "(" || text == ")" {
			break
		}
		if strings.HasPrefix(text, "-") {
			break
		}
		roots = append(roots, words[i])
	}
	if len(roots) == 0 {
		return "find -delete", true, true
	}
	for _, root := range roots {
		if isBroadTarget(root) {
			return "find " + root.Raw + " -delete", true, true
		}
	}
	return "find " + roots[0].Raw + " -delete", false, true
}

func worldWritableChmod(words []shellwords.Word) (string, bool) {
	options := true
	for i := 1; i < len(words); i++ {
		text := words[i].Text
		if options && text == "--" {
			options = false
			continue
		}
		if options && text == "--reference" {
			i++
			continue
		}
		if options && strings.HasPrefix(text, "--reference=") {
			continue
		}
		if options && strings.HasPrefix(text, "-") {
			continue
		}
		if numericWorldWrite(text) || symbolicWorldWrite(text) {
			return "chmod " + words[i].Raw, true
		}
		return "", false
	}
	return "", false
}

func numericWorldWrite(mode string) bool {
	mode = strings.TrimLeft(mode, "0")
	if mode == "" {
		mode = "0"
	}
	if len(mode) > 4 {
		return false
	}
	value, err := strconv.ParseUint(mode, 8, 16)
	return err == nil && value&0002 != 0
}

func symbolicWorldWrite(mode string) bool {
	for _, clause := range strings.Split(mode, ",") {
		operator := strings.IndexAny(clause, "+-=")
		if operator <= 0 || operator+1 >= len(clause) {
			continue
		}
		classes, op, permissions := clause[:operator], clause[operator], clause[operator+1:]
		if (op == '+' || op == '=') && strings.ContainsAny(classes, "ao") && strings.ContainsRune(permissions, 'w') {
			return true
		}
	}
	return false
}

func isBroadTarget(word shellwords.Word) bool {
	text := normalizeDotSegments(strings.TrimSpace(strings.ReplaceAll(word.Text, `\`, "/")))
	lower := strings.ToLower(text)
	if text == "" {
		return false
	}
	if !word.Literal && !isLiteralHomeExpression(text) {
		return false
	}
	if lower == "/" || strings.HasPrefix(lower, "/") && onlyBroadSuffix(strings.TrimPrefix(lower, "/")) {
		return true
	}
	if strings.HasPrefix(lower, "/") {
		suffix := strings.TrimPrefix(lower, "/")
		if len(suffix) > 0 && suffix[0] < ' ' {
			return true
		}
	}
	if len(lower) >= 3 && lower[1] == ':' && lower[2] == '/' && onlyBroadSuffix(lower[3:]) {
		return true
	}
	bases := []string{"${userprofile}", "$userprofile", "%userprofile%", "${home}", "$home", "$env:userprofile", "${env:userprofile}", "$env:home", "${env:home}", "~", "..", "."}
	for _, base := range bases {
		if lower == base || lower == base+"/" {
			return true
		}
		if strings.HasPrefix(lower, base+"/") && onlyBroadSuffix(strings.TrimPrefix(lower, base+"/")) {
			return true
		}
	}
	return false
}

// normalizeDotSegments performs bounded lexical normalization for literal
// paths. It never consults the filesystem, follows symlinks, or expands a
// variable; it only removes . and collapses .. where a preceding segment is
// available. Absolute roots clamp attempts to walk above the root.
func normalizeDotSegments(text string) string {
	text = strings.ReplaceAll(text, `\`, "/")
	if text == "" {
		return text
	}
	if text == "." || text == "./" {
		return "."
	}
	prefix, rest := "", text
	if strings.HasPrefix(rest, "/") {
		prefix, rest = "/", strings.TrimPrefix(rest, "/")
	} else if len(rest) >= 3 && rest[1] == ':' && rest[2] == '/' {
		prefix, rest = rest[:3], rest[3:]
	} else {
		for _, base := range []string{"${HOME}", "$HOME", "%USERPROFILE%", "${USERPROFILE}", "$USERPROFILE", "$env:HOME", "${env:HOME}", "$env:USERPROFILE", "${env:USERPROFILE}"} {
			if strings.EqualFold(rest, base) {
				return rest
			}
			if len(rest) > len(base) && strings.EqualFold(rest[:len(base)], base) && rest[len(base)] == '/' {
				prefix, rest = rest[:len(base)+1], rest[len(base)+1:]
				break
			}
		}
	}
	parts := strings.Split(rest, "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) > 0 && stack[len(stack)-1] != ".." {
				stack = stack[:len(stack)-1]
			} else if prefix == "" {
				stack = append(stack, part)
			}
		default:
			stack = append(stack, part)
		}
	}
	if prefix == "/" || strings.HasSuffix(prefix, "/") {
		return prefix + strings.Join(stack, "/")
	}
	if len(stack) == 0 {
		if prefix == "" {
			return "."
		}
		return prefix
	}
	if prefix == "" && onlyBroadSuffix(strings.Join(stack, "/")) {
		return "./" + strings.Join(stack, "/")
	}
	return prefix + strings.Join(stack, "/")
}

func isLiteralHomeExpression(text string) bool {
	lower := strings.ToLower(text)
	for _, prefix := range []string{"$home", "${home}", "$userprofile", "${userprofile}", "%userprofile%", "$env:home", "${env:home}", "$env:userprofile", "${env:userprofile}"} {
		if lower == prefix || strings.HasPrefix(lower, prefix+"/") {
			return true
		}
	}
	return false
}

func onlyBroadSuffix(suffix string) bool {
	for i := 0; i < len(suffix); i++ {
		if suffix[i] == '[' {
			// Range and character-class bytes belong to the glob, not a fixed child name.
			if end := strings.IndexByte(suffix[i+1:], ']'); end >= 0 {
				body := suffix[i+1 : i+1+end]
				if strings.ContainsAny(body, "/\\") {
					return false
				}
				i += end + 1
				continue
			}
		}
		if !strings.ContainsRune("/*?[]!", rune(suffix[i])) {
			return false
		}
	}
	return true
}
