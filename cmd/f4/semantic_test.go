package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type semanticBlockingLocalVFS struct {
	*vfs.OSVFS
	readStarted chan struct{}
	readDone    chan struct{}
}

type semanticMaterializingVFS struct {
	*vfs.NullVFS
	localRoot string
}

func (s *semanticMaterializingVFS) LocalPath(logicalPath string) (string, error) {
	return filepath.Join(s.localRoot, filepath.Base(logicalPath)), nil
}

func semanticMetadataChunkForModel(t *testing.T, model extui.PanelModel) map[string]any {
	t.Helper()
	chunk, ok := BuildPanelCatalogMetadataChunk(model.ID, model.Path,
		model.CatalogRevision, model.MetadataRevision, 0, 128)
	if !ok {
		t.Fatalf("metadata chunk rejected for panel=%q path=%q catalog=%d metadata=%d",
			model.ID, model.Path, model.CatalogRevision, model.MetadataRevision)
	}
	return chunk
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

func TestSemanticPanelFileInfoSettingIsBoundedDynamicState(t *testing.T) {
	previousCapability := setExtUiPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogMetadataEnabled(previousCapability) })
	previousSetting := AppConfig.ShowPanelFileInfo
	t.Cleanup(func() { AppConfig.ShowPanelFileInfo = previousSetting })

	panel := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(t.TempDir()),
		table:         vtui.NewTable(0, 0, 40, 10, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "one.txt", Size: 1}},
			{VFSItem: vfs.VFSItem{Name: "two.txt", Size: 2}},
		},
	}
	t.Cleanup(panel.unpublishSemanticMetadataSnapshot)

	AppConfig.ShowPanelFileInfo = false
	full := panel.semanticPanelModel(nil, 0, true)
	if full.ShowFileInfo {
		t.Fatal("disabled file-information setting was exported as enabled")
	}

	AppConfig.ShowPanelFileInfo = true
	header, ok := panel.semanticPanelHeaderModel(nil, 0, true)
	if !ok {
		t.Fatal("warm panel did not provide a row-free incremental header")
	}
	if !header.ShowFileInfo {
		t.Fatal("enabled file-information setting was missing from incremental state")
	}
	if len(header.Entries) != 0 || header.CatalogRevision != full.CatalogRevision {
		t.Fatalf("file-information toggle rebuilt or leaked the catalog: %#v", header)
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

func TestToastDismissSemanticAction(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.ShowToast("PlugRing: 1 plugin update available!", time.Minute)
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(time.Second):
		t.Fatal("toast task was not posted")
	}

	if !HandleSemanticAction(map[string]any{
		"action": "toast.dismiss",
		"target": "toast",
	}) {
		t.Fatal("toast dismiss action was not handled")
	}
	if got := vtui.FrameManager.GetActiveToast(); got != "" {
		t.Fatalf("toast remained visible after dismiss: %q", got)
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
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 1234, MTime: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), Mode: "-rw-r--r--", IsHidden: true}, Selected: true},
		},
	}
	fp.SetCanFocus(true)
	fp.SetPosition(0, 0, 39, 9)
	fp.table.Columns = []vtui.TableColumn{
		{Title: "Name", Width: 27},
		{Title: "Size ↓", Width: 10},
	}
	fp.SetCursorIndex(1)
	fp.fastFindMode = true
	fp.fastFindStr = "*pha"

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
	if node["metadataDeferred"] != true || node["metadataRevision"] != int64(1) {
		t.Fatalf("deferred metadata contract missing: %#v", node)
	}
	if node["cursorEntryId"] != entries[1]["entryId"] || entries[1]["entryId"] == "" {
		t.Fatalf("cursor identity was not exported: panel=%#v entry=%#v", node["cursorEntryId"], entries[1]["entryId"])
	}
	if node["fastFind"] != true || node["fastFindText"] != "*pha" {
		t.Fatalf("quick-search query was not exported: %#v", node)
	}
	if node["fastFindMatchColor"] != semanticAttrColor(vtui.Palette[ColPanelHighlightText], true) {
		t.Fatalf("quick-search color = %v, want active Panel.Text.Highlight", node["fastFindMatchColor"])
	}
	matches := appMap(node["fastFindMatches"])
	match := appMap(matches[semanticString(entries[1]["entryId"])])
	if semanticInt(match["start"]) != 2 || semanticInt(match["length"]) != 3 {
		t.Fatalf("quick-search match span was not exported: %#v", matches)
	}
	columns := node["galleryColumns"].([]map[string]any)
	if len(columns) != 2 || columns[0]["role"] != "name" || columns[0]["sortMode"] != "name" || columns[0]["sortable"] != true {
		t.Fatalf("unexpected unified name column metadata: %#v", columns)
	}
	if columns[1]["role"] != "size" || columns[1]["sortMode"] != "size" || columns[1]["alignment"] != "right" {
		t.Fatalf("unexpected unified size column metadata: %#v", columns[1])
	}
	if entries[1]["isImage"] != false || entries[1]["isHidden"] != true {
		t.Fatalf("minimal identity/type metadata is incomplete: %#v", entries[1])
	}
	for _, deferred := range []string{"path", "localPath", "size", "sizeText", "mtimeNanos", "version", "mode"} {
		if _, present := entries[1][deferred]; present {
			t.Fatalf("deferred entry field %q leaked into base scene: %#v", deferred, entries[1])
		}
	}
	for _, deferred := range []string{"highlightRevision", "selectedSize", "totalSize"} {
		if _, present := node[deferred]; present {
			t.Fatalf("deferred panel field %q leaked into base scene: %#v", deferred, node)
		}
	}
	styles := appMap(node["highlightStyles"])
	referencedStyles := make(map[string]bool)
	for _, entry := range entries {
		styleID := semanticString(entry["highlightStyleId"])
		if styleID == "" {
			continue
		}
		referencedStyles[styleID] = true
		if _, present := styles[styleID]; !present {
			t.Fatalf("minimal entry references missing highlight style %q: entries=%#v styles=%#v", styleID, entries, styles)
		}
	}
	if len(styles) != len(referencedStyles) {
		t.Fatalf("base scene exported unreferenced highlight styles: referenced=%#v styles=%#v", referencedStyles, styles)
	}
	chunk := semanticMetadataChunkForModel(t, model)
	metadataEntries := appMapSlice(chunk["entries"])
	if metadataEntries[1]["localPath"] != filepath.Join(tmp, "alpha.txt") {
		t.Fatalf("localPath = %v, want %s", metadataEntries[1]["localPath"], filepath.Join(tmp, "alpha.txt"))
	}
	wantNanos := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC).UnixNano()
	if metadataEntries[1]["mtimeNanos"] != wantNanos || metadataEntries[1]["size"] != int64(1234) {
		t.Fatalf("unexpected deferred file version metadata: %#v", metadataEntries[1])
	}
	for _, redundant := range []string{"physicalSize", "isExecutable", "isImage", "sizeCalculated", "atimeNanos", "ctimeNanos", "version", "revision", "unixMode", "uid", "gid", "winAttrs"} {
		if _, present := metadataEntries[1][redundant]; present {
			t.Fatalf("redundant metadata field %q was serialized: %#v", redundant, metadataEntries[1])
		}
	}
}

