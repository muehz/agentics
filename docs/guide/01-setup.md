# 01. Setup

Install Agentics from a versioned release package. It contains the plugin manifests, skills, hook adapters, and `agx`
launchers. A source checkout can build a development install with
[`npm run plugin:setup`](../../CONTRIBUTING.md#local-setup).

## Get the release files

Choose a tag on the [official GitHub Releases page](https://github.com/muehz/agentics/releases). Download
`agentics-plugin-<tag>.tar.gz` and `SHA256SUMS` from that same tagged release into an empty directory. Substitute the
actual tag, including its leading `v`, in the commands below. For offline installation, bring the checksum file from the
official release or use a copy whose archive digest was independently authenticated before going offline.

The launcher and hooks can provision the matching runtime at first use when they can access the release asset. For an
offline install or an asset that requires sign-in, also bring the runtime for the operating system and architecture
where the tool will run. Choose the asset by its exact release filename.

| Host                | Runtime asset           |
| ------------------- | ----------------------- |
| Intel macOS         | `agx-darwin-amd64`      |
| Apple Silicon macOS | `agx-darwin-arm64`      |
| x64 Linux           | `agx-linux-amd64`       |
| ARM64 Linux         | `agx-linux-arm64`       |
| x64 Windows         | `agx-windows-amd64.exe` |
| ARM64 Windows       | `agx-windows-arm64.exe` |

## Verify and unpack

Run the block for your shell from the download directory. Each block requires one exact archive filename entry and
checks its bytes before extraction. For an offline install, set `runtime` or `$runtime` to the downloaded runtime
filename. The block checks that file against the manifest inside the verified package and copies it to `hooks/bin/`
before registration. Keep the `verified` directory as long as a tool uses its local package path.

### Bash

```bash
(
  set -eu
  tag=v1.230.0  # Replace with the chosen release tag.
  runtime=''    # For offline use, enter the matching runtime filename.
  archive="agentics-plugin-$tag.tar.gz"

  verify_asset() {
    expected=$(awk -v name="$2" '
      $2 == name || $2 == "*" name {
        count++
        if (length($1) == 64 && $1 !~ /[^0-9a-f]/ && $0 == $1 "  " name) digest = $1
      }
      END {
        if (count != 1 || !digest) exit 1
        print digest
      }
    ' "$1") || { printf 'Missing, duplicate, or invalid checksum for %s\n' "$2" >&2; return 1; }
    if command -v sha256sum >/dev/null 2>&1; then
      result=$(sha256sum "$3") || return 1
    else
      result=$(shasum -a 256 "$3") || return 1
    fi
    [ "${result%% *}" = "$expected" ] || { printf 'Checksum mismatch for %s\n' "$2" >&2; return 1; }
  }

  verify_asset SHA256SUMS "$archive" "$archive"
  mkdir verified
  tar -xzf "$archive" -C verified
  package="verified/agentics-plugin-$tag"
  if [ -n "$runtime" ]; then
    verify_asset "$package/hooks/SHA256SUMS" "bin/$runtime" "$runtime"
    mkdir -p "$package/hooks/bin"
    cp "$runtime" "$package/hooks/bin/$runtime"
    chmod 0755 "$package/hooks/bin/$runtime"
  fi
  printf 'Verified package: %s/%s\n' "$PWD" "$package"
)
```

### PowerShell

```powershell
& {
    $ErrorActionPreference = 'Stop'
    $tag = 'v1.230.0'  # Replace with the chosen release tag.
    $runtime = ''     # For offline use, enter the matching runtime filename.
    $archive = "agentics-plugin-$tag.tar.gz"

    function Confirm-Asset($manifest, $name, $file) {
        $escaped = [regex]::Escape($name)
        $entries = @(Get-Content -LiteralPath $manifest | Where-Object {
            $_ -cmatch "^[^\s]+\s+\*?$escaped(?:\s|$)"
        })
        if ($entries.Count -ne 1) { throw "Missing or duplicate checksum for $name" }
        $match = [regex]::Match($entries[0], "^([0-9a-f]{64})  $escaped$")
        if (!$match.Success) { throw "Invalid checksum for $name" }
        $expected = $match.Groups[1].Value
        $actual = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -cne $expected) { throw "Checksum mismatch for $name" }
    }

    Confirm-Asset 'SHA256SUMS' $archive $archive
    New-Item -ItemType Directory -Path verified | Out-Null
    tar.exe -xzf $archive -C verified
    if ($LASTEXITCODE -ne 0) { throw 'Package extraction failed' }
    $package = Join-Path verified "agentics-plugin-$tag"
    if ($runtime) {
        Confirm-Asset (Join-Path $package 'hooks/SHA256SUMS') "bin/$runtime" $runtime
        $bin = Join-Path $package 'hooks/bin'
        New-Item -ItemType Directory -Path $bin -Force | Out-Null
        Copy-Item -LiteralPath $runtime -Destination (Join-Path $bin $runtime)
    }
    Write-Output "Verified package: $(Resolve-Path -LiteralPath $package)"
}
```

Proceed to registration when the block prints `Verified package`. If a check fails, obtain the correct files and rerun
from a fresh directory. For offline use, set `AGX_NO_FETCH=offline` in the environment that starts the coding tool and
in any terminal that runs `agx`. After registration, use the tool's installed plugin root or the original package path
when the tool loads that path live. Check that the preseed is at `<plugin-root>/hooks/bin/<runtime-asset>` and run
`<plugin-root>/bin/agx help` on POSIX or `& '<plugin-root>\bin\agx.exe' help` in Windows PowerShell.

## Register on your coding tool

Open the page for your tool and substitute the absolute `Verified package` path from the preceding block.

- [Claude Code](../surfaces/claude-code.md#install-and-discover)
- [Copilot CLI](../surfaces/copilot-cli.md#install-and-discover)
- [Codex](../surfaces/codex.md#local-install)
- [Antigravity](../surfaces/antigravity.md#install-and-discover)
- [VS Code Copilot Chat](../surfaces/vscode.md#configuration)

The [Cursor plugin](../surfaces/cursor.md) was removed.

## Run agx directly

Use the absolute launcher path from the startup hint when your tool delivers it. You can also find the installed plugin
root in your tool's plugin inventory. Run `'<plugin-root>/bin/agx' help` from Bash or
`& '<plugin-root>\bin\agx.exe' help` from Windows PowerShell. Use that path if the tool cannot find bare `agx`.

When `hooks/bin/` has no matching runtime, the launcher and dispatcher look for verified bytes in the shared cache at
`<cache-root>/agentics/<manifest-version>/<runtime-sha256>/<runtime-asset>`. They fetch only that platform asset if it
is missing and fetching is enabled. The cache root is `~/Library/Caches` on macOS, `$XDG_CACHE_HOME` or `~/.cache` on
Linux, and `$env:LOCALAPPDATA` on Windows.

## Verify

On any surface, send:

```text
Use Agentics routing to help me set up TDD for a new TypeScript service.
```

Why this phrasing works: "Agentics routing" exercises routing, Agentics's automatic entrance, and "set up TDD" names the
[`tdd`](../../skills/tdd/SKILL.md) skill. A test-first ask with no approved plan starts at `intent`, and inside a plan
`execute` applies `tdd` to each task. A correct install therefore answers by starting `intent` for the change and saying
`execute` will apply `tdd` to each task, not with a generic testing essay. If you get a generic essay, the response does
not establish that Agentics loaded. Re-check the inventory and runtime evidence described for your surface (on
Antigravity, check `agy plugin list --json` and invoke skills as `/<skill-name>`). The VS Code page distinguishes the
maintainer's Local-harness startup report from a package-load check on your harness.

## Trust the hooks (Codex)

Agentics ships three guard hooks (page [07. Hooks](07-hooks.md) covers what they do). On Codex, restart after install
and run:

```text
/hooks
```

Review and trust the Agentics hooks there. Until you do, they will not run automatically. Claude Code and Copilot CLI
load the hook configs declared in the plugin manifest without a separate trust step.

**Pitfall:** don't stop at "the install command exited 0." The verify prompt above is the actual test. It proves the
skills loaded and that routing selects the workflow. An install that lists the plugin but answers generically is the
classic symptom of a wrong surface path or a skipped restart.

Next: [02. Routing](02-routing.md)
