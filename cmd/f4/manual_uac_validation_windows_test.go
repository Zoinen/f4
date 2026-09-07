//go:build windows

package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// This opt-in test exercises the real ShellExecuteExW/runas path with a
// native f4 binary. It is intentionally not part of the normal test suite:
// running it may display a UAC consent prompt.
func TestManualUACElevationValidation(t *testing.T) {
	if os.Getenv("F4_RUN_MANUAL_UAC") != "1" {
		t.Skip("set F4_RUN_MANUAL_UAC=1 to exercise native UAC")
	}
	binary := os.Getenv("F4_VALIDATION_BINARY")
	if binary == "" {
		t.Fatal("F4_VALIDATION_BINARY must point to a native f4.exe")
	}

	marker := "f4-uac-validation-" + strconv.Itoa(os.Getpid()) + ".txt"
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	f, err := zw.Create(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("elevated update helper completed")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	oldExecutable := osExecutable
	osExecutable = func() (string, error) { return binary, nil }
	defer func() { osExecutable = oldExecutable }()

	if err := runElevatedUpdate(archive.Bytes(), "zip"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(binary), marker)
	if data, err := os.ReadFile(path); err != nil {
		t.Fatalf("elevated helper did not create %s: %v", path, err)
	} else if string(data) != "elevated update helper completed" {
		t.Fatalf("unexpected helper output: %q", data)
	}
}
