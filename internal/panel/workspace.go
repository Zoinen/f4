package panel

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
)

type PanelSessionState struct {
	Path          string
	Cursor        string
	ViewMode      int
	Gallery       PanelGallerySessionState
	SortMode      int
	SortReverse   bool
	UseSortGroups bool
}

type WorkspaceSessionState struct {
	Number                          int
	Left, Right                     PanelSessionState
	ActivePanel, WidePanel          int
	ShowPanels, ShowLeft, ShowRight bool
}

var (
	LastWorkspaceSessions []WorkspaceSessionState
	LastActiveWorkspace   int
)

// persistNativePanelLayoutSession is deliberately a hook: semantic unit tests
// construct detached PanelsFrames which must never overwrite the user's real
// session.ini. Production saves only frames that belong to the live manager.
var persistNativePanelLayoutSession = func(pf *PanelsFrame) {
	if pf == nil || vtui.FrameManager == nil {
		return
	}
	for _, screen := range vtui.FrameManager.Screens {
		if panelsFrameOnScreen(screen) == pf {
			SaveSession()
			return
		}
	}
}

func LegacyWorkspaceSession() WorkspaceSessionState {
	return WorkspaceSessionState{
		Number: 1,
		Left: PanelSessionState{
			Path: LastLeftPath, Cursor: LastLeftCursor, ViewMode: LastLeftViewMode,
			Gallery:  ClonePanelGallerySessionState(LastLeftGalleryState),
			SortMode: LastLeftSortMode, SortReverse: LastLeftSortRev,
			UseSortGroups: LastLeftSortGroups,
		},
		Right: PanelSessionState{
			Path: LastRightPath, Cursor: LastRightCursor, ViewMode: LastRightViewMode,
			Gallery:  ClonePanelGallerySessionState(LastRightGalleryState),
			SortMode: LastRightSortMode, SortReverse: LastRightSortRev,
			UseSortGroups: LastRightSortGroups,
		},
		ActivePanel: LastActivePanel,
		WidePanel:   LastWidePanel,
		ShowPanels:  LastShowPanels,
		ShowLeft:    LastShowLeft,
		ShowRight:   LastShowRight,
	}
}

func SetLegacyWorkspaceSession(state WorkspaceSessionState) {
	LastLeftPath, LastRightPath = state.Left.Path, state.Right.Path
	LastLeftCursor, LastRightCursor = state.Left.Cursor, state.Right.Cursor
	LastLeftViewMode, LastRightViewMode = state.Left.ViewMode, state.Right.ViewMode
	LastLeftGalleryState, LastRightGalleryState = ClonePanelGallerySessionState(state.Left.Gallery), ClonePanelGallerySessionState(state.Right.Gallery)
	LastLeftSortMode, LastRightSortMode = state.Left.SortMode, state.Right.SortMode
	LastLeftSortRev, LastRightSortRev = state.Left.SortReverse, state.Right.SortReverse
	LastLeftSortGroups, LastRightSortGroups = state.Left.UseSortGroups, state.Right.UseSortGroups
	LastActivePanel, LastWidePanel = state.ActivePanel, state.WidePanel
	LastShowPanels, LastShowLeft, LastShowRight = state.ShowPanels, state.ShowLeft, state.ShowRight
}

func panelsFrameOnScreen(screen *vtui.AppScreen) *PanelsFrame {
	if screen == nil {
		return nil
	}
	for _, frame := range screen.Frames {
		if pf, ok := frame.(*PanelsFrame); ok && !pf.Closed {
			return pf
		}
	}
	return nil
}

