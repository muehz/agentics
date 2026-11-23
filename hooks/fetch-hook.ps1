# Resolve the installed runtime or publish a verified release asset to the
# user cache. The public output protocol is one absolute UTF-8 path plus LF.
# cmd.exe asks for an ASCII key so its active code page cannot corrupt a path.
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('agx-windows-amd64.exe', 'agx-windows-arm64.exe')]
    [string]$Name,
    [switch]$LocationKey
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
# Providers may pass a PowerShell 7 or workspace module path to Windows
# PowerShell 5.1. Resolve its JSON and web cmdlets from its own installation.
$env:PSModulePath = [IO.Path]::Combine($PSHOME, 'Modules')

function Get-Attributes($path) {
    try { return [IO.File]::GetAttributes($path) }
    catch [IO.FileNotFoundException] { return $null }
    catch [IO.DirectoryNotFoundException] { return $null }
}

function Test-Digest($path) {
    $attributes = Get-Attributes $path
    if ($null -eq $attributes -or ($attributes -band ([IO.FileAttributes]::Directory -bor [IO.FileAttributes]::ReparsePoint))) { return $false }
    # A locked or unreadable preseed is unusable; a verified cache entry can
    # still serve the invocation.
    try { $stream = [IO.File]::Open($path, 'Open', 'Read', ([IO.FileShare]::Read -bor [IO.FileShare]::Delete)) }
    catch [IO.IOException] { return $false }
    catch [UnauthorizedAccessException] { return $false }
    try {
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $got = [BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-', '') }
        finally { $sha.Dispose() }
    } finally { $stream.Dispose() }
    return $got -eq $want
}

function Write-Resolution($path, $key) {
    if ($env:AGX_DEBUG) { [Console]::Error.WriteLine("run-hook resolved binary: $path") }
    $value = if ($LocationKey) { $key } else { $path }
    # OpenStandardOutput avoids both the OEM code page and a UTF-8 BOM.
    $bytes = [Text.Encoding]::UTF8.GetBytes($value + [char]10)
    [Console]::OpenStandardOutput().Write($bytes, 0, $bytes.Length)
}

function Get-CacheDirectory([bool]$create) {
    $dir = $cacheRoot
    if ($create) { [void][IO.Directory]::CreateDirectory($dir) }
    foreach ($component in 'agentics', $version, $want) {
        $dir = [IO.Path]::Combine($dir, $component)
        $attributes = Get-Attributes $dir
        if ($null -eq $attributes -and $create) {
            [void][IO.Directory]::CreateDirectory($dir)
            $attributes = Get-Attributes $dir
        }
        if ($null -eq $attributes) { return $null }
        if (($attributes -band [IO.FileAttributes]::ReparsePoint) -or -not ($attributes -band [IO.FileAttributes]::Directory)) {
            throw 'unsafe runtime cache directory'
        }
    }
    return $dir
}

$part = $null
try {
    $manifest = [IO.Path]::Combine($PSScriptRoot, 'SHA256SUMS')
    $metadataDir = [IO.Path]::Combine($PSScriptRoot, '..\.claude-plugin')
    $meta = [IO.Path]::Combine($metadataDir, 'plugin.json')
    foreach ($path in $manifest, $metadataDir, $meta) {
        if ((Get-Attributes $path) -band [IO.FileAttributes]::ReparsePoint) { throw 'unsafe installed verification metadata' }
    }
    $want = $null
    foreach ($line in [IO.File]::ReadLines($manifest)) {
        $fields = ($line -replace '^[ \t]+', '') -split '[ \t]+'
        if ($fields.Length -ge 2 -and $fields[1] -ceq "bin/$Name") {
            $want = $fields[0].ToLowerInvariant()
            break
        }
    }
    if ($want -cnotmatch '\A[0-9a-f]{64}\z') { throw 'invalid checksum in hooks/SHA256SUMS' }
    $version = ([IO.File]::ReadAllText($meta) | ConvertFrom-Json).version
    if ($version -isnot [string] -or $version -cnotmatch '\A[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9-]+([.+-][A-Za-z0-9-]+)*)?\z') {
        throw 'invalid version in installed plugin metadata'
    }

    $bin = [IO.Path]::Combine($PSScriptRoot, 'bin')
    $installed = [IO.Path]::Combine($bin, $Name)
    if (-not ((Get-Attributes $bin) -band [IO.FileAttributes]::ReparsePoint) -and (Test-Digest $installed)) {
        Write-Resolution $installed 'installed'
        exit 0
    }
    $cacheRoot = $env:LOCALAPPDATA
    if (!$cacheRoot -or $cacheRoot -notmatch '\A([A-Za-z]:[\\/]|\\\\[^\\]+\\[^\\]+)') {
        throw 'set LOCALAPPDATA to an absolute user cache directory'
    }
    $key = "agentics\$version\$want\$Name"
    $dest = [IO.Path]::Combine($cacheRoot, $key)
    $dir = Get-CacheDirectory $false
    if ($dir -and (Test-Digest $dest)) {
        Write-Resolution $dest $key
        exit 0
    }
    if ($env:AGX_NO_FETCH) { throw 'fetch disabled; preseed the matching runtime in hooks/bin' }
    $dir = Get-CacheDirectory $true
    $attributes = Get-Attributes $dest
    if ($attributes -band ([IO.FileAttributes]::Directory -bor [IO.FileAttributes]::ReparsePoint)) { throw 'unsafe runtime cache destination' }
    $candidate = [IO.Path]::Combine($dir, ".$Name.part.$([Guid]::NewGuid())")
    $base = if ($env:AGX_RELEASE_URL) { $env:AGX_RELEASE_URL } else { 'https://github.com/muehz/agentics/releases/download' }
    $url = "$base/v$version/$Name"
    if ($env:AGX_DEBUG) { [Console]::Error.WriteLine("fetch-hook.ps1 fetching: $url") }
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    # Windows PowerShell 5.1 resolves -OutFile as a wildcard path. Reserve the
    # file exclusively and escape the literal filename for that single API.
    [IO.File]::Open($candidate, 'CreateNew', 'Write', 'None').Dispose()
    $part = $candidate
    Invoke-WebRequest -Uri $url -OutFile ([Management.Automation.WildcardPattern]::Escape($part)) -UseBasicParsing -TimeoutSec 8
    if (!(Test-Digest $part)) {
        [Console]::Error.WriteLine("agx: downloaded $Name does not match hooks/SHA256SUMS - discarded")
        throw 'release checksum mismatch'
    }
    if (!(Get-CacheDirectory $false)) { throw 'runtime cache disappeared' }
    $attributes = Get-Attributes $dest
    if ($attributes -band ([IO.FileAttributes]::Directory -bor [IO.FileAttributes]::ReparsePoint)) { throw 'unsafe runtime cache destination' }
    try {
        if ($null -eq $attributes) { [IO.File]::Move($part, $dest) }
        elseif (!(Test-Digest $dest)) { [IO.File]::Replace($part, $dest, [NullString]::Value) }
    } catch [IO.IOException] {
        # A competing publisher may already have installed the same bytes.
        if (!(Test-Digest $dest)) { throw }
    }
    if (!(Get-CacheDirectory $false) -or !(Test-Digest $dest)) { throw 'published runtime failed verification' }
    Write-Resolution $dest $key
} catch {
    if (!$LocationKey -or $env:AGX_DEBUG) {
        [Console]::Error.WriteLine("agx: cannot provision runtime; verify hooks/SHA256SUMS, cache and release access: $_")
    }
    exit 1
} finally {
    if ($part) { [IO.File]::Delete($part) }
}
exit 0
