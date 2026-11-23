# Agentics hook binary.
#
# Builds cmd/hook for every platform Agentics dispatches hooks on. The output
# lands in the PLUGIN's hooks/bin/, not here: this module is maintainer-facing
# source, while the binaries are part of what a consumer installs. The paths
# below are a contract — hooks/run-hook.cmd selects a binary by GOOS/GOARCH
# name, so renaming one breaks dispatch.
#
#   darwin/amd64    ../bin/agx-darwin-amd64
#   darwin/arm64    ../bin/agx-darwin-arm64
#   linux/amd64     ../bin/agx-linux-amd64
#   linux/arm64     ../bin/agx-linux-arm64
#   windows/amd64   ../bin/agx-windows-amd64.exe
#   windows/arm64   ../bin/agx-windows-arm64.exe
#
# It also builds the agx launcher, which is not a hook binary: it lands at the
# repository root and is outside SHA256SUMS (verify compares a local copy with
# a fresh build when one exists).
#
#   windows/amd64   ../../bin/agx.exe (cmd/launcher)
#
# The .exe suffix is MANDATORY: cmd.exe will not exec an extensionless PE.
#
# Recipe lines MUST begin with a literal TAB — a space-indented recipe is a
# hard make error ("missing separator"). .editorconfig declares
# indent_style = tab for [{Makefile,*.mk}] so editors comply automatically.

GO       ?= go
HOOKS_DIR := ..
BIN_DIR   := $(HOOKS_DIR)/bin
MANIFEST  := $(HOOKS_DIR)/SHA256SUMS
CMD       := ./cmd/hook

# Static binaries: cgo off so a linux build runs against any libc and no host
# toolchain is needed to cross-compile.
export CGO_ENABLED := 0

# Pin the microarchitecture-tuning knobs `go build` otherwise reads from the
# environment to Go's own defaults. A maintainer or CI host with GOAMD64=v3
# set (or any non-default GOARM64) would produce different bytes from
# identical source, the same Go version, and these same flags — a drift the
# rebuild-comparison gate in hooks/go/internal/conformance cannot tell
# apart from real tampering. Pinning here, not just in that gate, is what
# makes the release binaries reproducible in the first place.
export GOAMD64 := v1
export GOARM64 := v8.0

# Every artifact embeds a digest of the Go source that built it, so the
# freshness guard (the conformance suite's `hooks freshness` check, in
# hooks/go/internal/conformance) can verify the platform binaries it cannot
# execute by provenance: it recomputes the digest from the checkout and scans
# each rebuilt binary for the stamp.
#
# cmd/sourcedigest prints hookprovenance.SourceDigest, the same digest the
# freshness guard recomputes, so the stamp and the check cannot disagree about
# which files count. GOOS and GOARCH are cleared because `go run` must build
# the command for the host even when the caller exports a foreign target.
#
# This is a parse-time assignment, so every target, clean and fmt included,
# needs a Go toolchain that compiles cmd/sourcedigest.
#
# $(shell) discards the command's exit status, so a failed digest would build
# binaries stamped "agentics-src-" with nothing after it. The command prints
# nothing on stdout when it fails, which makes an empty value the failure
# signal; $(or) turns it into a hard stop on any make, including the 3.81 macOS
# ships, where .SHELLSTATUS does not exist. The message names a literal
# `go run` so it still reads right when GO is overridden.
SOURCE_DIGEST := $(or $(shell GOOS= GOARCH= $(GO) run ./cmd/sourcedigest),$(error cmd/sourcedigest printed no digest; run `go run ./cmd/sourcedigest` in hooks/go to see why))

# -s -w drop the symbol table and DWARF. -trimpath keeps absolute build-machine
# paths out of the binary so independent release builds agree.
#
# -buildvcs=false is what makes `make verify` possible, and it is not optional.
# By default `go build` stamps the git revision, the commit time, and a
# dirty-tree flag into every main package. Three separate facts then leak into
# the artifact, and each one alone defeats a rebuild-and-compare:
#   - the revision differs for every commit, so the bytes change when no source
#     changed;
#   - the dirty flag records a working-tree state nobody can recreate later;
#   - a consumer builds from an install cache, which carries no .git at all, so
#     Go writes no stamp there and the bytes differ even at the right commit.
# With the flag off, the output depends only on the source, the toolchain, the
# flags, GOOS/GOARCH, and the GOAMD64/GOARM64 tuning pinned above. Two hosts
# running different operating systems then produce byte-identical artifacts
# for all six targets.
LDFLAGS   := -s -w -X main.sourceDigest=agentics-src-$(SOURCE_DIGEST)
BUILDOPTS := -trimpath -buildvcs=false -ldflags="$(LDFLAGS)"

BINARIES := \
	$(BIN_DIR)/agx-darwin-amd64 \
	$(BIN_DIR)/agx-darwin-arm64 \
	$(BIN_DIR)/agx-linux-amd64 \
	$(BIN_DIR)/agx-linux-arm64 \
	$(BIN_DIR)/agx-windows-amd64.exe \
	$(BIN_DIR)/agx-windows-arm64.exe

