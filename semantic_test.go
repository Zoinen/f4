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
	"github.com/unxed/vtinput"
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

func TestSemanticPanelTitleExcludesTUILoadingPulse(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fp := NewFileSystemPanel(0, 0, 40, 10, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fp)

	fp.isLoading = true
	fp.loadingFrame = 3
	fp.updateTitle(nil)
	if !strings.HasSuffix(fp.currentTitle, panelLoadingPulse[3]) {
		t.Fatalf("TUI title %q has no loading pulse", fp.currentTitle)
	}
	model := fp.semanticPanelModel(nil, 0, true)
	if model.Title != fp.vfs.GetPath() {
		t.Fatalf("semantic title = %q, want clean path %q", model.Title, fp.vfs.GetPath())
	}
	if !model.Loading {
		t.Fatal("semantic loading state was lost with the presentation-neutral title")
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
	if node["sourceKind"] != "local" || node["previewCapable"] != true {
		t.Fatalf("unexpected gallery capability metadata: %#v", node)
	}
	for _, retired := range []string{"presentation", "viewModeName", "columns", "top"} {
		if _, present := node[retired]; present {
			t.Fatalf("retired panel field %q was exported: %#v", retired, node)
		}
	}
	if node["catalogRevision"] != int64(1) || node["selectionRevision"] != int64(1) {
		t.Fatalf("unexpected initial revisions: catalog=%v selection=%v", node["catalogRevision"], node["selectionRevision"])
	}
	if node["cursorEntryId"] != entries[1]["entryId"] || entries[1]["entryId"] == "" {
		t.Fatalf("cursor identity was not exported: panel=%#v entry=%#v", node["cursorEntryId"], entries[1]["entryId"])
	}
	columns := node["galleryColumns"].([]map[string]any)
	if len(columns) != 2 || columns[0]["role"] != "name" || columns[0]["sortMode"] != "name" || columns[0]["sortable"] != true {
		t.Fatalf("unexpected unified name column metadata: %#v", columns)
	}
	if columns[1]["role"] != "size" || columns[1]["sortMode"] != "size" || columns[1]["alignment"] != "right" {
		t.Fatalf("unexpected unified size column metadata: %#v", columns[1])
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
		selectedItems: make(map[string]bool),
		entries:       []*fileEntry{{VFSItem: vfs.VFSItem{Name: "remote.jpg", Size: 1}}},
	}
	model := fp.semanticPanelModel(nil, 0, true)
	if model.SourceKind != "vfs" || model.PreviewCapable {
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
	if len(initial.GalleryColumns) != 2 ||
		initial.GalleryColumns[0].ID != "name" ||
		initial.GalleryColumns[0].Role != "name" ||
		initial.GalleryColumns[1].ID != "size" ||
		initial.GalleryColumns[1].Role != "size" ||
		initial.GalleryColumns[1].Alignment != "right" {
		t.Fatalf("unexpected unified Details schema: %#v", initial.GalleryColumns)
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
	if columns.GalleryLayoutMode != "columns" ||
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

	beforeDensityLayout := left.galleryLayoutMode
	beforeDensityRevision := left.galleryLayoutRevision
	if !pf.HandleSemanticAction(map[string]any{
		"action":            "panel.setGalleryDensity",
		"side":              0,
		"galleryLayoutMode": "icons",
		"density":           999,
	}) {
		t.Fatal("Icons density action was rejected")
	}
	if left.galleryLayoutMode != beforeDensityLayout {
		t.Fatal("density-only action changed the active layout")
	}
	if left.galleryDensity(GalleryLayoutIcons) != 256 ||
		left.galleryLayoutRevision != beforeDensityRevision+1 {
		t.Fatalf("density action did not clamp/revise state: density=%d revision=%d",
			left.galleryDensity(GalleryLayoutIcons), left.galleryLayoutRevision)
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":     "panel.resetGalleryDensity",
		"side":       0,
		"layoutMode": "icons",
	}) || left.galleryDensity(GalleryLayoutIcons) != 64 {
		t.Fatalf("density reset did not restore the mode default: %d",
			left.galleryDensity(GalleryLayoutIcons))
	}

	// The long-standing TUI commands also select the corresponding strategy of
	// the unified native renderer.
	pf.setPanelViewMode(0, ViewModeDetailed)
	if left.viewMode != ViewModeDetailed ||
		left.galleryLayoutMode != GalleryLayoutDetails {
		t.Fatal("Detailed action did not select unified Details")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":  "panel.setWide",
		"side":    0,
		"enabled": true,
	}) || !pf.wide || pf.widePanel != 0 {
		t.Fatal("independent Wide toggle did not expand the requested panel")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":      "panel.setGalleryLayout",
		"side":        0,
		"layoutMode":  "icons",
		"columnCount": 0,
	}) || !pf.wide || pf.widePanel != 0 {
		t.Fatal("unified renderer selection disturbed independent Wide layout")
	}
	pf.setPanelViewMode(0, ViewModeBrief)
	if !pf.wide || pf.widePanel != 0 ||
		left.galleryLayoutMode != GalleryLayoutColumns ||
		left.galleryColumnCount != 3 {
		t.Fatal("Columns 3 alias disturbed independent Wide layout")
	}
	wideModel := left.semanticPanelModel(nil, 0, true)
	if wideModel.GalleryLayoutMode != "columns" ||
		wideModel.GalleryColumnCount != 3 {
		t.Fatalf("Wide leaked into renderer identity: %#v", wideModel)
	}
	// This is the exact state transition owned by the Tab branch; the full
	// initialized PanelsFrame keyboard test below exercises its key routing.
	pf.activeIdx = 1 - pf.activeIdx
	pf.widePanel = pf.activeIdx
	if pf.activeIdx != 1 || pf.widePanel != 1 || !pf.wide {
		t.Fatal("Tab did not transfer independent Wide to the second panel")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":  "panel.setWide",
		"side":    1,
		"enabled": false,
	}) || pf.wide || pf.widePanel != -1 {
		t.Fatal("Wide toggle did not restore split layout")
	}
	pf.setWidePanel(1)
	if !pf.wide || pf.widePanel != 1 {
		t.Fatal("Wide command did not select the requested panel")
	}
}

