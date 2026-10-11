//go:build lite

package unpack

import (
	"errors"
	"testing"
)

// TestLiteSevenZipIsUnsupported pins what a lite build does with a .7z: it
// refuses up front instead of writing anything, since it links no 7z reader.
// TestExtractors covers the ZIP and tar.gz readers lite does have.
func TestLiteSevenZipIsUnsupported(t *testing.T) {
	dest := t.TempDir()
	if err := SevenZip([]byte("7z\xbc\xaf\x27\x1c"), dest); !errors.Is(err, ErrSevenZipUnsupported) {
		t.Fatalf("SevenZip in a lite build = %v, want ErrSevenZipUnsupported", err)
	}
}
