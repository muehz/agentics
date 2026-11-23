package conformance

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

// The freshness bite checks exercise the real verdict machinery — digest
// scan and toolchain scan — against the tiny hook module
// (tinyhook_test.go), whose committed binaries each scenario stamps or
// corrupts deliberately. Skipped under -short: building the tiny module's
// six contract binaries costs six cross-compiles.
//
// RunFreshness itself builds nothing and executes nothing, so no scenario
// here needs a host contract target; only the ones that rebuild the host
// binary do.

func skipFreshnessScenario(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: freshness scenarios cross-build the tiny module's six contract binaries")
	}
}

func requireHostTarget(t *testing.T) {
	t.Helper()
	if hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH) == "" {
		t.Skip("host has no contract binary target")
	}
}

func TestFreshnessPassOnPristineTinyRepo(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	failures, err := RunFreshness(root)
	assertGreen(t, failures, err)
}

func TestFreshnessPassesWithoutBinaries(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	assertGreen(t, failures, err)
}

func TestSourceOnlyFreshnessRequiresSixManifestEntries(t *testing.T) {
	root := cloneTinyRepo(t)
	removeRel(t, root, "hooks/bin")
	removeRel(t, root, "bin/agx.exe")
	removeRel(t, root, ".gitattributes")
	fixtureGit(t, root, "", "init", "-q")
	fixtureGit(t, root, "", "-c", "core.autocrlf=false", "add", ".")
	failures, err := runModuleGates(root, AuditOptions{SkipTests: true, SourceOnly: true})
	assertGreen(t, failures, err)
	failures, err = runFreshness(root, true)
	assertGreen(t, failures, err)
	writeRel(t, root, manifestRel, strings.Repeat("0", 64)+"  bin/"+hookbuild.ContractBinaries[0]+"\n")
	failures, err = runFreshness(root, true)
	if err != nil || len(failures) != len(hookbuild.ContractBinaries)-1 {
		t.Fatalf("missing manifest entries: %v, %v", failures, err)
	}
	assertFindingAt(t, failures, "freshness-manifest", manifestRel)
}

func TestFreshnessBitesStaleDigest(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	// Touch the source: the checkout digest moves, every committed stamp is
	// now stale. The mutation is a comment, so the binaries would behave
	// identically — exactly the drift only provenance can see.
	mutateTinyFile(t, root, "hooks/go/internal/provider/provider.go", "package provider", "package provider\n\n// a comment the binaries were not built from")
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-stale-digest")
}

func TestFreshnessBitesMissingDigestStamp(t *testing.T) {
	skipFreshnessScenario(t)
	requireHostTarget(t)
	root := cloneTinyRepo(t)
	// Rebuild the host binary without any stamp: it predates digest
	// stamping as far as the guard can tell. Its build info is intact, so
	// only the digest verdict may fire.
	rebuildTinyHost(t, root, "")
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-no-digest")
	for _, f := range failures {
		if f.Rule == "freshness-toolchain-drift" || f.Rule == "freshness-no-toolchain" {
			t.Fatalf("a real go build carries build info; the toolchain verdict must not fire: %v", f)
		}
	}
}

// The drift the digest cannot see: the source is untouched and every stamp
// matches, but the pin the binaries must answer to has moved away from the
// toolchain that built them.
func TestFreshnessBitesToolchainDrift(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	mutateTinyFile(t, root, "hooks/go/go.mod", tinyGoDirective(t), "go 1.24")
	// go.mod is hashed, so re-stamp: provenance must look clean and leave
	// the toolchain verdict as the only one that can fire.
	digest, err := hookprovenance.SourceDigest(filepath.Join(root, engineRel))
	if err != nil {
		t.Fatal(err)
	}
	if err := stampTinyBinariesRaw(root, "agentics-src-"+digest); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-toolchain-drift")
	// The launcher answers to the same pin as the six hook binaries.
	assertFindingAt(t, failures, "freshness-toolchain-drift", "bin/agx.exe")
	for _, f := range failures {
		if f.Rule == "freshness-stale-digest" || f.Rule == "freshness-no-digest" {
			t.Fatalf("the re-stamp should have satisfied the digest; only the toolchain may fire: %v", f)
		}
	}
}

func TestFreshnessBitesUnreadableBuildInfo(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	victim := filepath.Join(root, "hooks", "bin", "agx-linux-amd64")
	if err := os.WriteFile(victim, []byte("not an executable, padded well past the size floor........."), 0o755); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-no-toolchain")
}

