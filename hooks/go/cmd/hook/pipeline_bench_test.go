package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
)

// BenchmarkHookPipeline excludes fixture setup and process startup while
// exercising the same provider-to-output paths as the latency baseline.
func BenchmarkHookPipeline(b *testing.B) {
	table := loadCorpusTable(b)
	entries := make(map[string]corpusEntry, len(table.Entries))
	for _, entry := range table.Entries {
		entries[entry.Name] = entry
	}
	for _, probe := range latencyProbes {
		b.Run(strings.TrimPrefix(probe.name, "Benchmark"), func(b *testing.B) {
			argv, stdin := latencyProbeInput(b, table, entries, probe)
			benchmarkInvoke(b, hookcost.Probe{Name: probe.name, Argv: argv, Stdin: stdin})
		})
	}
}
