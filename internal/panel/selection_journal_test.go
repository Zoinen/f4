package panel

import (
	semantic "github.com/unxed/f4/internal/semantic"
	extui "github.com/unxed/f4/sdk/extui"
	testing "testing"
)

func TestFilePanelSelectionJournalKeepsOneSparseChangeForLargeCatalog(t *testing.T) {
	const entryCount = 20_000
	entries := make([]*FileEntry, entryCount)
	staticEntries := make([]extui.FileEntryModel, entryCount)
	for index := range entries {
		entries[index] = &FileEntry{}
	}
	staticEntries[entryCount-1].EntryID = "entry-last"
	panel := &FileSystemPanel{
		Entries: entries, catalogRevision: 11, selectionRevision: 5,
		semanticStaticCache: &semanticPanelStaticCache{
			catalogRevision: 11, entries: staticEntries,
		},
	}

	panel.SetItemSelected(entryCount-1, true)
	patch, ok := panel.SemanticSelectionPatch(5)
	if !ok || patch.Op != "selection_delta" ||
		patch.BaseSelection != 5 || patch.SelectionRevision != 6 {
		t.Fatalf("selection journal patch = %#v, ok=%v", patch, ok)
	}
	if len(patch.SelectionChanges) != 1 ||
		semantic.Int(patch.SelectionChanges[0]["index"]) != entryCount-1 {
		t.Fatalf("selection delta carried %d changes: %#v",
			len(patch.SelectionChanges), patch.SelectionChanges)
	}
}
