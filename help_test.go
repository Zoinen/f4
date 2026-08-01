package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

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

	expectedStickyHeader := "f4 (an experimental Far Manager / far2l clone in Go)"
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
