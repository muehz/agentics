package commandguard

import (
	"regexp"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// A repository's Git hooks run its checks before a commit. An agent that points core.hooksPath elsewhere, empties
// a hook, or removes one skips them without --no-verify, so C10 cannot see it. C11 judges the file edit itself: the
// write-tool forms here, and the shell forms in hookedit_shell.go. Modifying a hook is not a bypass, so a write
// that leaves a hook non-empty and sets no hooksPath passes.
//
// The guard reads the call and never the file. An Edit that replaces a whole file's content with nothing, a Codex
// apply_patch that adds or updates a protected file with empty text (the adapter drops it) or only deletes one,
// and the shell limits settingsedit.go lists are not detected. A Codex patch carries no paired old text, so it
// counts every added hooksPath line.
const (
	hookEditCategory   = "git_hook_bypass"
	hookEditSuggestion = "Do not edit, empty, move or delete the Git hook files (.git/config hooksPath, .git/hooks, .husky, .pre-commit-config.yaml) to skip them. Tell the user which hook blocks the change and ask them to change it themselves."
)

// isGitConfigPath reports whether p names a repository's .git/config.
func isGitConfigPath(p string) bool {
	c := cleanSettingsPath(p)
	return c == ".git/config" || strings.HasSuffix(c, "/.git/config")
}

// isHookFilePath reports whether p names a file C11 protects: .git/config, a file under .git/hooks/ or .husky/, or
// .pre-commit-config.yaml. It normalizes as isSettingsPath does: either separator, any case, and the trailing dots
// and spaces Windows drops.
func isHookFilePath(p string) bool {
	c := cleanSettingsPath(p)
	switch {
	case isGitConfigPath(c):
		return true
	case strings.HasPrefix(c, ".git/hooks/"), strings.Contains(c, "/.git/hooks/"):
		return true
	case strings.HasPrefix(c, ".husky/"), strings.Contains(c, "/.husky/"):
		return true
	}
	return c == ".pre-commit-config.yaml" || strings.HasSuffix(c, "/.pre-commit-config.yaml")
}

// isHookDirectory reports whether p names the .husky or .git/hooks directory itself. Removing or moving the
// directory drops every hook in it, so the shell forms treat it as protected. The write-tool forms name files.
func isHookDirectory(p string) bool {
	c := cleanSettingsPath(p)
	return c == ".husky" || strings.HasSuffix(c, "/.husky") || c == ".git/hooks" || strings.HasSuffix(c, "/.git/hooks")
}

// isHookPath reports whether p is a protected file or directory.
func isHookPath(p string) bool { return isHookFilePath(p) || isHookDirectory(p) }

// hooksPathLine matches a git-config line that assigns hooksPath, with or without a section header before it.
var hooksPathLine = regexp.MustCompile(`(?i)^\s*(?:\[[^\]]*\]\s*)?hookspath\s*=`)

// addsHooksPath reports whether text adds a hooksPath assignment. A line that old, the text an edit replaces,
// already holds is not an addition. A comment line never matches. Git skips a UTF-8 byte order mark at the start of
// a config file, but neither TrimSpace nor the regexp's \s strips it, so it is removed from each line here.
func addsHooksPath(text, old string) bool {
	oldLines := map[string]bool{}
	for _, line := range strings.Split(old, "\n") {
		oldLines[strings.TrimPrefix(strings.TrimSpace(line), "\xef\xbb\xbf")] = true
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimPrefix(strings.TrimSpace(line), "\xef\xbb\xbf")
		if hooksPathLine.MatchString(line) && !oldLines[line] {
			return true
		}
	}
	return false
}

// wholeFileContentEmpty reports whether a Write call carries whole-file content and every such value is empty or
// whitespace-only. The keys are Claude Code's content and Copilot's file_text; an Edit's new_string is a fragment
// and never counts.
func wholeFileContentEmpty(w toolcall.Writes) bool {
	if len(w.WholeFileTexts) == 0 {
		return false
	}
	for _, text := range w.WholeFileTexts {
		if strings.TrimSpace(text) != "" {
			return false
		}
	}
	return true
}

// hookEditTarget returns the protected path a file-writing tool call bypasses the hooks through: a text that adds
// hooksPath to .git/config, or a Write that empties a protected file. It reads the call's argument layers, never its
// proof layers, and does not read the write-parse-error flag: a call the adapter could not read is judged on the
// targets and texts that can be seen.
func hookEditTarget(fields normalizedFields) (string, bool) {
	if !toolcall.IsFileWrite(fields.toolName) {
		return "", false
	}
	writes := fields.call.Writes()
	for _, written := range writes.Texts {
		if isGitConfigPath(written.Path) && addsHooksPath(written.Text, written.Old) {
			return written.Path, true
		}
	}
	if fields.toolName == "Write" && wholeFileContentEmpty(writes) {
		for _, target := range writes.Targets {
			if isHookFilePath(target) {
				return target, true
			}
		}
	}
	return "", false
}
