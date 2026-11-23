package flow

import (
	"bytes"
	"os/exec"
	"strings"
	"sync"
)

// costCounter counts the work whose growth lookup must bound: each Git process
// by its subcommand, and each state.tsv read by its path. It is nil unless a
// test installs one, so production pays only a nil check.
type costCounter struct {
	mu         sync.Mutex
	gitStarts  map[string]int
	stateReads map[string]int
}

var cost *costCounter

func countGit(args []string) {
	if cost == nil {
		return
	}
	sub := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			sub = arg
			break
		}
	}
	cost.mu.Lock()
	cost.gitStarts[sub]++
	cost.mu.Unlock()
}

func countStateRead(path string) {
	if cost == nil {
		return
	}
	cost.mu.Lock()
	cost.stateReads[path]++
	cost.mu.Unlock()
}

// runGit starts every Git process of the package, so that one place bounds and
// counts them.
func runGit(dir string, input []byte, args ...string) ([]byte, error) {
	countGit(args)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	return cmd.Output()
}

// gitText mirrors shell command substitution: discard a failed answer and
// remove trailing LF only. Artifact and pipeline bytes must not use this path.
func gitText(dir string, args ...string) string {
	out, err := runGit(dir, nil, args...)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}
