package scriptsnap

import (
	"bytes"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const (
	headerPeekBytes        = 4096
	maxShebangCaptureBytes = 257
)

// inspectHeader extracts fixed-size binary and shebang metadata from at most the first 4 KiB of head without
// retaining unbounded file content.
func inspectHeader(head []byte) shellwords.ScriptHeader {
	if len(head) > headerPeekBytes {
		head = head[:headerPeekBytes]
	}
	hasNUL := bytes.IndexByte(head, 0) >= 0
	return shellwords.ScriptHeader{
		HasNUL:      hasNUL,
		BinaryMagic: hasBinaryMagic(head, hasNUL),
		Shebang:     extractShebang(head),
	}
}

func hasBinaryMagic(head []byte, hasNUL bool) bool {
	if hasNUL && len(head) >= 2 && head[0] == 'M' && head[1] == 'Z' {
		return true
	}
	if len(head) < 4 {
		return false
	}
	switch [4]byte(head[:4]) {
	case [4]byte{0x7f, 0x45, 0x4c, 0x46},
		[4]byte{0xfe, 0xed, 0xfa, 0xce},
		[4]byte{0xce, 0xfa, 0xed, 0xfe},
		[4]byte{0xfe, 0xed, 0xfa, 0xcf},
		[4]byte{0xcf, 0xfa, 0xed, 0xfe},
		[4]byte{0xca, 0xfe, 0xba, 0xbe},
		[4]byte{0xbe, 0xba, 0xfe, 0xca}:
		return true
	default:
		return false
	}
}

func extractShebang(head []byte) string {
	if !bytes.HasPrefix(head, []byte("#!")) {
		return ""
	}
	line := head
	if nl := bytes.IndexByte(head, '\n'); nl >= 0 {
		line = head[:nl]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
	}
	if len(line) > maxShebangCaptureBytes {
		line = line[:maxShebangCaptureBytes]
	}
	return string(line)
}
