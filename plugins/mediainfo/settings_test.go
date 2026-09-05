package mediainfo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSettingsStoreDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := newSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.snapshot(); got != DefaultSettings() {
		t.Fatalf("defaults = %#v, want %#v", got, DefaultSettings())
	}

	want := Settings{
		ShowInPluginMenu: false,
		EnableQuickView:  false,
		UseEditor:        true,
		Prefix:           "media_info-2",
		Language:         "RU",
		Template:         "General;%Format%",
	}
	if err := store.save(want); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	want.Language = "ru"
	if got := reloaded.snapshot(); got != want {
		t.Fatalf("reloaded = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "plugins", "mediainfo.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsConcurrentSavesKeepMemoryAndDiskInSync(t *testing.T) {
	store, err := newSettingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for index := 0; index < 16; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			settings := DefaultSettings()
			settings.Prefix = fmt.Sprintf("MediaInfo%c", 'A'+index)
			if err := store.save(settings); err != nil {
				t.Errorf("save %d: %v", index, err)
			}
		}(index)
	}
	group.Wait()

	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	var disk Settings
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if disk != store.snapshot() {
		t.Fatalf("disk settings = %#v, memory settings = %#v", disk, store.snapshot())
	}
}

func TestSettingsRejectInvalidPrefixAndOversizedTemplate(t *testing.T) {
	store, err := newSettingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	invalid := DefaultSettings()
	invalid.Prefix = "9 media"
	if err := store.save(invalid); err == nil {
		t.Fatal("invalid prefix was accepted")
	}
	invalid = DefaultSettings()
	invalid.Template = strings.Repeat("x", maxTemplateSize+1)
	if err := store.save(invalid); err == nil {
		t.Fatal("oversized template was accepted")
	}
}

func TestSettingsMalformedFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins", "mediainfo.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newSettingsStore(dir)
	if err == nil {
		t.Fatal("malformed settings did not report an error")
	}
	if got := store.snapshot(); got != DefaultSettings() {
		t.Fatalf("fallback = %#v, want defaults", got)
	}
}
