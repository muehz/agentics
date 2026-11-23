#!/usr/bin/env bash
#
# Deterministic digest of the Go source tree that builds the hook binary:
# go.mod, the digest-exclude list when present, and every non-test .go file in
# this module outside bin/ and outside each directory that list names (a
# _test.go file never reaches the built binary, so it is excluded), hashed as
# (path, content-hash) pairs in C-locale sort order, then hashed once more.
# Prints the 64-hex-char digest on stdout.
#
# The walk covers the whole module rather than naming the directories to hash.
# An earlier version enumerated `cmd` and `internal`, so a package that moved
# out of those two roots silently stopped being hashed while the digest still
# computed, still matched, and still vouched for a binary built from source it
# no longer covered. Walking the whole module keeps that failure mode out:
# source added or moved anywhere is covered automatically. What is skipped is
# named instead: bin/, which holds the build outputs, and the directories in
# digest-exclude, whose code ships in no hook binary.
# TestDigestExcludeMatchesClosure checks that list against what the binaries
# import, and the list file itself is hashed, so editing it changes the digest.
#
# Two consumers must agree on this value byte-for-byte:
#   - hooks/go/Makefile stamps "agentics-src-<digest>" into every artifact via
#     -ldflags "-X main.sourceDigest=..."
#   - the conformance suite's `hooks freshness` check
#     (hooks/go/internal/conformance) recomputes it from the checkout and
#     scans each committed binary's bytes for the stamp, which is how the
#     three platform binaries CI cannot execute are verified.
#
# Usage: source-digest.sh [--list] [dir]
#
# An optional dir digests a different source root (the freshness guard's
# self-test uses this to prove a mutated copy digests differently). --list,
# accepted only as the first argument, prints the hashed file set instead of
# the digest: one module-relative path per line, C-locale sorted. The Makefile
# uses it as the hook binaries' prerequisites, so a rebuild follows exactly the
# files the stamp vouches for. A malformed digest-exclude fails both modes
# with nothing on stdout.

set -euo pipefail
# No pathname expansion: the exclude entries below are split unquoted, and an
# entry must match itself, never as a glob.
set -f

list_mode=false
case "${1-}" in
  --list)
    list_mode=true
    shift
    ;;
  -*)
    echo "source-digest.sh: unknown option ${1}; usage: source-digest.sh [--list] [dir]" >&2
    exit 2
    ;;
esac

cd "${1:-$(dirname "$0")}"

hash_stream() {
  # GNU coreutils on Linux, perl shasum on macOS — same algorithm, same hex.
  # The line is pinned to the GNU/Linux stdin rendering, "<hex>  -", which is
  # what the committed stamps embed and what the Go reimplementation writes.
  # MSYS coreutils on Windows print "<hex> *-" for the binary-mode stdin read,
  # and left as-is that one byte changed every digest computed there.
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1 "  -"}'
  else
    shasum -a 256 | awk '{print $1 "  -"}'
  fi
}

# hookprovenance.ExcludeList owns the list rules; this reader applies the same
# checks in the same order so both reject a line for the same reason. Reading
# happens here, outside any pipeline, so a malformed line stops the script
# before anything reaches stdout.
list_error() {
  printf 'source-digest.sh: digest-exclude line %d "%s" %s\n' "$lineno" "$line" "$1" >&2
  exit 1
}

# Space-separated: a valid entry never contains whitespace.
excludes=
if [ -e digest-exclude ]; then
  lineno=0
  # The || clause keeps a final line that has no trailing newline.
  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$((lineno + 1))
    case "$line" in
      *$'\r'*) list_error "carries a carriage return" ;;
      '' | '#'*) continue ;;
      *' '* | *$'\t'*) list_error "contains whitespace" ;;
      *\\*) list_error "contains a backslash; separate with /" ;;
      *:* | /*) list_error "is absolute; name a directory relative to the module root" ;;
    esac
    case "/$line/" in
      */../*) list_error "contains a .. element" ;;
      *//* | */./*) list_error "is not a clean path (no ., //, leading ./ or trailing /)" ;;
    esac
    [ -d "$line" ] || list_error "names no directory under the module root"
    excludes="$excludes $line"
  done <digest-exclude
fi

# The glob excludes *_test.go DELIBERATELY: a test file never reaches the
# built binary, so it must not affect the digest that vouches for it. The
# reimplementation in hooks/go/internal/hookprovenance/provenance.go
# (hookprovenance.SourceFiles and SourceDigest) is pinned to this file set by
# TestHookToolDigestParity, which shells out to this real script and asserts
# byte parity for both the digest and --list. Narrowing or widening the file
# set here alone breaks that test; change the two together.
source_files() {
  echo go.mod
  if [ -e digest-exclude ]; then
    echo digest-exclude
  fi
  find . -name '*.go' -not -path './bin/*' -not -name '*_test.go' | sed 's|^\./||' |
    while IFS= read -r file; do
      # A quoted entry matches literally, and the trailing / matches whole
      # path components only, so a listed a/b never excludes a/bc.
      for entry in $excludes; do
        case "$file" in "$entry"/*) continue 2 ;; esac
      done
      printf '%s\n' "$file"
    done
}

if [ "$list_mode" = true ]; then
  source_files | LC_ALL=C sort
  exit 0
fi

{
  source_files | LC_ALL=C sort |
    while IFS= read -r file; do
      printf '%s %s\n' "$file" "$(hash_stream <"$file")"
    done
} | hash_stream | awk '{print $1}'