func TestSemanticPagedPanelExportsViewportAndServesOnlyRequestedRows(t *testing.T) {
	previousRowsCapability := setExtUiPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogRowsEnabled(previousRowsCapability) })

	directory := t.TempDir()
	const entryCount = 30_000
	entries := make([]*fileEntry, entryCount)
	for index := range entries {
		entries[index] = &fileEntry{VFSItem: vfs.VFSItem{
			Name:  fmt.Sprintf("entry-%05d.txt", index),
			IsDir: index%7 == 0,
		}}
	}
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(directory),
		table:         vtui.NewTable(0, 0, 80, 40, nil),
		selectedItems: make(map[string]bool),
		entries:       entries,
	}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)
	fp.SetCursorIndex(15_000)

	model := fp.semanticPanelModel(
		&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
	if !model.CatalogRowsDeferred || model.TotalCount != entryCount {
		t.Fatalf("paged catalog contract = deferred %v total %d, want true/%d",
			model.CatalogRowsDeferred, model.TotalCount, entryCount)
	}
	if len(model.Entries) != initialPanelCatalogRowsLimit {
		t.Fatalf("initial catalog page = %d rows, want %d",
			len(model.Entries), initialPanelCatalogRowsLimit)
	}
	if fp.semanticMetadataSnapshot != nil {
		t.Fatal("paged panel retained a complete metadata snapshot")
	}

	const offset = 12_345
	response, ok := BuildLivePanelCatalogRows(
		model.ID, directory, model.CatalogRevision, offset,
		maxPanelCatalogRowsLimit)
	if !ok {
		t.Fatal("current bounded row request was rejected")
	}
	page := appMapSlice(response["entries"])
	if len(page) != maxPanelCatalogRowsLimit {
		t.Fatalf("requested page = %d rows, want %d",
			len(page), maxPanelCatalogRowsLimit)
	}
	if semanticInt(page[0]["index"]) != offset ||
		semanticString(page[0]["name"]) != "entry-12345.txt" {
		t.Fatalf("first requested row = %#v", page[0])
	}
	if _, ok := BuildLivePanelCatalogRows(
		model.ID, directory, model.CatalogRevision-1, offset, 1); ok {
		t.Fatal("stale catalog revision unexpectedly served rows")
	}
}

func TestSemanticPagedFastFindHeaderMatchesOnlyViewportWindow(t *testing.T) {
	previousRowsCapability := setExtUiPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogRowsEnabled(previousRowsCapability) })

	directory := t.TempDir()
	const entryCount = 30_000
	const cursor = 15_000
	entries := make([]*fileEntry, entryCount)
	for index := range entries {
		entries[index] = &fileEntry{VFSItem: vfs.VFSItem{
			Name: fmt.Sprintf("entry-%05d.txt", index),
		}}
	}
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(directory),
		table:         vtui.NewTable(0, 0, 80, 40, nil),
		selectedItems: make(map[string]bool),
		entries:       entries,
	}
	fp.SetCursorIndex(cursor)
	fp.fastFindMode = true
	fp.fastFindStr = "entry"

	header, ok := fp.semanticPanelHeaderModel(
		&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
	if !ok {
		t.Fatal("paged fast-find header was rejected")
	}
	if len(header.Entries) != 0 {
		t.Fatalf("fast-find header leaked %d catalog rows", len(header.Entries))
	}
	if got := len(header.FastFindMatches); got != semanticFastFindRowsLimit {
		t.Fatalf("fast-find map has %d rows, want bounded window %d", got,
			semanticFastFindRowsLimit)
	}
	if got := len(fp.fastFindMatchCache); got != semanticFastFindRowsLimit {
		t.Fatalf("fast-find evaluated %d of %d rows", got, entryCount)
	}
	if _, present := header.FastFindMatches[header.CursorEntryID]; !present {
		t.Fatalf("cursor %q is outside bounded fast-find map", header.CursorEntryID)
	}

	// Re-exporting the same header must reuse the lazy row results instead of
	// evaluating the remaining 29,744 entries.
	second, ok := fp.semanticPanelHeaderModel(nil, 0, true)
	if !ok || len(second.FastFindMatches) != semanticFastFindRowsLimit {
		t.Fatalf("second bounded header = ok:%v matches:%d", ok,
			len(second.FastFindMatches))
	}
	if got := len(fp.fastFindMatchCache); got != semanticFastFindRowsLimit {
		t.Fatalf("unchanged header expanded match cache to %d rows", got)
	}

	first, end, active := fp.semanticFastFindRange()
	fp.SetCursorIndex(cursor + 1)
	stableFirst, stableEnd, stableActive := fp.semanticFastFindRange()
	if !active || !stableActive || first != stableFirst || end != stableEnd {
		t.Fatalf("adjacent cursor shifted fast-find window: (%d,%d,%v) -> (%d,%d,%v)",
			first, end, active, stableFirst, stableEnd, stableActive)
	}
	third, ok := fp.semanticPanelHeaderModel(nil, 0, true)
	if !ok || !reflect.DeepEqual(second.FastFindMatches, third.FastFindMatches) {
		t.Fatal("adjacent cursor rebuilt the bounded fast-find match map")
	}
	if got := len(fp.fastFindMatchCache); got != semanticFastFindRowsLimit {
		t.Fatalf("stable cursor step expanded match cache to %d rows", got)
	}
}