func TestPanelsFrameSemanticPanelNavigatePath(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(root))
	rightRoot := t.TempDir()
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(rightRoot))
	pf := &PanelsFrame{panels: [2]Panel{left, right}, activeIdx: 1}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.navigatePath",
		"side":   0,
		"path":   child,
	}) {
		t.Fatal("path navigation action was rejected")
	}
	if got := left.vfs.GetPath(); got != child {
		t.Fatalf("left panel path = %q, want %q", got, child)
	}
	if got := right.vfs.GetPath(); got != rightRoot {
		t.Fatalf("navigation changed the wrong panel: %q", got)
	}
	if pf.activeIdx != 0 {
		t.Fatalf("navigated panel was not activated: %d", pf.activeIdx)
	}
	if left.pendingSelection != filepath.Base(root) {
		t.Fatalf("parent return selection = %q, want %q",
			left.pendingSelection, filepath.Base(root))
	}
	if pf.HandleSemanticAction(map[string]any{
		"action": "panel.navigatePath",
		"side":   0,
		"path":   "",
	}) {
		t.Fatal("empty path was accepted")
	}
}

func TestPanelViewModeCommandsAlsoSelectUnifiedLayouts(t *testing.T) {
	panel := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	if panel.cancelLoad != nil {
		defer panel.cancelLoad()
	}
	frame := &PanelsFrame{panels: [2]Panel{panel, panel}}

	for _, tc := range []struct {
		name    string
		command int
		mode    ViewMode
		layout  GalleryLayoutMode
		columns int
	}{
		{name: "columns 3", command: CmLeftBrief, mode: ViewModeBrief, layout: GalleryLayoutColumns, columns: 3},
		{name: "columns 2", command: CmLeftMedium, mode: ViewModeMedium, layout: GalleryLayoutColumns, columns: 2},
		{name: "details", command: CmLeftDetailed, mode: ViewModeDetailed, layout: GalleryLayoutDetails, columns: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !frame.HandleCommand(tc.command, nil) {
				t.Fatalf("command %d was not handled", tc.command)
			}
			if panel.viewMode != tc.mode ||
				panel.effectiveGalleryLayoutMode() != tc.layout ||
				panel.effectiveGalleryColumnCount() != tc.columns {
				t.Fatalf("mode command produced TUI=%v layout=%q columns=%d",
					panel.viewMode, panel.effectiveGalleryLayoutMode(),
					panel.effectiveGalleryColumnCount())
			}
		})
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
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("quick-view unexpectedly requires fallback: %q", reason)
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

func awaitSemanticViewerWindow(t *testing.T, viewer *ViewerView, minimumRows int) map[string]any {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		node := viewer.SemanticNode(nil)
		if len(appMapSlice(node["windowRows"])) >= minimumRows {
			return node
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timed out waiting for semantic viewer window: %#v", node)
		}
	}
}

