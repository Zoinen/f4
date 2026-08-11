package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type semanticBlockingLocalVFS struct {
	*vfs.OSVFS
	readStarted chan struct{}
	readDone    chan struct{}
}

func TestSemanticPanelReusesStaticCatalogForFocusOnlyChanges(t *testing.T) {
	panel := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(t.TempDir()),
		table:         vtui.NewTable(0, 0, 40, 10, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "main.go", Size: 10}},
			{VFSItem: vfs.VFSItem{Name: "README.md", Size: 20}},
		},
	}

	first := panel.semanticPanelModel(nil, 0, true)
	cache := panel.semanticStaticCache
	if cache == nil || len(first.Entries) != 2 {
		t.Fatal("semantic static catalog was not initialized")
	}

	panel.entries[1].Selected = true
	second := panel.semanticPanelModel(nil, 0, false)
	if panel.semanticStaticCache != cache {
		t.Fatal("focus/selection-only change rebuilt the static catalog")
	}
	if second.Active || !second.Entries[1].Selected || second.SelectedCount != 1 {
		t.Fatalf("dynamic panel state was not refreshed: %#v", second)
	}

	panel.entries[0].Name = "changed.go"
	third := panel.semanticPanelModel(nil, 0, false)
	if panel.semanticStaticCache == cache {
		t.Fatal("catalog mutation did not invalidate the static cache")
	}
	if third.Entries[0].Name != "changed.go" {
		t.Fatalf("rebuilt catalog contains stale entry: %#v", third.Entries[0])
	}
}

func (v *semanticBlockingLocalVFS) ReadDir(ctx context.Context, _ string, _ func([]vfs.VFSItem)) error {
	close(v.readStarted)
	defer close(v.readDone)
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkspaceSemanticSwitchUsesExistingScreenModel(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(vtui.NewWindow(0, 0, 20, 10, "One"))
	vtui.FrameManager.AddScreen(vtui.NewWindow(0, 0, 20, 10, "Two"))

	if len(vtui.FrameManager.Screens) != 2 {
		t.Fatalf("workspace count = %d, want 2", len(vtui.FrameManager.Screens))
	}
	if got := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].GetTitle(); got != "Two" {
		t.Fatalf("initial active workspace = %q, want Two", got)
	}

	if !HandleSemanticAction(map[string]any{
		"target": "workspace-tab-1",
		"action": "workspace.activate",
		"index":  0,
	}) {
		t.Fatal("workspace switch action was not handled")
	}
	if got := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].GetTitle(); got != "One" {
		t.Fatalf("active workspace after switch = %q, want One", got)
	}

	if HandleSemanticAction(map[string]any{
		"target": "workspace-tab-99",
		"action": "workspace.activate",
		"index":  99,
	}) {
		t.Fatal("out-of-range workspace switch was accepted")
	}
}

func TestMenuBarPointerSelectionRejectsStalePopup(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(vtui.NewDesktop())

	menuBar := vtui.NewMenuBar([]string{"&Left", "&Right"})
	menuBar.SetPosition(0, 0, 79, 0)
	menuBar.Items[0].SubItems = []vtui.MenuItem{
		{Text: "First"},
		{Text: "Second"},
	}
	menuBar.Items[1].SubItems = []vtui.MenuItem{
		{Text: "Other first"},
		{Text: "Other second"},
	}
	vtui.FrameManager.MenuBar = menuBar
	menuBar.Active = true
	menuBar.ActivateSubMenu(0)

	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	firstFrame := frames[len(frames)-1]
	firstMenu, _ := appFrameVMenu(firstFrame)
	if firstMenu == nil {
		t.Fatal("first menu-bar submenu was not materialized")
	}
	firstID := vtui.SemanticID(firstFrame)
	if !HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemSelect",
		"target":    firstID,
		"menuIndex": 0,
		"index":     1,
	}) {
		t.Fatal("current popup hover was rejected")
	}
	if firstMenu.SelectPos != 1 {
		t.Fatalf("first submenu selection = %d, want 1", firstMenu.SelectPos)
	}

	menuBar.ActivateSubMenu(1)
	frames = vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	secondFrame := frames[len(frames)-1]
	secondMenu, _ := appFrameVMenu(secondFrame)
	if secondMenu == nil {
		t.Fatal("second menu-bar submenu was not materialized")
	}
	if HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemSelect",
		"target":    firstID,
		"menuIndex": 0,
		"index":     0,
	}) {
		t.Fatal("stale hover action unexpectedly changed the active submenu")
	}
	if menuBar.SelectPos != 1 || secondMenu.SelectPos != 0 {
		t.Fatalf("stale hover mutated active menu: bar=%d row=%d",
			menuBar.SelectPos, secondMenu.SelectPos)
	}
	if HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemSelect",
		"target":    firstID,
		"menuIndex": 1,
		"index":     1,
	}) {
		t.Fatal("hover with a replaced popup id was accepted")
	}
}

