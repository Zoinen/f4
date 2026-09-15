package panel

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/unxed/f4/internal/filemenu"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// ContextMenuCommand is wired by the composition root to existing operations.
// The caller checks the captured panel/selection before dispatching it.
var ContextMenuCommand = func(*PanelsFrame, string) {}
var FileMenuRunner = filemenu.Run

type fileMenuSelection struct {
	owner        *PanelsFrame
	panel        *FileSystemPanel
	filesystem   vfs.VFS
	base         string
	names, paths []string
	index        int
	workspace    *vtui.AppScreen
}

func captureFileMenuSelection(pf *PanelsFrame) *fileMenuSelection {
	if pf == nil || pf.Closed || !pf.ShowPanels {
		return nil
	}
	fp := pf.GetActivePanel()
	if fp == nil || fp.Vfs == nil {
		return nil
	}
	s := &fileMenuSelection{owner: pf, panel: fp, filesystem: fp.Vfs, base: fp.Vfs.GetPath(), index: pf.ActiveIdx}
	if vtui.FrameManager != nil && vtui.FrameManager.ActiveIdx >= 0 && vtui.FrameManager.ActiveIdx < len(vtui.FrameManager.Screens) {
		s.workspace = vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx]
	}
	for _, name := range fp.GetSelectedNames() {
		if name != "" && name != ".." {
			s.names = append(s.names, name)
			s.paths = append(s.paths, fp.Vfs.Join(s.base, name))
		}
	}
	if len(s.paths) == 0 {
		return nil
	}
	return s
}

func (s *fileMenuSelection) valid() bool {
	if s.owner.Closed || s.owner.Panels[s.index] != s.panel || !fileops.SameVFSInstance(s.panel.Vfs, s.filesystem) || s.filesystem.GetPath() != s.base {
		return false
	}
	var current []string
	for _, name := range s.panel.GetSelectedNames() {
		if name != "" && name != ".." {
			current = append(current, name)
		}
	}
	return slices.Equal(current, s.names)
}

func (s *fileMenuSelection) activate() bool {
	if s.workspace == nil {
		return true
	}
	for i, workspace := range vtui.FrameManager.Screens {
		if workspace == s.workspace {
			if vtui.FrameManager.ActiveIdx != i {
				vtui.FrameManager.SwitchScreen(i)
			}
			return true
		}
	}
	return false
}

func (s *fileMenuSelection) localPaths() ([]string, bool) {
	switch s.filesystem.(type) {
	case *vfs.OSVFS, *vfs.DisksVFS:
		var paths []string
		for _, path := range s.paths {
			abs, err := s.filesystem.Abs(path)
			if err != nil || !filepath.IsAbs(abs) {
				return nil, false
			}
			paths = append(paths, abs)
		}
		return paths, false
	}
	paths, readOnly, ok := fusefs.LocalPaths(s.filesystem, s.paths)
	if !ok {
		return nil, false
	}
	return paths, readOnly
}

func fileMenuLabel(id, fallback string) string {
	value := i18n.Msg("FileContext." + id)
	if strings.HasPrefix(value, "{") {
		return fallback
	}
	return value
}

func (s *fileMenuSelection) entries(native bool, local bool, readOnly bool) []filemenu.Entry {
	write := s.filesystem.GetCapabilities().HasWrite && !readOnly
	single := len(s.paths) == 1
	singleFile := single
	if single {
		for _, entry := range s.panel.Entries {
			if entry.Name == s.names[0] {
				singleFile = !entry.IsDir
				break
			}
		}
	}
	definitions := [][3]string{
		{"open", "Open", "local"}, {"open-with", "Open With", "local"},
		{"view", "View", "single-file"}, {"edit", "Edit", "file-write"},
		{"copy", "Copy…", ""}, {"move", "Move…", "write"}, {"rename", "Rename…", "single-write"},
		{"duplicate", "Duplicate…", "single-write"}, {"compress", "Compress…", "compress"},
		{"copy-path", "Copy Path", ""}, {"trash", "Move to Trash", "trash"}, {"properties", "Properties…", ""},
	}
	if native && filemenu.Platform == "darwin" {
		definitions = [][3]string{{"open", "Open", "local"}, {"open-with", "Open With", "local"}, {"quick-look", "Quick Look", "local"},
			{"properties", "Get Info", ""}, {"rename", "Rename…", "single-write"}, {"duplicate", "Duplicate…", "single-write"},
			{"compress", "Compress…", "compress"}, {"copy-files", "Copy", "local"}, {"copy-path", "Copy Pathname", ""}, {"trash", "Move to Trash", "trash"}}
	}
	canCompress := false
	for _, command := range plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, s.owner) {
		if command.ID == "archive.add" {
			canCompress = true
		}
	}
	_, trash := s.filesystem.(vfs.TrashVFS)
	var entries []filemenu.Entry
	for _, def := range definitions {
		enabled := true
		switch def[2] {
		case "local":
			enabled = local
		case "single":
			enabled = single
		case "single-file":
			enabled = singleFile
		case "file-write":
			enabled = singleFile && write
		case "single-write":
			enabled = single && write
		case "write":
			enabled = write
		case "trash":
			enabled = trash && write
		case "compress":
			enabled = canCompress && write
		}
		label := fileMenuLabel(def[0], def[1])
		if native && filemenu.Platform == "darwin" {
			label = fileMenuLabel("mac-"+def[0], def[1])
		}
		entries = append(entries, filemenu.Entry{ID: def[0], Label: label, Disabled: !enabled})
	}
	return entries
}