func TestSemantic_ViewerWindowIsBoundedAndByteAddressed(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "window.txt")
	var content strings.Builder
	var offsets []int64
	for i := 0; i < 80; i++ {
		offsets = append(offsets, int64(content.Len()))
		fmt.Fprintf(&content, "line-%02d %s\n", i, strings.Repeat("x", i%7))
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 39, 8) // Eight document rows below the top bar.
	viewer.TopOffset = offsets[30]
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	node := awaitSemanticViewerWindow(t, viewer, 8)
	windowRows := appMapSlice(node["windowRows"])
	visibleRows := appMapSlice(node["rows"])
	viewportRows := semanticInt(node["viewportRows"])
	viewportRow := semanticInt(node["viewportRow"])
	if semanticString(node["scrollUnit"]) != "bytes" {
		t.Fatalf("scrollUnit = %q", node["scrollUnit"])
	}
	if viewportRows != 8 || len(visibleRows) != viewportRows {
		t.Fatalf("viewport rows=%d visible=%d", viewportRows, len(visibleRows))
	}
	if len(windowRows) <= viewportRows || len(windowRows) > viewportRows+2*semanticWindowBufferRows(viewportRows) {
		t.Fatalf("bounded window rows = %d for viewport %d", len(windowRows), viewportRows)
	}
	if viewportRow < 1 || viewportRow >= len(windowRows) {
		t.Fatalf("viewportRow = %d, window rows = %d", viewportRow, len(windowRows))
	}
	if got := appInt64(windowRows[viewportRow]["offset"]); got != viewer.TopOffset {
		t.Fatalf("viewport offset = %d, want %d", got, viewer.TopOffset)
	}
	for i := 0; i+1 < len(windowRows); i++ {
		end := appInt64(windowRows[i]["endOffset"])
		next := appInt64(windowRows[i+1]["offset"])
		if end != next {
			t.Fatalf("row %d end=%d, next=%d", i, end, next)
		}
	}
	if appInt64(node["windowStart"]) > viewer.TopOffset ||
		appInt64(node["windowEnd"]) < viewer.TopOffset+appInt64(node["viewportSpan"]) ||
		appInt64(node["contentExtent"]) != int64(content.Len()) ||
		node["contentExtentKnown"] != true {
		t.Fatalf("invalid viewer window contract: %#v", node)
	}

	beforeGeneration := viewer.semanticWindowGeneration
	viewer.TopOffset = 0
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scroll", "offset": int64(-1),
	}) {
		t.Fatal("clamped viewer scroll was not acknowledged")
	}
	if viewer.TopOffset != 0 || viewer.semanticWindowGeneration != beforeGeneration+1 {
		t.Fatalf("clamped scroll offset=%d generation=%d", viewer.TopOffset,
			viewer.semanticWindowGeneration)
	}
}

