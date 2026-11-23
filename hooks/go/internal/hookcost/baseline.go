package hookcost

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadBaseline parses the Benchmark lines of a latency baseline file in go
// test -bench format: name, iterations, nanoseconds, "ns/op". Other lines,
// comments among them, are skipped.
func ReadBaseline(path string) (map[string]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no baseline at %s — run with -latency-update: %w", path, err)
	}
	defer f.Close()
	baseline := map[string]int64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") || fields[3] != "ns/op" {
			continue
		}
		ns, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: bad ns/op on line %q: %w", path, scanner.Text(), err)
		}
		baseline[fields[0]] = ns
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return baseline, nil
}

// WriteBaseline writes header verbatim, so it carries its own comment
// markers and newline, then one go test -bench line per name in order.
func WriteBaseline(path, header string, names []string, measured map[string]int64) error {
	var buf bytes.Buffer
	buf.WriteString(header)
	for _, name := range names {
		fmt.Fprintf(&buf, "%s\t1\t%d ns/op\n", name, measured[name])
	}
	if err := os.WriteFile(filepath.Clean(path), buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
