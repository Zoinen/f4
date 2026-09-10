package panel

import (
	context "context"
	fmt "fmt"
	appcmd "github.com/unxed/f4/internal/appcmd"
	cmdline "github.com/unxed/f4/internal/cmdline"
	config "github.com/unxed/f4/internal/config"
	i18n "github.com/unxed/f4/internal/i18n"
	ini "github.com/unxed/f4/internal/ini"
	semantic "github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/sysinfo"
	terminal "github.com/unxed/f4/internal/terminal"
	theme "github.com/unxed/f4/internal/theme"
	extui "github.com/unxed/f4/sdk/extui"
	vfs "github.com/unxed/f4/vfs"
	vtui "github.com/unxed/vtui"
	os "os"
	filepath "path/filepath"
	reflect "reflect"
	strings "strings"
	testing "testing"
	time "time"
)

func TestFileSystemPanelSemanticPanelNode(t *testing.T) {
	tmp := t.TempDir()
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		currentTitle:  "Friendly panel path",
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      ViewModeDetailed,
		SortMode:      SortSize,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true, Mode: "drwxr-xr-x"}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 1234, MTime: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), Mode: "-rw-r--r--", IsHidden: true}, Selected: true},
		},
	}
	fp.SetCanFocus(true)
	fp.SetPosition(0, 0, 39, 9)
	fp.Table.Columns = []vtui.TableColumn{
		{Title: "Name", Width: 27},
		{Title: "Size ↓", Width: 10},
	}
	fp.SetCursorIndex(1)
	fp.FastFindMode = true
	fp.FastFindStr = "*pha"

	model := fp.SemanticPanelModel(&vtui.SemanticContext{Width: 80, Height: 25}, 0, true)
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
	if node["fastFindMatchColor"] != semantic.SemanticAttrColor(vtui.Palette[theme.ColPanelHighlightText], true) {
		t.Fatalf("quick-search color = %v, want active Panel.Text.Highlight", node["fastFindMatchColor"])
	}
	matches := semantic.AppMap(node["fastFindMatches"])
	match := semantic.AppMap(matches[semantic.String(entries[1]["entryId"])])
	if semantic.Int(match["start"]) != 2 || semantic.Int(match["length"]) != 3 {
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
	for _, deferred := range []string{"highlightRevision"} {
		if _, present := node[deferred]; present {
			t.Fatalf("deferred panel field %q leaked into base scene: %#v", deferred, node)
		}
	}
	// Aggregate sizes are part of the panel status, not per-entry lazy metadata.
	if semantic.Int(node["selectedSize"]) != 1234 || semantic.Int(node["totalSize"]) != 1234 {
		t.Fatalf("panel status sizes = %#v", node)
	}
	styles := semantic.AppMap(node["highlightStyles"])
	referencedStyles := make(map[string]bool)
	for _, entry := range entries {
		styleID := semantic.String(entry["highlightStyleId"])
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
	metadataEntries := semantic.AppMapSlice(chunk["entries"])
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

func TestPanelsFrameSemanticActionAcceptsQMLNumbers(t *testing.T) {
	tmp := t.TempDir()
	left := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 12}},
			{VFSItem: vfs.VFSItem{Name: "beta.txt", Size: 34}},
		},
	}
	right := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(40, 0, 79, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(41, 1, 78, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "right.txt", Size: 56}},
		},
	}
	pf := &PanelsFrame{
		Panels:    [2]Panel{left, right},
		ActiveIdx: 0,
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
	if pf.ActiveIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.ActiveIdx)
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
	if pf.ActiveIdx != 0 {
		t.Fatalf("activeIdx = %d, want 0", pf.ActiveIdx)
	}
}

type semanticBlockingLocalVFS struct {
	*vfs.OSVFS
	readStarted chan struct{}
	readDone    chan struct{}
}

type semanticExpensiveReader struct{ vfs.ReadAtCloser }

func (semanticExpensiveReader) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessUnknownExpensive
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
		Vfs:           vfs.NewOSVFS(t.TempDir()),
		Table:         vtui.NewTable(0, 0, 40, 10, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "main.go", Size: 10}},
			{VFSItem: vfs.VFSItem{Name: "README.md", Size: 20}},
		},
	}

	first := panel.SemanticPanelModel(nil, 0, true)
	cache := panel.semanticStaticCache
	if cache == nil || len(first.Entries) != 2 {
		t.Fatal("semantic static catalog was not initialized")
	}

	panel.Entries[1].Selected = true
	second := panel.SemanticPanelModel(nil, 0, false)
	if panel.semanticStaticCache != cache {
		t.Fatal("focus/selection-only change rebuilt the static catalog")
	}
	if second.Active || !second.Entries[1].Selected || second.SelectedCount != 1 {
		t.Fatalf("dynamic panel state was not refreshed: %#v", second)
	}

	panel.Entries[0].Name = "changed.go"
	third := panel.SemanticPanelModel(nil, 0, false)
	if panel.semanticStaticCache == cache {
		t.Fatal("catalog mutation did not invalidate the static cache")
	}
	if third.Entries[0].Name != "changed.go" {
		t.Fatalf("rebuilt catalog contains stale entry: %#v", third.Entries[0])
	}
}

func TestSemanticPanelFileInfoSettingIsBoundedDynamicState(t *testing.T) {
	previousCapability := semantic.SetPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogMetadataEnabled(previousCapability) })
	previousSetting := config.App.ShowPanelFileInfo
	t.Cleanup(func() { config.App.ShowPanelFileInfo = previousSetting })

	panel := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(t.TempDir()),
		Table:         vtui.NewTable(0, 0, 40, 10, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "one.txt", Size: 1}},
			{VFSItem: vfs.VFSItem{Name: "two.txt", Size: 2}},
		},
	}
	t.Cleanup(panel.unpublishSemanticMetadataSnapshot)

	config.App.ShowPanelFileInfo = false
	full := panel.SemanticPanelModel(nil, 0, true)
	if full.ShowFileInfo {
		t.Fatal("disabled file-information setting was exported as enabled")
	}

	config.App.ShowPanelFileInfo = true
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

	fp.IsLoading = true
	fp.loadingFrame = 3
	fp.updateTitle(nil)
	if !strings.HasSuffix(fp.currentTitle, panelLoadingPulse[3]) {
		t.Fatalf("TUI title %q has no loading pulse", fp.currentTitle)
	}
	model := fp.SemanticPanelModel(nil, 0, true)
	if model.Title != fp.Vfs.GetPath() {
		t.Fatalf("semantic title = %q, want clean path %q", model.Title, fp.Vfs.GetPath())
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

func TestSemanticPagedPanelExportsViewportAndServesOnlyRequestedRows(t *testing.T) {
	previousRowsCapability := semantic.SetPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogRowsEnabled(previousRowsCapability) })

	directory := t.TempDir()
	const entryCount = 30_000
	entries := make([]*FileEntry, entryCount)
	for index := range entries {
		entries[index] = &FileEntry{VFSItem: vfs.VFSItem{
			Name:  fmt.Sprintf("entry-%05d.txt", index),
			IsDir: index%7 == 0,
		}}
	}
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(directory),
		Table:         vtui.NewTable(0, 0, 80, 40, nil),
		SelectedItems: make(map[string]bool),
		Entries:       entries,
	}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)
	fp.SetCursorIndex(15_000)

	model := fp.SemanticPanelModel(
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
	page := semantic.AppMapSlice(response["entries"])
	if len(page) != maxPanelCatalogRowsLimit {
		t.Fatalf("requested page = %d rows, want %d",
			len(page), maxPanelCatalogRowsLimit)
	}
	if semantic.Int(page[0]["index"]) != offset ||
		semantic.String(page[0]["name"]) != "entry-12345.txt" {
		t.Fatalf("first requested row = %#v", page[0])
	}
	if _, ok := BuildLivePanelCatalogRows(
		model.ID, directory, model.CatalogRevision-1, offset, 1); ok {
		t.Fatal("stale catalog revision unexpectedly served rows")
	}
}