func TestSemantic_ViewerTenGiBHexWindowStaysSparseAndInt64Addressed(t *testing.T) {
	vtui.SetDefaultPalette()
	const fileSize int64 = 10 * 1024 * 1024 * 1024
	file := &largeBinaryFile{size: fileSize}
	base := vfs.NewOSVFS(t.TempDir())
	viewer, err := NewViewerView(context.Background(), &singleFileVFS{VFS: base, file: file}, "ten-gib.7z")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	if !viewer.HexMode {
		t.Fatal("10 GiB binary fixture did not open in hex mode")
	}

	viewer.SetPosition(0, 0, 79, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	const requestedOffset int64 = 7*1024*1024*1024 + 123
	wantTop := requestedOffset &^ int64(0xF)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": requestedOffset,
	}) {
		t.Fatal("far 64-bit viewer scroll was not handled")
	}
	if viewer.TopOffset != wantTop {
		t.Fatalf("far scroll top=%d, want 16-byte-aligned %d", viewer.TopOffset, wantTop)
	}

	// The first snapshot starts one asynchronous cache fill at the far window.
	// Completing that one task must be sufficient; a sequential scan of the
	// preceding seven GiB would either time out or produce additional reads.
	_ = viewer.SemanticNode(nil)
	deadline := time.After(2 * time.Second)
	for {
		viewer.backend.mu.Lock()
		fetching := viewer.backend.isFetching
		cacheOff := viewer.backend.cacheOff
		cacheLen := len(viewer.backend.cacheData)
		viewer.backend.mu.Unlock()
		if !fetching && cacheLen > 0 && cacheOff > 6*1024*1024*1024 {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timed out loading sparse far window: off=%d len=%d fetching=%v",
				cacheOff, cacheLen, fetching)
		}
	}

	node := viewer.SemanticNode(nil)
	viewportRows := semanticInt(node["viewportRows"])
	bufferRows := semanticWindowBufferRows(viewportRows)
	windowRows := appMapSlice(node["windowRows"])
	if got := appInt64(node["contentExtent"]); got != fileSize {
		t.Fatalf("content extent=%d, want exact 10 GiB=%d", got, fileSize)
	}
	if got := appInt64(node["size"]); got != fileSize {
		t.Fatalf("surface size=%d, want exact 10 GiB=%d", got, fileSize)
	}
	if node["contentExtentKnown"] != true || semanticString(node["scrollUnit"]) != "bytes" {
		t.Fatalf("invalid global scrollbar contract: known=%v unit=%q",
			node["contentExtentKnown"], node["scrollUnit"])
	}
	if got := appInt64(node["viewportStart"]); got != wantTop {
		t.Fatalf("viewport start=%d, want %d", got, wantTop)
	}
	if viewportRows != 8 {
		t.Fatalf("viewport rows=%d, want 8", viewportRows)
	}
	if len(windowRows) != viewportRows+2*bufferRows {
		t.Fatalf("bounded window rows=%d, want %d (viewport=%d buffer=%d)",
			len(windowRows), viewportRows+2*bufferRows, viewportRows, bufferRows)
	}
	wantWindowStart := wantTop - int64(bufferRows*16)
	wantWindowEnd := wantWindowStart + int64(len(windowRows)*16)
	if got := appInt64(node["windowStart"]); got != wantWindowStart {
		t.Fatalf("window start=%d, want %d", got, wantWindowStart)
	}
	if got := appInt64(node["windowEnd"]); got != wantWindowEnd {
		t.Fatalf("window end=%d, want %d", got, wantWindowEnd)
	}
	if got := appInt64(node["viewportSpan"]); got != int64(viewportRows*16) {
		t.Fatalf("viewport span=%d, want %d", got, viewportRows*16)
	}
	viewportRow := semanticInt(node["viewportRow"])
	if viewportRow != bufferRows || appInt64(windowRows[viewportRow]["offset"]) != wantTop {
		t.Fatalf("viewport row=%d offset=%d, want row=%d offset=%d",
			viewportRow, appInt64(windowRows[viewportRow]["offset"]), bufferRows, wantTop)
	}
	for i, row := range windowRows {
		wantOffset := wantWindowStart + int64(i*16)
		if got := appInt64(row["offset"]); got != wantOffset {
			t.Fatalf("window row %d offset=%d, want %d", i, got, wantOffset)
		}
		if got := appInt64(row["endOffset"]); got != wantOffset+16 {
			t.Fatalf("window row %d end=%d, want %d", i, got, wantOffset+16)
		}
	}

	viewer.backend.mu.Lock()
	cacheOff := viewer.backend.cacheOff
	cacheBytes := len(viewer.backend.cacheData)
	viewer.backend.mu.Unlock()
	if cacheBytes == 0 || cacheBytes > 256*1024 {
		t.Fatalf("viewer retained %d cache bytes, want 1..256 KiB", cacheBytes)
	}
	if cacheOff > wantWindowStart || cacheOff+int64(cacheBytes) < wantWindowEnd {
		t.Fatalf("cache [%d,%d) does not cover semantic window [%d,%d)",
			cacheOff, cacheOff+int64(cacheBytes), wantWindowStart, wantWindowEnd)
	}

	reads := file.readRanges()
	if len(reads) != 2 {
		t.Fatalf("10 GiB sparse viewer performed %d reads, want header + one far cache fill: %#v",
			len(reads), reads)
	}
	if reads[0].offset != 0 || reads[0].length != 16*1024 {
		t.Fatalf("unexpected header read: %#v", reads[0])
	}
	if reads[1].offset <= 6*1024*1024*1024 || reads[1].length > 256*1024 {
		t.Fatalf("far cache read was not bounded/random-access: %#v", reads[1])
	}
	file.mu.Lock()
	maxRead := file.maxRead
	file.mu.Unlock()
	if maxRead > 256*1024 {
		t.Fatalf("largest read=%d, want at most 256 KiB", maxRead)
	}
}

