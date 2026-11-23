// Package pathshape owns the rules that call a path's spelling hazardous: a spelling that names a different file than
// its cleaned form, or that cannot be a plain local file at all. The rules read the spelling only and touch no
// filesystem.
//
// The package is a leaf: it imports nothing from this module, so every package that reads paths can depend on it
// without forming a cycle.
//
// A path is not readable here when [LocalFileProblem] returns a non-empty [Problem]. Every Problem other than
// [NoProblem] is non-empty, so a refusal is never mistaken for a clean path. Each consumer keeps its own posture
// toward a hazardous spelling:
//   - the script reader refuses `script-unreadable` when [LocalFileProblem] names a Problem, when
//     [Host.DrivePathOffHost] reports, or when [Host.ParentAfterNameOffHost] reports;
//   - the script parser refuses `script-path-not-literal` when [Host.ChangedByCleaning] reports;
//   - the write-target check counts a write to a word as a match on an inspected script when
//     [Host.MayNameAnotherFile] reports.
//
// The drive and parent-after-name rules stay outside LocalFileProblem because each is a hazard only off Windows;
// folding either in would refuse every Windows drive path.
package pathshape

import (
	"path"
	"regexp"
	"runtime"
	"strings"
)

// Host is the operating system whose path rules apply. The zero value means the running host, resolved at each call.
// Any system other than Windows and macOS follows the Linux rules.
type Host struct {
	goos string
}

// Current returns the running host.
func Current() Host { return For(runtime.GOOS) }

// For returns the host named by a GOOS value, so a test can apply another host's rules. An empty name means the
// running host.
func For(goos string) Host { return Host{goos: goos} }

func (h Host) os() string {
	if h.goos == "" {
		return runtime.GOOS
	}
	return h.goos
}

// Windows reports a Windows host.
func (h Host) Windows() bool { return h.os() == "windows" }

// FoldsCase reports a host whose default file system treats names that differ only in case as one file: Windows or
// macOS.
func (h Host) FoldsCase() bool { return h.Windows() || h.os() == "darwin" }

// Native maps a Git Bash style /c/dir to c:/dir on Windows and returns p unchanged elsewhere.
func (h Host) Native(p string) string {
	if h.Windows() && len(p) >= 3 && p[0] == '/' && p[2] == '/' &&
		((p[1] >= 'a' && p[1] <= 'z') || (p[1] >= 'A' && p[1] <= 'Z')) {
		return p[1:2] + ":" + p[2:]
	}
	return p
}

