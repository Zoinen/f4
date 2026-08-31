//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/winshell"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type windowsLocationRow struct {
	node      winshell.Node
	depth     int
	parentURI string
	loading   bool
}

type windowsLocationsMenu struct {
	*vtui.VMenu

	pf       *PanelsFrame
	panelIdx int
	parent   *vtui.VMenu
	client   *winshell.Client

	mu       sync.Mutex
	roots    []winshell.Node
	children map[string][]winshell.Node
	expanded map[string]bool
	loading  map[string]bool
	rows     []windowsLocationRow
	tasks    []*vtui.TaskContext

	dragArmed  bool
	dragging   bool
	dragStartX int
	dragStartY int
	dragSource *FileSystemPanel
	dragNames  []string
}

type shellContextRow struct {
	command winshell.ContextCommand
}

type shellContextMenu struct {
	*vtui.VMenu

	owner    *windowsLocationsMenu
	parent   *shellContextMenu
	root     *shellContextMenu
	token    uint64
	rows     []shellContextRow
	invoking bool
}

func addWindowsLocationsDriveItem(pf *PanelsFrame, panelIdx int, menu *vtui.VMenu) bool {
	menu.AddItem(vtui.MenuItem{
		Text:     Msg("WindowsLocations.DriveItem"),
		Icon:     driveMenuIconLocal,
		Shortcut: "▶",
		UserData: driveMenuCascadeAction(func(parent *vtui.VMenu) {
			showWindowsLocationsMenu(pf, panelIdx, parent)
		}),
	})
	return true
}

func showWindowsLocationsMenu(pf *PanelsFrame, panelIdx int, parent *vtui.VMenu) {
	client, err := winshell.DefaultClient()
	if err != nil {
		vtui.ShowMessage(" Windows locations ", err.Error(), []string{"&Ok"})
		return
	}
	menu := &windowsLocationsMenu{
		VMenu:    vtui.NewVMenu(" " + Msg("WindowsLocations.Title") + " "),
		pf:       pf,
		panelIdx: panelIdx,
		parent:   parent,
		client:   client,
		children: make(map[string][]winshell.Node),
		expanded: make(map[string]bool),
		loading:  make(map[string]bool),
	}
	configureWindowsShellMenu(menu.VMenu)
	menu.HideShadow = false
	menu.rebuild([]windowsLocationRow{{loading: true}})
	menu.positionBesideParent()
	vtui.FrameManager.Push(menu)

	task := vtui.RunAsync(func(task *vtui.TaskContext) {
		roots, loadErr := client.Roots(task.Context)
		task.RunOnUI(func() {
			if menu.IsDone() {
				return
			}
			if loadErr != nil {
				menu.rebuild([]windowsLocationRow{{node: winshell.Node{Name: loadErr.Error()}}})
				return
			}
			menu.mu.Lock()
			menu.roots = roots
			menu.mu.Unlock()
			menu.rebuildFromModel("")
		})
	})
	menu.tasks = append(menu.tasks, task)
}

func (m *windowsLocationsMenu) reloadRoots() {
	if m == nil || m.IsDone() {
		return
	}
	task := vtui.RunAsync(func(task *vtui.TaskContext) {
		roots, err := m.client.Roots(task.Context)
		task.RunOnUI(func() {
			if m.IsDone() {
				return
			}
			if err != nil {
				vtui.ShowMessage(" Windows locations ", err.Error(), []string{"&Ok"})
				return
			}
			m.mu.Lock()
			m.roots = roots
			m.children = make(map[string][]winshell.Node)
			m.expanded = make(map[string]bool)
			m.loading = make(map[string]bool)
			m.mu.Unlock()
			m.rebuildFromModel("")
		})
	})
	m.tasks = append(m.tasks, task)
}

func (m *windowsLocationsMenu) positionBesideParent() {
	screenW := vtui.FrameManager.GetScreenSize()
	screenH := vtui.FrameManager.GetScreenHeight()
	w, h := m.desiredSize(screenW, screenH)
	x := m.parent.X2 + 1
	if x+w > screenW {
		x = m.parent.X1 - w
	}
	if x < 0 {
		x = 0
	}
	y := m.parent.Y1
	if y+h > screenH {
		y = screenH - h
	}
	if y < 0 {
		y = 0
	}
	m.SetPosition(x, y, x+w-1, y+h-1)
}