func TestSemanticPagedPanelServesPendingAuthoritativeSource(t *testing.T) {
	previousRowsCapability := semantic.SetPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogRowsEnabled(previousRowsCapability) })

	const sourceCount = 96
	directory := t.TempDir()
	fullSource := make([]*FileEntry, sourceCount)
	for index := range fullSource {
		fullSource[index] = &FileEntry{VFSItem: vfs.VFSItem{
			Name: fmt.Sprintf("image-%03d.jpg", index),
		}}
	}
	prefix := make([]*FileEntry, 0, initialPanelCatalogRowsLimit+1)
	prefix = append(prefix, &FileEntry{VFSItem: vfs.VFSItem{
		Name: "..", IsDir: true,
	}})
	prefix = append(prefix, fullSource[:initialPanelCatalogRowsLimit-1]...)
	panel := &FileSystemPanel{
		Vfs:                 vfs.NewOSVFS(directory),
		Table:               vtui.NewTable(0, 0, 80, 40, nil),
		SelectedItems:       make(map[string]bool),
		Entries:             prefix,
		catalogLogicalCount: sourceCount + 1,
		IsLoading:           true,
		loadGeneration:      9,
	}
	t.Cleanup(panel.unpublishSemanticMetadataSnapshot)
	panel.resetSemanticPendingSource(panel.loadGeneration)
	panel.setSemanticPendingSource(panel.loadGeneration, fullSource, true)

	model := panel.SemanticPanelModel(nil, 0, true)
	if model.TotalCount != sourceCount+1 {
		t.Fatalf("pending source total = %d, want %d", model.TotalCount, sourceCount+1)
	}
	const offset = initialPanelCatalogRowsLimit + 7
	response, ok := BuildLivePanelCatalogRows(
		model.ID, directory, model.CatalogRevision, offset, 16)
	if !ok {
		t.Fatalf("pending authoritative source rejected row request at %d", offset)
	}
	rows := semantic.AppMapSlice(response["entries"])
	if len(rows) != 16 || semantic.String(rows[0]["name"]) != "image-054.jpg" {
		t.Fatalf("pending source page = %d rows, first=%q",
			len(rows), semantic.String(rows[0]["name"]))
	}
}

func TestSemanticPagedPanelUsesDenseCatalogForSettledSmallDirectory(t *testing.T) {
	previousRowsCapability := semantic.SetPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogRowsEnabled(previousRowsCapability) })

	const imageCount = 297
	directory := t.TempDir()
	entries := make([]*FileEntry, 0, imageCount+1)
	entries = append(entries, &FileEntry{VFSItem: vfs.VFSItem{
		Name: "..", IsDir: true,
	}})
	for index := 0; index < imageCount; index++ {
		entries = append(entries, &FileEntry{VFSItem: vfs.VFSItem{
			Name: fmt.Sprintf("image-%03d.jpg", index),
		}})
	}
	panel := &FileSystemPanel{
		Vfs:               vfs.NewOSVFS(directory),
		Table:             vtui.NewTable(0, 0, 80, 40, nil),
		SelectedItems:     make(map[string]bool),
		Entries:           entries,
		GalleryLayoutMode: GalleryLayoutMasonry,
	}
	t.Cleanup(panel.unpublishSemanticMetadataSnapshot)

	model := panel.SemanticPanelModel(nil, 1, false)
	if model.CatalogRowsDeferred {
		t.Fatal("settled small catalog was left in sparse mode")
	}
	if model.TotalCount != imageCount+1 || len(model.Entries) != imageCount+1 {
		t.Fatalf("dense catalog = %d/%d rows, want %d/%d",
			len(model.Entries), model.TotalCount, imageCount+1, imageCount+1)
	}
	if model.Entries[0].Name != ".." ||
		model.Entries[len(model.Entries)-1].Name != "image-296.jpg" {
		t.Fatalf("dense catalog endpoints = %q/%q",
			model.Entries[0].Name, model.Entries[len(model.Entries)-1].Name)
	}
}

func TestSemanticPagedFastFindHeaderMatchesOnlyViewportWindow(t *testing.T) {
	previousRowsCapability := semantic.SetPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogRowsEnabled(previousRowsCapability) })

	directory := t.TempDir()
	const entryCount = 30_000
	const cursor = 15_000
	entries := make([]*FileEntry, entryCount)
	for index := range entries {
		entries[index] = &FileEntry{VFSItem: vfs.VFSItem{
			Name: fmt.Sprintf("entry-%05d.txt", index),
		}}
	}
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(directory),
		Table:         vtui.NewTable(0, 0, 80, 40, nil),
		SelectedItems: make(map[string]bool),
		Entries:       entries,
	}
	fp.SetCursorIndex(cursor)
	fp.FastFindMode = true
	fp.FastFindStr = "entry"

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
	fp := &FileSystemPanel{GalleryLayoutMode: GalleryLayoutDetails}
	if got := fp.semanticFastFindWindowLimit(); got != semanticFastFindDetailsRowsLimit {
		t.Fatalf("details fast-find window = %d, want %d",
			got, semanticFastFindDetailsRowsLimit)
	}
	fp.GalleryLayoutMode = GalleryLayoutGrid
	if got := fp.semanticFastFindWindowLimit(); got != semanticFastFindRowsLimit {
		t.Fatalf("grid fast-find window = %d, want %d",
			got, semanticFastFindRowsLimit)
	}
}

