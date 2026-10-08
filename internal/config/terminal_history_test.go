package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/ini"
)

func TestTerminalHistoryInheritanceDefaultsOff(t *testing.T) {
	if DefaultConfig().InheritTerminalHistory {
		t.Fatal("new tabs inherit terminal history by default")
	}
	cfg := DefaultConfig()
	cfg.InheritTerminalHistory = true
	parseConfigInto(&cfg, ini.Parse(strings.NewReader("[Panel]\nShowHiddenFiles = 1\n")))
	if cfg.InheritTerminalHistory {
		t.Fatal("absent history preference did not restore the default")
	}
}

func TestTerminalHistoryInheritanceSaveAndLoad(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := DefaultConfig()
		cfg.InheritTerminalHistory = enabled
		data := SerializeSettingsConfig(cfg)
		if !bytes.Contains(data, []byte("InheritTerminalHistory = ")) {
			t.Fatal("terminal history preference is missing from settings.ini")
		}
		back := DefaultConfig()
		back.InheritTerminalHistory = !enabled
		parseConfigInto(&back, ini.Parse(bytes.NewReader(data)))
		if back.InheritTerminalHistory != enabled {
			t.Fatalf("history inheritance changed after save/load: got %v, want %v", back.InheritTerminalHistory, enabled)
		}
	}
}