func TestSemanticFastFindWindowLimitMatchesLayoutCapacity(t *testing.T) {
	fp := &FileSystemPanel{galleryLayoutMode: GalleryLayoutDetails}
	if got := fp.semanticFastFindWindowLimit(); got != semanticFastFindDetailsRowsLimit {
		t.Fatalf("details fast-find window = %d, want %d",
			got, semanticFastFindDetailsRowsLimit)
	}
	fp.galleryLayoutMode = GalleryLayoutGrid
	if got := fp.semanticFastFindWindowLimit(); got != semanticFastFindRowsLimit {
		t.Fatalf("grid fast-find window = %d, want %d",
			got, semanticFastFindRowsLimit)
	}
}

func TestSemanticPanelLegacyFallbackIncludesCompleteMetadata(t *testing.T) {
	previousCapability := setExtUiPanelCatalogMetadataEnabled(false)
	t.Cleanup(func() { setExtUiPanelCatalogMetadataEnabled(previousCapability) })
	previousHighlighter := GlobalFileHighlighter
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ParseIni(strings.NewReader(`
[Highlight_0]
Name = Text
Mask = *.txt
NormalColor = foreground:#123456
`)))
	GlobalFileHighlighter = highlighter
	t.Cleanup(func() { GlobalFileHighlighter = previousHighlighter })

	tmp := t.TempDir()
	mtime := time.Date(2026, 8, 17, 10, 11, 12, 13, time.UTC)
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		table:         vtui.NewTable(0, 0, 40, 10, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{{VFSItem: vfs.VFSItem{
			Name:         "legacy.txt",
			Size:         1234,
			MTime:        mtime,
			Mode:         "-rwxr-xr-x",
			IsHidden:     true,
			IsExecutable: true,
		}, Selected: true}},
	}

	model := fp.semanticPanelModel(nil, 0, true)
	node := model.ToMap()
	if model.MetadataDeferred || node["metadataDeferred"] != nil || node["metadataRevision"] != nil {
		t.Fatalf("legacy panel advertised deferred metadata: %#v", node)
	}
	entries := appMapSlice(node["entries"])
	if len(entries) != 1 {
		t.Fatalf("legacy entries = %#v", entries)
	}
	entry := entries[0]
	if entry["localPath"] != filepath.Join(tmp, "legacy.txt") ||
		appInt64(entry["size"]) != 1234 ||
		entry["sizeText"] != "1 234" ||
		appInt64(entry["mtimeNanos"]) != mtime.UnixNano() ||
		entry["mode"] != "-rwxr-xr-x" || entry["isHidden"] != true ||
		entry["isExecutable"] != true || semanticString(entry["highlightStyleId"]) == "" {
		t.Fatalf("legacy full entry metadata is incomplete: %#v", entry)
	}
	if appInt64(node["selectedSize"]) != 1234 || appInt64(node["totalSize"]) != 1234 ||
		appInt64(node["highlightRevision"]) == 0 || len(appMap(node["highlightStyles"])) != 1 {
		t.Fatalf("legacy full panel metadata is incomplete: %#v", node)
	}
	if _, ok := BuildPanelCatalogMetadataChunk(model.ID, model.Path,
		model.CatalogRevision, model.MetadataRevision, 0, 1); ok {
		t.Fatal("legacy full catalog unexpectedly required/published a metadata snapshot")
	}

	// A legacy client has no MetadataRevision field, so a serialized metadata
	// mutation must remain authoritative through CatalogRevision.
	fp.entries[0].Size++
	changed := fp.semanticPanelModel(nil, 0, true)
	if changed.CatalogRevision != model.CatalogRevision+1 {
		t.Fatalf("legacy metadata change did not advance catalog revision: before=%d after=%d",
			model.CatalogRevision, changed.CatalogRevision)
	}
}

func TestSemanticDeferredMetadataRevisionTracksOnlyRelevantAccessTime(t *testing.T) {
	previousHighlighter := GlobalFileHighlighter
	GlobalFileHighlighter = &FileHighlighter{}
	t.Cleanup(func() { GlobalFileHighlighter = previousHighlighter })

	baseTime := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(t.TempDir()),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "test"),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{{VFSItem: vfs.VFSItem{
			Name: "folder", IsDir: true, MTime: baseTime, ATime: baseTime,
		}}},
	}

	first := fp.semanticPanelModel(nil, 0, true)
	fp.entries[0].ATime = baseTime.Add(time.Second)
	unchanged := fp.semanticPanelModel(nil, 0, true)
	if unchanged.MetadataRevision != first.MetadataRevision {
		t.Fatalf("unobserved access time advanced deferred metadata revision: before=%d after=%d",
			first.MetadataRevision, unchanged.MetadataRevision)
	}
	if !reflect.DeepEqual(first.ToMap(), unchanged.ToMap()) {
		t.Fatal("unobserved access time changed the deferred panel model")
	}

	GlobalFileHighlighter = &FileHighlighter{
		Revision: 1,
		Rules: []HighlightRule{{
			RuleID: "recent-access", DateType: DateAccessed,
			DateAfter: baseTime, NormalStr: "yellow",
		}},
	}
	observed := fp.semanticPanelModel(nil, 0, true)
	fp.entries[0].ATime = baseTime.Add(-time.Second)
	changed := fp.semanticPanelModel(nil, 0, true)
	if changed.MetadataRevision != observed.MetadataRevision+1 {
		t.Fatalf("highlight-relevant access time did not advance metadata revision: before=%d after=%d",
			observed.MetadataRevision, changed.MetadataRevision)
	}
}