func (m *windowsLocationsMenu) desiredSize(screenW, screenH int) (int, int) {
	w := 34
	for _, item := range m.Items {
		clean, _, _ := vtui.ParseAmpersandString(item.Text)
		candidate := runewidth.StringWidth(clean) + runewidth.StringWidth(item.Shortcut) + 6
		if candidate > w {
			w = candidate
		}
	}
	if screenW > 4 && w > screenW-4 {
		w = screenW - 4
	}
	h := len(m.Items) + 2
	if screenH > 2 && h > screenH-2 {
		h = screenH - 2
	}
	if h < 3 {
		h = 3
	}
	return w, h
}

func (m *windowsLocationsMenu) rebuildFromModel(preferredURI string) {
	m.mu.Lock()
	rows := make([]windowsLocationRow, 0, len(m.roots)+8)
	var appendNodes func([]winshell.Node, int, string)
	appendNodes = func(nodes []winshell.Node, depth int, parentURI string) {
		for _, node := range nodes {
			rows = append(rows, windowsLocationRow{node: node, depth: depth, parentURI: parentURI})
			if node.Separator || node.URI == "" || !m.expanded[node.URI] {
				continue
			}
			if m.loading[node.URI] {
				rows = append(rows, windowsLocationRow{depth: depth + 1, parentURI: node.URI, loading: true})
				continue
			}
			appendNodes(m.children[node.URI], depth+1, node.URI)
		}
	}
	appendNodes(m.roots, 0, "")
	m.mu.Unlock()
	m.rebuild(rows)
	if preferredURI != "" {
		for index, row := range m.rows {
			if row.node.URI == preferredURI {
				m.SetSelectPos(index)
				break
			}
		}
	}
	m.positionBesideParent()
	vtui.FrameManager.Redraw()
}

func (m *windowsLocationsMenu) rebuild(rows []windowsLocationRow) {
	m.rows = rows
	m.Items = m.Items[:0]
	for _, row := range rows {
		if row.node.Separator {
			m.Items = append(m.Items, vtui.MenuItem{Separator: true})
			continue
		}
		if row.loading {
			m.Items = append(m.Items, vtui.MenuItem{Text: strings.Repeat("  ", row.depth) + "◌ Loading…"})
			continue
		}
		prefix := strings.Repeat("  ", row.depth)
		if row.node.Folder && row.node.HasChildren {
			if m.expanded[row.node.URI] {
				prefix += "▾ "
			} else {
				prefix += "▸ "
			}
		} else {
			prefix += "  "
		}
		fallbackIcon := windowsLocationFallbackIcon(row.node)
		shortcut := ""
		if row.node.Pinned {
			shortcut = "⌖"
		}
		m.Items = append(m.Items, vtui.MenuItem{
			Text:     prefix + fallbackIcon + " " + escapeAmpersand(row.node.Name),
			Shortcut: shortcut,
		})
	}
	m.ItemCount = len(m.Items)
	if m.ItemCount == 0 {
		m.Items = append(m.Items, vtui.MenuItem{Text: "No Windows Shell locations"})
		m.rows = append(m.rows, windowsLocationRow{})
		m.ItemCount = 1
	}
	if m.SelectPos < 0 || m.SelectPos >= m.ItemCount || m.Items[m.SelectPos].Separator {
		m.SetSelectPos(0)
	}
}

func windowsLocationFallbackIcon(node winshell.Node) string {
	lower := strings.ToLower(node.Name + " " + node.ParsingName)
	switch {
	case strings.Contains(lower, "f874310e") || strings.Contains(lower, "home") || strings.Contains(lower, "главная"):
		return "⌂"
	case strings.Contains(lower, "gallery") || strings.Contains(lower, "галере"):
		return "▣"
	case strings.Contains(lower, "f02c1a0d") || strings.Contains(lower, "network") || strings.Contains(lower, "сеть"):
		return "◎"
	case strings.Contains(lower, "b2b4a4d1") || strings.Contains(lower, "linux"):
		return "◈"
	case node.FileSystemPath != "" && len(node.FileSystemPath) >= 2 && node.FileSystemPath[1] == ':':
		return "▰"
	case node.Folder:
		return "▰"
	default:
		return "◇"
	}
}

