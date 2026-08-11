package main

import (
	"strings"
	"testing"
)

func TestGenerateKeysHelpTopic_Hebrew(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	oldLang := AppConfig.Language
	defer func() {
		AppConfig.Language = oldLang
		InitLang()
	}()
	AppConfig.Language = "he"
	InitLang()

	topic := generateKeysHelpTopic("PanelNav", "t", []string{"Shell"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "פתיחת קובץ בצופה") {
		t.Errorf("Expected Hebrew description in generated topic\n---\n%s", joined)
	}

	topic2 := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if !strings.Contains(strings.Join(topic2.Lines, "\n"), "שמירת קובץ") {
		t.Errorf("Expected Hebrew editor description in generated topic")
	}
}

func TestGenerateKeysHelpTopic_HelpLanguageOverridesUI_Hebrew(t *testing.T) {
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
	helpActionStrings = loadHelpLangStrings("he")
	if helpActionStrings == nil {
		t.Fatal("Hebrew help strings not found")
	}

	topic := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "שמירת קובץ") {
		t.Errorf("Expected Hebrew description with English UI\n---\n%s", joined)
	}
	if !strings.Contains(joined, "עורך:") {
		t.Errorf("Expected Hebrew area header\n---\n%s", joined)
	}
}