func TestSemanticPanelExportsCatalogProvisionalState(t *testing.T) {
	fp := &FileSystemPanel{
		vfs:                vfs.NewOSVFS("/"),
		table:              vtui.NewTable(0, 0, 80, 24, nil),
		selectedItems:      make(map[string]bool),
		catalogProvisional: true,
		isLoading:          true,
		entries: []*fileEntry{{
			VFSItem: vfs.VFSItem{Name: "..", IsDir: true},
		}},
	}
	model := fp.semanticPanelModel(nil, 0, true)
	if !model.CatalogProvisional || !model.Loading {
		t.Fatalf("placeholder state not exported: %#v", model)
	}
	fp.catalogInteractive = true
	model = fp.semanticPanelModel(nil, 0, true)
	if !model.CatalogProvisional || model.Loading {
		t.Fatalf("usable provisional catalog remained interaction-blocking: %#v", model)
	}
	fp.catalogInteractive = false
	fp.catalogProvisional = false
	model = fp.semanticPanelModel(nil, 0, true)
	if model.CatalogProvisional || !model.Loading {
		t.Fatalf("cold authoritative base must remain loading until completion: %#v", model)
	}
}

func TestSemanticOpenSurvivesMatchingProvisionalCatalogReplacement(t *testing.T) {
	previousRowsCapability := setExtUiPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogRowsEnabled(previousRowsCapability) })

	directory := t.TempDir()
	fp := &FileSystemPanel{
		vfs:                vfs.NewOSVFS(directory),
		table:              vtui.NewTable(0, 0, 80, 24, nil),
		selectedItems:      make(map[string]bool),
		catalogProvisional: true,
		catalogInteractive: true,
		entries: []*fileEntry{{
			VFSItem: vfs.VFSItem{Name: "..", IsDir: true},
		}},
	}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)

	preview := fp.semanticPanelModel(nil, 0, true)
	if len(preview.Entries) != 1 {
		t.Fatalf("preview entries = %d, want 1", len(preview.Entries))
	}
	upID := preview.Entries[0].EntryID

	fp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "authoritative", IsDir: true}},
	}
	fp.catalogProvisional = false
	fp.markSemanticCatalogMutation()
	authoritative := fp.semanticPanelModel(nil, 0, true)
	if authoritative.CatalogRevision == preview.CatalogRevision {
		t.Fatal("authoritative replacement did not advance catalog revision")
	}

	staleOpen := map[string]any{
		"action":          "panel.open",
		"entryId":         upID,
		"index":           0,
		"catalogRevision": preview.CatalogRevision,
	}
	if index, ok := fp.semanticEntryIndex(staleOpen); !ok || index != 0 {
		t.Fatalf("matching stale open resolved to (%d, %v), want (0, true)", index, ok)
	}

	staleCursor := map[string]any{
		"action":          "panel.cursor",
		"entryId":         upID,
		"index":           0,
		"catalogRevision": preview.CatalogRevision,
	}
	if index, ok := fp.semanticEntryIndex(staleCursor); ok {
		t.Fatalf("stale cursor unexpectedly resolved to %d", index)
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
		t.Fatalf("provisional highlight metadata missing from base model: revision=%d styles=%#v",
			model.HighlightRevision, model.HighlightStyles)
	}
	firstStyleID := model.Entries[0].HighlightStyleID
	if firstStyleID == "" || firstStyleID != model.Entries[1].HighlightStyleID {
		t.Fatalf("base styles were not deduplicated: %#v", model.Entries)
	}
	style := model.HighlightStyles[firstStyleID]
	if style.Icon != "qrc:/F4QtHost/icons/lucide/file-text.svg" ||
		style.Normal.Foreground != "#123456" {
		t.Fatalf("unexpected provisional style: %#v", style)
	}
	chunk := semanticMetadataChunkForModel(t, model)
	if appInt64(chunk["highlightRevision"]) != model.HighlightRevision {
		t.Fatalf("deferred highlight revision differs from base: %#v", chunk)
	}
	entries := appMapSlice(chunk["entries"])
	resolvedStyleID := semanticString(entries[0]["highlightStyleId"])
	if resolvedStyleID == "" || resolvedStyleID != semanticString(entries[1]["highlightStyleId"]) {
		t.Fatalf("styles were not deduplicated: %#v", entries)
	}
	styles := appMap(chunk["highlightStyles"])
	resolvedStyle := appMap(styles[resolvedStyleID])
	if semanticString(resolvedStyle["icon"]) != "qrc:/F4QtHost/icons/lucide/file-text.svg" ||
		semanticString(appMap(resolvedStyle["normal"])["foreground"]) != "#123456" {
		t.Fatalf("unexpected normalized style: %#v", resolvedStyle)
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
	if changed.CatalogRevision != selected.CatalogRevision || changed.MetadataRevision != selected.MetadataRevision+1 {
		t.Fatalf("mtime change touched wrong revisions: selected=(%d,%d) changed=(%d,%d)",
			selected.CatalogRevision, selected.MetadataRevision, changed.CatalogRevision, changed.MetadataRevision)
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

func TestFileSystemPanelSemanticVFSEnablesBrokerPreview(t *testing.T) {
	remote := vfs.NewNullVFS(0)
	fp := &FileSystemPanel{
		vfs:           remote,
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "/"),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		selectedItems: make(map[string]bool),
		entries:       []*fileEntry{{VFSItem: vfs.VFSItem{Name: "remote.jpg", Size: 1}}},
	}
	model := fp.semanticPanelModel(nil, 0, true)
	if model.SourceKind != "vfs" || !model.PreviewCapable {
		t.Fatalf("unexpected remote capability model: %+v", model)
	}
	if model.Entries[0].LocalPath != "" || model.Entries[0].Path == "" ||
		model.Entries[0].EntryID == "" || !model.Entries[0].IsImage ||
		model.Entries[0].Source == nil || model.Entries[0].Source.SourceKey == "" {
		t.Fatalf("unexpected remote entry metadata: %+v", model.Entries[0])
	}
	metadata := appMapSlice(semanticMetadataChunkForModel(t, model)["entries"])
	if metadata[0]["localPath"] != "" {
		t.Fatalf("remote entry unexpectedly resolved a local path: %#v", metadata[0])
	}
}

func TestSemanticBaseCatalogIgnoresAllDeferredMetadata(t *testing.T) {
	previousHighlighter := GlobalFileHighlighter
	GlobalFileHighlighter = &FileHighlighter{}
	defer func() { GlobalFileHighlighter = previousHighlighter }()

	// Loaded once, up front: a name-mask rule like this one is resolved by
	// the fast base pass (see FileHighlighter.SemanticStyle's metadataKnown
	// parameter), so its highlightStyleId is part of the base/minimal
	// catalog from the first call and must stay identical across both calls
	// below. Only the deferred per-file fields mutated between the two
	// calls are under test here.
	GlobalFileHighlighter.LoadFromIni(ParseIni(strings.NewReader(`
[Highlight_0]
Name = Photos
Mask = *.jpg
NormalColor = foreground:#ABCDEF
`)))

	tmp := t.TempDir()
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		table:         vtui.NewTable(0, 0, 40, 10, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{{VFSItem: vfs.VFSItem{
			Name: "photo.jpg", Size: 10, MTime: time.Unix(10, 20),
			Mode: "-rw-r--r--", Uid: 1, Gid: 2,
		}}},
	}
	first := fp.semanticPanelModel(nil, 0, true)
	firstBase := first.ToMap()["entries"]

	fp.entries[0].Size = 999
	fp.entries[0].PhysicalSize = 4096
	fp.entries[0].MTime = time.Unix(30, 40)
	fp.entries[0].ATime = time.Unix(31, 41)
	fp.entries[0].CTime = time.Unix(32, 42)
	fp.entries[0].Mode = "-rwx------"
	fp.entries[0].UnixMode = 0o700
	fp.entries[0].Uid = 42
	fp.entries[0].Gid = 43
	fp.entries[0].WinAttrs = 7
	fp.entries[0].IsExecutable = true
	fp.entries[0].SizeCalculated = true

	second := fp.semanticPanelModel(nil, 0, true)
	if second.CatalogRevision != first.CatalogRevision {
		t.Fatalf("deferred fields advanced catalog revision: first=%d second=%d",
			first.CatalogRevision, second.CatalogRevision)
	}
	if second.MetadataRevision != first.MetadataRevision+1 {
		t.Fatalf("deferred fields did not advance metadata revision: first=%d second=%d",
			first.MetadataRevision, second.MetadataRevision)
	}
	if !reflect.DeepEqual(firstBase, second.ToMap()["entries"]) {
		t.Fatalf("minimal entries changed with deferred metadata:\nfirst=%#v\nsecond=%#v",
			firstBase, second.ToMap()["entries"])
	}
	chunk := semanticMetadataChunkForModel(t, second)
	entry := appMapSlice(chunk["entries"])[0]
	if appInt64(entry["size"]) != 999 || semanticString(entry["mode"]) != "-rwx------" ||
		semanticString(entry["highlightStyleId"]) == "" {
		t.Fatalf("deferred mutation missing from metadata chunk: %#v", entry)
	}
	if _, duplicated := entry["isHidden"]; duplicated {
		t.Fatalf("base hidden state was duplicated in metadata: %#v", entry)
	}
	for _, redundant := range []string{"physicalSize", "isExecutable", "sizeCalculated", "atimeNanos", "ctimeNanos", "version", "revision", "unixMode", "uid", "gid", "winAttrs"} {
		if _, present := entry[redundant]; present {
			t.Fatalf("redundant metadata field %q was serialized: %#v", redundant, entry)
		}
	}

	fp.entries[0].IsHidden = true
	third := fp.semanticPanelModel(nil, 0, true)
	if third.CatalogRevision != second.CatalogRevision+1 {
		t.Fatalf("hidden state did not advance catalog revision: second=%d third=%d",
			second.CatalogRevision, third.CatalogRevision)
	}
	if third.MetadataRevision != second.MetadataRevision {
		t.Fatalf("hidden state incorrectly advanced metadata revision: second=%d third=%d",
			second.MetadataRevision, third.MetadataRevision)
	}
	thirdEntry := appMapSlice(third.ToMap()["entries"])[0]
	if thirdEntry["isHidden"] != true {
		t.Fatalf("authoritative hidden state missing from base catalog: %#v", thirdEntry)
	}
	if metadataEntry := appMapSlice(semanticMetadataChunkForModel(t, third)["entries"])[0]; metadataEntry["isHidden"] != nil {
		t.Fatalf("hidden state was redundantly deferred: %#v", metadataEntry)
	}
}

func TestPanelCatalogMetadataChunksAreOrderedBoundedAndRejectStaleRequests(t *testing.T) {
	previousCapability := setExtUiPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogMetadataEnabled(previousCapability) })
	tmp := t.TempDir()
	entries := make([]*fileEntry, 0, 205)
	for i := 0; i < 205; i++ {
		entries = append(entries, &fileEntry{VFSItem: vfs.VFSItem{
			Name: fmt.Sprintf("entry-%03d.txt", i), Size: int64(i),
		}})
	}
	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		table:         vtui.NewTable(0, 0, 40, 10, nil),
		selectedItems: make(map[string]bool),
		entries:       entries,
	}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)
	first := fp.semanticPanelModel(nil, 0, true)
	defaultChunk, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 0)
	if !ok || defaultChunk["limit"] != 8 || len(appMapSlice(defaultChunk["entries"])) != 8 {
		t.Fatalf("unexpected default metadata chunk: %#v", defaultChunk)
	}
	firstChunk, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 64)
	if !ok || firstChunk["offset"] != 0 || firstChunk["limit"] != 64 ||
		firstChunk["total"] != 205 || firstChunk["final"] != false {
		t.Fatalf("unexpected first metadata chunk: %#v", firstChunk)
	}
	firstRows := appMapSlice(firstChunk["entries"])
	if len(firstRows) != 64 || semanticString(firstRows[0]["entryId"]) != first.Entries[0].EntryID ||
		semanticString(firstRows[63]["entryId"]) != first.Entries[63].EntryID {
		t.Fatalf("first chunk order mismatch: %#v", firstRows)
	}

	lastChunk, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 192, 999)
	if !ok || lastChunk["limit"] != 128 || lastChunk["final"] != true ||
		len(appMapSlice(lastChunk["entries"])) != 13 {
		t.Fatalf("unexpected last metadata chunk: %#v", lastChunk)
	}

	// The raw entry can mutate before the next scene is published without
	// changing what an already-acknowledged revision returns.
	fp.entries[0].Size = 9999
	repeat, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 1)
	if !ok || appInt64(appMapSlice(repeat["entries"])[0]["size"]) != -1 {
		t.Fatalf("published metadata snapshot was not immutable: %#v", repeat)
	}

	second := fp.semanticPanelModel(nil, 0, true)
	if _, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 1); ok {
		t.Fatal("metadata-only stale request was accepted")
	}
	fp.entries[0].Name = "renamed.txt"
	third := fp.semanticPanelModel(nil, 0, true)
	if third.CatalogRevision != second.CatalogRevision+1 {
		t.Fatalf("rename did not advance catalog revision: second=%d third=%d",
			second.CatalogRevision, third.CatalogRevision)
	}
	if _, ok := BuildPanelCatalogMetadataChunk(second.ID, second.Path,
		second.CatalogRevision, second.MetadataRevision, 0, 1); ok {
		t.Fatal("catalog-stale metadata request was accepted")
	}
	if _, ok := BuildPanelCatalogMetadataChunk(third.ID, third.Path+"-stale",
		third.CatalogRevision, third.MetadataRevision, 0, 1); ok {
		t.Fatal("path-stale metadata request was accepted")
	}
	if _, ok := BuildPanelCatalogMetadataChunk(third.ID, third.Path,
		third.CatalogRevision, third.MetadataRevision, -1, 1); ok {
		t.Fatal("negative metadata offset was accepted")
	}
}

