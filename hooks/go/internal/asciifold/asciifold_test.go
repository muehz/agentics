package asciifold

import (
	"regexp"
	"strings"
	"testing"
)

func TestStringFoldsExactlyTheRunesGoFoldsIntoASCII(t *testing.T) {
	cases := map[string]string{
		"":                                "",
		"already lower":                   "already lower",
		"MiXeD Case 123":                  "mixed case 123",
		"\u017F" + "ecret=x":              "secret=x",
		"\u212A" + "elvin K":              "kelvin k",
		"caf" + "\u00e9" + " " + "\u00c9": "caf" + "\u00e9" + " " + "\u00c9", // Latin-1 letters are not ASCII letters
		"\xff\xfeAB":                      "\xff\xfeab",                      // invalid UTF-8 bytes copy through
	}
	for in, want := range cases {
		if got := String(in); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStringReturnsInputWhenUnchanged(t *testing.T) {
	in := "lower only, no folds"
	if got := String(in); got != in {
		t.Fatalf("changed a string with nothing to fold")
	}
	if n := testing.AllocsPerRun(100, func() { String(in) }); n != 0 {
		t.Fatalf("String allocated %v times on an unchanged input", n)
	}
}

// TestPrefilterSoundness is the invariant every gate in the guards rests
// on: when the folded text lacks a literal, no (?i) regexp requiring that
// literal can match the original text. Seeds are joined at run time so
// none of them is a credential-shaped literal in the source.
func TestPrefilterSoundness(t *testing.T) {
	literals := []string{"secret", "key", "sudo ", "rm -rf /", "kubectl"}
	value := " = " + strings.Repeat("x", 8)
	seeds := []string{
		"SECRET" + value, "\u017F" + "ecret" + value, "\u212A" + "EY" + value, "Kelvin K", "kubectl apply",
		"KUBECTL APPLY", "\u212A" + "ubectl apply", "sudo rm -rf /", "SUDO RM -RF /", "\u017F" + "udo ls",
		"nothing here", "s" + "\u0301" + "ecret", "\u017F", "\u212A", "\xffSECRET" + value,
	}
	for _, literal := range literals {
		re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(literal))
		for _, seed := range seeds {
			if re.MatchString(seed) && !strings.Contains(String(seed), literal) {
				t.Errorf("gate %q would skip %q, which %s matches", literal, seed, re)
			}
		}
	}
}