# The windows/amd64 agx launcher. Not in BINARIES: the manifest covers only
# the six hook binaries run-hook.cmd selects.
LAUNCHER := ../../bin/agx.exe

# The checksum manifest hooks/run-hook.cmd checks a downloaded binary against
# before it runs one. It lives beside the dispatcher, not in bin/, so a
# checkout that ships no binaries still carries it. The entries are relative
# to hooks/ so `sha256sum -c SHA256SUMS` works from that directory.

# The rebuild prerequisites are the files the digest hashes, read from
# hookprovenance.SourceFiles through `cmd/sourcedigest --list` rather than
# restated here. A second copy of the file set drifts from the digest's: a file
# the stamp covers but make does not watch leaves a stale binary behind
# "nothing to be done". Empty means the command failed, as for SOURCE_DIGEST
# above.
SOURCES := $(or $(shell GOOS= GOARCH= $(GO) run ./cmd/sourcedigest --list),$(error cmd/sourcedigest --list printed no files; run `go run ./cmd/sourcedigest --list` in hooks/go to see why))

.PHONY: all build-all manifest verify test mutation vet fmt bench bench-update bench-e2e clean

all: build-all

## build-all: cross-compile every contract target into bin/, build the launcher, and rewrite the manifest
build-all: $(BINARIES) $(LAUNCHER) manifest

# Always rewritten, never mtime-gated: a stale manifest that is newer than the
# binaries must still be replaced, because the conformance failure text sends
# the maintainer here. GNU coreutils on Linux, perl shasum on macOS; both print
# "<hex>  <path>". --text matters on Git Bash: MSYS sha256sum defaults to
# binary mode and writes "<hex> *<path>", which the manifest reader rejects.
# MSYS sha256sum has been seen writing `*bin/` even with --text; the sed folds
# it back so a Windows rebuild diffs clean.
#
# The hasher writes to a temp file first, then sed reads that file into
# SHA256SUMS. `set -o pipefail` is not portable to the /bin/sh make invokes,
# so a `hasher | sed > SHA256SUMS` pipeline would let a hasher failure exit 0
# (sed still succeeds on empty input) and silently truncate the manifest.
# Routing through a temp file makes the hasher's own exit status the recipe's
# exit status, via `set -e` below.
manifest: $(BINARIES)
	set -e; \
	cd $(HOOKS_DIR); \
	tmp="$$(mktemp)"; \
	trap 'rm -f "$$tmp"' EXIT; \
	if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum --text bin/agx-darwin-amd64 bin/agx-darwin-arm64 bin/agx-linux-amd64 bin/agx-linux-arm64 bin/agx-windows-amd64.exe bin/agx-windows-arm64.exe > "$$tmp"; \
	else \
		shasum -a 256 bin/agx-darwin-amd64 bin/agx-darwin-arm64 bin/agx-linux-amd64 bin/agx-linux-arm64 bin/agx-windows-amd64.exe bin/agx-windows-arm64.exe > "$$tmp"; \
	fi; \
	sed 's#  *\*bin/#  bin/#' "$$tmp" > SHA256SUMS

$(BIN_DIR)/agx-darwin-amd64: $(SOURCES)
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(BUILDOPTS) -o $@ $(CMD)

$(BIN_DIR)/agx-darwin-arm64: $(SOURCES)
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(BUILDOPTS) -o $@ $(CMD)

$(BIN_DIR)/agx-linux-amd64: $(SOURCES)
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(BUILDOPTS) -o $@ $(CMD)

$(BIN_DIR)/agx-linux-arm64: $(SOURCES)
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(BUILDOPTS) -o $@ $(CMD)

$(BIN_DIR)/agx-windows-amd64.exe: $(SOURCES)
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(BUILDOPTS) -o $@ $(CMD)

$(BIN_DIR)/agx-windows-arm64.exe: $(SOURCES)
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=arm64 $(GO) build $(BUILDOPTS) -o $@ $(CMD)

$(LAUNCHER): $(SOURCES)
	@mkdir -p $(dir $@)
	GOOS=windows GOARCH=amd64 $(GO) build $(BUILDOPTS) -o $@ ./cmd/launcher

