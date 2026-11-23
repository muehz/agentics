package releaseguard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestValidReleaseRecordHash(t *testing.T) {
	content := []byte("# Change Record: rotate keys\n\n**ID:** 2026-09-12-rotate\n")

	// hash returns nil for "leave AGX_RELEASE_RECORD_SHA256 unset" and a
	// non-nil pointer (possibly to "") for "set it to this value" — the two
	// are different configurations: unset validates as before, while
	// set-but-empty is a misconfiguration that must fail closed.
	unset := func([]byte) *string { return nil }
	set := func(v string) func([]byte) *string {
		return func([]byte) *string { return &v }
	}

	cases := []struct {
		name string
		hash func(content []byte) *string
		want error // nil means valid
	}{
		{
			name: "unset validates as before",
			hash: unset,
			want: nil,
		},
		{
			name: "set but empty denies as not a digest",
			hash: set(""),
			want: errRecordHashNotHex,
		},
		{
			name: "matching lowercase hash passes",
			hash: func(c []byte) *string { v := sha256Hex(c); return &v },
			want: nil,
		},
		{
			name: "matching uppercase hash passes",
			hash: func(c []byte) *string { v := strings.ToUpper(sha256Hex(c)); return &v },
			want: nil,
		},
		{
			name: "another 64-hex value denies as a mismatch",
			hash: set(strings.Repeat("a", 64)),
			want: errRecordHashMismatch,
		},
		{
			name: "a short value denies as not a digest",
			hash: set("deadbeef"),
			want: errRecordHashNotHex,
		},
		{
			name: "a non-hex 64-length value denies as not a digest",
			hash: set(strings.Repeat("g", 64)),
			want: errRecordHashNotHex,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeRecord(t, string(content), time.Minute)
			if hash := tc.hash(content); hash == nil {
				os.Unsetenv(recordHashVar)
			} else {
				t.Setenv(recordHashVar, *hash)
			}
			err := validReleaseRecord(path, time.Now())
			if tc.want == nil {
				if err != nil {
					t.Fatalf("err=%v, want nil (valid)", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v, want errors.Is(err, %v)", err, tc.want)
			}
		})
	}
}

func TestValidReleaseRecordHashRejectsOversizedRecord(t *testing.T) {
	big := make([]byte, recordHashLimit+1)
	for i := range big {
		big[i] = 'x'
	}
	content := append([]byte(recordMarker+"\n"), big...)
	path := writeRecord(t, string(content), time.Minute)
	t.Setenv(recordHashVar, strings.Repeat("a", 64))
	err := validReleaseRecord(path, time.Now())
	if err == nil {
		t.Fatalf("err = nil, want an error for an oversized record")
	}
	if !errors.Is(err, errRecordTooLarge) {
		t.Fatalf("err = %v, want errRecordTooLarge", err)
	}
}
