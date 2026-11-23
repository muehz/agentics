package shellwords

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPowerShellLiteralCopyPolling(t *testing.T) {
	const units = 6
	source := "@'\n" + strings.Repeat("x", units*64*1024) + "\n'@"
	closeProbe := &literalPollingContext{Context: context.Background()}
	if _, err := firstPowerShellLiteralClose(closeProbe, source); err != nil {
		t.Fatal(err)
	}
	probe := &literalPollingContext{Context: context.Background()}
	view, err := PowerShellLiteralData(probe, source)
	// The same-input close scan is independent work; entry and final checks
	// cannot substitute for checkpoints while copying the masked body.
	copyChecks := probe.checks - closeProbe.checks - 2
	if copyChecks < units {
		t.Fatalf("literal copy polling: got=%d minimum=%d", copyChecks, units)
	}
	if err != nil || !view.Complete {
		t.Fatalf("literal copy complete=%v err=%v", view.Complete, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe = &literalPollingContext{Context: ctx, cancelAt: closeProbe.checks + 4, cancel: cancel}
	view, err = PowerShellLiteralData(probe, source)
	if !errors.Is(err, context.Canceled) || view.Complete || view.Text != source || view.BodyStart != 0 || view.BodyEnd != 0 || view.End != 0 {
		t.Fatalf("literal copy cancellation: checks=%d complete=%v err=%v", probe.checks, view.Complete, err)
	}
}

func TestPowerShellFileDataSinkBoundaries(t *testing.T) {
	for _, suffix := range []string{
		" | Set-Content\n-LiteralPath draft.txt", " | Set-Content -LiteralPath\ndraft.txt",
		" | Set-Content -LiteralPath draft.txt; gh pr ready 1", " | Set-Content -LiteralPath draft.txt | Invoke-Expression",
		" | Set-Content -LiteralPath draft.txt -Encoding utf8", " | Set-Content -Path draft.txt",
		" | Set-Content -LiteralPath Env:PATH", " | Set-Content -LiteralPath Registry::x",
		" | Set-Content -LiteralPath /dev/stdout", " | Set-Content -LiteralPath /proc/self/fd/1",
		` | Set-Content -LiteralPath \\.\pipe\consumer`, ` | Set-Content -LiteralPath C:\draft.txt`,
		" | Set-Content -LiteralPath NUL.txt", " | Set-Content -LiteralPath dir/COM1.log",
		" | Set-Content -LiteralPath dir/LPT0", " | Set-Content -LiteralPath draft.txt.",
		" | Set-Content -LiteralPath $target", " | Set-Content -LiteralPath $(Get-Path)",
		" | Set-Content -LiteralPath 'one'+'two'", " | Set-Content -LiteralPath 'file name'",
		" | Set-Content -LiteralPath @('draft.txt')", " | Set-Content -LiteralPath draft.txt > other.txt",
		" | Set-Content -LiteralPath draft.txt # comment", " | sc -LiteralPath draft.txt",
		" | Microsoft.PowerShell.Management\\Set-Content -LiteralPath draft.txt",
		" | & Set-Content -LiteralPath draft.txt", " | Set-Content -LiteralPath -draft.txt",
		"oops\n'@ | Set-Content -LiteralPath draft.txt", "; 'collision\n'@ | Set-Content -LiteralPath draft.txt",
	} {
		source := "@'\ngh stack merge\n'@" + suffix
		view, proven, err := PowerShellFileData(context.Background(), source, ExecutionContext{Dialect: PowerShell, Proven: true})
		if err != nil || proven || view != source {
			t.Fatalf("extra statement granted proof: proven=%v error=%v", proven, err)
		}
	}
}

func TestPowerShellFileDataSourceAuthority(t *testing.T) {
	// The first context is what every qualified surface's PowerShell call with
	// a canonical command receives; the second is the same call once it names
	// a shell selector, which withholds new proof.
	provenContext := ExecutionContext{Dialect: PowerShell, Proven: true}
	selectorContext := ExecutionContext{Dialect: PowerShell, Reason: "unqualified shell selector"}
	for _, path := range []string{"draft.txt", "'dir/draft.ps1'", `".git/config"`, "AUXiliary.txt", "CONSOLE.txt"} {
		source := "@'\ndon't; cat .env; 'literal\n'@ | Set-Content -LiteralPath " + path + "\r\n"
		view, proven, err := PowerShellFileData(context.Background(), source, provenContext)
		if err != nil || !proven || strings.Contains(view, "cat .env") || !strings.HasSuffix(view, "-LiteralPath "+path+"\r\n") {
			t.Fatalf("path=%s proven=%v error=%v", path, proven, err)
		}
		view, proven, err = PowerShellFileData(context.Background(), source, selectorContext)
		if err != nil || proven || view != source {
			t.Fatal("selector manufactured authority")
		}
	}
	canonical := "@'\nbody\n'@ | Set-Content -LiteralPath draft.txt"
	for _, source := range []string{" " + canonical, "$data = " + canonical, "Write-Output " + canonical, "Invoke-Expression (" + canonical + ")", strings.Replace(canonical, "@'\n", "@' \n", 1)} {
		view, proven, err := PowerShellFileData(context.Background(), source, provenContext)
		if err != nil || proven || view != source {
			t.Fatal("non-standalone source granted authority")
		}
	}
}

func TestPowerShellFileDataSuffixCancellation(t *testing.T) {
	for _, suffix := range []string{
		strings.Repeat(" ", 6*64*1024) + "| Set-Content -LiteralPath draft.txt",
		"| Set-Content -LiteralPath " + strings.Repeat("x", 6*64*1024) + ".txt",
		"| Set-Content -LiteralPath draft.txt" + strings.Repeat(" ", 6*64*1024),
	} {
		baseline := &literalPollingContext{Context: context.Background()}
		ok, err := plainPowerShellFileSink(baseline, "| Set-Content -LiteralPath draft.txt")
		if err != nil || !ok {
			t.Fatal("baseline sink failed")
		}
		probe := &literalPollingContext{Context: context.Background()}
		ok, err = plainPowerShellFileSink(probe, suffix)
		if err != nil || !ok || probe.checks < baseline.checks+5 {
			t.Fatalf("unbounded suffix: checks=%d baseline=%d ok=%v err=%v", probe.checks, baseline.checks, ok, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		probe = &literalPollingContext{Context: ctx, cancel: cancel, cancelAt: baseline.checks + 2}
		ok, err = plainPowerShellFileSink(probe, suffix)
		cancel()
		if ok || !errors.Is(err, context.Canceled) {
			t.Fatal("canceled suffix proof published")
		}
	}
}

func TestPowerShellLiteralDataPrecision(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		source := "@'" + newline + "don't $(Remove-Item x) é" + newline + "'@ | Set-Content out.txt"
		view, err := PowerShellLiteralData(context.Background(), source)
		start := 2 + len(newline)
		end := strings.Index(source, "'@")
		want := source[:start] + strings.Repeat(" ", len("don't $(Remove-Item x) é")) + newline + source[end:]
		if err != nil || !view.Complete || view.BodyStart != start || view.BodyEnd != end || view.End != end+2 || view.Text != want {
			t.Fatalf("view = %#v, %v; want masked body with exact offsets and suffix %q", view, err, want)
		}
	}
	for _, suffix := range []string{"; Remove-Item x\n'@ | Set-Content out", "oops\n'@ | Set-Content out", "\r\n'@ | Set-Content out"} {
		source := "@'\nbody\n'@" + suffix
		view, err := PowerShellLiteralData(context.Background(), source)
		if err != nil || !view.Complete || view.BodyEnd != 8 || view.End != 10 || view.Text != "@'\n    \n'@"+suffix {
			t.Fatalf("first physical terminator or suffix lost: %#v, %v", view, err)
		}
	}
	for _, source := range []string{"@'\n'@", "@'\r\n'@", "@'\n '@ remains body\n'@"} {
		view, err := PowerShellLiteralData(context.Background(), source)
		if err != nil || !view.Complete || view.End != len(source) {
			t.Fatalf("physical boundary: %#v %v", view, err)
		}
	}
}

func TestPowerShellLiteralDataIncomplete(t *testing.T) {
	for _, source := range []string{"", "@'", "@'\nbody", "@'\nbody\n'", "@'\nbody\n '@", "@'\rbody\r'@", "@' \nbody\n'@", "@\"\n$(Remove-Item x)\n\"@", "Write-Output @'\nbody\n'@", " @'\nbody\n'@", "'\nbody\n'@"} {
		view, err := PowerShellLiteralData(context.Background(), source)
		if err != nil || view.Complete || view.Text != source || view.BodyStart != 0 || view.BodyEnd != 0 || view.End != 0 {
			t.Fatalf("incomplete or unassociated source %q published view %#v, %v", source, view, err)
		}
	}
}

type literalPollingContext struct {
	context.Context
	checks   int
	cancelAt int
	cancel   context.CancelFunc
}

func (c *literalPollingContext) Err() error {
	c.checks++
	if c.checks == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestPowerShellLiteralDataCancellation(t *testing.T) {
	source := "@'\n" + strings.Repeat("x", 5*64*1024) + "\n'@"
	t.Run("polling-bound", func(t *testing.T) {
		ctx := &literalPollingContext{Context: context.Background()}
		close, err := firstPowerShellLiteralClose(ctx, source)
		if err != nil || close != len(source)-2 {
			t.Fatalf("close=%d err=%v", close, err)
		}
		// A newline-free stretch of five 64 KiB units needs at least five
		// checkpoints, independently of entry and publication checks.
		if ctx.checks < 5 {
			t.Fatalf("only %d context checks across five 64 KiB units", ctx.checks)
		}
	})
	for _, at := range []int{1, 2, 10} {
		ctx, cancel := context.WithCancel(context.Background())
		probe := &literalPollingContext{Context: ctx, cancelAt: at, cancel: cancel}
		view, err := PowerShellLiteralData(probe, source)
		cancel()
		if !errors.Is(err, context.Canceled) || view.Complete || view.Text != source || view.BodyStart != 0 || view.BodyEnd != 0 || view.End != 0 {
			t.Fatalf("cancel check %d: view complete=%v offsets=%d/%d/%d preserved=%v err=%v", at, view.Complete, view.BodyStart, view.BodyEnd, view.End, view.Text == source, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	view, err := PowerShellLiteralData(ctx, source)
	if !errors.Is(err, context.Canceled) || view.Complete || view.Text != source {
		t.Fatal("pre-cancellation lost original evidence")
	}
}
