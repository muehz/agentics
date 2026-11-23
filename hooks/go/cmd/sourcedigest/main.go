// Command sourcedigest prints the digest of the Go source tree that builds
// the hook binaries, or with --list the files that digest hashes.
//
// Usage: sourcedigest [--list] [dir]
//
// dir is the module root and defaults to the working directory. --list is
// accepted only as the first argument and prints hookprovenance.SourceFiles,
// one module-relative path per line; without it the command prints
// hookprovenance.SourceDigest in hex. The Makefile stamps that digest into
// every hook binary and takes the list as the binaries' prerequisites.
//
// Exit status is 0 on success, 2 for a usage error, and 1 for any other
// error, such as a missing dir or a malformed digest-exclude. On failure
// nothing reaches stdout, so a caller never mistakes an error for a digest
// or a file list.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

const usage = "usage: sourcedigest [--list] [dir]"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	list := len(args) > 0 && args[0] == "--list"
	if list {
		args = args[1:]
	}
	// Any other dash argument is refused rather than read as a directory, so
	// a mistyped --list fails loudly instead of digesting a path named -lst.
	if len(args) > 1 || len(args) == 1 && strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	// SourceFiles lists go.mod without opening it, so without this check
	// --list on a regular file would print a file set for a tree that is not
	// there.
	info, err := os.Stat(dir)
	if err == nil && !info.IsDir() {
		err = fmt.Errorf("%s is not a directory", dir)
	}
	var out string
	if err == nil {
		out, err = render(dir, list)
	}
	if err != nil {
		fmt.Fprintf(stderr, "sourcedigest: %v\n", err)
		return 1
	}
	if _, err := io.WriteString(stdout, out); err != nil {
		fmt.Fprintf(stderr, "sourcedigest: %v\n", err)
		return 1
	}
	return 0
}

// render returns the complete stdout for dir, so a failure part-way leaves
// nothing written.
func render(dir string, list bool) (string, error) {
	if list {
		files, err := hookprovenance.SourceFiles(dir)
		if err != nil {
			return "", err
		}
		return strings.Join(files, "\n") + "\n", nil
	}
	digest, err := hookprovenance.SourceDigest(dir)
	if err != nil {
		return "", err
	}
	return digest + "\n", nil
}