func showGalleryIndexingRequired(pf *PanelsFrame) *vtui.Window {
	dialog := vtui.ShowMessageEx(
		" "+Msg("WindowsLocations.GalleryTitle")+" ",
		Msg("WindowsLocations.GalleryIndexingInstruction")+"\n\n"+
			Msg("WindowsLocations.GalleryIndexingContent"),
		[]string{
			Msg("WindowsLocations.GalleryOpenIndexingSettings"),
			Msg("WindowsLocations.GalleryCancel"),
		},
		vtui.MessageWarn,
	)
	dialog.OnResult = func(code int) {
		if code != 0 {
			return
		}
		vtui.RunAsync(func(task *vtui.TaskContext) {
			err := pf.runExternalUICommand("control.exe", []string{"/name", "Microsoft.IndexingOptions"}, "")
			if err == nil {
				return
			}
			task.RunOnUI(func() {
				vtui.ShowMessageEx(
					" "+Msg("WindowsLocations.GalleryTitle")+" ",
					fmt.Sprintf(Msg("WindowsLocations.GallerySettingsErrorFmt"), err),
					[]string{"&Ok"},
					vtui.MessageWarn,
				)
			})
		})
	}
	return dialog
}

func windowsLocationNavigationError(node winshell.Node, target string) error {
	if node.FileSystemPath != "" {
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New(Msg("WindowsLocations.AccessNotFolder"))
		}
	}
	return errors.New(Msg("WindowsLocations.AccessUnavailable"))
}

func showWindowsLocationAccessError(node winshell.Node, target string, err error) *vtui.Window {
	title := strings.TrimSpace(node.Name)
	if title == "" {
		title = strings.TrimSpace(Msg("WindowsLocations.Title"))
	}
	return vtui.ShowMessageEx(
		" "+title+" ",
		fmt.Sprintf(Msg("WindowsLocations.AccessErrorFmt"), target, err),
		[]string{Msg("vtui.Ok")},
		vtui.MessageWarn,
	)
}

func (m *windowsLocationsMenu) selectedRow() (windowsLocationRow, bool) {
	if m.SelectPos < 0 || m.SelectPos >= len(m.rows) || m.SelectPos >= len(m.Items) || m.Items[m.SelectPos].Separator {
		return windowsLocationRow{}, false
	}
	row := m.rows[m.SelectPos]
	return row, !row.loading && row.node.URI != ""
}

func (m *windowsLocationsMenu) toggleSelected(expandOnly bool) bool {
	row, ok := m.selectedRow()
	if !ok || !row.node.Folder || !row.node.HasChildren {
		return false
	}
	uri := row.node.URI
	m.mu.Lock()
	if m.expanded[uri] && !expandOnly {
		m.expanded[uri] = false
		m.mu.Unlock()
		m.rebuildFromModel(uri)
		return true
	}
	if m.expanded[uri] && expandOnly {
		m.mu.Unlock()
		return true
	}
	m.expanded[uri] = true
	children, loaded := m.children[uri]
	_ = children
	if loaded || m.loading[uri] {
		m.mu.Unlock()
		m.rebuildFromModel(uri)
		return true
	}
	m.loading[uri] = true
	m.mu.Unlock()
	m.rebuildFromModel(uri)

	task := vtui.RunAsync(func(task *vtui.TaskContext) {
		children, err := m.client.NavigationChildren(task.Context, row.node.ParsingName)
		task.RunOnUI(func() {
			if m.IsDone() {
				return
			}
			m.mu.Lock()
			delete(m.loading, uri)
			if err == nil {
				m.children[uri] = children
			} else {
				m.expanded[uri] = false
			}
			m.mu.Unlock()
			m.rebuildFromModel(uri)
			if err != nil {
				vtui.ShowMessage(" Windows locations ", fmt.Sprintf("Cannot expand %s:\n%v", row.node.Name, err), []string{"&Ok"})
			}
		})
	})
	m.tasks = append(m.tasks, task)
	return true
}

func (m *windowsLocationsMenu) collapseSelected() bool {
	row, ok := m.selectedRow()
	if !ok {
		return false
	}
	m.mu.Lock()
	if m.expanded[row.node.URI] {
		m.expanded[row.node.URI] = false
		m.mu.Unlock()
		m.rebuildFromModel(row.node.URI)
		return true
	}
	parentURI := row.parentURI
	m.mu.Unlock()
	if parentURI != "" {
		for index, candidate := range m.rows {
			if candidate.node.URI == parentURI {
				m.SetSelectPos(index)
				return true
			}
		}
	}
	return false
}