func TestSemanticPanelLegacyFallbackIncludesCompleteMetadata(t *testing.T) {
	previousCapability := semantic.SetPanelCatalogMetadataEnabled(false)
	t.Cleanup(func() { semantic.SetPanelCatalogMetadataEnabled(previousCapability) })
	previousHighlighter := theme.GlobalFileHighlighter
	highlighter := &theme.FileHighlighter{}
	highlighter.LoadFromIni(ini.Parse(strings.NewReader(`
[Highlight_0]
Name = Text
Mask = *.txt
NormalColor = foreground:#123456
`)))
	theme.GlobalFileHighlighter = highlighter
	t.Cleanup(func() { theme.GlobalFileHighlighter = previousHighlighter })

	tmp := t.TempDir()
	mtime := time.Date(2026, 8, 17, 10, 11, 12, 13, time.UTC)
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Table:         vtui.NewTable(0, 0, 40, 10, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{{VFSItem: vfs.VFSItem{
			Name:         "legacy.txt",
			Size:         1234,
			MTime:        mtime,
			Mode:         "-rwxr-xr-x",
			IsHidden:     true,
			IsExecutable: true,
		}, Selected: true}},
	}

	model := fp.SemanticPanelModel(nil, 0, true)
	node := model.ToMap()
	if model.MetadataDeferred || node["metadataDeferred"] != nil || node["metadataRevision"] != nil {
		t.Fatalf("legacy panel advertised deferred metadata: %#v", node)
	}
	entries := semantic.AppMapSlice(node["entries"])
	if len(entries) != 1 {
		t.Fatalf("legacy entries = %#v", entries)
	}
	entry := entries[0]
	if entry["localPath"] != filepath.Join(tmp, "legacy.txt") ||
		semantic.AppInt64(entry["size"]) != 1234 ||
		entry["sizeText"] != "1 234" ||
		semantic.AppInt64(entry["mtimeNanos"]) != mtime.UnixNano() ||
		entry["mode"] != "-rwxr-xr-x" || entry["isHidden"] != true ||
		entry["isExecutable"] != true || semantic.String(entry["highlightStyleId"]) == "" {
		t.Fatalf("legacy full entry metadata is incomplete: %#v", entry)
	}
	if semantic.AppInt64(node["selectedSize"]) != 1234 || semantic.AppInt64(node["totalSize"]) != 1234 ||
		semantic.AppInt64(node["highlightRevision"]) == 0 || len(semantic.AppMap(node["highlightStyles"])) != 1 {
		t.Fatalf("legacy full panel metadata is incomplete: %#v", node)
	}
	if _, ok := BuildPanelCatalogMetadataChunk(model.ID, model.Path,
		model.CatalogRevision, model.MetadataRevision, 0, 1); ok {
		t.Fatal("legacy full catalog unexpectedly required/published a metadata snapshot")
	}

	// A legacy client has no MetadataRevision field, so a serialized metadata
	// mutation must remain authoritative through CatalogRevision.
	fp.Entries[0].Size++
	changed := fp.SemanticPanelModel(nil, 0, true)
	if changed.CatalogRevision != model.CatalogRevision+1 {
		t.Fatalf("legacy metadata change did not advance catalog revision: before=%d after=%d",
			model.CatalogRevision, changed.CatalogRevision)
	}
}

func TestSemanticDeferredMetadataRevisionTracksOnlyRelevantAccessTime(t *testing.T) {
	previousHighlighter := theme.GlobalFileHighlighter
	theme.GlobalFileHighlighter = &theme.FileHighlighter{}
	t.Cleanup(func() { theme.GlobalFileHighlighter = previousHighlighter })

	baseTime := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(t.TempDir()),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "test"),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{{VFSItem: vfs.VFSItem{
			Name: "folder", IsDir: true, MTime: baseTime, ATime: baseTime,
		}}},
	}

	first := fp.SemanticPanelModel(nil, 0, true)
	fp.Entries[0].ATime = baseTime.Add(time.Second)
	unchanged := fp.SemanticPanelModel(nil, 0, true)
	if unchanged.MetadataRevision != first.MetadataRevision {
		t.Fatalf("unobserved access time advanced deferred metadata revision: before=%d after=%d",
			first.MetadataRevision, unchanged.MetadataRevision)
	}
	if !reflect.DeepEqual(first.ToMap(), unchanged.ToMap()) {
		t.Fatal("unobserved access time changed the deferred panel model")
	}

	theme.GlobalFileHighlighter = &theme.FileHighlighter{
		Revision: 1,
		Rules: []theme.HighlightRule{{
			RuleID: "recent-access", DateType: theme.DateAccessed,
			DateAfter: baseTime, NormalStr: "yellow",
		}},
	}
	observed := fp.SemanticPanelModel(nil, 0, true)
	fp.Entries[0].ATime = baseTime.Add(-time.Second)
	changed := fp.SemanticPanelModel(nil, 0, true)
	if changed.MetadataRevision != observed.MetadataRevision+1 {
		t.Fatalf("highlight-relevant access time did not advance metadata revision: before=%d after=%d",
			observed.MetadataRevision, changed.MetadataRevision)
	}
}

func TestSemanticPanelExportsCatalogProvisionalState(t *testing.T) {
	fp := &FileSystemPanel{
		Vfs:                vfs.NewOSVFS("/"),
		Table:              vtui.NewTable(0, 0, 80, 24, nil),
		SelectedItems:      make(map[string]bool),
		catalogProvisional: true,
		IsLoading:          true,
		Entries: []*FileEntry{{
			VFSItem: vfs.VFSItem{Name: "..", IsDir: true},
		}},
	}
	model := fp.SemanticPanelModel(nil, 0, true)
	if !model.CatalogProvisional || !model.Loading {
		t.Fatalf("placeholder state not exported: %#v", model)
	}
	fp.catalogInteractive = true
	model = fp.SemanticPanelModel(nil, 0, true)
	if !model.CatalogProvisional || model.Loading {
		t.Fatalf("usable provisional catalog remained interaction-blocking: %#v", model)
	}
	fp.catalogInteractive = false
	fp.catalogProvisional = false
	model = fp.SemanticPanelModel(nil, 0, true)
	if model.CatalogProvisional || !model.Loading {
		t.Fatalf("cold authoritative base must remain loading until completion: %#v", model)
	}
}

