package panel

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestParseTypedFileFieldsUsesNumericValuesAndRejectsInvalidData(t *testing.T) {
	tests := []struct {
		field string
		raw   any
		want  extui.FileFieldValue
		ok    bool
	}{
		{"exif.exposure_time", 1.0 / 125, extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldNumber, Unit: "s", Format: "exposure", Precision: 3, Number: 1.0 / 125}, true},
		{"exif.iso", uint32(800), extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldInteger, Format: "integer", Integer: 800}, true},
		{"exif.f_number", 2.8, extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldNumber, Unit: "f", Format: "aperture", Precision: 1, Number: 2.8}, true},
		{"exif.focal_length_35mm", 75.0, extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldNumber, Unit: "mm", Format: "quantity", Number: 75}, true},
		{"exif.lens_focal_range", map[string]any{"min": 24.0, "max": 70.0}, extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldRange, Unit: "mm", Format: "range", Min: 24, Max: 70}, true},
		{"exif.camera_model", "  Canon R5  ", extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldText, Format: "text", Text: "Canon R5"}, true},
		{"exif.lens_model", "RF 24-70mm", extui.FileFieldValue{State: extui.FileFieldKnown, Kind: extui.FileFieldText, Format: "text", Text: "RF 24-70mm"}, true},
		{"exif.iso", 0, extui.FileFieldValue{}, false},
		{"exif.lens_focal_range", map[string]any{"min": 70.0, "max": 24.0}, extui.FileFieldValue{}, false},
		{"exif.camera_model", "  ", extui.FileFieldValue{}, false},
	}
	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			descriptor, ok := fileFieldDescriptor(test.field)
			if !ok {
				t.Fatalf("descriptor %q is missing", test.field)
			}
			got, ok := parseTypedFileField(descriptor, test.raw)
			if ok != test.ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseTypedFileField() = %#v, %v; want %#v, %v", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestFileFieldFiltersKeepUnreadRowsAndRestoreHiddenRows(t *testing.T) {
	fp := groupTestPanel(t)
	known := &FileEntry{VFSItem: vfs.VFSItem{Name: "known.jpg"}, FileFieldsSourceKey: "known", FileFieldsSourceVersion: "v1", FileFieldsComplete: true,
		FileFields: map[string]extui.FileFieldValue{"exif.iso": {State: extui.FileFieldKnown, Kind: extui.FileFieldInteger, Integer: 800}}}
	missing := &FileEntry{VFSItem: vfs.VFSItem{Name: "missing.jpg"}, FileFieldsSourceKey: "missing", FileFieldsSourceVersion: "v1", FileFieldsComplete: true,
		FileFields: map[string]extui.FileFieldValue{"exif.iso": {State: extui.FileFieldMissing, Kind: extui.FileFieldInteger}}}
	unread := &FileEntry{VFSItem: vfs.VFSItem{Name: "unread.jpg"}}
	fp.Entries = []*FileEntry{known, missing, unread}
	stampFileFieldEntry(fp, known)
	stampFileFieldEntry(fp, missing)
	if !fp.SetFileFieldFilters([]FileFieldFilter{{FieldID: "exif.iso", Operation: "ge", Value: "400"}}, false) {
		t.Fatal("valid filter rejected")
	}
	if got := []string{fp.Entries[0].Name, fp.Entries[1].Name}; !reflect.DeepEqual(got, []string{"known.jpg", "unread.jpg"}) {
		t.Fatalf("visible rows = %v, want known and unread", got)
	}
	if got := fp.FileFieldFilterPendingCount(); got != 1 {
		t.Fatalf("pending count = %d, want 1", got)
	}
	if !fp.SetFileFieldFilters(nil, false) {
		t.Fatal("clearing filter was treated as invalid")
	}
	if got := []string{fp.Entries[0].Name, fp.Entries[1].Name, fp.Entries[2].Name}; !reflect.DeepEqual(got, []string{"known.jpg", "missing.jpg", "unread.jpg"}) {
		t.Fatalf("clearing filter lost hidden rows: %v", got)
	}
}

