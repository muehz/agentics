package secretsguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// padDir fills dir with enough empty files that listing it exhausts the entry budget.
func padDir(t *testing.T, dir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("pad%05d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGuardGlobLargeAncestorKeepsLiteralPrefixCheap(t *testing.T) {
	parent := globFixture(t)
	padDir(t, parent, globEntryLimit+8)
	root := filepath.Join(parent, "fixture")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("placeholder\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, tool, command, cwd string }{
		{"absolute", "PowerShell", "Get-Content '" + root + string(filepath.Separator) + ".e[n]v'", ""},
		{"relative", "PowerShell", "Get-Content fixture/.e[n]v", parent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, tc.tool, map[string]any{"command": tc.command}, tc.cwd), nil)
			if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
				t.Fatalf("kind=%s message=%s", kind, message)
			}
		})
	}
}

func TestGuardGlobEntryLimitStillBitesOnWildcardComponent(t *testing.T) {
	root := globFixture(t)
	padDir(t, root, globEntryLimit+8)
	command := "Get-Content '" + root + string(filepath.Separator) + "pad*'"
	kind, message := run(t, buildEventAt(t, "PowerShell", map[string]any{"command": command}, ""), nil)
	if kind != "deny" || !strings.Contains(message, "directory entry limit exceeded") {
		t.Fatalf("kind=%s message=%s", kind, message)
	}
}

func TestGuardGlobWrongCaseLiteralPrefixFollowsFilesystem(t *testing.T) {
	root := globFixture(t, "Store/.env")
	_, err := os.Lstat(filepath.Join(root, "STORE"))
	resolves := err == nil
	command := "Get-Content '" + root + string(filepath.Separator) + "STORE" + string(filepath.Separator) + ".e[n]v'"
	kind, message := run(t, buildEventAt(t, "PowerShell", map[string]any{"command": command}, ""), nil)
	if resolves && (kind != "deny" || !strings.Contains(message, "(dotenv, critical)")) {
		t.Fatalf("filesystem resolves the spelling but kind=%s message=%s", kind, message)
	}
	if !resolves && kind != "pass" {
		t.Fatalf("filesystem rejects the spelling but kind=%s message=%s", kind, message)
	}
}

func TestGuardGlobFinalLiteralAfterWildcardMatchesCaseInsensitively(t *testing.T) {
	root := globFixture(t, "Store/.env")
	command := "Get-Content '" + root + string(filepath.Separator) + "Sto*" + string(filepath.Separator) + ".ENV'"
	kind, message := run(t, buildEventAt(t, "PowerShell", map[string]any{"command": command}, ""), nil)
	if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
		t.Fatalf("kind=%s message=%s", kind, message)
	}
}
