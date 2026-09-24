package app

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

func TestColorerDialogFieldHelpers(t *testing.T) {
	if got := padLabels("&Type", "Parameter", "Ж"); !reflect.DeepEqual(got, []string{"&Type     ", "Parameter", "Ж        "}) {
		t.Fatalf("padLabels=%q", got)
	}

	cb := vtui.NewComboBox(0, 0, 20, []string{"old"})
	setComboItems(cb, []string{"one", "two"}, 1, "two")
	if got := cb.Menu.Items; len(got) != 2 || got[0].Text != "one" || got[1].Text != "two" {
		t.Fatalf("combo items=%v", got)
	}
	if cb.Menu.SelectPos != 1 || cb.Edit.GetText() != "two" {
		t.Fatalf("combo selection=%d text=%q", cb.Menu.SelectPos, cb.Edit.GetText())
	}
	setComboItems(cb, nil, -1, "")
	if len(cb.Menu.Items) != 0 || cb.Edit.GetText() != "" {
		t.Fatalf("empty combo items=%v text=%q", cb.Menu.Items, cb.Edit.GetText())
	}
}

func TestActionColorerSettingsBuildsAndLaysOutDialog(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	saved := config.App
	t.Cleanup(func() { config.App = saved })
	config.App.EditorHighlighter = "None"
	config.App.EditorCrossMode = -1
	config.App.EditorColorerCatalog = "  "
	config.App.EditorColorerUserHrc = ""
	config.App.EditorColorerUserHrd = ""
	config.App.EditorColorerHrcSettings = ""
	actionColorerSettings(nil)
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("actionColorerSettings did not push a dialog")
	}
	container, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("colorer settings frame=%T, want container", top)
	}
	if container == nil {
		t.Fatal("colorer settings container is nil")
	}
	vtui.FrameManager.Pop()
}

func TestActionColorerTypeSettingsReportsMissingCatalog(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	actionColorerTypeSettings(editor.ColorerSource{ConfigsDir: t.TempDir()})
	if top := vtui.FrameManager.GetTopFrame(); top == nil {
		t.Fatal("missing Colorer catalog did not produce a message")
	} else {
		top.Close()
	}
}
