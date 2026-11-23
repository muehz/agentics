: << 'CMDBLOCK'
@echo off
setlocal DisableDelayedExpansion
REM Provider dispatch preserves the child's stdin and cmd.exe status. The
REM separate --provision-only interface prints one verified absolute path in
REM UTF-8 on success; callers must check the exit code before using stdout.
if "%~1"=="" (
    echo run-hook.cmd: missing provider >&2
    exit /b 1
)

set "HOOK_ROOT=%~dp0"
REM x64-emulated processes on Windows ARM64 report AMD64 in
REM PROCESSOR_ARCHITECTURE and leave PROCESSOR_ARCHITEW6432 unset;
REM PROCESSOR_IDENTIFIER still names the real CPU (e.g. "ARMv8 (64-bit)
REM Family 8 ..."), so it is checked too. The %%PROCESSOR_IDENTIFIER:~0,5%%
REM substring is guarded by "if defined" first - expanding it while unset
REM would not match ARMv8 here, but there is no reason to rely on that.
set "HOOK_ARCH=amd64"
if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "HOOK_ARCH=arm64"
if /i "%PROCESSOR_ARCHITEW6432%"=="ARM64" set "HOOK_ARCH=arm64"
if defined PROCESSOR_IDENTIFIER if /i "%PROCESSOR_IDENTIFIER:~0,5%"=="ARMv8" set "HOOK_ARCH=arm64"
set "HOOK_NAME=agx-windows-%HOOK_ARCH%.exe"
set "ComSpec=%SystemRoot%\System32\cmd.exe"
set "HOOK_PS=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"
if "%~1"=="--provision-only" goto provision_only
if "%~2"=="startup" goto startup
call :resolve_key
if not defined HOOK_KEY goto missing_binary
"%HOOK_EXE%" %*
set "HOOK_STATUS=%ERRORLEVEL%"
if /i "%~2"=="pre-tool" goto pre_tool_status
exit /b %HOOK_STATUS%

:resolve_key
REM Only an ASCII location key crosses the cmd code-page boundary. The nested
REM command expands the installed script path once, inside double quotes.
set "HOOK_KEY="
for /f "usebackq delims=" %%k in (`""%%HOOK_PS%%" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%%HOOK_ROOT%%fetch-hook.ps1" -Name %HOOK_NAME% -LocationKey"`) do set "HOOK_KEY=%%k"
if not defined HOOK_KEY exit /b 1
set "HOOK_EXE=%LOCALAPPDATA%\%HOOK_KEY%"
if "%HOOK_KEY%"=="installed" set "HOOK_EXE=%HOOK_ROOT%bin\%HOOK_NAME%"
exit /b 0

:startup
if not "%~3"=="" exit /b 0
call :resolve_key 2>nul
if not defined HOOK_KEY exit /b 0
set "AGX_SOURCE_DIGEST="
"%HOOK_EXE%" --startup-hint "%~1" "%HOOK_ROOT%.." 2>nul
exit /b 0

:provision_only
if not "%~2"=="" (
    echo agx: invoke --provision-only without hook arguments >&2
    exit /b 1
)
"%HOOK_PS%" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%HOOK_ROOT%fetch-hook.ps1" -Name %HOOK_NAME%
exit /b %ERRORLEVEL%

:pre_tool_status
if "%HOOK_STATUS%"=="0" exit /b 0
if /i "%~1"=="antigravity" (
    echo {"decision":"deny","reason":"agx: guard runtime unavailable - failing closed"}
    echo agx: guard runtime unavailable - failing closed >&2
    exit /b 0
)
if "%HOOK_STATUS%"=="2" exit /b 2
echo agx: guard runtime unavailable - failing closed >&2
exit /b 2

:missing_binary

REM No .exe: apply the failure posture directly. All hook behavior lives in
REM the committed binary, so no shell can substitute for it - a bash launched
REM here could only re-resolve the same missing .exe (the bash half below is
REM the primary Unix path, not a recovery route). Exit 2 is Claude Code's
REM blocking signal; only the guards may use it. Antigravity fails open on
REM non-zero hook exits, so its guards emit a deny JSON document on stdout
REM and exit 0.
if /i "%~2"=="pre-tool" goto missing_guard
if /i "%~2"=="command-guardian" goto missing_guard
if /i "%~2"=="release-guardian" goto missing_guard
if /i "%~2"=="secrets-guardian" goto missing_guard
exit /b 0

:missing_guard
if /i "%~1"=="antigravity" (
    echo {"decision":"deny","reason":"agx: guard binary missing - failing closed"}
    echo agx: guard binary missing - failing closed >&2
    exit /b 0
)
echo agx: guard binary missing - failing closed >&2
exit /b 2
CMDBLOCK

if [ "$#" -lt 1 ]; then
  echo "run-hook.cmd: missing provider" >&2
  exit 1
fi

missing_runtime() {
  if [ "${1:-}" = "--provision-only" ]; then
    echo "agx: cannot provision runtime; check the installed manifest, cache directory and release access, or preseed hooks/bin" >&2
    exit 1
  fi
  case "${2:-}" in
    pre-tool|command-guardian|release-guardian|secrets-guardian)
      if [ "${1:-}" = "antigravity" ]; then
        printf '%s\n' '{"decision":"deny","reason":"agx: guard runtime unavailable - failing closed"}'
        echo "agx: guard runtime unavailable - failing closed" >&2
        exit 0
      fi
      echo "agx: guard runtime unavailable - failing closed" >&2
      exit 2 ;;
  esac
  exit 0
}

