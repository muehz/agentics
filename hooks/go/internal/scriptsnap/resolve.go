package scriptsnap

import (
	"os"
	"path/filepath"
	"strings"
)

// maxLinkHops bounds the links followed while resolving one path, so a cycle ends in a refusal to resolve.
const maxLinkHops = 64

// resolveExisting resolves the links in path, whether or not the final components exist. When a component is missing
// the rest is appended as written, so a missing file under a link into a protected directory still resolves there.
//
// filepath.EvalSymlinks is tried first, but on Windows it fails for any path below a junction, so a failure falls back
// to following each component by hand with os.Readlink, which reads symlinks and junctions alike.
func resolveExisting(path string) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, true
	}
	return resolveByHand(path)
}

func resolveByHand(path string) (string, bool) {
	sep := string(filepath.Separator)
	cur := filepath.Clean(path)
	for hops := 0; hops <= maxLinkHops; hops++ {
		volume := filepath.VolumeName(cur)
		rest := cur[len(volume):]
		built := volume
		if strings.HasPrefix(rest, sep) {
			built += sep
			rest = rest[1:]
		}
		parts := strings.Split(rest, sep)
		followed := false
		for i, part := range parts {
			if part == "" {
				continue
			}
			next := filepath.Join(built, part)
			info, err := os.Lstat(next)
			if err != nil {
				return filepath.Join(append([]string{built}, parts[i:]...)...), true
			}
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				target, err := os.Readlink(next)
				if err == nil {
					if !filepath.IsAbs(target) {
						target = filepath.Join(built, target)
					}
					cur = filepath.Join(append([]string{target}, parts[i+1:]...)...)
					followed = true
					break
				}
				if info.Mode()&os.ModeSymlink != 0 {
					return "", false
				}
			}
			built = next
		}
		if !followed {
			return built, true
		}
	}
	return "", false
}
