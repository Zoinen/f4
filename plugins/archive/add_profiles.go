package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/unxed/f4/internal/config"
)

// addArchiveProfile is a named set of Add to archive options (f4#2002,
// part 5). It names the format, level and method by what they are rather
// than by their place in the lists, so that a profile survives a list that
// grows. The password is never stored.
type addArchiveProfile struct {
	Format         string `json:"format"` // extension, such as ".7z"
	Level          int    `json:"level"`  // compression level, 0 = default
	Method         string `json:"method,omitempty"`
	Solid          bool   `json:"solid,omitempty"`
	VolumeSize     string `json:"volume_size,omitempty"` // as typed, such as "700M"; "" = one file
	DeleteAfter    bool   `json:"delete_after,omitempty"`
	SkipUnreadable bool   `json:"skip_unreadable,omitempty"`
	Filter         string `json:"filter,omitempty"`
}

// addArchiveProfilesPath is where the profiles are kept; a variable so that
// tests keep theirs apart.
var addArchiveProfilesPath = func() string {
	return filepath.Join(config.GetF4ConfigDir(), "archive_profiles.json")
}

func loadAddArchiveProfiles() map[string]addArchiveProfile {
	profiles := map[string]addArchiveProfile{}
	data, err := os.ReadFile(addArchiveProfilesPath())
	if err != nil {
		return profiles
	}
	if json.Unmarshal(data, &profiles) != nil {
		return map[string]addArchiveProfile{}
	}
	return profiles
}

func saveAddArchiveProfiles(profiles map[string]addArchiveProfile) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	path := addArchiveProfilesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func addArchiveProfileNames(profiles map[string]addArchiveProfile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
