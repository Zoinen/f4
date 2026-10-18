package archive

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/unxed/f4/internal/config"
)

type addArchivePreferences struct {
	KeepFileExtension *bool `json:"keep_file_extension"`
	// Defaults are the options the window opens with, kept by its "Save
	// settings" button (f4#2002); nil until then.
	Defaults *addArchiveProfile `json:"defaults,omitempty"`
}

var addArchivePreferencesPath = func() string {
	return filepath.Join(config.GetF4ConfigDir(), "archive_preferences.json")
}

func loadAddArchivePreferences() addArchivePreferences {
	var preferences addArchivePreferences
	data, err := os.ReadFile(addArchivePreferencesPath())
	if err != nil || json.Unmarshal(data, &preferences) != nil {
		return addArchivePreferences{}
	}
	return preferences
}

func saveAddArchivePreferences(preferences addArchivePreferences) error {
	data, err := json.Marshal(preferences)
	if err != nil {
		return err
	}
	path := addArchivePreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G703 -- path is in f4's private config directory.
		return err
	}
	return os.Rename(tmp, path)
}

func loadKeepArchiveFileExtension() bool {
	preferences := loadAddArchivePreferences()
	if preferences.KeepFileExtension == nil {
		return true
	}
	return *preferences.KeepFileExtension
}

// saveKeepArchiveFileExtension keeps the choice and leaves the rest of the
// preferences as they are.
func saveKeepArchiveFileExtension(keep bool) error {
	preferences := loadAddArchivePreferences()
	preferences.KeepFileExtension = &keep
	return saveAddArchivePreferences(preferences)
}

// saveAddArchiveDefaults keeps p and the file extension choice as what the
// window opens with.
func saveAddArchiveDefaults(p addArchiveProfile, keepExtension bool) error {
	preferences := loadAddArchivePreferences()
	preferences.Defaults = &p
	preferences.KeepFileExtension = &keepExtension
	return saveAddArchivePreferences(preferences)
}
