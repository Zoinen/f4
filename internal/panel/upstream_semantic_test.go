package panel

import (
	config "github.com/unxed/f4/internal/config"
	sysinfo "github.com/unxed/f4/internal/sysinfo"
	androidfs "github.com/unxed/f4/plugins/android"
	iosfs "github.com/unxed/f4/plugins/ios"
	extui "github.com/unxed/f4/sdk/extui"
	vfs "github.com/unxed/f4/vfs"
	reflect "reflect"
	testing "testing"
)

func TestNativePanelStatusCountsCalculatedDirectoriesAndSortGroups(t *testing.T) {
	panel := &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir()), UseSortGroups: true,
		Entries: []*FileEntry{
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
	panel.Entries[1].Selected = false
	panel.selectionRevision++
	panel.enrichNativePanelStatus(&model)
	if model.SelectedSize != 100 || model.SelectedFiles != 0 {
		t.Fatal("selection totals did not update")
	}
}

func TestNativeDriveDetailsPreserveUnpaddedFields(t *testing.T) {
	row := DriveMenuPlatformRow{Base: "D:", Kind: "Fixed", Label: "Work disk", Filesystem: "NTFS", Total: "2 TB", Free: "1 TB", network: "server"}
	details := row.semanticDetails()
	if len(details) != 9 || details["label"] != "Work disk" || details["free"] != "1 TB" {
		t.Fatalf("details = %+v", details)
	}
	item := extui.MenuItemModel{Text: "console padded label", Details: details}
	if !reflect.DeepEqual(item.ToMap()["details"], details) {
		t.Fatal("menu serialization lost structured fields")
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
		row := DriveMenuPlatformRow{Total: "rounded total", totalBytes: tc.total, freeBytes: tc.free}
		if got := row.semanticDetails()["usedFraction"]; got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
		row.Total = ""
		if _, ok := row.semanticDetails()["usedFraction"]; ok {
			t.Fatal("hidden capacity leaked")
		}
	}
}

type panelIconTestVFS struct{ vfs.VFS }

func (*panelIconTestVFS) PanelIcon() string { return "custom-plugin" }

func TestNativePanelIconFollowsVFS(t *testing.T) {
	row := driveMenuPlatformRowFor(sysinfo.DriveEntry{Name: "Windows Registry", Icon: "blocks"}, config.DefaultDriveMenuOptions)
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
	}
}
