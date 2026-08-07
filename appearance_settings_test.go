package main

import (
	"path/filepath"
	"testing"
)

func TestEnforceColorCorrection_ConfigRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	oldGetConfig := getUserConfigIniPath
	defer func() { getUserConfigIniPath = oldGetConfig }()
	getUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}

	AppConfig.EnforceColorCorrection = true
	SaveConfig()

	LoadConfig()
	if !AppConfig.EnforceColorCorrection {
		t.Errorf("expected EnforceColorCorrection to be saved as true and loaded as true")
	}

	AppConfig.EnforceColorCorrection = false
	SaveConfig()

	LoadConfig()
	if AppConfig.EnforceColorCorrection {
		t.Errorf("expected EnforceColorCorrection to be saved as false and loaded as false")
	}
}
