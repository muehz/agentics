# Preview the Agentics planning protection block for a repository's root .gitignore.
#
# Usage: planning-protection.ps1 --parent <relative-parent> [--gitignore <existing-root-.gitignore>]
#
# Prints the full merged .gitignore on stdout and a limitation note on stderr.
# Writes no file. Exits 0 on success and 1 on a usage, parent, marker or
# encoding error. The only template is ../references/planning-protection.gitignore.
# Runs under Windows PowerShell 5.1 and PowerShell 7.
#
# The script declares no parameters and reads $args, so the flag loop sees the
# same tokens in the same order as the shell twin. A param block would hand
# the arguments to PowerShell's parameter binder, which consumes a bare "--"
# as its end-of-parameters marker before any script code runs. Every string
# operation is ordinal and case-sensitive. Output goes through the raw
# standard streams as UTF-8 bytes: PowerShell's text pipeline would re-encode
# the merged file and add a newline.
$Rest = $args

$ErrorActionPreference = 'Stop'
# Progress records would reach stderr as CLIXML.
$ProgressPreference = 'SilentlyContinue'

$Begin = '# BEGIN AGENTICS PLANNING PROTECTION'
$End = '# END AGENTICS PLANNING PROTECTION'
$Ordinal = [System.StringComparison]::Ordinal
$Utf8 = New-Object System.Text.UTF8Encoding($false)
# Strict decoding throws on any invalid sequence instead of substituting
# U+FFFD. Decoding the whole byte array keeps a BOM as U+FEFF, and encoding
# that character again restores the BOM.
$StrictUtf8 = New-Object System.Text.UTF8Encoding($false, $true)
$Stdout = [Console]::OpenStandardOutput()
$Stderr = [Console]::OpenStandardError()

# -ceq compares under the current culture, which treats some characters, such
# as a BOM, as ignorable; an exact comparison must not.
function Test-Same([string]$Left, [string]$Right) {
    return [string]::Equals($Left, $Right, $Ordinal)
}

function Write-Stream([System.IO.Stream]$Stream, [string]$Text) {
    $bytes = $Utf8.GetBytes($Text)
    $Stream.Write($bytes, 0, $bytes.Length)
}

function Exit-Script([int]$Code) {
    $Stdout.Flush()
    $Stderr.Flush()
    exit $Code
}

function Stop-Script([string]$Message) {
    Write-Stream $Stderr "$Message`n"
    Exit-Script 1
}

function Stop-Usage {
    Stop-Script 'Usage: planning-protection.ps1 --parent <relative-parent> [--gitignore <existing-root-.gitignore>]'
}

try {
    $arguments = @()
    if ($null -ne $Rest) { $arguments = @($Rest) }
    $parent = $null
    $gitignore = $null
    for ($index = 0; $index -lt $arguments.Length; $index += 2) {
        $flag = $arguments[$index]
        if ($index + 1 -eq $arguments.Length) { Stop-Usage }
        $value = $arguments[$index + 1]
        if ($value.StartsWith('--', $Ordinal)) { Stop-Usage }
        if (Test-Same $flag '--parent') {
            if ($null -ne $parent) { Stop-Usage }
            $parent = $value
        } elseif (Test-Same $flag '--gitignore') {
            if ($null -ne $gitignore) { Stop-Usage }
            $gitignore = $value
        } else {
            Stop-Usage
        }
    }
    if ($null -eq $parent) { Stop-Usage }

    # One trailing slash is conventional and dropped; any other empty component is not.
    if ($parent.EndsWith('/', $Ordinal)) { $parent = $parent.Substring(0, $parent.Length - 1) }
    $components = $parent.Split([char]'/')
    if ($parent.Length -eq 0 -or
        $parent.IndexOfAny([char[]]@("`r", "`n", [char]0, '\', ':')) -ge 0 -or
        $parent.StartsWith('/', $Ordinal) -or
        @($components | Where-Object { $_.Length -eq 0 }).Count -gt 0) {
        Stop-Script 'Invalid planning parent: use a nonempty repository-relative slash-separated path'
    }
    if (@($components | Where-Object { (Test-Same $_ '.') -or (Test-Same $_ '..') }).Count -gt 0) {
        Stop-Script 'Invalid planning parent: traversal components are forbidden'
    }

    # Forward slashes, because PowerShell 7 on Linux runs this too and .NET
    # there treats a backslash as an ordinary file name character.
    $templatePath = Join-Path $PSScriptRoot '../references/planning-protection.gitignore'
    try {
        $template = $StrictUtf8.GetString([System.IO.File]::ReadAllBytes($templatePath))
    } catch {
        Stop-Script "Cannot read the planning protection template: $templatePath"
    }

    # Characters that .gitignore treats as pattern syntax get a backslash.
    $escaped = New-Object System.Text.StringBuilder
    foreach ($character in $parent.ToCharArray()) {
        if ('\*?[]!# '.IndexOf($character) -ge 0) { [void]$escaped.Append('\') }
        [void]$escaped.Append($character)
    }
    $escapedParent = $escaped.ToString()

    $existing = ''
    if ($null -ne $gitignore) {
        # .NET resolves a relative path against the process directory, which
        # need not be the PowerShell location.
        $gitignorePath = $gitignore
        if (-not [System.IO.Path]::IsPathRooted($gitignorePath)) {
            $gitignorePath = Join-Path (Get-Location).ProviderPath $gitignorePath
        }
        try {
            $bytes = [System.IO.File]::ReadAllBytes($gitignorePath)
        } catch {
            Stop-Script "Cannot read the .gitignore file: $gitignore"
        }
        try {
            $existing = $StrictUtf8.GetString($bytes)
        } catch [System.Text.DecoderFallbackException] {
            Stop-Script 'Invalid .gitignore encoding: expected UTF-8'
        }
    }

    $eol = "`n"
    if ($existing.IndexOf("`r`n", $Ordinal) -ge 0) { $eol = "`r`n" }
    $block = [regex]::Replace($template.Replace('{{PARENT}}', $escapedParent), "`r?`n", $eol)

    $markerLines = @([regex]::Split($existing, "`r?`n") |
            Where-Object { [regex]::IsMatch($_, '# (?:BEGIN|END) AGENTICS PLANNING PROTECTION') })
    if ($markerLines.Count -gt 0 -and
        ($markerLines.Count -ne 2 -or -not (Test-Same $markerLines[0] $Begin) -or -not (Test-Same $markerLines[1] $End))) {
        Stop-Script 'Invalid planning protection markers: expected exactly one complete standalone block'
    }

    $beginAt = $existing.IndexOf($Begin, $Ordinal)
    if ($beginAt -lt 0) {
        $result = $existing
        if ($existing.Length -gt 0 -and -not $existing.EndsWith("`n", $Ordinal)) { $result += $eol }
        $result += $block
    } else {
        $suffixAt = $existing.IndexOf($End, $Ordinal) + $End.Length
        $after = $existing.Substring($suffixAt)
        if ($after.StartsWith("`r`n", $Ordinal)) {
            $suffixAt += 2
        } elseif ($after.StartsWith("`n", $Ordinal)) {
            $suffixAt += 1
        }
        $result = $existing.Substring(0, $beginAt) + $block + $existing.Substring($suffixAt)
    }

    Write-Stream $Stderr "Limitation: existing ignored ancestors can prevent discovery; this preview does not unignore ancestors. Ignores cannot untrack files or prevent force-adds; native validation remains separate.`n"
    Write-Stream $Stdout $result
    Exit-Script 0
} catch {
    Write-Stream $Stderr "planning-protection.ps1: $($_.Exception.Message)`n"
    Exit-Script 1
}
