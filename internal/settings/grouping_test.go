package settings

import (
	"context"
	"github.com/unxed/f4/internal/config"
	"testing"
)

func TestGroupSettingsValidationAndCancel(t *testing.T) {
	before := config.App
	defer func() { config.App = before }()
	config.App.PanelGroupSmallMiB, config.App.PanelGroupMediumMiB, config.App.PanelGroupLargeMiB = 5, 10, 100
	d, err := (coreSettingsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Values["PanelGroupSmallMiB"] = "10"
	if len(d.Validate()) != 3 {
		t.Fatal("equal limits accepted")
	}
	d.Values["PanelGroupSmallMiB"] = "999999999999999999999999"
	if len(d.Validate()) != 3 {
		t.Fatal("overflow accepted")
	}
	d.Values["PanelGroupSmallMiB"] = "4"
	if len(d.Validate()) != 0 {
		t.Fatal(d.Validate())
	}
	d.Close()
	if config.App.PanelGroupSmallMiB != 5 {
		t.Fatal("cancel applied limits")
	}
}

func TestGroupSettingsApply(t *testing.T) {
	before, writer := config.App, writeSettingsCandidate
	defer func() { config.App = before; writeSettingsCandidate = writer }()
	config.App.PanelGroupSmallMiB, config.App.PanelGroupMediumMiB, config.App.PanelGroupLargeMiB = 5, 10, 100
	writes := 0
	writeSettingsCandidate = func(config.F4Config, config.F4Config) error { writes++; return nil }
	d, _ := (coreSettingsProvider{}).Begin(context.Background())
	d.Values["PanelGroupSmallMiB"] = "11"
	if result := d.Commit(context.Background()); len(result.Errors) == 0 || writes != 0 {
		t.Fatal("invalid tuple applied")
	}
	d.Values["PanelGroupSmallMiB"] = "4"
	if result := d.Commit(context.Background()); len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	d.Values["PanelGroupSmallMiB"] = "3"
	d.Close()
	if writes != 1 || config.App.PanelGroupSmallMiB != 4 {
		t.Fatal("Apply/Cancel changed wrong baseline")
	}
}

func TestGroupSettingsDeepLinkFocus(t *testing.T) {
	p := coreSettingsProvider{}
	d, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: p.Catalog(), draft: d}})
	c.ResizeConsole(120, 40)
	c.navigate("panels", "", "PanelGroupSmallMiB", false)
	if item := c.page.GetFocusedItem(); item == nil || item.GetId() != "setting:PanelGroupSmallMiB" {
		t.Fatalf("focused %v", item)
	}
}
