package panel

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"strings"
)

// Native drops use identities, never console cells or a view's row number.
// Both ends are revalidated when the UI queue actually processes the drop.
func (pf *PanelsFrame) semanticDragPanel(a map[string]any) *FileSystemPanel {
	if pf.Closed {
		return nil
	}
	fp := pf.panelForSemanticAction(a)
	if fp == nil || fp.Vfs == nil || semantic.String(a["panelId"]) != vtui.SemanticID(fp) {
		return nil
	}
	fp.updateSemanticRevisions()
	if semantic.Int64(a["catalogRevision"]) != fp.catalogRevision || semantic.String(a["path"]) != fp.Vfs.GetPath() {
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
		if semantic.WorkspaceSemanticTarget(screen.Number) != id || len(screen.Frames) == 0 {
			continue
		}
		// A document or modal on top is not a file-panel destination.
		pf, ok := screen.Frames[len(screen.Frames)-1].(*PanelsFrame)
		if ok && !pf.Closed && pf.GetWorkspaceTabSurfaceKind() == "panels" {
			return pf
		}
	}
	return nil
}

func HandleSemanticWorkspaceDrag(a map[string]any) bool {
	pf := semanticWorkspaceDragTarget(semantic.String(a["target"]))
	if pf == nil {
		return true
	}
	vtui.FrameManager.HandleSemanticAction(map[string]any{"action": "workspace.activate", "target": a["target"]})
	if semantic.String(a["action"]) == "workspace.dragActivate" {
		return true
	}
	// A drop on the tab itself targets its active panel's current directory.
	// Resolve it here, even if the hover's new scene has not reached Qt yet.
	fp := pf.panelForSemanticAction(nil)
	if fp == nil || fp.Vfs == nil {
		return true
	}
	fp.updateSemanticRevisions()
	request := make(map[string]any, len(a)+4)
	for key, value := range a {
		request[key] = value
	}
	request["action"] = "panel.dropFiles"
	request["side"] = pf.ActiveIdx
	request["panelId"] = vtui.SemanticID(fp)
	request["path"] = fp.Vfs.GetPath()
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
func PrepareSemanticDrag(a map[string]any) map[string]any {
	result := map[string]any{"type": "drag_prepared", "requestId": a["requestId"], "ok": false}
	defer func() {
		vtui.DebugLog("QT_DND: prepared request=%v ok=%v local=%v", a["requestId"], result["ok"], result["local"])
	}()
	value, exists := semanticLivePanels.Load(semantic.String(a["panelId"]))
	if !exists {
		vtui.DebugLog("QT_DND: panel not registered: %v", a["panelId"])
		return result
	}
	fp, ok := value.(*FileSystemPanel)
	if !ok || fp == nil || fp.Vfs == nil {
		return result
	}
	fp.updateSemanticRevisions()
	if fp.Vfs.GetPath() != semantic.String(a["path"]) || fp.catalogRevision != semantic.Int64(a["catalogRevision"]) {
		vtui.DebugLog("QT_DND: stale prepare: received %v@%v path=%v current=%v path=%v", a["panelId"], a["catalogRevision"], a["path"], fp.catalogRevision, fp.Vfs.GetPath())
		return result
	}
	ids := semantic.StringSlice(a["entryIds"])
	if len(ids) == 0 {
		return result
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	namesByID := make(map[string]string, len(ids))
	kind, _ := fp.semanticSourceInfo()
	for _, e := range fp.Entries {
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
	paths, local := LocalDragPaths(fp, names)
	result["ok"] = true
	result["local"] = local
	result["paths"] = paths
	return result
}

func (pf *PanelsFrame) planSemanticDrop(a map[string]any) (semanticDropPlan, error) {
	var p semanticDropPlan
	fp := pf.panelForSemanticAction(a)
	if pf.Closed || !pf.ShowPanels || fp == nil || fp.Vfs == nil || !VfsAcceptsDrop(fp.Vfs) {
		return p, fmt.Errorf("The destination is no longer available or is read-only")
	}
	if semantic.String(a["panelId"]) != vtui.SemanticID(fp) || semantic.String(a["path"]) != fp.Vfs.GetPath() {
		return p, fmt.Errorf("The destination changed during dragging")
	}
	fp.updateSemanticRevisions()
	if revision := semantic.Int64(a["catalogRevision"]); revision <= 0 || revision > fp.catalogRevision {
		return p, fmt.Errorf("The destination changed during dragging")
	}
	side := pf.panelIndexForSemanticAction(a)
	if pf.AltPanels[side] != nil || (pf.Wide && side != pf.WidePanel) || (!pf.Wide && ((side == 0 && !pf.ShowLeftPanel) || (side == 1 && !pf.ShowRightPanel))) {
		return p, fmt.Errorf("The destination panel is hidden")
	}
	p.target = dropTargetInfo{panelIdx: side, Panel: fp, fs: fp.Vfs, Dir: fp.Vfs.GetPath(), entryIdx: -1}
	if semantic.String(a["entryId"]) != "" {
		// A copy (including a cancelled copy) can refresh the destination while
		// the drop is in transit. Resolve its stable ID in the current listing;
		// never reuse an old row index or redirect to a different panel/path.
		idx, ok := fp.semanticEntryIndex(map[string]any{"entryId": a["entryId"], "catalogRevision": fp.catalogRevision})
		if !ok || idx < 0 || idx >= len(fp.Entries) {
			return p, fmt.Errorf("The destination changed during dragging")
		}
		e := fp.Entries[idx]
		if e.IsDir {
			if e.Name == ".." {
				p.target.Dir = fp.Vfs.Dir(p.target.Dir)
			} else {
				p.target.Dir = fp.Vfs.Join(p.target.Dir, e.Name)
			}
			p.target.entryIdx = idx
		}
	}
	operation := semantic.String(a["operation"])
	if operation != "copy" && operation != "move" {
		return p, fmt.Errorf("Unsupported drop operation")
	}
	p.move = operation == "move"
	if temp, ok := p.target.fs.(*TempPanelVFS); ok {
		if p.target.Dir == temp.root() {
			p.references = temp
			// Like F5/F6, adding references never deletes the originals.
			p.move = false
		} else {
			ref, path, _, valid := temp.Resolve(p.target.Dir)
			if !valid || !VfsAcceptsDrop(ref.Source) {
				return p, fmt.Errorf("The referenced destination is unavailable or read-only")
			}
			p.target.fs, p.target.Dir = ref.Source, path
		}
	}
	if source, ok := a["source"].(map[string]any); ok {
		src, owner := pf.semanticDragSource(source)
		p.sourceOwner = owner
		if src == nil {
			return p, fmt.Errorf("The source changed during dragging")
		}
		ids := semantic.StringSlice(source["entryIds"])
		if len(ids) == 0 {
			return p, fmt.Errorf("No files to transfer")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			idx, valid := src.semanticEntryIndex(map[string]any{"entryId": id, "catalogRevision": source["catalogRevision"]})
			if !valid || idx < 0 || idx >= len(src.Entries) {
				return p, fmt.Errorf("A dragged file is no longer available")
			}
			name := src.Entries[idx].Name
			if name == ".." || name == "." || name == "" {
				return p, fmt.Errorf("Cannot drag the parent directory")
			}
			if !seen[name] {
				p.names = append(p.names, name)
				seen[name] = true
			}
		}
		p.source = src.Vfs
		p.sourceDir = src.Vfs.GetPath()
		if p.move && !VfsAcceptsDrop(src.Vfs) {
			return p, fmt.Errorf("The source is read-only")
		}
		if src.Vfs == fp.Vfs && src.Vfs.GetPath() == p.target.Dir {
			p.skip = true
			return p, nil
		}
	} else {
		// Desktop moves need a separately negotiated completion/deletion protocol.
		if p.move {
			return p, fmt.Errorf("External drops currently support copying only")
		}
		p.paths = semantic.StringSlice(a["paths"])
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
	vtui.DebugLog("QT_DND: drop operation=%s panel=%s", semantic.String(a["operation"]), semantic.String(a["panelId"]))
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
			for _, group := range GroupDropSources(p.paths) {
				if addErr := p.references.AddReferences(context.Background(), vfs.NewOSVFS(group.Dir), group.Names); err == nil {
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
	go fileops.ExecuteFileOpAtIn(pf, p.source, p.target.fs, p.sourceDir, p.names, p.target.Dir, p.move, config.App.DefaultFileOpMode, func() {
		vtui.FrameManager.PostTask(func() {
			locations := []panelOperationLocation{{pf, p.target.fs, p.target.Dir}}
			if p.move {
				locations = append(locations, panelOperationLocation{p.sourceOwner, p.source, p.sourceDir})
			}
			refreshOperationViews(locations...)
			vtui.FrameManager.Redraw()
		})
	})
	return true
}
