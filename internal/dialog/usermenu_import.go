package dialog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Far3UserMenuPath resolves a menu file, profile, or Far installation. User
// menus use the roaming profile, unlike history.db in the local profile.
func Far3UserMenuPath(source string) (string, error) {
	source = expandFar3Path(strings.Trim(strings.TrimSpace(source), "\""), "")
	if source == "" {
		return "", fmt.Errorf("enter a FarMenu.ini file, profile, or Far installation folder")
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Base(source), "Far.exe") {
			return source, nil
		}
		source = filepath.Dir(source)
	}
	direct := filepath.Join(source, "FarMenu.ini")
	if info, err := os.Stat(direct); err == nil && !info.IsDir() {
		return direct, nil
	}
	if _, err := os.Stat(filepath.Join(source, "Far.exe.ini")); err != nil {
		if _, err := os.Stat(filepath.Join(source, "Far.exe")); err != nil {
			return "", fmt.Errorf("FarMenu.ini was not found in %s", source)
		}
	}
	settings, err := readFar3INI(filepath.Join(source, "Far.exe.ini"))
	if err != nil {
		return "", err
	}
	profile := "%APPDATA%/Far Manager"
	if settings.GetString("General", "UseSystemProfiles", "1") == "0" {
		profile = settings.GetString("General", "UserProfileDir", "%FARHOME%/Profile")
	}
	profile = expandFar3Path(strings.Trim(profile, "\""), source)
	if !filepath.IsAbs(profile) {
		profile = filepath.Join(source, profile)
	}
	path := filepath.Join(profile, "FarMenu.ini")
	info, err = os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	return path, nil
}