func TestFileSystemPanelSemanticPanelNode(t *testing.T) {
	tmp := t.TempDir()
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		currentTitle:  "Friendly panel path",
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		sortMode:      SortSize,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true, Mode: "drwxr-xr-x"}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 1234, MTime: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), Mode: "-rw-r--r--"}, Selected: true},
		},
	}
	fp.SetCanFocus(true)
	fp.SetPosition(0, 0, 39, 9)
	fp.table.Columns = []vtui.TableColumn{
		{Title: "Name", Width: 27},
		{Title: "Size ↓", Width: 10},
	}
	fp.SetCursorIndex(1)

	model := fp.semanticPanelModel(&vtui.SemanticContext{Width: 80, Height: 25}, 0, true)
	node := model.ToMap()

	if node["kind"] != "filePanel" {
		t.Fatalf("kind = %v, want filePanel", node["kind"])
	}
	if node["active"] != true || node["side"] != 0 {
		t.Fatalf("unexpected panel identity: active=%v side=%v", node["active"], node["side"])
	}
	if node["cursor"] != 1 {
		t.Fatalf("cursor = %v, want 1", node["cursor"])
	}
	if node["path"] != tmp {
		t.Fatalf("path = %v, want %s", node["path"], tmp)
	}
	if node["title"] != "Friendly panel path" {
		t.Fatalf("title = %v, want friendly panel title", node["title"])
	}
	if node["selectedCount"] != 1 {
		t.Fatalf("selectedCount = %v, want 1", node["selectedCount"])
	}
	entries := node["entries"].([]map[string]any)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[1]["name"] != "alpha.txt" || entries[1]["selected"] != true {
		t.Fatalf("unexpected entry snapshot: %#v", entries[1])
	}
	if node["presentation"] != "list" || node["sourceKind"] != "local" || node["previewCapable"] != true {
		t.Fatalf("unexpected gallery capability metadata: %#v", node)
	}
	if node["catalogRevision"] != int64(1) || node["selectionRevision"] != int64(1) {
		t.Fatalf("unexpected initial revisions: catalog=%v selection=%v", node["catalogRevision"], node["selectionRevision"])
	}
	if node["cursorEntryId"] != entries[1]["entryId"] || entries[1]["entryId"] == "" {
		t.Fatalf("cursor identity was not exported: panel=%#v entry=%#v", node["cursorEntryId"], entries[1]["entryId"])
	}
	columns := node["columns"].([]map[string]any)
	if len(columns) != 2 || columns[0]["title"] != "Name" || columns[0]["sortMode"] != "name" || columns[0]["sortable"] != true {
		t.Fatalf("unexpected name column metadata: %#v", columns)
	}
	if columns[1]["title"] != "Size ↓" || columns[1]["sortMode"] != "size" || columns[1]["width"] != 10 {
		t.Fatalf("unexpected size column metadata: %#v", columns[1])
	}
	if entries[1]["localPath"] != filepath.Join(tmp, "alpha.txt") {
		t.Fatalf("localPath = %v, want %s", entries[1]["localPath"], filepath.Join(tmp, "alpha.txt"))
	}
	wantNanos := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC).UnixNano()
	if entries[1]["mtimeNanos"] != wantNanos || entries[1]["version"] != fmt.Sprintf("%d:1234", wantNanos) {
		t.Fatalf("unexpected file version metadata: %#v", entries[1])
	}
}

func TestSemanticPanelKeepsVirtualDottedNamesExtensionless(t *testing.T) {
	previous := AppConfig.SeparateFileExtensions
	AppConfig.SeparateFileExtensions = true
	defer func() { AppConfig.SeparateFileExtensions = previous }()

	fp := &FileSystemPanel{
		vfs:           vfs.NewNullVFS(0),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "iOS"),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "com.example.files", IsExecutable: true, NoExtension: true}},
			{VFSItem: vfs.VFSItem{Name: "notes.txt"}},
		},
	}

	model := fp.semanticPanelModel(nil, 0, true)
	if got := model.Entries[0]; got.DisplayBaseName != "com.example.files" || got.DisplayExtension != "" {
		t.Fatalf("virtual dotted name was split as an extension: %+v", got)
	}
	if got := model.Entries[1]; got.DisplayBaseName != "notes" || got.DisplayExtension != "txt" {
		t.Fatalf("ordinary file extension was not split: %+v", got)
	}
}