## verify: rebuild every runtime and launcher, check the manifest and compare local copies
#
# The six hook builds must match hooks/SHA256SUMS. When a local hook binary or
# launcher exists, its bytes must also match the corresponding build.
#
# The launcher is rebuilt even in a source-only checkout. Its release digest
# is verified with the plugin archive because the runtime manifest does not
# list the launcher.
#
# It writes only to a temp directory. hooks/bin/ is never touched, so running
# it can never make a checkout stale.
#
# Cross-compilation means ONE host verifies EVERY binary. A consumer on
# macOS proves the Windows artifacts too.
#
# The toolchain guard runs first and on purpose. Every Go patch release
# produces different bytes from identical source, so a version mismatch shows
# up as a DIFFERS line per binary that looks like tampering and is not. go.mod pins
# the exact version; the guard reports a mismatch as a mismatch.
verify:
	@set -e; \
	pin="go$$(sed -n 's/^go[[:space:]]\{1,\}\([0-9][0-9.]*\)[[:space:]]*$$/\1/p' go.mod)"; \
	have="$$($(GO) version | awk '{print $$3}')"; \
	if [ "$$pin" = "go" ]; then \
		echo "verify: go.mod carries no usable \`go <version>\` directive" >&2; \
		exit 1; \
	fi; \
	if [ "$$have" != "$$pin" ]; then \
		echo "verify: go.mod pins $$pin, but the go command reports $$have." >&2; \
		echo "verify: a different Go patch release builds different bytes, so the" >&2; \
		echo "verify: comparison would fail for the wrong reason." >&2; \
		echo "verify: unset GOTOOLCHAIN (or set GOTOOLCHAIN=auto) and Go will fetch $$pin." >&2; \
		exit 1; \
	fi; \
	out="$$(mktemp -d)"; \
	trap 'rm -rf "$$out"' EXIT; \
	echo "source digest: $(SOURCE_DIGEST)"; \
	echo "toolchain:     $$have"; \
	echo; \
	status=0; \
	for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
		os="$${target%/*}"; arch="$${target#*/}"; ext=""; \
		if [ "$$os" = windows ]; then ext=".exe"; fi; \
		name="agx-$$os-$$arch$$ext"; \
		GOOS="$$os" GOARCH="$$arch" $(GO) build $(BUILDOPTS) -o "$$out/$$name" $(CMD); \
		want="$$(awk -v k="bin/$$name" '$$2 == k { print tolower($$1); exit }' $(MANIFEST))"; \
		have="$$( (sha256sum "$$out/$$name" 2>/dev/null || shasum -a 256 "$$out/$$name") | awk '{print $$1}')"; \
		if [ -z "$$want" ] || [ "$$have" != "$$want" ]; then \
			echo "DIFFERS    $$name (manifest)"; status=1; \
		elif [ -e "$(BIN_DIR)/$$name" ] && ! cmp -s "$$out/$$name" "$(BIN_DIR)/$$name"; then \
			echo "DIFFERS    $$name (hooks/bin)"; status=1; \
		else \
			echo "IDENTICAL  $$name"; \
		fi; \
	done; \
	GOOS=windows GOARCH=amd64 $(GO) build $(BUILDOPTS) -o "$$out/agx.exe" ./cmd/launcher; \
	if [ ! -e "$(LAUNCHER)" ] && [ ! -L "$(LAUNCHER)" ]; then \
		echo "BUILT      bin/agx.exe (no local copy)"; \
	elif ! cmp -s "$$out/agx.exe" "$(LAUNCHER)"; then \
		echo "DIFFERS    bin/agx.exe"; status=1; \
	else \
		echo "IDENTICAL  bin/agx.exe"; \
	fi; \
	echo; \
	if [ "$$status" -eq 0 ]; then \
		echo "Runtime builds match hooks/SHA256SUMS, and local copies match the builds."; \
	else \
		echo "At least one build does not match hooks/SHA256SUMS or a local copy." >&2; \
		echo "Do not run these hooks. Report it at https://github.com/muehz/agentics/issues" >&2; \
	fi; \
	exit $$status

## test: run the Go test suite
test:
	$(GO) test ./...

## mutation: run the gremlins baseline and gremlins on one package (PKG=./internal/<name>), as CI does
GREMLINS_VERSION ?= v0.6.0
mutation:
	@test -n "$(PKG)" || { echo "usage: make mutation PKG=./internal/<name>" >&2; exit 2; }
	$(GO) install github.com/go-gremlins/gremlins/cmd/gremlins@$(GREMLINS_VERSION)
	PATH="$$($(GO) env GOPATH)/bin:$$PATH" node ../../.github/tools/src/scripts/mutation-gremlins.ts --packages $(PKG)

## bench: compare each hook's latency with bench-baseline.txt (advisory; never run in CI).
# Fails past 2x the recorded ns/op. The numbers are machine-specific — the
# file's first line names the machine that recorded them.
bench:
	$(GO) test ./cmd/hook -run '^TestLatencyBaseline$$' -count=1 -v -latency

## bench-update: re-record bench-baseline.txt from this machine, deliberately.
bench-update:
	$(GO) test ./cmd/hook -run '^TestLatencyBaseline$$' -count=1 -latency -latency-update

## bench-e2e: capture packaged/config launch costs; real host scheduling needs separate evidence.
bench-e2e:
	$(GO) test ./cmd/hook -run '^TestHookCostCapture$$' -count=1 -timeout=45m -v -hook-cost $(HOOK_COST_ARGS)

## vet: run go vet
vet:
	$(GO) vet ./...

## fmt: rewrite Go sources with gofmt
fmt:
	$(GO) fmt ./...

## clean: remove build output
clean:
	rm -rf $(BIN_DIR)
	rm -f $(LAUNCHER)
