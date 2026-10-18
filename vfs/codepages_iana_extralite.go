//go:build !windows && extralite

package vfs

import (
	"errors"

	"golang.org/x/text/encoding"
)

// ianaName is the IANA name of enc among the single-byte encodings.
func ianaName(enc encoding.Encoding) (string, error) {
	if name, ok := singleByteIANANames[enc]; ok {
		return name, nil
	}
	return "", errors.New("vfs: no IANA name for this encoding")
}
