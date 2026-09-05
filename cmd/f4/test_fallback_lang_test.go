package main

import (
	"path/filepath"
	"testing"
)

func TestConfig_FallbackLanguagePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	oldCfg := AppConfig
	oldGetUserConfigIniPath := getUserConfigIniPath
	oldGetConfigPaths := getConfigIniPaths
	defer func() {
		AppConfig = oldCfg
		getUserConfigIniPath = oldGetUserConfigIniPath
		getConfigIniPaths = oldGetConfigPaths
	}()
	getUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}
	getConfigIniPaths = func() []string {
		return []string{filepath.Join(tmpDir, "settings.ini")}
	}

	AppConfig.Language = "ka"
	AppConfig.FallbackLanguage = "ru"
	SaveConfig()

	// Reset in-memory values
	AppConfig.Language = ""
	AppConfig.FallbackLanguage = ""

	LoadConfig()

	if AppConfig.Language != "ka" {
		t.Errorf("expected Primary Language 'ka', got '%s'", AppConfig.Language)
	}
	if AppConfig.FallbackLanguage != "ru" {
		t.Errorf("expected Fallback Language 'ru', got '%s'", AppConfig.FallbackLanguage)
	}
}