func TestDeferredMetadataDistinguishesUnknownAndKnownEmptyFileSizes(t *testing.T) {
	previousCapability := setExtUiPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogMetadataEnabled(previousCapability) })

	fp := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(t.TempDir()),
		table:         vtui.NewTable(0, 0, 40, 10, nil),
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "pending.txt"}},
			{VFSItem: vfs.VFSItem{Name: "empty.txt", SizeKnown: true}},
			{VFSItem: vfs.VFSItem{Name: "payload.txt", Size: 42}},
		},
	}

	provisional := fp.semanticPanelModel(nil, 0, true)
	rows := appMapSlice(semanticMetadataChunkForModel(t, provisional)["entries"])
	if got := appInt64(rows[0]["size"]); got != -1 || semanticString(rows[0]["sizeText"]) != "" {
		t.Fatalf("unknown size was serialized as a real value: %#v", rows[0])
	}
	if got := appInt64(rows[1]["size"]); got != 0 || semanticString(rows[1]["sizeText"]) != "0" {
		t.Fatalf("known empty file lost its zero size: %#v", rows[1])
	}
	if got := appInt64(rows[2]["size"]); got != 42 || semanticString(rows[2]["sizeText"]) != "42" {
		t.Fatalf("legacy non-zero size was not treated as known: %#v", rows[2])
	}

	// The metadata phase can resolve an unknown row to an actual zero-byte
	// file. That transition must advance only MetadataRevision, otherwise the
	// frontend would retain the provisional blank/zero snapshot forever.
	fp.entries[0].SizeKnown = true
	resolved := fp.semanticPanelModel(nil, 0, true)
	if resolved.CatalogRevision != provisional.CatalogRevision ||
		resolved.MetadataRevision != provisional.MetadataRevision+1 {
		t.Fatalf("known-zero enrichment changed wrong revisions: before=(%d,%d) after=(%d,%d)",
			provisional.CatalogRevision, provisional.MetadataRevision,
			resolved.CatalogRevision, resolved.MetadataRevision)
	}
	resolvedRows := appMapSlice(semanticMetadataChunkForModel(t, resolved)["entries"])
	if got := appInt64(resolvedRows[0]["size"]); got != 0 || semanticString(resolvedRows[0]["sizeText"]) != "0" {
		t.Fatalf("resolved empty file was not published: %#v", resolvedRows[0])
	}
}

