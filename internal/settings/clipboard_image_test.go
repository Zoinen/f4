package settings

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestClipboardImageSettingsValidationAndApply(t *testing.T) {
	before, writer := config.App, writeSettingsCandidate
	defer func() { config.App = before; writeSettingsCandidate = writer }()
	config.App = config.DefaultConfig()
	writes := 0
	writeSettingsCandidate = func(config.F4Config, config.F4Config) error { writes++; return nil }
	d, err := (coreSettingsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for id, value := range map[string]string{"ClipboardImageJPEGQuality": "101", "ClipboardImageTemplate": "no sequence", "ClipboardImageDigitFormat": "%03d", "ClipboardImagePrefix": "../bad", "ClipboardImageFormat": "webp"} {
		d.Values[id] = value
	}
	if failures := d.Validate(); len(failures) != 5 {
		t.Fatalf("validation = %v", failures)
	}
	if result := d.Commit(context.Background()); len(result.Errors) == 0 || writes != 0 {
		t.Fatal("invalid settings committed")
	}
	d.Close()
	if config.App.ClipboardImagePrefix != "screenshot" {
		t.Fatal("cancel changed settings")
	}
	d, err = (coreSettingsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Values["ClipboardImagePrefix"] = "photo"
	d.Values["ClipboardImageFormat"] = "jpeg"
	if result := d.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	d.Values["ClipboardImagePrefix"] = "unapplied"
	d.Close()
	if config.App.ClipboardImagePrefix != "photo" || config.App.ClipboardImageFormat != "jpeg" || writes != 1 {
		t.Fatal("Apply/Cancel baseline failed")
	}
}