func TestFileFieldFiltersKeepDirectoriesAndCombineWithNameSearch(t *testing.T) {
	fp := groupTestPanel(t)
	previousAutoFilter := config.App.PanelAutoFilter
	config.App.PanelAutoFilter = true
	defer func() { config.App.PanelAutoFilter = previousAutoFilter }()

	parent := &FileEntry{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}
	folder := &FileEntry{VFSItem: vfs.VFSItem{Name: "Camera Roll", IsDir: true}}
	otherFolder := &FileEntry{VFSItem: vfs.VFSItem{Name: "Vacation", IsDir: true}}
	known := fieldTestEntry("Camera-ISO-800.jpg", 800)
	missing := &FileEntry{VFSItem: vfs.VFSItem{Name: "Camera-no-ISO.jpg"},
		FileFieldsSourceKey: "missing", FileFieldsSourceVersion: "v1",
		FileFieldsComplete: true,
		FileFields: map[string]extui.FileFieldValue{
			"exif.iso": {State: extui.FileFieldMissing, Kind: extui.FileFieldInteger},
		}}
	unread := &FileEntry{VFSItem: vfs.VFSItem{Name: "Camera-pending.jpg"}}
	fp.Entries = []*FileEntry{parent, folder, otherFolder, known, missing, unread}
	stampFileFieldEntry(fp, known)
	stampFileFieldEntry(fp, missing)

	if !fp.SetFileFieldFilters([]FileFieldFilter{{
		FieldID: "exif.iso", Operation: "ge", Value: "400",
	}}, false) {
		t.Fatal("valid filter rejected")
	}
	assertPanelEntryNames(t, fp.Entries, "..", "Camera Roll", "Vacation", "Camera-ISO-800.jpg", "Camera-pending.jpg")
	if got := fp.FileFieldFilterPendingCount(); got != 1 {
		t.Fatalf("pending count = %d, want 1 image row", got)
	}

	fp.FastFindMode = true
	fp.FastFindStr = "Camera"
	fp.applyFastFind()
	assertPanelEntryNames(t, fp.Entries, "..", "Camera Roll", "Camera-ISO-800.jpg", "Camera-pending.jpg")

	fp.FastFindMode = false
	fp.FastFindStr = ""
	fp.refilterEntries()
	assertPanelEntryNames(t, fp.Entries, "..", "Camera Roll", "Vacation", "Camera-ISO-800.jpg", "Camera-pending.jpg")
	if !fp.SetFileFieldFilters(nil, false) {
		t.Fatal("clearing filter was treated as invalid")
	}
	assertPanelEntryNames(t, fp.Entries, "..", "Camera Roll", "Vacation", "Camera-ISO-800.jpg", "Camera-no-ISO.jpg", "Camera-pending.jpg")
}

func TestNonImageExifFieldsAreKnownMissingWithoutMetadataRead(t *testing.T) {
	descriptor, ok := fileFieldDescriptor("exif.iso")
	if !ok {
		t.Fatal("ISO descriptor is missing")
	}
	entry := &FileEntry{VFSItem: vfs.VFSItem{Name: "readme.txt"}}
	if got := fieldValueForSort(entry, descriptor); got.State != extui.FileFieldMissing {
		t.Fatalf("non-image EXIF state = %q, want missing", got.State)
	}
	if got := matchFileFieldFilter(
		fieldValueForSort(entry, descriptor), descriptor,
		FileFieldFilter{FieldID: descriptor.ID, Operation: "missing"}); got != fileFieldMatchTrue {
		t.Fatalf("missing filter state = %v, want true", got)
	}
}

func TestFileFieldFilterParsesFractionalExposureAndMetricUnits(t *testing.T) {
	exposure, _ := fileFieldDescriptor("exif.exposure_time")
	for _, input := range []string{"1/125", "1/125 s", "1/125 с"} {
		got, ok := parseFieldFilterNumber(exposure, input)
		if !ok || math.Abs(got-1.0/125) > 1e-12 {
			t.Errorf("parseFieldFilterNumber(%q) = %v, %v; want 1/125", input, got, ok)
		}
	}
	focal, _ := fileFieldDescriptor("exif.focal_length_35mm")
	for _, input := range []string{"75 mm", "75 мм"} {
		if got, ok := parseFieldFilterNumber(focal, input); !ok || got != 75 {
			t.Errorf("parseFieldFilterNumber(%q) = %v, %v; want 75", input, got, ok)
		}
	}
	for _, input := range []string{"24-70 mm", "24–70 мм"} {
		if minimum, maximum, ok := parseFieldFilterRange(input); !ok || minimum != 24 || maximum != 70 {
			t.Errorf("parseFieldFilterRange(%q) = %v, %v, %v; want 24, 70", input, minimum, maximum, ok)
		}
	}
}

