//go:build !lite

package unpack

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/unxed/sevenzip"
)

// TestSevenZipExtractor is TestExtractors' 7z case. It lives apart because a
// lite build has no 7z reader (formats_lite.go).
func TestSevenZipExtractor(t *testing.T) {
	binaryContent := []byte("fake_executable_data")
	pluginContent := []byte("plugin_data")
	badAbsPath := "/etc/passwd"

	var sevenBuf memoryWriteSeeker
	sw, err := sevenzip.NewWriter(&sevenBuf)
	if err != nil {
		t.Fatal(err)
	}
	sf1, err := sw.Create("f4.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf1.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	if err := sf1.Close(); err != nil {
		t.Fatal(err)
	}
	sf2, err := sw.Create("plugins/dummy.dll")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf2.Write(pluginContent); err != nil {
		t.Fatal(err)
	}
	if err := sf2.Close(); err != nil {
		t.Fatal(err)
	}
	sfBad, err := sw.Create(badAbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sfBad.Write([]byte("hacked")); err != nil {
		t.Fatal(err)
	}
	if err := sfBad.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sw.Close(); err != nil {
		t.Fatal(err)
	}
	sevenData := append([]byte(nil), sevenBuf.data...)
	dest7z := t.TempDir()
	err = SevenZip(sevenData, dest7z)
	if err != nil {
		t.Fatalf("SevenZip failed: %v", err)
	}
	b1, _ := os.ReadFile(filepath.Join(dest7z, "f4.exe"))
	b2, _ := os.ReadFile(filepath.Join(dest7z, "plugins", "dummy.dll"))
	if string(b1) != "fake_executable_data" || string(b2) != "plugin_data" {
		t.Errorf("7z extraction mismatch")
	}
	if _, err := os.Stat(filepath.Join(dest7z, "etc", "passwd")); !os.IsNotExist(err) {
		t.Error("7z Zip Slip vulnerability detected (absolute path extracted)!")
	}
	runtime.KeepAlive(sw)
}
