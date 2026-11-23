# Bisection script to find which test creates unwanted files/state.
# The Windows twin of find-polluter.sh, with the same arguments, messages and exit codes.
#
# Usage: find-polluter.ps1 <file_or_dir_to_check> <test_pattern>
# Example: find-polluter.ps1 'packages/core/leaked-state' 'src/**/*.test.ts'
#
# Run it from the project root with Windows PowerShell 5.1 or PowerShell 7:
#   powershell -NoProfile -ExecutionPolicy Bypass -File find-polluter.ps1 <file_or_dir_to_check> <test_pattern>
# and read the result from $LASTEXITCODE.
#
# The artifact must not exist yet: a path that already exists cannot show which test creates it.
#
# <test_pattern> is matched against paths relative to the current directory; a leading `./` is
# optional. `**/` matches any number of directories, including none, so `src/**/a.test.ts` finds
# both `src/a.test.ts` and `src/sub/a.test.ts`. `*` and `?` match within one path segment, so
# `src/*.test.ts` does not find nested tests. Every other character, including `[`, `]`, `{` and
# `}`, matches itself. node_modules is skipped. Paths are written with `/` and sorted ordinally.
#
# Each test file runs on its own as `npm test <file>`. For a project that uses another package
# manager, set FIND_POLLUTER_RUNNER to npm, pnpm or yarn; the script then runs `<runner> test <file>`.
#
# Exit codes:
#   0  no polluter found, and every test run exited 0
#   1  a polluter was found
#   2  cannot diagnose: bad arguments, the artifact already exists, no test matched, the runner could
#      not start, or the run was interrupted
#   3  no polluter found, but some test runs exited non-zero or were killed, so those tests may have
#      stopped before they could create the artifact. This is not a clean result.
#
# The arguments arrive in $args, counted by the script. Declared parameters would let the binder
# act first: a missing mandatory one prompts or throws, an extra one fails binding, and an
# argument such as `-v` or `-ea` binds by prefix to a declared or common parameter. Each of those
# exits 1, the code for a found polluter.
param()

$ErrorActionPreference = 'Stop'
$arguments = @($args)

# Output goes out as UTF-8 bytes on the raw streams: Windows PowerShell 5.1 turns the emoji into
# `??` on its text pipeline, and LF is written explicitly to match the sh twin's bytes.
$utf8 = New-Object System.Text.UTF8Encoding $false
$stdoutStream = [Console]::OpenStandardOutput()
$stderrStream = [Console]::OpenStandardError()

function Write-StreamLine([System.IO.Stream]$Stream, [string]$Text) {
    $bytes = $utf8.GetBytes($Text + "`n")
    $Stream.Write($bytes, 0, $bytes.Length)
    $Stream.Flush()
}

function Write-StdOut([string]$Text) { Write-StreamLine $stdoutStream $Text }

function Write-StdErr([string]$Text) { Write-StreamLine $stderrStream $Text }

# True for anything at the path, including a dangling symlink, as `[ -e ] || [ -L ]` is in sh.
function Test-Artifact([string]$Path) {
    if ($Path -eq '') { return $false }
    return $null -ne (Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue)
}

# The glob rules from the header as an anchored regular expression over `./`-prefixed paths.
function ConvertTo-PatternRegex([string]$Pattern) {
    $rest = $Pattern
    if ($rest.StartsWith('./', [StringComparison]::Ordinal)) { $rest = $rest.Substring(2) }
    $regex = New-Object System.Text.StringBuilder
    [void]$regex.Append('^\./')
    $prev = '/'
    $i = 0
    while ($i -lt $rest.Length) {
        $char = $rest.Substring($i, 1)
        $tail = $rest.Substring($i)
        $step = 1
        # `**` is a globstar only at the start of a path segment; elsewhere it is two `*`.
        if ($prev -eq '/' -and $tail.StartsWith('**/', [StringComparison]::Ordinal)) {
            [void]$regex.Append('(.*/)?')
            $step = 3
            $char = '/'
        } elseif ($prev -eq '/' -and $tail.StartsWith('**', [StringComparison]::Ordinal)) {
            [void]$regex.Append('.*')
            $step = 2
        } elseif ($char -eq '*') {
            [void]$regex.Append('[^/]*')
        } elseif ($char -eq '?') {
            [void]$regex.Append('[^/]')
        } else {
            [void]$regex.Append([regex]::Escape($char))
        }
        $prev = $char
        $i += $step
    }
    [void]$regex.Append('$')
    return New-Object System.Text.RegularExpressions.Regex($regex.ToString(), [System.Text.RegularExpressions.RegexOptions]::CultureInvariant)
}

