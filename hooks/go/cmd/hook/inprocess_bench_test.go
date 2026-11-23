package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// BenchmarkHookCostInProcess amortizes timer granularity across calibrated
// iterations of the same inputs the launch collector uses.
func BenchmarkHookCostInProcess(b *testing.B) {
	checkout, err := filepath.Abs(hosttest.Checkout(b))
	if err != nil {
		b.Fatal(err)
	}
	target, err := hookcost.ResolveTarget(checkout, os.Getenv)
	if errors.Is(err, hookcost.ErrOffContract) {
		b.Skipf("no packaged hook binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err != nil {
		b.Fatal(err)
	}
	hooksDir := target.HooksDir
	registrations, err := hosttest.Registrations(hooksDir)
	if err != nil {
		b.Fatal(err)
	}
	fixture, _ := hookcost.Prepare(b, target)
	for _, provider := range surface.All() {
		for _, tool := range []string{"Bash", "Read"} {
			_, configs := hookcost.Selected(b, registrations, provider, tool, runtime.GOOS)
			for _, config := range configs {
				payload := hookCostStdin(provider, tool, fixture)
				var stdout, stderr bytes.Buffer
				if code := run(config.Argv, strings.NewReader(payload), &stdout, &stderr); code != 0 {
					b.Fatalf("reference input rejected: exit=%d stderr=%q", code, stderr.String())
				}
				b.Run(string(provider)+"/"+tool+"/"+config.Hook, func(b *testing.B) {
					b.Logf("argv=%q stdin=%s", config.Argv, payload)
					benchmarkInvoke(b, hookcost.Probe{Name: config.Hook, Argv: config.Argv, Stdin: payload})
				})
			}
		}
	}
}