func TestPagedPresentationRevisionKeepsMetadataReadGeneration(t *testing.T) {
	fp := groupTestPanel(t)
	fp.semanticCatalogGeneration = 1
	fp.ensureSemanticPagedRevisions("vfs")
	initialCatalog, initialMetadata := fp.catalogRevision, fp.metadataRevision

	// Sorting, filtering, grouping, and column changes mutate the catalog
	// projection without changing the directory's content-read epoch.
	fp.semanticCatalogGeneration++
	fp.ensureSemanticPagedRevisions("vfs")
	if fp.catalogRevision != initialCatalog+1 {
		t.Fatalf("catalog revision = %d, want %d", fp.catalogRevision, initialCatalog+1)
	}
	if fp.metadataRevision != initialMetadata {
		t.Fatalf("presentation change advanced metadata revision to %d", fp.metadataRevision)
	}

	// A committed directory/content generation still invalidates metadata
	// chunks that belong to the previous contents.
	fp.mediaSourceEpoch++
	fp.semanticCatalogGeneration++
	fp.ensureSemanticPagedRevisions("vfs")
	if fp.metadataRevision != initialMetadata+1 {
		t.Fatalf("content generation metadata revision = %d, want %d", fp.metadataRevision, initialMetadata+1)
	}
}

func TestFileFieldSortKeepsUnknownAndMissingLastInBothDirections(t *testing.T) {
	fp := groupTestPanel(t)
	known800 := fieldTestEntry("iso800.jpg", 800)
	known100 := fieldTestEntry("iso100.jpg", 100)
	missing := &FileEntry{VFSItem: vfs.VFSItem{Name: "missing.jpg"}, FileFieldsSourceKey: "src", FileFieldsSourceVersion: "v1", FileFieldsComplete: true,
		FileFields: map[string]extui.FileFieldValue{"exif.iso": {State: extui.FileFieldMissing, Kind: extui.FileFieldInteger}}}
	unread := &FileEntry{VFSItem: vfs.VFSItem{Name: "unread.jpg"}}
	fp.Entries = []*FileEntry{unread, missing, known800, known100}
	stampFileFieldEntry(fp, known800)
	stampFileFieldEntry(fp, known100)
	stampFileFieldEntry(fp, missing)
	if !fp.SetFileFieldSort("exif.iso") {
		t.Fatal("field sort rejected")
	}
	assertPanelEntryNames(t, fp.Entries, "iso100.jpg", "iso800.jpg", "missing.jpg", "unread.jpg")
	if !fp.SetFileFieldSort("exif.iso") {
		t.Fatal("reverse field sort rejected")
	}
	assertPanelEntryNames(t, fp.Entries, "iso800.jpg", "iso100.jpg", "missing.jpg", "unread.jpg")
}

