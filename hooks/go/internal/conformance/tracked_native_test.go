package conformance

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fixtureGit(t *testing.T, root, input string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "init.templateDir="}, args...)...)
	cmd.Dir = root
	cmd.Env = slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(strings.ToUpper(e), "GIT_") })
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ALLOW_PROTOCOL=file")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSourceOnlyIndexFormats(t *testing.T) {
	root := t.TempDir()
	fixtureGit(t, root, "", "init", "-q")
	// Object and executable variants use identical magic bytes.
	elfHeader := func(typ elf.Type, order binary.ByteOrder) []byte {
		b := make([]byte, 64)
		copy(b, elf.ELFMAG)
		b[4], b[5], b[6] = byte(elf.ELFCLASS64), byte(elf.ELFDATA2MSB), 1
		if order == binary.LittleEndian {
			b[5] = byte(elf.ELFDATA2LSB)
		}
		order.PutUint16(b[16:], uint16(typ))
		order.PutUint16(b[18:], uint16(elf.EM_X86_64))
		order.PutUint32(b[20:], 1)
		order.PutUint16(b[52:], 64)
		return b
	}
	mach := func(typ macho.Type) []byte {
		b := make([]byte, 32)
		binary.BigEndian.PutUint32(b, macho.Magic64)
		binary.BigEndian.PutUint32(b[4:], uint32(macho.CpuAmd64))
		binary.BigEndian.PutUint32(b[12:], uint32(typ))
		return b
	}
	fat := func(image []byte) []byte {
		b := make([]byte, 32)
		binary.BigEndian.PutUint32(b, macho.MagicFat)
		binary.BigEndian.PutUint32(b[4:], 1)
		binary.BigEndian.PutUint32(b[8:], uint32(macho.CpuAmd64))
		binary.BigEndian.PutUint32(b[16:], uint32(len(b)))
		binary.BigEndian.PutUint32(b[20:], uint32(len(image)))
		return append(b, image...)
	}
	pe := func(executable bool) []byte {
		const offset = 8192
		b := make([]byte, offset+24+240)
		copy(b, "MZ")
		binary.LittleEndian.PutUint32(b[60:], offset)
		copy(b[offset:], "PE\x00\x00")
		binary.LittleEndian.PutUint16(b[offset+4:], 0x8664)
		binary.LittleEndian.PutUint16(b[offset+20:], 240)
		binary.LittleEndian.PutUint16(b[offset+24:], 0x20b)
		if executable {
			binary.LittleEndian.PutUint16(b[offset+22:], 2)
		}
		return b
	}
	cases := []struct {
		path string
		blob []byte
		kind string
	}{
		{"exe", elfHeader(elf.ET_EXEC, binary.BigEndian), "ELF"},
		{"pie", elfHeader(elf.ET_DYN, binary.LittleEndian), "ELF"},
		{"elf-object", elfHeader(elf.ET_REL, binary.LittleEndian), ""},
		{"app", mach(macho.TypeExec), "Mach-O"},
		{"mach-object", mach(macho.TypeObj), ""},
		{"universal", fat(mach(macho.TypeExec)), "Mach-O"},
		{"fat-object", fat(mach(macho.TypeObj)), ""},
		{"program with spaces", pe(true), "PE"},
		{"non-image", pe(false), ""},
		{"text.exe", bytes.Repeat([]byte("MZ is a prefix in text. "), 10), ""},
		{"java-class", []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 61}, ""},
	}
	for _, tc := range cases {
		if err := os.WriteFile(filepath.Join(root, tc.path), tc.blob, 0644); err != nil {
			t.Fatal(err)
		}
	}
	fixtureGit(t, root, "", "-c", "core.autocrlf=false", "add", ".")
	found, err := checkTrackedNative(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Failure{}
	for _, f := range found {
		got[f.Path] = f
	}
	for _, tc := range cases {
		f, ok := got[tc.path]
		if ok != (tc.kind != "") || ok && (f.Rule != "tracked-native-executable" || !strings.Contains(f.Message, tc.kind)) {
			t.Errorf("%s: got %+v, want %q", tc.path, f, tc.kind)
		}
	}
}

