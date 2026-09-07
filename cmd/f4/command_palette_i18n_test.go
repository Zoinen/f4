package main

import (
	"reflect"
	"testing"

	"github.com/unxed/vtui"
)

func TestBuildCommandPaletteTranslationIndexDeduplicatesEquivalentAliases(t *testing.T) {
	packs := []vtui.LanguagePack{
		{Name: "one", Strings: map[string]string{"Action.Test": "&Settings"}},
		{Name: "two", Strings: map[string]string{"Action.Test": "settings"}},
		{Name: "three", Strings: map[string]string{"Action.Test": "Настройки"}},
	}
	index := buildCommandPaletteTranslationIndex(packs)
	if got, want := index["Action.Test"], []string{"&Settings", "Настройки"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("translation aliases = %#v, want %#v", got, want)
	}
}

func TestCommandPaletteMatchesEveryShippedTranslationWithoutChangingDisplay(t *testing.T) {
	const key = "Menu.PluginConfiguration"
	packs := LoadAllLanguagePacks()
	index := buildCommandPaletteTranslationIndex(packs)
	aliases := index[key]
	if len(aliases) < 2 {
		t.Fatalf("%s has only %d translated aliases", key, len(aliases))
	}

	const displayLabel = "CURRENT UI LABEL"
	entry := commandPaletteEntry{
		Key:          "settings.plugin-configuration",
		Label:        displayLabel,
		SearchFields: aliases,
	}
	checked := 0
	for _, pack := range packs {
		translation := pack.Strings[key]
		if normalizeCommandPaletteText(translation) == "" {
			continue
		}
		checked++
		results := rankCommandPaletteEntries([]commandPaletteEntry{entry}, translation, nil)
		if len(results) != 1 {
			t.Errorf("language %s translation %q did not find the command", pack.Name, translation)
			continue
		}
		if results[0].Label != displayLabel {
			t.Errorf("language %s changed display label to %q", pack.Name, results[0].Label)
		}
	}
	if checked < 2 {
		t.Fatalf("checked only %d shipped translations", checked)
	}
}

func TestCommandPaletteActionCatalogIndexesTranslationsButDisplaysCurrentLanguage(t *testing.T) {
	action, ok := GetAction("Settings.PluginConfiguration")
	if !ok {
		t.Fatal("Settings.PluginConfiguration is not registered")
	}
	wantLabel := plainLabel(action.DisplayLabel())

	var target commandPaletteEntry
	for _, entry := range commandPaletteActionEntries("Shell") {
		if entry.ID == action.Name {
			target = entry
			break
		}
	}
	if target.ID == "" {
		t.Fatal("plugin configuration is missing from palette catalog")
	}
	results := rankCommandPaletteEntries([]commandPaletteEntry{target}, "настройка плагинов", nil)
	if len(results) != 1 {
		t.Fatal("Russian translation did not find plugin configuration")
	}
	if results[0].Label != wantLabel {
		t.Fatalf("result label = %q, want current UI label %q", results[0].Label, wantLabel)
	}
}

func TestResetCommandPaletteTranslationsDropsStaleIndex(t *testing.T) {
	commandPaletteTranslationsCache.Lock()
	commandPaletteTranslationsCache.loaded = true
	commandPaletteTranslationsCache.byKey = map[string][]string{"stale": {"old value"}}
	commandPaletteTranslationsCache.Unlock()

	resetCommandPaletteTranslations()

	commandPaletteTranslationsCache.Lock()
	defer commandPaletteTranslationsCache.Unlock()
	if commandPaletteTranslationsCache.loaded || commandPaletteTranslationsCache.byKey != nil {
		t.Fatalf("translation cache was not reset: loaded=%v index=%#v", commandPaletteTranslationsCache.loaded, commandPaletteTranslationsCache.byKey)
	}
}