func TestFreshnessPassesWithSomeBinaries(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	// A missing contract binary is not a failure: the dispatcher fetches it
	// and verifies the download against the manifest, which stays intact.
	if err := os.Remove(filepath.Join(root, "hooks", "bin", "agx-linux-arm64")); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	assertGreen(t, failures, err)
}

// The manifest is the anchor whether or not any binary is committed:
// run-hook.cmd cannot verify a downloaded binary without it.
func TestFreshnessBitesMissingManifestWithoutBinaries(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "hooks", "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-manifest")
}

func TestFreshnessBitesIncompleteManifest(t *testing.T) {
	skipFreshnessScenario(t)

	t.Run("missing entry", func(t *testing.T) {
		root := cloneTinyRepo(t)
		if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
			t.Fatal(err)
		}
		manifestPath := filepath.Join(root, "hooks", "SHA256SUMS")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.SplitN(string(data), "\n", 2)
		if len(lines) < 2 {
			t.Fatalf("manifest has too few lines to drop one: %q", data)
		}
		if err := os.WriteFile(manifestPath, []byte(lines[1]), 0o644); err != nil {
			t.Fatal(err)
		}
		failures, err := RunFreshness(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertRuleFired(t, failures, "freshness-manifest")
	})

	// An unprefixed contract name is an entry run-hook.cmd never matches, so
	// it counts as extra even though the name itself is a contract binary.
	for name, line := range map[string]string{
		"extra entry":      strings.Repeat("a", 64) + "  bin/agx-plan9-amd64\n",
		"unprefixed entry": strings.Repeat("a", 64) + "  agx-linux-amd64\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := cloneTinyRepo(t)
			if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(root, "hooks", "SHA256SUMS")
			f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(line); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			failures, err := RunFreshness(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRuleFired(t, failures, "freshness-manifest")
		})
	}
}