func TestSemanticOpenSurvivesMatchingProvisionalCatalogReplacement(t *testing.T) {
	previousRowsCapability := semantic.SetPanelCatalogRowsEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogRowsEnabled(previousRowsCapability) })

	directory := t.TempDir()
	fp := &FileSystemPanel{
		Vfs:                vfs.NewOSVFS(directory),
		Table:              vtui.NewTable(0, 0, 80, 24, nil),
		SelectedItems:      make(map[string]bool),
		catalogProvisional: true,
		catalogInteractive: true,
		Entries: []*FileEntry{{
			VFSItem: vfs.VFSItem{Name: "..", IsDir: true},
		}},
	}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)

	preview := fp.SemanticPanelModel(nil, 0, true)
	if len(preview.Entries) != 1 {
		t.Fatalf("preview entries = %d, want 1", len(preview.Entries))
	}
	upID := preview.Entries[0].EntryID

	fp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "authoritative", IsDir: true}},
	}
	fp.catalogProvisional = false
	fp.markSemanticCatalogMutation()
	authoritative := fp.SemanticPanelModel(nil, 0, true)
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
	previous := config.App.SeparateFileExtensions
	config.App.SeparateFileExtensions = true
	defer func() { config.App.SeparateFileExtensions = previous }()

	fp := &FileSystemPanel{
		Vfs:           vfs.NewNullVFS(0),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "iOS"),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "com.example.files", IsExecutable: true, NoExtension: true}},
			{VFSItem: vfs.VFSItem{Name: "notes.txt"}},
		},
	}

	model := fp.SemanticPanelModel(nil, 0, true)
	if got := model.Entries[0]; got.DisplayBaseName != "com.example.files" || got.DisplayExtension != "" {
		t.Fatalf("virtual dotted name was split as an extension: %+v", got)
	}
	if got := model.Entries[1]; got.DisplayBaseName != "notes" || got.DisplayExtension != "txt" {
		t.Fatalf("ordinary file extension was not split: %+v", got)
	}
}

func TestFileSystemPanelExportsDeduplicatedHighlightStyles(t *testing.T) {
	previous := theme.GlobalFileHighlighter
	theme.GlobalFileHighlighter = &theme.FileHighlighter{}
	defer func() { theme.GlobalFileHighlighter = previous }()
	theme.GlobalFileHighlighter.LoadFromIni(ini.Parse(strings.NewReader(`
[Highlight_0]
Name = Text
Mask = *.txt
Icon = qrc:/F4QtHost/icons/lucide/file-text.svg
NormalColor = foreground:#123456
`)))

	tmp := t.TempDir()
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "one.txt"}},
			{VFSItem: vfs.VFSItem{Name: "two.txt"}},
		},
	}
	model := fp.SemanticPanelModel(nil, 0, true)
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
	if semantic.AppInt64(chunk["highlightRevision"]) != model.HighlightRevision {
		t.Fatalf("deferred highlight revision differs from base: %#v", chunk)
	}
	entries := semantic.AppMapSlice(chunk["entries"])
	resolvedStyleID := semantic.String(entries[0]["highlightStyleId"])
	if resolvedStyleID == "" || resolvedStyleID != semantic.String(entries[1]["highlightStyleId"]) {
		t.Fatalf("styles were not deduplicated: %#v", entries)
	}
	styles := semantic.AppMap(chunk["highlightStyles"])
	resolvedStyle := semantic.AppMap(styles[resolvedStyleID])
	if semantic.String(resolvedStyle["icon"]) != "qrc:/F4QtHost/icons/lucide/file-text.svg" ||
		semantic.String(semantic.AppMap(resolvedStyle["normal"])["foreground"]) != "#123456" {
		t.Fatalf("unexpected normalized style: %#v", resolvedStyle)
	}
}

func TestFileSystemPanelSemanticRevisionsAndStableIdentity(t *testing.T) {
	previousSeparateFileExtensions := config.App.SeparateFileExtensions
	config.App.SeparateFileExtensions = false
	defer func() { config.App.SeparateFileExtensions = previousSeparateFileExtensions }()

	tmp := t.TempDir()
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "a.jpg", Size: 10, MTime: time.Unix(10, 20)}},
			{VFSItem: vfs.VFSItem{Name: "b.jpg", Size: 20, MTime: time.Unix(30, 40)}},
		},
	}

	first := fp.SemanticPanelModel(nil, 0, true)
	second := fp.SemanticPanelModel(nil, 0, true)
	if second.CatalogRevision != first.CatalogRevision || second.SelectionRevision != first.SelectionRevision {
		t.Fatalf("unchanged snapshot advanced revisions: first=%+v second=%+v", first, second)
	}
	aID := first.Entries[0].EntryID

	fp.SetItemSelected(0, true)
	selected := fp.SemanticPanelModel(nil, 0, true)
	if selected.CatalogRevision != first.CatalogRevision || selected.SelectionRevision != first.SelectionRevision+1 {
		t.Fatalf("selection changed wrong revisions: first=%+v selected=%+v", first, selected)
	}

	fp.Entries[0].MTime = fp.Entries[0].MTime.Add(time.Nanosecond)
	changed := fp.SemanticPanelModel(nil, 0, true)
	if changed.CatalogRevision != selected.CatalogRevision || changed.MetadataRevision != selected.MetadataRevision+1 {
		t.Fatalf("mtime change touched wrong revisions: selected=(%d,%d) changed=(%d,%d)",
			selected.CatalogRevision, selected.MetadataRevision, changed.CatalogRevision, changed.MetadataRevision)
	}
	if changed.Entries[0].EntryID != aID {
		t.Fatalf("content metadata changed stable ID: got %q want %q", changed.Entries[0].EntryID, aID)
	}

	fp.Entries[0], fp.Entries[1] = fp.Entries[1], fp.Entries[0]
	reordered := fp.SemanticPanelModel(nil, 0, true)
	if reordered.CatalogRevision != changed.CatalogRevision+1 || reordered.Entries[1].EntryID != aID {
		t.Fatalf("reorder revision/identity mismatch: %+v", reordered)
	}
	if reordered.SelectionRevision != changed.SelectionRevision {
		t.Fatalf("catalog reorder advanced selection revision: changed=%d reordered=%d", changed.SelectionRevision, reordered.SelectionRevision)
	}

	config.App.SeparateFileExtensions = true
	separated := fp.SemanticPanelModel(nil, 0, true)
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
		Vfs:           remote,
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, "/"),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		SelectedItems: make(map[string]bool),
		Entries:       []*FileEntry{{VFSItem: vfs.VFSItem{Name: "remote.jpg", Size: 1}}},
	}
	model := fp.SemanticPanelModel(nil, 0, true)
	if model.SourceKind != "vfs" || !model.PreviewCapable {
		t.Fatalf("unexpected remote capability model: %+v", model)
	}
	if model.Entries[0].LocalPath != "" || model.Entries[0].Path == "" ||
		model.Entries[0].EntryID == "" || !model.Entries[0].IsImage ||
		model.Entries[0].Source == nil || model.Entries[0].Source.SourceKey == "" {
		t.Fatalf("unexpected remote entry metadata: %+v", model.Entries[0])
	}
	metadata := semantic.AppMapSlice(semanticMetadataChunkForModel(t, model)["entries"])
	if metadata[0]["localPath"] != "" {
		t.Fatalf("remote entry unexpectedly resolved a local path: %#v", metadata[0])
	}
}