func TestPanelsFrameCloseUnpublishesOnlyOwnedSemanticMetadataSnapshot(t *testing.T) {
	newPanel := func() (*FileSystemPanel, extui.PanelModel) {
		fp := &FileSystemPanel{
			vfs:           vfs.NewOSVFS(t.TempDir()),
			table:         vtui.NewTable(0, 0, 40, 10, nil),
			selectedItems: make(map[string]bool),
			entries: []*fileEntry{{VFSItem: vfs.VFSItem{
				Name: "entry.txt", Size: 1,
			}}},
		}
		return fp, fp.semanticPanelModel(nil, 0, true)
	}

	owned, ownedModel := newPanel()
	ownedSnapshot := owned.semanticMetadataSnapshot
	(&PanelsFrame{panels: [2]Panel{owned, nil}}).Close()
	if _, present := semanticPanelMetadataSnapshots.Load(ownedModel.ID); present {
		t.Fatal("workspace close retained its owned semantic metadata snapshot")
	}

	stale, staleModel := newPanel()
	replacement := &semanticPanelMetadataSnapshot{panelID: staleModel.ID, path: "newer"}
	semanticPanelMetadataSnapshots.Store(staleModel.ID, replacement)
	t.Cleanup(func() {
		semanticPanelMetadataSnapshots.CompareAndDelete(staleModel.ID, replacement)
	})
	(&PanelsFrame{panels: [2]Panel{stale, nil}}).Close()
	loaded, present := semanticPanelMetadataSnapshots.Load(staleModel.ID)
	if !present || loaded != replacement {
		t.Fatalf("stale panel removed newer snapshot: present=%v loaded=%p replacement=%p owned=%p",
			present, loaded, replacement, ownedSnapshot)
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
		"index":    float64(1),
		"activate": true,
	}) {
		t.Fatal("atomic cursor-and-activate action was not handled")
	}
	if pf.activeIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.activeIdx)
	}
	if right.GetCursorIndex() != 1 {
		t.Fatalf("right cursor = %d, want 1", right.GetCursorIndex())
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

func TestPanelsFrameSemanticActivationNotifiesRenderer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	renderer := &searchFirstActivationRenderer{side: -1}
	vtui.FrameManager.Screen().Renderer = renderer

	tmp := t.TempDir()
	left := &FileSystemPanel{
		vfs:           vfs.NewOSVFS(tmp),
		frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		table:         vtui.NewTable(1, 1, 38, 6, nil),
		viewMode:      ViewModeDetailed,
		selectedItems: make(map[string]bool),
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "left.txt", Size: 10}},
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
			{VFSItem: vfs.VFSItem{Name: "right.txt", Size: 20}},
		},
	}
	pf := &PanelsFrame{
		panels:         [2]Panel{left, right},
		activeIdx:      1,
		showPanels:     true,
		showLeftPanel:  true,
		showRightPanel: true,
		cmdLine:        NewCommandLine("$ "),
		termView:       NewTerminalView(80, 24),
	}
	vtui.FrameManager.Push(pf)

	// 1. panel.activate on left panel (side 0)
	renderer.calls = 0
	renderer.side = -1
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.activate",
		"side":   0,
	}) {
		t.Fatal("panel.activate on left panel was not handled")
	}
	if pf.activeIdx != 0 {
		t.Fatalf("activeIdx = %d, want 0", pf.activeIdx)
	}
	if renderer.calls != 1 || renderer.side != 0 {
		t.Fatalf("panel.activate did not notify renderer: calls=%d side=%d", renderer.calls, renderer.side)
	}

	// Redundant activate on already active panel should NOT re-notify renderer
	renderer.calls = 0
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.activate",
		"side":   0,
	}) {
		t.Fatal("redundant panel.activate was not handled")
	}
	if renderer.calls != 0 {
		t.Fatalf("redundant panel.activate notified renderer: calls=%d", renderer.calls)
	}

	// 2. panel.cursor with activate: true on right panel (side 1)
	renderer.calls = 0
	renderer.side = -1
	rightModel := right.semanticPanelModel(nil, 1, true)
	if !pf.HandleSemanticAction(map[string]any{
		"action":          "panel.cursor",
		"side":            1,
		"entryId":         rightModel.Entries[1].EntryID,
		"index":           1,
		"catalogRevision": rightModel.CatalogRevision,
		"activate":        true,
	}) {
		t.Fatal("panel.cursor with activate was not handled")
	}
	if pf.activeIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.activeIdx)
	}
	if renderer.calls != 1 || renderer.side != 1 {
		t.Fatalf("panel.cursor with activate did not notify renderer: calls=%d side=%d", renderer.calls, renderer.side)
	}

	// 3. panel.cursor with activate: true and stale/mismatched revision should STILL activate panel
	renderer.calls = 0
	renderer.side = -1
	if !pf.HandleSemanticAction(map[string]any{
		"action":          "panel.cursor",
		"side":            0,
		"entryId":         "stale-entry-id",
		"index":           999,
		"catalogRevision": int64(99999),
		"activate":        true,
	}) {
		t.Fatal("panel.cursor with stale revision and activate was not handled")
	}
	if pf.activeIdx != 0 {
		t.Fatalf("activeIdx = %d, want 0", pf.activeIdx)
	}
	if renderer.calls != 1 || renderer.side != 0 {
		t.Fatalf("stale panel.cursor with activate did not notify renderer: calls=%d side=%d", renderer.calls, renderer.side)
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

	transactionBase := left.semanticPanelModel(nil, 0, true)
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.setSelection",
		"side":   0,
		"mode":   "set",
		"changes": []any{
			map[string]any{"entryId": alphaID, "selected": false},
			map[string]any{"entryId": betaID, "selected": true},
		},
		"cursorEntryId":     alphaID,
		"cursorIndex":       1,
		"catalogRevision":   transactionBase.CatalogRevision,
		"selectionRevision": transactionBase.SelectionRevision,
	}) || left.entries[1].Selected || !left.entries[2].Selected ||
		left.GetCursorIndex() != 1 {
		t.Fatal("atomic selection/cursor transaction was not applied")
	}

	// Validate every stable identity before mutating either half. A bad final
	// cursor must not leave an otherwise-valid selection change behind.
	beforeRejected := left.entries[1].Selected
	fresh := left.semanticPanelModel(nil, 0, true)
	if pf.HandleSemanticAction(map[string]any{
		"action": "panel.setSelection",
		"side":   0,
		"mode":   "set",
		"changes": []any{
			map[string]any{"entryId": alphaID, "selected": !beforeRejected},
		},
		"cursorEntryId":     "missing",
		"catalogRevision":   fresh.CatalogRevision,
		"selectionRevision": fresh.SelectionRevision,
	}) || left.entries[1].Selected != beforeRejected {
		t.Fatal("invalid atomic selection cursor partially mutated selection")
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
		columns.GalleryDensity != 0 ||
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

	// Compact density changes are independent layout-only state, just like
	// image-centric modes, and remain bounded at the semantic boundary.
	left.galleryDensities[GalleryLayoutDetails] = 47
	detailsRevision := left.galleryLayoutRevision
	if !pf.HandleSemanticAction(map[string]any{
		"action":     "panel.setGalleryDensity",
		"side":       0,
		"layoutMode": "details",
		"density":    31,
	}) || left.galleryDensity(GalleryLayoutDetails) != 31 {
		t.Fatal("Details density action was not applied")
	}
	if details := left.semanticPanelModel(nil, 0, true); details.GalleryDensity != 0 || details.GalleryDensities["details"] != 31 {
		t.Fatalf("Details density was not exported in bounded layout state: %#v",
			details.GalleryDensities)
	}
	if left.galleryLayoutRevision != detailsRevision+1 {
		t.Fatalf("Details density revision = %d, want %d",
			left.galleryLayoutRevision, detailsRevision+1)
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":     "panel.resetGalleryDensity",
		"side":       0,
		"layoutMode": "details",
	}) || left.galleryDensity(GalleryLayoutDetails) != 0 {
		t.Fatal("Details density reset did not restore the host default")
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

func TestSemanticGalleryLayoutPersistsEachPanelIndependently(t *testing.T) {
	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	pf := &PanelsFrame{panels: [2]Panel{left, right}, activeIdx: 0}

	originalPersist := persistNativePanelLayoutSession
	defer func() { persistNativePanelLayoutSession = originalPersist }()
	var snapshots []workspaceSessionState
	persistNativePanelLayoutSession = func(got *PanelsFrame) {
		if got != pf {
			t.Fatalf("persisted unexpected PanelsFrame %p, want %p", got, pf)
		}
		snapshots = append(snapshots, captureWorkspaceSession(got))
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.setGalleryLayout", "side": 0,
		"layoutMode": "icons",
	}) {
		t.Fatal("left native layout was rejected")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.setGalleryLayout", "side": 1,
		"layoutMode": "details",
	}) {
		t.Fatal("right native layout was rejected")
	}
	if len(snapshots) != 2 {
		t.Fatalf("persistence calls = %d, want 2", len(snapshots))
	}
	latest := snapshots[1]
	if latest.Left.Gallery.LayoutMode != GalleryLayoutIcons ||
		latest.Right.Gallery.LayoutMode != GalleryLayoutDetails {
		t.Fatalf("per-panel native layouts were conflated: left=%#v right=%#v",
			latest.Left.Gallery, latest.Right.Gallery)
	}

	// An acknowledgement of the already-saved mode must not rewrite the
	// session file, while a per-mode zoom preference must be persisted.
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.setGalleryLayout", "side": 1,
		"layoutMode": "details",
	}) || len(snapshots) != 2 {
		t.Fatalf("unchanged layout triggered persistence: calls=%d", len(snapshots))
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.setGalleryDensity", "side": 0,
		"layoutMode": "icons", "density": 96,
	}) {
		t.Fatal("left native density was rejected")
	}
	if len(snapshots) != 3 ||
		snapshots[2].Left.Gallery.Densities[GalleryLayoutIcons] != 96 ||
		snapshots[2].Right.Gallery.LayoutMode != GalleryLayoutDetails {
		t.Fatalf("native density persistence lost panel identity: %#v", snapshots)
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

func TestPanelsFrameSemanticPanelDriveMenu(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	pf := &PanelsFrame{panels: [2]Panel{left, right}, activeIdx: 1}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.driveMenu",
		"side":   0,
	}) {
		t.Fatal("drive menu action was rejected")
	}
	if pf.activeIdx != 0 {
		t.Fatalf("drive menu did not activate requested panel: %d", pf.activeIdx)
	}
	if menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu); !ok {
		t.Fatalf("drive menu action opened %T instead of VMenu",
			vtui.FrameManager.GetTopFrame())
	} else if menu.GetTitle() != Msg("Drive.Title") {
		t.Fatalf("drive menu title = %q, want %q", menu.GetTitle(), Msg("Drive.Title"))
	} else {
		if len(menu.Items) == 0 || menu.Items[0].Icon != driveMenuIconOtherPanel {
			t.Fatalf("other-panel drive item lacks its semantic icon: %#v", menu.Items)
		}
		for _, item := range menu.Items {
			if !item.Separator && item.Icon == "" {
				t.Fatalf("drive item %q lacks a semantic icon", item.Text)
			}
		}
	}
	vtui.FrameManager.Pop()
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
	metadataOnly := panel.semanticPanelModel(nil, 0, false)
	if metadataOnly.CatalogRevision != stale.CatalogRevision ||
		metadataOnly.MetadataRevision != stale.MetadataRevision+1 {
		t.Fatalf("metadata-only mutation changed wrong revision domain: stale=(%d,%d) current=(%d,%d)",
			stale.CatalogRevision, stale.MetadataRevision,
			metadataOnly.CatalogRevision, metadataOnly.MetadataRevision)
	}
	panel.entries[1].Name = "renamed-decoy"
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

