package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostTestsKeepCommandIntegrationOnUnix(t *testing.T) {
	root := hostTestsFixture(t, "")
	for _, host := range []string{"windows", "linux", "darwin"} {
		t.Run(host, func(t *testing.T) {
			failures, err := checkHostTests(root, "go", host)
			if err != nil {
				t.Fatal(err)
			}
			if host == "windows" {
				if len(failures) != 0 {
					t.Fatalf("Windows compiled excluded command tests: %v", failures)
				}
			} else if len(failures) != 1 || !strings.Contains(failures[0].Message, "commandIntegrationMustNotCompileOnWindows") {
				t.Fatalf("%s omitted command integration tests: %v", host, failures)
			}
		})
	}
}

func TestHostTestsWindowsPropagatesRemainingPackageFailure(t *testing.T) {
	root := hostTestsFixture(t, `t.Fatal("remaining-package-failure")`)
	failures, err := checkHostTests(root, "go", "windows")
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || failures[0].Rule != "module-test" || !strings.Contains(failures[0].Message, "remaining-package-failure") {
		t.Fatalf("remaining package failure was lost: %v", failures)
	}
}

func hostTestsFixture(t *testing.T, testBody string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                       "module example.test/hooks\n\n" + tinyGoDirective(t) + "\n",
		"cmd/hook/main.go":             "package main\nfunc main() {}\n",
		"cmd/hook/main_test.go":        "package main\nvar _ = commandIntegrationMustNotCompileOnWindows\n",
		"internal/value/value.go":      "package value\nfunc Value() int { return 1 }\n",
		"internal/value/value_test.go": "package value\nimport \"testing\"\nfunc TestValue(t *testing.T) { " + testBody + " }\n",
	}
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
