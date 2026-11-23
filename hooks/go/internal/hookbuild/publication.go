package hookbuild

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Publication moves a finished Result into an output directory all or
// nothing. OpenPublication runs every refusal that needs no build, Scratch
// hands out the directory to build into, and Commit publishes the binaries
// from it and removes what it published if one fails. The caller must own the
// output directory exclusively while Commit runs, because the rollback removes
// by path.
type Publication struct {
	dest     string
	launcher bool
	scratch  string
}

// OpenPublication resolves symbolic links in out's parent and in out, and
// refuses an out at or under a hooks/bin directory, so a build cannot
// overwrite the committed binaries. With launcher the launcher is published
// into <out>/bin, so the check starts one level lower and also refuses an out
// that names a hooks directory. It refuses an out that already holds one of
// the six contract binaries, or with launcher bin/agx.exe, and it creates
// nothing. out must be absolute.
func OpenPublication(out string, launcher bool) (*Publication, error) {
	parent, err := filepath.EvalSymlinks(filepath.Dir(out))
	if err != nil {
		return nil, fmt.Errorf("output parent: %w", err)
	}
	dest := filepath.Join(parent, filepath.Base(out))
	if resolved, e := filepath.EvalSymlinks(dest); e == nil {
		dest = resolved
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	start := dest
	if launcher {
		start = filepath.Join(dest, "bin")
	}
	for path := start; ; path = filepath.Dir(path) {
		if strings.EqualFold(filepath.Base(path), "bin") && strings.EqualFold(filepath.Base(filepath.Dir(path)), "hooks") {
			return nil, errors.New("refusing to write hooks/bin")
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	for _, name := range ContractBinaries {
		if _, err := os.Lstat(filepath.Join(dest, name)); err == nil {
			return nil, fmt.Errorf("output already exists: %s", name)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if launcher {
		launcherDest := filepath.Join(dest, filepath.FromSlash(LauncherOutput))
		launcherDir := filepath.Dir(launcherDest)
		info, err := os.Lstat(launcherDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err == nil && info.Mode().Type() != os.ModeDir {
			return nil, fmt.Errorf("launcher output parent must be a directory without redirection: %s", launcherDir)
		}
		if _, err := os.Lstat(launcherDest); err == nil {
			return nil, fmt.Errorf("output already exists: %s", LauncherOutput)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return &Publication{dest: dest, launcher: launcher}, nil
}

// Scratch returns a directory inside the destination to build into, creating
// the destination and the directory on the first call. Later calls return the
// same directory. It sits inside the destination so that publishing is a copy
// within one volume.
func (p *Publication) Scratch() (string, error) {
	if p.scratch != "" {
		return p.scratch, nil
	}
	if err := os.MkdirAll(p.dest, 0o755); err != nil {
		return "", err
	}
	scratch, err := os.MkdirTemp(p.dest, ".hooktool-")
	if err != nil {
		return "", err
	}
	p.scratch = scratch
	return scratch, nil
}

// Commit publishes res into the destination: the launcher first, as
// bin/agx.exe, then each target from its Path to <dest>/<Name> in Result order.
// A failure reads "publishing <name>: <cause>". When one of the targets fails,
// Commit removes every file this call published, in publish order, and joins
// each removal error to the failure; the destination, its bin directory and
// any other file stay. The launcher goes first so that a failure among the
// targets removes it with them.
//
// Before it publishes anything Commit refuses a target whose Name is not a
// contract binary, which also stops a name that climbs out of the destination,
// and a res whose launcher presence differs from the launcher flag given to
// OpenPublication, since the opening refusals were made for that flag. It does
// not judge the targets' Err fields or read the sources first; the caller
// verifies the Result before committing.
func (p *Publication) Commit(res Result) error {
	for _, target := range res.Targets {
		if _, ok := ContractTargets[target.Name]; !ok {
			return fmt.Errorf("hookbuild: %s is not a contract binary", target.Name)
		}
	}
	switch {
	case res.Launcher != nil && !p.launcher:
		return errors.New("hookbuild: the result has a launcher, but the publication was opened without one")
	case res.Launcher == nil && p.launcher:
		return errors.New("hookbuild: the result has no launcher, but the publication was opened with one")
	}
	var published []string
	rollback := func(err error) error {
		for _, path := range published {
			if cleanupErr := os.Remove(path); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("removing %s: %w", path, cleanupErr))
			}
		}
		return err
	}
	if res.Launcher != nil {
		launcherDest := filepath.Join(p.dest, filepath.FromSlash(LauncherOutput))
		if err := os.MkdirAll(filepath.Dir(launcherDest), 0o755); err != nil {
			return err
		}
		if err := publishFile(res.Launcher.Path, launcherDest); err != nil {
			return fmt.Errorf("publishing %s: %w", LauncherOutput, err)
		}
		published = append(published, launcherDest)
	}
	for _, target := range res.Targets {
		dst := filepath.Join(p.dest, target.Name)
		if err := publishFile(target.Path, dst); err != nil {
			return rollback(fmt.Errorf("publishing %s: %w", target.Name, err))
		}
		published = append(published, dst)
	}
	return nil
}

// Close removes the scratch directory and nothing else. It returns nil when
// Scratch was never called.
func (p *Publication) Close() error {
	if p.scratch == "" {
		return nil
	}
	return os.RemoveAll(p.scratch)
}

// publishFile copies src to a new file dst, refusing to overwrite one, and
// removes a partial dst when the copy fails.
func publishFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		if cleanupErr := os.Remove(dst); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("removing %s: %w", dst, cleanupErr))
		}
		return err
	}
	return nil
}