func TestFileSystemPanelExportsDeduplicatedHighlightStyles(t *testing.T) {
	previous := GlobalFileHighlighter
	GlobalFileHighlighter = &FileHighlighter{}
	defer func() { GlobalFileHighlighter = previous }()
	GlobalFileHighlighter.LoadFromIni(ParseIni(strings.NewReader(`
[Highlight_0]
Name = Text
Mask = *.txt
Icon = qrc:/F4QtHost/icons/lucide/file-text.svg
NormalColor = foreground:#123456
`)))

	tmp := t.TempDir()
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "one.txt"}},
			{VFSItem: vfs.VFSItem{Name: "two.txt"}},
		},
	}
	model := fp.semanticPanelModel(nil, 0, true)
	if model.HighlightRevision == 0 || len(model.HighlightStyles) != 1 {
		t.Fatalf("unexpected highlight metadata: revision=%d styles=%#v",
			model.HighlightRevision, model.HighlightStyles)
	}
	if model.Entries[0].HighlightStyleID == "" ||
		model.Entries[0].HighlightStyleID != model.Entries[1].HighlightStyleID {
		t.Fatalf("styles were not deduplicated: %#v", model.Entries)
	}
	style := model.HighlightStyles[model.Entries[0].HighlightStyleID]
	if style.Icon != "qrc:/F4QtHost/icons/lucide/file-text.svg" ||
		style.Normal.Foreground != "#123456" {
		t.Fatalf("unexpected normalized style: %#v", style)
	}
}

func TestFileSystemPanelSemanticRevisionsAndStableIdentity(t *testing.T) {
	previousSeparateFileExtensions := AppConfig.SeparateFileExtensions
	AppConfig.SeparateFileExtensions = false
	defer func() { AppConfig.SeparateFileExtensions = previousSeparateFileExtensions }()

	tmp := t.TempDir()
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		presentation:  PanelPresentationGallery,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "a.jpg", Size: 10, MTime: time.Unix(10, 20)}},
			{VFSItem: vfs.VFSItem{Name: "b.jpg", Size: 20, MTime: time.Unix(30, 40)}},
		},
	}

	first := fp.semanticPanelModel(nil, 0, true)
	second := fp.semanticPanelModel(nil, 0, true)
	if second.CatalogRevision != first.CatalogRevision || second.SelectionRevision != first.SelectionRevision {
		t.Fatalf("unchanged snapshot advanced revisions: first=%+v second=%+v", first, second)
	}
	aID := first.Entries[0].EntryID

	fp.SetItemSelected(0, true)
	selected := fp.semanticPanelModel(nil, 0, true)
	if selected.CatalogRevision != first.CatalogRevision || selected.SelectionRevision != first.SelectionRevision+1 {
		t.Fatalf("selection changed wrong revisions: first=%+v selected=%+v", first, selected)
	}

	fp.entries[0].MTime = fp.entries[0].MTime.Add(time.Nanosecond)
	changed := fp.semanticPanelModel(nil, 0, true)
	if changed.CatalogRevision != selected.CatalogRevision+1 {
		t.Fatalf("catalog metadata change did not advance revision: selected=%d changed=%d", selected.CatalogRevision, changed.CatalogRevision)
	}
	if changed.Entries[0].EntryID != aID {
		t.Fatalf("content metadata changed stable ID: got %q want %q", changed.Entries[0].EntryID, aID)
	}

	fp.entries[0], fp.entries[1] = fp.entries[1], fp.entries[0]
	reordered := fp.semanticPanelModel(nil, 0, true)
	if reordered.CatalogRevision != changed.CatalogRevision+1 || reordered.Entries[1].EntryID != aID {
		t.Fatalf("reorder revision/identity mismatch: %+v", reordered)
	}
	if reordered.SelectionRevision != changed.SelectionRevision {
		t.Fatalf("catalog reorder advanced selection revision: changed=%d reordered=%d", changed.SelectionRevision, reordered.SelectionRevision)
	}

	AppConfig.SeparateFileExtensions = true
	separated := fp.semanticPanelModel(nil, 0, true)
	if separated.CatalogRevision != reordered.CatalogRevision+1 ||
		separated.SelectionRevision != reordered.SelectionRevision {
		t.Fatalf("extension alignment changed wrong revisions: reordered=%+v separated=%+v", reordered, separated)
	}
	if separated.Entries[1].DisplayBaseName != "a" ||
		separated.Entries[1].DisplayExtension != "jpg" {
		t.Fatalf("extension alignment did not refresh Gallery fields: %+v", separated.Entries[1])
	}
}

func TestFileSystemPanelSemanticVFSFallbackKeepsGalleryPreference(t *testing.T) {
	remote := vfs.NewNullVFS(0)
	fp := &FileSystemPanel{
		vfs:           remote,
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "/"),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		presentation:  PanelPresentationGallery,
		selectedItems: make(map[string]bool),
		entries:       []*fileEntry{{VFSItem: vfs.VFSItem{Name: "remote.jpg", Size: 1}}},
	}
	model := fp.semanticPanelModel(nil, 0, true)
	if model.Presentation != "gallery" || model.SourceKind != "vfs" || model.PreviewCapable {
		t.Fatalf("unexpected remote capability model: %+v", model)
	}
	if model.Entries[0].LocalPath != "" || model.Entries[0].EntryID == "" {
		t.Fatalf("unexpected remote entry metadata: %+v", model.Entries[0])
	}
}

