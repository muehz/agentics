package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// readSurfaceFixture returns one payload fixture for one surface.
func readSurfaceFixture(t *testing.T, providerName surface.ID, fixture string) string {
	t.Helper()
	path := filepath.Join("..", "..", "surfaces", string(providerName), "payloads", fixture)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s fixture %s: %v", providerName, fixture, err)
	}
	return string(payload)
}

// TestMCPPublicationIsGatedOnEverySurface drives the real pipeline — dispatch,
// normalize, the command-guardian core, render — with a GitHub MCP call that
// opens a live pull request.
//
// It runs per surface because the hole this closes was per surface in shape:
// each adapter carries the tool name and the typed input under its own keys,
// and a gate that reads typed fields is only as good as the normalization that
// delivers them. A unit test on the core cannot see a key that never arrives.
func TestMCPPublicationIsGatedOnEverySurface(t *testing.T) {
	for _, providerName := range surface.All() {
		t.Run(string(providerName), func(t *testing.T) {
			payload := readSurfaceFixture(t, providerName, "pre-tool-use-mcp-publication.json")
			code, stdout, stderr := invoke(t, payload, string(providerName), "command-guardian", "--mode", "block")
			requireDeny(t, providerName, code, stdout, stderr)
		})
	}
}

// TestMCPCredentialReadIsGatedOnEverySurface is the same end-to-end proof for
// the other hook: a remote read of a credential path.
func TestMCPCredentialReadIsGatedOnEverySurface(t *testing.T) {
	for _, providerName := range surface.All() {
		t.Run(string(providerName), func(t *testing.T) {
			payload := readSurfaceFixture(t, providerName, "pre-tool-use-mcp-credential-read.json")
			code, stdout, stderr := invoke(t, payload, string(providerName), "secrets-guardian", "--mode", "block")
			requireDeny(t, providerName, code, stdout, stderr)
		})
	}
}