func TestSemanticBaseCatalogIgnoresAllDeferredMetadata(t *testing.T) {
	previousHighlighter := theme.GlobalFileHighlighter
	theme.GlobalFileHighlighter = &theme.FileHighlighter{}
	defer func() { theme.GlobalFileHighlighter = previousHighlighter }()

	// Loaded once, up front: a name-mask rule like this one is resolved by
	// the fast base pass (see FileHighlighter.SemanticStyle's metadataKnown
	// parameter), so its highlightStyleId is part of the base/minimal
	// catalog from the first call and must stay identical across both calls
	// below. Only the deferred per-file fields mutated between the two
	// calls are under test here.
	theme.GlobalFileHighlighter.LoadFromIni(ini.Parse(strings.NewReader(`
[Highlight_0]
Name = Photos
Mask = *.jpg
NormalColor = foreground:#ABCDEF
`)))

	tmp := t.TempDir()
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Table:         vtui.NewTable(0, 0, 40, 10, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{{VFSItem: vfs.VFSItem{
			Name: "photo.jpg", Size: 10, MTime: time.Unix(10, 20),
			Mode: "-rw-r--r--", Uid: 1, Gid: 2,
		}}},
	}
	first := fp.SemanticPanelModel(nil, 0, true)
	firstBase := first.ToMap()["entries"]

	fp.Entries[0].Size = 999
	fp.Entries[0].PhysicalSize = 4096
	fp.Entries[0].MTime = time.Unix(30, 40)
	fp.Entries[0].ATime = time.Unix(31, 41)
	fp.Entries[0].CTime = time.Unix(32, 42)
	fp.Entries[0].Mode = "-rwx------"
	fp.Entries[0].UnixMode = 0o700
	fp.Entries[0].Uid = 42
	fp.Entries[0].Gid = 43
	fp.Entries[0].WinAttrs = 7
	fp.Entries[0].IsExecutable = true
	fp.Entries[0].SizeCalculated = true

	second := fp.SemanticPanelModel(nil, 0, true)
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
	entry := semantic.AppMapSlice(chunk["entries"])[0]
	if semantic.AppInt64(entry["size"]) != 999 || semantic.String(entry["mode"]) != "-rwx------" ||
		semantic.String(entry["highlightStyleId"]) == "" {
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

	fp.Entries[0].IsHidden = true
	third := fp.SemanticPanelModel(nil, 0, true)
	if third.CatalogRevision != second.CatalogRevision+1 {
		t.Fatalf("hidden state did not advance catalog revision: second=%d third=%d",
			second.CatalogRevision, third.CatalogRevision)
	}
	if third.MetadataRevision != second.MetadataRevision {
		t.Fatalf("hidden state incorrectly advanced metadata revision: second=%d third=%d",
			second.MetadataRevision, third.MetadataRevision)
	}
	thirdEntry := semantic.AppMapSlice(third.ToMap()["entries"])[0]
	if thirdEntry["isHidden"] != true {
		t.Fatalf("authoritative hidden state missing from base catalog: %#v", thirdEntry)
	}
	if metadataEntry := semantic.AppMapSlice(semanticMetadataChunkForModel(t, third)["entries"])[0]; metadataEntry["isHidden"] != nil {
		t.Fatalf("hidden state was redundantly deferred: %#v", metadataEntry)
	}
}

func TestPanelCatalogMetadataChunksAreOrderedBoundedAndRejectStaleRequests(t *testing.T) {
	previousCapability := semantic.SetPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogMetadataEnabled(previousCapability) })
	tmp := t.TempDir()
	entries := make([]*FileEntry, 0, 205)
	for i := 0; i < 205; i++ {
		entries = append(entries, &FileEntry{VFSItem: vfs.VFSItem{
			Name: fmt.Sprintf("entry-%03d.txt", i), Size: int64(i),
		}})
	}
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Table:         vtui.NewTable(0, 0, 40, 10, nil),
		SelectedItems: make(map[string]bool),
		Entries:       entries,
	}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)
	first := fp.SemanticPanelModel(nil, 0, true)
	defaultChunk, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 0)
	if !ok || defaultChunk["limit"] != 8 || len(semantic.AppMapSlice(defaultChunk["entries"])) != 8 {
		t.Fatalf("unexpected default metadata chunk: %#v", defaultChunk)
	}
	firstChunk, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 64)
	if !ok || firstChunk["offset"] != 0 || firstChunk["limit"] != 64 ||
		firstChunk["total"] != 205 || firstChunk["final"] != false {
		t.Fatalf("unexpected first metadata chunk: %#v", firstChunk)
	}
	firstRows := semantic.AppMapSlice(firstChunk["entries"])
	if len(firstRows) != 64 || semantic.String(firstRows[0]["entryId"]) != first.Entries[0].EntryID ||
		semantic.String(firstRows[63]["entryId"]) != first.Entries[63].EntryID {
		t.Fatalf("first chunk order mismatch: %#v", firstRows)
	}

	lastChunk, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 192, 999)
	if !ok || lastChunk["limit"] != 128 || lastChunk["final"] != true ||
		len(semantic.AppMapSlice(lastChunk["entries"])) != 13 {
		t.Fatalf("unexpected last metadata chunk: %#v", lastChunk)
	}

	// The raw entry can mutate before the next scene is published without
	// changing what an already-acknowledged revision returns.
	fp.Entries[0].Size = 9999
	repeat, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 1)
	if !ok || semantic.AppInt64(semantic.AppMapSlice(repeat["entries"])[0]["size"]) != -1 {
		t.Fatalf("published metadata snapshot was not immutable: %#v", repeat)
	}

	second := fp.SemanticPanelModel(nil, 0, true)
	if _, ok := BuildPanelCatalogMetadataChunk(first.ID, first.Path,
		first.CatalogRevision, first.MetadataRevision, 0, 1); ok {
		t.Fatal("metadata-only stale request was accepted")
	}
	fp.Entries[0].Name = "renamed.txt"
	third := fp.SemanticPanelModel(nil, 0, true)
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
	previousCapability := semantic.SetPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogMetadataEnabled(previousCapability) })

	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(t.TempDir()),
		Table:         vtui.NewTable(0, 0, 40, 10, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "pending.txt"}},
			{VFSItem: vfs.VFSItem{Name: "empty.txt", SizeKnown: true}},
			{VFSItem: vfs.VFSItem{Name: "payload.txt", Size: 42}},
		},
	}

	provisional := fp.SemanticPanelModel(nil, 0, true)
	rows := semantic.AppMapSlice(semanticMetadataChunkForModel(t, provisional)["entries"])
	if got := semantic.AppInt64(rows[0]["size"]); got != -1 || semantic.String(rows[0]["sizeText"]) != "" {
		t.Fatalf("unknown size was serialized as a real value: %#v", rows[0])
	}
	if got := semantic.AppInt64(rows[1]["size"]); got != 0 || semantic.String(rows[1]["sizeText"]) != "0" {
		t.Fatalf("known empty file lost its zero size: %#v", rows[1])
	}
	if got := semantic.AppInt64(rows[2]["size"]); got != 42 || semantic.String(rows[2]["sizeText"]) != "42" {
		t.Fatalf("legacy non-zero size was not treated as known: %#v", rows[2])
	}

	// The metadata phase can resolve an unknown row to an actual zero-byte
	// file. That transition must advance only MetadataRevision, otherwise the
	// frontend would retain the provisional blank/zero snapshot forever.
	fp.Entries[0].SizeKnown = true
	resolved := fp.SemanticPanelModel(nil, 0, true)
	if resolved.CatalogRevision != provisional.CatalogRevision ||
		resolved.MetadataRevision != provisional.MetadataRevision+1 {
		t.Fatalf("known-zero enrichment changed wrong revisions: before=(%d,%d) after=(%d,%d)",
			provisional.CatalogRevision, provisional.MetadataRevision,
			resolved.CatalogRevision, resolved.MetadataRevision)
	}
	resolvedRows := semantic.AppMapSlice(semanticMetadataChunkForModel(t, resolved)["entries"])
	if got := semantic.AppInt64(resolvedRows[0]["size"]); got != 0 || semantic.String(resolvedRows[0]["sizeText"]) != "0" {
		t.Fatalf("resolved empty file was not published: %#v", resolvedRows[0])
	}
}