func TestApplyFileFieldsUpdateSurvivesSortAndRejectsStaleDirectory(t *testing.T) {
	fp := groupTestPanel(t)
	fp.mediaSourceEpoch = 7
	// This VFS reports neither an opaque revision nor file size/mtime. Its
	// identity must therefore use the directory observation epoch, not the
	// catalog revision that changes when the presentation is sorted or filtered.
	entry := &FileEntry{VFSItem: vfs.VFSItem{Name: "photo.jpg"}}
	fp.Entries = []*FileEntry{entry}
	key, version := fp.fileFieldIdentity(entry)
	update := map[string]any{
		"panelId": vtui.SemanticID(fp), "generation": int64(7),
		"sourceKey": key, "sourceVersion": version, "complete": true,
		"values": map[string]any{"exif.iso": uint32(640)},
	}
	fp.catalogRevision++ // presentation/catalog revision is independent of content generation
	if currentKey, currentVersion := fp.fileFieldIdentity(entry); currentKey != key || currentVersion != version {
		t.Fatalf("sorting/filtering changed an unversioned source identity: key %q/%q version %q/%q",
			key, currentKey, version, currentVersion)
	}
	if !fp.ApplyFileFieldsUpdate(update) {
		t.Fatal("current result was discarded after a representation revision")
	}
	if value := entry.FileFields["exif.iso"]; value.State != extui.FileFieldKnown || value.Integer != 640 {
		t.Fatalf("ISO value was not applied: %#v", value)
	}
	if entry.FileFields["exif.camera_model"].State != extui.FileFieldMissing {
		t.Fatal("a completed read did not record absent tags as missing")
	}
	if fp.ApplyFileFieldsUpdate(update) {
		t.Fatal("an identical result caused another state transition")
	}
	stale := make(map[string]any, len(update))
	for name, value := range update {
		stale[name] = value
	}
	stale["generation"] = int64(6)
	if fp.ApplyFileFieldsUpdate(stale) {
		t.Fatal("result from the old directory generation was accepted")
	}
}

func TestApplyFileFieldsUpdatesReconcilesOnceForThumbnailBatch(t *testing.T) {
	fp := groupTestPanel(t)
	fp.mediaSourceEpoch = 12
	first := &FileEntry{VFSItem: vfs.VFSItem{Name: "first.jpg", Revision: "first-v1"}}
	second := &FileEntry{VFSItem: vfs.VFSItem{Name: "second.jpg", Revision: "second-v1"}}
	fp.Entries = []*FileEntry{first, second}
	updates := make([]map[string]any, 0, 2)
	for index, entry := range fp.Entries {
		key, version := fp.fileFieldIdentity(entry)
		updates = append(updates, map[string]any{
			"panelId": vtui.SemanticID(fp), "generation": int64(12),
			"sourceKey": key, "sourceVersion": version, "complete": true,
			"values": map[string]any{"exif.iso": uint32(400 + index*400)},
		})
	}
	if !fp.SetFileFieldSort("exif.iso") {
		t.Fatal("field sort rejected")
	}
	before := fp.semanticCatalogGeneration
	if !fp.ApplyFileFieldsUpdates(updates) {
		t.Fatal("thumbnail field batch was ignored")
	}
	if fp.semanticCatalogGeneration != before+1 {
		t.Fatalf("one two-result batch caused %d catalog reconciliations, want 1",
			fp.semanticCatalogGeneration-before)
	}
	if first.FileFields["exif.iso"].Integer != 400 ||
		second.FileFields["exif.iso"].Integer != 800 {
		t.Fatalf("batch fields not applied: first=%#v second=%#v",
			first.FileFields, second.FileFields)
	}
}

func TestFileFieldPanelSessionRoundTripAndLegacyDefaults(t *testing.T) {
	state := PanelGallerySessionState{
		LayoutMode: GalleryLayoutDetails, ColumnCount: 2,
		FileFieldColumns: []FileFieldColumn{{FieldID: "exif.iso", Width: 15}, {FieldID: "exif.camera_model", Width: 24}},
		GalleryColumnWidths: map[string]int{
			"name": 46, "size": 13, "exif.iso": 18,
		},
		FileFieldFilters:   []FileFieldFilter{{FieldID: "exif.exposure_time", Operation: "le", Value: "1/125"}},
		FileFieldFilterAny: true, FileFieldSort: "exif.iso",
		FileFieldGroup: "exif.lens_focal_range", FileFieldGroupReverse: true,
	}
	var encoded strings.Builder
	WritePanelGallerySessionState(&encoded, state)
	parsed := ini.Parse(strings.NewReader("[LeftPanel]\n" + encoded.String()))
	got := LoadUnifiedPanelGallerySessionState(parsed, "LeftPanel", ViewModeDetailed)
	if !reflect.DeepEqual(got.FileFieldColumns, state.FileFieldColumns) ||
		!reflect.DeepEqual(got.GalleryColumnWidths, state.GalleryColumnWidths) ||
		!reflect.DeepEqual(got.FileFieldFilters, state.FileFieldFilters) ||
		got.FileFieldSort != state.FileFieldSort || got.FileFieldGroup != state.FileFieldGroup ||
		!got.FileFieldFilterAny || !got.FileFieldGroupReverse {
		t.Fatalf("file field session did not round-trip: %#v", got)
	}
	legacy := LoadUnifiedPanelGallerySessionState(ini.New(), "LeftPanel", ViewModeDetailed)
	if len(legacy.FileFieldColumns) != 0 || len(legacy.GalleryColumnWidths) != 0 ||
		len(legacy.FileFieldFilters) != 0 ||
		legacy.FileFieldSort != "" || legacy.FileFieldGroup != "" {
		t.Fatalf("legacy session did not use empty file field defaults: %#v", legacy)
	}
}

