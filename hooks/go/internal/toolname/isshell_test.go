package toolname

import "testing"

func TestIsShell(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Bash", true},
		{"PowerShell", true},
		{"Write", false},
		{"Edit", false},
		{"", false},
		// Case is part of the name; a host does not send these spellings.
		{"bash", false},
		{"powershell", false},
		// A namespaced spelling resolves through Base.
		{"mcp__host__Bash", true},
		{"host.PowerShell", true},
	}
	for _, tc := range cases {
		if got := IsShell(tc.in); got != tc.want {
			t.Errorf("IsShell(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