func TestPanelsFrameCloseUnpublishesOnlyOwnedSemanticMetadataSnapshot(t *testing.T) {
	newPanel := func() (*FileSystemPanel, extui.PanelModel) {
		fp := &FileSystemPanel{
			Vfs:           vfs.NewOSVFS(t.TempDir()),
			Table:         vtui.NewTable(0, 0, 40, 10, nil),
			SelectedItems: make(map[string]bool),
			Entries: []*FileEntry{{VFSItem: vfs.VFSItem{
				Name: "entry.txt", Size: 1,
			}}},
		}
		return fp, fp.SemanticPanelModel(nil, 0, true)
	}

	owned, ownedModel := newPanel()
	ownedSnapshot := owned.semanticMetadataSnapshot
	(&PanelsFrame{Panels: [2]Panel{owned, nil}}).Close()
	if _, present := semanticPanelMetadataSnapshots.Load(ownedModel.ID); present {
		t.Fatal("workspace close retained its owned semantic metadata snapshot")
	}

	stale, staleModel := newPanel()
	replacement := &semanticPanelMetadataSnapshot{panelID: staleModel.ID, path: "newer"}
	semanticPanelMetadataSnapshots.Store(staleModel.ID, replacement)
	t.Cleanup(func() {
		semanticPanelMetadataSnapshots.CompareAndDelete(staleModel.ID, replacement)
	})
	(&PanelsFrame{Panels: [2]Panel{stale, nil}}).Close()
	loaded, present := semanticPanelMetadataSnapshots.Load(staleModel.ID)
	if !present || loaded != replacement {
		t.Fatalf("stale panel removed newer snapshot: present=%v loaded=%p replacement=%p owned=%p",
			present, loaded, replacement, ownedSnapshot)
	}
}

func TestPanelsFrameSemanticActivationNotifiesRenderer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	renderer := &searchFirstActivationRenderer{side: -1}
	vtui.FrameManager.Screen().Renderer = renderer

	tmp := t.TempDir()
	left := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "left.txt", Size: 10}},
		},
	}
	right := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(40, 0, 79, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(41, 1, 78, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "right.txt", Size: 20}},
		},
	}
	pf := &PanelsFrame{
		Panels:         [2]Panel{left, right},
		ActiveIdx:      1,
		ShowPanels:     true,
		ShowLeftPanel:  true,
		ShowRightPanel: true,
		CmdLine:        cmdline.NewCommandLine("$ "),
		TermView:       terminal.NewTerminalView(80, 24),
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
	if pf.ActiveIdx != 0 {
		t.Fatalf("activeIdx = %d, want 0", pf.ActiveIdx)
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
	rightModel := right.SemanticPanelModel(nil, 1, true)
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
	if pf.ActiveIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.ActiveIdx)
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
	if pf.ActiveIdx != 0 {
		t.Fatalf("activeIdx = %d, want 0", pf.ActiveIdx)
	}
	if renderer.calls != 1 || renderer.side != 0 {
		t.Fatalf("stale panel.cursor with activate did not notify renderer: calls=%d side=%d", renderer.calls, renderer.side)
	}
}

func TestPanelsFrameSemanticPointerIntentsClearFastFind(t *testing.T) {
	tmp := t.TempDir()
	panel := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.jpg", Size: 12}},
		},
	}
	frame := &PanelsFrame{Panels: [2]Panel{panel, panel}, ActiveIdx: 0}
	model := panel.SemanticPanelModel(nil, 0, true)
	alphaID := model.Entries[1].EntryID

	setFastFind := func() {
		panel.FastFindMode = true
		panel.FastFindStr = "alpha"
	}
	assertCleared := func(action string) {
		t.Helper()
		if panel.FastFindMode || panel.FastFindStr != "" {
			t.Fatalf("%s left fast-find active: mode=%v text=%q",
				action, panel.FastFindMode, panel.FastFindStr)
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
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.jpg", Size: 12}},
			{VFSItem: vfs.VFSItem{Name: "beta.jpg", Size: 34}},
		},
	}
	pf := &PanelsFrame{Panels: [2]Panel{left, left}, ActiveIdx: 0}
	model := left.SemanticPanelModel(nil, 0, true)
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
	}) || !left.Entries[1].Selected || !left.Entries[2].Selected {
		t.Fatal("stable-ID replacement selection was not applied")
	}
	selected := left.SemanticPanelModel(nil, 0, true)
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
	}) || left.Entries[2].Selected {
		t.Fatal("batch index compatibility selection was not applied")
	}

	transactionBase := left.SemanticPanelModel(nil, 0, true)
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
	}) || left.Entries[1].Selected || !left.Entries[2].Selected ||
		left.GetCursorIndex() != 1 {
		t.Fatal("atomic selection/cursor transaction was not applied")
	}

	// Validate every stable identity before mutating either half. A bad final
	// cursor must not leave an otherwise-valid selection change behind.
	beforeRejected := left.Entries[1].Selected
	fresh := left.SemanticPanelModel(nil, 0, true)
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
	}) || left.Entries[1].Selected != beforeRejected {
		t.Fatal("invalid atomic selection cursor partially mutated selection")
	}

	beforeAlpha := left.Entries[1].Selected
	if pf.HandleSemanticAction(map[string]any{
		"action":   "panel.setSelection",
		"side":     0,
		"mode":     "replace",
		"entryIds": []string{alphaID, "missing"},
	}) || left.Entries[1].Selected != beforeAlpha {
		t.Fatal("invalid stable-ID batch was not rejected atomically")
	}

}

