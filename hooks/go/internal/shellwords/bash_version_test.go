package shellwords

import (
	"fmt"
	"testing"
)

// bashOlderThan44 reports whether a bash with this version is older than 4.4. The parser models bash 4.4 and later; on
// older bash, `time` after `time` or `!` is not a keyword and some heredoc forms read differently.
func bashOlderThan44(major, minor int) bool {
	return major < 4 || (major == 4 && minor < 4)
}

// oldBashSkipMessage is the skip reason for a bash older than 4.4.
func oldBashSkipMessage(major, minor int) string {
	return fmt.Sprintf("bash %d.%d is older than 4.4: the parser models bash 4.4 and later, and these constructs read differently on older bash", major, minor)
}

func TestBashOlderThan44(t *testing.T) {
	for _, c := range []struct {
		major, minor int
		want         bool
	}{
		{3, 2, true}, {4, 3, true}, {4, 4, false}, {5, 0, false}, {5, 2, false},
	} {
		if got := bashOlderThan44(c.major, c.minor); got != c.want {
			t.Errorf("bashOlderThan44(%d, %d) = %v, want %v", c.major, c.minor, got, c.want)
		}
	}
}
