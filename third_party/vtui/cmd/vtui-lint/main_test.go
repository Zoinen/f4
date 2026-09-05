package main

import (
	"os"
	"path/filepath"
	"testing"
)

func findVuiFile(t *testing.T, name string) string {
	candidates := []string{
		filepath.Join("testdata", name),
		filepath.Join("..", "..", "testdata", name),
		filepath.Join("..", "testdata", name),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatalf("file %s not found in testdata", name)
	return ""
}

func TestLintFile_HelloVui(t *testing.T) {
	vuiPath := findVuiFile(t, "hello.vui")
	if err := lintFile(vuiPath); err != nil {
		t.Errorf("lintFile(%q) failed: %v", vuiPath, err)
	}
}

func TestLintFile_InvalidJson(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.vui")
	_ = os.WriteFile(badPath, []byte("invalid json content"), 0644)

	if err := lintFile(badPath); err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
