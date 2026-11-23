// Package hosttest is test support imported only by tests: it finds the
// checkout the tests run against. It is separate from the packages it serves
// so a non-test package never imports testing, and it is listed in
// digest-exclude because no hook binary imports it.
package hosttest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	// envRepoRoot names the variable that pins the checkout root, for runs
	// whose working directory is not inside the checkout.
	envRepoRoot = "AGX_REPO_ROOT"

	// marker is the checkout-relative file that identifies a checkout root.
	marker = ".claude-plugin/plugin.json"
)

// FindCheckout returns the checkout root: the AGX_REPO_ROOT value verbatim
// when it is set and non-empty, otherwise the nearest ancestor of the working
// directory that holds .claude-plugin/plugin.json, as a native absolute path.
// It returns an error when neither names a checkout. A caller that needs
// slash-separated paths converts at its own call site.
func FindCheckout() (string, error) {
	if root := os.Getenv(envRepoRoot); root != "" {
		return root, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find the checkout: %w", err)
	}
	if root, found := walk(wd); found {
		return root, nil
	}
	return "", fmt.Errorf("no checkout found: no %s above %s and %s is unset", marker, wd, envRepoRoot)
}

// Checkout returns the checkout root and fails the test when there is none.
func Checkout(t testing.TB) string {
	t.Helper()
	root, err := FindCheckout()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// walk climbs from dir to the first directory that holds the marker.
func walk(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(marker))); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
