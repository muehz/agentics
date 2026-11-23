package flow

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedStateClock() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 999, time.FixedZone("offset", -6*60*60))
}

func TestStateWritesUTF8WithoutBOMAndBareLF(t *testing.T) {
	t.Parallel()
	folder := filepath.Join(t.TempDir(), "f")
	values := [7]string{"intent", "create", "intent.md", "ok", "why", "intent.md", "spec go"}
	if err := appendState(folder, folder, "s1", values, fixedStateClock); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(folder, "state.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	want := headerRow + "\n2026-01-02T09:04:05Z\ts1\tintent\tcreate\tintent.md\tok\twhy\tintent.md\tspec go\n"
	if string(raw) != want {
		t.Fatalf("bytes = %q, want %q", raw, want)
	}
	if bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) || bytes.Contains(raw, []byte("\r")) {
		t.Fatal("state contains BOM or CR")
	}
	values[4] = "\t café 世界\r\n"
	if err := appendState(folder, folder, "", values, fixedStateClock); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(folder, "state.tsv"))
	if !bytes.HasSuffix(raw, []byte("\t-\tintent\tcreate\tintent.md\tok\tcafé 世界\tintent.md\tspec go\n")) {
		t.Fatalf("UTF-8/session bytes = %q", raw)
	}
}

func TestStateExistingFileIsNotRewritten(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	file := filepath.Join(folder, "state.tsv")
	const existing = "existing bytes without newline"
	if err := os.WriteFile(file, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendState(folder, folder, "s1", [7]string{"intent", "create", "intent.md", "ok", "why", "none", "spec go"}, fixedStateClock); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	if !bytes.HasPrefix(raw, []byte(existing+"2026-01-02T09:04:05Z")) || bytes.Contains(raw, []byte(headerRow)) {
		t.Fatalf("existing bytes rewritten: %q", raw)
	}
}

func TestStateCreationPreservesHostPermissions(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	control := filepath.Join(folder, "control")
	f, err := os.OpenFile(control, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := appendState(folder, folder, "s1", [7]string{}, fixedStateClock); err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(filepath.Join(folder, "state.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("state permissions=%o, ordinary create=%o", got.Mode().Perm(), want.Mode().Perm())
	}
}

func TestStateLosingPublicationKeepsTheWinner(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	file := filepath.Join(folder, "state.tsv")
	temporary := filepath.Join(folder, "temporary")
	winner := headerRow + "\nwinner row\n"
	if err := os.WriteFile(file, []byte(winner), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("losing header"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishState(temporary, file); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != winner {
		t.Fatalf("winner replaced: bytes=%q error=%v", got, err)
	}
	if err := publishState(filepath.Join(folder, "missing"), filepath.Join(folder, "absent")); err == nil {
		t.Fatal("non-existence link failure was swallowed")
	}
}

func checkConcurrentState(t *testing.T, folder string, wantRows, distinct int) {
	t.Helper()
	lines := readLines(t, filepath.Join(folder, "state.tsv"))
	if len(lines) != 1+wantRows || lines[0] != headerRow {
		t.Fatalf("want one header row and %d rows, got %d lines: %q", wantRows, len(lines), lines)
	}
	seen := make(map[string]bool)
	for _, line := range lines[1:] {
		c := cells(line)
		if len(c) != 9 {
			t.Fatalf("row torn into %d cells: %q", len(c), line)
		}
		seen[c[4]] = true
	}
	if len(seen) != distinct {
		t.Fatalf("distinct targets = %d, want %d", len(seen), distinct)
	}
	entries, err := os.ReadDir(folder)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.tsv" {
		t.Fatalf("initialization left temporary names: entries=%v error=%v", entries, err)
	}
	t.Logf("preserved %d complete rows and %d distinct targets with one header", wantRows, distinct)
}

func TestStateConcurrentWritersKeepEveryRow(t *testing.T) {
	t.Parallel()
	for _, seeded := range []bool{false, true} {
		t.Run(fmt.Sprint(seeded), func(t *testing.T) {
			t.Parallel()
			folder := filepath.Join(t.TempDir(), "f")
			want := 64
			if seeded {
				appendTestState(t, filepath.Dir(folder), "f", "intent", "create", "seed", "ok", "seed", "none", "spec go")
				want++
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range 64 {
				wg.Go(func() {
					<-start
					if err := appendState(folder, folder, "s1", [7]string{"execute", "enter", "task-" + strconv.Itoa(i), "ok", "parallel row", "none", "execute go"}, fixedStateClock); err != nil {
						t.Error(err)
					}
				})
			}
			close(start)
			wg.Wait()
			checkConcurrentState(t, folder, want, want)
		})
	}
}

func TestStateWriterProcessHelper(t *testing.T) {
	t.Parallel()
	folder := os.Getenv("AGX_FLOW_STATE_CHILD")
	if folder == "" {
		return
	}
	id := os.Getenv("AGX_FLOW_STATE_ID")
	for i := range 12 {
		if err := appendState(folder, folder, "s1", [7]string{"execute", "enter", "task-" + id + "-" + strconv.Itoa(i), "ok", "parallel row", "none", "execute go"}, fixedStateClock); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStateConcurrentProcessesKeepEveryRow(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(t.TempDir(), "f")
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			cmd := exec.Command(executable, "-test.run=^TestStateWriterProcessHelper$")
			cmd.Env = baseEnv("AGX_FLOW_STATE_CHILD="+folder, "AGX_FLOW_STATE_ID="+strconv.Itoa(i))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("child: %v\n%s", err, out)
			}
		})
	}
	wg.Wait()
	checkConcurrentState(t, folder, 72, 72)
}

type rowWriteProbe struct {
	calls int
	data  []byte
	short bool
	err   error
}

func (p *rowWriteProbe) Write(b []byte) (int, error) {
	p.calls++
	p.data = append(p.data, b...)
	if p.short {
		return len(b) - 1, nil
	}
	return len(b), p.err
}

func TestStateRowIsOneWrite(t *testing.T) {
	t.Parallel()
	p := new(rowWriteProbe)
	if err := writeStateBytes(p, []byte("a\tb\n")); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || string(p.data) != "a\tb\n" {
		t.Fatalf("writes=%d data=%q", p.calls, p.data)
	}
	if err := writeStateBytes(&rowWriteProbe{short: true}, []byte("row\n")); err != io.ErrShortWrite {
		t.Fatalf("short write = %v", err)
	}
	wantErr := fmt.Errorf("sentinel write error")
	if err := writeStateBytes(&rowWriteProbe{err: wantErr}, []byte("row\n")); err != wantErr {
		t.Fatalf("write error = %v", err)
	}
}

func TestStateWriterReportsFilesystemFailures(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeFile(t, repo, "blocked", "not a directory")
	for _, folder := range []string{filepath.Join(repo, "blocked", "f"), filepath.Join(repo, "directory-state")} {
		want := "flow: cannot create folder "
		if strings.HasSuffix(folder, "directory-state") {
			want = "flow: cannot append "
			if err := os.MkdirAll(filepath.Join(folder, "state.tsv"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		// The message names the logical folder, not the directory that holds it.
		const logical = "docs/specs/2026-11/logical"
		if strings.HasSuffix(folder, "directory-state") {
			want += logical + "/state.tsv: "
		} else {
			want += logical + ": "
		}
		err := appendState(folder, logical, "s1", [7]string{}, fixedStateClock)
		if err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Fatalf("filesystem failure = %v, want prefix %q", err, want)
		}
	}
}
