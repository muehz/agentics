package hookutiltest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// interpreterProbeTimeout bounds one probe, so a hung launcher cannot stall a test run.
const interpreterProbeTimeout = 10 * time.Second

// requiredInterpretersEnv names the interpreters a CI job must have. A listed interpreter that is missing fails the
// test instead of skipping it, so a runner cannot pass on a silent skip. The CI variable is never consulted.
const requiredInterpretersEnv = "AGX_REQUIRED_INTERPRETERS"

var knownInterpreters = []string{"sh", "bash", "pwsh", "powershell"}

func probeArgs(name string) []string {
	if name == "pwsh" || name == "powershell" {
		return []string{"-NoProfile", "-Command", "exit 0"}
	}
	return []string{"-c", "exit 0"}
}

// FindInterpreter returns the absolute path of the first entry of pathList that holds a working name. A candidate
// works when it exits 0 for a trivial command within ten seconds. A candidate that is present but broken is passed
// over, because on Windows System32\bash.exe is the WSL launcher: it fails without an installed distribution and can
// precede a working Git bash. The error says why no candidate worked.
func FindInterpreter(name, pathList string) (string, error) {
	var failures []string
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			continue
		}
		// LookPath on the joined path applies PATHEXT on Windows.
		candidate, err := exec.LookPath(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if abs, err := filepath.Abs(candidate); err == nil {
			candidate = abs
		}
		ctx, cancel := context.WithTimeout(context.Background(), interpreterProbeTimeout)
		err = exec.CommandContext(ctx, candidate, probeArgs(name)...).Run()
		cancel()
		if err == nil {
			return candidate, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
	}
	if len(failures) == 0 {
		return "", fmt.Errorf("not found on PATH")
	}
	return "", fmt.Errorf("no candidate ran a trivial command: %s", strings.Join(failures, "; "))
}

// parseRequiredInterpreters splits the list, trims entries and drops empty ones. An entry that is not a known
// interpreter is an error, so a misspelt name cannot silently stop requiring anything.
func parseRequiredInterpreters(raw string) ([]string, error) {
	var required []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !slices.Contains(knownInterpreters, entry) {
			return nil, fmt.Errorf("requireInterpreter: unknown interpreter %q in %s; expected one of %s",
				entry, requiredInterpretersEnv, strings.Join(knownInterpreters, ", "))
		}
		required = append(required, entry)
	}
	return required, nil
}

type interpreterOutcome int

const (
	interpreterFound interpreterOutcome = iota
	interpreterSkip
	interpreterFail
)

// decideInterpreter is RequireInterpreter without the testing.TB, so the unit tests can inject PATH and the list.
func decideInterpreter(name, pathList, requiredRaw string) (string, interpreterOutcome, string) {
	if !slices.Contains(knownInterpreters, name) {
		return "", interpreterFail, fmt.Sprintf("requireInterpreter: unknown interpreter %q; expected one of %s",
			name, strings.Join(knownInterpreters, ", "))
	}
	required, err := parseRequiredInterpreters(requiredRaw)
	if err != nil {
		return "", interpreterFail, err.Error()
	}
	path, err := FindInterpreter(name, pathList)
	if err == nil {
		return path, interpreterFound, ""
	}
	if slices.Contains(required, name) {
		return "", interpreterFail, fmt.Sprintf("requireInterpreter: %s is required by %s but not available (%v)",
			name, requiredInterpretersEnv, err)
	}
	return "", interpreterSkip, fmt.Sprintf("requireInterpreter: %s is not available (%v)", name, err)
}

var (
	interpreterMu    sync.Mutex
	interpreterCache = map[string]string{}
)

// RequireInterpreter returns the absolute path of a working sh, bash, pwsh or powershell. Callers spawn that path and
// never the bare name, so the interpreter that passed the probe is the one that runs. A missing interpreter skips the
// test, or fails it when AGX_REQUIRED_INTERPRETERS lists it.
func RequireInterpreter(t testing.TB, name string) string {
	t.Helper()
	pathList := os.Getenv("PATH")
	required := os.Getenv(requiredInterpretersEnv)
	// A probe costs a process start, which is slow on Windows, so a found path is reused while PATH is unchanged.
	// The list is still read on every call, so an unknown name fails every test.
	key := name + "\x00" + pathList
	interpreterMu.Lock()
	cached, ok := interpreterCache[key]
	interpreterMu.Unlock()
	if ok {
		if _, err := parseRequiredInterpreters(required); err != nil {
			t.Fatal(err)
		}
		return cached
	}
	path, outcome, msg := decideInterpreter(name, pathList, required)
	switch outcome {
	case interpreterSkip:
		t.Skip(msg)
	case interpreterFail:
		t.Fatal(msg)
	}
	interpreterMu.Lock()
	interpreterCache[key] = path
	interpreterMu.Unlock()
	return path
}
