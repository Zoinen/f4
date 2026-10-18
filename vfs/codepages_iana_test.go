//go:build !windows

package vfs

import (
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// The system locale's single-byte code pages are told apart in both builds,
// the extra-lite one without the IANA index (f4#1671).
func TestCodepageIDForSingleByteEncodings(t *testing.T) {
	for _, tc := range []struct {
		enc  *charmap.Charmap
		want int
	}{
		{charmap.Windows1251, 1251},
		{charmap.CodePage866, 866},
		{charmap.ISO8859_15, 28605},
		{charmap.KOI8R, 20866},
		{charmap.KOI8U, 21866},
	} {
		if got := codepageIDForEncoding(tc.enc, -1); got != tc.want {
			t.Errorf("codepageIDForEncoding(%v) = %d, want %d", tc.enc, got, tc.want)
		}
	}
}
