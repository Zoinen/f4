package config

import (
	"path/filepath"
	"testing"
)

func TestEnforceColorCorrection_ConfigRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	oldCfg := App
	oldGetConfig := GetUserConfigIniPath
	defer func() {
		App = oldCfg
		GetUserConfigIniPath = oldGetConfig
	}()
	GetUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}

	App.EnforceColorCorrection = true
	SaveConfig()

	LoadConfig()
	if !App.EnforceColorCorrection {
		t.Errorf("expected EnforceColorCorrection to be saved as true and loaded as true")
	}

	App.EnforceColorCorrection = false
	SaveConfig()

	LoadConfig()
	if App.EnforceColorCorrection {
		t.Errorf("expected EnforceColorCorrection to be saved as false and loaded as false")
	}
}
