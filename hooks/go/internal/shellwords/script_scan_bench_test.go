package shellwords

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// benignScriptBody returns at least n bytes of varied benign shell: assignments, tests, loops, substitutions, a
// function with an unshadowed name, here-documents and comments, none of which any guard refuses.
func benignScriptBody(n int) string {
	block := []string{
		"#!/usr/bin/env bash",
		"set -eu",
		"# note: nothing here changes the environment",
		"count=0",
		"for item in alpha beta gamma; do",
		"  count=$((count + 1))",
		"  echo \"item $item of $count\"",
		"done",
		"if [ -n \"${name:-}\" ] && [ \"$count\" -gt 2 ]; then",
		"  printf '%s\\n' \"$name\" | sed -e 's/a/b/' | grep -c b",
		"fi",
		"report() {",
		"  local label=$1",
		"  echo \"report: $label\"",
		"}",
		"report summary",
		"value=$(echo hello | tr a-z A-Z)",
		"if [[ \"$value\" == HELLO ]]; then echo ok; fi",
		"cat <<'EOF'",
		"usage: tool [--flag] file",
		"EOF",
		"git status --short",
		"ls -la ./docs | head -n 3",
		"echo done # trailing comment",
		"",
	}
	var b strings.Builder
	for b.Len() < n {
		for _, l := range block {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func benchParse(b *testing.B, text string) {
	files := fakeReader{"/ws/s.sh": {Text: text, Bytes: len(text)}}
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := ParseScripts(context.Background(), "bash /ws/s.sh", POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
		if len(res.Invocations) == 0 {
			b.Fatal("no invocations")
		}
	}
}

func BenchmarkParseScripts64KiB(b *testing.B) { benchParse(b, benignScriptBody(64<<10)) }
func BenchmarkParseScripts16KiB(b *testing.B) { benchParse(b, benignScriptBody(16<<10)) }

func BenchmarkParseScriptsFlow(b *testing.B) {
	data, err := os.ReadFile(filepath.Join(hosttest.Checkout(b), "hooks", "go", "internal", "shellwords", "testdata", "flow.sh"))
	if err != nil {
		b.Skip(err)
	}
	benchParse(b, strings.ReplaceAll(string(data), "\r", ""))
}

func BenchmarkScanTextConstructs64KiB(b *testing.B) {
	text := benignScriptBody(64 << 10)
	b.SetBytes(int64(len(text)))
	for i := 0; i < b.N; i++ {
		scanTextConstructs(text)
	}
}
