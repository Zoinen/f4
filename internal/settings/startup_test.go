package settings

import (
	"github.com/unxed/f4/internal/config"
	"testing"
)

func TestStartupModeUsesPersistedChoiceNames(t *testing.T) {
	for _, tc := range []struct {
		mode  config.StartupMode
		value string
	}{{config.StartupModeAuto, "auto"}, {config.StartupModeTTY, "tty"}, {config.StartupModeGui, "gui"}} {
		cfg := config.F4Config{StartupMode: tc.mode}
		if got := coreSettingValue(cfg, "StartupMode"); got != tc.value {
			t.Fatalf("startup choice=%q, want %q", got, tc.value)
		}
		next := config.F4Config{}
		if err := setCoreSetting(&next, "StartupMode", tc.value); err != nil {
			t.Fatal(err)
		}
		if next.StartupMode != tc.mode {
			t.Fatalf("startup mode=%v, want %v", next.StartupMode, tc.mode)
		}
	}
}
