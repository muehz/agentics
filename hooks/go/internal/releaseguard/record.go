package releaseguard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// recordMarker is the H1 prefix of an /agentics:gate change record
// (skills/gate/references/change/change-record-template.md).
const recordMarker = "# Change Record:"

// recordMaxAge bounds how long a record authorizes deploys: a record is set
// for a release session, not exported from a profile.
const recordMaxAge = 24 * time.Hour

// recordClockSkew tolerates a marker-bearing file whose mtime is slightly
// ahead of this process's clock — a few minutes of drift on a networked
// filesystem or container — while still rejecting an mtime further in the
// future than that: a future timestamp cannot pre-authorize deploys.
const recordClockSkew = 5 * time.Minute

// recordReadLimit bounds the bytes read while looking for the marker.
const recordReadLimit = 64 << 10

// recordHashLimit bounds the bytes hashed when AGX_RELEASE_RECORD_SHA256
// is set: a record is a short change-record document, not an arbitrary file.
const recordHashLimit = 16 << 20

// recordHashVar names the host-set content hash that, when present, binds a
// release record's bytes to a known value.
const recordHashVar = "AGX_RELEASE_RECORD_SHA256"

// recordRemedy is the one remediation clause every release-guardian deny
// reason ends with, shared by Run and the tests that assert its message.
const recordRemedy = "run /agentics:gate change ($agentics:gate change on Codex) and set the release record in the host environment"

// errRecordInvalid is the generic "no usable record" sentinel: a missing
// path, a missing or non-regular or empty file, a stale or future mtime, an
// unreadable file, or one with no template marker all report it. The caller
// tests for it with errors.Is and falls back to its own generic
// production-shaped deny message.
var errRecordInvalid = errors.New("release record is missing, unreadable, stale, or unmarked")

// errRecordHashNotHex, errRecordTooLarge and errRecordHashMismatch are the
// three AGX_RELEASE_RECORD_SHA256 failures. Their Error() text is the
// reason phrase for a host-set hash that fails to validate, with no
// "release-guardian: " prefix and no remedy suffix — the caller assembles
// both around whichever of these it gets back, so the phrase is declared
// once instead of twice.
var (
	errRecordHashNotHex   = errors.New("the host-set record hash is not a SHA-256 hex digest")
	errRecordTooLarge     = errors.New("the release record exceeds 16 MiB")
	errRecordHashMismatch = errors.New("the release record does not match the host-set record hash")
)

// validReleaseRecord reports whether path names a fresh /agentics:gate change record.
// Existence alone is not enough: an agent can create an empty file with an
// unguarded command, so the file must carry the template's marker and a
// recent modification time — one neither older than recordMaxAge nor set
// into the future beyond recordClockSkew. When AGX_RELEASE_RECORD_SHA256 is
// set — even to the empty string, which is a misconfiguration rather than
// "unset" — the record's content must also hash to that value.
//
// nil means valid. A non-nil result naming errRecordHashNotHex,
// errRecordTooLarge or errRecordHashMismatch (test with errors.Is) is a
// host-set-hash failure specifically; every other invalid path returns
// errRecordInvalid, including one with no AGX_RELEASE_RECORD_SHA256
// configured — the caller falls back to its own generic deny message in
// that case.
func validReleaseRecord(path string, now time.Time) error {
	if path == "" {
		return errRecordInvalid
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return errRecordInvalid
	}
	if age := now.Sub(info.ModTime()); age > recordMaxAge || age < -recordClockSkew {
		return errRecordInvalid
	}
	handle, err := os.Open(path)
	if err != nil {
		return errRecordInvalid
	}
	defer handle.Close()
	head, err := io.ReadAll(io.LimitReader(handle, recordReadLimit))
	if err != nil {
		return errRecordInvalid
	}
	if !bytes.Contains(head, []byte(recordMarker)) {
		return errRecordInvalid
	}
	want, hashConfigured := os.LookupEnv(recordHashVar)
	if !hashConfigured {
		return nil
	}
	// A host that exports the variable empty is misconfigured, not
	// unconfigured: it must fail closed the same as a malformed digest,
	// consistent with AGX_RELEASE_ENVIRONMENTS. len(want) != 64 already
	// covers "".
	if len(want) != 64 {
		return errRecordHashNotHex
	}
	if _, err := hex.DecodeString(want); err != nil {
		return errRecordHashNotHex
	}
	if _, err := handle.Seek(0, io.SeekStart); err != nil {
		return errRecordInvalid
	}
	hasher := sha256.New()
	n, err := io.Copy(hasher, io.LimitReader(handle, recordHashLimit+1))
	if err != nil {
		return errRecordInvalid
	}
	if n > recordHashLimit {
		return errRecordTooLarge
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, want) {
		return errRecordHashMismatch
	}
	return nil
}
