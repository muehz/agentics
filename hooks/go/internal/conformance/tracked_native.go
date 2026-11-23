package conformance

import (
	"bufio"
	"bytes"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// The explicit git directory also accepts a linked worktree's .git file. A
// missing .git must fail here rather than discovering a surrounding checkout.
func candidateGit(root string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"--git-dir", filepath.Join(root, ".git"), "--work-tree", root,
		"--no-replace-objects", "--no-optional-locks", "-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = root
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	// Older Git ignores GIT_NO_LAZY_FETCH. An empty protocol allowlist also
	// prevents those versions from fetching a missing promisor object.
	cmd.Env = append(cmd.Env, "GIT_ALLOW_PROTOCOL=", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	return cmd
}

type indexBlob struct{ oid, path string }

// checkTrackedNative reads staged regular files, including mode 100644.
// Git's NUL-delimited index and raw object protocol preserve arbitrary paths
// and bypass checkout filters. A single batch process serves all objects.
func checkTrackedNative(root string) ([]Failure, error) {
	list := candidateGit(root, "ls-files", "--stage", "-z")
	var diagnostic bytes.Buffer
	list.Stderr = &diagnostic
	index, err := list.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(diagnostic.String()))
	}
	var blobs []indexBlob
	var requests strings.Builder
	for len(index) != 0 {
		record, rest, ok := bytes.Cut(index, []byte{0})
		meta, path, tab := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if !ok || !tab || len(path) == 0 || len(fields) != 3 {
			return nil, fmt.Errorf("invalid Git index record %q", record)
		}
		if fields[2] != "0" {
			return nil, fmt.Errorf("unmerged Git index entry %q (stage %s)", path, fields[2])
		}
		switch fields[0] {
		case "100644", "100755":
			blobs = append(blobs, indexBlob{fields[1], string(path)})
			requests.WriteString(fields[1] + "\n")
		case "120000", "160000": // symlink target text or a submodule commit
		default:
			return nil, fmt.Errorf("invalid Git index mode %q at %q", fields[0], path)
		}
		index = rest
	}
	if len(blobs) == 0 {
		return nil, nil
	}
	cmd := candidateGit(root, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(requests.String())
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git cat-file: %w", err)
	}
	reader := bufio.NewReader(pipe)
	var failures []Failure
	for _, blob := range blobs {
		var kind string
		kind, err = readIndexBlob(reader, blob.oid)
		if err != nil {
			err = fmt.Errorf("git cat-file for %q: %w", blob.path, err)
			break
		}
		if kind != "" {
			failures = append(failures, Failure{Rule: "tracked-native-executable", Path: blob.path,
				Message: "the Git index contains a " + kind + " executable; generate native artifacts outside Git"})
		}
	}
	pipe.Close()
	waitErr := cmd.Wait()
	if err != nil {
		return failures, err
	}
	if waitErr != nil {
		return failures, fmt.Errorf("git cat-file: %w", waitErr)
	}
	return failures, nil
}

func readIndexBlob(reader *bufio.Reader, oid string) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	fields := strings.Fields(line)
	if len(fields) != 3 || fields[0] != oid || fields[1] != "blob" {
		return "", fmt.Errorf("expected blob %s, got %q", oid, strings.TrimSpace(line))
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || size < 0 {
		return "", fmt.Errorf("invalid blob length %q", fields[2])
	}
	body := &io.LimitedReader{R: reader, N: size}
	var header [64]byte
	n, err := io.ReadFull(body, header[:min(size, int64(len(header)))])
	if err != nil {
		return "", err
	}
	kind, err := nativeExecutable(header[:n], body)
	if err != nil {
		return "", err
	}
	if _, err := io.CopyN(io.Discard, reader, body.N); err != nil {
		return "", err
	}
	if delimiter, err := reader.ReadByte(); err != nil || delimiter != '\n' {
		return "", fmt.Errorf("invalid blob terminator for %s", oid)
	}
	return kind, nil
}

