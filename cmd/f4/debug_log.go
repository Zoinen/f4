package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func f4DebugLogPath(configDir string) string {
	return filepath.Join(configDir, "logs", "debug.log")
}

// configureF4DebugLogPath keeps zoin-bot's default diagnostics inside the
// active profile while preserving explicit VTUI_DEBUG file paths.
func configureF4DebugLogPath(configDir string) {
	switch os.Getenv("VTUI_DEBUG") {
	case "1", "true":
		path := f4DebugLogPath(configDir)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "f4: cannot create debug log directory %q: %v\n", filepath.Dir(path), err)
			return
		}
		_ = os.Setenv("VTUI_DEBUG", path)
	}
}
