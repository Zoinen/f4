package main

import (
	"reflect"
	"testing"

	"github.com/unxed/vtui"
)

func TestParseFontconfigPathsDeduplicatesAndFilters(t *testing.T) {
	got := parseFontconfigPaths("/fonts/NotoSansCJK.ttc\n/fonts/NotoSansCJK.ttc\nnot-a-font.txt\n /fonts/Mono.ttf \n")
	want := []string{"/fonts/Mono.ttf", "/fonts/NotoSansCJK.ttc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFontconfigPaths = %#v, want %#v", got, want)
	}
}

func TestGuiFontChoicesPreserveManualValueAndCJKRecommendation(t *testing.T) {
	previous := discoverInstalledGuiFonts
	discoverInstalledGuiFonts = func(string) []string {
		return []string{"/fonts/NotoSansCJK.ttc", "/fonts/Mono.ttf", "/fonts/NotoSansCJK.ttc"}
	}
	t.Cleanup(func() { discoverInstalledGuiFonts = previous })

	got := guiFontChoices("zh", "/custom/font.otf")
	want := []string{"/custom/font.otf", "/fonts/NotoSansCJK.ttc", "/fonts/Mono.ttf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guiFontChoices = %#v, want %#v", got, want)
	}
	if shouldSuggestFontForLanguage("en", "") {
		t.Fatal("English must not trigger a CJK font recommendation")
	}
	if !shouldSuggestFontForLanguage("zh_CN", "") {
		t.Fatal("empty font must trigger a CJK font recommendation")
	}
	if shouldSuggestFontForLanguage("zh-CN", "/fonts/NotoSansCJK.ttc") {
		t.Fatal("already selected CJK font must not trigger another recommendation")
	}
	if !shouldSuggestFontForLanguage("ja", "/custom/font.otf") {
		t.Fatal("unknown manual font must trigger a CJK font recommendation")
	}
}

func TestGuiFontDisplayChoicesUseShortNamesAndKeepManualPaths(t *testing.T) {
	previous := discoverInstalledGuiFonts
	discoverInstalledGuiFonts = func(string) []string {
		return []string{"/fonts/NotoSansCJK.ttc", "/fonts/JetBrainsMono-Regular.ttf"}
	}
	t.Cleanup(func() { discoverInstalledGuiFonts = previous })

	got := guiFontDisplayChoices("en", "/custom/my-font.otf")
	want := []string{"/custom/my-font.otf", "NotoSansCJK", "JetBrainsMono-Regular"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guiFontDisplayChoices = %#v, want %#v", got, want)
	}
	if got := guiFontCurrentDisplayName("en", "/custom/my-font.otf"); got != "/custom/my-font.otf" {
		t.Fatalf("manual current font display = %q, want full path", got)
	}
	if got := guiFontValueForDisplay("en", "/custom/my-font.otf", "JetBrainsMono-Regular"); got != "/fonts/JetBrainsMono-Regular.ttf" {
		t.Fatalf("selected font value = %q, want catalog path", got)
	}
}

func TestFilterGuiFontDisplayChoicesMatchesSubstring(t *testing.T) {
	choices := []string{"JetBrainsMono-Regular", "NotoSansCJK", "Liberation Mono"}
	for _, test := range []struct {
		query string
		want  []string
	}{
		{query: "brain", want: []string{"JetBrainsMono-Regular"}},
		{query: "MONO", want: []string{"JetBrainsMono-Regular", "Liberation Mono"}},
		{query: "", want: choices},
	} {
		if got := filterGuiFontDisplayChoices(choices, test.query); !reflect.DeepEqual(got, test.want) {
			t.Errorf("filterGuiFontDisplayChoices(%q) = %#v, want %#v", test.query, got, test.want)
		}
	}
}

