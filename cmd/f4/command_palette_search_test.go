package main

import (
	"reflect"
	"testing"
)

func TestRankCommandPaletteEntriesUnicodeAndEnglishFallback(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "settings.plugins", Label: "&Настройки плагинов", EnglishLabel: "Plugin settings", Category: "Настройки"},
		{Key: "settings.panel", Label: "Настройки панели", EnglishLabel: "Panel settings", Category: "Настройки"},
		{Key: "file.copy", Label: "Копировать", EnglishLabel: "Copy", Category: "Файлы"},
	}

	for _, test := range []struct {
		name  string
		query string
		want  string
	}{
		{name: "russian case and accelerator", query: "НАСТРОЙКИ   ПЛАГИНОВ", want: "settings.plugins"},
		{name: "russian transposition typo", query: "плагниов", want: "settings.plugins"},
		{name: "english fallback", query: "plugin settings", want: "settings.plugins"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := rankCommandPaletteEntries(entries, test.query, nil)
			if len(got) == 0 || got[0].Key != test.want {
				t.Fatalf("rank(%q) = %#v, want first key %q", test.query, commandPaletteTestKeys(got), test.want)
			}
		})
	}
}

func TestRankCommandPaletteEntriesTypoAndSubsequence(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "settings", Label: "Settings", Category: "Application"},
		{Key: "command.palette", Label: "Command Palette", Category: "Application"},
		{Key: "unrelated", Label: "Unrelated", Category: "Application"},
	}

	typo := rankCommandPaletteEntries(entries, "settigns", nil)
	if got := commandPaletteTestKeys(typo); !reflect.DeepEqual(got, []string{"settings"}) {
		t.Fatalf("transposition typo result = %v, want [settings]", got)
	}

	subsequence := rankCommandPaletteEntries(entries, "cmdplte", nil)
	if got := commandPaletteTestKeys(subsequence); !reflect.DeepEqual(got, []string{"command.palette"}) {
		t.Fatalf("Unicode subsequence result = %v, want [command.palette]", got)
	}
}

func TestRankCommandPaletteEntriesRequiresAllTokensAcrossFields(t *testing.T) {
	entries := []commandPaletteEntry{
		{
			Key:          "plugin.configure",
			Label:        "Configure",
			ID:           "media.info",
			Category:     "Plugins",
			Description:  "Change metadata settings",
			SearchFields: []string{"audio video"},
			Shortcut:     "Ctrl+M",
		},
		{Key: "plugin.other", Label: "Configure", ID: "other", Category: "Plugins"},
	}

	got := rankCommandPaletteEntries(entries, "media plugins metadata audio ctrl+m", nil)
	if keys := commandPaletteTestKeys(got); !reflect.DeepEqual(keys, []string{"plugin.configure"}) {
		t.Fatalf("cross-field result = %v, want [plugin.configure]", keys)
	}

	if got := rankCommandPaletteEntries(entries, "media missing", nil); len(got) != 0 {
		t.Fatalf("entry missing one query token was retained: %v", commandPaletteTestKeys(got))
	}
}

func TestRankCommandPaletteEntriesEmptyQueryMRUThenStableOrder(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "z", Label: "Zulu", Category: "Files"},
		{Key: "a", Label: "Alpha", Category: "Files"},
		{Key: "b", Label: "Beta", Category: "Application"},
		{Key: "c", Label: "Charlie", Category: "Application"},
	}

	got := rankCommandPaletteEntries(entries, "  &&  ", []string{"z", "b", "z", "not-present"})
	want := []string{"z", "b", "c", "a"}
	if keys := commandPaletteTestKeys(got); !reflect.DeepEqual(keys, want) {
		t.Fatalf("empty-query ordering = %v, want %v", keys, want)
	}

	// A second call must not depend on map iteration or mutate the input.
	again := rankCommandPaletteEntries(entries, "", []string{"z", "b"})
	if keys := commandPaletteTestKeys(again); !reflect.DeepEqual(keys, want) {
		t.Fatalf("second empty-query ordering = %v, want %v", keys, want)
	}
	if keys := commandPaletteTestKeys(entries); !reflect.DeepEqual(keys, []string{"z", "a", "b", "c"}) {
		t.Fatalf("ranker mutated input order: %v", keys)
	}
}

func TestRankCommandPaletteEntriesNonEmptyMRUIsOnlyTieBreak(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "strong", Label: "Plugin Set", Category: "Application"},
		{Key: "tie-a", Label: "Plugin Set Extra", Category: "Application"},
		{Key: "tie-b", Label: "Plugin Set Extra", Category: "Application"},
		{Key: "weak", Label: "Open", Description: "xxplugin yyset", Category: "Application"},
	}

	got := rankCommandPaletteEntries(entries, "plugin set", []string{"weak", "tie-b", "tie-a"})
	want := []string{"strong", "tie-b", "tie-a", "weak"}
	if keys := commandPaletteTestKeys(got); !reflect.DeepEqual(keys, want) {
		t.Fatalf("non-empty MRU ordering = %v, want %v", keys, want)
	}
}

func TestNormalizeCommandPaletteText(t *testing.T) {
	if got, want := normalizeCommandPaletteText("  &ПЛАГИН\t\n Settings  "), "плагин settings"; got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
}

func commandPaletteTestKeys(entries []commandPaletteEntry) []string {
	keys := make([]string, len(entries))
	for index := range entries {
		keys[index] = entries[index].Key
	}
	return keys
}
