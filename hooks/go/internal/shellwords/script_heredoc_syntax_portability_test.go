package shellwords

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestHeredocOracleSyntaxClassificationAgainstBash(t *testing.T) {
	o := requireOracle(t, oracleBash44())
	for _, row := range []struct {
		name       string
		text       string
		parseExit  int
		sourceExit int
		syntax     bool
		ran        []int
	}{
		{
			name:       "conditional source-time rejection",
			text:       "cat <<E; [[ a &&\nb ]]; echo L2 >&2\necho L3 >&2\nE\necho L5 >&2\n",
			parseExit:  -1,
			sourceExit: 2,
			syntax:     true,
		},
		{
			name:       "execution before later parse rejection",
			text:       "echo L1 >&2\nif\n",
			parseExit:  2,
			sourceExit: 2,
			syntax:     true,
			ran:        []int{1},
		},
		{
			name:       "valid script exits two at runtime",
			text:       "echo L1 >&2\nexit 2\necho L3 >&2\n",
			parseExit:  0,
			sourceExit: 2,
			ran:        []int{1},
		},
		{
			name:      "valid script preserves markers",
			text:      "echo L1 >&2\n: <<E\necho L3 >&2\nE\necho L5 >&2\n",
			parseExit: 0,
			ran:       []int{1, 5},
		},
		{
			name:      "bare conditional diagnostic is ordinary output",
			text:      "echo L1 >&2\nprintf '%s\\n' 'conditional binary operator expected' >&2\necho L3 >&2\n",
			parseExit: 0,
			ran:       []int{1, 3},
		},
		{
			name:      "wrong diagnostic filename is ordinary output",
			text:      "echo L1 >&2\nprintf '%s\\n' './other.sh: line 5: conditional binary operator expected' >&2\necho L3 >&2\n",
			parseExit: 0,
			ran:       []int{1, 3},
		},
		{
			name:      "nonnumeric diagnostic line is ordinary output",
			text:      "echo L1 >&2\nprintf '%s\\n' './t.sh: line five: conditional binary operator expected' >&2\necho L3 >&2\n",
			parseExit: 0,
			ran:       []int{1, 3},
		},
		{
			name:      "diagnostic suffix is ordinary output",
			text:      "echo L1 >&2\nprintf '%s\\n' './t.sh: line 5: conditional binary operator expected suffix' >&2\necho L3 >&2\n",
			parseExit: 0,
			ran:       []int{1, 3},
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			dir := t.TempDir()
			run := func(body, probe string) string { return runProbe(t, o, dir, body, probe) }
			// Bash can reject a conditional only while sourcing, even after -n accepted it.
			if row.parseExit >= 0 {
				parseOutput := run(row.text, `"$BASH" --norc --noprofile -n ./t.sh >/dev/null 2>&1; printf '%d\n' "$?"`)
				parseExit, err := strconv.Atoi(strings.TrimSpace(parseOutput))
				if err != nil || parseExit != row.parseExit {
					t.Fatalf("real Bash parse-only status = %q (error %v), want %d", parseOutput, err, row.parseExit)
				}
			}
			// The subshell lets the parent observe status even when the fixture calls exit.
			sourceOutput := run(row.text, "("+heredocMarkerProbe+") >/dev/null 2>&1; printf '%d\\n' \"$?\"")
			sourceExit, err := strconv.Atoi(strings.TrimSpace(sourceOutput))
			if err != nil || sourceExit != row.sourceExit {
				t.Fatalf("real Bash source status = %q (error %v), want %d", sourceOutput, err, row.sourceExit)
			}
			out := run(row.text, heredocMarkerProbe)
			got := parseHeredocOracleOutput(out)
			if got.syntax != row.syntax {
				t.Errorf("oracle syntax = %v, want %v with real Bash source status %d; output %q", got.syntax, row.syntax, sourceExit, out)
			}
			if markers := sortedKeys(got.ran); !slices.Equal(markers, row.ran) {
				t.Errorf("oracle ran markers %v, want %v; output %q", markers, row.ran, out)
			}
		})
	}
}
