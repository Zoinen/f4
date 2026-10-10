package config

import (
	"path/filepath"
	"testing"
)

func TestClipboardImageSettingsRoundTrip(t *testing.T) {
	before, path := App, GetUserConfigIniPath
	defer func() { App = before; GetUserConfigIniPath = path }()
	dir := t.TempDir()
	GetUserConfigIniPath = func() string { return filepath.Join(dir, "settings.ini") }
	App = DefaultConfig()
	App.ClipboardImageFormat = "jpeg"
	App.ClipboardImagePNGCompression = "best"
	App.ClipboardImageJPEGQuality = 77
	App.ClipboardImagePrefix = "photo"
	App.ClipboardImageTemplate = "!{prefix}!_!{seq}!"
	App.ClipboardImageDigitFormat = "00000"
	SaveConfig()
	App = DefaultConfig()
	LoadConfig()
	if App.ClipboardImageFormat != "jpeg" || App.ClipboardImagePNGCompression != "best" || App.ClipboardImageJPEGQuality != 77 || App.ClipboardImagePrefix != "photo" || App.ClipboardImageTemplate != "!{prefix}!_!{seq}!" || App.ClipboardImageDigitFormat != "00000" {
		t.Fatal("clipboard image preferences did not round trip")
	}
}
