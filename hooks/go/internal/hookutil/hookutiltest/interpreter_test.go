package hookutiltest

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// stubInterpreter writes a stub named name that exits with code into a new directory and returns the directory.
func stubInterpreter(t *testing.T, name string, code int) string {
	t.Helper()
	dir := t.TempDir()
	body, file := "#!/bin/sh\nexit "+strconv.Itoa(code)+"\n", name
	if runtime.GOOS == "windows" {
		body, file = "@exit /b "+strconv.Itoa(code)+"\r\n", name+".cmd"
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInterpreterPassesOverABrokenCandidate(t *testing.T) {
	broken, working := stubInterpreter(t, "bash", 1), stubInterpreter(t, "bash", 0)
	got, err := FindInterpreter("bash", broken+string(os.PathListSeparator)+working)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != working {
		t.Fatalf("resolved %s, want the candidate in %s", got, working)
	}
}

func TestInterpreterSkipsWhenOnlyBrokenCandidateExists(t *testing.T) {
	broken := stubInterpreter(t, "bash", 1)
	_, outcome, msg := decideInterpreter("bash", broken, "")
	if outcome != interpreterSkip || !strings.HasPrefix(msg, "requireInterpreter: bash is not available") {
		t.Fatalf("got outcome %d, message %q", outcome, msg)
	}
}

func TestInterpreterFailsWhenRequired(t *testing.T) {
	broken := stubInterpreter(t, "bash", 1)
	_, outcome, msg := decideInterpreter("bash", broken, "sh, bash")
	if outcome != interpreterFail || !strings.HasPrefix(msg, "requireInterpreter: bash is required by AGX_REQUIRED_INTERPRETERS but not available") {
		t.Fatalf("got outcome %d, message %q", outcome, msg)
	}
	if _, outcome, _ := decideInterpreter("bash", broken, "sh"); outcome != interpreterSkip {
		t.Fatalf("a list that omits bash must still skip, got outcome %d", outcome)
	}
}

func TestInterpreterListParsing(t *testing.T) {
	got, err := parseRequiredInterpreters(" sh , ,bash,, ")
	if err != nil || !slices.Equal(got, []string{"sh", "bash"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	// A name that only contains bash is unknown: it fails the reader and never counts as bash.
	if _, err := parseRequiredInterpreters("bash5"); err == nil {
		t.Fatal("an unknown name must be an error")
	}
	broken := stubInterpreter(t, "bash", 1)
	if _, outcome, msg := decideInterpreter("bash", broken, "bash5"); outcome != interpreterFail || !strings.Contains(msg, "unknown interpreter") {
		t.Fatalf("got outcome %d, message %q", outcome, msg)
	}
}

func TestInterpreterIgnoresCI(t *testing.T) {
	t.Setenv("CI", "true")
	broken := stubInterpreter(t, "bash", 1)
	if _, outcome, _ := decideInterpreter("bash", broken, ""); outcome != interpreterSkip {
		t.Fatalf("CI alone must not turn a skip into a failure, got outcome %d", outcome)
	}
}
