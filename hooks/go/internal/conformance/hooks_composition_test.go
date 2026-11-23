package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestAuditFailureJSON pins the keys of the audit result the TypeScript
// conformance reads: a failure marshals to exactly rule, path, and message, and
// an area with no findings still marshals an empty failures array, not null.
func TestAuditFailureJSON(t *testing.T) {
	area := AuditArea{Name: "schema", Failures: []Failure{{Rule: "r", Path: "p", Message: "m"}}}
	encoded, err := json.Marshal(area)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Failures []map[string]any `json:"failures"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal %s: %v", encoded, err)
	}
	if len(decoded.Failures) != 1 {
		t.Fatalf("failures = %v, want one entry in %s", decoded.Failures, encoded)
	}
	want := map[string]any{"rule": "r", "path": "p", "message": "m"}
	if !reflect.DeepEqual(decoded.Failures[0], want) {
		t.Fatalf("failure keys = %v, want exactly %v", decoded.Failures[0], want)
	}

	empty, err := json.Marshal(AuditArea{Name: "ascii", Failures: []Failure{}})
	if err != nil {
		t.Fatalf("marshal empty area: %v", err)
	}
	if !strings.Contains(string(empty), `"failures":[]`) {
		t.Fatalf("empty area = %s, want \"failures\":[]", empty)
	}
}

// auditFailures runs Audit with default options and flattens the areas'
// failures in area order, the shape the composition tests read.
func auditFailures(t *testing.T, root string) []Failure {
	t.Helper()
	result, err := Audit(root, AuditOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var failures []Failure
	for _, area := range result.Areas {
		failures = append(failures, area.Failures...)
	}
	return failures
}

func TestAuditComposesEveryHookAreaAndPropagatesFailures(t *testing.T) {
	want := []string{"ascii", "schema", "module", "freshness"}
	var got []string
	for _, area := range hookAreas {
		got = append(got, area.name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hook area roster = %v, want %v", got, want)
	}

	original := hookAreas
	t.Cleanup(func() { hookAreas = original })
	hookAreas = nil
	received := map[string]AuditOptions{}
	for _, name := range want {
		name := name
		hookAreas = append(hookAreas, hookArea{name: name, run: func(_ string, options AuditOptions) ([]Failure, error) {
			received[name] = options
			return []Failure{{Rule: "injected-" + name}}, nil
		}})
	}

	result, err := Audit(t.TempDir(), AuditOptions{SkipTests: true})
	if err != nil {
		t.Fatalf("Audit returned an error: %v", err)
	}
	var failures []Failure
	for _, area := range result.Areas {
		failures = append(failures, area.Failures...)
	}
	for _, name := range want {
		if !received[name].SkipTests {
			t.Errorf("injected %q area did not receive the audit options: %+v", name, received[name])
		}
	}
	if len(failures) != len(want) {
		t.Fatalf("Audit returned %d injected failures, want %d: %v", len(failures), len(want), failures)
	}
	for i, name := range want {
		if failures[i].Rule != "injected-"+name {
			t.Errorf("failure %d = %q, want %q", i, failures[i].Rule, "injected-"+name)
		}
	}
}

// TestAuditDedupesRebuildFindingsBehindStaleDigest is the #572 scenario: a
// stale embedded source digest makes RunFreshness report
// freshness-stale-digest AND makes RunModuleGates's rebuild comparison fail
// for the identical reason, once per contract binary (module-rebuild-mismatch
// and module-rebuild-manifest). That is up to 12 findings for one cause a
// maintainer can only fix one way (make -C hooks/go build-all). Audit must
// report the root cause once and drop the redundant rebuild findings it
// causes.
func TestAuditDedupesRebuildFindingsBehindStaleDigest(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	// Touch the source without rebuilding: the checkout digest moves, but
	// every committed binary — and the manifest recorded for it — still
	// answers for the OLD digest. The mutation is a comment, so this is not
	// a real behavior change, only the drift only provenance can see.
	mutateTinyFile(t, root, "hooks/go/internal/provider/provider.go", "package provider", "package provider\n\n// a comment the binaries were not built from")

	failures := auditFailures(t, root)
	assertRuleFired(t, failures, "freshness-stale-digest")
	for _, f := range failures {
		if f.Rule == "module-rebuild-mismatch" || f.Rule == "module-rebuild-manifest" {
			t.Fatalf("a stale source digest already explains every rebuild finding; Audit must not also report %v", f)
		}
	}
}

// TestAuditKeepsRebuildMismatchWithFreshDigest is the negative case for the
// dedup filter: a genuine byte mismatch with no stale digest involved (the
// checkout is untouched; only the committed binary was tampered with, not
// rebuilt from it) must still be reported. The filter only fires alongside
// freshness-stale-digest, never unconditionally.
func TestAuditKeepsRebuildMismatchWithFreshDigest(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	// Tamper the committed bytes directly: nothing rebuilt this binary, so
	// the source digest stays fresh and RunFreshness's digest verdict must
	// not fire. Appending a byte (rather than flipping one in place) leaves
	// every header, section table, and embedded stamp exactly where the
	// machine-format, toolchain, and digest checks expect to find them.
	victim := filepath.Join(root, "hooks", "bin", "agx-linux-amd64")
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, append(data, 0x00), 0o755); err != nil {
		t.Fatal(err)
	}

	failures := auditFailures(t, root)
	for _, f := range failures {
		if f.Rule == "freshness-stale-digest" {
			t.Fatalf("tampering committed bytes directly must not move the source digest: %v", f)
		}
	}
	assertRuleFired(t, failures, "module-rebuild-mismatch")
}

// TestAuditKeepsOtherBinariesRebuildFindingsWhenOnlyOneIsStale is the mixed
// scenario the per-Path filter exists for: one committed binary's embedded
// digest is stale, and a DIFFERENT committed binary was genuinely tampered
// while its own digest stays fresh. A rule-only filter (drop every
// module-rebuild-mismatch/-manifest as soon as ANY freshness-stale-digest
// fires) would hide the second binary's tamper — and `make build-all` would
// then silently overwrite the only evidence of it. The stale digest is
// forged directly in the committed binary's embedded stamp bytes (same
// length in place, so the file otherwise decodes exactly as it did): moving
// the real source digest would stale EVERY binary identically, since one
// digest covers the whole module, and could never exercise "only one is
// stale".
func TestAuditKeepsOtherBinariesRebuildFindingsWhenOnlyOneIsStale(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	const staleBinary = "agx-darwin-arm64"
	const tamperedBinary = "agx-linux-amd64"

	fakeStaleStamp(t, filepath.Join(root, "hooks", "bin", staleBinary))

	// tamperedBinary: genuinely different bytes with a FRESH digest (its
	// stamp, and the source, are untouched), plus its own manifest entry
	// corrupted independently — this binary's findings have nothing to do
	// with the OTHER binary's stale digest and must survive the filter.
	victim := filepath.Join(root, "hooks", "bin", tamperedBinary)
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, append(data, 0x00), 0o755); err != nil {
		t.Fatal(err)
	}
	corruptManifestEntry(t, root, tamperedBinary)

	failures := auditFailures(t, root)

	assertRuleFired(t, failures, "freshness-stale-digest")
	assertRuleFired(t, failures, "module-rebuild-manifest")
	var sawTamperedMismatch bool
	for _, f := range failures {
		if f.Rule != "module-rebuild-mismatch" {
			continue
		}
		switch f.Path {
		case "hooks/bin/" + staleBinary:
			t.Fatalf("%s's own rebuild mismatch is explained by its own stale digest and must be dropped: %v", staleBinary, f)
		case "hooks/bin/" + tamperedBinary:
			sawTamperedMismatch = true
		}
	}
	if !sawTamperedMismatch {
		t.Fatalf("%s's rebuild mismatch is not explained by %s's stale digest and must survive: %v", tamperedBinary, staleBinary, failures)
	}
}

// fakeStaleStamp overwrites a committed binary's embedded agentics-src-<digest>
// stamp with a same-length bogus value, in place. This forges "this one
// binary's digest is stale" without moving the real source digest, which
// would stale every committed binary identically.
func fakeStaleStamp(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loc := digestStampPattern.FindIndex(data)
	if loc == nil {
		t.Fatalf("no source-digest stamp found in %s", path)
	}
	fake := []byte("agentics-src-" + strings.Repeat("f", 64))
	if len(fake) != loc[1]-loc[0] {
		t.Fatalf("forged stamp length %d != matched stamp length %d", len(fake), loc[1]-loc[0])
	}
	copy(data[loc[0]:loc[1]], fake)
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// corruptManifestEntry rewrites name's recorded hash in the cloned repo's
// hooks/SHA256SUMS to a value that matches neither the committed binary nor
// a fresh rebuild, independent of any byte tamper on the committed binary
// itself.
func corruptManifestEntry(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, "hooks", "SHA256SUMS")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	found := false
	for i, line := range lines {
		if strings.HasSuffix(line, "bin/"+name) {
			lines[i] = strings.Repeat("e", 64) + "  bin/" + name
			found = true
		}
	}
	if !found {
		t.Fatalf("hooks/SHA256SUMS has no entry for bin/%s", name)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDedupeStaleRebuildFiltersOnlyExplainedFindings unit-tests the filter
// directly against synthetic findings, pinning the per-Path semantics the
// end-to-end scenarios above exercise through the real checks.
func TestDedupeStaleRebuildFiltersOnlyExplainedFindings(t *testing.T) {
	binaries := []string{"a", "b"}
	cases := []struct {
		name string
		in   []Failure
		want []Failure
	}{
		{
			name: "no stale digest keeps every rebuild finding",
			in: []Failure{
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/a"},
				{Rule: "module-rebuild-manifest", Path: "hooks/SHA256SUMS"},
			},
			want: []Failure{
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/a"},
				{Rule: "module-rebuild-manifest", Path: "hooks/SHA256SUMS"},
			},
		},
		{
			name: "one binary stale drops only that binary's mismatch, keeps the other's and the manifest finding",
			in: []Failure{
				{Rule: "freshness-stale-digest", Path: "hooks/bin/a"},
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/a"},
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/b"},
				{Rule: "module-rebuild-manifest", Path: "hooks/SHA256SUMS"},
			},
			want: []Failure{
				{Rule: "freshness-stale-digest", Path: "hooks/bin/a"},
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/b"},
				{Rule: "module-rebuild-manifest", Path: "hooks/SHA256SUMS"},
			},
		},
		{
			name: "every binary stale drops both rebuild rules",
			in: []Failure{
				{Rule: "freshness-stale-digest", Path: "hooks/bin/a"},
				{Rule: "freshness-stale-digest", Path: "hooks/bin/b"},
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/a"},
				{Rule: "module-rebuild-mismatch", Path: "hooks/bin/b"},
				{Rule: "module-rebuild-manifest", Path: "hooks/SHA256SUMS"},
			},
			want: []Failure{
				{Rule: "freshness-stale-digest", Path: "hooks/bin/a"},
				{Rule: "freshness-stale-digest", Path: "hooks/bin/b"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dedupeStaleRebuild(c.in, binaries)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("dedupeStaleRebuild = %v, want %v", got, c.want)
			}
		})
	}
}