func CaptureWorkspaceSession(pf *PanelsFrame) WorkspaceSessionState {
	activePanel := pf.ActiveIdx
	for idx, p := range pf.Panels {
		if IsAIPanel(p) {
			activePanel = 1 - idx
			break
		}
	}

	state := WorkspaceSessionState{
		ActivePanel: activePanel,
		WidePanel:   -1,
		ShowPanels:  pf.ShowPanels,
		ShowLeft:    pf.ShowLeftPanel,
		ShowRight:   pf.ShowRightPanel,
	}
	if pf.Wide {
		if pf.WidePanel >= 0 && pf.WidePanel < 2 && !IsAIPanel(pf.Panels[pf.WidePanel]) {
			state.WidePanel = pf.WidePanel
		}
	}
	if left, ok := pf.Panels[0].(*FileSystemPanel); ok {
		path := left.PersistentPath()
		cursor := left.GetSelectedName()
		if IsAIPanel(left) {
			path = AIPrevPath[0]
			if path == "" {
				path = "."
			}
			cursor = ""
		}
		state.Left = PanelSessionState{
			Path: path, Cursor: cursor, ViewMode: int(left.ViewMode),
			Gallery:  CapturePanelGallerySessionState(left),
			SortMode: int(left.SortMode), SortReverse: left.SortReverse,
			UseSortGroups: left.UseSortGroups,
		}
	}
	if right, ok := pf.Panels[1].(*FileSystemPanel); ok {
		path := right.PersistentPath()
		cursor := right.GetSelectedName()
		if IsAIPanel(right) {
			path = AIPrevPath[1]
			if path == "" {
				path = "."
			}
			cursor = ""
		}
		state.Right = PanelSessionState{
			Path: path, Cursor: cursor, ViewMode: int(right.ViewMode),
			Gallery:  CapturePanelGallerySessionState(right),
			SortMode: int(right.SortMode), SortReverse: right.SortReverse,
			UseSortGroups: right.UseSortGroups,
		}
	}
	return state
}

func CaptureWorkspaceSessions() ([]WorkspaceSessionState, int) {
	if vtui.FrameManager == nil {
		return nil, 0
	}
	states := make([]WorkspaceSessionState, 0, len(vtui.FrameManager.Screens))
	active := 0
	lastNonAIActive := 0

	for screenIdx, screen := range vtui.FrameManager.Screens {
		pf := panelsFrameOnScreen(screen)
		if pf == nil {
			continue
		}
		hasAI := IsAIPanel(pf.Panels[0]) || IsAIPanel(pf.Panels[1])
		if !hasAI {
			lastNonAIActive = len(states)
		}
		if screenIdx == vtui.FrameManager.ActiveIdx {
			active = len(states)
		}
		state := CaptureWorkspaceSession(pf)
		state.Number = screen.Number
		states = append(states, state)
	}
	if active >= len(states) {
		active = 0
	}
	if active < len(vtui.FrameManager.Screens) {
		if pf := panelsFrameOnScreen(vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx]); pf != nil {
			if IsAIPanel(pf.Panels[0]) || IsAIPanel(pf.Panels[1]) {
				active = lastNonAIActive
			}
		}
	}
	return states, active
}

func parseSessionInt(ini *ini.File, section, key string, fallback int) int {
	value := fallback
	fmt.Sscanf(ini.GetString(section, key, fmt.Sprintf("%d", fallback)), "%d", &value)
	return value
}