func (m *windowsLocationsMenu) activateSelected() bool {
	row, ok := m.selectedRow()
	if !ok || !row.node.Folder {
		return true
	}
	if row.node.RequiresIndexing {
		m.Close()
		m.parent.Close()
		vtui.FrameManager.PostTask(func() { showGalleryIndexingRequired(m.pf) })
		return true
	}
	target := row.node.URI
	if row.node.FileSystemPath != "" {
		target = row.node.FileSystemPath
	}
	m.Close()
	m.parent.Close()
	vtui.FrameManager.PostTask(func() {
		fsp, ok := m.pf.panels[m.panelIdx].(*FileSystemPanel)
		if ok && !m.pf.NavigateToPath(fsp, target) {
			showWindowsLocationAccessError(row.node, target, windowsLocationNavigationError(row.node, target))
		}
	})
	return true
}

func (m *windowsLocationsMenu) openContextMenu(row windowsLocationRow, x, y int) {
	if row.node.URI == "" || row.node.ParsingName == "" {
		return
	}
	task := vtui.RunAsync(func(task *vtui.TaskContext) {
		menu, err := m.client.ContextMenu(task.Context, row.node.ParsingName)
		task.RunOnUI(func() {
			if m.IsDone() {
				if err == nil && menu.Token != 0 {
					go dismissShellContextToken(m.client, menu.Token)
				}
				return
			}
			if err != nil {
				vtui.ShowMessage(" Windows Shell ", fmt.Sprintf("Cannot read commands for %s:\n%v", row.node.Name, err), []string{"&Ok"})
				return
			}
			showShellContextMenu(m, nil, menu.Token, menu.Commands, x, y)
		})
	})
	m.tasks = append(m.tasks, task)
}

func showShellContextMenu(owner *windowsLocationsMenu, parent *shellContextMenu, token uint64, commands []winshell.ContextCommand, x, y int) {
	menu := &shellContextMenu{
		VMenu:  vtui.NewVMenu(""),
		owner:  owner,
		parent: parent,
		token:  token,
	}
	configureWindowsShellMenu(menu.VMenu)
	if parent == nil {
		menu.root = menu
	} else {
		menu.root = parent.root
	}
	for _, command := range commands {
		menu.rows = append(menu.rows, shellContextRow{command: command})
		if command.Separator {
			menu.AddSeparator()
			continue
		}
		text, shortcut := splitShellContextLabel(command.Text)
		if command.Default {
			text = "◆ " + text
		}
		if !command.Enabled {
			text = "· " + text
		}
		if len(command.Children) > 0 {
			shortcut = "▶"
		}
		menu.AddItem(vtui.MenuItem{Text: text, Shortcut: shortcut})
	}
	menu.position(x, y)
	vtui.FrameManager.Push(menu)
}

func configureWindowsShellMenu(menu *vtui.VMenu) {
	if menu != nil && menu.ScrollBar != nil {
		menu.ScrollBar.ColorIdx = vtui.ColMenuBox
	}
}

func splitShellContextLabel(value string) (text, shortcut string) {
	parts := strings.SplitN(value, "\t", 2)
	text = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		shortcut = strings.TrimSpace(parts[1])
	}
	return text, shortcut
}

func (m *shellContextMenu) position(x, y int) {
	screenW := vtui.FrameManager.GetScreenSize()
	screenH := vtui.FrameManager.GetScreenHeight()
	w := 24
	for _, item := range m.Items {
		clean, _, _ := vtui.ParseAmpersandString(item.Text)
		candidate := runewidth.StringWidth(clean) + runewidth.StringWidth(item.Shortcut) + 6
		if candidate > w {
			w = candidate
		}
	}
	if screenW > 4 && w > screenW-4 {
		w = screenW - 4
	}
	h := len(m.Items) + 2
	if screenH > 2 && h > screenH-2 {
		h = screenH - 2
	}
	if x+w > screenW {
		x = x - w
	}
	if x < 0 {
		x = 0
	}
	if y+h > screenH {
		y = screenH - h
	}
	if y < 0 {
		y = 0
	}
	m.SetPosition(x, y, x+w-1, y+h-1)
}

func (m *shellContextMenu) selectedCommand() (winshell.ContextCommand, bool) {
	if m.SelectPos < 0 || m.SelectPos >= len(m.rows) || m.SelectPos >= len(m.Items) || m.Items[m.SelectPos].Separator {
		return winshell.ContextCommand{}, false
	}
	command := m.rows[m.SelectPos].command
	return command, command.Enabled
}

