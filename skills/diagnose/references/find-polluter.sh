#!/usr/bin/env bash
# Bisection script to find which test creates unwanted files/state
#
# Usage: find-polluter.sh <file_or_dir_to_check> <test_pattern>
# Example: find-polluter.sh 'packages/core/leaked-state' 'src/**/*.test.ts'
#
# Run it from the project root and quote both arguments so your shell does not expand them.
#
# The artifact must not exist yet: a path that already exists cannot show which test creates it.
#
# <test_pattern> is matched against paths relative to the current directory; a leading `./` is
# optional. `**/` matches any number of directories, including none, so `src/**/a.test.ts` finds
# both `src/a.test.ts` and `src/sub/a.test.ts`. `*` and `?` match within one path segment, so
# `src/*.test.ts` does not find nested tests. Every other character, including `[`, `]`, `{` and
# `}`, matches itself. node_modules is skipped.
#
# Each test file runs on its own as `npm test <file>`. For a project that uses another package
# manager, set FIND_POLLUTER_RUNNER to npm, pnpm or yarn; the script then runs `<runner> test <file>`.
#   FIND_POLLUTER_RUNNER=pnpm find-polluter.sh 'tmp/out.txt' 'src/**/*.test.ts'
#
# Exit codes:
#   0  no polluter found, and every test run exited 0
#   1  a polluter was found
#   2  cannot diagnose: bad arguments, the artifact already exists, no test matched, the runner could
#      not start, or the run was interrupted
#   3  no polluter found, but some test runs exited non-zero or were killed, so those tests may have
#      stopped before they could create the artifact. This is not a clean result.

set -e

if [ $# -ne 2 ]; then
  echo "Usage: $0 <file_to_check> <test_pattern>"
  echo "Example: $0 'packages/core/leaked-state' 'src/**/*.test.ts'"
  exit 2
fi

POLLUTION_CHECK="$1"
TEST_PATTERN="$2"
RUNNER="${FIND_POLLUTER_RUNNER:-npm}"

case "$RUNNER" in
  npm | pnpm | yarn) ;;
  *)
    echo "Cannot diagnose: FIND_POLLUTER_RUNNER must be npm, pnpm or yarn, not: $RUNNER" >&2
    exit 2
    ;;
esac

if [ -e "$POLLUTION_CHECK" ] || [ -L "$POLLUTION_CHECK" ]; then
  echo "Cannot diagnose: pollution artifact already exists: $POLLUTION_CHECK" >&2
  exit 2
fi

echo "🔍 Searching for test that creates: $POLLUTION_CHECK"
echo "Test pattern: $TEST_PATTERN"
echo ""

# find has no `**`, and the `*` of find -path crosses directories, so neither can express the
# pattern. Translate it to an anchored extended regular expression and filter find's output.
REST="${TEST_PATTERN#./}"
REGEX='^\./'
PREV=/
while [ -n "$REST" ]; do
  CHAR="${REST%"${REST#?}"}"
  case "$REST" in
    '**/'*)
      # `**/` is a globstar only at the start of a path segment.
      if [ "$PREV" = / ]; then
        REGEX="$REGEX(.*/)?"
        REST="${REST#??}"
        CHAR=/
      else
        REGEX="${REGEX}[^/]*"
      fi
      ;;
    '**'*)
      if [ "$PREV" = / ]; then
        REGEX="$REGEX.*"
        REST="${REST#?}"
      else
        REGEX="${REGEX}[^/]*"
      fi
      ;;
    '*'*) REGEX="${REGEX}[^/]*" ;;
    '?'*) REGEX="${REGEX}[^/]" ;;
    [.+\|^\$\(\)\{\\]*) REGEX="$REGEX\\$CHAR" ;;
    '['*) REGEX="$REGEX\\[" ;;
    *) REGEX="$REGEX$CHAR" ;;
  esac
  PREV="$CHAR"
  REST="${REST#?}"
done
REGEX="$REGEX\$"

# Get list of test files
TEST_FILES=$(find . -type f -not -path '*/node_modules/*' | grep -E -e "$REGEX" | sort)
if [ -z "$TEST_FILES" ]; then
  echo "Cannot diagnose: no tests match: $TEST_PATTERN" >&2
  exit 2
fi
TOTAL=$(printf '%s\n' "$TEST_FILES" | wc -l | tr -d ' ')

echo "Found $TOTAL test files"
echo ""

COUNT=0
FAILED=0
FAILED_FILES=""
while IFS= read -r TEST_FILE; do
  COUNT=$((COUNT + 1))

  if [ -e "$POLLUTION_CHECK" ] || [ -L "$POLLUTION_CHECK" ]; then
    echo "Cannot diagnose: pollution artifact appeared before test: $TEST_FILE" >&2
    exit 2
  fi

  echo "[$COUNT/$TOTAL] Testing: $TEST_FILE"

  # A failing test can still create the artifact, so a non-zero exit does not stop the search.
  # Stdin comes from /dev/null so the runner cannot consume the list of test files.
  STATUS=0
  case "$RUNNER" in
    npm) OUTPUT=$(npm test "$TEST_FILE" 2>&1 < /dev/null) || STATUS=$? ;;
    pnpm) OUTPUT=$(pnpm test "$TEST_FILE" 2>&1 < /dev/null) || STATUS=$? ;;
    yarn) OUTPUT=$(yarn test "$TEST_FILE" 2>&1 < /dev/null) || STATUS=$? ;;
  esac

  # Check if pollution appeared
  if [ -e "$POLLUTION_CHECK" ] || [ -L "$POLLUTION_CHECK" ]; then
    echo ""
    echo "🎯 FOUND POLLUTER!"
    echo "   Test: $TEST_FILE"
    echo "   Created: $POLLUTION_CHECK"
    echo ""
    echo "Pollution details:"
    ls -la "$POLLUTION_CHECK"
    echo ""
    echo "To investigate:"
    echo "  $RUNNER test \"$TEST_FILE\"    # Run just this test"
    echo "  cat \"$TEST_FILE\"         # Review test code"
    exit 1
  fi

  # 126 and 127 mean the shell could not start the runner, so no test ran. 130 is an interrupt,
  # which must stop the search instead of moving on to the next test. Any other status,
  # including one from a test that a signal killed, is a test run that failed.
  case "$STATUS" in
    126 | 127)
      echo "" >&2
      echo "Cannot diagnose: test runner failed to run (exit $STATUS): $RUNNER test $TEST_FILE" >&2
      printf '%s\n' "$OUTPUT" | tail -n 20 >&2
      exit 2
      ;;
    130)
      echo "" >&2
      echo "Cannot diagnose: interrupted (exit $STATUS) during: $RUNNER test $TEST_FILE" >&2
      exit 2
      ;;
  esac

  if [ "$STATUS" -ne 0 ]; then
    FAILED=$((FAILED + 1))
    FAILED_FILES="$FAILED_FILES
   $TEST_FILE (exit $STATUS)"
  fi
done <<EOF
$TEST_FILES
EOF

echo ""
if [ "$FAILED" -ne 0 ]; then
  echo "No polluter found, but $FAILED of $TOTAL test runs exited non-zero:$FAILED_FILES" >&2
  echo "A test that stops early may never reach the code that creates $POLLUTION_CHECK." >&2
  echo "Run one of them directly to see why it failed: $RUNNER test <test file>" >&2
  exit 3
fi

echo "✅ No polluter found - all tests clean!"
exit 0
