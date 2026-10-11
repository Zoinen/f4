package panel

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestHostDefaultCommandCallbacks(t *testing.T) {
	if AppCommand(nil, 0, nil) {
		t.Fatal("default AppCommand reported a handled command")
	}
	if RunAction("") {
		t.Fatal("default RunAction reported a handled action")
	}
	if items := BuildMenuBarItems(""); items != nil {
		t.Fatalf("default BuildMenuBarItems = %#v, want nil", items)
	}
	SaveSession()
}

func TestHostDefaultOpenCallbacks(t *testing.T) {
	OpenEditor(nil, nil, "")
	OpenViewer(nil, nil, "")
	OpenViewerInternal(nil, nil, "")
	OpenEditFileIn(nil, "")
}

func TestHostDefaultViewCallbacks(t *testing.T) {
	ShowViewer(nil, nil, "")
	ShowEditor(nil, nil, "", nil)
	if editor, index := FindOpenedEditor(nil, ""); editor != nil || index != -1 {
		t.Fatalf("default FindOpenedEditor = (%v, %d), want (nil, -1)", editor, index)
	}
}

func TestHostDefaultExecutionCallbacks(t *testing.T) {
	Execute(nil, nil, "", "", "")
	SortMenuForPanel(nil, nil)
	if WorkspaceClose() {
		t.Fatal("default WorkspaceClose reported success")
	}
	if Arkanoid() {
		t.Fatal("default Arkanoid reported success")
	}
}

func TestHostDefaultAreaAndKeyCallbacks(t *testing.T) {
	if area := CurrentArea(); area != "Common" {
		t.Fatalf("default CurrentArea = %q, want Common", area)
	}
	if MacroHotkey(nil) {
		t.Fatal("default MacroHotkey reported a consumed key")
	}
}

func TestHostDefaultAISetViewMode(t *testing.T) {
	old := AISetViewMode
	t.Cleanup(func() { AISetViewMode = old })
	called := false
	var panel FileSystemPanel
	AISetViewMode = func(got *FileSystemPanel, path string, isChat bool) {
		called = got == &panel && path == "context" && isChat
	}
	panel.AiSetViewMode("context", true)
	if !called {
		t.Fatal("AiSetViewMode was not forwarded")
	}
}

func TestHostDefaultKeyFilter(t *testing.T) {
	old := KeyFilter
	t.Cleanup(func() { KeyFilter = old })
	want := &vtinput.InputEvent{}
	var got *vtinput.InputEvent
	KeyFilter = func(event *vtinput.InputEvent) bool {
		got = event
		return true
	}
	if !KeyFilter(want) || got != want {
		t.Fatal("KeyFilter callback was not invoked")
	}
}

func TestHostDefaultSaveSessionCanBeReplaced(t *testing.T) {
	old := SaveSession
	t.Cleanup(func() { SaveSession = old })
	called := false
	SaveSession = func() { called = true }
	SaveSession()
	if !called {
		t.Fatal("SaveSession replacement was not invoked")
	}
}

func TestHostDefaultCurrentAreaCanBeReplaced(t *testing.T) {
	old := CurrentArea
	t.Cleanup(func() { CurrentArea = old })
	CurrentArea = func() string { return "Dialog" }
	if got := CurrentArea(); got != "Dialog" {
		t.Fatalf("replacement CurrentArea = %q", got)
	}
}

func TestHostDefaultMenuBuilderCanBeReplaced(t *testing.T) {
	old := BuildMenuBarItems
	t.Cleanup(func() { BuildMenuBarItems = old })
	want := []vtui.MenuBarItem{{}}
	BuildMenuBarItems = func(string) []vtui.MenuBarItem { return want }
	if got := BuildMenuBarItems("Shell"); len(got) != 1 {
		t.Fatalf("replacement BuildMenuBarItems length = %d, want 1", len(got))
	}
}
