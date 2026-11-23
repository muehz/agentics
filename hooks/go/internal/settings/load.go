package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// supportedVersion is the only settings file version this binary reads.
	supportedVersion = "1"
	// maxFileBytes bounds a settings file; a larger file is invalid.
	maxFileBytes = 64 << 10
	dirName      = ".agentics"
	fileName     = "hooks.json"

	// classAbsent marks a file that does not exist. It is not an error.
	classAbsent = "absent"
)

// Resolve resolves the settings for an event whose working directory is cwd,
// using the home directory os.UserHomeDir reports. An empty cwd means the
// process working directory.
func Resolve(cwd string) Result {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return ResolveIn(cwd, home)
}

// ResolveIn is Resolve with an explicit home directory.
//
// The user file applies first and the repository file second, so a repository
// leaf overrides a user leaf. If either file is invalid, every override from
// both is discarded: a broken repository file must not inherit a user-level
// false, and a broken user file must not let a valid repository file weaken
// policy.
func ResolveIn(cwd, home string) Result {
	res := Result{Policy: Defaults()}
	if home != "" {
		res.UserPath = filepath.Join(home, dirName, fileName)
	}
	if root := repositoryRoot(cwd); root != "" {
		res.RepoPath = filepath.Join(root, dirName, fileName)
	}
	paths := []string{res.UserPath, res.RepoPath}
	if res.RepoPath == res.UserPath {
		// The home directory is itself the repository root: one file, read once.
		paths = paths[:1]
	}
	var layers []map[Key]bool
	for _, path := range paths {
		if path == "" {
			continue
		}
		overrides, class := readFile(path)
		switch class {
		case classAbsent:
		case "":
			layers = append(layers, overrides)
		default:
			res.Invalid = append(res.Invalid, Invalid{Path: path, Class: class})
		}
	}
	if len(res.Invalid) != 0 {
		return res
	}
	for _, layer := range layers {
		for k, v := range layer {
			res.Policy[k] = v
		}
	}
	return res
}

// repositoryRoot returns the nearest ancestor of cwd, or cwd itself, that holds
// a .git file or directory, or "" when there is none. It runs no git command
// and does not look past that directory. A cwd that is relative or cannot be
// resolved yields "".
func repositoryRoot(cwd string) string {
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		cwd = wd
	}
	if !filepath.IsAbs(cwd) {
		return ""
	}
	dir := filepath.Clean(cwd)
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// readFile reads and validates one settings file. It returns the explicit
// leaves, or an error class: classAbsent when the file does not exist.
func readFile(path string) (map[Key]bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Stat follows links, so a dangling link reports not-exist although
			// an entry occupies the path. That file exists and cannot be read.
			if _, linkErr := os.Lstat(path); linkErr == nil {
				return nil, ClassUnreadable
			}
			return nil, classAbsent
		}
		return nil, ClassUnreadable
	}
	if !info.Mode().IsRegular() {
		return nil, ClassNotAFile
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ClassUnreadable
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, ClassUnreadable
	}
	if len(data) > maxFileBytes {
		return nil, ClassTooLarge
	}
	return parse(data)
}

// policyNames maps a leaf name inside the policy object to its Key.
var policyNames = func() map[string]Key {
	m := make(map[string]Key, len(Keys))
	for _, k := range Keys {
		m[strings.TrimPrefix(string(k), "policy.")] = k
	}
	return m
}()

// parse validates a settings document and returns its explicit leaves, or the
// class of the first fault. It walks the token stream because encoding/json
// accepts duplicate keys silently and does not report an unknown key at every
// depth.
func parse(data []byte) (map[Key]bool, string) {
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) || !utf8.Valid(data) {
		return nil, ClassNotJSON
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, ClassNotJSON
	}
	if tok != json.Delim('{') {
		return nil, ClassBadType
	}
	overrides := map[Key]bool{}
	seen := map[string]bool{}
	sawVersion := false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, ClassNotJSON
		}
		name, _ := tok.(string)
		if seen[name] {
			return nil, ClassDuplicateKey
		}
		seen[name] = true
		switch name {
		case "version":
			tok, err := dec.Token()
			if err != nil {
				return nil, ClassNotJSON
			}
			n, ok := tok.(json.Number)
			if !ok {
				return nil, ClassBadType
			}
			if n.String() != supportedVersion {
				return nil, ClassBadVersion
			}
			sawVersion = true
		case "policy":
			if class := parsePolicy(dec, overrides); class != "" {
				return nil, class
			}
		default:
			return nil, ClassUnknownKey
		}
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, ClassNotJSON
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, ClassTrailingData
	}
	if !sawVersion {
		return nil, ClassBadVersion
	}
	return overrides, ""
}

// parsePolicy reads the policy object into out and returns a fault class, or
// "" when the object is valid.
func parsePolicy(dec *json.Decoder, out map[Key]bool) string {
	tok, err := dec.Token()
	if err != nil {
		return ClassNotJSON
	}
	if tok != json.Delim('{') {
		return ClassBadType
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return ClassNotJSON
		}
		name, _ := tok.(string)
		if seen[name] {
			return ClassDuplicateKey
		}
		seen[name] = true
		key, known := policyNames[name]
		if !known {
			return ClassUnknownKey
		}
		tok, err = dec.Token()
		if err != nil {
			return ClassNotJSON
		}
		value, ok := tok.(bool)
		if !ok {
			return ClassBadType
		}
		out[key] = value
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return ClassNotJSON
	}
	return ""
}
