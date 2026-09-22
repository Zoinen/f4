package panel

import (
	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
	"strings"
)

// GUIViewModesAvailable checks the running renderer, not a saved startup preference.
func GUIViewModesAvailable() bool {
	if vtui.FrameManager == nil || vtui.FrameManager.Screen() == nil || config.App.GuiPresentation == config.GuiPresentationText {
		return false
	}
	renderer, ok := vtui.FrameManager.Screen().Renderer.(interface{ NativeSemanticSurfaceActive() bool })
	return ok && renderer.NativeSemanticSurfaceActive()
}

func (pf *PanelsFrame) withGalleryMenuItems(side int, menu vtui.MenuBarItem) vtui.MenuBarItem {
	items := make([]vtui.MenuItem, 0, len(menu.SubItems)+2)
	for _, item := range menu.SubItems {
		if item.Command == appcmd.CmLeftGallery || item.Command == appcmd.CmRightGallery {
			if !GUIViewModesAvailable() {
				continue
			}
			icons, grid := appcmd.CmLeftIcons, appcmd.CmLeftGrid
			if side == 1 {
				icons, grid = appcmd.CmRightIcons, appcmd.CmRightGrid
			}
			items = append(items,
				vtui.MenuItem{Text: i18n.Msg("Panel.ViewIcons"), Command: icons},
				vtui.MenuItem{Text: i18n.Msg("Panel.ViewGrid"), Command: grid})
		}
		items = append(items, item)
	}
	menu.SubItems = items
	for index := range menu.SubItems {
		item := &menu.SubItems[index]
		switch item.Command {
		case appcmd.CmLeftMedium, appcmd.CmRightMedium:
			item.Icon = "columns-2"
		case appcmd.CmLeftBrief, appcmd.CmRightBrief:
			item.Icon = "columns-3"
		case appcmd.CmLeftDetailed, appcmd.CmRightDetailed:
			item.Icon = "list"
		case appcmd.CmLeftWide, appcmd.CmRightWide:
			item.Icon = "panel-left"
		case appcmd.CmLeftIcons, appcmd.CmRightIcons:
			item.Icon = "images"
		case appcmd.CmLeftGrid, appcmd.CmRightGrid:
			item.Icon = "grid-3x3"
		case appcmd.CmLeftGallery, appcmd.CmRightGallery:
			item.Icon = "layout-dashboard"
		case appcmd.CmLeftSortName, appcmd.CmRightSortName:
			item.Icon = "arrow-down-a-z"
		case appcmd.CmLeftSortExt, appcmd.CmRightSortExt:
			item.Icon = "file-type"
		case appcmd.CmLeftSortTime, appcmd.CmRightSortTime:
			item.Icon = "clock-3"
		case appcmd.CmLeftSortSize, appcmd.CmRightSortSize:
			item.Icon = "arrow-down-wide-narrow"
		case appcmd.CmLeftSortUnsorted, appcmd.CmRightSortUnsorted,
			appcmd.CmLeftSortGroups, appcmd.CmRightSortGroups:
			item.Icon = "list"
		}
	}
	return menu
}

func (pf *PanelsFrame) updateSideMenuCheckmarks(side int, items []vtui.MenuItem) {
	fsp, ok := pf.Panels[side].(*FileSystemPanel)
	if !ok || IsAIPanel(fsp) {
		return
	}
	mode, columns := fsp.effectiveGalleryLayoutMode(), fsp.effectiveGalleryColumnCount()
	if !GUIViewModesAvailable() {
		switch fsp.ViewMode {
		case ViewModeMedium:
			mode, columns = GalleryLayoutColumns, 2
		case ViewModeBrief:
			mode, columns = GalleryLayoutColumns, 3
		case ViewModeDetailed:
			mode = GalleryLayoutDetails
		}
	}
	for index := range items {
		item := &items[index]
		checked := false
		switch item.Command {
		case appcmd.CmLeftMedium, appcmd.CmRightMedium:
			checked = mode == GalleryLayoutColumns && columns == 2
		case appcmd.CmLeftBrief, appcmd.CmRightBrief:
			checked = mode == GalleryLayoutColumns && columns == 3
		case appcmd.CmLeftDetailed, appcmd.CmRightDetailed:
			checked = mode == GalleryLayoutDetails
		case appcmd.CmLeftWide, appcmd.CmRightWide:
			checked = pf.Wide && pf.WidePanel == side
		case appcmd.CmLeftIcons, appcmd.CmRightIcons:
			checked = mode == GalleryLayoutIcons
		case appcmd.CmLeftGrid, appcmd.CmRightGrid:
			checked = mode == GalleryLayoutGrid
		case appcmd.CmLeftGallery, appcmd.CmRightGallery:
			checked = mode == GalleryLayoutMasonry
		case appcmd.CmLeftSortName, appcmd.CmRightSortName:
			checked = fsp.SortMode == SortName
		case appcmd.CmLeftSortExt, appcmd.CmRightSortExt:
			checked = fsp.SortMode == SortExt
		case appcmd.CmLeftSortTime, appcmd.CmRightSortTime:
			checked = fsp.SortMode == SortTime
		case appcmd.CmLeftSortSize, appcmd.CmRightSortSize:
			checked = fsp.SortMode == SortSize
		case appcmd.CmLeftSortUnsorted, appcmd.CmRightSortUnsorted:
			checked = fsp.SortMode == SortUnsorted
		case appcmd.CmLeftSortGroups, appcmd.CmRightSortGroups:
			checked = fsp.UseSortGroups
		default:
			continue
		}
		item.Text = menuCheckText(checked, strings.TrimLeft(item.Text, "√ "))
	}
}
