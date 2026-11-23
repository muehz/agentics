package commandguard

import "github.com/muehz/agentics/hooks/go/internal/shellwords"

// The gh rule suggestions, shared by the regex rules in ghScanner.check and
// their invocation-path counterparts, so both paths name the same remedy.
const (
	ghPrCreateSuggestion    = "Add --draft (or -d). If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."
	ghPrReadySuggestion     = "Do not mark the pull request ready for review. Tell the user that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."
	ghStackSubmitSuggestion = "Add --auto without --open to create draft pull requests. If the user wants the pull requests ready for review, tell them that agents cannot publish them as ready and ask them to mark each one ready in the pull request UI or with 'gh pr ready'."
	ghStackLinkSuggestion   = "Drop --open so the linked pull requests remain drafts. If the user wants them ready for review, tell them that agents cannot publish them as ready and ask them to mark each one ready in the pull request UI or with 'gh pr ready'."
)

// checkGhVerbInvocation gates the three create/stack verbs on a parsed gh
// invocation. The regex rules and this path both run on every gh invocation,
// reading two different spellings of the same command: the regex rules read
// the blanked command line, this path walks the parsed words, and Run dedupes
// the two readings by the reported verb. This path covers what the regex
// rules cannot see: a quoted executable, a global flag before the
// subcommand, a .exe suffix. Quoted words are never read as flags, the
// stricter reading the regex rules also take.
func checkGhVerbInvocation(command string, words, args []shellwords.Word, globalHelp, complete bool, report reportFunc) {
	switch command {
	case "pr create", "stack submit", "stack link":
	default:
		return
	}
	match := joinInvocation(words)
	help, draft, auto, open := booleanFlagState{}, booleanFlagState{}, booleanFlagState{}, booleanFlagState{}
	for _, arg := range args {
		token := arg.Text
		if isPosixCommentWord(arg) || token == "--" {
			break
		}
		if arg.Quoted {
			continue
		}
		switch {
		case observeGhBooleanFlag(&help, token, "--help"), observeGhBooleanFlag(&help, token, "-h"):
		case observeGhBooleanFlag(&draft, token, "--draft"):
		case command == "pr create" && observeGhBooleanFlag(&draft, token, "-d"):
		case observeGhBooleanFlag(&auto, token, "--auto"):
		case observeGhBooleanFlag(&open, token, "--open"):
		}
	}
	if complete && (globalHelp || help.effectiveTrue()) {
		return
	}
	openSeen := open.trueSeen || open.invalidSeen
	switch command {
	case "pr create":
		if complete && draft.effectiveTrue() {
			return
		}
		report(ruleGhPrCreate, match, ghPrCreateSuggestion)
	case "stack submit":
		if complete && auto.effectiveTrue() && !openSeen {
			return
		}
		report(ruleGhStackSubmit, match, ghStackSubmitSuggestion)
	case "stack link":
		// Stricter than the regex rule, which reads --open anywhere: an
		// incomplete invocation denies here, and only the spellings the
		// regex cannot see reach this path.
		if openSeen || !complete {
			report(ruleGhStackLink, match, ghStackLinkSuggestion)
		}
	}
}
