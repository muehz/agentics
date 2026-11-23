package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// The dispatcher supplies the installed root. The executable running this
// code may reside in the versioned user cache, outside the installation.
func writeStartupHint(provider, root string, stdin io.Reader, stdout io.Writer) {
	id, known := surface.Parse(provider)
	if !known || (id == surface.Copilot && !freshCopilotSession(stdin)) {
		return
	}
	path := installedLauncher(root)
	if path == "" {
		return
	}
	bash := "'" + strings.ReplaceAll(filepath.ToSlash(path), "'", "'\\''") + "' help"
	// PowerShell recognizes curly single quotes as string delimiters too.
	quote := strings.NewReplacer("'", "''", "‘", "'‘", "’", "'’", "‚", "'‚", "‛", "'‛")
	hint := "For agx commands, run PowerShell: & '" + quote.Replace(path) + "' help or Bash: " + bash + "."
	var response any
	switch id {
	case surface.ClaudeCode, surface.Codex:
		response = map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": "SessionStart", "additionalContext": hint}}
	case surface.Copilot:
		response = map[string]string{"additionalContext": hint}
	case surface.Antigravity:
		response = map[string]any{"injectSteps": []any{map[string]any{"systemMessage": map[string]string{"systemMessage": hint}}}}
	}
	_ = json.NewEncoder(stdout).Encode(response)
}

func installedLauncher(root string) string {
	if !startupRootAllowed(root) {
		return ""
	}
	root = filepath.Clean(root)
	bin := filepath.Join(root, "bin")
	hooks := filepath.Join(root, "hooks")
	name := "agx"
	dirs := []string{root, bin, hooks}
	scripts := []string{filepath.Join(hooks, "run-hook.cmd")}
	if runtime.GOOS == "windows" {
		// The native launcher provisions through both scripts and rejects
		// redirected directories anywhere between its install and the drive.
		name = "agx.exe"
		for dir := root; filepath.Dir(dir) != dir; {
			dir = filepath.Dir(dir)
			dirs = append(dirs, dir)
		}
		scripts = append(scripts, filepath.Join(hooks, "fetch-hook.ps1"))
	}
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if err != nil || info.Mode().Type() != os.ModeDir {
			return ""
		}
	}
	path := filepath.Join(bin, name)
	for _, file := range append(scripts, path) {
		info, err := os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() {
			return ""
		}
	}
	if _, err := exec.LookPath(path); err != nil {
		return ""
	}
	return path
}

func startupRootAllowed(root string) bool {
	return filepath.IsAbs(root) && utf8.ValidString(root) && strings.IndexFunc(root, unicode.IsControl) < 0 &&
		(runtime.GOOS != "windows" || len(filepath.VolumeName(root)) == 2)
}

// Copilot exposes the start source on stdin and has no sessionStart matcher.
func freshCopilotSession(stdin io.Reader) bool {
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(stdin, limit+1))
	if err != nil || len(data) > limit {
		return false
	}
	var event struct{ Source string }
	return json.Unmarshal(data, &event) == nil && (event.Source == "startup" || event.Source == "new")
}