func TestPanelsFrameSemanticActionAcceptsQMLNumbers(t *testing.T) {
	tmp := t.TempDir()
	left := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 12}},
			{VFSItem: vfs.VFSItem{Name: "beta.txt", Size: 34}},
		},
	}
	right := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(40, 0, 79, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(41, 1, 78, 6, nil),
		viewMode:      ViewModeDetailed,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "right.txt", Size: 56}},
		},
	}
	pf := &PanelsFrame{
		panels:    [2]Panel{left, right},
		activeIdx: 0,
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.cursor",
		"side":   float64(0),
		"index":  float64(2),
	}) {
		t.Fatal("panel cursor action was not handled")
	}
	if left.GetCursorIndex() != 2 {
		t.Fatalf("left cursor = %d, want 2", left.GetCursorIndex())
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.cursor",
		"side":   0,
		"index":  99,
	}) || left.GetCursorIndex() != 2 {
		t.Fatal("legacy out-of-range index action no longer clamps")
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action":   "panel.cursor",
		"side":     float64(1),
		"index":    float64(0),
		"activate": true,
	}) {
		t.Fatal("atomic cursor-and-activate action was not handled")
	}
	if pf.activeIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.activeIdx)
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.activate",
		"side":   float64(0),
	}) {
		t.Fatal("activate panel action was not handled")
	}
	if pf.activeIdx != 0 {
		t.Fatalf("activeIdx = %d, want 0", pf.activeIdx)
	}
}

func TestPanelsFrameSemanticPointerIntentsClearFastFind(t *testing.T) {
	tmp := t.TempDir()
	panel := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		presentation:  PanelPresentationGallery,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.jpg", Size: 12}},
		},
	}
	frame := &PanelsFrame{panels: [2]Panel{panel, panel}, activeIdx: 0}
	model := panel.semanticPanelModel(nil, 0, true)
	alphaID := model.Entries[1].EntryID

	setFastFind := func() {
		panel.fastFindMode = true
		panel.fastFindStr = "alpha"
	}
	assertCleared := func(action string) {
		t.Helper()
		if panel.fastFindMode || panel.fastFindStr != "" {
			t.Fatalf("%s left fast-find active: mode=%v text=%q",
				action, panel.fastFindMode, panel.fastFindStr)
		}
	}

	setFastFind()
	if !frame.HandleSemanticAction(map[string]any{
		"action": "panel.activate",
		"side":   0,
	}) {
		t.Fatal("panel activate action was not handled")
	}
	assertCleared("panel.activate")

	setFastFind()
	if !frame.HandleSemanticAction(map[string]any{
		"action":          "panel.cursor",
		"side":            0,
		"entryId":         alphaID,
		"catalogRevision": model.CatalogRevision,
	}) {
		t.Fatal("panel cursor action was not handled")
	}
	assertCleared("panel.cursor")

	setFastFind()
	if !frame.HandleSemanticAction(map[string]any{
		"action":          "panel.setSelection",
		"side":            0,
		"mode":            "toggle",
		"entryIds":        []string{alphaID},
		"catalogRevision": model.CatalogRevision,
	}) {
		t.Fatal("panel selection action was not handled")
	}
	assertCleared("panel.setSelection")
}

func TestPanelsFrameSemanticGalleryActionsUseStableIDsAndRevisions(t *testing.T) {
	tmp := t.TempDir()
	left := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		presentation:  PanelPresentationList,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.jpg", Size: 12}},
			{VFSItem: vfs.VFSItem{Name: "beta.jpg", Size: 34}},
		},
	}
	pf := &PanelsFrame{panels: [2]Panel{left, left}, activeIdx: 0}
	model := left.semanticPanelModel(nil, 0, true)
	alphaID := model.Entries[1].EntryID
	betaID := model.Entries[2].EntryID

	if !pf.HandleSemanticAction(map[string]any{
		"action":          "panel.cursor",
		"side":            float64(0),
		"entryId":         betaID,
		"catalogRevision": float64(model.CatalogRevision),
	}) || left.GetCursorIndex() != 2 {
		t.Fatal("stable-ID cursor action was not applied")
	}
	if pf.HandleSemanticAction(map[string]any{
		"action":          "panel.cursor",
		"side":            0,
		"entryId":         alphaID,
		"catalogRevision": model.CatalogRevision - 1,
	}) {
		t.Fatal("stale catalog action was accepted")
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action":            "panel.setSelection",
		"side":              0,
		"mode":              "replace",
		"entryIds":          []any{alphaID, betaID},
		"catalogRevision":   model.CatalogRevision,
		"selectionRevision": model.SelectionRevision,
	}) || !left.entries[1].Selected || !left.entries[2].Selected {
		t.Fatal("stable-ID replacement selection was not applied")
	}
	selected := left.semanticPanelModel(nil, 0, true)
	if pf.HandleSemanticAction(map[string]any{
		"action":            "panel.toggleSelection",
		"side":              0,
		"entryId":           alphaID,
		"catalogRevision":   selected.CatalogRevision,
		"selectionRevision": model.SelectionRevision,
	}) {
		t.Fatal("stale selection toggle was accepted")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":            "panel.setSelection",
		"side":              0,
		"mode":              "remove",
		"indices":           []any{float64(2)},
		"catalogRevision":   selected.CatalogRevision,
		"selectionRevision": selected.SelectionRevision,
	}) || left.entries[2].Selected {
		t.Fatal("batch index compatibility selection was not applied")
	}

	beforeAlpha := left.entries[1].Selected
	if pf.HandleSemanticAction(map[string]any{
		"action":   "panel.setSelection",
		"side":     0,
		"mode":     "replace",
		"entryIds": []string{alphaID, "missing"},
	}) || left.entries[1].Selected != beforeAlpha {
		t.Fatal("invalid stable-ID batch was not rejected atomically")
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action":       "panel.setPresentation",
		"side":         0,
		"presentation": "gallery",
	}) || left.presentation != PanelPresentationGallery {
		t.Fatal("gallery presentation action was not applied")
	}
	if pf.HandleSemanticAction(map[string]any{
		"action":       "panel.setPresentation",
		"side":         0,
		"presentation": "unknown",
	}) {
		t.Fatal("invalid presentation was accepted")
	}
}