// fileMenuFrame owns cancellation even when its workspace is closed.
type fileMenuFrame struct {
	*vtui.VMenu
	cancel  context.CancelFunc
	pending bool
}

func (m *fileMenuFrame) Show(scr *vtui.ScreenBuf) {
	if !m.pending {
		m.VMenu.Show(scr)
	}
}

func (m *fileMenuFrame) HasShadow() bool { return !m.pending && m.VMenu.HasShadow() }

func (m *fileMenuFrame) Close()               { m.cancel(); m.VMenu.Close() }
func (m *fileMenuFrame) SetExitCode(code int) { m.cancel(); m.VMenu.SetExitCode(code) }
func (m *fileMenuFrame) ProcessKey(e *vtinput.InputEvent) bool {
	if m.pending {
		if e.KeyDown && e.VirtualKeyCode == vtinput.VK_ESCAPE {
			m.Close()
		}
		return true
	}
	handled := m.VMenu.ProcessKey(e)
	if m.IsDone() {
		m.cancel()
	}
	return handled
}
func (m *fileMenuFrame) ProcessMouse(e *vtinput.InputEvent) bool {
	if m.pending {
		return true
	}
	handled := m.VMenu.ProcessMouse(e)
	if m.IsDone() {
		m.cancel()
	}
	return handled
}

var activeFileMenu *fileMenuFrame // UI-thread owned
var activeFileMenuOwner *PanelsFrame

func ShowFileContextMenu(pf *PanelsFrame) {
	if pf == nil || pf.GetActivePanel() == nil {
		return
	}
	x, y := fileMenuAnchor(pf.GetActivePanel())
	position := filemenu.Point{}
	if vtui.FrameManager != nil {
		position = filemenu.PanelPoint(x, y, vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	}
	ShowFileContextMenuAt(pf, x, y, position)
}

// ShowFileContextMenuAt accepts an explicit anchor for future mouse dispatch.
// Cell coordinates place the fallback; desktop coordinates place native menus.
func ShowFileContextMenuAt(pf *PanelsFrame, x, y int, position filemenu.Point) {
	if activeFileMenu != nil && !activeFileMenu.IsDone() {
		return
	}
	s := captureFileMenuSelection(pf)
	if s == nil || vtui.FrameManager == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	if pf.fileMenuCancel != nil {
		pf.fileMenuCancel()
	}
	pf.fileMenuCancel = cancel
	menu := &fileMenuFrame{VMenu: vtui.NewVMenu(fileMenuLabel("title", "File Context Menu")), cancel: cancel, pending: true}
	activeFileMenu = menu
	activeFileMenuOwner = pf
	menu.SetHelp("FileContextMenu")
	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if e.KeyDown && e.VirtualKeyCode == vtinput.VK_ESCAPE {
			menu.Close()
			return true
		}
		return false
	}
	placeFileMenu(menu.VMenu, x, y)
	vtui.FrameManager.Push(menu)
	nativeEntries := s.entries(true, true, false)
	runner := FileMenuRunner
	fm := vtui.FrameManager
	go func() {
		paths, readOnly := s.localPaths()
		if readOnly {
			for i := range nativeEntries {
				switch nativeEntries[i].ID {
				case "edit", "rename", "move", "duplicate", "compress", "trash":
					nativeEntries[i].Disabled = true
				}
			}
		}
		result := filemenu.Result{Outcome: filemenu.Unavailable}
		if position.Valid && len(paths) > 0 && filemenu.Platform != "linux" && !(readOnly && filemenu.Platform == "windows") {
			result = runner(ctx, filemenu.Request{Paths: paths, Entries: nativeEntries, Position: position})
		}
		var apps []filemenu.Entry
		if result.Outcome == filemenu.Unavailable && len(paths) > 0 {
			discoveryCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			appResult := runner(discoveryCtx, filemenu.Request{Paths: paths, Operation: "applications"})
			apps = appResult.Entries
			stop()
		}
		fm.PostTask(func() {
			if menu.IsDone() || ctx.Err() != nil {
				return
			}
			if !s.valid() {
				menu.Close()
				return
			}
			switch result.Outcome {
			case filemenu.Unavailable:
				menu.pending = false
				entries := s.entries(false, len(paths) > 0, readOnly)
				menu.Items = nil
				menuItems := fallbackMenuItems(entries, apps, func(id string) { menu.Close(); fm.PostTask(func() { s.invoke(id, paths, readOnly) }) })
				for _, item := range menuItems {
					menu.AddItem(item)
				}
				placeFileMenu(menu.VMenu, x, y)
				fm.Redraw()
			case filemenu.Selected:
				menu.Close()
				fm.PostTask(func() { s.invoke(result.Action, paths, readOnly) })
			case filemenu.Failed:
				menu.Close()
				showFileMenuError(result.Error)
			default:
				menu.Close()
				if result.Outcome == filemenu.Invoked {
					s.owner.RefreshAll()
				}
			}
		})
	}()
}