func TestSemantic_ViewerNoWrapWindowConsumesWholeLogicalLine(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "long-line.txt")
	first := strings.Repeat("a", 300)
	content := first + "\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.WrapMode = false
	viewer.SetPosition(0, 0, 19, 5)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	rows := appMapSlice(awaitSemanticViewerWindow(t, viewer, 2)["windowRows"])
	if len(rows) < 2 {
		t.Fatalf("rows = %#v", rows)
	}
	if got, want := appInt64(rows[1]["offset"]), int64(len(first)+1); got != want {
		t.Fatalf("second logical row offset=%d, want %d", got, want)
	}
}

func TestSemantic_ViewerWrappedScrollWindowUsesVisualRows(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "wrapped-window.txt")
	content := strings.Repeat("x", 4096) + "\nnext\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 19, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	width := viewer.semanticContentWidth()
	if width != 19 {
		t.Fatalf("viewer content width=%d, want 19", width)
	}
	targetRow := 24
	wantTop := int64(targetRow * width)
	requestedOffset := wantTop + 7
	beforeGeneration := viewer.semanticWindowGeneration
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": requestedOffset,
	}) {
		t.Fatal("wrapped viewer scroll was not handled")
	}
	if !viewer.semanticPendingScroll {
		t.Fatal("initial uncached wrapped seek did not enter pending state")
	}

	deadline := time.After(2 * time.Second)
	var node map[string]any
	for viewer.semanticPendingScroll {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timed out resolving wrapped viewer seek: top=%d", viewer.TopOffset)
		}
		node = viewer.SemanticNode(nil)
	}
	if viewer.TopOffset != wantTop {
		t.Fatalf("wrapped seek top=%d, want visual row start %d", viewer.TopOffset, wantTop)
	}
	if viewer.semanticWindowGeneration != beforeGeneration+1 {
		t.Fatalf("wrapped seek generation=%d, want %d",
			viewer.semanticWindowGeneration, beforeGeneration+1)
	}
	if node == nil {
		node = viewer.SemanticNode(nil)
	}

	windowRows := appMapSlice(node["windowRows"])
	viewportRow := semanticInt(node["viewportRow"])
	if viewportRow < 0 || viewportRow >= len(windowRows) {
		t.Fatalf("viewportRow=%d outside %d wrapped rows", viewportRow, len(windowRows))
	}
	if got := appInt64(windowRows[viewportRow]["offset"]); got != wantTop {
		t.Fatalf("wrapped viewport row offset=%d, want %d", got, wantTop)
	}
	bufferRows := semanticWindowBufferRows(8)
	wantWindowStart := wantTop - int64(bufferRows*width)
	if got := appInt64(node["windowStart"]); got != wantWindowStart {
		t.Fatalf("wrapped window start=%d, want %d visual rows before viewport",
			got, wantWindowStart)
	}

	// The same resolver must also work synchronously once the bounded backend
	// cache is warm; otherwise only the delayed/pending path would be covered.
	nextRow := targetRow + 7
	nextTop := int64(nextRow * width)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": nextTop + 3,
	}) {
		t.Fatal("cached wrapped viewer scroll was not handled")
	}
	if viewer.semanticPendingScroll {
		t.Fatal("cached wrapped viewer scroll unexpectedly became pending")
	}
	if viewer.TopOffset != nextTop {
		t.Fatalf("cached wrapped seek top=%d, want visual row start %d",
			viewer.TopOffset, nextTop)
	}
}

