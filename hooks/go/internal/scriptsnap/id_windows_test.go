//go:build windows

package scriptsnap

import "testing"

func TestWindowsIdentityRejectsDegenerateIndexes(t *testing.T) {
	for _, c := range [][3]uint32{{1, 0, 0}, {1, 0xFFFFFFFF, 0xFFFFFFFF}} {
		if got := windowsID(c[0], c[1], c[2]); got != "" {
			t.Errorf("%v gave %q", c, got)
		}
	}
	if got := windowsID(1, 2, 3); got != "1:2:3" {
		t.Fatalf("got %q", got)
	}
}
