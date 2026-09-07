package vfs

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
)

var terminalExts = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true,
	".sh": true, ".bash": true, ".py": true, ".pl": true,
	".rb": true, ".js": true, ".php": true, ".lua": true,
}

// IsTerminalRunnable checks if a file can be executed in the built-in terminal.
func IsTerminalRunnable(ctx context.Context, v VFS, path string) bool {
	info, err := v.Stat(ctx, path)
	if err != nil || info.IsDir {
		return false
	}

	// 1. Check executable bit on Unix
	if runtime.GOOS != "windows" && info.IsExecutable {
		return true
	}

	// 2. Check common executable/script extensions
	ext := strings.ToLower(filepath.Ext(info.Name))
	if terminalExts[ext] {
		return true
	}

	// 3. Check for Shebang
	f, err := v.Open(ctx, path)
	if err == nil {
		defer func() {
			_ = f.Close() // The shebang probe is read-only.
		}()
		buf := make([]byte, 2)
		n, _ := f.Read(ctx, buf)
		if n == 2 && string(buf) == "#!" {
			return true
		}
	}

	return false
}