func TestSemantic_ViewerWrappedSeekResumesAcrossCacheWindows(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "megabyte-wrapped-window.txt")
	content := strings.Repeat("x", 1280*1024) // Deliberately no newline.
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 19, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	width := viewer.semanticContentWidth()
	targetRow := (1024 * 1024) / width
	wantTop := int64(targetRow * width)
	requestedOffset := wantTop + int64(width/2)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": requestedOffset,
	}) {
		t.Fatal("megabyte wrapped viewer scroll was not handled")
	}
	if !viewer.semanticPendingScroll {
		t.Fatal("megabyte wrapped seek unexpectedly resolved without cache fills")
	}

	deadline := time.After(10 * time.Second)
	cacheFills := 0
	var node map[string]any
	for viewer.semanticPendingScroll {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			cacheFills++
		case <-deadline:
			t.Fatalf("wrapped seek did not resume to %d: curr=%d target=%d fills=%d",
				wantTop, viewer.semanticWrapSeek.curr,
				viewer.semanticWrapSeek.target, cacheFills)
		}
		node = viewer.SemanticNode(nil)
	}
	if cacheFills < 4 {
		t.Fatalf("test crossed only %d cache fills; expected a multi-window seek", cacheFills)
	}
	if viewer.TopOffset != wantTop {
		t.Fatalf("megabyte wrapped seek top=%d, want %d", viewer.TopOffset, wantTop)
	}

	bufferRows := semanticWindowBufferRows(8)
	wantHistory := bufferRows + 1
	seek := &viewer.semanticWrapSeek
	if seek.active || !seek.ready {
		t.Fatalf("completed wrapped seek state active=%v ready=%v", seek.active, seek.ready)
	}
	if len(seek.history) != wantHistory || seek.historyCount != wantHistory {
		t.Fatalf("wrapped history backing=%d count=%d, want bounded %d",
			len(seek.history), seek.historyCount, wantHistory)
	}
	previous := wantTop
	for row := 1; row <= bufferRows; row++ {
		got, ok := seek.previousHistoryOffset(previous, width)
		want := wantTop - int64(row*width)
		if !ok || got != want {
			t.Fatalf("history predecessor %d=(%d,%v), want %d", row, got, ok, want)
		}
		previous = got
	}
	viewer.backend.mu.Lock()
	cachedBytes := len(viewer.backend.cacheData)
	viewer.backend.mu.Unlock()
	if cachedBytes > 256*1024 {
		t.Fatalf("wrapped seek retained %d cache bytes, want at most 256 KiB", cachedBytes)
	}

	if node == nil {
		node = viewer.SemanticNode(nil)
	}
	wantWindowStart := wantTop - int64(bufferRows*width)
	if got := appInt64(node["windowStart"]); got != wantWindowStart {
		t.Fatalf("megabyte wrapped window start=%d, want %d", got, wantWindowStart)
	}
	viewportRow := semanticInt(node["viewportRow"])
	rows := appMapSlice(node["windowRows"])
	if viewportRow != bufferRows || viewportRow >= len(rows) ||
		appInt64(rows[viewportRow]["offset"]) != wantTop {
		t.Fatalf("megabyte wrapped viewport row=%d rows=%#v", viewportRow, rows)
	}
}

func TestSemantic_EditorWindowAndScrollPreserveCursor(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&content, "row-%03d\n", i)
	}
	ev := NewEditorView(piecetable.New([]byte(content.String())), nil, "window.txt")
	ev.SetPosition(0, 0, 39, 8)
	ev.ScrollTopRow = 40
	ev.CursorLine = 44
	ev.CursorPos = 2
	cursorLine, cursorPos := ev.CursorLine, ev.CursorPos

	node := ev.SemanticNode(nil)
	windowRows := appMapSlice(node["windowRows"])
	viewportRow := semanticInt(node["viewportRow"])
	if semanticString(node["scrollUnit"]) != "rows" ||
		appInt64(node["viewportStart"]) != 40 ||
		appInt64(node["contentExtent"]) != int64(ev.engine.GetTotalVisualRows()) ||
		node["contentExtentKnown"] != true {
		t.Fatalf("invalid editor window contract: %#v", node)
	}
	if viewportRow < 1 || viewportRow >= len(windowRows) ||
		semanticInt(windowRows[viewportRow]["visualRow"]) != 40 {
		t.Fatalf("viewport row=%d rows=%#v", viewportRow, windowRows)
	}
	for i := 0; i+1 < len(windowRows); i++ {
		if semanticInt(windowRows[i+1]["visualRow"]) != semanticInt(windowRows[i]["visualRow"])+1 {
			t.Fatalf("non-contiguous editor rows at %d", i)
		}
	}
	if got := appInt64(node["cursorAbsoluteRow"]); got != int64(cursorLine) {
		t.Fatalf("cursor absolute row=%d, want %d", got, cursorLine)
	}
	if !ev.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(ev), "action": "editor.scroll", "visualRow": 73,
	}) {
		t.Fatal("editor scroll was not handled")
	}
	if ev.ScrollTopRow != 73 || ev.CursorLine != cursorLine || ev.CursorPos != cursorPos {
		t.Fatalf("scroll changed editor state: top=%d cursor=%d:%d", ev.ScrollTopRow,
			ev.CursorLine, ev.CursorPos)
	}
	before := ev.semanticWindowGeneration
	ev.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(ev), "action": "editor.scroll", "visualRow": 1 << 30,
	})
	if ev.semanticWindowGeneration != before+1 {
		t.Fatalf("clamped editor generation=%d, want %d", ev.semanticWindowGeneration, before+1)
	}
}

