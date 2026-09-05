package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type pinnedHostIdentity struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

func verifyPinnedHost(path string) (pinnedHostIdentity, error) {
	if path == "" {
		return pinnedHostIdentity{}, fmt.Errorf("pinned OpenConsole.exe path is required")
	}
	if !strings.EqualFold(filepath.Base(path), "OpenConsole.exe") {
		return pinnedHostIdentity{}, fmt.Errorf("refusing non-OpenConsole host %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return pinnedHostIdentity{}, fmt.Errorf("pinned OpenConsole.exe is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return pinnedHostIdentity{}, fmt.Errorf("pinned host is not a regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return pinnedHostIdentity{}, fmt.Errorf("open pinned host: %w", err)
	}
	defer file.Close()
	hash := newSHA256()
	if _, err := io.Copy(hash, file); err != nil {
		return pinnedHostIdentity{}, fmt.Errorf("hash pinned host: %w", err)
	}
	actualSHA := hashHex(hash)
	if actualSHA != pinnedOpenConsoleSHA256 {
		return pinnedHostIdentity{Path: path, SHA256: actualSHA}, fmt.Errorf("pinned host SHA-256 mismatch: got %s want %s", actualSHA, pinnedOpenConsoleSHA256)
	}
	version, err := readHostProductVersion(path)
	if err != nil {
		return pinnedHostIdentity{Path: path, SHA256: actualSHA}, err
	}
	if version != pinnedOpenConsoleVersion {
		return pinnedHostIdentity{Path: path, Version: version, SHA256: actualSHA}, fmt.Errorf("pinned host version mismatch: got %q want %q", version, pinnedOpenConsoleVersion)
	}
	return pinnedHostIdentity{Path: path, Version: version, SHA256: actualSHA}, nil
}
