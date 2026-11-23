package hookutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// GuardLog is the guard.log owner for one guard: where its directory is, how
// the working directory anchors the ownership check, and how a record is
// appended. Guard names the guard in messages (for example
// "command-guardian"); EnvDir names the environment variable that overrides
// the directory. Each guard keeps its own record type and its own response
// to an error from Append.
type GuardLog struct{ Guard, EnvDir string }

// Dir resolves where guard.log lives: the value of the EnvDir environment
// variable when it is set and not empty, else agentics/logs/<Guard> under
// logRoot when logRoot is not empty, else the hidden .agentics/logs/<Guard>
// under the process working directory. The parts are joined with a slash so
// a filesystem error names the same path on every platform.
func (g GuardLog) Dir(logRoot string) string {
	if g.EnvDir != "" {
		if dir := os.Getenv(g.EnvDir); dir != "" {
			return dir
		}
	}
	if logRoot != "" {
		return logRoot + "/agentics/logs/" + g.Guard
	}
	return ".agentics/logs/" + g.Guard
}

// Append writes record as one JSON line, without HTML escaping, to guard.log
// in Dir(logRoot), creating the directory on the way. An encoding error is
// returned as is. The working directory anchors the ownership check that the
// append runs against a relative directory: an unreadable working
// directory cannot be trusted to bound that check, so the write fails with
// "<Guard> could not resolve the working directory: <cause>" instead of
// walking an unresolved root. A relative directory is joined to the working
// directory, and the append goes through appendOwnedLog with the working
// directory as the trusted root. A path that would redirect the write
// returns an error that errors.Is ErrLogAlias and reads "<Guard> refused to
// append guard.log: a linked path component or a non-regular guard.log would
// redirect the write". Any other error from the append is returned as is.
func (g GuardLog) Append(logRoot string, record any) error {
	var line bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(record); err != nil {
		return err
	}
	trusted, err := os.Getwd()
	if err == nil && runtime.GOOS == "darwin" {
		// Getwd can return the old pathname after macOS has removed the directory.
		err = checkWorkingDirectory(trusted)
	}
	if err != nil {
		return fmt.Errorf("%s could not resolve the working directory: %w", g.Guard, err)
	}
	dir := g.Dir(logRoot)
	if !filepath.IsAbs(filepath.FromSlash(dir)) {
		dir = filepath.Join(trusted, dir)
	}
	if err := appendOwnedLog(trusted, dir, "guard.log", line.Bytes()); err != nil {
		if errors.Is(err, ErrLogAlias) {
			return guardAliasError{guard: g.Guard}
		}
		return err
	}
	return nil
}

func checkWorkingDirectory(path string) error {
	named, err := os.Stat(path)
	if err != nil {
		return err
	}
	current, err := os.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(named, current) {
		return &os.PathError{Op: "getwd", Path: path, Err: os.ErrNotExist}
	}
	return nil
}

// guardAliasError is the redirected-path error with the guard's own sentence;
// Unwrap keeps errors.Is(err, ErrLogAlias) true.
type guardAliasError struct{ guard string }

func (e guardAliasError) Error() string {
	return e.guard + " refused to append guard.log: a linked path component or a non-regular guard.log would redirect the write"
}

func (guardAliasError) Unwrap() error { return ErrLogAlias }