func TestSemantic_EditorWindowExportsCursorOutsideCanonicalViewport(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&content, "row-%03d\n", i)
	}
	ev := NewEditorView(piecetable.New([]byte(content.String())), nil, "cursor-window.txt")
	ev.SetPosition(0, 0, 39, 8)
	ev.SetVisible(true)
	ev.ScrollTopRow = 40
	ev.CursorLine = 37 // Above the terminal viewport, but inside its top overscan.
	ev.CursorPos = 2

	node := ev.SemanticNode(nil)
	if got := appInt64(node["cursorAbsoluteRow"]); got != 37 {
		t.Fatalf("cursor absolute row=%d, want 37", got)
	}
	if got := semanticInt(node["cursorVisualRow"]); got != -3 {
		t.Fatalf("cursor viewport row=%d, want -3", got)
	}
	if got := semanticInt(node["cursorVisualColumn"]); got != 2 {
		t.Fatalf("cursor visual column=%d, want 2", got)
	}
	if node["cursorVisible"] != true || semanticString(node["cursorShape"]) != "underline" {
		t.Fatalf("cursor metadata not exported outside canonical viewport: %#v", node)
	}
	windowRows := appMapSlice(node["windowRows"])
	foundCursorRow := false
	for _, row := range windowRows {
		if semanticInt(row["visualRow"]) == 37 {
			foundCursorRow = true
			break
		}
	}
	if !foundCursorRow {
		t.Fatalf("cursor row 37 missing from overscan: %#v", windowRows)
	}

	ev.ScrollLeft = 3
	ev.overtype = true
	node = ev.SemanticNode(nil)
	if got := semanticInt(node["cursorVisualColumn"]); got != -1 {
		t.Fatalf("horizontally clipped cursor column=%d, want -1", got)
	}
	if node["cursorVisible"] != false || semanticString(node["cursorShape"]) != "block" {
		t.Fatalf("horizontal clipping or cursor shape was lost: %#v", node)
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
	if model.CursorPosition != 3 {
		t.Fatalf("command text cursor=%d, want UTF-16 position 3", model.CursorPosition)
	}

	cl.Edit.SetText("a😀b")
	model = cl.semanticModel(nil)
	if model.CursorPosition != 4 {
		t.Fatalf("unicode command cursor=%d, want UTF-16 position 4", model.CursorPosition)
	}
	cl.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_LEFT,
	})
	model = cl.semanticModel(nil)
	if model.CursorPosition != 3 {
		t.Fatalf("unicode cursor after Left=%d, want UTF-16 position 3", model.CursorPosition)
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

func TestCommandSubmitWritesPTYAndRevealsTerminal(t *testing.T) {
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(100, 30)
	pf.cmdLine.Edit.SetText("ls")
	pty, ok := pf.pty.(*mockPty)
	if !ok {
		t.Fatalf("test PTY has unexpected type %T", pf.pty)
	}
	pty.Reset()

	if !pf.HandleSemanticAction(map[string]any{"action": "command.submit"}) {
		t.Fatal("command.submit was not handled")
	}
	if !strings.Contains(pty.String(), "ls") {
		t.Fatalf("command was not written to PTY: %q", pty.String())
	}
	if pf.showPanels {
		t.Fatal("command submission did not reveal the terminal")
	}
	if !pf.returnToPanels {
		t.Fatal("foreground command would not restore panels on completion")
	}
	if !pf.cmdLine.IsEmpty() {
		t.Fatalf("submitted command line was not cleared: %q", pf.cmdLine.Edit.GetText())
	}
	// The clean command becomes visible atomically when the managed shell
	// acknowledges that its technical wrapper has finished echoing.
	pf.parser.Process([]byte("\x1b]133;C\x07"))
	terminal := pf.termView.semanticModel(nil)
	var rendered strings.Builder
	for _, row := range terminal.Rows {
		for _, run := range row.Runs {
			rendered.WriteString(run.Text)
		}
	}
	if !strings.Contains(rendered.String(), "ls") {
		t.Fatalf("submitted command was absent from terminal semantic rows: %q",
			rendered.String())
	}
}

type synchronousCommandEchoPTY struct {
	mockPty
	parser *AnsiParser
}

func (p *synchronousCommandEchoPTY) Write(data []byte) (int, error) {
	p.mockPty.Write(data)
	// Model the native race: the read goroutine may consume a fragmented shell
	// echo, OSC C, stdout and OSC D before Write returns to PanelsFrame.
	middle := len(data) / 2
	p.parser.Process(data[:middle])
	p.parser.Process(data[middle:])
	p.parser.Process([]byte("\r\n\x1b]133;C\x07F4_OUTPUT_OK\r\n\x1b]133;D\x07"))
	return len(data), nil
}

func TestCommandSubmitPreparesCleanOutputBeforeSynchronousPTYEcho(t *testing.T) {
	path := t.TempDir()
	left := &FileSystemPanel{vfs: vfs.NewOSVFS(path)}
	right := &FileSystemPanel{vfs: vfs.NewOSVFS(path)}
	terminal := NewTerminalView(100, 24)
	pty := &synchronousCommandEchoPTY{}
	parser := NewAnsiParser(terminal, pty)
	pty.parser = parser
	pf := &PanelsFrame{
		panels:         [2]Panel{left, right},
		activeIdx:      1,
		showPanels:     true,
		showLeftPanel:  true,
		showRightPanel: true,
		cmdLine:        NewCommandLine(">"),
		termView:       terminal,
		parser:         parser,
		pty:            pty,
	}
	pf.cmdLine.Edit.SetText("echo F4_USER_COMMAND")

	if !pf.HandleSemanticAction(map[string]any{"action": "command.submit"}) {
		t.Fatal("command.submit was not handled")
	}

	got := string(terminal.GetAllLogBytes())
	if !strings.Contains(got, "echo F4_USER_COMMAND") || !strings.Contains(got, "F4_OUTPUT_OK") {
		t.Fatalf("clean command/output missing after synchronous PTY echo: %q", got)
	}
	for _, technical := range []string{"set +H", "FARVTRESULT", `printf "\033]133`} {
		if strings.Contains(got, technical) {
			t.Fatalf("technical wrapper leaked through pre-Write race (%q): %q", technical, got)
		}
	}
	if terminal.Muted || terminal.pendingCleanCommand != "" {
		t.Fatalf("managed command did not settle: muted=%v pending=%q",
			terminal.Muted, terminal.pendingCleanCommand)
	}
}

func TestGlobalCommandSubmitBypassesAutocompleteOverlay(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(100, 30)
	pf.cmdLine.Edit.SetText("ls")
	pty := pf.pty.(*mockPty)
	pty.Reset()
	vtui.FrameManager.Push(pf)
	autocomplete := vtui.NewAutoCompleteMenu(pf.cmdLine.Edit)
	vtui.FrameManager.Push(autocomplete)

	if !HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(pf),
		"action": "command.submit",
	}) {
		t.Fatal("global command.submit was not handled")
	}
	if got := pty.String(); !strings.Contains(got, "ls") {
		t.Fatalf("autocomplete overlay intercepted command submission: %q", got)
	}
	if pf.showPanels {
		t.Fatal("submitted command did not reveal terminal")
	}
}

func TestCommandSubmitWithoutPTYKeepsCommandAndPanels(t *testing.T) {
	pf := setupMockPanelsFrame()
	pf.ptyMutex.Lock()
	oldPTY := pf.pty
	pf.pty = nil
	pf.ptyMutex.Unlock()
	defer func() {
		_ = oldPTY.Close()
		pf.Close()
	}()
	pf.cmdLine.Edit.SetText("ls")

	if !pf.HandleSemanticAction(map[string]any{"action": "command.submit"}) {
		t.Fatal("command.submit without PTY was not consumed")
	}
	if !pf.showPanels {
		t.Fatal("missing PTY hid the panels")
	}
	if got := pf.cmdLine.Edit.GetText(); got != "ls" {
		t.Fatalf("missing PTY discarded command %q", got)
	}
}
