package main

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unxed/f4/vfs"
)

// normalizedURIIdentity normalizes only the case-insensitive URI components.
// It deliberately does not filepath.Clean or decode the path: provider IDs and
// escaped literal dot segments are opaque persistent identity.
func normalizedURIIdentity(raw string) (string, bool) {
	if !vfs.IsURIPath(raw) {
		return "", false
	}
	if runtime.GOOS == "windows" && filepath.VolumeName(raw) != "" {
		return "", false
	}
	separator := strings.Index(raw, "://")
	scheme := strings.ToLower(raw[:separator])
	rest := raw[separator+3:]
	authority := rest
	suffix := ""
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		authority = rest[:slash]
		suffix = rest[slash:]
	}
	identity := scheme + "://" + strings.ToLower(authority) + suffix
	if len(suffix) > 1 {
		identity = strings.TrimSuffix(identity, "/")
	}
	return identity, true
}

func isPersistentURIPath(path string) bool {
	_, ok := normalizedURIIdentity(path)
	return ok
}
