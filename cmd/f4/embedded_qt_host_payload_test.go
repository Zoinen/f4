//go:build f4_embedded_qt_host

package main

import (
	"bytes"
	"os"
	"runtime"
	"testing"
)

func TestGeneratedEmbeddedQtHostPayload(t *testing.T) {
	if len(embeddedQtHostGzip) == 0 {
		t.Fatal("portable build tag produced an empty Qt host payload")
	}
	path, err := materializeEmbeddedQtHost(
		embeddedQtHostGzip, t.TempDir(), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < 4 {
		t.Fatalf("generated Qt host is only %d bytes", len(content))
	}
	wantMagic := []byte{0x7f, 'E', 'L', 'F'}
	switch runtime.GOOS {
	case "windows":
		wantMagic = []byte{'M', 'Z'}
	case "darwin":
		wantMagic = []byte{0xcf, 0xfa, 0xed, 0xfe}
	}
	if !bytes.HasPrefix(content, wantMagic) {
		t.Fatalf("generated Qt host has invalid executable magic % x", content[:4])
	}
}