func TestGalleryColumnWidthsChangePresentationAndPersistWithoutCatalogReload(t *testing.T) {
	fp := groupTestPanel(t)
	if !fp.SetFileFieldColumns([]FileFieldColumn{{
		FieldID: "exif.iso", Width: 12,
	}}) {
		t.Fatal("failed to configure the EXIF column")
	}
	beforeGeneration := fp.semanticCatalogGeneration
	widths := []GalleryColumnWidth{
		{ID: "name", Width: 44},
		{ID: "size", Width: 14},
		{ID: "exif.iso", Width: 22},
	}
	if !fp.SetGalleryColumnWidths(widths) {
		t.Fatal("valid gallery column widths were rejected")
	}
	if fp.semanticCatalogGeneration != beforeGeneration {
		t.Fatalf("resizing columns reloaded the catalog: generation %d -> %d",
			beforeGeneration, fp.semanticCatalogGeneration)
	}
	columns := fp.semanticGalleryColumns()
	if len(columns) != 3 || columns[0].Width != 44 ||
		columns[1].Width != 14 || columns[2].Width != 22 {
		t.Fatalf("semantic widths = %#v, want name=44 size=14 iso=22", columns)
	}
	if fp.FileFieldColumns[0].Width != 12 {
		t.Fatalf("generic resize rewrote the legacy field preset: %#v",
			fp.FileFieldColumns)
	}
	if !fp.SetGalleryColumnWidths(widths) ||
		fp.semanticCatalogGeneration != beforeGeneration {
		t.Fatal("an identical resize was not idempotent")
	}
	if fp.SetGalleryColumnWidths([]GalleryColumnWidth{{ID: "exif.unknown", Width: 4}}) {
		t.Fatal("unknown field width was accepted")
	}
	if fp.SetGalleryColumnWidths([]GalleryColumnWidth{{ID: "size", Width: 0}}) {
		t.Fatal("zero width was accepted")
	}
	if got := fp.GalleryColumnWidths["size"]; got != 14 {
		t.Fatalf("rejected resize changed the saved size width to %d", got)
	}
}

func TestSemanticGalleryColumnWidthsParseQMLAction(t *testing.T) {
	widths, ok := semanticGalleryColumnWidthsFromAction([]any{
		map[string]any{"id": "name", "width": 44.0},
		map[string]any{"id": "size", "width": 14},
	})
	if !ok || !reflect.DeepEqual(widths, []GalleryColumnWidth{
		{ID: "name", Width: 44}, {ID: "size", Width: 14},
	}) {
		t.Fatalf("parsed gallery widths = %#v, %v", widths, ok)
	}
}

func fieldTestEntry(name string, iso int64) *FileEntry {
	return &FileEntry{VFSItem: vfs.VFSItem{Name: name},
		FileFields: map[string]extui.FileFieldValue{"exif.iso": {State: extui.FileFieldKnown, Kind: extui.FileFieldInteger, Integer: iso}}}
}

func stampFileFieldEntry(fp *FileSystemPanel, entry *FileEntry) {
	entry.FileFieldsSourceKey, entry.FileFieldsSourceVersion = fp.fileFieldIdentity(entry)
	entry.FileFieldsGeneration = fp.mediaSourceEpoch
}

func assertPanelEntryNames(t *testing.T, entries []*FileEntry, want ...string) {
	t.Helper()
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entry order = %v, want %v", got, want)
	}
}