func TestPanelsFrameSemanticGalleryLayoutActions(t *testing.T) {
	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	pf := &PanelsFrame{Panels: [2]Panel{left, right}, ActiveIdx: 0}
	left.SetViewMode(ViewModeBrief)

	initial := left.SemanticPanelModel(nil, 0, true)
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
	columns := left.SemanticPanelModel(nil, 0, true)
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

	beforeDensityLayout := left.GalleryLayoutMode
	beforeDensityRevision := left.GalleryLayoutRevision
	if !pf.HandleSemanticAction(map[string]any{
		"action":            "panel.setGalleryDensity",
		"side":              0,
		"galleryLayoutMode": "icons",
		"density":           999,
	}) {
		t.Fatal("Icons density action was rejected")
	}
	if left.GalleryLayoutMode != beforeDensityLayout {
		t.Fatal("density-only action changed the active layout")
	}
	if left.galleryDensity(GalleryLayoutIcons) != 256 ||
		left.GalleryLayoutRevision != beforeDensityRevision+1 {
		t.Fatalf("density action did not clamp/revise state: density=%d revision=%d",
			left.galleryDensity(GalleryLayoutIcons), left.GalleryLayoutRevision)
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
	left.GalleryDensities[GalleryLayoutDetails] = 47
	detailsRevision := left.GalleryLayoutRevision
	if !pf.HandleSemanticAction(map[string]any{
		"action":     "panel.setGalleryDensity",
		"side":       0,
		"layoutMode": "details",
		"density":    31,
	}) || left.galleryDensity(GalleryLayoutDetails) != 31 {
		t.Fatal("Details density action was not applied")
	}
	if details := left.SemanticPanelModel(nil, 0, true); details.GalleryDensity != 0 || details.GalleryDensities["details"] != 31 {
		t.Fatalf("Details density was not exported in bounded layout state: %#v",
			details.GalleryDensities)
	}
	if left.GalleryLayoutRevision != detailsRevision+1 {
		t.Fatalf("Details density revision = %d, want %d",
			left.GalleryLayoutRevision, detailsRevision+1)
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
	pf.SetPanelViewMode(0, ViewModeDetailed)
	if left.ViewMode != ViewModeDetailed ||
		left.GalleryLayoutMode != GalleryLayoutDetails {
		t.Fatal("Detailed action did not select unified Details")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":  "panel.setWide",
		"side":    0,
		"enabled": true,
	}) || !pf.Wide || pf.WidePanel != 0 {
		t.Fatal("independent Wide toggle did not expand the requested panel")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":      "panel.setGalleryLayout",
		"side":        0,
		"layoutMode":  "icons",
		"columnCount": 0,
	}) || !pf.Wide || pf.WidePanel != 0 {
		t.Fatal("unified renderer selection disturbed independent Wide layout")
	}
	pf.SetPanelViewMode(0, ViewModeBrief)
	if !pf.Wide || pf.WidePanel != 0 ||
		left.GalleryLayoutMode != GalleryLayoutColumns ||
		left.GalleryColumnCount != 3 {
		t.Fatal("Columns 3 alias disturbed independent Wide layout")
	}
	wideModel := left.SemanticPanelModel(nil, 0, true)
	if wideModel.GalleryLayoutMode != "columns" ||
		wideModel.GalleryColumnCount != 3 {
		t.Fatalf("Wide leaked into renderer identity: %#v", wideModel)
	}
	// This is the exact state transition owned by the Tab branch; the full
	// initialized PanelsFrame keyboard test below exercises its key routing.
	pf.ActiveIdx = 1 - pf.ActiveIdx
	pf.WidePanel = pf.ActiveIdx
	if pf.ActiveIdx != 1 || pf.WidePanel != 1 || !pf.Wide {
		t.Fatal("Tab did not transfer independent Wide to the second panel")
	}
	if !pf.HandleSemanticAction(map[string]any{
		"action":  "panel.setWide",
		"side":    1,
		"enabled": false,
	}) || pf.Wide || pf.WidePanel != -1 {
		t.Fatal("Wide toggle did not restore split layout")
	}
	pf.SetWidePanel(1)
	if !pf.Wide || pf.WidePanel != 1 {
		t.Fatal("Wide command did not select the requested panel")
	}
}

func TestSemanticGalleryLayoutPersistsEachPanelIndependently(t *testing.T) {
	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	pf := &PanelsFrame{Panels: [2]Panel{left, right}, ActiveIdx: 0}

	originalPersist := persistNativePanelLayoutSession
	defer func() { persistNativePanelLayoutSession = originalPersist }()
	var snapshots []WorkspaceSessionState
	persistNativePanelLayoutSession = func(got *PanelsFrame) {
		if got != pf {
			t.Fatalf("persisted unexpected PanelsFrame %p, want %p", got, pf)
		}
		snapshots = append(snapshots, CaptureWorkspaceSession(got))
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
	pf := &PanelsFrame{Panels: [2]Panel{left, right}, ActiveIdx: 1}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.navigatePath",
		"side":   0,
		"path":   child,
	}) {
		t.Fatal("path navigation action was rejected")
	}
	if got := left.Vfs.GetPath(); got != child {
		t.Fatalf("left panel path = %q, want %q", got, child)
	}
	if got := right.Vfs.GetPath(); got != rightRoot {
		t.Fatalf("navigation changed the wrong panel: %q", got)
	}
	if pf.ActiveIdx != 0 {
		t.Fatalf("navigated panel was not activated: %d", pf.ActiveIdx)
	}
	if left.PendingSelection != filepath.Base(root) {
		t.Fatalf("parent return selection = %q, want %q",
			left.PendingSelection, filepath.Base(root))
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
	screen := vtui.NewSilentScreenBuf()
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()
	renderer := &searchFirstActivationRenderer{side: -1}
	screen.Renderer = renderer

	left := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	pf := &PanelsFrame{
		Panels: [2]Panel{left, right}, ActiveIdx: 1,
		ShowPanels: true, ShowLeftPanel: true, ShowRightPanel: true,
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.driveMenu",
		"side":   0,
	}) {
		t.Fatal("drive menu action was rejected")
	}
	if pf.ActiveIdx != 0 {
		t.Fatalf("drive menu did not activate requested panel: %d", pf.ActiveIdx)
	}
	if renderer.calls != 1 || renderer.side != 0 || renderer.activationMenus != 1 {
		t.Fatalf("inactive drive click published activation/menu contract = %d/%d/%d, want 1/0/1",
			renderer.calls, renderer.side, renderer.activationMenus)
	}
	if menu, _ := driveMenuFromFrame(vtui.FrameManager.GetTopFrame()); menu == nil {
		t.Fatalf("drive menu action opened %T instead of VMenu",
			vtui.FrameManager.GetTopFrame())
	} else if menu.GetTitle() != i18n.Msg("Drive.Title") {
		t.Fatalf("drive menu title = %q, want %q", menu.GetTitle(), i18n.Msg("Drive.Title"))
	} else {
		if len(menu.Items) == 0 || menu.Items[0].Icon != sysinfo.DriveMenuIconOtherPanel {
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
	if panel.CancelLoad != nil {
		defer panel.CancelLoad()
	}
	frame := &PanelsFrame{Panels: [2]Panel{panel, panel}}

	for _, tc := range []struct {
		name    string
		command int
		mode    ViewMode
		layout  GalleryLayoutMode
		columns int
	}{
		{name: "columns 3", command: appcmd.CmLeftBrief, mode: ViewModeBrief, layout: GalleryLayoutColumns, columns: 3},
		{name: "columns 2", command: appcmd.CmLeftMedium, mode: ViewModeMedium, layout: GalleryLayoutColumns, columns: 2},
		{name: "details", command: appcmd.CmLeftDetailed, mode: ViewModeDetailed, layout: GalleryLayoutDetails, columns: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !frame.HandleCommand(tc.command, nil) {
				t.Fatalf("command %d was not handled", tc.command)
			}
			if panel.ViewMode != tc.mode ||
				panel.effectiveGalleryLayoutMode() != tc.layout ||
				panel.effectiveGalleryColumnCount() != tc.columns {
				t.Fatalf("mode command produced TUI=%v layout=%q columns=%d",
					panel.ViewMode, panel.effectiveGalleryLayoutMode(),
					panel.effectiveGalleryColumnCount())
			}
		})
	}
}

