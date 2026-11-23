package hookprovenance

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var faultNames = map[FaultKind]string{
	FaultCarriageReturn: "carriage return",
	FaultBlank:          "blank",
	FaultFieldCount:     "field count",
	FaultHashNotHex:     "hash not hex",
	FaultHashNotLower:   "hash not lower",
	FaultDuplicate:      "duplicate",
}

// writeManifest writes body to a fresh temporary file and returns its path.
func writeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SHA256SUMS")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestManifestFaults pins the single fault each line carries. Each pair row
// is a line that has the lower-ranked fault and would still have it once the
// higher-ranked fault is removed, so the row fails if the precedence is
// reordered. A blank line cannot carry any other fault, so blank pairs only
// with a carriage return (the line "\r"); the eleven pairs below are all that
// can be built.
func TestManifestFaults(t *testing.T) {
	lower := strings.Repeat("a", 64)
	upper := strings.Repeat("A", 64)
	notHex := strings.Repeat("z", 64)
	notHexUpper := "A" + strings.Repeat("z", 63)
	clean := lower + "  bin/x"
	for _, tc := range []struct {
		name string
		body string
		want []LineFault
	}{
		{"carriage return", lower + "  bin/x\r\n", []LineFault{{1, FaultCarriageReturn, "bin/x\r"}}},
		{"blank", " \t\n", []LineFault{{1, FaultBlank, ""}}},
		{"field count one", "broken\n", []LineFault{{1, FaultFieldCount, ""}}},
		{"field count three", clean + " extra\n", []LineFault{{1, FaultFieldCount, "bin/x"}}},
		{"hash not hex", notHex + "  bin/x\n", []LineFault{{1, FaultHashNotHex, "bin/x"}}},
		{"hash too short", lower[8:] + "  bin/x\n", []LineFault{{1, FaultHashNotHex, "bin/x"}}},
		{"hash not lower", upper + "  bin/x\n", []LineFault{{1, FaultHashNotLower, "bin/x"}}},
		{"duplicate", clean + "\n" + clean + "\n", []LineFault{{2, FaultDuplicate, "bin/x"}}},

		{"carriage return over blank", "\r\n", []LineFault{{1, FaultCarriageReturn, ""}}},
		{"carriage return over field count", "broken\r\n", []LineFault{{1, FaultCarriageReturn, ""}}},
		{"carriage return over hash not hex", notHex + "  bin/x\r\n", []LineFault{{1, FaultCarriageReturn, "bin/x\r"}}},
		{"carriage return over hash not lower", upper + "  bin/x\r\n", []LineFault{{1, FaultCarriageReturn, "bin/x\r"}}},
		{"carriage return over duplicate", clean + "\n" + lower + "\r  bin/x\n", []LineFault{{2, FaultCarriageReturn, "bin/x"}}},
		{"field count over hash not hex", notHex + "  bin/x extra\n", []LineFault{{1, FaultFieldCount, "bin/x"}}},
		{"field count over hash not lower", upper + "  bin/x extra\n", []LineFault{{1, FaultFieldCount, "bin/x"}}},
		{"field count over duplicate", clean + "\n" + clean + " extra\n", []LineFault{{2, FaultFieldCount, "bin/x"}}},
		{"hash not hex over hash not lower", notHexUpper + "  bin/x\n", []LineFault{{1, FaultHashNotHex, "bin/x"}}},
		{"hash not hex over duplicate", clean + "\n" + notHex + "  bin/x\n", []LineFault{{2, FaultHashNotHex, "bin/x"}}},
		{"hash not lower over duplicate", clean + "\n" + upper + "  bin/x\n", []LineFault{{2, FaultHashNotLower, "bin/x"}}},

		// A path is a duplicate only of an earlier fault-free line.
		{"duplicate of a faulted line is not a duplicate", upper + "  bin/x\n" + clean + "\n", []LineFault{{1, FaultHashNotLower, "bin/x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := LoadManifest(writeManifest(t, tc.body))
			if err != nil {
				t.Fatal(err)
			}
			got := manifest.Faults()
			if !slices.Equal(got, tc.want) {
				t.Fatalf("faults = %s, want %s", describeFaults(got), describeFaults(tc.want))
			}
		})
	}
}

func describeFaults(faults []LineFault) string {
	var parts []string
	for _, f := range faults {
		parts = append(parts, fmt.Sprintf("line %d %s %q", f.Line, faultNames[f.Kind], f.Path))
	}
	return "[" + strings.Join(parts, "; ") + "]"
}
