package commandguard

import (
	"testing"
	"time"
)

// TestReadNpmLongLine bounds readNpm on a long line of options, so hostile input cannot push the guard past its
// deadline.
func TestReadNpmLongLine(t *testing.T) {
	values := make([]string, 0, 20002)
	values = append(values, "npm")
	for range 20000 {
		values = append(values, "--json")
	}
	values = append(values, "publish")
	words := literalWordsForTest(values...)

	start := time.Now()
	cmd := readNpm(words)
	elapsed := time.Since(start)

	if !cmd.found || cmd.subcommand.Text != "publish" {
		t.Fatalf("readNpm found subcommand %q (found %v), want publish", cmd.subcommand.Text, cmd.found)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("readNpm took %s on %d words, want at most 2s", elapsed, len(words))
	}
}

// TestReadNpmLongLineAmbiguousPrefix bounds readNpm on a long line of an option prefix that matches several npm long
// options without being one. Each such word makes the long-option lookups scan both flag maps, so a pass that rescans
// per word would grow quadratically. The check compares the best of several runs at two sizes rather than a wall-clock
// bound, so it holds on a slow or loaded host and still fails on quadratic growth on a fast one.
func TestReadNpmLongLineAmbiguousPrefix(t *testing.T) {
	const prefix = "--a"
	if _, ok := npmBooleanConfigFlags[prefix]; ok {
		t.Fatalf("%s is a boolean option, want an ambiguous prefix", prefix)
	}
	if _, ok := npmValueConfigFlags[prefix]; ok {
		t.Fatalf("%s is a value option, want an ambiguous prefix", prefix)
	}

	run := func(n int) time.Duration {
		values := make([]string, 0, n+2)
		values = append(values, "npm")
		for range n {
			values = append(values, prefix)
		}
		values = append(values, "publish")
		words := literalWordsForTest(values...)

		start := time.Now()
		cmd := readNpm(words)
		elapsed := time.Since(start)

		if !cmd.found || cmd.subcommand.Text != "publish" {
			t.Fatalf("readNpm found subcommand %q (found %v) on %d words, want publish", cmd.subcommand.Text, cmd.found, len(words))
		}
		return elapsed
	}

	best := func(n int) time.Duration {
		fastest := run(n)
		for range ambiguousPrefixRepeats - 1 {
			fastest = min(fastest, run(n))
		}
		return fastest
	}

	small := best(5000)
	large := best(20000)
	if growsFasterThanLinear(small, large) {
		t.Fatalf("readNpm took %s on 20000 words and %s on 5000, want growth below %dx plus %s", large, small, maxLinearGrowthRatio, growthTimerAllowance)
	}
}

const (
	// ambiguousPrefixRepeats is how many times each size runs; the fastest run is the one least disturbed by the host.
	ambiguousPrefixRepeats = 3
	// maxLinearGrowthRatio sits between the 4x that linear work shows for a 4x larger input and the 16x of quadratic
	// work, with room above 4x for cache and allocation effects.
	maxLinearGrowthRatio = 10
	// growthTimerAllowance absorbs timer resolution and scheduling jitter when the small run is only a few milliseconds.
	growthTimerAllowance = 20 * time.Millisecond
)

// growsFasterThanLinear reports whether large, the time for four times the input of small, is too far beyond a linear
// multiple of small to be linear work.
func growsFasterThanLinear(small, large time.Duration) bool {
	return large > maxLinearGrowthRatio*small+growthTimerAllowance
}

func TestGrowsFasterThanLinear(t *testing.T) {
	tests := []struct {
		name         string
		small, large time.Duration
		want         bool
	}{
		{"linear on a slow runner", 600 * time.Millisecond, 2771 * time.Millisecond, false},
		{"linear on a fast host", 50 * time.Millisecond, 200 * time.Millisecond, false},
		{"exactly four times", 100 * time.Millisecond, 400 * time.Millisecond, false},
		{"quadratic on a slow runner", 600 * time.Millisecond, 9600 * time.Millisecond, true},
		{"quadratic on a fast host", 50 * time.Millisecond, 800 * time.Millisecond, true},
		{"quadratic with a tiny small run", 5 * time.Millisecond, 80 * time.Millisecond, true},
		{"large time against a fast small run", 200 * time.Millisecond, 3200 * time.Millisecond, true},
		{"both near zero", time.Millisecond, 4 * time.Millisecond, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := growsFasterThanLinear(tt.small, tt.large); got != tt.want {
				t.Fatalf("growsFasterThanLinear(%s, %s) = %v, want %v", tt.small, tt.large, got, tt.want)
			}
		})
	}
}
