package flow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// gitBytes keeps artifact and pipeline bytes intact and distinguishes an empty
// successful answer from failure, such as rev-parse at the repository root.
func gitBytes(dir string, input []byte, args ...string) ([]byte, error) {
	return runGit(dir, input, args...)
}

// contentHashAt is the first-rule hash of artifact read from the directory
// files, with Git attributes taken as if it lay in named. A flow in the local
// store is read there but named by its work-root path, so that it hashes as
// the same text committed under the work root would.
func contentHashAt(dir, files, named, artifact string) string {
	body, err := os.ReadFile(filepath.Join(files, artifact))
	if err != nil {
		return ""
	}
	return hashApprovedBytes(dir, filepath.Join(named, artifact), artifact, body)
}

func hashApprovedBytes(dir, path, artifact string, body []byte) string {
	out, err := gitBytes(dir, approvedText(artifact, body), "hash-object", "--stdin", "--path="+filepath.ToSlash(path))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

func approvedText(artifact string, body []byte) []byte {
	if artifact != "plan.md" && artifact != "intent.md" && artifact != "spec.md" {
		return body
	}
	var kept [][]byte
	skip := false
	for len(body) > 0 {
		line, rest, _ := bytes.Cut(body, []byte{'\n'})
		body = rest
		heading := bytes.TrimSuffix(line, []byte{'\r'})
		if bytes.HasPrefix(heading, []byte("## ")) {
			skip = bytes.Equal(heading, []byte("## Divergences")) || (artifact == "plan.md" && bytes.Equal(heading, []byte("## Standing orders")))
		}
		if !skip {
			kept = append(kept, line)
		}
	}
	if artifact != "plan.md" {
		for len(kept) > 0 && len(bytes.TrimSuffix(kept[len(kept)-1], []byte{'\r'})) == 0 {
			kept = kept[:len(kept)-1]
		}
	}
	var out bytes.Buffer
	for _, line := range kept {
		out.Write(line)
		// awk print terminates even an originally unterminated final record.
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// textArtifacts are the artifacts the second approval rule binds, in the order
// status prints their hashes.
var textArtifacts = []string{"intent.md", "spec.md", "plan.md", "brief.md"}

// fileTextHash returns the second-rule hash of artifact in dir, and false when
// the artifact is not a readable regular file.
func fileTextHash(dir, artifact string) (string, bool) {
	file := filepath.Join(dir, artifact)
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}
	return textHash(artifact, body), true
}

// textHash is the second approval rule. It runs no Git process and applies no
// Git attribute, so one text hashes the same on every platform and under any
// core.autocrlf value; the first rule, approvedText with git hash-object, is
// kept unchanged so rows written under it still verify.
func textHash(artifact string, body []byte) string {
	name := path.Base(filepath.ToSlash(artifact))
	known := slices.Contains(textArtifacts, name)
	var kept [][]byte
	skip := false
	for _, line := range artifactLines(body) {
		// A heading line inside a fenced code block is code, so it neither starts
		// nor ends a section.
		if known && line.section {
			skip = string(line.text) == "## Divergences" || (name == "plan.md" && string(line.text) == "## Standing orders")
		}
		if !skip {
			kept = append(kept, line.text)
		}
	}
	for len(kept) > 0 && len(kept[len(kept)-1]) == 0 {
		kept = kept[:len(kept)-1]
	}
	sum := sha256.New()
	for _, line := range kept {
		sum.Write(line)
		sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// patchHeader is the commit header of every patch handed to git patch-id: the
// form it recognizes, fixed so that no format.pretty setting changes it.
const patchHeader = "commit %H"

// patchDiffOptions keep a user's diff settings out of every patch handed to git
// patch-id. Color codes leave patch-id no diff to read, so it prints no id, and
// an external diff driver or a textconv filter rewrites the diff, so one change
// would get a different id under another user's configuration.
var patchDiffOptions = []string{"--no-color", "--no-ext-diff", "--no-textconv"}

func patchIDOf(dir, sha string) string {
	args := append([]string{"show", "--format=" + patchHeader}, patchDiffOptions...)
	patch, err := gitBytes(dir, nil, append(args, sha)...)
	if err != nil {
		return ""
	}
	out, err := gitBytes(dir, patch, "patch-id", "--stable")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