// Only image headers need inspection. For PE the header offset can be deep
// into a file; stream up to it instead of buffering the blob in memory.
func nativeExecutable(header []byte, body *io.LimitedReader) (string, error) {
	if len(header) < 4 {
		return "", nil
	}
	magic := binary.BigEndian.Uint32(header)
	switch {
	case bytes.HasPrefix(header, []byte("MZ")) && len(header) == 64:
		offset := int64(binary.LittleEndian.Uint32(header[60:]))
		skip := offset - int64(len(header))
		if skip < 0 || skip+26 > body.N {
			return "", nil
		}
		if _, err := io.CopyN(io.Discard, body, skip); err != nil {
			return "", err
		}
		var pe [26]byte
		if _, err := io.ReadFull(body, pe[:]); err != nil {
			return "", err
		}
		optional := int64(binary.LittleEndian.Uint16(pe[20:]))
		format := binary.LittleEndian.Uint16(pe[24:])
		flags := binary.LittleEndian.Uint16(pe[22:])
		// PE32 and PE32+ have 96 and 112 bytes before their data directories.
		validOptional := format == 0x10b && optional >= 96 || format == 0x20b && optional >= 112
		if string(pe[:4]) == "PE\x00\x00" && binary.LittleEndian.Uint16(pe[4:]) != 0 && flags&0x0002 != 0 && validOptional && optional-2 <= body.N {
			return "PE", nil
		}
	case bytes.HasPrefix(header, []byte(elf.ELFMAG)) && len(header) >= 52:
		order := nativeByteOrder(header[5] == byte(elf.ELFDATA2LSB))
		if header[5] != byte(elf.ELFDATA2MSB) && header[5] != byte(elf.ELFDATA2LSB) || header[6] != 1 {
			return "", nil
		}
		if order.Uint16(header[18:]) == 0 || order.Uint32(header[20:]) != uint32(elf.EV_CURRENT) {
			return "", nil
		}
		header32 := header[4] == byte(elf.ELFCLASS32) && order.Uint16(header[40:]) == 52
		header64 := header[4] == byte(elf.ELFCLASS64) && len(header) == 64 && order.Uint16(header[52:]) == 64
		if !header32 && !header64 {
			return "", nil
		}
		if typ := elf.Type(order.Uint16(header[16:])); typ == elf.ET_EXEC || typ == elf.ET_DYN {
			return "ELF", nil
		}
	case thinMachExecutable(header):
		return "Mach-O", nil
	case magic == 0xcafebabe || magic == 0xbebafeca || magic == 0xcafebabf || magic == 0xbfbafeca:
		return fatMachExecutable(header, body, magic)
	}
	return "", nil
}

func nativeByteOrder(little bool) binary.ByteOrder {
	if little {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

func thinMachExecutable(header []byte) bool {
	if len(header) < 28 {
		return false
	}
	for _, order := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		magic := order.Uint32(header)
		if (magic == macho.Magic32 || magic == macho.Magic64 && len(header) >= 32) && order.Uint32(header[4:]) != 0 && order.Uint32(header[12:]) == uint32(macho.TypeExec) {
			return true
		}
	}
	return false
}

// A universal image's slices may be in any order. Spool only prospective
// universal images so slice offsets can be inspected with bounded memory.
// Java class files share the FAT_MAGIC prefix but lack Mach-O slices.
func fatMachExecutable(header []byte, body *io.LimitedReader, magic uint32) (string, error) {
	if len(header) < 8 {
		return "", nil
	}
	order := nativeByteOrder(magic == 0xbebafeca || magic == 0xbfbafeca)
	wide := magic == 0xcafebabf || magic == 0xbfbafeca
	stride := int64(20)
	if wide {
		stride = 32
	}
	count := int64(order.Uint32(header[4:]))
	size := int64(len(header)) + body.N
	tableEnd := 8 + count*stride
	if count == 0 || tableEnd > size {
		return "", nil
	}
	file, err := os.CreateTemp("", "hooktool-fat-image-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := io.CopyN(file, io.MultiReader(bytes.NewReader(header), body), size); err != nil {
		return "", err
	}
	var entry, slice [32]byte
	for i := int64(0); i < count; i++ {
		if _, err := file.ReadAt(entry[:stride], 8+i*stride); err != nil {
			return "", err
		}
		offset, length := uint64(order.Uint32(entry[8:])), uint64(order.Uint32(entry[12:]))
		if wide {
			offset, length = order.Uint64(entry[8:]), order.Uint64(entry[16:])
		}
		if offset < uint64(tableEnd) || offset > uint64(size) || length > uint64(size)-offset {
			continue
		}
		buf := slice[:min(length, uint64(len(slice)))]
		if _, err := file.ReadAt(buf, int64(offset)); err != nil {
			return "", err
		}
		if thinMachExecutable(buf) {
			return "Mach-O", nil
		}
	}
	return "", nil
}
