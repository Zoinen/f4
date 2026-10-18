//go:build !extralite && !windows

package vfs

import (
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
)

// "us-ascii" in the WHATWG index, which the ASCII code page used to be read
// from, is windows-1252, the encoding it is now named as (f4#1671).
func TestASCIICodepageIsTheWHATWGOne(t *testing.T) {
	if enc, err := htmlindex.Get("us-ascii"); err != nil || enc != charmap.Windows1252 {
		t.Fatalf("htmlindex us-ascii = %v (%v), want windows-1252", enc, err)
	}
}

// The extra-lite build names the single-byte encodings from its own table;
// the names are the ones the IANA index gives, so both builds tell the same
// code pages apart.
func TestSingleByteIANANamesAreTheIndexOnes(t *testing.T) {
	for enc, want := range singleByteIANANames {
		if got, err := ianaindex.IANA.Name(enc); err != nil || got != want {
			t.Errorf("%v: table %q, IANA index %q (%v)", enc, want, got, err)
		}
	}
}
