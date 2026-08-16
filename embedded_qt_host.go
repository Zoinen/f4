package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const embeddedQtHostCacheEnv = "F4_QT_HOST_CACHE_DIR"

func ensureEmbeddedQtHost() (string, error) {
	if len(embeddedQtHostGzip) == 0 || (runtime.GOOS != "linux" && runtime.GOOS != "windows") {
		return "", nil
	}

	cacheRoot := os.Getenv(embeddedQtHostCacheEnv)
	if cacheRoot == "" {
		var err error
		cacheRoot, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("resolve user cache for embedded Qt host: %w", err)
		}
	}
	return materializeEmbeddedQtHost(embeddedQtHostGzip, cacheRoot, runtime.GOOS)
}

func materializeEmbeddedQtHost(payload []byte, cacheRoot, goos string) (string, error) {
	if len(payload) == 0 {
		return "", nil
	}
	if cacheRoot == "" {
		return "", fmt.Errorf("embedded Qt host cache root is empty")
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	hostName := "f4-qt-host"
	if goos == "windows" {
		hostName += ".exe"
	}
	hostDir := filepath.Join(cacheRoot, "f4", "qt-host", digest)
	hostPath := filepath.Join(hostDir, hostName)
	if info, err := os.Lstat(hostPath); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("embedded Qt host cache path is not a regular file: %s", hostPath)
		}
		return hostPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect embedded Qt host cache: %w", err)
	}

	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return "", fmt.Errorf("create embedded Qt host cache: %w", err)
	}
	if err := os.Chmod(hostDir, 0o700); err != nil && goos != "windows" {
		return "", fmt.Errorf("secure embedded Qt host cache: %w", err)
	}

	temp, err := os.CreateTemp(hostDir, ".f4-qt-host-*")
	if err != nil {
		return "", fmt.Errorf("create temporary embedded Qt host: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := false
	defer func() {
		_ = temp.Close()
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o700); err != nil && goos != "windows" {
		return "", fmt.Errorf("make embedded Qt host executable: %w", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("open embedded Qt host payload: %w", err)
	}
	if _, err := io.Copy(temp, reader); err != nil {
		_ = reader.Close()
		return "", fmt.Errorf("extract embedded Qt host payload: %w", err)
	}
	if err := reader.Close(); err != nil {
		return "", fmt.Errorf("verify embedded Qt host payload: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("sync embedded Qt host payload: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close embedded Qt host payload: %w", err)
	}

	if err := os.Rename(tempPath, hostPath); err != nil {
		// Another process may have won the first-launch race. Its destination is
		// content-addressed from the same payload, so a regular file is valid.
		if info, statErr := os.Lstat(hostPath); statErr == nil && info.Mode().IsRegular() {
			return hostPath, nil
		}
		return "", fmt.Errorf("publish embedded Qt host: %w", err)
	}
	keepTemp = true
	return hostPath, nil
}
