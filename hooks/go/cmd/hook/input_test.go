package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
)

type hookInputProviderCase struct {
	provider      string
	event         string
	payload       string
	wantStdout    string
	wantErrorExit int
}

var hookInputProviderCases = []hookInputProviderCase{
	{
		provider:      "claude-code",
		event:         "PreToolUse",
		payload:       `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"printf ok"},"untrusted":"DO_NOT_DISCLOSE"}`,
		wantErrorExit: hookspec.ExitUsage,
	},
	{
		provider:      "copilot",
		event:         "preToolUse",
		payload:       `{"toolName":"bash","toolArgs":{"cmd":"printf ok"},"untrusted":"DO_NOT_DISCLOSE"}`,
		wantErrorExit: hookspec.ExitOK,
	},
	{
		provider:      "codex",
		event:         "PreToolUse",
		payload:       `{"hook_event_name":"PreToolUse","cwd":"/tmp","tool_name":"exec_command","tool_input":{"command":"printf ok"},"untrusted":"DO_NOT_DISCLOSE"}`,
		wantStdout:    "{}\n",
		wantErrorExit: hookspec.ExitOK,
	},
}

func TestHookInputLimit(t *testing.T) {
	const payloadLimit = 64 << 20
	for _, testCase := range hookInputProviderCases {
		t.Run(testCase.provider+"/exact limit", func(t *testing.T) {
			got := invokeHookInputCase(t, testCase, paddedHookInput(t, testCase.payload, payloadLimit))
			if got.exit != hookspec.ExitOK || got.stdout != testCase.wantStdout || got.stderr != "" {
				t.Fatalf("response = exit %d stdout %q stderr %q, want exit 0 stdout %q stderr empty", got.exit, got.stdout, got.stderr, testCase.wantStdout)
			}
			if got.normalizeCalls != 1 || got.handlerCalls != 1 {
				t.Fatalf("normalize/handler calls = %d/%d, want 1/1", got.normalizeCalls, got.handlerCalls)
			}
		})

		t.Run(testCase.provider+"/limit plus one", func(t *testing.T) {
			got := invokeHookInputCase(t, testCase, paddedHookInput(t, testCase.payload, payloadLimit+1))
			wantStderr := "unable to read the hook payload: hook payload exceeds 64 MiB limit\n"
			assertRejectedHookInput(t, testCase, got, wantStderr)
		})

		t.Run(testCase.provider+"/malformed", func(t *testing.T) {
			got := invokeHookInputCase(t, testCase, strings.NewReader(`{"DO_NOT_DISCLOSE":`))
			wantStderr := testCase.provider + ": malformed " + testCase.event + " payload: payload is not valid JSON: unexpected EOF\n"
			if got.exit != testCase.wantErrorExit || got.stdout != testCase.wantStdout || got.stderr != wantStderr {
				t.Fatalf("response = exit %d stdout %q stderr %q, want exit %d stdout %q stderr %q", got.exit, got.stdout, got.stderr, testCase.wantErrorExit, testCase.wantStdout, wantStderr)
			}
			if got.normalizeCalls != 1 || got.handlerCalls != 0 {
				t.Fatalf("normalize/handler calls = %d/%d, want 1/0", got.normalizeCalls, got.handlerCalls)
			}
			if strings.Contains(got.stderr, "DO_NOT_DISCLOSE") {
				t.Fatalf("malformed-input diagnostic disclosed rejected content: %q", got.stderr)
			}
		})

		t.Run(testCase.provider+"/partial reader error", func(t *testing.T) {
			readErr := errors.New("synthetic read failure")
			reader := &errorAfterHookInputReader{data: []byte(testCase.payload), err: readErr}
			got := invokeHookInputCase(t, testCase, reader)
			wantStderr := "unable to read the hook payload: " + readErr.Error() + "\n"
			assertRejectedHookInput(t, testCase, got, wantStderr)
		})
	}
}

type hookInputResult struct {
	exit                         int
	stdout, stderr               string
	normalizeCalls, handlerCalls int
}

func invokeHookInputCase(t *testing.T, testCase hookInputProviderCase, reader io.Reader) hookInputResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	normalizeCalls := 0
	handlerCalls := 0
	deps := productionHookDependencies()
	productionNormalize := deps.normalize
	deps.normalize = func(surface, event string, payload []byte) (hookspec.Event, *provider.Rendered) {
		normalizeCalls++
		return productionNormalize(surface, event, payload)
	}
	deps.lookup = func(string) (hookspec.Handler, bool) {
		return func(hookspec.Request) hookspec.Outcome {
			handlerCalls++
			return contract.Pass()
		}, true
	}

	exit := runWithDependencies(
		deps,
		[]string{testCase.provider, "command-guardian", "--mode", "block"},
		reader,
		&stdout,
		&stderr,
	)
	return hookInputResult{
		exit:           exit,
		stdout:         stdout.String(),
		stderr:         stderr.String(),
		normalizeCalls: normalizeCalls,
		handlerCalls:   handlerCalls,
	}
}

func assertRejectedHookInput(t *testing.T, testCase hookInputProviderCase, got hookInputResult, wantStderr string) {
	t.Helper()
	if got.exit != testCase.wantErrorExit || got.stdout != testCase.wantStdout || got.stderr != wantStderr {
		t.Fatalf("response = exit %d stdout %q stderr %q, want exit %d stdout %q stderr %q", got.exit, got.stdout, got.stderr, testCase.wantErrorExit, testCase.wantStdout, wantStderr)
	}
	if got.normalizeCalls != 0 || got.handlerCalls != 0 {
		t.Fatalf("normalize/handler calls = %d/%d, want 0/0", got.normalizeCalls, got.handlerCalls)
	}
	if strings.Contains(got.stderr, "DO_NOT_DISCLOSE") {
		t.Fatalf("read diagnostic disclosed rejected content: %q", got.stderr)
	}
}

func paddedHookInput(t testing.TB, payload string, size int64) io.Reader {
	t.Helper()
	if int64(len(payload)) > size {
		t.Fatalf("payload length %d exceeds requested reader size %d", len(payload), size)
	}
	return io.MultiReader(
		strings.NewReader(payload),
		io.LimitReader(repeatedHookInputByte(' '), size-int64(len(payload))),
	)
}

type repeatedHookInputByte byte

func (reader repeatedHookInputByte) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = byte(reader)
	}
	return len(buffer), nil
}

type errorAfterHookInputReader struct {
	data []byte
	err  error
}

func (reader *errorAfterHookInputReader) Read(buffer []byte) (int, error) {
	if len(reader.data) == 0 {
		return 0, reader.err
	}
	count := copy(buffer, reader.data)
	reader.data = reader.data[count:]
	if len(reader.data) == 0 {
		return count, reader.err
	}
	return count, nil
}