func TestPanelsFrameSemanticGridFallbackForUnsupportedPanelLayouts(t *testing.T) {
	pf := &PanelsFrame{
		showPanels: true, showLeftPanel: true, showRightPanel: true,
		lastW: 100, lastH: 30,
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("default panel layout unexpectedly requires fallback: %q", reason)
	}

	pf.widthDecrement = 6
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("native horizontal split unexpectedly requires fallback: %q", reason)
	}
	layout := pf.semanticPanelLayoutModel(&vtui.SemanticContext{Width: 100, Height: 30})
	if layout.Columns != 100 || layout.SplitColumn != 44 ||
		layout.LeftBottomInsetRows != 0 || layout.RightBottomInsetRows != 0 {
		t.Fatalf("horizontal split layout = %#v, want 100 columns at split 44", layout)
	}
	pf.widthDecrement = 0
	pf.leftHeightDecrement = 1
	if reason := pf.semanticGridFallbackReason(); !strings.Contains(reason, "shortened-panel") {
		t.Fatalf("shortened panel fallback reason = %q", reason)
	}
	pf.leftHeightDecrement = 0
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
	defer ev.Close()
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
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmp)
	viewer, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("failed to create viewer: %v", err)
	}
	defer viewer.Close()

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
	if got := semanticString(node["topBarLeft"]); got != " window.txt" {
		t.Fatalf("viewer top bar left=%q, want %q", got, " window.txt")
	}
	viewerStatus := semanticString(node["topBarRight"])
	if !strings.Contains(viewerStatus, vfs.DisplayCodepageName(viewer.Codepage)) ||
		!strings.Contains(viewerStatus, "%") {
		t.Fatalf("viewer top bar right=%q does not contain codepage and progress", viewerStatus)
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
	content := strings.Repeat("x", 64*1024) + "\nnext\n"
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
	// NewViewerView deliberately seeds the first 16 KiB encoding-probe prefix
	// into the bounded backend cache. Seek beyond it to exercise async loading.
	targetRow := 2048
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
	if got := semanticString(node["topBarLeft"]); got != " window.txt" {
		t.Fatalf("editor top bar left=%q, want %q", got, " window.txt")
	}
	editorStatus := semanticString(node["topBarRight"])
	if !strings.Contains(editorStatus, vfs.DisplayCodepageName(ev.Codepage)) ||
		!strings.Contains(editorStatus, "Ln  45/") ||
		!strings.Contains(editorStatus, "Col   3") {
		t.Fatalf("editor top bar right=%q does not contain codepage and cursor", editorStatus)
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
	pf := setupMockPanelsFrame(t)
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

	pf := setupMockPanelsFrame(t)
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
	pf := setupMockPanelsFrame(t)
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
