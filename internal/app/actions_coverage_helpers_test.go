package app

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestActionsCoverageHelpers(t *testing.T) {
	t.Run("choiceText", func(t *testing.T) {
		choices := []string{"first", "second"}
		for _, tc := range []struct {
			name     string
			selected int
			want     string
		}{
			{name: "first", selected: 0, want: "first"},
			{name: "second", selected: 1, want: "second"},
			{name: "before", selected: -1},
			{name: "after", selected: len(choices)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := choiceText(choices, tc.selected); got != tc.want {
					t.Fatalf("choiceText(%v, %d) = %q, want %q", choices, tc.selected, got, tc.want)
				}
			})
		}
	})

	t.Run("checkbox widths", func(t *testing.T) {
		short := vtui.NewCheckbox(0, 0, "a", false)
		short.SetPosition(5, 1, 8, 1)
		wide := vtui.NewCheckbox(0, 0, "long label", false)
		wide.SetPosition(2, 2, 12, 2)
		if got := elementWidth(short); got != 4 {
			t.Fatalf("elementWidth(short) = %d, want 4", got)
		}
		if got := checkboxColumnWidth(short, wide); got != 11 {
			t.Fatalf("checkboxColumnWidth = %d, want 11", got)
		}
		if got := checkboxColumnWidth(); got != 0 {
			t.Fatalf("empty checkboxColumnWidth = %d, want 0", got)
		}
	})

	t.Run("boolean states", func(t *testing.T) {
		if got := boolToCheckboxState(false); got != 0 {
			t.Fatalf("boolToCheckboxState(false) = %d, want 0", got)
		}
		if got := boolToCheckboxState(true); got != 1 {
			t.Fatalf("boolToCheckboxState(true) = %d, want 1", got)
		}
	})
}

type actionsDuplicateFinderVFS struct{ vfs.VFS }

func (actionsDuplicateFinderVFS) FindDuplicates(context.Context, string, func(vfs.DuplicateProgress)) ([][]string, error) {
	return nil, nil
}

func TestPanelCanFindDuplicates(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	if panelCanFindDuplicates() {
		t.Fatal("panelCanFindDuplicates without a screen = true, want false")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	withoutFinder := &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir())}
	pf := &panel.PanelsFrame{
		Panels:     [2]panel.Panel{withoutFinder, nil},
		ActiveIdx:  0,
		ShowPanels: true,
	}
	vtui.FrameManager.Screens = []*vtui.AppScreen{{Number: 1, Frames: []vtui.Frame{pf}}}
	vtui.FrameManager.ActiveIdx = 0
	if panelCanFindDuplicates() {
		t.Fatal("ordinary VFS unexpectedly advertises duplicate search")
	}

	withFinder := &panel.FileSystemPanel{Vfs: actionsDuplicateFinderVFS{VFS: vfs.NewOSVFS(t.TempDir())}}
	pf.Panels[0] = withFinder
	if !panelCanFindDuplicates() {
		t.Fatal("duplicate-finder VFS does not advertise duplicate search")
	}
}
