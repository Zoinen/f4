package main

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

// writeFileAtomically publishes a complete file without exposing a truncated
// target. The temporary file lives beside the target so Rename is atomic on
// filesystems that support atomic same-directory replacement.
func writeFileAtomically(path string, data []byte, mode os.FileMode) (returnErr error) {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".f4-atomic-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := f.Close(); returnErr == nil && closeErr != nil {
				returnErr = closeErr
			}
		}
		if returnErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := f.Chmod(mode); err != nil {
		return err
	}
	for len(data) > 0 {
		written, err := f.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	// The deferred Close is now harmless and the temp name is removed only if
	// publication fails.
	// Publication is a same-directory rename, atomic where it matters. On
	// Windows it still fails transiently: MoveFileEx reports access denied
	// while another writer is replacing the same target, and again while a
	// scanner holds the freshly closed temporary file open. Both clear on
	// their own, so this waits them out.
	//
	// A flat twenty attempts five milliseconds apart gave up after a tenth of
	// a second, which sixteen concurrent writers on a loaded runner outlast --
	// fourteen of them failed at once. The pause widens instead, so the common
	// case still costs a millisecond while the contended one gets seconds.
	var renameErr error
	deadline := time.Now().Add(5 * time.Second)
	for pause := time.Millisecond; ; {
		if renameErr = os.Rename(tmpPath, path); renameErr == nil {
			break
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(pause)
		if pause < 50*time.Millisecond {
			pause *= 2
		}
	}
	if renameErr != nil {
		return renameErr
	}
	returnErr = nil
	// Directory fsync is a best-effort durability improvement. It is not
	// available on every supported platform, while the same-directory rename
	// remains the atomic publication point.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
