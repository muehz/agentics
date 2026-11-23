package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// launcherScriptRel is the shell launcher Git Bash, macOS and Linux run. A CR
// byte breaks its shebang line and every command it runs.
const launcherScriptRel = "bin/agx"

// launcherFormat is the contract name whose machine format the committed
// launcher must have: hookbuild.LauncherTarget is windows/amd64, so the
// launcher is checked as a PE32+ x86-64 image.
const launcherFormat = "agx-windows-amd64.exe"

// checkBinLaunchers is the bin-launchers rule. bin/ is the directory a user
// puts on PATH, so it holds exactly the two launchers: any other name would
// be a command no reproducibility gate covers. bin/agx must contain no CR
// byte, and .gitattributes must mark bin/agx.exe binary so Git never
// rewrites its bytes on checkout. The rule reads files only; the git mode of
// bin/agx is checked by a test that queries the index. An absent bin/ or
// .gitattributes is a finding, never an operational error.
func checkBinLaunchers(repoRoot string, sourceOnly bool) []Failure {
	var failures []Failure
	binDir := filepath.Join(repoRoot, "bin")
	launchers := []string{filepath.Base(launcherScriptRel), filepath.Base(hookbuild.LauncherOutput)}

	// An unreadable bin/ lists nothing; the missing-launcher findings below
	// then report it.
	entries, _ := os.ReadDir(binDir)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || slices.Contains(launchers, name) {
			continue
		}
		failures = append(failures, Failure{
			Rule: "bin-launchers", Path: "bin/" + name,
			Message: "bin/ may hold only agx and agx.exe; delete this stray entry — bin/ is on the user's PATH, so any other name is a command no freshness or rebuild check covers",
		})
	}
	for _, name := range launchers {
		rel := "bin/" + name
		if sourceOnly && rel == hookbuild.LauncherOutput {
			continue
		}
		data, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil {
			failures = append(failures, Failure{
				Rule: "bin-launchers", Path: rel,
				Message: fmt.Sprintf("the launcher is missing or unreadable (%v); commit it (make -C hooks/go build-all builds bin/agx.exe)", err),
			})
			continue
		}
		if rel == launcherScriptRel && bytes.IndexByte(data, '\r') >= 0 {
			failures = append(failures, Failure{
				Rule: "bin-launchers", Path: rel,
				Message: "contains a CR byte; a shell cannot run a CRLF script, so commit it with LF line endings",
			})
		} else if rel == launcherScriptRel {
			digest := sha256.Sum256(data)
			if got := hex.EncodeToString(digest[:]); got != shellwords.AgxLauncherSHA256 {
				failures = append(failures, Failure{
					Rule: "bin-launchers", Path: rel,
					Message: fmt.Sprintf("SHA-256 differs from shellwords.AgxLauncherSHA256 (got %s)", got),
				})
			}
		}
	}
	if !sourceOnly && !marksLauncherBinary(filepath.Join(repoRoot, ".gitattributes")) {
		failures = append(failures, Failure{
			Rule: "bin-launchers", Path: ".gitattributes",
			Message: "no `bin/agx.exe binary` line; without it Git may convert line endings inside the committed launcher, and the rebuild comparison then fails on every checkout",
		})
	}
	return failures
}

// marksLauncherBinary reports whether a non-comment line of the attributes
// file names bin/agx.exe first and sets binary among its attributes. A
// missing file marks nothing.
func marksLauncherBinary(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[0] == hookbuild.LauncherOutput && slices.Contains(fields[1:], "binary") {
			return true
		}
	}
	return false
}
