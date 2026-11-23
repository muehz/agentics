package scriptsnap

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func write(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSnapshotIsOnceOnly(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	s := New()
	first := s.ReadScript(p)
	if first.Limit != shellwords.RuleNone || first.Text != "echo one\n" {
		t.Fatalf("first read: %+v", first)
	}
	if err := os.WriteFile(p, []byte("rm -rf /\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := s.ReadScript(p); second != first {
		t.Fatalf("second read differs: %+v vs %+v", second, first)
	}
}

func TestSnapshotRefusalIsOnceOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "late.sh")
	s := New()
	first := s.ReadScript(p)
	if first.Limit != shellwords.RuleUnreadable {
		t.Fatalf("missing file: %+v", first)
	}
	if err := os.WriteFile(p, []byte("echo one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := s.ReadScript(p); second != first {
		t.Fatalf("a refusal must stay final: %+v vs %+v", second, first)
	}
}

func TestSnapshotKeysAreIndependent(t *testing.T) {
	a := write(t, "a.sh", []byte("echo a\n"))
	b := write(t, "b.sh", []byte("echo b\n"))
	s := New()
	ra, rb := s.ReadScript(a), s.ReadScript(b)
	if ra.Text != "echo a\n" || rb.Text != "echo b\n" {
		t.Fatalf("paths shared an entry: %+v %+v", ra, rb)
	}
}

func TestSnapshotConcurrentReadersAgree(t *testing.T) {
	p := write(t, "a.sh", []byte("echo one\n"))
	s := New()
	var wg sync.WaitGroup
	results := make([]shellwords.ScriptRead, 16)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = s.ReadScript(p) }(i)
	}
	wg.Wait()
	for _, r := range results {
		if r.SHA256 == "" || r.Limit != shellwords.RuleNone || r != results[0] {
			t.Fatalf("readers disagree: %+v", results)
		}
	}
}

func TestReadRecordsHashAndBytes(t *testing.T) {
	body := []byte("echo one\n")
	p := write(t, "a.sh", body)
	sum := sha256.Sum256(body)
	got := New().ReadScript(p)
	if got.SHA256 != hex.EncodeToString(sum[:]) || got.Bytes != len(body) {
		t.Fatalf("got %+v", got)
	}
}

func TestReadRefusals(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		limit shellwords.Rule
	}{
		{"crlf.sh", []byte("echo a\r\necho b\r\n"), shellwords.RuleEncoding},
		{"cr.sh", []byte("echo a\recho b\n"), shellwords.RuleEncoding},
		{"nulbyte.sh", []byte("echo a\x00b\n"), shellwords.RuleEncoding},
		{"bad.sh", []byte("echo \xff\xfe\n"), shellwords.RuleEncoding},
		{"bom.sh", []byte("\xef\xbb\xbfecho a\n"), shellwords.RuleEncoding},
		{"utf16.sh", []byte("\xff\xfee\x00c\x00"), shellwords.RuleEncoding},
		{"utf16be.sh", []byte("\xfe\xff\x00e\x00c"), shellwords.RuleEncoding},
		{"big.sh", []byte(strings.Repeat("# x\n", (MaxScriptBytes/4)+1)), shellwords.RuleTooLarge},
	}
	for _, tc := range cases {
		got := New().ReadScript(write(t, tc.name, tc.data))
		if got.Limit != tc.limit || got.Text != "" || got.SHA256 != "" || got.Bytes != 0 || strings.Contains(got.Detail, "echo") {
			t.Errorf("%s: want %s with no text, got %+v", tc.name, tc.limit, got)
		}
	}
	dir := t.TempDir()
	if got := New().ReadScript(dir); got.Limit != shellwords.RuleUnreadable {
		t.Errorf("directory: %+v", got)
	}
	if got := New().ReadScript(filepath.Join(dir, "missing.sh")); got.Limit != shellwords.RuleUnreadable {
		t.Errorf("missing: %+v", got)
	}
}
