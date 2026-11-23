package hosttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMarker(t *testing.T, root string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(marker))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindCheckoutVariableWinsOverWalk(t *testing.T) {
	root := t.TempDir()
	writeMarker(t, root)
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	// The value is returned verbatim, even when it names no checkout.
	const value = "/not/a/real/checkout"
	t.Setenv(envRepoRoot, value)

	got, err := FindCheckout()
	if err != nil {
		t.Fatal(err)
	}
	if got != value {
		t.Fatalf("FindCheckout() = %q, want %q", got, value)
	}
}

func TestFindCheckoutWalksToMarker(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeMarker(t, root)
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv(envRepoRoot, "")

	got, err := FindCheckout()
	if err != nil {
		t.Fatal(err)
	}
	if want := root; !sameDir(t, got, want) {
		t.Fatalf("FindCheckout() = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("FindCheckout() = %q, want an absolute native path", got)
	}
}

func TestFindCheckoutNoMarkerErrorNamesMarkerAndVariable(t *testing.T) {
	// A temporary directory sits outside any checkout on CI and developer
	// hosts alike; skip when this host places it inside one.
	dir := t.TempDir()
	if _, found := walk(dir); found {
		t.Skip("temporary directory sits inside a checkout")
	}
	t.Chdir(dir)
	t.Setenv(envRepoRoot, "")

	got, err := FindCheckout()
	if err == nil {
		t.Fatalf("FindCheckout() = %q, want an error", got)
	}
	for _, want := range []string{".claude-plugin/plugin.json", "AGX_REPO_ROOT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestCheckoutReturnsFoundRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv(envRepoRoot, root)
	if got := Checkout(t); got != root {
		t.Fatalf("Checkout() = %q, want %q", got, root)
	}
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
}
