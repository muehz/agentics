// Package argv holds the command grammar that sits below shellwords and is
// shared by the guards and the script proof. It imports nothing from this
// module, so any package can depend on it without a cycle, and it decides no
// guard posture: input a reader cannot interpret is reported as such, and each
// consumer keeps its own answer for it.
//
// # Consumer postures
//
// A reader that finds nothing in a line it could not read has not shown the
// line is harmless, so ReadGit and Scan report that case apart from "nothing
// found": ReadGit as GitUnread (the grammar cannot tell where the subcommand
// starts) or GitPrints (a global option makes git print and exit), and Scan as
// a non-nil Problem. Each consumer's answer for it is listed here, with the
// test that pins it where one exists.
//
// For GitUnread and GitPrints:
//
//   - command-guardian clean and reset: the rules do not run, because they
//     read only a line read to its verb, so such a line passes them.
//     TestGitUnreadablePosture pins it.
//   - command-guardian hook bypass: the -c and --config-env specs are still
//     read, through ConfigSpecs and its lenient walk, so a hooks-path override
//     before an unreadable verb or past an unknown option is still denied.
//     TestGitUnreadablePosture pins it.
//   - command-guardian gh blanking: the commit message is not blanked, so it
//     stays visible to the gh rules. A line read to its verb is blanked as
//     before.
//   - command-guardian redaction: the line is redacted whenever words follow
//     the point where the reading stopped, so a threat match in it is not
//     echoed.
//   - command-guardian commit heredoc and git grep data: not eligible, because
//     both need a line read to its verb.
//   - secrets-guardian git show: no object paths are named, so the line is not
//     read as a credential read. TestRunGitShowUnreadablePosture pins it.
//   - script proof operand rule: protected, so a non-literal operand of the
//     line must be proven. TestScriptOperandUnreadableGitPosture pins it.
//   - script proof predecessor writer: a writer when words are left over after
//     the walk stopped, since git may still run one of them as a writing
//     subcommand; with nothing left it runs no subcommand.
//
// For a Scan Problem:
//
//   - release-guardian: deny with the matcher's own message for the first
//     unreadable word, byte for byte. TestReleaseScanPosture pins it.
//   - secrets-guardian options: ambiguous. A word that may hide structured
//     Secret output ends in a deny, and a line whose output is only a name
//     passes. TestSecretsScanPosture pins it.
//   - command-guardian curl upload: the unreadable word is skipped and the scan
//     continues, so a later upload option still denies. TestCurlScanPosture
//     pins it.
package argv
