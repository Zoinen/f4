//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"strings"

	"github.com/unxed/vtui"
)

// ToUnixPath turns a path as the Wine process names it into the path the host
// (and so the native X11 drop target) names. Wine maps drive Z: to the host's
// root, so "Z:\home\me\a.txt" is "/home/me/a.txt"; any other path is returned
// unchanged -- other drives need Wine's own lookup, which is not done here.
func ToUnixPath(p string) string {
	if len(p) >= 3 && (p[0] == 'Z' || p[0] == 'z') && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return strings.ReplaceAll(p[2:], `\`, "/")
	}
	return p
}

// unixPayload returns payload with its Paths converted by ToUnixPath, so that
// the text/uri-list the X11 target receives names files it can open.
func unixPayload(payload vtui.DragPayload) vtui.DragPayload {
	if len(payload.Paths) == 0 {
		return payload
	}
	paths := make([]string, len(payload.Paths))
	for i, p := range payload.Paths {
		paths[i] = ToUnixPath(p)
	}
	payload.Paths = paths
	return payload
}
