package config

import (
	"github.com/unxed/f4/internal/ini"
	"strconv"
	"strings"
	"testing"
)

func TestGroupLimits(t *testing.T) {
	for _, tt := range []struct {
		a, b, c int
		valid   bool
	}{{5, 10, 100, true}, {1, 2, 3, true}, {0, 10, 100, false}, {5, 5, 100, false}, {10, 5, 100, false}, {-1, 5, 10, false}} {
		if got := ValidPanelGroupLimits(tt.a, tt.b, tt.c); got != tt.valid {
			t.Fatalf("%+v got %t", tt, got)
		}
	}
	if strconv.IntSize == 64 {
		n := int64(1) << 44
		if ValidPanelGroupLimits(1, 2, int(n)) {
			t.Fatal("byte overflow accepted")
		}
	}
	before := App
	defer func() { App = before }()
	App.PanelGroupSmallMiB = 0
	if got := PanelGroupLimits(); got != [3]int64{5 << 20, 10 << 20, 100 << 20} {
		t.Fatal(got)
	}
}

func TestGroupSavedLimits(t *testing.T) {
	for _, value := range []string{"0", "-5", "100", "999999999999999999999"} {
		var cfg F4Config
		parseConfigInto(&cfg, ini.Parse(strings.NewReader("[Panel]\nPanelGroupSmallMiB="+value+"\nPanelGroupMediumMiB=10\nPanelGroupLargeMiB=100\n")))
		if cfg.PanelGroupSmallMiB != 5 || cfg.PanelGroupMediumMiB != 10 || cfg.PanelGroupLargeMiB != 100 {
			t.Fatal(cfg.PanelGroupSmallMiB, cfg.PanelGroupMediumMiB, cfg.PanelGroupLargeMiB)
		}
	}
}
