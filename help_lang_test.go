package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestHelpLanguageSwitch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "f4-help-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = os.MkdirAll(filepath.Join(tempDir, "help"), 0755)
	if err != nil {
		t.Fatalf("failed to create help dir: %v", err)
	}

	ruHelpContent := "@TestTopic\n$RU Title\nRussian Help Content\n"
	err = os.WriteFile(filepath.Join(tempDir, "help", "ru.hlf"), []byte(ruHelpContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test hlf: %v", err)
	}

	oldF4ConfigDir := cachedF4ConfigDir
	cachedF4ConfigDir = tempDir
	defer func() { cachedF4ConfigDir = oldF4ConfigDir }()

	AppConfig.HelpLanguage = "ru"
	InitHelpSystem()

	topic := vtui.GlobalHelpEngine.GetTopic("TestTopic")
	if topic == nil {
		t.Errorf("expected TestTopic to be loaded from ru.hlf")
	} else if topic.Name != "TestTopic" || len(topic.Lines) == 0 || !strings.Contains(topic.Lines[0], "Russian Help Content") {
		t.Logf("Found TestTopic: %+v", topic)
	}
}