func LoadWorkspaceSessions(ini *ini.File) ([]WorkspaceSessionState, int) {
	count := parseSessionInt(ini, "Workspaces", "Count", 0)
	if count <= 0 || count > 100 {
		return nil, 0
	}
	states := make([]WorkspaceSessionState, 0, count)
	for i := 0; i < count; i++ {
		section := fmt.Sprintf("Workspace/%d", i)
		leftSection := section + "/Left"
		rightSection := section + "/Right"
		leftViewMode := parseSessionInt(ini, leftSection, "ViewMode", int(ViewModeMedium))
		rightViewMode := parseSessionInt(ini, rightSection, "ViewMode", int(ViewModeMedium))
		state := WorkspaceSessionState{
			Number:      parseSessionInt(ini, section, "Number", i+1),
			ActivePanel: parseSessionInt(ini, section, "ActivePanel", 1),
			WidePanel:   parseSessionInt(ini, section, "WidePanel", -1),
			ShowPanels:  ini.GetString(section, "ShowPanels", "1") == "1",
			ShowLeft:    ini.GetString(section, "ShowLeft", "1") == "1",
			ShowRight:   ini.GetString(section, "ShowRight", "1") == "1",
			Left: PanelSessionState{
				Path: ini.GetString(leftSection, "Folder", ""), Cursor: ini.GetString(leftSection, "CurFile", ""),
				Gallery:       LoadUnifiedPanelGallerySessionState(ini, leftSection, ValidSessionViewMode(leftViewMode)),
				ViewMode:      leftViewMode,
				SortMode:      parseSessionInt(ini, leftSection, "SortMode", int(SortName)),
				SortReverse:   ini.GetString(leftSection, "SortReverse", "0") == "1",
				UseSortGroups: ini.GetString(leftSection, "UseSortGroups", "0") == "1",
			},
			Right: PanelSessionState{
				Path: ini.GetString(rightSection, "Folder", ""), Cursor: ini.GetString(rightSection, "CurFile", ""),
				Gallery:       LoadUnifiedPanelGallerySessionState(ini, rightSection, ValidSessionViewMode(rightViewMode)),
				ViewMode:      rightViewMode,
				SortMode:      parseSessionInt(ini, rightSection, "SortMode", int(SortName)),
				SortReverse:   ini.GetString(rightSection, "SortReverse", "0") == "1",
				UseSortGroups: ini.GetString(rightSection, "UseSortGroups", "0") == "1",
			},
		}
		if state.ActivePanel < 0 || state.ActivePanel > 1 {
			state.ActivePanel = 1
		}
		if state.WidePanel < -1 || state.WidePanel > 1 {
			state.WidePanel = -1
		}
		if state.Number < 1 {
			state.Number = i + 1
		}
		states = append(states, state)
	}
	active := parseSessionInt(ini, "Workspaces", "Active", 0)
	if active < 0 || active >= len(states) {
		active = 0
	}
	return states, active
}

func WorkspaceSessionsForRestore(states []WorkspaceSessionState, active int, restoreTabs bool) ([]WorkspaceSessionState, int) {
	if restoreTabs || len(states) == 0 {
		return states, active
	}
	if active < 0 || active >= len(states) {
		active = 0
	}
	for i, state := range states {
		if i == active {
			return []WorkspaceSessionState{state}, 0
		}
	}
	return nil, 0
}

func RenumberWorkspaceScreens() {
	if vtui.FrameManager == nil {
		return
	}
	for i, screen := range vtui.FrameManager.Screens {
		if screen != nil {
			screen.Number = i + 1
		}
	}
}

func writePanelSession(sb *strings.Builder, section string, state PanelSessionState) {
	fmt.Fprintf(sb, "\n[%s]\n", section)
	fmt.Fprintf(sb, "Folder = %s\n", state.Path)
	fmt.Fprintf(sb, "CurFile = %s\n", state.Cursor)
	fmt.Fprintf(sb, "ViewMode = %d\n", state.ViewMode)
	WritePanelGallerySessionState(sb, state.Gallery)
	fmt.Fprintf(sb, "SortMode = %d\n", state.SortMode)
	fmt.Fprintf(sb, "SortReverse = %d\n", map[bool]int{true: 1}[state.SortReverse])
	fmt.Fprintf(sb, "UseSortGroups = %d\n", map[bool]int{true: 1}[state.UseSortGroups])
}

func WriteWorkspaceSessions(sb *strings.Builder, states []WorkspaceSessionState, active int) {
	if len(states) == 0 {
		return
	}
	fmt.Fprintf(sb, "\n[Workspaces]\nCount = %d\nActive = %d\n", len(states), active)
	for i, state := range states {
		section := fmt.Sprintf("Workspace/%d", i)
		fmt.Fprintf(sb, "\n[%s]\n", section)
		fmt.Fprintf(sb, "Number = %d\nActivePanel = %d\nWidePanel = %d\n", state.Number, state.ActivePanel, state.WidePanel)
		fmt.Fprintf(sb, "ShowPanels = %d\nShowLeft = %d\nShowRight = %d\n",
			map[bool]int{true: 1}[state.ShowPanels], map[bool]int{true: 1}[state.ShowLeft], map[bool]int{true: 1}[state.ShowRight])
		writePanelSession(sb, section+"/Left", state.Left)
		writePanelSession(sb, section+"/Right", state.Right)
	}
}

