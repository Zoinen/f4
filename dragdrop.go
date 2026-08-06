package main

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// dragOutState remembers a left button press that landed on a marked file.
// That is the only press allowed to become a drag out of f4: a press on an
// unmarked file still just moves the cursor, as it always has.
type dragOutState struct {
	panel *FileSystemPanel
	x, y  int
	armed bool
}

// readOnlyVFS is implemented by a file system that already knows it cannot
// be written to. Nothing implements it yet: a VFS that does not is assumed
// to accept files, and a refusal then arrives as an ordinary file operation
// error, in the dialog the user already knows. Archives that are opened
// read-only should implement it so the pointer says "no" before the drop.
type readOnlyVFS interface {
	IsReadOnly() bool
}

// dropTargetInfo is where a payload dropped at a screen cell would land.
// The destination is a directory inside the panel's own VFS, which may be an
// archive or a network connection just as well as the local disk.
type dropTargetInfo struct {
	panelIdx int
	panel    *FileSystemPanel
	dir      string
	fs       vfs.VFS
	entryIdx int
}

// dropSourceGroup is one source directory and the names taken from it. A
// file manager copies "these names out of that directory", so a drop of
// files scattered over several directories becomes several operations.
type dropSourceGroup struct {
	dir   string
	names []string
}

// groupDropSources turns the absolute paths of a payload into groups,
// preserving the order the directories were first seen and sorting the names
// inside each group, so the same drop always produces the same operations.
func groupDropSources(paths []string) []dropSourceGroup {
	byDir := make(map[string][]string)
	seen := make(map[string]bool)
	var order []string

	for _, raw := range paths {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		cleaned := filepath.Clean(raw)
		dir, name := filepath.Split(cleaned)
		dir = filepath.Clean(dir)
		if name == "" || name == "." || name == ".." {
			continue
		}
		key := dir + string(filepath.Separator) + name
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, ok := byDir[dir]; !ok {
			order = append(order, dir)
		}
		byDir[dir] = append(byDir[dir], name)
	}

	groups := make([]dropSourceGroup, 0, len(order))
	for _, dir := range order {
		names := byDir[dir]
		sort.Strings(names)
		groups = append(groups, dropSourceGroup{dir: dir, names: names})
	}
	return groups
}

// chooseDropAction picks what a drop does. Every graphical file manager, and
// Far itself for its own drags, agrees on the modifiers: Shift moves, Ctrl
// copies. With neither the source's own suggestion wins, and copy is the
// fallback because it is the one that cannot lose data.
func chooseDropAction(allowed, suggested vtui.DropAction, mods vtinput.ControlKeyState) vtui.DropAction {
	if allowed == vtui.DropNone {
		return vtui.DropNone
	}
	shift := mods&vtinput.ShiftPressed != 0
	ctrl := mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0

	if shift && !ctrl && allowed.Has(vtui.DropMove) {
		return vtui.DropMove
	}
	if ctrl && !shift && allowed.Has(vtui.DropCopy) {
		return vtui.DropCopy
	}
	if suggested != vtui.DropNone && allowed.Has(suggested) {
		return suggested
	}
	if allowed.Has(vtui.DropCopy) {
		return vtui.DropCopy
	}
	if allowed.Has(vtui.DropMove) {
		return vtui.DropMove
	}
	return vtui.DropNone
}

// vfsAcceptsDrop reports whether files can be written into this file system.
func vfsAcceptsDrop(v vfs.VFS) bool {
	if v == nil {
		return false
	}
	if ro, ok := v.(readOnlyVFS); ok && ro.IsReadOnly() {
		return false
	}
	return true
}