# True for a symbolic link or a junction. The ReparsePoint attribute alone does not make a link:
# OneDrive and other cloud-file folders carry it on ordinary directories, and LinkType is empty
# for them. A hard link is an ordinary file, as it is to `find -type f`.
function Test-Link($Item) {
    return $Item.LinkType -eq 'SymbolicLink' -or $Item.LinkType -eq 'Junction'
}

# Every regular file under $Directory as a `./`-prefixed path with `/` separators. Like the sh
# twin's `find -type f`, it neither lists links to files nor follows links to directories, and it
# skips node_modules.
function Get-TreeFile([string]$Directory, [string]$Relative) {
    foreach ($item in Get-ChildItem -LiteralPath $Directory -Force) {
        if (Test-Link $item) { continue }
        $path = "$Relative/$($item.Name)"
        if ($item.PSIsContainer) {
            if ($item.Name -cne 'node_modules') { Get-TreeFile $item.FullName $path }
        } else {
            $path
        }
    }
}

function ConvertTo-OutputLine($Record) {
    if ($Record -is [System.Management.Automation.ErrorRecord]) {
        # ToString() renders an empty native stderr line as the exception's type name. The line
        # is the target object where PowerShell sets one, and otherwise the exception message.
        if ($Record.TargetObject -is [string]) { return $Record.TargetObject }
        return [string]$Record.Exception.Message
    }
    return [string]$Record
}

# Runs one test file with the runner's two streams merged, as `2>&1` does in the sh twin.
function Invoke-TestRunner([string]$Runner, [string]$File) {
    # Under 'Stop', Windows PowerShell 5.1 turns a native runner's first stderr line into a
    # terminating error once the stream is merged, which would end the search.
    $ErrorActionPreference = 'Continue'
    $global:LASTEXITCODE = 0
    try {
        # The piped $null keeps the runner off the console's stdin. The file is passed as a
        # quoted string because PowerShell on Unix expands wildcards in an unquoted argument to
        # a native command, which would turn a test named `[ab]_test.js` into `a_test.js`.
        $output = @($null | & $Runner test "$File" 2>&1 | ForEach-Object { ConvertTo-OutputLine $_ })
    } catch [System.Management.Automation.CommandNotFoundException] {
        return @{ NotFound = $true; Status = 127; Output = @() }
    }
    return @{ NotFound = $false; Status = [int]$global:LASTEXITCODE; Output = $output }
}

