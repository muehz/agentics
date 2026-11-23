package flow

import (
	"path/filepath"
	"strings"
)

// localPointer is the pointer of a stale approval in the local store, whose
// files no commit holds, so no revision can be compared.
const localPointer = "the work folder is local to this clone; no commit holds it"

// stalePointer returns the pointer of a stale approval of artifact in the
// located flow loc, whose files stored holds: localPointer in the local store,
// and otherwise the commit approvedPointer finds.
func stalePointer(stored storedFolder, dir string, loc locatedFlow, artifact, key, want string) string {
	if stored.store == storageLocal {
		return localPointer
	}
	return approvedPointer(dir, loc, artifact, key, want)
}

// approvedPointer searches the last 20 commits touching artifact in the
// located flow's folder for the revision whose hash, by the rule key names
// ("text" for the second rule, otherwise the first), equals want.
func approvedPointer(dir string, loc locatedFlow, artifact, key, want string) string {
	const notFound = "approved content not found in the last 20 commits touching the file"
	if want == "" {
		return notFound
	}
	path := loc.dir
	prefix, err := gitBytes(path, nil, "rev-parse", "--show-prefix")
	if err != nil {
		return notFound
	}
	rel := strings.TrimRight(string(prefix), "\n") + artifact
	shas, err := gitBytes(path, nil, "log", "-n", "20", "--format=%H", "--", ":(top)"+rel)
	if err != nil {
		return notFound
	}
	for _, sha := range strings.Fields(string(shas)) {
		body, err := gitBytes(path, nil, "show", sha+":"+rel)
		if err != nil {
			// A deletion has no file revision, but an older commit can match.
			continue
		}
		var hash string
		if key == "text" {
			hash = textHash(artifact, body)
		} else {
			hash = hashApprovedBytes(dir, filepath.Join(path, artifact), artifact, body)
		}
		if hash == "" || hash != want {
			continue
		}
		short, err := gitBytes(path, nil, "rev-parse", "--short", sha)
		if err != nil {
			break
		}
		id := strings.TrimRight(string(short), "\n")
		return "compare with commit " + id + " (from the repository root): git diff " + id + " -- " + quotePointerPath(rel)
	}
	return notFound
}

func quotePointerPath(path string) string {
	for _, b := range []byte(path) {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.ContainsRune("._/@%+=:,-", rune(b))) {
			return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
		}
	}
	return path
}
