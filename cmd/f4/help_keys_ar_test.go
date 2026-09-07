package main

import (
	"strings"
	"testing"
)

func TestGenerateKeysHelpTopic_Arabic(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	oldLang := AppConfig.Language
	defer func() {
		AppConfig.Language = oldLang
		InitLang()
	}()
	AppConfig.Language = "ar"
	InitLang()

	topic := generateKeysHelpTopic("PanelNav", "t", []string{"Shell"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "فتح الملف في المستعرض") {
		t.Errorf("Expected Arabic description in generated topic\n---\n%s", joined)
	}

	topic2 := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if !strings.Contains(strings.Join(topic2.Lines, "\n"), "حفظ الملف") {
		t.Errorf("Expected Arabic editor description in generated topic")
	}
}

func TestGenerateKeysHelpTopic_HelpLanguageOverridesUI_Arabic(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	oldLang := AppConfig.Language
	defer func() {
		AppConfig.Language = oldLang
		InitLang()
	}()
	AppConfig.Language = "en"
	InitLang()

	oldStrings := helpActionStrings
	defer func() { helpActionStrings = oldStrings }()
	helpActionStrings = loadHelpLangStrings("ar")
	if helpActionStrings == nil {
		t.Fatal("Arabic help strings not found")
	}

	topic := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "حفظ الملف") {
		t.Errorf("Expected Arabic description with English UI\n---\n%s", joined)
	}
	if !strings.Contains(joined, "المحرر:") {
		t.Errorf("Expected Arabic area header\n---\n%s", joined)
	}
}
