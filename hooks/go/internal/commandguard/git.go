package commandguard

import (
	"context"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const (
	gitCleanSuggestion = "Scope it with an explicit pathspec — 'git clean -f <path>' is allowed; preview with 'git clean -n <path>' first. Whole-tree cleans and -x/-X (ignored files) stay blocked."
	gitResetSuggestion = "Use 'git restore <path>' (or 'git restore .') to discard working-tree changes, 'git stash' to keep them; 'git reset --hard HEAD' (exactly HEAD) is allowed."
)

// checkGitEnvironmentCommands reports the first Git rule violation in
// invocations. bare holds assignments from segments that run no command; the
// guard cannot know whether the shell exports them, so they count as Git hook
// bypasses the same way command-prefix assignments do.
func checkGitEnvironmentCommands(ctx context.Context, invocations []shellwords.Invocation, bare []shellwords.Word, pending gitHookIssue, report reportFunc) {
	for _, inv := range invocations {
		g, isGit := readGitInvocation(inv)
		if isGit && inv.Inspectable {
			args := g.Args
			switch g.Subcommand() {
			case "clean":
				if !inv.Complete {
					report(ruleGitClean, "git clean", gitCleanSuggestion)
					return
				}
				if match, denied := unsafeGitClean(args); denied {
					report(ruleGitClean, match, gitCleanSuggestion)
					return
				}
			case "reset":
				if !inv.Complete {
					report(ruleGitResetHard, "git reset --hard", gitResetSuggestion)
					return
				}
				if match, denied := unsafeGitReset(args); denied {
					report(ruleGitResetHard, match, gitResetSuggestion)
					return
				}
			}
		}
		issue := inspectGitHookBypass(ctx, inv, g, isGit)
		switch issue.kind {
		case confirmedGitHookBypass:
			report(issue.rule, issue.match, hookBypassSuggestion)
			return
		case unknownGitHookEffect:
			pending = preferGitIssue(pending, issue)
		}
	}
	pending = preferGitIssue(pending, hooksPathCandidate(bare))
	if pending.kind == confirmedGitHookBypass {
		report(pending.rule, pending.match, hookBypassSuggestion)
	} else if pending.kind == unknownGitHookEffect {
		report(pending.rule, pending.match, gitIssueSuggestion(pending.rule))
	}
}

// readGitInvocation reads an invocation whose literal head names git, in any
// letter case and with any directory or extension, and that has at least one
// word after the head. The bool reports only that head test: the caller
// still requires Inspectable, and a line whose state is not GitVerb selects no
// subcommand, so the clean and reset rules never read a word after an option
// the grammar cannot place or after one that only prints.
func readGitInvocation(inv shellwords.Invocation) (argv.Git, bool) {
	if len(inv.Words) < 2 || !inv.Words[0].Literal || !strings.EqualFold(shellwords.ExecutableName(inv.Words[0].Text), "git") {
		return argv.Git{}, false
	}
	return argv.ReadGit(inv.Words[1:]), true
}

func unsafeGitClean(args []shellwords.Word) (string, bool) {
	force, dryRun, ignored, endOptions, skipNext := false, false, false, false, false
	var pathspecs []shellwords.Word
	for _, word := range args {
		text := word.Text
		if skipNext {
			skipNext = false
			continue
		}
		if !endOptions && text == "--" {
			endOptions = true
			continue
		}
		if !endOptions {
			switch text {
			case "--force":
				force = true
				continue
			case "--dry-run":
				dryRun = true
				continue
			case "--no-dry-run":
				dryRun = false
				continue
			case "--exclude":
				skipNext = true
				continue
			}
			if strings.HasPrefix(text, "--exclude=") || text == "--quiet" {
				continue
			}
			if len(text) > 1 && text[0] == '-' && text[1] != '-' {
				for j := 1; j < len(text); j++ {
					switch text[j] {
					case 'f':
						force = true
					case 'n':
						dryRun = true
					case 'x', 'X':
						ignored = true
					case 'e':
						// -e takes the remaining suffix as its pattern;
						// those bytes are data even when they contain flag
						// letters. Without a suffix, consume the next word.
						if j+1 == len(text) {
							skipNext = true
						}
						j = len(text)
					}
				}
				continue
			}
			if strings.HasPrefix(text, "-") {
				continue
			}
		}
		pathspecs = append(pathspecs, word)
	}
	if !force || dryRun {
		return "", false
	}
	if ignored || len(pathspecs) == 0 {
		return "git clean", true
	}
	for _, pathspec := range pathspecs {
		if !pathspec.Literal || strings.HasPrefix(pathspec.Text, ":") || isBroadTarget(pathspec) {
			return "git clean " + pathspec.Raw, true
		}
	}
	return "", false
}

func unsafeGitReset(args []shellwords.Word) (string, bool) {
	hard, endOptions := false, false
	var targets []shellwords.Word
	for _, word := range args {
		text := word.Text
		if !endOptions && text == "--" {
			endOptions = true
			continue
		}
		if !endOptions && text == "--hard" {
			hard = true
			continue
		}
		if !endOptions && strings.HasPrefix(text, "-") {
			continue
		}
		targets = append(targets, word)
	}
	if !hard {
		return "", false
	}
	if len(targets) == 1 && targets[0].Literal && targets[0].Text == "HEAD" {
		return "", false
	}
	if len(targets) == 0 {
		return "git reset --hard", true
	}
	return "git reset --hard " + targets[0].Raw, true
}