func TestPanelsFrameSemanticGalleryLayoutActions(t *testing.T) {
	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	pf := &PanelsFrame{panels: [2]Panel{left, right}, activeIdx: 0}
	left.SetViewMode(ViewModeBrief)

	initial := left.semanticPanelModel(nil, 0, true)
	if initial.GalleryLayoutMode != "masonry" ||
		initial.GalleryColumnCount != 2 || initial.GalleryDensity != 150 ||
		initial.GalleryLayoutRevision < 1 {
		t.Fatalf("unexpected semantic gallery defaults: %#v", initial)
	}
	if len(initial.Columns) != 3 || len(initial.GalleryColumns) != 2 ||
		initial.GalleryColumns[0].ID != "name" ||
		initial.GalleryColumns[0].Role != "name" ||
		initial.GalleryColumns[1].ID != "size" ||
		initial.GalleryColumns[1].Role != "size" ||
		initial.GalleryColumns[1].Alignment != "right" {
		t.Fatalf("Gallery Details schema followed legacy Brief columns: legacy=%#v gallery=%#v",
			initial.Columns, initial.GalleryColumns)
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action":      "panel.setGalleryLayout",
		"side":        0,
		"layoutMode":  "columns",
		"columnCount": 3,
	}) {
		t.Fatal("Columns layout action was rejected")
	}
	columns := left.semanticPanelModel(nil, 0, true)
	if columns.Presentation != "gallery" ||
		columns.GalleryLayoutMode != "columns" ||
		columns.GalleryColumnCount != 3 ||
		columns.GalleryLayoutRevision <= initial.GalleryLayoutRevision {
		t.Fatalf("layout action was not reflected in semantic scene: %#v", columns)
	}

	if pf.HandleSemanticAction(map[string]any{
		"action":      "panel.setGalleryLayout",
		"side":        0,
		"layoutMode":  "columns",
		"columnCount": 7,
	}) {
		t.Fatal("invalid column count was accepted")
	}

	left.SetPresentation(PanelPresentationList)
	beforeDensityRevision := left.galleryLayoutRevision
	if !pf.HandleSemanticAction(map[string]any{
		"action":            "panel.setGalleryDensity",
		"side":              0,
		"galleryLayoutMode": "icons",
		"density":           999,
	}) {
		t.Fatal("Icons density action was rejected")
	}
	if left.presentation != PanelPresentationList {
		t.Fatal("density-only action changed presentation")
	}
	if left.galleryDensity(GalleryLayoutIcons) != 256 ||
		left.galleryLayoutRevision != beforeDensityRevision+1 {
		t.Fatalf("density action did not clamp/revise state: density=%d revision=%d",
			left.galleryDensity(GalleryLayoutIcons), left.galleryLayoutRevision)
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action":   "panel.setViewMode",
		"side":     0,
		"viewMode": "detailed",
	}) || left.viewMode != ViewModeDetailed ||
		left.presentation != PanelPresentationList {
		t.Fatal("legacy Detailed action did not retain list semantics")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":   "panel.setViewMode",
		"side":     1,
		"viewMode": "wide",
	}) || !pf.wide || pf.widePanel != 1 {
		t.Fatal("legacy Wide action did not select the requested panel")
	}
}