func TestConfigureGuiFontComboFiltersWhileKeepingManualInput(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager = nil

	choices := []string{"JetBrainsMono-Regular", "NotoSansCJK", "Liberation Mono"}
	combo := vtui.NewComboBox(0, 0, 30, choices)
	configureGuiFontCombo(combo, choices)

	if combo.DropdownOnly {
		t.Fatal("font combo must allow manual input")
	}
	combo.Edit.OnTextChange("brain")
	if len(combo.Menu.Items) != 1 || combo.Menu.Items[0].Text != "JetBrainsMono-Regular" {
		t.Fatalf("filtered font menu = %#v, want JetBrainsMono-Regular", combo.Menu.Items)
	}

	combo.Menu.OnAction(0)
	if got := combo.Edit.GetText(); got != "JetBrainsMono-Regular" {
		t.Fatalf("selected font text = %q, want JetBrainsMono-Regular", got)
	}
	if len(combo.Menu.Items) != len(choices) {
		t.Fatalf("font menu after selection has %d items, want %d", len(combo.Menu.Items), len(choices))
	}

	combo.Edit.SetText("/custom/font.ttf")
	if got := combo.Edit.GetText(); got != "/custom/font.ttf" {
		t.Fatalf("manual font text = %q, want full path", got)
	}
}

func TestGuiFontComboClosesMenuWhenTextIsEmpty(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		filteredCount int
		want          bool
	}{
		{name: "empty", text: "", filteredCount: 3, want: true},
		{name: "whitespace", text: "   ", filteredCount: 3, want: true},
		{name: "no matches", text: "missing", filteredCount: 0, want: true},
		{name: "matching text", text: "brain", filteredCount: 1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guiFontComboShouldCloseMenu(tt.text, tt.filteredCount); got != tt.want {
				t.Fatalf("guiFontComboShouldCloseMenu(%q, %d) = %v, want %v", tt.text, tt.filteredCount, got, tt.want)
			}
		})
	}
}

func TestFontconfigPatternByLanguage(t *testing.T) {
	for _, test := range []struct {
		language string
		want     string
	}{
		{language: "zh_CN", want: ":lang=zh"},
		{language: "ja-JP", want: ":lang=ja"},
		{language: "ko", want: ":lang=ko"},
		{language: "en", want: ":spacing=100"},
	} {
		if got := cjkFontconfigPattern(test.language); got != test.want {
			t.Errorf("cjkFontconfigPattern(%q) = %q, want %q", test.language, got, test.want)
		}
	}
}

func TestAppearanceSettingsFontComboRemainsEditable(t *testing.T) {
	previous := discoverInstalledGuiFonts
	discoverInstalledGuiFonts = func(string) []string { return []string{"/fonts/NotoSansCJK.ttc", "/fonts/Mono.ttf"} }
	t.Cleanup(func() { discoverInstalledGuiFonts = previous })

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	oldConfig := AppConfig
	oldPath := getUserConfigIniPath
	AppConfig.GuiFont = "/custom/font.otf"
	AppConfig.Language = "zh"
	getUserConfigIniPath = func() string { return t.TempDir() + "/settings.ini" }
	t.Cleanup(func() {
		AppConfig = oldConfig
		getUserConfigIniPath = oldPath
	})

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	actionAppearanceSettings(pf)
	top := vtui.FrameManager.GetTopFrame().(vtui.Container)

	var fontCombo *vtui.ComboBox
	for _, child := range top.GetChildren() {
		combo, ok := child.(*vtui.ComboBox)
		if !ok || len(combo.Menu.Items) == 0 {
			continue
		}
		if combo.Menu.Items[0].Text == "/custom/font.otf" {
			fontCombo = combo
			break
		}
	}
	if fontCombo == nil {
		t.Fatal("font catalog combobox not found")
	}
	if fontCombo.DropdownOnly {
		t.Fatal("font catalog combobox must preserve manual entry")
	}
	fontCombo.Edit.SetText("/manually/entered/font.ttf")
	clickDialogButton(t, top, "Ok")
	if AppConfig.GuiFont != "/manually/entered/font.ttf" {
		t.Fatalf("manual font path = %q", AppConfig.GuiFont)
	}
}
