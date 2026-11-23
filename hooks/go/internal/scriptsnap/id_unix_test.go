//go:build unix

package scriptsnap

import "testing"

func TestUnixIdentityWithoutInodeIsEmpty(t *testing.T) {
	if got := unixID(7, 0); got != "" {
		t.Fatalf("zero inode gave %q", got)
	}
	if got := unixID(7, 9); got != "7:9" {
		t.Fatalf("got %q", got)
	}
}
