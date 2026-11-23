package scriptsnap

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestReadSizeBoundary(t *testing.T) {
	atLimit := strings.Repeat("a", MaxScriptBytes-1) + "\n"
	got := New().ReadScript(write(t, "exact.sh", []byte(atLimit)))
	if got.Limit != shellwords.RuleNone || got.Bytes != MaxScriptBytes || len(got.Text) != MaxScriptBytes {
		t.Fatalf("a script of exactly the limit must be read: limit=%q bytes=%d", got.Limit, got.Bytes)
	}
	over := strings.Repeat("a", MaxScriptBytes) + "\n"
	got = New().ReadScript(write(t, "over.sh", []byte(over)))
	if got.Limit != shellwords.RuleTooLarge || got.Text != "" {
		t.Fatalf("one byte over the limit must be refused: %+v", got)
	}
}

func TestReadAcceptsPlainTextAndEmpty(t *testing.T) {
	for name, body := range map[string]string{"empty.sh": "", "utf8.sh": "echo \U000000e9\U00004e2d\n", "nonl.sh": "echo a"} {
		got := New().ReadScript(write(t, name, []byte(body)))
		if got.Limit != shellwords.RuleNone || got.Text != body || got.Bytes != len(body) {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// A UTF-16 byte-order mark is also invalid UTF-8, so only the reason distinguishes a dedicated check from the
// generic one.
func TestByteOrderMarksAreNamed(t *testing.T) {
	for name, body := range map[string]string{
		"utf8bom.sh":  "\xef\xbb\xbfecho a\n",
		"utf16le.sh":  "\xff\xfee\x00c\x00",
		"utf16be.sh":  "\xfe\xff\x00e\x00c",
		"utf16bom.sh": "\xff\xfe",
	} {
		got := New().ReadScript(write(t, name, []byte(body)))
		if got.Limit != shellwords.RuleEncoding || got.Detail != "byte-order mark" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestRefusalDetailHoldsNoFileBytes(t *testing.T) {
	const marker = "SECRETMARKER"
	bodies := map[string]string{
		"cr.sh":      marker + "\r\n",
		"nulbyte.sh": marker + "\x00\n",
		"bad.sh":     marker + "\xff\n",
		"bom.sh":     "\xef\xbb\xbf" + marker + "\n",
		"big.sh":     marker + strings.Repeat("a", MaxScriptBytes),
		"utf16.sh":   "\xff\xfe" + marker,
	}
	for name, body := range bodies {
		got := New().ReadScript(write(t, name, []byte(body)))
		if got.Limit == shellwords.RuleNone || got.Detail == "" {
			t.Errorf("%s: want a named limit with a detail, got %+v", name, got)
		}
		if strings.Contains(got.Detail, marker) || strings.Contains(got.Text, marker) {
			t.Errorf("%s: file bytes leaked: %+v", name, got)
		}
	}
}