// ChangedByCleaning reports a word whose cleaned form would name a different file than the shell opens, judged as
// written, before any cleaning. It covers three spellings:
//   - a leading backslash or double slash, a network or device namespace that is never the local path it cleans to,
//     on every host;
//   - a backslash anywhere, off Windows, where it is an ordinary file name character and cleaning would turn it into
//     a separator;
//   - a .. after a named component, off Windows (see [Host.ParentAfterNameOffHost]).
//
// A leading run of .. is not covered: whether it is safe depends on the directory it starts from.
func (h Host) ChangedByCleaning(word string) bool {
	if strings.HasPrefix(word, `\`) || strings.HasPrefix(strings.ReplaceAll(word, `\`, "/"), "//") {
		return true
	}
	if h.Windows() {
		return false
	}
	return strings.Contains(word, `\`) || parentAfterName(word)
}

// MayNameAnotherFile reports a word that may name another file than it shows: one [Host.ChangedByCleaning] reports,
// or on Windows a colon past the drive (a stream name), a component ending in a dot or a space, or a short 8.3 name
// (NAME~1), each of which Windows resolves to a different name than it shows.
func (h Host) MayNameAnotherFile(word string) bool {
	return h.ChangedByCleaning(word) || h.Windows() && windowsNameHazard(word)
}

// DrivePathOffHost reports a drive prefix on a host that is not Windows, where it is no drive and the path names a
// relative file whose first component holds a colon.
func (h Host) DrivePathOffHost(p string) bool { return !h.Windows() && HasDrive(p) }

// ParentAfterNameOffHost reports a .. component after a named component, on a host that is not Windows. There the
// shell resolves such a .. against the real directory, so when the named component is a symlink the path names
// another file than its cleaned form does.
func (h Host) ParentAfterNameOffHost(p string) bool { return !h.Windows() && parentAfterName(p) }

// Problem names why a path cannot be a plain local file. Its text is the detail a refusal shows.
type Problem string

// The problems [LocalFileProblem] reports.
const (
	NoProblem                 Problem = ""
	ProblemNamespace          Problem = "network or device namespace path"
	ProblemDriveRelative      Problem = "drive-relative path"
	ProblemStream             Problem = "alternate data stream"
	ProblemProcessOrDevice    Problem = "process or device object"
	ProblemReservedName       Problem = "reserved device name"
	ProblemTrailingDotOrSpace Problem = "trailing dot or space"
)

// LocalFileProblem names why p cannot be a plain local file, or returns [NoProblem]. The rules are host-independent,
// so a spelling hazardous on any host is refused everywhere. The named-pipe namespace (`\\.\pipe\x`) and the NT
// object namespace (`\??\`) are covered by the namespace rule.
func LocalFileProblem(p string) Problem {
	slash := strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(slash, "//") || strings.HasPrefix(slash, "/??/") || slash == "/??" {
		return ProblemNamespace
	}
	rest := slash
	drive := HasDrive(slash)
	if drive {
		rest = slash[2:]
		if rest == "" || rest[0] != '/' {
			return ProblemDriveRelative
		}
	}
	if strings.Contains(rest, ":") {
		return ProblemStream
	}
	if !drive {
		// Case-insensitive because the same spelling reaches the object on a case-insensitive host.
		clean := strings.ToLower(path.Clean(slash))
		if clean == "/proc" || strings.HasPrefix(clean, "/proc/") || clean == "/dev" || strings.HasPrefix(clean, "/dev/") {
			return ProblemProcessOrDevice
		}
	}
	for _, part := range strings.Split(rest, "/") {
		if part == "" || part == "." || part == ".." {
			continue
		}
		if ReservedDeviceName(part) {
			return ProblemReservedName
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return ProblemTrailingDotOrSpace
		}
	}
	return NoProblem
}

// HasDrive reports a leading drive letter and colon.
func HasDrive(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	c := p[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// reservedName matches the base name of a Windows legacy device, which the system opens as a device whatever
// extension or trailing spaces follow it. The superscript digits are reserved too. The match folds case as the
// regular expression engine does, by simple folding; strings.ToUpper would map the dotless i to I and refuse a name
// Windows does not reserve.
var reservedName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|conin\$|conout\$|(com|lpt)[0-9\x{00b9}\x{00b2}\x{00b3}])$`)

// ReservedDeviceName reports one path element whose base, the text before its first dot with trailing spaces
// trimmed, is a Windows legacy device name in any letter case.
func ReservedDeviceName(element string) bool {
	base := element
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	return reservedName.MatchString(strings.TrimRight(base, " "))
}

// parentAfterName reports a .. component that follows a named component, splitting on / only.
func parentAfterName(p string) bool {
	named := false
	for _, part := range strings.Split(p, "/") {
		switch part {
		case "", ".":
		case "..":
			if named {
				return true
			}
		default:
			named = true
		}
	}
	return false
}

// windowsNameHazard reports a spelling Windows resolves to a different name than it shows: a component with a
// trailing dot or space, a short 8.3 name (NAME~1), or a colon past the drive (a stream name).
func windowsNameHazard(text string) bool {
	rest := text
	if HasDrive(rest) {
		rest = rest[2:]
	}
	if strings.Contains(rest, ":") {
		return true
	}
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "." || part == ".." {
			continue
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return true
		}
		if i := strings.IndexByte(part, '~'); i >= 0 && i+1 < len(part) && part[i+1] >= '0' && part[i+1] <= '9' {
			return true
		}
	}
	return false
}