func ValidSessionViewMode(mode int) ViewMode {
	viewMode := ViewMode(mode)
	if viewMode != ViewModeMedium && viewMode != ViewModeDetailed && viewMode != ViewModeBrief {
		return ViewModeMedium
	}
	return viewMode
}

// navigatePanelTo moves a panel to a saved path. A failed restore is left
// alone: retrying the same path through the current VFS can turn a provider's
// internal path into a local OS path during startup.
func navigatePanelTo(pf *PanelsFrame, panel *FileSystemPanel, path string) {
	if path != "" {
		pf.NavigateToPath(panel, path)
	}
}

// ApplyStartupDirs opens left and right in the two panels, so `cd dir && f4`
// shows dir and `f4 dir1 dir2` shows both, rather than session.ini's paths. It
// runs after ApplyWorkspaceSession and therefore wins; an empty left changes
// nothing, and an empty right sends both panels to left.
//
// A right that differs from left only ever comes from the command line, so it
// also says the focus belongs on the directory named first.
func ApplyStartupDirs(pf *PanelsFrame, left, right string) {
	if pf == nil || left == "" {
		return
	}
	fromCommandLine := right != "" && right != left
	if right == "" {
		right = left
	}
	for idx, dir := range [2]string{left, right} {
		if fsp, ok := pf.Panels[idx].(*FileSystemPanel); ok && fsp != nil {
			navigatePanelTo(pf, fsp, dir)
			// The pending cursor names a file of the directory just left.
			fsp.PendingSelection = ""
		}
	}
	if fromCommandLine {
		pf.ActiveIdx = 0
	}
}

func ApplyWorkspaceSession(pf *PanelsFrame, state WorkspaceSessionState, width, height int, restorePaths bool) {
	if pf == nil {
		return
	}
	left, leftOK := pf.Panels[0].(*FileSystemPanel)
	right, rightOK := pf.Panels[1].(*FileSystemPanel)
	if !leftOK || left == nil || !rightOK || right == nil {
		// A freshly constructed background workspace has not been laid out yet,
		// so ResizeConsole must create its file panels before session state can
		// be applied. The first workspace is already resized by SetupUI, while
		// restored background workspaces reach this function directly.
		pf.ResizeConsole(width, height)
		left, leftOK = pf.Panels[0].(*FileSystemPanel)
		right, rightOK = pf.Panels[1].(*FileSystemPanel)
		if !leftOK || left == nil || !rightOK || right == nil {
			return
		}
	}
	left.SetViewMode(ValidSessionViewMode(state.Left.ViewMode))
	right.SetViewMode(ValidSessionViewMode(state.Right.ViewMode))
	RestorePanelGallerySessionState(left, state.Left.Gallery)
	RestorePanelGallerySessionState(right, state.Right.Gallery)
	left.SortMode, right.SortMode = SortMode(state.Left.SortMode), SortMode(state.Right.SortMode)
	left.SortReverse, right.SortReverse = state.Left.SortReverse, state.Right.SortReverse
	left.UseSortGroups, right.UseSortGroups = state.Left.UseSortGroups, state.Right.UseSortGroups

	if restorePaths {
		navigatePanelTo(pf, left, state.Left.Path)
		navigatePanelTo(pf, right, state.Right.Path)
		left.PendingSelection, right.PendingSelection = state.Left.Cursor, state.Right.Cursor
	}

	pf.ActiveIdx = state.ActivePanel
	if pf.ActiveIdx < 0 || pf.ActiveIdx > 1 {
		pf.ActiveIdx = 1
	}
	pf.ShowPanels, pf.ShowLeftPanel, pf.ShowRightPanel = state.ShowPanels, state.ShowLeft, state.ShowRight
	pf.Wide, pf.WidePanel = false, -1
	if state.WidePanel == 0 || state.WidePanel == 1 {
		pf.Wide, pf.WidePanel, pf.ActiveIdx, pf.ShowPanels = true, state.WidePanel, state.WidePanel, true
	}
	pf.ResizeConsole(width, height)
}
