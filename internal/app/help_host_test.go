package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpLanguageSwitch(t *testing.T) {
	tempDir := t.TempDir()
	oldHelpEngine := vtui.GlobalHelpEngine
	oldHelpActionStrings := dialog.HelpActionStrings
	t.Cleanup(func() {
		vtui.GlobalHelpEngine = oldHelpEngine
		dialog.HelpActionStrings = oldHelpActionStrings
	})

	err := os.MkdirAll(filepath.Join(tempDir, "help"), 0700)
	if err != nil {
		t.Fatalf("failed to create help dir: %v", err)
	}

	ruHelpContent := "@TestTopic\n$RU Title\nRussian Help Content\n"
	err = os.WriteFile(filepath.Join(tempDir, "help", "ru.hlf"), []byte(ruHelpContent), 0600)
	if err != nil {
		t.Fatalf("failed to write test hlf: %v", err)
	}

	// Ensure config.GetF4ConfigDir's once-only detector cannot overwrite the test
	// override on its first call from InitHelpSystem.
	_ = config.GetF4ConfigDir()
	oldF4ConfigDir := config.CachedF4ConfigDir
	oldHelpLanguage := config.App.HelpLanguage
	oldUseLocalLanguageFiles := config.App.UseLocalLanguageFiles
	config.CachedF4ConfigDir = tempDir
	t.Cleanup(func() {
		config.CachedF4ConfigDir = oldF4ConfigDir
		config.App.HelpLanguage = oldHelpLanguage
		config.App.UseLocalLanguageFiles = oldUseLocalLanguageFiles
	})

	config.App.HelpLanguage = "ru"
	config.App.UseLocalLanguageFiles = true
	InitHelpSystem()

	topic := vtui.GlobalHelpEngine.GetTopic("TestTopic")
	if topic == nil {
		t.Errorf("expected TestTopic to be loaded from ru.hlf")
	} else if topic.Name != "TestTopic" || len(topic.Lines) == 0 || !strings.Contains(topic.Lines[0], "Russian Help Content") {
		t.Logf("Found TestTopic: %+v", topic)
	}
}

func TestHelpSystem_Initialization(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	InitHelpSystem()

	if vtui.GlobalHelpEngine == nil {
		t.Fatal("GlobalHelpEngine was not initialized")
	}

	contents := vtui.GlobalHelpEngine.GetTopic("Contents")
	if contents == nil {
		t.Fatal("Contents topic not found in HelpEngine")
	}

	readme := vtui.GlobalHelpEngine.GetTopic("README")
	if readme == nil {
		t.Fatal("README topic not found in HelpEngine")
	}

	// Verify that the markdown-to-HLF parser parsed the README correctly
	// The first H1 header should be treated as a sticky row (at index 0 without surrounding '#')
	if len(readme.Lines) == 0 {
		t.Fatal("README lines are empty")
	}

	expectedStickyHeader := "f4 — efficient and cozy file manager in go"
	if !strings.Contains(readme.Lines[0], expectedStickyHeader) {
		t.Errorf("Expected sticky header %q, got %q", expectedStickyHeader, readme.Lines[0])
	}

	// Other headers like '## Philosophy & Goals' should be formatted with surrounding '#'
	foundH2Header := false
	for _, line := range readme.Lines {
		if strings.Contains(line, "#Philosophy & Goals#") {
			foundH2Header = true
			break
		}
	}
	if !foundH2Header {
		t.Error("Markdown H2 header conversion failed")
	}
}