func (m *shellContextMenu) activateSelected() bool {
	command, ok := m.selectedCommand()
	if !ok {
		return true
	}
	if len(command.Children) > 0 {
		showShellContextMenu(m.owner, m, m.token, command.Children, m.X2+1, m.Y1+m.SelectPos+1)
		return true
	}
	root := m.root
	if root.invoking {
		return true
	}
	root.invoking = true
	for current := m; current != nil; current = current.parent {
		current.VMenu.Close()
	}
	client, token := m.owner.client, m.token
	vtui.RunAsync(func(task *vtui.TaskContext) {
		err := client.InvokeContextCommand(task.Context, token, command.ID)
		task.RunOnUI(func() {
			if err != nil {
				vtui.ShowMessage(" Windows Shell ", fmt.Sprintf("Command failed:\n%v", err), []string{"&Ok"})
				return
			}
			m.owner.reloadRoots()
		})
	})
	return true
}

func (m *shellContextMenu) dismissRoot() {
	if m.root != m || m.invoking || m.token == 0 {
		return
	}
	token := m.token
	m.token = 0
	go dismissShellContextToken(m.owner.client, token)
}

func dismissShellContextToken(client *winshell.Client, token uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = client.DismissContextMenu(ctx, token)
}

func (m *shellContextMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return m.VMenu.ProcessKey(e)
	}
	if !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10, vtinput.VK_LEFT:
		m.Close()
		return true
	case vtinput.VK_RETURN, vtinput.VK_RIGHT:
		return m.activateSelected()
	}
	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		translated := unicode.ToLower(vtui.GlobalXlator.Translate(e.Char))
		for index, item := range m.Items {
			if item.Separator {
				continue
			}
			hotkey := vtui.ExtractHotkey(item.Text)
			if hotkey != 0 && (hotkey == charLower || hotkey == translated) {
				m.SetSelectPos(index)
				return m.activateSelected()
			}
		}
	}
	return m.VMenu.ProcessKey(e)
}

func (m *shellContextMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}
	if e.ButtonState != vtinput.FromLeft1stButtonPressed || !e.KeyDown || e.MouseEventFlags&vtinput.MouseMoved != 0 {
		return m.VMenu.ProcessMouse(e)
	}
	index := m.GetClickIndex(int(e.MouseY))
	if index < 0 || index >= len(m.Items) || m.Items[index].Separator {
		return false
	}
	m.SetSelectPos(index)
	return m.activateSelected()
}

func (m *shellContextMenu) Close() {
	if m.parent == nil {
		m.dismissRoot()
	}
	m.VMenu.Close()
}

var _ vtui.Frame = (*shellContextMenu)(nil)

func (m *windowsLocationsMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return m.VMenu.ProcessKey(e)
	}
	if !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		m.Close()
		return true
	case vtinput.VK_RIGHT:
		if m.toggleSelected(true) {
			return true
		}
	case vtinput.VK_LEFT:
		if m.collapseSelected() {
			return true
		}
		m.Close()
		return true
	case vtinput.VK_RETURN:
		return m.activateSelected()
	}
	return m.VMenu.ProcessKey(e)
}

func (m *windowsLocationsMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}
	if e.ButtonState == vtinput.RightmostButtonPressed && e.KeyDown && e.MouseEventFlags&vtinput.MouseMoved == 0 {
		index := m.GetClickIndex(int(e.MouseY))
		if index < 0 || index >= len(m.Items) || m.Items[index].Separator {
			return false
		}
		m.SetSelectPos(index)
		if row, ok := m.selectedRow(); ok {
			m.openContextMenu(row, int(e.MouseX), int(e.MouseY))
		}
		return true
	}
	isMove := e.MouseEventFlags&vtinput.MouseMoved != 0
	isRelease := !isMove && (e.ButtonState&vtinput.FromLeft1stButtonPressed == 0 || !e.KeyDown)
	if isRelease && m.dragArmed {
		dragging := m.dragging
		m.dragArmed, m.dragging = false, false
		if dragging {
			if row, ok := m.selectedRow(); ok && row.node.Folder {
				move := e.ControlKeyState&vtinput.ShiftPressed != 0 &&
					e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) == 0
				m.executeInternalDrop(row.node, move)
			}
			return true
		}
		return m.activateSelected()
	}
	if isMove && m.dragArmed && e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 {
		if int(e.MouseX) != m.dragStartX || int(e.MouseY) != m.dragStartY {
			m.dragging = true
		}
		index := m.GetClickIndex(int(e.MouseY))
		if index >= 0 && index < len(m.Items) && !m.Items[index].Separator {
			m.SetSelectPos(index)
			vtui.FrameManager.Redraw()
		}
		return true
	}
	if e.ButtonState != vtinput.FromLeft1stButtonPressed || !e.KeyDown || isMove {
		return m.VMenu.ProcessMouse(e)
	}
	index := m.GetClickIndex(int(e.MouseY))
	if index < 0 || index >= len(m.Items) || m.Items[index].Separator {
		return false
	}
	m.SetSelectPos(index)
	row, ok := m.selectedRow()
	if !ok {
		return true
	}
	chevronRight := m.X1 + 3 + row.depth*2
	if row.node.HasChildren && int(e.MouseX) <= chevronRight {
		return m.toggleSelected(false)
	}
	m.dragArmed = true
	m.dragStartX, m.dragStartY = int(e.MouseX), int(e.MouseY)
	m.dragSource, m.dragNames = nil, nil
	if source, sourceOK := m.pf.panels[m.panelIdx].(*FileSystemPanel); sourceOK {
		m.dragNames = source.GetMarkedNames()
		if len(m.dragNames) > 0 {
			m.dragSource = source
		} else {
			m.dragSource = nil
		}
	}
	return true
}

