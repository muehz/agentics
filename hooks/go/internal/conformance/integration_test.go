package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// TestIntegrationDigestIsLineEndingInvariant proves the checkout's
// hookprovenance digest depends only on the content of the files SourceFiles
// names: an LF copy and a CRLF copy of exactly those files, beside empty
// copies of the digest-exclude directories, digest the same as the checkout.
// A checkout with core.autocrlf materializes the source as CRLF, so without
// this every committed binary would read as stale there. It also proves no
// listed file lies under a digest-exclude directory, so the Makefile's
// prerequisites and the digest cover one set.
func TestIntegrationDigestIsLineEndingInvariant(t *testing.T) {
	root := hosttest.Checkout(t)
	module := filepath.Join(root, filepath.FromSlash(engineRel))
	want, err := hookprovenance.SourceDigest(module)
	if err != nil {
		t.Fatalf("hookprovenance.SourceDigest: %v", err)
	}
	files, err := hookprovenance.SourceFiles(module)
	if err != nil {
		t.Fatalf("hookprovenance.SourceFiles: %v", err)
	}
	entries, err := hookprovenance.ExcludeList(module)
	if err != nil {
		t.Fatalf("hookprovenance.ExcludeList: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("digest-exclude lists no directory; the exclusion check would prove nothing")
	}
	for _, file := range files {
		for _, entry := range entries {
			if strings.HasPrefix(file, entry+"/") {
				t.Errorf("SourceFiles names %s under listed directory %s", file, entry)
			}
		}
	}

	for _, ending := range []string{"\n", "\r\n"} {
		copyDir := t.TempDir()
		// ExcludeList requires each listed directory to exist, and leaving
		// them empty proves the excluded content takes no part in the digest.
		for _, entry := range entries {
			if err := os.MkdirAll(filepath.Join(copyDir, filepath.FromSlash(entry)), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		gainedCR := false
		for _, file := range files {
			data, err := os.ReadFile(filepath.Join(module, filepath.FromSlash(file)))
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			// A carriage return in digest-exclude is a malformed line, not a
			// line ending, so the list keeps LF in both copies.
			if file != "digest-exclude" {
				converted := strings.ReplaceAll(text, "\n", ending)
				gainedCR = gainedCR || converted != text
				text = converted
			}
			dest := filepath.Join(copyDir, filepath.FromSlash(file))
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dest, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if ending == "\r\n" && !gainedCR {
			t.Fatal("the CRLF copy gained no carriage return; the comparison is inert")
		}
		got, err := hookprovenance.SourceDigest(copyDir)
		if err != nil {
			t.Fatalf("hookprovenance.SourceDigest(%q copy): %v", ending, err)
		}
		if got != want {
			t.Errorf("%q copy of SourceFiles digests %s, checkout digests %s", ending, got, want)
		}
	}
}
