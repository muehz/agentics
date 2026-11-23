package commandguard

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// protectedCallerArgument reports whether one caller-supplied word names something command-guardian protects from
// deletion (the root, the home directory, a path with a .git or .env component) or is a flag that bypasses a check.
// Matching is on the whole word.
func protectedCallerArgument(arg string) bool {
	switch arg {
	case "/", "~", "--no-verify", "--force":
		return true
	}
	if isNoVerifyAbbrev(arg) || scanShortNoVerifyCluster(arg) {
		return true
	}
	// The current and parent directory are ordinary arguments to a formatter or a linter; every other broad target is not.
	if arg != "." && arg != ".." && arg != "./" && arg != "../" && isBroadTarget(shellwords.Word{Text: arg, Raw: arg, Literal: true}) {
		return true
	}
	for _, component := range strings.FieldsFunc(arg, func(r rune) bool { return r == '/' || r == '\\' }) {
		if protectedDeletionComponent(component) {
			return true
		}
	}
	return false
}

// checkScriptCallerArguments denies a call whose literal arguments reach a script that uses its parameters and name a
// protected operand. An unquoted pattern argument is denied as well, because the script receives the names the shell
// expands it to, which the call does not show. A script that uses no parameters ignores its arguments. report takes a
// label, never matched text, and the fixed advice never repeats the argument, which can be text the script's own
// caller composed.
func checkScriptCallerArguments(calls []shellwords.ScriptCall, report reportFunc) {
	for _, call := range calls {
		if !call.ConsumesParams {
			continue
		}
		denied := call.PatternArgs
		for _, word := range call.CallerWords() {
			denied = denied || protectedCallerArgument(word)
		}
		if denied {
			refusal := shellwords.CallerArgumentRefusal(call)
			remedy, _ := refusal.Remedy()
			report(ruleScriptCallerArgument, refusal.String(), remedy)
		}
	}
}

// scanShortNoVerifyCluster reports a short-option cluster that Git reads as -n on commit or am, the two commands where -n
// skips the hooks. A caller -n reaches any parameter-consuming script this way, so a dry-run flag is refused too. The
// word is read as the first argument after git commit and after git am.
func scanShortNoVerifyCluster(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	word := shellwords.Word{Text: arg, Raw: arg, Literal: true}
	for _, verb := range []string{"commit", "am"} {
		if gitSkipsHooksByLetter(hookBypassVerbSpecs[verb].Scan([]shellwords.Word{word}).Args[0]) {
			return true
		}
	}
	return false
}
