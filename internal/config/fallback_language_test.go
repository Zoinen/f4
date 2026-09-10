package config

import (
	"path/filepath"
	"testing"
)

func TestConfig_FallbackLanguagePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	oldCfg := App
	oldGetUserConfigIniPath := GetUserConfigIniPath
	oldGetConfigPaths := GetConfigIniPaths
	defer func() {
		App = oldCfg
		GetUserConfigIniPath = oldGetUserConfigIniPath
		GetConfigIniPaths = oldGetConfigPaths
	}()
	GetUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}
	GetConfigIniPaths = func() []string {
		return []string{filepath.Join(tmpDir, "settings.ini")}
	}

	App.Language = "ka"
	App.FallbackLanguage = "ru"
	SaveConfig()

	// Reset in-memory values
	App.Language = ""
	App.FallbackLanguage = ""

	LoadConfig()

	if App.Language != "ka" {
		t.Errorf("expected Primary Language 'ka', got '%s'", App.Language)
	}
	if App.FallbackLanguage != "ru" {
		t.Errorf("expected Fallback Language 'ru', got '%s'", App.FallbackLanguage)
	}
}