// resolveDropTarget maps a screen cell to the panel under it and to the
// directory a drop there would go into: the directory under the cursor if
// the cursor is on one, otherwise the panel's current directory.
func (pf *PanelsFrame) resolveDropTarget(mx, my int) (dropTargetInfo, bool) {
	info := dropTargetInfo{panelIdx: -1, entryIdx: -1}
	if pf == nil || !pf.showPanels {
		return info, false
	}

	for i, p := range pf.panels {
		if pf.wide && i != pf.widePanel {
			continue
		}
		if !pf.wide && i == 0 && !pf.showLeftPanel {
			continue
		}
		if !pf.wide && i == 1 && !pf.showRightPanel {
			continue
		}
		fsp, ok := p.(*FileSystemPanel)
		if !ok || fsp == nil || fsp.vfs == nil {
			continue
		}
		x1, y1, x2, y2 := fsp.GetPosition()
		if mx < x1 || mx > x2 || my < y1 || my > y2 {
			continue
		}

		info.panelIdx, info.panel, info.fs = i, fsp, fsp.vfs
		info.dir = fsp.vfs.GetPath()

		// An info or quick view panel covers the file panel: the file
		// panel is still the logical target, but no row under the
		// cursor belongs to it, so the drop goes to its directory.
		if pf.altPanels[i] != nil {
			return info, true
		}

		if idx := fsp.mouseEntryIndex(mx, my); idx >= 0 && idx < len(fsp.entries) {
			e := fsp.entries[idx]
			if e.IsDir && e.Name != ".." {
				info.dir = fsp.vfs.Join(fsp.vfs.GetPath(), e.Name)
				info.entryIdx = idx
			}
		}
		return info, true
	}
	return info, false
}

// HandleDrag implements vtui.DropTarget. It answers what a drop at this cell
// would do, and on the drop itself starts the transfer.
func (pf *PanelsFrame) HandleDrag(ev *vtui.DragEvent) vtui.DropAction {
	if ev == nil || ev.Phase == vtui.DragLeave {
		return vtui.DropNone
	}
	if !ev.Payload.OffersFiles() {
		return vtui.DropNone
	}
	// During the drag a protocol may only announce the type; by the drop
	// the paths have to be there, or there is nothing to transfer.
	if ev.Phase == vtui.DragDrop && !ev.Payload.HasFiles() {
		return vtui.DropNone
	}
	info, ok := pf.resolveDropTarget(ev.X, ev.Y)
	if !ok || !vfsAcceptsDrop(info.fs) {
		if ev.Phase != vtui.DragOver {
			vtui.DebugLog("DND: nothing at cell %d,%d accepts a drop", ev.X, ev.Y)
		}
		return vtui.DropNone
	}
	action := chooseDropAction(ev.Allowed, ev.Suggested, ev.Modifiers)
	if ev.Phase != vtui.DragOver {
		vtui.DebugLog("DND: a drop at %d,%d would go to %q as %s",
			ev.X, ev.Y, info.dir, action)
	}
	if action == vtui.DropNone || ev.Phase != vtui.DragDrop {
		return action
	}
	pf.dropExternalFiles(info, ev.Payload.Paths, action == vtui.DropMove)
	return action
}

// dropExternalFiles brings files dropped by another application into the
// panel. The panel may be showing an archive or a network connection, so the
// transfer goes through its VFS and the usual progress, overwrite and error
// dialogs - the same road F5 takes. Groups run one after another rather than
// at once, so their dialogs do not fight over the screen.
func (pf *PanelsFrame) dropExternalFiles(info dropTargetInfo, paths []string, isMove bool) {
	if pf == nil || info.fs == nil {
		return
	}
	groups := groupDropSources(paths)
	if len(groups) == 0 {
		return
	}

	dst, dstDir := info.fs, info.dir
	var run func(i int)
	run = func(i int) {
		if i >= len(groups) {
			vtui.FrameManager.PostTask(func() {
				pf.RefreshAll()
				vtui.FrameManager.Redraw()
			})
			return
		}
		g := groups[i]
		src := vfs.NewOSVFS(g.dir)
		go ExecuteFileOp(pf, src, dst, g.names, dstDir, isMove, AppConfig.DefaultFileOpMode, func() {
			run(i + 1)
		})
	}
	run(0)
}