// The manifest is what run-hook.cmd checks a download against, so a stale
// entry makes the dispatcher discard the bytes the release ships. The digest
// and toolchain verdicts cannot see that: the binaries here are pristine.
func TestFreshnessBitesStaleManifest(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	data, err := os.ReadFile(filepath.Join(root, "hooks", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the first entry's hash; the path and the binary stay intact.
	mutated := "0000" + string(data)[4:]
	if err := os.WriteFile(filepath.Join(root, "hooks", "SHA256SUMS"), []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-manifest")
	for _, f := range failures {
		if f.Rule != "freshness-manifest" {
			t.Fatalf("only the manifest verdict may fire on pristine binaries: %v", f)
		}
	}
}

func TestFreshnessBitesMissingManifest(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	if err := os.Remove(filepath.Join(root, "hooks", "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "freshness-manifest")
}

// TestFreshnessBitesDuplicateOrMalformedLine covers what the entries alone
// hide: the manifest keeps the first entry per path and skips unparseable
// lines, so both cases reach the roster check looking like a clean six-entry
// manifest.
func TestFreshnessBitesDuplicateOrMalformedLine(t *testing.T) {
	skipFreshnessScenario(t)
	for name, tc := range map[string]struct {
		extra   func(manifest string) string
		rewrite func(manifest string) string
		want    string
	}{
		"duplicate entry": {
			extra: func(manifest string) string { first, _, _ := strings.Cut(manifest, "\n"); return first + "\n" },
			want:  "duplicate entry bin/",
		},
		"malformed line": {
			extra: func(string) string { return "not-a-manifest-line\n" },
			want:  "malformed line 7",
		},
		"truncated hash": {
			rewrite: func(manifest string) string {
				first, rest, _ := strings.Cut(manifest, "\n")
				return first[8:] + "\n" + rest
			},
			want: "bad hash on line 1",
		},
		"uppercase hash": {
			rewrite: func(manifest string) string {
				first, rest, _ := strings.Cut(manifest, "\n")
				return strings.ToUpper(first[:64]) + first[64:] + "\n" + rest
			},
			want: "bad hash on line 1",
		},
		"non-hex hash": {
			rewrite: func(manifest string) string {
				first, rest, _ := strings.Cut(manifest, "\n")
				return "g" + first[1:] + "\n" + rest
			},
			want: "bad hash on line 1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := cloneTinyRepo(t)
			if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "hooks", "SHA256SUMS")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var mutated []byte
			if tc.rewrite != nil {
				mutated = []byte(tc.rewrite(string(data)))
			} else {
				mutated = append(data, tc.extra(string(data))...)
			}
			if err := os.WriteFile(path, mutated, 0o644); err != nil {
				t.Fatal(err)
			}
			failures, err := RunFreshness(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, f := range failures {
				if f.Rule == "freshness-manifest" && strings.Contains(f.Message, tc.want) {
					return
				}
			}
			t.Fatalf("want a freshness-manifest failure containing %q, got %v", tc.want, failures)
		})
	}
}

// A manifest the dispatcher cannot read vouches for no binary, so the one
// unusable-manifest finding stands alone: the stale hash beside it must not
// also be reported against a committed binary.
func TestFreshnessManifestUnusableSkipsEntryChecks(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	path := filepath.Join(root, "hooks", "SHA256SUMS")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	lines[0] = "0000" + lines[0][4:]
	lines[1] += "\r"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failures) != 1 || failures[0].Rule != "freshness-manifest" || !strings.Contains(failures[0].Message, "carries a carriage return") {
		t.Fatalf("want exactly the carriage-return finding, got %v", failures)
	}
}

// TestManifestFindingsMessages pins the findings the manifest reading reports,
// message bytes included, for each fault the guard names. The expected values
// are literal, so a reworded message or a changed line number fails here
// rather than passing because two readings were changed together.
func TestManifestFindingsMessages(t *testing.T) {
	const suffix = " (make -C hooks/go build-all)"
	const unusable = " — run-hook.cmd cannot verify a downloaded binary without it" + suffix
	fail := func(message string) []Failure {
		return []Failure{{Rule: "freshness-manifest", Path: "hooks/SHA256SUMS", Message: message}}
	}
	lines := func(edit func(l []string) []string) string {
		l := []string{
			strings.Repeat("a", 64) + "  bin/agx-darwin-amd64",
			strings.Repeat("b", 64) + "  bin/agx-darwin-arm64",
			strings.Repeat("c", 64) + "  bin/agx-linux-amd64",
			strings.Repeat("d", 64) + "  bin/agx-linux-arm64",
			strings.Repeat("e", 64) + "  bin/agx-windows-amd64.exe",
			strings.Repeat("f", 64) + "  bin/agx-windows-arm64.exe",
		}
		if edit != nil {
			l = edit(l)
		}
		return strings.Join(l, "\n") + "\n"
	}
	for _, tc := range []struct {
		name string
		body string
		want []Failure
	}{
		{"clean manifest", lines(nil), nil},
		{"empty manifest", "", fail("the checksum manifest has no entries" + unusable)},
		{
			"carriage return",
			lines(func(l []string) []string { l[2] += "\r"; return l }),
			fail("the checksum manifest line 3 carries a carriage return" + unusable),
		},
		{
			"missing entry",
			lines(func(l []string) []string { return l[1:] }),
			fail("no entry for bin/agx-darwin-amd64 — run-hook.cmd cannot verify a downloaded agx-darwin-amd64" + suffix),
		},
		{
			"unexpected entry",
			lines(func(l []string) []string { return append(l, strings.Repeat("9", 64)+"  bin/agx-plan9-amd64") }),
			fail("unexpected entry bin/agx-plan9-amd64 — the manifest lists only the six contract binaries" + suffix),
		},
		{
			"blank interior line",
			lines(func(l []string) []string { return append(l[:3], append([]string{""}, l[3:]...)...) }),
			fail("malformed line 4 — each line is \"<sha256>  bin/<name>\"" + suffix),
		},
		{
			"three-field line",
			lines(func(l []string) []string { l[1] += "  extra"; return l }),
			fail("malformed line 2 — each line is \"<sha256>  bin/<name>\"" + suffix),
		},
		{
			"uppercase hash",
			lines(func(l []string) []string { l[0] = strings.ToUpper(l[0][:64]) + l[0][64:]; return l }),
			fail("bad hash on line 1 — each hash is 64 lowercase hex characters" + suffix),
		},
		{
			"duplicate entry",
			lines(func(l []string) []string { return append(l, strings.Repeat("7", 64)+"  bin/agx-darwin-amd64") }),
			fail("duplicate entry bin/agx-darwin-amd64 — run-hook.cmd reads only the first" + suffix),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "SHA256SUMS")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, _ := manifestFindings(path)
			if !slices.Equal(got, tc.want) {
				t.Errorf("findings\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}
