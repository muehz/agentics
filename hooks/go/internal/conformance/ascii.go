package conformance

import (
	"fmt"
	"os"
	"path/filepath"
)

// runASCII ports validate.yml's "run-hook.cmd must be ASCII-only" step.
//
// cmd.exe's batch parser tracks file position in BYTES; a multibyte UTF-8
// sequence desyncs it into executing comment fragments as commands; an em-dash
// can turn the dispatcher header into garbage and a deny into exit 255.
// The entire polyglot must therefore be pure ASCII because cmd parses it from
// the top, including the shell branch.
func runASCII(repoRoot string) ([]Failure, error) {
	path := filepath.Join("hooks", "run-hook.cmd")
	data, err := os.ReadFile(filepath.Join(repoRoot, path))
	if err != nil {
		return nil, fmt.Errorf("run-hook-ascii: reading %s: %w", path, err)
	}

	line := 1
	for _, b := range data {
		if b == '\n' {
			line++
			continue
		}
		if b > 0x7F {
			return []Failure{{
				Rule: "run-hook-ascii",
				// Reported paths are repo-relative and forward-slash on every OS;
				// path stays OS-native because it is also used to read the file.
				Path: filepath.ToSlash(path),
				Message: fmt.Sprintf(
					"non-ASCII byte 0x%02X at line %d — cmd.exe misparses multibyte sequences; use ASCII only",
					b, line),
			}}, nil
		}
	}
	return nil, nil
}
