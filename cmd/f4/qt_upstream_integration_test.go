package main

import (
	androidfs "github.com/unxed/f4/plugins/android"
	iosfs "github.com/unxed/f4/plugins/ios"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"reflect"
	"testing"
)

func TestNativeSecondaryCaretsKeepBaseRowsAndSurviveModelConversion(t *testing.T) {
	previous := AppConfig.EditorMarkOccurrences
	AppConfig.EditorMarkOccurrences = false
	t.Cleanup(func() { AppConfig.EditorMarkOccurrences = previous })
	editor := projectionTestEditor(t, "abcdefghij\nsecond row\n")
	editor.highlighter = nil
	first := editor.SemanticNode(nil)
	beforeRows := editor.semanticStyledRowsRendered
	editor.extraCursors = []extraCaret{{off: 7, anchor: 3, hasSel: true}}
	second := editor.SemanticNode(nil)
	if !reflect.DeepEqual(first["windowRows"], second["windowRows"]) || editor.semanticStyledRowsRendered != beforeRows {
		t.Fatal("secondary selection repainted native base rows")
	}
	model := appSurfaceFromLegacy(second)
	if len(model.SecondaryCarets) != 1 || model.SecondaryCarets[0].CursorAbsoluteColumn != 7 || !model.SecondaryCarets[0].Selection {
		t.Fatalf("secondary caret lost in typed model: %+v", model.SecondaryCarets)
	}
	guard := editor.editorCursorStateGuard()
	editor.extraCursors[0].off++
	if !guard.canPublish(editor, true) {
		t.Fatal("secondary movement cannot publish compact cursor state")
	}
	third := editor.SemanticNode(nil)
	if first["windowContentKey"] != third["windowContentKey"] {
		t.Fatal("caret movement changed row content key")
	}
	editor.extraCursors = nil
	if len(appSurfaceFromLegacy(editor.SemanticNode(nil)).SecondaryCarets) != 0 {
		t.Fatal("removed caret retained")
	}
}

func TestNativePanelStatusCountsCalculatedDirectoriesAndSortGroups(t *testing.T) {
	panel := &FileSystemPanel{vfs: vfs.NewOSVFS(t.TempDir()), useSortGroups: true,
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "file", Size: 42}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true, Size: 100}, SizeCalculated: true, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "unknown", IsDir: true, Size: 900}, Selected: true},
		}}
	model := extui.PanelModel{MetadataDeferred: true}
	panel.enrichNativePanelStatus(&model)
	if model.SelectedFiles != 1 || model.SelectedDirectories != 2 || model.SelectedSize != 142 || model.TotalSize != 42 || !model.UseSortGroups {
		t.Fatalf("wrong status: %+v", model)
	}
	converted := appPanelFromLegacy(model.ToMap())
	if converted.SelectedSize != 142 || !converted.UseSortGroups {
		t.Fatal("deferred model dropped status")
	}
	panel.entries[1].Selected = false
	panel.selectionRevision++
	panel.enrichNativePanelStatus(&model)
	if model.SelectedSize != 100 || model.SelectedFiles != 0 {
		t.Fatal("selection totals did not update")
	}
}

func TestNativeDriveDetailsPreserveUnpaddedFields(t *testing.T) {
	row := driveMenuPlatformRow{base: "D:", kind: "Fixed", label: "Work disk", filesystem: "NTFS", total: "2 TB", free: "1 TB", network: "server"}
	details := row.semanticDetails()
	if len(details) != 9 || details["label"] != "Work disk" || details["free"] != "1 TB" {
		t.Fatalf("details = %+v", details)
	}
	item := extui.MenuItemModel{Text: "console padded label", Details: details}
	if !reflect.DeepEqual(item.ToMap()["details"], details) {
		t.Fatal("menu serialization lost structured fields")
	}
}

func TestNativeViewerContentRevisionInvalidatesPendingConstruction(t *testing.T) {
	viewer := cachedSemanticViewer([]byte("old text"))
	defer viewer.Close()
	before := viewer.constructionKey(0, 20, 4, 2)
	viewer.backend.DropCache()
	after := viewer.constructionKey(0, 20, 4, 2)
	if before == after {
		t.Fatal("same-size cache invalidation reused a pending construction key")
	}
}

func TestDriveSemanticCapacityAndIcons(t *testing.T) {
	for _, kind := range []driveMenuKind{driveMenuKindUnknown, driveMenuKindFixed, driveMenuKindRemovable, driveMenuKindCD, driveMenuKindRemote, driveMenuKindSubstitute, driveMenuKindPhysical, driveMenuKindRAM} {
		if driveMenuKindIcon(kind) == "" {
			t.Fatalf("missing icon for %v", kind)
		}
	}
	for _, tc := range []struct {
		total, free uint64
		want        string
	}{
		{0, 0, ""}, {100, 0, "1.000000000"}, {100, 25, "0.750000000"}, {100, 100, "0.000000000"}, {100, 200, "0.000000000"},
	} {
		row := driveMenuPlatformRow{total: "rounded total", totalBytes: tc.total, freeBytes: tc.free}
		if got := row.semanticDetails()["usedFraction"]; got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
		row.total = ""
		if _, ok := row.semanticDetails()["usedFraction"]; ok {
			t.Fatal("hidden capacity leaked")
		}
	}
}

type panelIconTestVFS struct{ vfs.VFS }

func (*panelIconTestVFS) PanelIcon() string { return "custom-plugin" }
func TestNativePanelIconFollowsVFS(t *testing.T) {
	row := driveMenuPlatformRowFor(DriveEntry{Name: "Windows Registry", Icon: "blocks"}, defaultDriveMenuOptions)
	if row.semanticDetails()["icon"] != "blocks" || row.semanticDetails()["isDrive"] != "false" {
		t.Fatal("registry drive icon was overwritten")
	}

	for _, tc := range []struct {
		fs   vfs.VFS
		icon string
	}{
		{&androidfs.ManagerVFS{}, "android-logo"}, {&androidfs.SyncVFS{}, "android-logo"},
		{&iosfs.ManagerVFS{}, "apple-logo"}, {&iosfs.CoreVFS{}, "apple-logo"},
		{&vfs.OSVFS{}, ""}, {&panelIconTestVFS{}, "custom-plugin"},
	} {
		if got := semanticPanelIcon(tc.fs); got != tc.icon {
			t.Fatalf("%T: %q", tc.fs, got)
		}
		model := extui.PanelModel{PathIcon: tc.icon}
		if got := appPanelFromLegacy(model.ToMap()).PathIcon; got != tc.icon {
			t.Fatalf("lost icon: %s", got)
		}
	}
}
