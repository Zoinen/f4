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

// folderHistoryPathIdentity returns the exact identity used by folder history
// deduplication. Keeping it as a reusable key avoids repeatedly normalizing the
// same paths in O(N^2) history merges on every directory transition.
func folderHistoryPathIdentity(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if identity, ok := normalizedURIIdentity(raw); ok {
		return "uri:" + identity, true
	}
	colon := strings.IndexByte(raw, ':')
	if colon > 1 && len(raw) > colon+1 &&
		(raw[colon+1] == '/' || raw[colon+1] == '\\') {
		if runtime.GOOS == "windows" {
			raw = strings.ReplaceAll(raw, "/", "\\")
		} else {
			raw = strings.ReplaceAll(raw, "\\", "/")
		}
		return "visual:" + raw, true
	}
	raw = filepath.Clean(raw)
	if runtime.GOOS == "windows" {
		raw = strings.ToLower(raw)
	}
	return "local:" + raw, true
}

func sameFolderHistoryPath(a, b string) bool {
	identityA, validA := folderHistoryPathIdentity(a)
	identityB, validB := folderHistoryPathIdentity(b)
	return validA && validB && identityA == identityB
}

func isPersistentURIPath(path string) bool {
	_, ok := normalizedURIIdentity(path)
	return ok
}