// processDragOutGesture watches the left button for the start of a drag out
// of f4. It returns true only once a drag has actually begun, so the press
// itself and every ordinary drag fall through to the panel untouched.
func (pf *PanelsFrame) processDragOutGesture(e *vtinput.InputEvent, mx, my int) bool {
	if pf == nil || e == nil || e.Type != vtinput.MouseEventType || e.WheelDirection != 0 {
		return false
	}
	if e.ButtonState&vtinput.FromLeft1stButtonPressed == 0 {
		pf.dragOut = dragOutState{}
		return false
	}

	if e.MouseEventFlags&vtinput.MouseMoved == 0 {
		pf.dragOut = dragOutState{}
		if !e.KeyDown {
			return false
		}
		info, ok := pf.resolveDropTarget(mx, my)
		if !ok || info.panel == nil || pf.altPanels[info.panelIdx] != nil {
			return false
		}
		idx := info.panel.mouseEntryIndex(mx, my)
		if idx < 0 || idx >= len(info.panel.entries) || !info.panel.entries[idx].Selected {
			return false
		}
		pf.dragOut = dragOutState{panel: info.panel, x: mx, y: my, armed: true}
		vtui.DebugLog("DND: drag out armed on a marked file at %d,%d", mx, my)
		return false
	}

	if !pf.dragOut.armed || (mx == pf.dragOut.x && my == pf.dragOut.y) {
		return false
	}
	panel := pf.dragOut.panel
	pf.dragOut = dragOutState{}
	vtui.DebugLog("DND: drag out gesture triggered at %d,%d", mx, my)
	return pf.startDragOut(panel)
}

// dragOutRefusal names the reason a drag out cannot start, or "" when it
// can. One place rather than three, so the guard and the log line it writes
// can never drift apart.
func dragOutRefusal(fsp *FileSystemPanel) string {
	if fsp == nil {
		return "no panel under the pointer"
	}
	if !vtui.DragOutSupported() {
		return "the backend offers no drag source"
	}
	if len(fsp.GetMarkedNames()) == 0 {
		return "nothing is marked"
	}
	return ""
}

// startDragOut offers the marked files to the rest of the desktop. Only copy
// is offered: a move would have f4 delete the originals because the receiver
// said it took them, which is more trust than files deserve.
func (pf *PanelsFrame) startDragOut(fsp *FileSystemPanel) bool {
	if reason := dragOutRefusal(fsp); reason != "" {
		vtui.DebugLog("DND: drag out not started: %s", reason)
		return false
	}
	names := fsp.GetMarkedNames()
	paths, ok := localDragPaths(fsp, names)
	if !ok {
		vtui.ShowToast("Dragging files out of an archive or a network panel is not supported yet", 3*time.Second)
		return true
	}

	payload := vtui.DragPayload{Kinds: []string{"text/uri-list"}, Paths: paths}
	go func() {
		action, err := vtui.StartDrag(payload, vtui.DropCopy)
		if err != nil {
			vtui.DebugLog("DND: drag out failed: %v", err)
			return
		}
		vtui.DebugLog("DND: drag out finished as %s", action)
	}()
	return true
}

// localDragPaths turns marked names into paths another application can open.
// It reports false for anything but a local file system: a file inside an
// archive or on a remote host has no such path until it is materialised into
// a temporary directory, which is a copy nobody asked for and needs its own
// progress and cleanup - see DRAGDROP.md.
func localDragPaths(fsp *FileSystemPanel, names []string) ([]string, bool) {
	local, ok := fsp.vfs.(*vfs.OSVFS)
	if !ok {
		return nil, false
	}
	paths := make([]string, 0, len(names))
	for _, n := range names {
		paths = append(paths, local.Join(local.GetPath(), n))
	}
	return paths, true
}

// installPanelDropTarget makes the panels the drop target of whatever
// graphical backend is running. In a terminal no backend registers a drag
// and drop protocol, so the target is simply never asked anything.
func installPanelDropTarget(pf *PanelsFrame) {
	if pf == nil {
		return
	}
	vtui.SetDropTarget(pf)
}
