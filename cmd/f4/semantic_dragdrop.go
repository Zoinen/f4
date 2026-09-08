package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Native drops use identities, never console cells or a view's row number.
// Both ends are revalidated when the UI queue actually processes the drop.
func (pf *PanelsFrame) semanticDragPanel(a map[string]any) *FileSystemPanel {
	if pf.closed {
		return nil
	}
	fp := pf.panelForSemanticAction(a)
	if fp == nil || fp.vfs == nil || semanticString(a["panelId"]) != vtui.SemanticID(fp) {
		return nil
	}
	fp.updateSemanticRevisions()
	if semanticInt64(a["catalogRevision"]) != fp.catalogRevision || semanticString(a["path"]) != fp.vfs.GetPath() {
		return nil
	}
	return fp
}

// A drag keeps its original panel identity when hover activates another tab.
// Search only still-owned panels, never a retained metadata cache or side alone.
func (pf *PanelsFrame) semanticDragSource(a map[string]any) (*FileSystemPanel, *PanelsFrame) {
	if source := pf.semanticDragPanel(a); source != nil {
		return source, pf
	}
	if vtui.FrameManager == nil {
		return nil, nil
	}
	for _, screen := range vtui.FrameManager.Screens {
		if screen == nil {
			continue
		}
		for _, frame := range screen.Frames {
			if owner, ok := frame.(*PanelsFrame); ok {
				if source := owner.semanticDragPanel(a); source != nil {
					return source, owner
				}
			}
		}
	}
	return nil, nil
}

func semanticWorkspaceDragTarget(id string) *PanelsFrame {
	if vtui.FrameManager == nil {
		return nil
	}
	for _, screen := range vtui.FrameManager.Screens {
		if screen == nil {
			continue
		}
		if workspaceSemanticTarget(screen.Number) != id || len(screen.Frames) == 0 {
			continue
		}
		// A document or modal on top is not a file-panel destination.
		pf, ok := screen.Frames[len(screen.Frames)-1].(*PanelsFrame)
		if ok && !pf.closed && pf.GetWorkspaceTabSurfaceKind() == "panels" {
			return pf
		}
	}
	return nil
}

func handleSemanticWorkspaceDrag(a map[string]any) bool {
	pf := semanticWorkspaceDragTarget(semanticString(a["target"]))
	if pf == nil {
		return true
	}
	vtui.FrameManager.HandleSemanticAction(map[string]any{"action": "workspace.activate", "target": a["target"]})
	if semanticString(a["action"]) == "workspace.dragActivate" {
		return true
	}
	// A drop on the tab itself targets its active panel's current directory.
	// Resolve it here, even if the hover's new scene has not reached Qt yet.
	fp := pf.panelForSemanticAction(nil)
	if fp == nil || fp.vfs == nil {
		return true
	}
	fp.updateSemanticRevisions()
	request := make(map[string]any, len(a)+4)
	for key, value := range a {
		request[key] = value
	}
	request["action"] = "panel.dropFiles"
	request["side"] = pf.activeIdx
	request["panelId"] = vtui.SemanticID(fp)
	request["path"] = fp.vfs.GetPath()
	request["catalogRevision"] = fp.catalogRevision
	delete(request, "entryId")
	return pf.handleSemanticDrop(request)
}

type semanticDropPlan struct {
	skip        bool
	target      dropTargetInfo
	source      vfs.VFS
	sourceOwner *PanelsFrame
	sourceDir   string
	names       []string
	paths       []string
	move        bool
	references  *TempPanelVFS
}