# POSIX hosts may enter through /bin/sh; Bash initializes the platform values.
if [ -z "${BASH_VERSION:-}" ]; then
  if command -v bash >/dev/null 2>&1 && system_bash="$(command -p -v bash)"; then
    exec "$system_bash" "$0" "$@"
  fi
  missing_runtime "$@"
fi

# The script directory by parameter expansion: no subshell, no dirname.
# A bare "$0" (invoked as `bash run-hook.cmd`) has no slash and means the
# working directory; every host passes an absolute path.
case "$0" in
  */*) SCRIPT_DIR="${0%/*}" ;;
  *)   SCRIPT_DIR="." ;;
esac

if [ "$1" = "--provision-only" ] && [ "$#" -ne 1 ]; then
  echo "agx: invoke --provision-only without hook arguments" >&2
  exit 1
fi
SCRIPT_DIR="$(CDPATH= cd -P -- "$SCRIPT_DIR" && pwd -P)" || missing_runtime "$@"

# Bash initializes these values without launching a platform probe.
case "${OSTYPE:-}" in
  darwin*)                HOOK_OS="darwin" ;;
  linux*)                 HOOK_OS="linux" ;;
  mingw*|msys*|cygwin*)   HOOK_OS="windows" ;;
  *)                      HOOK_OS="" ;;
esac
case "${HOSTTYPE:-${MACHTYPE%%-*}}" in
  arm64|aarch64)          HOOK_ARCH="arm64" ;;
  x86_64|amd64)           HOOK_ARCH="amd64" ;;
  *)                      HOOK_ARCH="" ;;
esac

# An x64-emulated shell on Windows ARM64 reports x86_64, and Rosetta does
# the same on Apple Silicon: prefer the native binary in both cases.
if [ "$HOOK_ARCH" = "amd64" ]; then
  case "$HOOK_OS" in
    windows)
      case "${PROCESSOR_ARCHITECTURE:-}:${PROCESSOR_ARCHITEW6432:-}:${PROCESSOR_IDENTIFIER:-}" in
        *ARM64*|*ARMv8*) HOOK_ARCH="arm64" ;;
      esac ;;
    darwin)
      if [ "$(/usr/sbin/sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
        HOOK_ARCH="arm64"
      fi ;;
  esac
fi

# Hash stdin so that whitespace and quoting in paths never enter hash output.
verified() {
  [ ! -L "$1" ] && [ -f "$1" ] && [ -x "$1" ] || return 1
  matches_digest "$1"
}

matches_digest() {
  local got
  got="$(command -p "${hash_tool[@]}" < "$1")" || return 1
  [ "${got%% *}" = "$want" ]
}

# Walk one component at a time: mkdir -p on the complete destination would
# follow an existing symlink before we could inspect it.
cache_directory() {
  local dir="$cache_root" component
  if [ "$1" = create ]; then
    (umask 077 && command -p mkdir -p -- "$dir") || return 1
  fi
  for component in agentics "$version" "$want"; do
    dir="$dir/$component"
    [ ! -L "$dir" ] || return 1
    if [ "$1" = create ] && [ ! -d "$dir" ]; then
      (umask 077 && command -p mkdir -- "$dir") 2>/dev/null || [ -d "$dir" ] || return 1
    fi
    [ ! -L "$dir" ] && [ -d "$dir" ] || return 1
  done
}

download() (
  local part url
  part="$(command -p mktemp "$dir/.$name.part.XXXXXXXX")" || return 1
  trap 'command -p rm -f -- "$part"' EXIT
  url="${AGX_RELEASE_URL:-https://github.com/muehz/agentics/releases/download}/v$version/$name"
  [ -z "${AGX_DEBUG:-}" ] || echo "run-hook fetching: $url" >&2
  command -p curl -fsL --connect-timeout 3 --max-time 8 -o "$part" -- "$url" >/dev/null || return 1
  if ! matches_digest "$part"; then
    echo "agx: downloaded $name does not match hooks/SHA256SUMS - discarded" >&2
    return 1
  fi
  cache_directory check && [ ! -L "$dest" ] && { [ ! -e "$dest" ] || [ -f "$dest" ]; } || return 1
  command -p chmod 0700 "$part" && command -p mv -f -- "$part" "$dest"
)

resolve_runtime() {
  local name want version installed cache_root dir dest
  local -a hash_tool
  [ -n "$HOOK_OS" ] && [ -n "$HOOK_ARCH" ] || return 1
  name="agx-$HOOK_OS-$HOOK_ARCH"
  [ "$HOOK_OS" != windows ] || name="$name.exe"
  # Never take the verifier, manifest reader or publisher from a workspace PATH.
  # Honor a host without a configured hash utility before opening the network.
  if command -v sha256sum >/dev/null 2>&1 && command -p -v sha256sum >/dev/null; then
    hash_tool=(sha256sum)
  elif command -v shasum >/dev/null 2>&1 && command -p -v shasum >/dev/null; then
    hash_tool=(shasum -a 256)
  else
    return 1
  fi
  [ ! -L "$SCRIPT_DIR/SHA256SUMS" ] && [ -f "$SCRIPT_DIR/SHA256SUMS" ] || return 1
  local metadata="$SCRIPT_DIR/../.claude-plugin"
  [ ! -L "$metadata" ] && [ ! -L "$metadata/plugin.json" ] && [ -f "$metadata/plugin.json" ] || return 1
  want="$(command -p awk -v file="bin/$name" '$2 == file {print tolower($1); exit}' "$SCRIPT_DIR/SHA256SUMS")" || return 1
  [[ "$want" =~ ^[0-9a-f]{64}$ ]] || return 1
  version="$(command -p sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$metadata/plugin.json")" || return 1
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9-]+([.+-][A-Za-z0-9-]+)*)?$ ]] || return 1
  installed="$SCRIPT_DIR/bin/$name"
  if [ ! -L "$SCRIPT_DIR/bin" ] && verified "$installed"; then
    HOOK_BIN="$installed"
    return 0
  fi

  case "$HOOK_OS" in
    darwin)  cache_root="${HOME:+$HOME/Library/Caches}" ;;
    windows) cache_root="${LOCALAPPDATA:-}"
             if [ -n "$cache_root" ]; then cache_root="$(command -p cygpath -u -- "$cache_root")" || return 1; fi ;;
    linux)   cache_root="${XDG_CACHE_HOME:-${HOME:+$HOME/.cache}}" ;;
  esac
  [[ "$cache_root" = /* && "$cache_root" != *$'\n'* && "$cache_root" != *$'\r'* ]] || return 1
  dir="${cache_root%/}/agentics/$version/$want"
  dest="$dir/$name"
  if ! cache_directory check || ! verified "$dest"; then
    [ -z "${AGX_NO_FETCH:-}" ] && cache_directory create || return 1
    [ ! -L "$dest" ] && { [ ! -e "$dest" ] || [ -f "$dest" ]; } || return 1
    download || return 1
    cache_directory check && verified "$dest" || return 1
  fi
  HOOK_BIN="$dest"
}

print_runtime_path() {
  # RFC 3629 restricts the second byte after E0, ED, F0 and F4. Match
  # bytes in the C locale regardless of the invoking terminal's encoding.
  local LC_ALL=C
  local scalar=$'[\x01-\x7f]|[\xc2-\xdf][\x80-\xbf]'
  scalar+=$'|(\xe0[\xa0-\xbf]|[\xe1-\xec\xee-\xef][\x80-\xbf]|\xed[\x80-\x9f])[\x80-\xbf]'
  scalar+=$'|(\xf0[\x90-\xbf]|[\xf1-\xf3][\x80-\xbf]|\xf4[\x80-\x8f])[\x80-\xbf]{2}'
  local path_pattern="^($scalar)+$"
  if [[ "$1" = *$'\n'* || "$1" = *$'\r'* || ! "$1" =~ $path_pattern ]]; then
    echo "agx: use a single-line UTF-8 path for the installed plugin or runtime cache" >&2
    return 1
  fi
  printf '%s\n' "$1"
}

if [ "${2:-}" = startup ]; then
  if [ "$#" -eq 2 ] && resolve_runtime 2>/dev/null; then
    AGX_SOURCE_DIGEST= "$HOOK_BIN" --startup-hint "$1" "${SCRIPT_DIR%/*}" 2>/dev/null
  fi
  exit 0
fi

if resolve_runtime; then
  if [ "$1" = "--provision-only" ]; then
    print_runtime_path "$HOOK_BIN"
    exit $?
  fi
  [ -n "${AGX_DEBUG:-}" ] && echo "run-hook resolved binary: $HOOK_BIN" >&2
  # Antigravity fails open on non-zero hook exits, so grouped pre-tool runs as
  # a child process and converts any non-zero exit (launch failure, crash, or
  # usage error) into a fail-closed deny document on exit 0.
  if [ "${1:-}" = "antigravity" ] && [ "${2:-}" = "pre-tool" ]; then
    if "$HOOK_BIN" "$@"; then
      exit 0
    fi
    missing_runtime "$@"
  fi
  # A present executable can still lack its interpreter. Only grouped events
  # recover a failed exec; a successful exec preserves the child's status.
  case "${2:-}" in
    pre-tool) shopt -s execfail ;;
  esac
  exec "$HOOK_BIN" "$@"
  missing_runtime "$@"
fi

# Failure posture. The binary is missing for this platform and the fetch did
# not produce one (or the platform is unrecognized). All three guards fail CLOSED: a missing binary must never silently
# disable a guard. Exit 2 blocks on Claude Code, Copilot CLI, and Codex;
# Antigravity fails open on non-zero exits, so missing_runtime emits a deny
# JSON decision document on stdout and exits 0.
#
# secrets-guardian prevents a credential reaching the transcript, and
# release-guardian prevents an unauthorized deploy. Neither can be undone by a
# later hook.
missing_runtime "$@"
