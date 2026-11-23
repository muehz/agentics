package hookcost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestSummarize(t *testing.T) {
	s := Summarize([]int64{1, 2, 3, 4, 100})
	if s.Count != 5 || s.MeanNS != 22 || s.MedianNS != 3 || s.P95NS != 100 || s.MinNS != 1 || s.MaxNS != 100 || math.Abs(s.StddevNS-math.Sqrt(1522)) > 1e-9 {
		t.Fatalf("bad statistics: %+v", s)
	}
	if got := Summarize([]int64{2, 4}).MedianNS; got != 3 {
		t.Fatalf("even median = %v", got)
	}
}

func TestMeasureColdOrder(t *testing.T) {
	var calls []string
	arm := Arm{ID: "ordering", Preflight: func() error { calls = append(calls, "preflight"); return nil }, Run: func() (int64, error) { calls = append(calls, "run"); return 7, nil }, Validate: func() error { calls = append(calls, "validate"); return nil }}
	records := Measure([]Arm{arm}, true, 1, nil)
	if !reflect.DeepEqual(calls, []string{"run", "validate"}) || !reflect.DeepEqual(records[0].SamplesNS, []int64{7}) {
		t.Fatalf("cold did extra work: %v %+v", calls, records)
	}
	calls = nil
	Measure([]Arm{arm}, false, 2, nil)
	if len(calls) != 8 || calls[0] != "preflight" {
		t.Fatalf("warm order: %v", calls)
	}
	arm.Validate = func() error { return errors.New("invalid first-call output") }
	records = Measure([]Arm{arm}, true, 1, nil)
	if records[0].Summary.Count != 0 || len(records[0].SamplesNS) != 0 || !reflect.DeepEqual(records[0].RejectedSamplesNS, []int64{7}) || records[0].Error == "" {
		t.Fatalf("invalid cold call counted as evidence: %+v", records[0])
	}
}

func TestPairedDeltas(t *testing.T) {
	deltas := PairedDeltas([]Record{{ID: "p/Bash/native/h", Layer: "native-launch", SamplesNS: []int64{2, 4}}, {ID: "p/Bash/in-process/h", Layer: "in-process-run", SamplesNS: []int64{3, 2}}})
	if len(deltas) != 1 || !reflect.DeepEqual(deltas[0].SamplesNS, []int64{-1, 2}) {
		t.Fatalf("lost paired/noisy differences: %+v", deltas)
	}
}

