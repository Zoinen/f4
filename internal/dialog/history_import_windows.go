package dialog

import (
	"os"
	"path/filepath"
)

func defaultFar3Location() string {
	for _, path := range []string{filepath.Join(os.Getenv("SystemDrive")+`\`, "Programs", "Far3"),
		filepath.Join(os.Getenv("ProgramFiles"), "Far Manager"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Far Manager")} {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}
