package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

type galleryMenuRenderer struct {
	vtui.SurfaceRenderer
	native bool
}

func (r galleryMenuRenderer) NativeSemanticSurfaceActive() bool { return r.native }

func TestPanelSortMenuIconsMatchPathDropdown(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	bar := pf.GetMenuBar()
	for _, tc := range []struct {
		name     string
		commands [2]int
		icon     string
	}{
		{"name", [2]int{appcmd.CmLeftSortName, appcmd.CmRightSortName}, "arrow-down-a-z"},
		{"extension", [2]int{appcmd.CmLeftSortExt, appcmd.CmRightSortExt}, "file-type"},
		{"time", [2]int{appcmd.CmLeftSortTime, appcmd.CmRightSortTime}, "clock-3"},
		{"size", [2]int{appcmd.CmLeftSortSize, appcmd.CmRightSortSize}, "arrow-down-wide-narrow"},
		{"unsorted", [2]int{appcmd.CmLeftSortUnsorted, appcmd.CmRightSortUnsorted}, "list"},
		{"groups", [2]int{appcmd.CmLeftSortGroups, appcmd.CmRightSortGroups}, "list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for side, menuIndex := range []int{0, len(bar.Items) - 1} {
				found := false
				for _, item := range bar.Items[menuIndex].SubItems {
					if item.Command == tc.commands[side] {
						found = true
						if item.Icon != tc.icon {
							t.Errorf("side %d icon = %q, want %q", side, item.Icon, tc.icon)
						}
					}
				}
				if !found {
					t.Errorf("side %d missing sort command %d", side, tc.commands[side])
				}
			}
		})
	}
}

func TestPanelGalleryMenusFollowFrontendAndTargetSide(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	screen := vtui.FrameManager.Screen()
	renderer, presentation := screen.Renderer, config.App.GuiPresentation
	defer func() { screen.Renderer = renderer; config.App.GuiPresentation = presentation }()
	for _, tc := range []struct {
		name         string
		native       bool
		presentation config.GuiPresentationMode
		want         bool
	}{
		{"terminal", false, config.GuiPresentationGUI, false},
		{"qml-text", true, config.GuiPresentationText, false},
		{"qml-gui", true, config.GuiPresentationGUI, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screen.Renderer = galleryMenuRenderer{SurfaceRenderer: renderer, native: tc.native}
			config.App.GuiPresentation = tc.presentation
			for side, commands := range [2][]int{
				{appcmd.CmLeftIcons, appcmd.CmLeftGrid, appcmd.CmLeftGallery},
				{appcmd.CmRightIcons, appcmd.CmRightGrid, appcmd.CmRightGallery},
			} {
				for index, command := range commands {
					bar := pf.GetMenuBar()
					menuIndex := 0
					if side == 1 {
						menuIndex = len(bar.Items) - 1
					}
					found := false
					for _, item := range bar.Items[menuIndex].SubItems {
						found = found || item.Command == command
					}
					if found != tc.want {
						t.Fatalf("side %d command %d present=%v, want %v", side, command, found, tc.want)
					}
					if !tc.want {
						continue
					}
					other := pf.Panels[1-side].(*FileSystemPanel)
					before := other.GalleryLayoutMode
					if !pf.HandleCommand(command, nil) {
						t.Fatal("menu command was not handled")
					}
					want := []GalleryLayoutMode{GalleryLayoutIcons, GalleryLayoutGrid, GalleryLayoutMasonry}[index]
					if pf.Panels[side].(*FileSystemPanel).GalleryLayoutMode != want || other.GalleryLayoutMode != before {
						t.Fatalf("command %d did not select only side %d", command, side)
					}
					for _, item := range pf.GetMenuBar().Items[menuIndex].SubItems {
						if item.Command == command && item.Icon != []string{"images", "grid-3x3", "layout-dashboard"}[index] {
							t.Fatalf("incorrect mode icon: %#v", item)
						}
						if item.Command == command && !strings.HasPrefix(item.Text, "√") {
							t.Fatalf("missing checkmark: %#v", item)
						}
					}
				}
			}
		})
	}
}
