// Package scriptsnap reads local script files as data for the guards: one bounded, once-only read per file per
// request, so the three parallel guards inspect identical bytes. It never executes a script.
package scriptsnap

const (
	// MaxScriptBytes is the largest single script that is inspected.
	MaxScriptBytes = 256 << 10
	// MaxEventBytes is the total bytes of scripts inspected for one request. An alias of a file already read adds
	// nothing; a read without a file identity counts for every path that reaches it.
	MaxEventBytes = 1 << 20
	// MaxFiles is the number of distinct script paths one request may ask for. It counts paths, not successful reads:
	// a refused, unreadable or timed-out path spends a slot as well, and two paths to one file spend two.
	MaxFiles = 8
)
