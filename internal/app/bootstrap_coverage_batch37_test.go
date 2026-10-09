package app

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/panel"
)

func batch37Workspace(number int, left, right string, active int) panel.WorkspaceSessionState {
	return panel.WorkspaceSessionState{
		Number:      number,
		Left:        panel.PanelSessionState{Path: left, Cursor: left + "-cursor", ViewMode: 1},
		Right:       panel.PanelSessionState{Path: right, Cursor: right + "-cursor", ViewMode: 2},
		ActivePanel: active,
		ShowPanels:  true,
	}
}

func TestMergeWorkspaceSessionSaveUsesCurrentWhenNoPreviousCoverageBatch37(t *testing.T) {
	current := []panel.WorkspaceSessionState{batch37Workspace(1, "new-left", "new-right", 1)}
	got, active := mergeWorkspaceSessionSave(nil, 4, current, 2, false, false)
	if !reflect.DeepEqual(got, current) || active != 2 {
		t.Fatalf("empty previous = %#v, %d; want current and active 2", got, active)
	}
}

func TestMergeWorkspaceSessionSaveUsesCurrentWhenBothGroupsSavedCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(1, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{batch37Workspace(1, "new-left", "new-right", 1)}
	got, active := mergeWorkspaceSessionSave(previous, 0, current, 3, true, true)
	if !reflect.DeepEqual(got, current) || active != 3 {
		t.Fatalf("both groups saved = %#v, %d; want current and active 3", got, active)
	}
}

func TestMergeWorkspaceSessionSavePreservesPreviousWhenNothingSavedCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(1, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{batch37Workspace(1, "new-left", "new-right", 1)}
	got, active := mergeWorkspaceSessionSave(previous, 4, current, 2, false, false)
	if !reflect.DeepEqual(got, previous) || active != 4 {
		t.Fatalf("nothing saved = %#v, %d; want previous and active 4", got, active)
	}
}

func TestMergeWorkspaceSessionSaveMatchesNumberAndPreservesPathsCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(7, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{batch37Workspace(7, "new-left", "new-right", 1)}
	got, active := mergeWorkspaceSessionSave(previous, 0, current, 1, true, false)
	if active != 1 || len(got) != 1 || got[0].ActivePanel != 1 || got[0].Left.Path != "old-left" || got[0].Right.Path != "old-right" {
		t.Fatalf("matched workspace = %#v, active %d; want current metadata and previous paths", got, active)
	}
}

func TestMergeWorkspaceSessionSaveMatchesByIndexWhenNumberIsZeroCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(7, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{batch37Workspace(0, "new-left", "new-right", 1)}
	got, _ := mergeWorkspaceSessionSave(previous, 0, current, 0, true, false)
	if len(got) != 1 || got[0].Number != 0 || got[0].Left.Path != "old-left" || got[0].ActivePanel != 1 {
		t.Fatalf("index fallback = %#v; want current state with previous paths", got)
	}
}

func TestMergeWorkspaceSessionSaveAppendsNewWorkspaceCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(1, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{
		batch37Workspace(1, "new-left", "new-right", 1),
		batch37Workspace(9, "extra-left", "extra-right", 0),
	}
	got, active := mergeWorkspaceSessionSave(previous, 0, current, 1, true, false)
	if active != 1 || len(got) != 2 || got[1].Number != 9 || got[1].Left.Path != "extra-left" {
		t.Fatalf("appended workspace = %#v, active %d", got, active)
	}
}

func TestMergeWorkspaceSessionSaveDropsNewWorkspaceWhenPanelSettingsOffCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(1, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{
		batch37Workspace(1, "new-left", "new-right", 1),
		batch37Workspace(9, "extra-left", "extra-right", 1),
	}
	got, active := mergeWorkspaceSessionSave(previous, 4, current, 1, false, true)
	if active != 4 || len(got) != 1 || got[0].Number != 1 || got[0].Left.Path != "new-left" {
		t.Fatalf("unmatched workspace with panel settings off = %#v, active %d", got, active)
	}
}

func TestMergeWorkspaceSessionSaveUpdatesOnlyCurrentPanelCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(1, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{batch37Workspace(1, "new-left", "new-right", 1)}
	got, active := mergeWorkspaceSessionSave(previous, 4, current, 1, false, true)
	if active != 4 || len(got) != 1 || got[0].ActivePanel != 0 || got[0].Left.Path != "new-left" || got[0].Left.Cursor != "new-left-cursor" || got[0].Right.Path != "new-right" {
		t.Fatalf("current-panel merge = %#v, active %d; want old metadata and new paths", got, active)
	}
}

func TestMergeWorkspaceSessionSaveUpdatesOnlyPanelSettingsCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{batch37Workspace(1, "old-left", "old-right", 0)}
	current := []panel.WorkspaceSessionState{batch37Workspace(1, "new-left", "new-right", 1)}
	got, active := mergeWorkspaceSessionSave(previous, 4, current, 1, true, false)
	if active != 1 || len(got) != 1 || got[0].ActivePanel != 1 || got[0].Left.Path != "old-left" || got[0].Right.Path != "old-right" {
		t.Fatalf("panel-settings merge = %#v, active %d; want new metadata and old paths", got, active)
	}
}

func TestMergeWorkspaceSessionSaveMatchesReorderedNumbersCoverageBatch37(t *testing.T) {
	previous := []panel.WorkspaceSessionState{
		batch37Workspace(10, "old-ten", "old-ten-right", 0),
		batch37Workspace(20, "old-twenty", "old-twenty-right", 0),
	}
	current := []panel.WorkspaceSessionState{
		batch37Workspace(20, "new-twenty", "new-twenty-right", 1),
		batch37Workspace(10, "new-ten", "new-ten-right", 1),
	}
	got, _ := mergeWorkspaceSessionSave(previous, 0, current, 1, true, false)
	if len(got) != 2 || got[0].Number != 10 || got[0].Left.Path != "old-ten" || got[1].Number != 20 || got[1].Left.Path != "old-twenty" {
		t.Fatalf("reordered-number merge = %#v; want paths retained by workspace number", got)
	}
}
