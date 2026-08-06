package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// seedPanelForRestore wires up a PanelsFrame whose active panel shows the
// given entries and pushes it onto FrameManager so withPF handlers find it.
func seedPanelForRestore(t *testing.T, names []string) *PanelsFrame {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	t.Cleanup(func() { pf.Close() })
	pf.ResizeConsole(80, 25)

	fsp := pf.getActivePanel()
	fsp.vfs = vfs.NewOSVFS(".")

	entries := []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	for _, n := range names {
		entries = append(entries, &fileEntry{VFSItem: vfs.VFSItem{Name: n}})
	}
	fsp.entries = entries
	fsp.Refresh()

	vtui.FrameManager.Push(pf)
	return pf
}

func collectSelected(fsp *FileSystemPanel) []string {
	var out []string
	for _, e := range fsp.entries {
		if e.Selected {
			out = append(out, e.Name)
		}
	}
	return out
}

func TestFileSystemPanel_SaveRestoreSelection_Swaps(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"a.txt", "b.txt", "c.txt"})
	fsp := pf.getActivePanel()

	// Initial state: mark a.txt.
	fsp.SetItemSelected(1, true)

	// Take a snapshot and mutate: mark c.txt, unmark a.txt.
	fsp.SaveSelection()
	fsp.SetItemSelected(1, false)
	fsp.SetItemSelected(3, true)

	// First RestoreSelection: back to the snapshot state (a.txt marked, c.txt not).
	fsp.RestoreSelection()
	if got := collectSelected(fsp); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("after 1st restore: selected = %v, want [a.txt]", got)
	}

	// Second RestoreSelection: back to the state before the first restore
	// (c.txt marked, a.txt not). Ctrl+M is a two-way swap.
	fsp.RestoreSelection()
	if got := collectSelected(fsp); len(got) != 1 || got[0] != "c.txt" {
		t.Fatalf("after 2nd restore: selected = %v, want [c.txt]", got)
	}
}

func TestFileSystemPanel_RestoreSelection_LeavesParentAlone(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"a.txt"})
	fsp := pf.getActivePanel()

	// Force ".." into an impossible "PrevSelected=true" state — RestoreSelection
	// must not resurrect it into Selected. Guards against a regression where
	// far2l's Select() skip for parent-dir entries would not be honoured.
	fsp.entries[0].PrevSelected = true

	fsp.RestoreSelection()
	if fsp.entries[0].Selected {
		t.Error("RestoreSelection propagated selection to the '..' entry")
	}
}

func TestAction_PanelRestoreSelection_InvertRoundtrip(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"a.txt", "b.txt", "c.txt"})
	fsp := pf.getActivePanel()

	// Mark just a.txt.
	fsp.SetItemSelected(1, true)

	// Invert: expect {b, c} marked. InvertSelection must snapshot first.
	if !RunAction("Panel.InvertSelection") {
		t.Fatal("Panel.InvertSelection did not run")
	}
	got := collectSelected(fsp)
	if len(got) != 2 || got[0] != "b.txt" || got[1] != "c.txt" {
		t.Fatalf("after invert: selected = %v, want [b.txt c.txt]", got)
	}

	// Ctrl+M restores the pre-invert state: only a.txt marked.
	if !RunAction("Panel.RestoreSelection") {
		t.Fatal("Panel.RestoreSelection did not run")
	}
	got = collectSelected(fsp)
	if len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("after restore: selected = %v, want [a.txt]", got)
	}
}

func TestFileSystemPanel_ApplyMaskSelection_SnapshotsBeforeMutating(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"a.txt", "b.txt", "c.log"})
	fsp := pf.getActivePanel()

	// Mark a.txt as the starting state.
	fsp.SetItemSelected(1, true)

	// Mass-select all *.log: only c.log gets marked. a.txt loses no marks.
	fsp.ApplyMaskSelection("*.log", true)
	got := collectSelected(fsp)
	if len(got) != 2 || got[0] != "a.txt" || got[1] != "c.log" {
		t.Fatalf("after mask: selected = %v, want [a.txt c.log]", got)
	}

	// Ctrl+M restores the pre-mask state: only a.txt marked.
	fsp.RestoreSelection()
	got = collectSelected(fsp)
	if len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("after restore: selected = %v, want [a.txt]", got)
	}
}

func TestHotkeyManager_RestoreSelectionDefault_Issue289(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()
	if got := hm.GetAction("Shell", "CtrlM"); got != "Panel.RestoreSelection" {
		t.Errorf("Shell/CtrlM: got %q, want %q", got, "Panel.RestoreSelection")
	}
}
