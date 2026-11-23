package secretsguard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

const (
	globDirectoryLimit = 128
	globEntryLimit     = 4096
	globMatchLimit     = 4096
	globComponentLimit = 32
)

// A budget belongs to the complete hook invocation, including separate operands.
type globBudget struct{ directories, entries, matches int }

// cdpathUncertain reads CDPATH from the call's override containers. A set
// host CDPATH counts as unknown, and so does an uncertain Values reading, so a
// variant spelling of a container or of the name cannot leave a bare relative
// cd looking proven.
//
// Any source of CDPATH uncertainty prevents bare relative cd from proving cwd.
// Empty tool values cannot clear inherited or competing provenance without a
// known executor precedence contract.
func cdpathUncertain(o toolcall.Overrides) bool {
	if os.Getenv("CDPATH") != "" {
		return true
	}
	values, uncertain := o.Values("CDPATH")
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

// globOptionsUncertain reads BASHOPTS and GLOBIGNORE from the call's override
// containers. An unsupported host value withholds the glob proof, and so does an
// uncertain Values reading for either name.
//
// The resolver models ordinary Bash filename expansion. Options which can add
// matches withhold that proof; empty maps cannot clear inherited settings
// without an executor precedence contract.
func globOptionsUncertain(o toolcall.Overrides) bool {
	if unsupportedGlobOptions("BASHOPTS", os.Getenv("BASHOPTS")) || unsupportedGlobOptions("GLOBIGNORE", os.Getenv("GLOBIGNORE")) {
		return true
	}
	for _, name := range []string{"BASHOPTS", "GLOBIGNORE"} {
		values, uncertain := o.Values(name)
		if uncertain {
			return true
		}
		for _, value := range values {
			if unsupportedGlobOptions(name, value) {
				return true
			}
		}
	}
	return false
}

func unsupportedGlobOptions(name, value string) bool {
	if value == "" {
		return false
	}
	if name == "GLOBIGNORE" {
		return true
	}
	for _, option := range strings.Split(value, ":") {
		switch option {
		case "dotglob", "nocaseglob", "globstar", "extglob", "":
			return true
		}
		for _, ch := range option {
			if (ch < 'a' || ch > 'z') && ch != '_' {
				return true
			}
		}
	}
	return false
}

// Only an AND successor proves that a literal cd succeeded. Once control flow
// or a nested shell makes location ambiguous, relative globs cannot be resolved.
type shellGlobContext struct {
	cwd, globReason string
	conditionalCwd  bool
	groupUnknown    bool
	cdpath          bool
}

func (c *shellGlobContext) advance(inv shellwords.Invocation) {
	if c.conditionalCwd && inv.Before != shellwords.And {
		c.cwd = ""
		c.conditionalCwd = false
	}
	if inv.Before == shellwords.Group || inv.Before == shellwords.Substitution || inv.Parent >= 0 {
		c.groupUnknown = true
	}
}

func (c *shellGlobContext) observeAssignment(word shellwords.Word) {
	name, value, _ := strings.Cut(word.Text, "=")
	if name == "CDPATH" && (value != "" || !word.Literal) {
		c.cdpath = true
	}
	if name == "GLOBIGNORE" || name == "SHELLOPTS" || name == "BASHOPTS" {
		c.globReason = "explicit glob option changes are unsupported"
	}
}

func (c *shellGlobContext) observeCommand(inv shellwords.Invocation, verb string, andSuccessor bool, dialect shellwords.Dialect) {
	if dialect == shellwords.POSIX && (verb == "export" || verb == "readonly" || verb == "declare" || verb == "typeset") {
		for _, word := range inv.Words[1:] {
			c.observeAssignment(word)
		}
	}
	if verb == "shopt" || verb == "setopt" || verb == "unsetopt" {
		c.globReason = "explicit glob option changes are unsupported"
	}
	if verb == "set" {
		for _, word := range inv.Words[1:] {
			if strings.Contains(word.Text, "glob") || ((strings.HasPrefix(word.Text, "-") || strings.HasPrefix(word.Text, "+")) && strings.Contains(word.Text, "f")) {
				c.globReason = "explicit glob option changes are unsupported"
			}
		}
	}
	if dialect == shellwords.PowerShell {
		switch verb {
		case "sl":
			verb = "set-location"
		case "push-location":
			verb = "pushd"
		case "pop-location":
			verb = "popd"
		}
	}
	if verb != "cd" && verb != "chdir" && verb != "set-location" && verb != "pushd" && verb != "popd" {
		return
	}
	if c.groupUnknown || len(inv.Wrappers) != 0 {
		c.cwd = ""
		return
	}
	previous := c.cwd
	c.cwd = ""
	// An OR predecessor can skip this cd while its following AND still runs.
	if !andSuccessor || inv.Before == shellwords.Or || inv.PipelineInput || inv.PipelineOutput || (verb != "cd" && verb != "chdir") {
		return
	}
	args := inv.Words[1:]
	if len(args) == 2 && args[0].Text == "--" {
		args = args[1:]
	}
	if len(args) != 1 || !args[0].Literal || strings.ContainsAny(args[0].Text, "*?[~") || strings.HasPrefix(args[0].Text, "-") {
		return
	}
	destination := nativeShellPath(args[0].Text)
	if !filepath.IsAbs(destination) {
		if c.cdpath && dialect == shellwords.POSIX && destination != "." && destination != ".." && !strings.HasPrefix(destination, "./") && !strings.HasPrefix(destination, "../") {
			return
		}
		if !filepath.IsAbs(previous) {
			return
		}
		destination = filepath.Join(previous, destination)
	}
	info, err := os.Stat(destination)
	if err == nil && info.IsDir() {
		c.cwd = destination
		c.conditionalCwd = true
	}
}

// candidateContext returns the working directory and dialect a command runs under. A wrapper that is a PowerShell
// host or a POSIX shell sets the dialect; zsh and dash are kept as POSIX although the parser never unwraps them.
func (c *shellGlobContext) candidateContext(inv shellwords.Invocation, dialect shellwords.Dialect) (string, shellwords.Dialect) {
	cwd := c.cwd
	if c.groupUnknown || inv.Parent >= 0 || len(inv.Wrappers) != 0 {
		cwd = ""
	}
	for _, wrapper := range inv.Wrappers {
		name := strings.ToLower(shellwords.ExecutableName(wrapper.Text))
		switch argv.Lookup(name).Shell {
		case argv.PowerShellHost:
			dialect = shellwords.PowerShell
		case argv.PosixShell:
			dialect = shellwords.POSIX
		default:
			if name == "zsh" || name == "dash" {
				dialect = shellwords.POSIX
			}
		}
	}
	return cwd, dialect
}

func nativeShellPath(value string) string { return pathshape.Current().Native(value) }

// matchGlobComponent is filesystem-independent; POSIX leading dots require an
// explicit dot, while PowerShell wildcard comparisons ignore case.
func matchGlobComponent(pattern, name string, dialect shellwords.Dialect) (bool, error) {
	if dialect == shellwords.PowerShell {
		pattern, name = strings.ToLower(pattern), strings.ToLower(name)
	} else if strings.HasPrefix(name, ".") && !strings.HasPrefix(pattern, ".") && !strings.HasPrefix(pattern, `\.`) {
		return false, nil
	}
	return path.Match(pattern, name)
}

func shellGlobPattern(pattern string, dialect shellwords.Dialect) (string, error) {
	if dialect == shellwords.PowerShell {
		pattern = strings.ReplaceAll(pattern, `\`, "/")
		if strings.Contains(pattern, "[^") {
			return "", errors.New("unsupported PowerShell wildcard class")
		}
		var out strings.Builder
		for i := 0; i < len(pattern); i++ {
			if pattern[i] == '`' {
				if i+1 == len(pattern) {
					return "", errors.New("incomplete wildcard escape")
				}
				i++
				if strings.ContainsRune("*?[\\", rune(pattern[i])) {
					out.WriteByte('\\')
				}
			}
			out.WriteByte(pattern[i])
		}
		return out.String(), nil
	}
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		out.WriteByte(pattern[i])
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
			out.WriteByte(pattern[i])
			continue
		}
		if pattern[i] == '[' && i+1 < len(pattern) && pattern[i+1] == '!' {
			i++
			out.WriteByte('^')
		}
	}
	return out.String(), nil
}

