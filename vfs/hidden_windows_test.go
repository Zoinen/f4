//go:build windows

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIsHiddenUsesWindowsAttributeInsteadOfDotPrefix(t *testing.T) {
	directory := t.TempDir()
	dotPath := filepath.Join(directory, ".visible")
	if err := os.WriteFile(dotPath, []byte("visible"), 0o600); err != nil {
		t.Fatal(err)
	}
	dotInfo, err := os.Stat(dotPath)
	if err != nil {
		t.Fatal(err)
	}
	if isHidden(dotPath, dotInfo.Name(), dotInfo) {
		t.Fatal("a dot-prefixed file without FILE_ATTRIBUTE_HIDDEN was hidden")
	}

	hiddenPath := filepath.Join(directory, "hidden.txt")
	if err := os.WriteFile(hiddenPath, []byte("hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	hiddenPathUTF16, err := syscall.UTF16PtrFromString(hiddenPath)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := syscall.GetFileAttributes(hiddenPathUTF16)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(
		hiddenPathUTF16, attributes|syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.SetFileAttributes(hiddenPathUTF16, attributes)
	})
	hiddenInfo, err := os.Stat(hiddenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !isHidden(hiddenPath, hiddenInfo.Name(), hiddenInfo) {
		t.Fatal("FILE_ATTRIBUTE_HIDDEN was not recognized")
	}
}

func TestOSVFSReadDirPhasedBasePreservesWindowsHiddenFiltering(t *testing.T) {
	directory := t.TempDir()
	hiddenPath := filepath.Join(directory, "hidden.txt")
	if err := os.WriteFile(hiddenPath, []byte("hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	hiddenPathUTF16, err := syscall.UTF16PtrFromString(hiddenPath)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := syscall.GetFileAttributes(hiddenPathUTF16)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(hiddenPathUTF16, attributes|syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.SetFileAttributes(hiddenPathUTF16, attributes) })

	v := NewOSVFS(directory)
	baseHidden := false
	metadataHidden := false
	if err := v.ReadDirPhased(context.Background(), directory, func(phase DirectoryReadPhase, items []VFSItem) {
		for _, item := range items {
			if item.Name != "hidden.txt" {
				continue
			}
			if phase == DirectoryReadBase {
				baseHidden = item.IsHidden
			} else {
				metadataHidden = item.IsHidden
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !baseHidden || !metadataHidden {
		t.Fatalf("hidden classification changed between phases: base=%v metadata=%v", baseHidden, metadataHidden)
	}
}
