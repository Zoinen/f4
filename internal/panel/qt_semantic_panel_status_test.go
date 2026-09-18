package panel

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestNativePanelStatusTotalsAndCapacity(t *testing.T) {
	source := vfs.NewOSVFS(t.TempDir())
	fp := &FileSystemPanel{
		Vfs: source, Table: vtui.NewTable(0, 0, 40, 10, nil),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "a", Size: 100}, Selected: true},
			{VFSItem: vfs.VFSItem{Name: "b", Size: 200}},
			{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true, Size: 400}, Selected: true, SizeCalculated: true},
		},
	}
	fp.nativeStatus = nativePanelStatusCache{
		source: source, path: source.GetPath(), free: 250, capacity: 1000, freeKnown: true, querying: true,
	}
	model := extui.PanelModel{}
	fp.enrichNativePanelStatus(&model)
	if model.TotalFiles != 2 || model.TotalDirectories != 1 || model.TotalSize != 300 ||
		model.SelectedFiles != 1 || model.SelectedDirectories != 1 || model.SelectedSize != 500 {
		t.Fatalf("incorrect totals: %+v", model)
	}
	if !model.FreeSpaceKnown || model.FreeSpace != 250 || model.DiskTotalSpace != 1000 {
		t.Fatalf("missing disk capacity with file info disabled: %+v", model)
	}
	fp.Entries[1].Selected = false
	fp.selectionRevision++
	model = extui.PanelModel{}
	fp.enrichNativePanelStatus(&model)
	if model.SelectedFiles != 0 || model.SelectedSize != 400 || model.TotalFiles != 2 {
		t.Fatalf("selection refresh: %+v", model)
	}
	fp.Vfs = vfs.NewOSVFS(source.GetPath())
	model = extui.PanelModel{}
	fp.enrichNativePanelStatus(&model)
	if model.FreeSpaceKnown || model.DiskTotalSpace != 0 {
		t.Fatal("replacement source reused old capacity")
	}
}

func TestNativePanelStatusSelectionUpdatesCachedTotals(t *testing.T) {
	fp := &FileSystemPanel{
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "a", Size: 100}},
			{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true, Size: 400}, SizeCalculated: true},
		},
		nativeStatus: nativePanelStatusCache{initialized: true, totalFiles: 1, totalDirectories: 1, totalSize: 100},
	}
	fp.SetItemSelected(0, true)
	fp.SetItemSelected(1, true)
	if fp.nativeStatus.selection != fp.selectionRevision || fp.nativeStatus.files != 1 || fp.nativeStatus.directories != 1 || fp.nativeStatus.selectedSize != 500 {
		t.Fatalf("selection must update the cache without an exporter rescan: %+v", fp.nativeStatus)
	}
	fp.SetItemSelected(0, false)
	fp.SetItemSelected(1, false)
	if fp.nativeStatus.selectedSize != 0 || fp.nativeStatus.files != 0 || fp.nativeStatus.directories != 0 || fp.nativeStatus.totalSize != 100 {
		t.Fatalf("incorrect deselection totals: %+v", fp.nativeStatus)
	}
}

func BenchmarkNativePanelStatusSelection(b *testing.B) {
	for _, count := range []int{100, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			fp := &FileSystemPanel{
				Vfs: vfs.NewOSVFS(b.TempDir()), Table: vtui.NewTable(0, 0, 40, 10, nil),
				Entries: make([]*FileEntry, count),
			}
			for i := range fp.Entries {
				fp.Entries[i] = &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprint(i), Size: 100}}
			}
			var model extui.PanelModel
			fp.enrichNativePanelStatus(&model)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fp.SetItemSelected(0, i%2 == 0)
				fp.enrichNativePanelStatus(&model)
			}
		})
	}
}

func TestLiveSelectionValidatesIndexHintsBeforeApplying(t *testing.T) {
	old := semantic.SetPanelCatalogRowsEnabled(true)
	defer semantic.SetPanelCatalogRowsEnabled(old)
	fp := &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir()),
		Table: vtui.NewTable(0, 0, 40, 10, nil),
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "a", Size: 10}},
			{VFSItem: vfs.VFSItem{Name: "b", Size: 20}},
		},
	}
	fp.updateSemanticRevisions()
	id, _ := fp.semanticEntryMetadata(fp.Entries[1], "local")
	change := map[string]any{"entryId": id, "index": 0, "selected": true}
	action := map[string]any{"mode": "set", "catalogRevision": fp.catalogRevision,
		"changes": []map[string]any{change}}
	if fp.applySemanticSelection(action) {
		t.Fatal("accepted mismatched identity/index")
	}
	change["index"] = 1
	if !fp.applySemanticSelection(action) || !fp.Entries[1].Selected || fp.Entries[0].Selected {
		t.Fatal("valid sparse selection rejected")
	}
	action["catalogRevision"] = fp.catalogRevision - 1
	change["selected"] = false
	if fp.applySemanticSelection(action) || !fp.Entries[1].Selected {
		t.Fatal("accepted stale catalog")
	}
}

func BenchmarkLiveSelectionTransaction(b *testing.B) {
	old := semantic.SetPanelCatalogRowsEnabled(true)
	defer semantic.SetPanelCatalogRowsEnabled(old)
	for _, count := range []int{100, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			fp := &FileSystemPanel{Vfs: vfs.NewOSVFS(b.TempDir()),
				Table: vtui.NewTable(0, 0, 40, 10, nil), Entries: make([]*FileEntry, count)}
			for i := range fp.Entries {
				fp.Entries[i] = &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprint(i), Size: 100}}
			}
			fp.updateSemanticRevisions()
			var model extui.PanelModel
			fp.enrichNativePanelStatus(&model)
			id, _ := fp.semanticEntryMetadata(fp.Entries[0], "local")
			change := map[string]any{"entryId": id, "index": 0, "selected": false}
			action := map[string]any{"mode": "set", "catalogRevision": fp.catalogRevision,
				"changes": []map[string]any{change}, "cursorEntryId": id, "cursorIndex": 0}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				change["selected"] = i%2 == 0
				if !fp.applySemanticSelection(action) {
					b.Fatal("rejected transaction")
				}
				fp.enrichNativePanelStatus(&model)
			}
		})
	}
}