func (b *globBudget) resolveShellGlob(candidate shellPath) ([]string, error) {
	if candidate.GlobReason != "" {
		return nil, errors.New(candidate.GlobReason)
	}
	pattern := candidate.Pattern
	if pattern == "" {
		pattern = candidate.Text
	}
	pattern, patternErr := shellGlobPattern(pattern, candidate.Dialect)
	if patternErr != nil {
		return nil, patternErr
	}
	pattern = nativeShellPath(pattern)
	if strings.Contains(pattern, "**") || strings.ContainsAny(pattern, "{}") || strings.Contains(pattern, "[[:") {
		return nil, errors.New("unsupported wildcard syntax")
	}
	root := candidate.Cwd
	volume := filepath.VolumeName(pattern)
	if candidate.Dialect == shellwords.PowerShell && strings.Contains(strings.TrimPrefix(pattern, volume), ":") {
		return nil, errors.New("non-filesystem wildcard location is unsupported")
	}
	if volume != "" && !filepath.IsAbs(pattern) {
		return nil, errors.New("drive-relative wildcard location is unsupported")
	}
	if filepath.IsAbs(pattern) {
		root = volume + string(filepath.Separator)
		pattern = strings.TrimLeft(strings.TrimPrefix(pattern, volume), "/\\")
	} else if !filepath.IsAbs(root) {
		return nil, errors.New("relative wildcard has no known absolute working directory")
	}
	parts := strings.Split(pattern, "/")
	if len(parts) > globComponentLimit {
		return nil, errors.New("pattern component limit exceeded")
	}
	for _, component := range parts {
		if _, err := path.Match(component, ""); err != nil {
			return nil, fmt.Errorf("malformed wildcard: %w", err)
		}
	}
	literalPrefix := len(parts)
	for i, component := range parts {
		if strings.ContainsAny(component, "*?[\\") {
			literalPrefix = i
			break
		}
	}
	var matches []string
	var walk func(string, int) error
	walk = func(parent string, index int) error {
		if index == len(parts) {
			if b.matches >= globMatchLimit {
				return errors.New("match limit exceeded")
			}
			b.matches++
			matches = append(matches, parent)
			return nil
		}
		part := parts[index]
		if part == "" || part == "." {
			return walk(parent, index+1)
		}
		if part == ".." {
			physical, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return err
			}
			return walk(filepath.Dir(physical), index+1)
		}
		// Literal prefixes avoid opening ancestors unrelated to the wildcard. PowerShell off Windows still lists
		// every component after the first wildcard, because it matches those case-insensitively; a literal before
		// it resolves by name, as PowerShell itself resolves it.
		if !strings.ContainsAny(part, "*?[\\") && (candidate.Dialect == shellwords.POSIX || runtime.GOOS == "windows" || index < literalPrefix) {
			next := filepath.Join(parent, part)
			info, err := os.Lstat(next)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				info, err = os.Stat(next)
				if err != nil {
					return err
				}
			}
			if index+1 < len(parts) && !info.IsDir() {
				return nil
			}
			return walk(next, index+1)
		}
		if b.directories >= globDirectoryLimit {
			return errors.New("directory open limit exceeded")
		}
		b.directories++
		dir, err := os.Open(parent)
		if errors.Is(err, os.ErrNotExist) {
			if _, linkErr := os.Lstat(parent); errors.Is(linkErr, os.ErrNotExist) {
				return nil
			}
		}
		if err != nil {
			return err
		}
		defer dir.Close()
		for {
			// One extra entry establishes exhaustion without materializing the directory.
			entries, readErr := dir.ReadDir(min(64, globEntryLimit-b.entries+1))
			for _, entry := range entries {
				if b.entries >= globEntryLimit {
					return errors.New("directory entry limit exceeded")
				}
				b.entries++
				matched, err := matchGlobComponent(part, entry.Name(), candidate.Dialect)
				if err != nil {
					return err
				}
				if !matched {
					continue
				}
				next := filepath.Join(parent, entry.Name())
				info, err := os.Stat(next)
				if err != nil {
					return err
				}
				if index+1 < len(parts) && !info.IsDir() {
					continue
				}
				if err := walk(next, index+1); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	return matches, nil
}

func globOverridePaths(values []string, cwd string) []string {
	resolved := append([]string(nil), values...)
	for _, value := range values {
		value = nativeShellPath(value)
		if !filepath.IsAbs(value) && filepath.IsAbs(cwd) {
			resolved = append(resolved, filepath.Join(cwd, value))
		}
	}
	return resolved
}

// Both spellings matter: a safe-looking alias can point into a credential store.
// Ordinary directories are not files, but explicit credential stores stay protected.
func matchedCredentialPaths(match string) ([]string, error) {
	resolved, err := filepath.EvalSymlinks(match)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(match)
	if err != nil {
		return nil, err
	}
	paths := []string{match, resolved}
	if !info.IsDir() {
		return paths, nil
	}
	var protected []string
	for _, p := range paths {
		probe := "/" + strings.TrimPrefix(normalizeCredentialPath(p), "/") + "/"
		if _, ok := credentialStore(probe); ok {
			protected = append(protected, p+"/__directory__")
		}
	}
	return protected, nil
}