$previousEncoding = $null
try {
    if ($arguments.Count -ne 2) {
        Write-StdOut "Usage: $PSCommandPath <file_to_check> <test_pattern>"
        Write-StdOut "Example: $PSCommandPath 'packages/core/leaked-state' 'src/**/*.test.ts'"
        exit 2
    }

    $pollutionCheck = $arguments[0]
    $testPattern = $arguments[1]
    $runner = $env:FIND_POLLUTER_RUNNER
    if (-not $runner) { $runner = 'npm' }

    if (@('npm', 'pnpm', 'yarn') -cnotcontains $runner) {
        Write-StdErr "Cannot diagnose: FIND_POLLUTER_RUNNER must be npm, pnpm or yarn, not: $runner"
        exit 2
    }

    if (Test-Artifact $pollutionCheck) {
        Write-StdErr "Cannot diagnose: pollution artifact already exists: $pollutionCheck"
        exit 2
    }

    if ($null -ne $PSStyle) { $PSStyle.OutputRendering = 'PlainText' }
    # Windows PowerShell 5.1 decodes a native runner's output with the OEM code page otherwise,
    # which garbles non-ASCII text in the failed-to-run tail. The setting changes the console's
    # code page, which the caller shares, so it is restored on the way out. It is best effort:
    # a failure costs only that text, not the search.
    try {
        $previousEncoding = [Console]::OutputEncoding
        [Console]::OutputEncoding = $utf8
    } catch {
        $previousEncoding = $null
    }

    Write-StdOut "$([char]::ConvertFromUtf32(0x1F50D)) Searching for test that creates: $pollutionCheck"
    Write-StdOut "Test pattern: $testPattern"
    Write-StdOut ''

    $regex = ConvertTo-PatternRegex $testPattern
    [string[]]$testFiles = @(Get-TreeFile (Get-Location).ProviderPath '.' | Where-Object { $regex.IsMatch($_) })
    if ($testFiles.Length -eq 0) {
        Write-StdErr "Cannot diagnose: no tests match: $testPattern"
        exit 2
    }
    [Array]::Sort($testFiles, [StringComparer]::Ordinal)
    $total = $testFiles.Length

    Write-StdOut "Found $total test files"
    Write-StdOut ''

    $index = 0
    $failedFiles = New-Object System.Collections.Generic.List[string]
    foreach ($testFile in $testFiles) {
        $index++

        if (Test-Artifact $pollutionCheck) {
            Write-StdErr "Cannot diagnose: pollution artifact appeared before test: $testFile"
            exit 2
        }

        Write-StdOut "[$index/$total] Testing: $testFile"

        # A failing test can still create the artifact, so a non-zero exit does not stop the search.
        $run = Invoke-TestRunner $runner $testFile

        if (Test-Artifact $pollutionCheck) {
            Write-StdOut ''
            Write-StdOut "$([char]::ConvertFromUtf32(0x1F3AF)) FOUND POLLUTER!"
            Write-StdOut "   Test: $testFile"
            Write-StdOut "   Created: $pollutionCheck"
            Write-StdOut ''
            Write-StdOut 'Pollution details:'
            $details = Get-Item -LiteralPath $pollutionCheck -Force | Format-List | Out-String
            foreach ($line in ($details -split "`r?`n")) {
                if ($line.Trim() -ne '') { Write-StdOut $line.TrimEnd() }
            }
            Write-StdOut ''
            Write-StdOut 'To investigate:'
            Write-StdOut "  $runner test `"$testFile`"    # Run just this test"
            Write-StdOut "  cat `"$testFile`"         # Review test code"
            exit 1
        }

        # A runner PowerShell cannot find, or one exiting 126 or 127, means no test ran. 130, or
        # 0xC000013A from a Windows Ctrl+C, is an interrupt, which must stop the search instead of
        # moving on to the next test. Any other status is a test run that failed.
        $status = $run.Status
        if ($run.NotFound -or $status -eq 126 -or $status -eq 127) {
            Write-StdErr ''
            Write-StdErr "Cannot diagnose: test runner failed to run (exit $status): $runner test $testFile"
            if (-not $run.NotFound) {
                # The sh twin's `$(...)` drops trailing empty lines and `printf | tail -n 20`
                # prints one empty line for no output.
                $tail = [System.Collections.Generic.List[string]]$run.Output
                while ($tail.Count -gt 0 -and $tail[$tail.Count - 1] -eq '') { $tail.RemoveAt($tail.Count - 1) }
                if ($tail.Count -eq 0) { $tail.Add('') }
                foreach ($line in ($tail | Select-Object -Last 20)) { Write-StdErr $line }
            }
            exit 2
        }
        if ($status -eq 130 -or $status -eq -1073741510) {
            Write-StdErr ''
            Write-StdErr "Cannot diagnose: interrupted (exit 130) during: $runner test $testFile"
            exit 2
        }

        if ($status -ne 0) { $failedFiles.Add("   $testFile (exit $status)") }
    }

    Write-StdOut ''
    if ($failedFiles.Count -ne 0) {
        Write-StdErr "No polluter found, but $($failedFiles.Count) of $total test runs exited non-zero:"
        foreach ($line in $failedFiles) { Write-StdErr $line }
        Write-StdErr "A test that stops early may never reach the code that creates $pollutionCheck."
        Write-StdErr "Run one of them directly to see why it failed: $runner test <test file>"
        exit 3
    }

    Write-StdOut "$([char]::ConvertFromUtf32(0x2705)) No polluter found - all tests clean!"
    exit 0
} catch {
    # An uncaught error under -File exits 1, which reads as a found polluter.
    Write-StdErr "Cannot diagnose: $($_.Exception.Message)"
    exit 2
} finally {
    # Runs on `exit` too.
    if ($null -ne $previousEncoding) {
        try { [Console]::OutputEncoding = $previousEncoding } catch { }
    }
}
