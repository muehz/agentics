// Package flow implements the append-only workflow state CLI.
package flow

import (
	"fmt"
	"io"
	"time"
)

// Run executes a flow command with the caller's directory, environment, clock,
// and output streams. Its exit codes belong to flow, not to the hook protocol.
func Run(args []string, dir string, getenv func(string) string, now func() time.Time, stdout, stderr io.Writer) int {
	p := program{dir: dir, getenv: getenv, now: now, stdout: stdout, stderr: stderr, repo: newRepoView(dir, getenv)}
	return p.run(args, map[string]command{"log": p.log, "status": p.status, "where": p.where, "bind": p.bind, "path": p.path, "audit": p.audit})
}

type program struct {
	dir            string
	getenv         func(string) string
	now            func() time.Time
	stdout, stderr io.Writer
	// repo holds the Git facts of dir, asked once for the whole command,
	// including the log that bind runs.
	repo *repoView
}

type command func([]string) int

func (p *program) run(args []string, commands map[string]command) (exit int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			exit = p.fail(fmt.Sprintf("internal error: %v", recovered))
		}
	}()
	if len(args) == 0 {
		fmt.Fprint(p.stderr, usage)
		return 1
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(p.stdout, usage)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprint(p.stderr, usage)
		return 1
	}
	return cmd(args[1:])
}

func (p *program) fail(message string) int {
	fmt.Fprintln(p.stderr, "flow: "+message)
	return 1
}
