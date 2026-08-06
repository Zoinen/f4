package androidfs

import (
	"fmt"
	"path"
	"strings"
)

// quoteShellArg returns one POSIX-shell word whose value is exactly s. Android
// file names are allowed to contain whitespace, quotes and shell metacharacters,
// so interpolating a path into a command without this boundary is unsafe.
func quoteShellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// mutationPath validates the same invariant as the FISH+ helper: mutations
// only target absolute paths, never the root, and never accept a literal '..'
// component. Checking components before path.Clean is intentional; otherwise
// a typo such as /data/local/tmp/a/../.. could turn into a much broader target.
func mutationPath(p string) (string, error) {
	if strings.IndexByte(p, 0) >= 0 {
		return "", fmt.Errorf("android: path contains NUL")
	}
	if !path.IsAbs(p) {
		return "", fmt.Errorf("android: mutation path %q is not absolute", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fmt.Errorf("android: mutation path %q contains a '..' component", p)
		}
	}
	cleaned := path.Clean(p)
	if cleaned == "/" {
		return "", fmt.Errorf("android: refusing to mutate the device root")
	}
	return cleaned, nil
}