func (m *windowsLocationsMenu) executeInternalDrop(target winshell.Node, move bool) {
	source, names := m.dragSource, append([]string(nil), m.dragNames...)
	m.dragSource, m.dragNames = nil, nil
	if target.RequiresIndexing {
		m.Close()
		m.parent.Close()
		vtui.FrameManager.PostTask(func() { showGalleryIndexingRequired(m.pf) })
		return
	}
	if source == nil || source.vfs == nil || len(names) == 0 {
		return
	}
	m.Close()
	m.parent.Close()
	start := func(destination vfs.VFS) {
		if destination == nil {
			return
		}
		go ExecuteFileOp(m.pf, source.vfs, destination, names, destination.GetPath(), move,
			AppConfig.DefaultFileOpMode, func() {
				vtui.FrameManager.PostTask(func() {
					_ = destination.Close()
					m.pf.RefreshAll()
					vtui.FrameManager.Redraw()
				})
			})
	}
	if target.FileSystemPath != "" {
		start(vfs.NewOSVFS(target.FileSystemPath))
		return
	}
	provider := vfs.FindURIProvider(target.URI)
	if provider == nil {
		vtui.ShowMessage(" Windows locations ", "The Windows Shell provider is unavailable.", []string{"&Ok"})
		return
	}
	task := vtui.RunAsync(func(task *vtui.TaskContext) {
		destination, err := provider.OpenURI(task.Context, source.vfs, target.URI)
		task.RunOnUI(func() {
			if err != nil {
				vtui.ShowMessage(" Windows locations ", fmt.Sprintf("Cannot open drop target:\n%v", err), []string{"&Ok"})
				return
			}
			start(destination)
		})
	})
	m.tasks = append(m.tasks, task)
}

func (m *windowsLocationsMenu) ResizeConsole(_, _ int) {
	m.positionBesideParent()
}

func (m *windowsLocationsMenu) Show(scr *vtui.ScreenBuf) {
	m.VMenu.Show(scr)
	if scr == nil || !scr.SupportsGraphics() {
		return
	}
	visibleHeight := m.Y2 - m.Y1 - 1
	for visual := 0; visual < visibleHeight; visual++ {
		index := m.TopPos + visual
		if index < 0 || index >= len(m.rows) {
			continue
		}
		row := m.rows[index]
		icon := row.node.IconRGBA
		if row.loading || row.node.Separator || row.node.IconWidth <= 0 || row.node.IconHeight <= 0 ||
			len(icon) < row.node.IconWidth*row.node.IconHeight*4 {
			continue
		}
		surface := vtui.NewImageSurfaceFromPix(row.node.IconWidth, row.node.IconHeight,
			row.node.IconWidth*4, icon)
		scr.Graphics().DrawImage(fmt.Sprintf("windows-location-%p-%s", m, row.node.URI), vtui.ImagePlacement{
			Surface: surface,
			Col:     m.X1 + 4 + row.depth*2,
			Row:     m.Y1 + 1 + visual,
			Cols:    1,
			Rows:    1,
			ZIndex:  20,
		})
	}
}

func (m *windowsLocationsMenu) Close() {
	for _, task := range m.tasks {
		if task != nil && task.Cancel != nil {
			task.Cancel()
		}
	}
	m.VMenu.Close()
}

// Compile-time guard for the custom menu frame contract.
var _ vtui.Frame = (*windowsLocationsMenu)(nil)