func fallbackMenuItems(entries, apps []filemenu.Entry, invoke func(string)) []vtui.MenuItem {
	var items []vtui.MenuItem
	for _, entry := range entries {
		if entry.Disabled {
			continue
		}
		item := vtui.MenuItem{Text: entry.Label}
		id := entry.ID
		if id == "open-with" {
			if len(apps) == 0 {
				continue
			}
			item.SubItems = fallbackMenuItems(apps, nil, invoke)
		} else {
			item.OnClick = func() { invoke(id) }
		}
		items = append(items, item)
	}
	return items
}

func fileMenuAnchor(fp *FileSystemPanel) (int, int) {
	x, y, _, _ := fp.GetPosition()
	if fp.Table == nil || fp.Table.ViewHeight <= 0 {
		return x + 1, y + 2
	}
	index := max(0, fp.CursorIdx-fp.Table.TopPos)
	column := 0
	if fp.gridColumnCount() > 1 {
		column = index / fp.Table.ViewHeight
		index %= fp.Table.ViewHeight
	}
	x = fp.Table.X1
	for i := 0; i < column && i < len(fp.Table.Columns); i++ {
		x += fp.Table.Columns[i].Width + 1
	}
	y = fp.Table.Y1 + fp.Table.MarginTop + min(index, fp.Table.ViewHeight-1) + 1
	return x, y
}

func placeFileMenu(menu *vtui.VMenu, x, y int) {
	w, h := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	width := min(42, max(1, w))
	height := min(len(menu.Items)+2, max(1, h))
	x = min(max(0, x), max(0, w-width))
	y = min(max(0, y), max(0, h-height))
	menu.SetPosition(x, y, x+width-1, y+height-1)
}

func showFileMenuError(message string) {
	vtui.ShowMessage(fileMenuLabel("title", "File Context Menu"), message, []string{i18n.Msg("vtui.Ok")})
}

func (s *fileMenuSelection) invoke(id string, oldPaths []string, readOnly bool) {
	if !s.valid() || !s.activate() {
		return
	}
	// Never let a workspace/panel switch retarget an operation. The captured
	// frame is passed explicitly; each existing operation snapshots its inputs.
	s.owner.ActiveIdx = s.index
	paths := oldPaths
	allowed := false
	for _, entry := range s.entries(true, len(paths) > 0, readOnly) {
		if entry.ID == id && !entry.Disabled {
			allowed = true
		}
	}
	for _, entry := range s.entries(false, len(paths) > 0, readOnly) {
		if entry.ID == id && !entry.Disabled {
			allowed = true
		}
	}
	if strings.HasPrefix(id, "app:") {
		allowed = len(paths) > 0
	}
	if !allowed {
		return
	}
	if id == "copy-path" {
		terminal.SetClipboardAsync(strings.Join(s.paths, "\n"))
		return
	}
	if id == "open" || id == "quick-look" || id == "copy-files" || strings.HasPrefix(id, "app:") {
		if len(paths) == 0 {
			showFileMenuError(fileMenuLabel("stale", "The selected files or mount are no longer available."))
			return
		}
		operation := id
		if strings.HasPrefix(id, "app:") {
			operation = "open-with"
		}
		runner := FileMenuRunner
		ctx, cancel := context.WithCancel(context.Background())
		if s.owner.fileMenuCancel != nil {
			s.owner.fileMenuCancel()
		}
		s.owner.fileMenuCancel = cancel
		fm := vtui.FrameManager
		go func() {
			defer cancel()
			currentPaths, _ := s.localPaths()
			if !slices.Equal(currentPaths, oldPaths) {
				fm.PostTask(func() {
					if !s.owner.Closed {
						showFileMenuError(fileMenuLabel("stale", "The selected files or mount are no longer available."))
					}
				})
				return
			}
			result := runner(ctx, filemenu.Request{Paths: paths, Operation: operation, Application: id})
			fm.PostTask(func() {
				if !s.owner.Closed {
					if result.Outcome == filemenu.Failed || result.Outcome == filemenu.Unavailable {
						showFileMenuError(result.Error)
					}
					s.owner.RefreshAll()
				}
			})
		}()
		return
	}
	if id == "view" {
		OpenViewer(s.owner, s.filesystem, s.paths[0])
		return
	}
	if id == "edit" {
		OpenEditor(s.owner, s.filesystem, s.paths[0])
		return
	}
	if id == "compress" {
		plughost.ExecutePluginCommand(vfs.PluginCommandPanel, "archive.add", s.owner)
		return
	}
	if id == "rename" || id == "duplicate" {
		s.panel.SelectName(s.names[0])
	}
	ContextMenuCommand(s.owner, id)
}
