package vfs

import (
	"net/url"
	"strings"
)

// UNCToSMBURI turns a UNC-style network path into the smb:// URI that the
// NetFox SMB provider opens (f4#1702, part 3): \\host, \\host\share and
// \\host\share\dir\file, and with allowSlashes also //host/share/dir. On a
// system without native UNC paths this is what lets a name typed the way
// Windows users write it reach the share.
//
// It reports false for anything that is not a plain host-based UNC path:
// the device prefixes \\?\ and \\.\, an empty host, or a host that carries
// URI syntax of its own.
func UNCToSMBURI(p string, allowSlashes bool) (string, bool) {
	var rest string
	switch {
	case strings.HasPrefix(p, `\\`):
		rest = p[2:]
	case allowSlashes && strings.HasPrefix(p, "//"):
		rest = p[2:]
	default:
		return "", false
	}
	rest = strings.ReplaceAll(rest, `\`, "/")
	parts := strings.Split(rest, "/")
	host := parts[0]
	if host == "" || host == "?" || host == "." || strings.ContainsAny(host, " \t%?#") {
		return "", false
	}
	uri := "smb://" + host
	for _, seg := range parts[1:] {
		if seg == "" {
			continue
		}
		uri += "/" + url.PathEscape(seg)
	}
	return uri, true
}
