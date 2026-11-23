package flow

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// stateHeaderRow names the nine columns. A new file starts with it and carries
// no version line, since each row is read by its own content and nothing
// branches on a version. A file that starts with an older "# flow-state 1"
// line stays readable and appendable, and that line is never rewritten.
const stateHeaderRow = "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext"
const stateHeader = stateHeaderRow + "\n"

// appendState adds a row to the state.tsv in folder, creating both when
// missing. Its errors name logical, the folder as messages print it, since
// folder may be a local-store directory the caller never named.
func appendState(folder, logical, session string, values [7]string, now func() time.Time) error {
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return fmt.Errorf("flow: cannot create folder %s: %w", logical, err)
	}
	file := filepath.Join(folder, "state.tsv")
	if err := initializeState(file); err != nil {
		return fmt.Errorf("flow: cannot initialize %s/state.tsv: %w", logical, err)
	}
	row := []string{now().UTC().Format("2006-01-02T15:04:05Z"), sessionCell(session)}
	for _, value := range values {
		row = append(row, cleanCell(value))
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("flow: cannot append %s/state.tsv: %w", logical, err)
	}
	writeErr := writeStateBytes(f, []byte(strings.Join(row, "\t")+"\n"))
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("flow: cannot append %s/state.tsv: %w", logical, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("flow: cannot close %s/state.tsv: %w", logical, closeErr)
	}
	return nil
}

func initializeState(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	// Prepare a complete header for hardlink publication, retaining its bytes for
	// exclusive-create initialization on filesystems that cannot create links.
	temporary := filepath.Join(filepath.Dir(path), ".flow-state-"+rand.Text())
	f, err := os.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	header := []byte(stateHeader)
	writeErr := writeStateBytes(f, header)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return publishStateUsing(f.Name(), path, header, os.Link)
}

func publishState(temporary, path string) error {
	return publishStateUsing(temporary, path, []byte(stateHeader), os.Link)
}

func publishStateUsing(temporary, path string, header []byte, link func(string, string) error) error {
	err := link(temporary, path)
	if err == nil || os.IsExist(err) {
		return nil
	}
	source, err := os.Lstat(temporary)
	if err != nil {
		return err
	}
	if !source.Mode().IsRegular() {
		return &os.PathError{Op: "publish", Path: temporary, Err: os.ErrInvalid}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	writeErr := writeStateBytes(f, header)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func writeStateBytes(w io.Writer, body []byte) error {
	n, err := w.Write(body)
	if err == nil && n != len(body) {
		return io.ErrShortWrite
	}
	return err
}
