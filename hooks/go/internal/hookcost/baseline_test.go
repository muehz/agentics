package hookcost

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func TestReadBaselineCommitted(t *testing.T) {
	baseline, err := ReadBaseline(filepath.Join(hosttest.Checkout(t), "hooks", "go", "bench-baseline.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name, ns := range baseline {
		if ns <= 0 {
			t.Fatalf("%s: %d ns/op", name, ns)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{"BenchmarkCommandGuardian", "BenchmarkCommandGuardianNonASCII", "BenchmarkCommandGuardianSevenSegments", "BenchmarkReleaseGuardian", "BenchmarkScriptCall", "BenchmarkScriptCallGrouped", "BenchmarkSecretsGuardian", "BenchmarkSecretsGuardianWrite64KB"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestReadBaselineRows(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	good := write("good.txt", strings.Join([]string{
		"# Benchmark comment\t1\t5 ns/op",
		"BenchmarkShort\t1\t5",
		"TestNotABenchmark\t1\t5 ns/op",
		"BenchmarkAllocs\t1\t5 B/op",
		"BenchmarkKept\t1\t42 ns/op",
		"BenchmarkKept-8   100   43 ns/op   0 B/op",
	}, "\n")+"\n")
	baseline, err := ReadBaseline(good)
	if err != nil || !reflect.DeepEqual(baseline, map[string]int64{"BenchmarkKept": 42, "BenchmarkKept-8": 43}) {
		t.Fatalf("got %v, %v", baseline, err)
	}

	bad := write("bad.txt", "BenchmarkBad\t1\tx ns/op\n")
	if _, err := ReadBaseline(bad); err == nil || err.Error() != bad+`: bad ns/op on line "BenchmarkBad\t1\tx ns/op": strconv.ParseInt: parsing "x": invalid syntax` {
		t.Fatalf("got %v", err)
	}

	missing := filepath.Join(dir, "absent.txt")
	if _, err := ReadBaseline(missing); err == nil || !strings.HasPrefix(err.Error(), "no baseline at "+missing+" — run with -latency-update: ") {
		t.Fatalf("got %v", err)
	}

	long := write("long.txt", strings.Repeat("x", 70*1024)+"\n")
	if _, err := ReadBaseline(long); err == nil || err.Error() != "reading "+long+": bufio.Scanner: token too long" {
		t.Fatalf("got %v", err)
	}
}

func TestWriteBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench-baseline.txt")
	header := "# hook latency baseline, recorded today\n"
	measured := map[string]int64{"BenchmarkA": 7, "BenchmarkB": 1234567}
	if err := WriteBaseline(path, header, []string{"BenchmarkB", "BenchmarkA"}, measured); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := header + "BenchmarkB\t1\t1234567 ns/op\nBenchmarkA\t1\t7 ns/op\n"; string(data) != want {
		t.Fatalf("wrote %q, want %q", data, want)
	}
	read, err := ReadBaseline(path)
	if err != nil || !reflect.DeepEqual(read, measured) {
		t.Fatalf("round trip = %v, %v", read, err)
	}
	unwritable := filepath.Join(t.TempDir(), "absent", "bench-baseline.txt")
	if err := WriteBaseline(unwritable, header, nil, nil); err == nil || !strings.HasPrefix(err.Error(), "writing "+unwritable+": ") {
		t.Fatalf("got %v", err)
	}
}