// Resolve the entire marked set in Go, including entries outside Qt's sparse
// viewport cache. No filesystem I/O or copying takes place during preparation.
func prepareSemanticDrag(a map[string]any) map[string]any {
	result := map[string]any{"type": "drag_prepared", "requestId": a["requestId"], "ok": false}
	defer func() {
		vtui.DebugLog("QT_DND: prepared request=%v ok=%v local=%v", a["requestId"], result["ok"], result["local"])
	}()
	value, exists := semanticLivePanels.Load(semanticString(a["panelId"]))
	if !exists {
		vtui.DebugLog("QT_DND: panel not registered: %v", a["panelId"])
		return result
	}
	fp, ok := value.(*FileSystemPanel)
	if !ok || fp == nil || fp.vfs == nil {
		return result
	}
	fp.updateSemanticRevisions()
	if fp.vfs.GetPath() != semanticString(a["path"]) || fp.catalogRevision != semanticInt64(a["catalogRevision"]) {
		vtui.DebugLog("QT_DND: stale prepare: received %v@%v path=%v current=%v path=%v", a["panelId"], a["catalogRevision"], a["path"], fp.catalogRevision, fp.vfs.GetPath())
		return result
	}
	ids := semanticStringSlice(a["entryIds"])
	if len(ids) == 0 {
		return result
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	namesByID := make(map[string]string, len(ids))
	kind, _ := fp.semanticSourceInfo()
	for _, e := range fp.entries {
		id, _ := fp.semanticEntryMetadata(e, kind)
		if wanted[id] && e.Name != ".." && e.Name != "." && e.Name != "" {
			namesByID[id] = e.Name
		}
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name, found := namesByID[id]
		if !found {
			return result
		}
		names = append(names, name)
	}
	paths, local := localDragPaths(fp, names)
	result["ok"] = true
	result["local"] = local
	result["paths"] = paths
	return result
}

func (pf *PanelsFrame) planSemanticDrop(a map[string]any) (semanticDropPlan, error) {
	var p semanticDropPlan
	fp := pf.semanticDragPanel(a)
	if !pf.showPanels || fp == nil || !vfsAcceptsDrop(fp.vfs) {
		return p, fmt.Errorf("The destination is no longer available or is read-only")
	}
	side := pf.panelIndexForSemanticAction(a)
	if pf.altPanels[side] != nil || (pf.wide && side != pf.widePanel) || (!pf.wide && ((side == 0 && !pf.showLeftPanel) || (side == 1 && !pf.showRightPanel))) {
		return p, fmt.Errorf("The destination panel is hidden")
	}
	p.target = dropTargetInfo{panelIdx: side, panel: fp, fs: fp.vfs, dir: fp.vfs.GetPath(), entryIdx: -1}
	if semanticString(a["entryId"]) != "" {
		idx, ok := fp.semanticEntryIndex(a)
		if !ok || idx < 0 || idx >= len(fp.entries) {
			return p, fmt.Errorf("The destination changed during dragging")
		}
		e := fp.entries[idx]
		if e.IsDir && e.Name != ".." {
			p.target.dir = fp.vfs.Join(p.target.dir, e.Name)
			p.target.entryIdx = idx
		}
	}
	operation := semanticString(a["operation"])
	if operation != "copy" && operation != "move" {
		return p, fmt.Errorf("Unsupported drop operation")
	}
	p.move = operation == "move"
	if temp, ok := p.target.fs.(*TempPanelVFS); ok {
		if p.target.dir == temp.root() {
			p.references = temp
			// Like F5/F6, adding references never deletes the originals.
			p.move = false
		} else {
			ref, path, _, valid := temp.resolve(p.target.dir)
			if !valid || !vfsAcceptsDrop(ref.source) {
				return p, fmt.Errorf("The referenced destination is unavailable or read-only")
			}
			p.target.fs, p.target.dir = ref.source, path
		}
	}
	if source, ok := a["source"].(map[string]any); ok {
		src, owner := pf.semanticDragSource(source)
		p.sourceOwner = owner
		if src == nil {
			return p, fmt.Errorf("The source changed during dragging")
		}
		ids := semanticStringSlice(source["entryIds"])
		if len(ids) == 0 {
			return p, fmt.Errorf("No files to transfer")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			idx, valid := src.semanticEntryIndex(map[string]any{"entryId": id, "catalogRevision": source["catalogRevision"]})
			if !valid || idx < 0 || idx >= len(src.entries) {
				return p, fmt.Errorf("A dragged file is no longer available")
			}
			name := src.entries[idx].Name
			if name == ".." || name == "." || name == "" {
				return p, fmt.Errorf("Cannot drag the parent directory")
			}
			if !seen[name] {
				p.names = append(p.names, name)
				seen[name] = true
			}
		}
		p.source = src.vfs
		p.sourceDir = src.vfs.GetPath()
		if p.move && !vfsAcceptsDrop(src.vfs) {
			return p, fmt.Errorf("The source is read-only")
		}
		if src.vfs == fp.vfs && src.vfs.GetPath() == p.target.dir {
			p.skip = true
			return p, nil
		}
	} else {
		// Desktop moves need a separately negotiated completion/deletion protocol.
		if p.move {
			return p, fmt.Errorf("External drops currently support copying only")
		}
		p.paths = semanticStringSlice(a["paths"])
		if len(p.paths) == 0 {
			return p, fmt.Errorf("No local files in the drop")
		}
		for _, path := range p.paths {
			if strings.TrimSpace(path) == "" || !vfs.NewOSVFS("").IsAbs(path) {
				return p, fmt.Errorf("Dropped paths must be absolute")
			}
		}
	}
	return p, nil
}

func (pf *PanelsFrame) handleSemanticDrop(a map[string]any) bool {
	vtui.DebugLog("QT_DND: drop operation=%s panel=%s", semanticString(a["operation"]), semanticString(a["panelId"]))
	p, err := pf.planSemanticDrop(a)
	if err != nil {
		vtui.ShowMessage(" Drag and Drop ", err.Error(), []string{"&Ok"})
		return true
	}
	if p.skip {
		return true
	}
	if p.references != nil {
		if p.source != nil {
			err = p.references.store.addReferencesAt(context.Background(), p.references.slot, p.source, p.sourceDir, p.names)
		} else {
			for _, group := range groupDropSources(p.paths) {
				if addErr := p.references.AddReferences(context.Background(), vfs.NewOSVFS(group.dir), group.names); err == nil {
					err = addErr
				}
			}
		}
		if err != nil {
			vtui.ShowMessage(" Drag and Drop ", err.Error(), []string{"&Ok"})
		}
		pf.RefreshAll()
		vtui.FrameManager.Redraw()
		return true
	}
	if p.source == nil {
		pf.dropExternalFiles(p.target, p.paths, false)
		return true
	}
	go ExecuteFileOpAt(pf, p.source, p.target.fs, p.sourceDir, p.names, p.target.dir, p.move, AppConfig.DefaultFileOpMode, func() {
		vtui.FrameManager.PostTask(func() {
			locations := []panelOperationLocation{{pf, p.target.fs, p.target.dir}}
			if p.move {
				locations = append(locations, panelOperationLocation{p.sourceOwner, p.source, p.sourceDir})
			}
			refreshOperationViews(locations...)
			vtui.FrameManager.Redraw()
		})
	})
	return true
}