func TestSourceOnlySelectedIndex(t *testing.T) {
	root := t.TempDir()
	fixtureGit(t, root, "", "init", "-q")
	writeRel(t, root, "file", "source")
	fixtureGit(t, root, "", "add", "file")
	oid := fixtureGit(t, root, "", "rev-parse", ":file")
	fixtureGit(t, root, "", "-c", "user.name=fixture", "-c", "user.email=fixture@localhost", "commit", "-qm", "fixture")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, root, "", "worktree", "add", "-q", "--detach", linked)
	fixtureGit(t, root, "100644 "+oid+" 1\tconflict\n", "update-index", "--index-info")
	t.Run("caller points at another index", func(t *testing.T) {
		t.Setenv("GIT_DIR", filepath.Join(root, ".git"))
		t.Setenv("GIT_INDEX_FILE", filepath.Join(root, ".git", "index"))
		if found, err := checkTrackedNative(linked); err != nil || len(found) != 0 {
			t.Fatalf("expected the linked worktree's own clean index: %+v, %v", found, err)
		}
	})
	nested := filepath.Join(linked, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := checkTrackedNative(nested); err == nil {
		t.Fatal("accepted the parent repository for a nested candidate")
	}
	if _, err := checkTrackedNative(root); err == nil || !strings.Contains(err.Error(), "unmerged") {
		t.Fatalf("unmerged index: %v", err)
	}
	fixtureGit(t, root, "", "update-index", "--force-remove", "conflict")
	fixtureGit(t, root, "", "update-index", "--add", "--cacheinfo", "100644,"+strings.Repeat("1", len(oid))+",missing")
	if _, err := checkTrackedNative(root); err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "cat-file") {
		t.Fatalf("missing object: %v", err)
	}
}

func TestSourceOnlyMissingPromisorBlob(t *testing.T) {
	origin := t.TempDir()
	fixtureGit(t, origin, "", "init", "-q")
	writeRel(t, origin, "file", "only in the local promisor repository")
	fixtureGit(t, origin, "", "add", "file")
	fixtureGit(t, origin, "", "-c", "user.name=fixture", "-c", "user.email=fixture@localhost", "commit", "-qm", "fixture")
	fixtureGit(t, origin, "", "config", "uploadpack.allowAnySHA1InWant", "true")
	fixtureGit(t, origin, "", "config", "uploadpack.allowFilter", "true")
	oid := fixtureGit(t, origin, "", "rev-parse", ":file")
	root := t.TempDir()
	fixtureGit(t, root, "", "init", "-q")
	fixtureGit(t, root, "", "remote", "add", "origin", origin)
	fixtureGit(t, root, "", "config", "remote.origin.promisor", "true")
	fixtureGit(t, root, "", "update-index", "--add", "--cacheinfo", "100644,"+oid+",missing")
	if _, err := checkTrackedNative(root); err == nil || !strings.Contains(err.Error(), "cat-file") {
		t.Fatalf("the audit must report an unavailable blob: %v", err)
	}
	// Older Git releases ignore GIT_NO_LAZY_FETCH. Exercise the transport
	// restriction independently even on Git versions that know the setting.
	cmd := candidateGit(root, "cat-file", "blob", oid)
	cmd.Env = slices.DeleteFunc(cmd.Env, func(e string) bool { return strings.HasPrefix(e, "GIT_NO_LAZY_FETCH=") })
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("a missing blob was fetched from the fixture promisor: %q", out)
	}
	if got := fixtureGit(t, root, "", "cat-file", "blob", oid); got != "only in the local promisor repository" {
		t.Fatalf("local promisor control returned %q", got)
	}
}