func TestConfigInventory(t *testing.T) {
	root, err := filepath.Abs(hosttest.Checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	registrations, err := hosttest.Registrations(filepath.Join(root, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	inventory := map[string]int{}
	for _, c := range ConfigsFrom(registrations) {
		inventory[c.Event]++
	}
	if len(inventory) != 2 || inventory["PreToolUse"] != 6 || inventory["SessionStart"] != 6 {
		t.Fatalf("command variants by event: %v, want six each for PreToolUse and SessionStart", inventory)
	}
	for _, provider := range surface.All() {
		for _, tool := range []string{"Bash", "Read"} {
			_, selected := Selected(t, registrations, provider, tool, "windows")
			if len(selected) != 1 {
				t.Fatalf("%s/%s: got %d, want 1", provider, tool, len(selected))
			}
			for _, c := range selected {
				if c.Command == "" || c.Field == "" || c.TimeoutSeconds <= 0 || len(c.Argv) < 2 {
					t.Fatalf("incomplete metadata: %+v", c)
				}
			}
		}
	}
}

func TestEventSelection(t *testing.T) {
	registrations, err := hosttest.Registrations(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, osName := range []string{"windows", "linux", "darwin"} {
		for _, provider := range surface.All() {
			for _, tool := range []string{"Bash", "Read"} {
				_, selected := Selected(t, registrations, provider, tool, osName)
				for _, c := range selected {
					if c.Event != "PreToolUse" {
						t.Fatalf("unexpected event %s", c.Event)
					}
				}
				if len(selected) != 1 {
					t.Fatalf("%s %s/%s selected %d pre-tool configs, want 1", osName, provider, tool, len(selected))
				}
			}
		}
	}
}

// jsonKeys returns the top-level keys of v's JSON encoding in encoded order.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key.(string))
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// The capture is a JSON document other tooling reads, so each record keeps
// the keys, and the key order, existing captures use.
func TestRecordKeys(t *testing.T) {
	summary := Summary{Count: 1}
	cases := []struct {
		name  string
		value any
		want  []string
	}{
		{"Config", Config{File: "f", Provider: "p", Event: "e", Hook: "h", Field: "d", Command: "c", Argv: []string{"a"}, Matcher: "m", TimeoutSeconds: 1},
			[]string{"File", "Provider", "Event", "Hook", "Field", "Command", "Argv", "Matcher", "TimeoutSeconds"}},
		{"Command", Command{Executable: "x", Args: []string{"a"}, WorkingDir: "w", RawConfig: "r", ExpandedConfig: "e", Payload: "p", TimeoutSeconds: 1},
			[]string{"Executable", "Args", "WorkingDir", "RawConfig", "ExpandedConfig", "Payload", "TimeoutSeconds"}},
		{"Span", Span{Name: "n", StartNS: 1, EndNS: 2},
			[]string{"Name", "StartNS", "EndNS"}},
		{"Record", Record{ID: "i", Layer: "l", Commands: []Command{{}}, SamplesNS: []int64{1}, Summary: summary, Spans: [][]Span{{{}}}, Error: "e", RejectedSamplesNS: []int64{1}},
			[]string{"ID", "Layer", "Commands", "SamplesNS", "Summary", "Spans", "Error", "RejectedSamplesNS"}},
		{"Delta", Delta{ID: "i", Interpretation: "n", SamplesNS: []int64{1}, Summary: summary},
			[]string{"ID", "Interpretation", "SamplesNS", "Summary"}},
		{"Capture", Capture{StartedUTC: "s", RecordedUTC: "r", Platform: "p", Method: "m", Condition: "c", ColdEvidence: "e", Warmups: 1, Units: "u", FixtureFiles: map[string]string{"a": "b"}, FixtureDir: "d", Environment: map[string]string{"a": "b"}, Metadata: map[string]string{"a": "b"}, Hashes: map[string]string{"a": "b"}, Configs: []Config{{}}, Records: []Record{{}}, PairedDeltas: []Delta{{}}, Gaps: []string{"g"}},
			[]string{"StartedUTC", "RecordedUTC", "Platform", "Method", "Condition", "ColdEvidence", "Warmups", "Units", "FixtureFiles", "FixtureDir", "Environment", "Metadata", "Hashes", "Configs", "Records", "PairedDeltas", "Gaps"}},
		{"Summary", Summary{Count: 1, MeanNS: 1, MedianNS: 1, P95NS: 1, MinNS: 1, MaxNS: 1, StddevNS: 1},
			[]string{"Count", "MeanNS", "MedianNS", "P95NS", "MinNS", "MaxNS", "StddevNS"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonKeys(t, tc.value); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("keys = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckBinary(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent")
	directory := filepath.Join(dir, "dir")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runnable := filepath.Join(dir, "runnable")
	if err := os.WriteFile(runnable, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing", func(t *testing.T) {
		err := CheckBinary(missing)
		if err == nil || !strings.HasPrefix(err.Error(), "measured binary unavailable (fail-open output is not evidence): ") || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("directory", func(t *testing.T) {
		err := CheckBinary(directory)
		if want := "measured binary not executable regular file: " + directory; err == nil || err.Error() != want {
			t.Fatalf("got %v, want %q", err, want)
		}
	})
	t.Run("non-executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no executable permission bit")
		}
		err := CheckBinary(plain)
		if want := "measured binary not executable regular file: " + plain; err == nil || err.Error() != want {
			t.Fatalf("got %v, want %q", err, want)
		}
	})
	t.Run("executable", func(t *testing.T) {
		if err := CheckBinary(runnable); err != nil {
			t.Fatalf("got %v", err)
		}
	})
}

func TestMetadataHashes(t *testing.T) {
	root := t.TempDir()
	hooksDir := filepath.Join(root, "hooks")
	if err := os.Mkdir(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "probe.txt")
	body := []byte("hash me\n")
	if err := os.WriteFile(source, body, 0o644); err != nil {
		t.Fatal(err)
	}
	absent := filepath.Join(root, "absent.txt")
	binary := filepath.Join(root, "no-binary")

	_, hashes := Metadata(root, hooksDir, binary, []string{source, absent})

	sum := sha256.Sum256(body)
	if got, want := hashes[source], hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("hash of %s = %q, want %q", source, got, want)
	}
	if got := hashes[absent]; !strings.HasPrefix(got, "unavailable: ") {
		t.Fatalf("hash of a missing source = %q, want an unavailable: value", got)
	}
	fixed := []string{binary, filepath.Join(hooksDir, "run-hook.cmd"), filepath.Join(hooksDir, "hooks.claude.json"), filepath.Join(hooksDir, "hooks.codex.json"), filepath.Join(hooksDir, "hooks.copilot.json"), filepath.Join(root, "hooks.json")}
	for _, path := range fixed {
		if got := hashes[path]; !strings.HasPrefix(got, "unavailable: ") {
			t.Fatalf("fixed input %s = %q, want an unavailable: value", path, got)
		}
	}
	if len(hashes) != len(fixed)+2 {
		t.Fatalf("got %d hashes, want %d: %v", len(hashes), len(fixed)+2, hashes)
	}
}
