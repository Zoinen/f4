package settings

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/sdk/f4settings"
)

// f4#1768 step 6c: the idle time of the administrator rights (step 6a) is an
// integer of Settings Center, on the operations page, and it reads and writes
// config.App.ElevationIdleSeconds, the [Elevation] IdleSeconds of settings.ini.
func TestElevationIdleSecondsIsASettingsCenterField(t *testing.T) {
	var field *f4settings.Field
	for _, f := range coreSettingsStaticFields() {
		if f.ID == "ElevationIdleSeconds" {
			f := f
			field = &f
		}
	}
	if field == nil {
		t.Fatal("Settings Center has no ElevationIdleSeconds field")
	}
	if field.Category != "operations" || field.Kind != f4settings.Integer {
		t.Errorf("ElevationIdleSeconds: category %q, kind %q; want operations, integer", field.Category, field.Kind)
	}

	cfg := config.App
	cfg.ElevationIdleSeconds = config.DefaultElevationIdleSeconds
	if got := coreSettingValue(cfg, "ElevationIdleSeconds"); got != "120" {
		t.Errorf("value shown %q, want 120", got)
	}
	if err := setCoreSetting(&cfg, "ElevationIdleSeconds", "0"); err != nil || cfg.ElevationIdleSeconds != 0 {
		t.Errorf("set 0: err %v, ElevationIdleSeconds %d", err, cfg.ElevationIdleSeconds)
	}
}
