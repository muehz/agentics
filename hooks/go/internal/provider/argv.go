// Argv dispatch, shared by every surface. Every hook serves the surface's one
// tool-use event; this file parses the argv its hook configuration passes and
// rejects anything else, with one diagnostic shape on every surface:
//
//	<surface>: <hook>: <what is wrong>
package provider

import (
	"flag"
	"fmt"
	"io"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// dispatchForSurface is the shared body of every surface's dispatch. The event
// is read from the surface and no hook is named: every hook serves the same
// event, so pre-tool takes no arguments and any other hook takes exactly
// --mode block. register refuses a surface that names no event, so a
// registered adapter never yields an empty event.
func dispatchForSurface(t traits, id surface.ID, hook string, args []string) (string, *Rendered) {
	if hook == "pre-tool" {
		if len(args) != 0 {
			return "", argvReject(t, fmt.Sprintf("%s: %s: event commands take no arguments", t.name, hook))
		}
	} else if err := parseBlockArgs(args); err != nil {
		return "", argvReject(t, fmt.Sprintf("%s: %s: %v", t.name, hook, err))
	}
	return id.PreToolEvent(), nil
}

// parseBlockArgs accepts exactly the argv that carries --mode block.
//
// The only flag is --mode; it takes a value, an unknown flag or a positional
// argument is an error, and the flag package's own wording is used for the
// faults it detects.
func parseBlockArgs(args []string) error {
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}
	if *mode != "block" {
		return fmt.Errorf("--mode must be block")
	}
	return nil
}

// argvReject is one argv rejection in this surface's own shape: its empty body
// on stdout, the diagnostic on stderr, and its error exit — never the deny
// exit, because an argv typo must not read as a guardrail block.
func argvReject(t traits, message string) *Rendered {
	return &Rendered{
		Stdout: t.emptyBody,
		Stderr: []byte(message + "\n"),
		Exit:   t.errorExit,
	}
}
