//go:build !windows && !extralite

package vfs

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

// ianaName is the IANA name of enc, for telling which code page the system
// locale uses.
func ianaName(enc encoding.Encoding) (string, error) {
	return ianaindex.IANA.Name(enc)
}
