package flow

import (
	"strings"
	"testing"
)

func TestCleanCell(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"a\tb\rc\nd", "a b c d"}, {" \t\n\v\f\rkept \t\n\v\f\r", "kept"},
		{"\u00a0kept\u00a0", "\u00a0kept\u00a0"}, {"\u2003kept\u2003", "\u2003kept\u2003"},
		{"", ""}, {"-", "'-"}, {"plain", "plain"},
	} {
		if got := cleanCell(tc.in); got != tc.want {
			t.Errorf("cleanCell(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	for _, prefix := range []string{"=", "+", "-", "@", "'"} {
		for _, pad := range []string{"", " \t\v\f"} {
			in := pad + prefix + "value \r\n"
			if got := cleanCell(in); got != "'"+prefix+"value" {
				t.Errorf("cleanCell(%q)=%q", in, got)
			}
		}
	}
	for _, ws := range []string{" ", "\t", "\n", "\v", "\f", "\r"} {
		if got := cleanCell(ws + "text" + ws); got != "text" {
			t.Errorf("whitespace %q retained: %q", ws, got)
		}
	}
}

func TestSessionCell(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{{"", "-"}, {" \t\v\f\r\n", "-"}, {"-", "'-"}, {" @id ", "'@id"}, {"\u00a0", "\u00a0"}} {
		if got := sessionCell(tc.in); got != tc.want {
			t.Errorf("sessionCell(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func validCellValues() [7]string {
	return [7]string{"execute", "verify", "task-1", "ok", "verified", "commit=abc1234 patch-id=" + strings.Repeat("b", 40), "execute next"}
}

func TestValidateCells(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		index                int
		value, field, reason string
	}{
		{0, "=intent", "phase", "expected one of: intent spec decompose execute gate"},
		{1, "bogus", "event", "expected one of: create challenge approve revoke diverge verify revert demote block unblock pivot waive branch release change skip na enter bind pause done review escape order commit drop note"},
		{2, " \t\v\f", "target", "a target is required"},
		{3, "OK", "status", "expected one of: ok failed blocked pending declined cancelled"},
		{4, "", "why", "a one-line reason is required"},
		{5, "", "evidence", "evidence is required; write none when there is nothing to point at"},
		{6, "", "next", "next is required on every row"},
		{6, "bogus next", "next", "the first word must be a phase name or done"},
		{6, "intent\vnext", "next", "the first word must be a phase name or done"},
	} {
		v := validCellValues()
		v[tc.index] = tc.value
		_, err := validateCells(v)
		want := "flow: invalid " + tc.field + ": '" + stripCell(tc.value) + "' -- " + tc.reason
		assertCellError(t, err, want)
	}
	v := validCellValues()
	v[0] = " \tintent\r\n"
	v[2] = " @target "
	v[4] = " a\nb\tc\rd "
	v[6] = " done\tfinish "
	got, err := validateCells(v)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "intent" || got[2] != "@target" || got[4] != "a b c d" || got[6] != "done finish" {
		t.Fatalf("pre-guard values changed: %q", got)
	}
	for _, phase := range strings.Fields(phases) {
		v := validCellValues()
		v[0] = phase
		if _, err := validateCells(v); err != nil {
			t.Errorf("phase %s: %v", phase, err)
		}
	}
	// These events fix their target, status or evidence, so each gets a valid row
	// instead of the generic one; review and escape are covered with their evidence.
	shaped := map[string][3]string{
		"commit": {"fix", "ok", "auto:HEAD"},
		"drop":   {"task-1", "cancelled", "none"},
		"order":  {"1", "ok", "auto"},
		"note":   {"coverage", "ok", "none"},
		"waive":  {"aggregate-review", "ok", "none"},
		"demote": {"wave-2", "ok", "none"},
	}
	for _, event := range strings.Fields(events) {
		if event == "review" || event == "escape" {
			continue
		}
		v := validCellValues()
		v[1] = event
		if s, ok := shaped[event]; ok {
			v[2], v[3], v[5] = s[0], s[1], s[2]
		}
		if _, err := validateCells(v); err != nil {
			t.Errorf("event %s: %v", event, err)
		}
	}
	for _, status := range strings.Fields(statuses) {
		v := validCellValues()
		v[3] = status
		if _, err := validateCells(v); err != nil {
			t.Errorf("status %s: %v", status, err)
		}
	}
}

func assertCellError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("error=%v want %q", err, want)
	}
	e, ok := err.(*cellError)
	if !ok || e.ExitCode() != 2 {
		t.Fatalf("cell error must exit 2: %#v", err)
	}
}