func TestPanelsFrameSemanticPointerPreservesGoMouseSemantics(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	fp.SetViewMode(ViewModeDetailed)
	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "first.txt"}},
		{VFSItem: vfs.VFSItem{Name: "second.txt"}},
	}
	fp.Refresh()
	pf := &PanelsFrame{panels: [2]Panel{fp, fp}, activeIdx: 0}
	model := fp.semanticPanelModel(nil, 0, true)

	pointer := func(index int, button, phase string) bool {
		return pf.HandleSemanticAction(map[string]any{
			"action":          "panel.pointer",
			"side":            0,
			"button":          button,
			"phase":           phase,
			"entryId":         model.Entries[index].EntryID,
			"index":           index,
			"catalogRevision": model.CatalogRevision,
		})
	}

	if !pointer(2, "left", "down") || fp.GetCursorIndex() != 2 {
		t.Fatal("left mouse-down did not move the authoritative Go cursor")
	}
	if !pointer(2, "left", "click") {
		t.Fatal("single-click phase was not accepted")
	}
	if !pointer(1, "right", "down") || !fp.entries[1].Selected {
		t.Fatal("right mouse-down did not toggle selection")
	}
	if !pointer(1, "right", "up") {
		t.Fatal("right mouse-up was not accepted")
	}
	// Qt reports a normal second press before its doubleClicked signal. Go
	// must retain the first click's operation and spread that state, matching
	// the terminal backend's native DoubleClick event.
	if !pointer(1, "right", "down") || fp.entries[1].Selected {
		t.Fatal("second right mouse-down did not toggle the row")
	}
	if !pointer(1, "right", "doubleClick") {
		t.Fatal("right double-click was not accepted")
	}
	if !fp.entries[1].Selected || !fp.entries[2].Selected || fp.entries[0].Selected {
		t.Fatalf("right double-click did not spread the first click state: %#v", fp.entries)
	}
	if !pointer(1, "right", "up") {
		t.Fatal("final right mouse-up was not accepted")
	}
}

func TestPanelsFrameSemanticOpenResolvesStableIDAfterStaleCursorRejection(t *testing.T) {
	root := t.TempDir()
	decoyPath := filepath.Join(root, "decoy")
	targetPath := filepath.Join(root, "authoritative")
	if err := os.Mkdir(decoyPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}

	localVFS := &semanticBlockingLocalVFS{
		OSVFS:       vfs.NewOSVFS(root),
		readStarted: make(chan struct{}),
		readDone:    make(chan struct{}),
	}
	panel := &FileSystemPanel{
		vfs:           localVFS,
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, root),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		presentation:  PanelPresentationGallery,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "decoy", IsDir: true, Size: 1}},
			{VFSItem: vfs.VFSItem{Name: "authoritative", IsDir: true, Size: 1}},
		},
	}
	panel.SetCursorIndex(1)
	frame := &PanelsFrame{
		panels:     [2]Panel{panel, panel},
		activeIdx:  1,
		showPanels: true,
		cmdLine:    NewCommandLine(">"),
	}

	stale := panel.semanticPanelModel(nil, 0, false)
	targetID := stale.Entries[2].EntryID
	panel.entries[1].Size++
	current := panel.semanticPanelModel(nil, 0, false)
	if current.CatalogRevision != stale.CatalogRevision+1 {
		t.Fatalf("catalog revision did not advance: stale=%d current=%d",
			stale.CatalogRevision, current.CatalogRevision)
	}

	if frame.HandleSemanticAction(map[string]any{
		"action":          "panel.cursor",
		"side":            0,
		"entryId":         targetID,
		"catalogRevision": stale.CatalogRevision,
	}) {
		t.Fatal("stale revisioned cursor action was accepted")
	}
	if panel.GetCursorIndex() != 1 {
		t.Fatalf("stale cursor action moved cursor to %d", panel.GetCursorIndex())
	}

	handled := frame.HandleSemanticAction(map[string]any{
		"action":  "panel.open",
		"side":    0,
		"entryId": targetID,
	})
	select {
	case <-localVFS.readStarted:
	case <-time.After(time.Second):
		t.Fatal("opening the authoritative directory did not start its catalog load")
	}
	if panel.cancelLoad != nil {
		panel.cancelLoad()
	}
	if panel.loadingTimer != nil {
		panel.loadingTimer.Stop()
	}
	select {
	case <-localVFS.readDone:
	case <-time.After(time.Second):
		t.Fatal("authoritative directory catalog load did not stop after cancellation")
	}

	if !handled {
		t.Fatal("unrevisioned stable-ID open was rejected")
	}
	if frame.activeIdx != 0 {
		t.Fatalf("open did not activate its panel: active=%d", frame.activeIdx)
	}
	if got := panel.vfs.GetPath(); got != targetPath {
		t.Fatalf("open resolved the wrong entry: got %q want %q", got, targetPath)
	}
}