func TestPanelsFrameSemanticPointerPreservesGoMouseSemantics(t *testing.T) {
	fp := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(t.TempDir()))
	fp.SetViewMode(ViewModeDetailed)
	fp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "first.txt"}},
		{VFSItem: vfs.VFSItem{Name: "second.txt"}},
	}
	fp.Refresh()
	pf := &PanelsFrame{Panels: [2]Panel{fp, fp}, ActiveIdx: 0}
	model := fp.SemanticPanelModel(nil, 0, true)

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
	if !pointer(1, "right", "down") || !fp.Entries[1].Selected {
		t.Fatal("right mouse-down did not toggle selection")
	}
	if !pointer(1, "right", "up") {
		t.Fatal("right mouse-up was not accepted")
	}
	// Qt reports a normal second press before its doubleClicked signal. Go
	// must retain the first click's operation and spread that state, matching
	// the terminal backend's native DoubleClick event.
	if !pointer(1, "right", "down") || fp.Entries[1].Selected {
		t.Fatal("second right mouse-down did not toggle the row")
	}
	if !pointer(1, "right", "doubleClick") {
		t.Fatal("right double-click was not accepted")
	}
	if !fp.Entries[1].Selected || !fp.Entries[2].Selected || fp.Entries[0].Selected {
		t.Fatalf("right double-click did not spread the first click state: %#v", fp.Entries)
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
		Vfs:           localVFS,
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, root),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "decoy", IsDir: true, Size: 1}},
			{VFSItem: vfs.VFSItem{Name: "authoritative", IsDir: true, Size: 1}},
		},
	}
	panel.SetCursorIndex(1)
	frame := &PanelsFrame{
		Panels:     [2]Panel{panel, panel},
		ActiveIdx:  1,
		ShowPanels: true,
		CmdLine:    cmdline.NewCommandLine(">"),
	}

	stale := panel.SemanticPanelModel(nil, 0, false)
	targetID := stale.Entries[2].EntryID
	panel.Entries[1].Size++
	metadataOnly := panel.SemanticPanelModel(nil, 0, false)
	if metadataOnly.CatalogRevision != stale.CatalogRevision ||
		metadataOnly.MetadataRevision != stale.MetadataRevision+1 {
		t.Fatalf("metadata-only mutation changed wrong revision domain: stale=(%d,%d) current=(%d,%d)",
			stale.CatalogRevision, stale.MetadataRevision,
			metadataOnly.CatalogRevision, metadataOnly.MetadataRevision)
	}
	panel.Entries[1].Name = "renamed-decoy"
	current := panel.SemanticPanelModel(nil, 0, false)
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
	if panel.CancelLoad != nil {
		panel.CancelLoad()
	}
	if panel.LoadingTimer != nil {
		panel.LoadingTimer.Stop()
	}
	select {
	case <-localVFS.readDone:
	case <-time.After(time.Second):
		t.Fatal("authoritative directory catalog load did not stop after cancellation")
	}

	if !handled {
		t.Fatal("unrevisioned stable-ID open was rejected")
	}
	if frame.ActiveIdx != 0 {
		t.Fatalf("open did not activate its panel: active=%d", frame.ActiveIdx)
	}
	if got := panel.Vfs.GetPath(); got != targetPath {
		t.Fatalf("open resolved the wrong entry: got %q want %q", got, targetPath)
	}
}

func TestPanelsFrameSemanticRapidSelectionActionsRemainOrdered(t *testing.T) {
	tmp := t.TempDir()
	panel := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		SelectedItems: make(map[string]bool),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "first.jpg", Size: 10}},
			{VFSItem: vfs.VFSItem{Name: "second.jpg", Size: 20}},
		},
	}
	frame := &PanelsFrame{Panels: [2]Panel{panel, panel}}
	model := panel.SemanticPanelModel(nil, 0, true)

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
	if !panel.Entries[0].Selected || !panel.Entries[1].Selected {
		t.Fatalf("rapid ordered actions lost a selection: first=%v second=%v",
			panel.Entries[0].Selected, panel.Entries[1].Selected)
	}
}

func TestPanelsFrameSemanticGridFallbackForUnsupportedPanelLayouts(t *testing.T) {
	pf := &PanelsFrame{
		ShowPanels: true, ShowLeftPanel: true, ShowRightPanel: true,
		LastW: 100, LastH: 30,
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("default panel layout unexpectedly requires fallback: %q", reason)
	}

	pf.WidthDecrement = 6
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("native horizontal split unexpectedly requires fallback: %q", reason)
	}
	layout := pf.semanticPanelLayoutModel(&vtui.SemanticContext{Width: 100, Height: 30})
	if layout.Columns != 100 || layout.SplitColumn != 44 ||
		layout.LeftBottomInsetRows != 0 || layout.RightBottomInsetRows != 0 {
		t.Fatalf("horizontal split layout = %#v, want 100 columns at split 44", layout)
	}
	pf.WidthDecrement = 0
	pf.LeftHeightDecrement = 1
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("shortened panel fallback reason = %q", reason)
	}
	pf.LeftHeightDecrement = 0
	pf.ShowRightPanel = false
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("hidden panel layout unexpectedly requires fallback: %q", reason)
	}
	pf.ShowRightPanel = true
	pf.AltPanels[0] = &InfoPanel{}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("info panel unexpectedly requires fallback: %q", reason)
	}
	pf.AltPanels[0] = &QuickViewPanel{}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("quick-view unexpectedly requires fallback: %q", reason)
	}
}