func TestPanelsFrameSemanticRapidSelectionActionsRemainOrdered(t *testing.T) {
	tmp := t.TempDir()
	panel := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		presentation:  PanelPresentationGallery,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "first.jpg", Size: 10}},
			{VFSItem: vfs.VFSItem{Name: "second.jpg", Size: 20}},
		},
	}
	frame := &PanelsFrame{panels: [2]Panel{panel, panel}}
	model := panel.semanticPanelModel(nil, 0, true)

	if !frame.HandleSemanticAction(map[string]any{
		"action":            "panel.setSelection",
		"side":              0,
		"mode":              "toggle",
		"entryIds":          []string{model.Entries[0].EntryID},
		"catalogRevision":   model.CatalogRevision,
		"selectionRevision": model.SelectionRevision,
	}) {
		t.Fatal("first revisioned selection action was rejected")
	}
	// The bridge deliberately omits the optional selection revision on later
	// actions already ordered behind the first on the same IPC stream.
	if !frame.HandleSemanticAction(map[string]any{
		"action":          "panel.setSelection",
		"side":            0,
		"mode":            "toggle",
		"entryIds":        []string{model.Entries[1].EntryID},
		"catalogRevision": model.CatalogRevision,
	}) {
		t.Fatal("second ordered selection action was rejected")
	}
	if !panel.entries[0].Selected || !panel.entries[1].Selected {
		t.Fatalf("rapid ordered actions lost a selection: first=%v second=%v",
			panel.entries[0].Selected, panel.entries[1].Selected)
	}
}

func TestPanelsFrameSemanticGridFallbackForUpstreamPanelLayouts(t *testing.T) {
	pf := &PanelsFrame{showPanels: true, showLeftPanel: true, showRightPanel: true}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("default panel layout unexpectedly requires fallback: %q", reason)
	}

	pf.widthDecrement = 1
	if reason := pf.semanticGridFallbackReason(); !strings.Contains(reason, "resized-panel") {
		t.Fatalf("resized panel fallback reason = %q", reason)
	}
	pf.widthDecrement = 0
	pf.showRightPanel = false
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("hidden panel layout unexpectedly requires fallback: %q", reason)
	}
	pf.showRightPanel = true
	pf.altPanels[0] = &InfoPanel{}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("info panel unexpectedly requires fallback: %q", reason)
	}
	pf.altPanels[0] = &QuickViewPanel{}
	if reason := pf.semanticGridFallbackReason(); !strings.Contains(reason, "quick_view") {
		t.Fatalf("quick-view fallback reason = %q", reason)
	}
}

func TestSemantic_EditorViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	pt := piecetable.New([]byte("hello"))
	ev := NewEditorView(pt, nil, "test.txt")
	ev.modified = false
	ev.CursorPos = ev.getLineLength(0)

	// 1. Test insertText
	actionInsert := map[string]any{
		"target": vtui.SemanticID(ev),
		"action": "editor.insertText",
		"text":   " world",
	}
	if !ev.HandleSemanticAction(actionInsert) {
		t.Fatal("editor insert action was not handled")
	}
	if ev.GetText() != "hello world" {
		t.Errorf("expected 'hello world', got %q", ev.GetText())
	}
	if !ev.modified {
		t.Error("editor should be marked as modified after insertion")
	}

	// 2. Test Undo
	actionUndo := map[string]any{
		"target": vtui.SemanticID(ev),
		"action": "editor.undo",
	}
	if !ev.HandleSemanticAction(actionUndo) {
		t.Fatal("editor undo action was not handled")
	}
	if ev.GetText() != "hello" {
		t.Errorf("expected 'hello' after undo, got %q", ev.GetText())
	}
}

func TestSemantic_ViewerViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "view.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	v := vfs.NewOSVFS(tmp)
	viewer, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("failed to create viewer: %v", err)
	}

	// Test scroll action
	actionScroll := map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scroll",
		"offset": float64(6), // Starts 'line2'
	}
	if !viewer.HandleSemanticAction(actionScroll) {
		t.Fatal("viewer scroll action was not handled")
	}
	if viewer.TopOffset != 6 {
		t.Errorf("expected TopOffset 6, got %d", viewer.TopOffset)
	}
}

func TestSemanticRenderedSurfacePreservesColorsAndCursor(t *testing.T) {
	foreground := uint32(0x12abef)
	background := uint32(0x230f41)
	attr := vtui.SetRGBBoth(vtui.ForegroundIntensity|vtui.CommonLvbUnderscore,
		foreground, background)
	rendered := semanticRenderSurface(2, 3, 5, 4, func(scr *vtui.ScreenBuf) {
		scr.FillRect(2, 3, 5, 4, ' ', attr)
		scr.Write(2, 3, vtui.StringToCharInfo("test", attr))
		scr.SetCursorPos(4, 3)
		scr.SetCursorVisible(true)
		scr.SetCursorShape(vtui.CursorShapeBlock)
	})

	if len(rendered.Rows) != 2 || len(rendered.Rows[0]) != 1 {
		t.Fatalf("rendered rows = %#v", rendered.Rows)
	}
	run := rendered.Rows[0][0]
	if run.Foreground != "#12abef" || run.Background != "#230f41" {
		t.Fatalf("run colors = foreground %q background %q", run.Foreground, run.Background)
	}
	if !run.Bold || !run.Underline {
		t.Fatalf("run styles were lost: %#v", run)
	}
	if !rendered.CursorVisible || rendered.CursorX != 2 || rendered.CursorY != 0 || rendered.CursorShape != "block" {
		t.Fatalf("cursor = %#v", rendered)
	}
}

func TestSemanticAttrColorHonorsReverse(t *testing.T) {
	attr := vtui.SetRGBBoth(vtui.CommonLvbReverse, 0x102030, 0xa0b0c0)
	if got := semanticAttrColor(attr, true); got != "#a0b0c0" {
		t.Fatalf("reversed foreground = %q", got)
	}
	if got := semanticAttrColor(attr, false); got != "#102030" {
		t.Fatalf("reversed background = %q", got)
	}
}

func TestEditorSemanticSurfaceUsesHighlighterAndExportsCursor(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	pt := piecetable.New([]byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"))
	ev := NewEditorView(pt, nil, "main.go")
	defer ev.Close()
	ev.highlighter = (&chroma.ChromaProvider{}).Create("main.go", "")
	ev.SetPosition(0, 0, 79, 23)
	ev.SetVisible(true)
	ev.SetFocus(true)
	ev.CursorLine = 2
	ev.CursorPos = 5
	ev.ensureCursorVisible()

	rendered := semanticRenderSurface(ev.X1, ev.Y1+1, ev.X2-1, ev.Y2, ev.DisplayObject)
	if !rendered.CursorVisible {
		t.Fatalf("editor cursor was not exported: %#v", rendered)
	}
	colors := make(map[string]bool)
	for _, row := range rendered.Rows {
		for _, run := range row {
			if strings.TrimSpace(run.Text) != "" {
				colors[run.Foreground] = true
			}
		}
	}
	if len(colors) < 2 {
		t.Fatalf("syntax-colored editor surface has only %d foreground color(s): %#v", len(colors), colors)
	}
}

func TestEditorMenuBarSemanticClickOpensSubmenu(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	ev := NewEditorView(piecetable.New([]byte("package main\n")), nil, "main.go")
	defer ev.Close()
	ev.SetPosition(0, 1, 79, 23)
	ev.SetVisible(true)
	vtui.FrameManager.Push(ev)

	if !HandleSemanticAction(map[string]any{
		"action": "menuBar.toggle",
		"index":  0,
	}) {
		t.Fatal("editor menu-bar click was not handled")
	}
	if !ev.menuBar.Active || ev.menuBar.SelectPos != 0 {
		t.Fatalf("editor menu bar was not activated: active=%v selected=%d",
			ev.menuBar.Active, ev.menuBar.SelectPos)
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	if len(frames) < 2 {
		t.Fatalf("editor submenu was not pushed: %#v", frames)
	}
	if menu, _ := appFrameVMenu(frames[len(frames)-1]); menu == nil {
		t.Fatalf("top frame is not the editor submenu: %T", frames[len(frames)-1])
	}

	if !HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemActivate",
		"menuIndex": 0,
		// The upstream action registry inserts Switch to Viewer between Save
		// and Quit; semantic menu indices follow that generated order.
		"index": 2,
	}) {
		t.Fatal("editor Exit menu item was not activated")
	}
	frames = vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for _, frame := range frames {
		if frame == ev {
			t.Fatal("editor remained open after activating File > Exit")
		}
	}
}

func TestCommandLineSemanticModelUsesRenderedRunsAndCursor(t *testing.T) {
	cl := NewCommandLine(">")
	cl.SetPosition(0, 0, 19, 0)
	cl.InsertString("abc")

	model := cl.semanticModel(nil)
	if len(model.Runs) == 0 {
		t.Fatal("command line did not export its rendered color runs")
	}
	if !model.CursorVisible {
		t.Fatal("focused command line did not export a visible cursor")
	}
	if len(model.CursorPrefixRuns) == 0 {
		t.Fatal("command line did not export the rendered prefix used to place its cursor")
	}
	if model.CursorX != 4 {
		t.Fatalf("command cursor x=%d, want prompt width 1 + text width 3", model.CursorX)
	}
	if model.InputX != 1 {
		t.Fatalf("command input x=%d, want prompt width 1", model.InputX)
	}
}

func TestCommandCompletePreservesTextWithoutExplicitSelection(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := &PanelsFrame{cmdLine: NewCommandLine(">")}
	pf.cmdLine.Edit.SetText("git st")
	if !pf.HandleSemanticAction(map[string]any{"action": "command.complete"}) {
		t.Fatal("command.complete was not handled")
	}
	if got := pf.cmdLine.Edit.GetText(); got != "git st" {
		t.Fatalf("untouched completion changed command to %q", got)
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "command.complete",
		"text":   "git status",
	}) {
		t.Fatal("explicit command.complete was not handled")
	}
	if got := pf.cmdLine.Edit.GetText(); got != "git status" {
		t.Fatalf("explicit completion produced %q", got)
	}
}
