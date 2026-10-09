package panel

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestTempPanelStorePrivateOperations(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("temporary"), 0600); err != nil {
		t.Fatal(err)
	}
	source := vfs.NewOSVFS(root)

	if normalizeTempPanelSlot(-1) != 0 || normalizeTempPanelSlot(10) != 0 || normalizeTempPanelSlot(12) != 2 {
		t.Fatal("slot normalization is incorrect")
	}
	if got := tempPanelRoot(-1); got != "tmp:0" {
		t.Fatalf("tempPanelRoot(-1) = %q", got)
	}

	store := &TempPanelStore{}
	store.appendReferences(11, []tempPanelReference{
		{source: source, path: file},
		{source: source, path: file},
		{path: file},
	})
	refs := store.references(1)
	if len(refs) != 1 || refs[0].Id == 0 || refs[0].display != file {
		t.Fatalf("appendReferences = %#v", refs)
	}
	ref := refs[0]
	if got, ok := store.reference(1, ref.Id); !ok || got.path != file {
		t.Fatalf("reference lookup = %#v, %v", got, ok)
	}
	if _, ok := store.reference(1, ref.Id+1); ok {
		t.Fatal("missing reference unexpectedly found")
	}
	if got, ok := store.referenceByDisplay(1, file); !ok || got.Id != ref.Id {
		t.Fatalf("referenceByDisplay = %#v, %v", got, ok)
	}
	if !store.updateReference(1, ref.Id, func(updated *tempPanelReference) {
		updated.display = "renamed"
	}) {
		t.Fatal("updateReference did not update an existing item")
	}
	if store.updateReference(1, ref.Id+1, func(*tempPanelReference) {}) {
		t.Fatal("updateReference reported a missing item as present")
	}
	if !store.removeReference(1, ref.Id) || store.removeReference(1, ref.Id) {
		t.Fatal("removeReference result is incorrect")
	}

	store.ReplaceWithSearchResults(2, source, []FoundFile{
		{Path: file},
		{Path: file},
		{Path: ""},
	})
	if refs = store.references(2); len(refs) != 1 || refs[0].path != file {
		t.Fatalf("ReplaceWithSearchResults = %#v", refs)
	}

	full := &TempPanelStore{}
	for slot := 0; slot < tempPanelSlotCount; slot++ {
		full.appendReferences(slot, []tempPanelReference{{source: source, path: file}})
	}
	if got := full.SearchSlot(); got != 0 {
		t.Fatalf("first round-robin search slot = %d, want 0", got)
	}
	if got := full.SearchSlot(); got != 1 {
		t.Fatalf("second round-robin search slot = %d, want 1", got)
	}
	if (&TempPanelStore{}).SearchSlot() != 0 {
		t.Fatal("empty store did not choose slot 0")
	}
	var nilStore *TempPanelStore
	if nilStore.SearchSlot() != 0 || nilStore.references(0) != nil {
		t.Fatal("nil store did not return empty results")
	}

	addStore := &TempPanelStore{}
	tmp := NewTempPanelVFS(nil, addStore, 2)
	if err := tmp.AddReferences(context.Background(), source, []string{"file.txt", "missing.txt", ".."}); err == nil {
		t.Fatal("AddReferences hid the first missing source error")
	}
	if len(addStore.references(2)) != 1 {
		t.Fatalf("AddReferences kept unexpected references: %#v", addStore.references(2))
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tmp.AddReferences(canceled, source, []string{"file.txt"}); err != context.Canceled {
		t.Fatalf("canceled AddReferences error = %v", err)
	}
	if err := tmp.AddReferences(context.Background(), nil, []string{"file.txt"}); err == nil {
		t.Fatal("AddReferences accepted a nil source")
	}

	sourceTmp := NewTempPanelVFS(nil, &TempPanelStore{}, 0)
	if err := sourceTmp.AddReferences(context.Background(), source, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	sourceRef := sourceTmp.store.references(0)[0]
	targetTmp := NewTempPanelVFS(nil, &TempPanelStore{}, 1)
	if err := targetTmp.AddReferences(context.Background(), sourceTmp, []string{sourceRef.display, "missing"}); err == nil {
		t.Fatal("AddReferences from a temporary panel hid the missing-item error")
	}
	if len(targetTmp.store.references(1)) != 1 {
		t.Fatalf("temporary-panel AddReferences = %#v", targetTmp.store.references(1))
	}
}

func TestTempPanelVFSPrivateOperations(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	nested := filepath.Join(dir, "nested")
	file := filepath.Join(root, "file.txt")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("temporary"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "inside.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	source := vfs.NewOSVFS(root)
	store := &TempPanelStore{}
	tmp := NewTempPanelVFS(source, store, -1)
	if tmp.GetTitle() != "Temp" || !tmp.IsAtRoot() || tmp.GetPath() != "tmp:0" || !tmp.IsAbs("tmp:0/e/1") {
		t.Fatal("new temporary panel root is incorrect")
	}
	if tmp.ParentVFS() != source || tmp.PanelTitle("") == "" {
		t.Fatal("temporary panel metadata is incomplete")
	}
	if err := tmp.AddReferences(context.Background(), source, []string{"dir", "file.txt"}); err != nil {
		t.Fatal(err)
	}

	var dirRef, fileRef tempPanelReference
	for _, ref := range store.references(0) {
		switch ref.path {
		case filepath.Join(root, "dir"):
			dirRef = ref
		case file:
			fileRef = ref
		}
	}
	if dirRef.Id == 0 || fileRef.Id == 0 {
		t.Fatalf("stored references = %#v", store.references(0))
	}
	topDir := tmp.entryPath(dirRef.Id)
	topFile := tmp.entryPath(fileRef.Id)
	if ref, real, top, ok := tmp.resolve(tmp.root()); !ok || top || ref.Id != 0 || real != "" {
		t.Fatalf("root resolve = %#v, %q, %v, %v", ref, real, top, ok)
	}
	if ref, real, top, ok := tmp.resolve(topDir); !ok || !top || ref.Id != dirRef.Id || real != dir {
		t.Fatalf("directory resolve = %#v, %q, %v, %v", ref, real, top, ok)
	}
	if _, _, _, ok := tmp.resolve(tmp.root() + "/e/missing"); ok {
		t.Fatal("missing entry resolved")
	}
	if _, _, _, ok := tmp.resolve(topDir + "/bad"); ok {
		t.Fatal("malformed child path resolved")
	}

	child := tmp.Join(topDir, "nested")
	if ref, real, top, ok := tmp.resolve(child); !ok || top || ref.Id != dirRef.Id || real != nested {
		t.Fatalf("child resolve = %#v, %q, %v, %v", ref, real, top, ok)
	}
	newName := tmp.newNamePath("new name.txt")
	if got, ok := tmp.parseNewName(newName); !ok || got != "new name.txt" {
		t.Fatalf("new-name path = %q, %v", got, ok)
	}
	if _, ok := tmp.parseNewName(tmp.root() + "/n/not-base64!"); ok {
		t.Fatal("invalid new-name path parsed")
	}
	if got := tmp.Join(tmp.root(), dirRef.display); got != topDir {
		t.Fatalf("Join did not resolve a displayed reference: %q", got)
	}
	if got := tmp.Join(tmp.root(), "new item"); got != tmp.newNamePath("new item") {
		t.Fatalf("Join did not create a new-name path: %q", got)
	}
	if got := tmp.Join(child, ".."); got != topDir {
		t.Fatalf("Join(..) = %q, want %q", got, topDir)
	}
	if got, err := tmp.Abs(""); err != nil || got != tmp.GetPath() {
		t.Fatalf("Abs(empty) = %q, %v", got, err)
	}
	if got, err := tmp.Abs(topFile); err != nil || got != topFile {
		t.Fatalf("Abs(absolute) = %q, %v", got, err)
	}
	if tmp.Base(tmp.root()) != "" || tmp.Base(topDir) != "dir" || tmp.Base(newName) != "new name.txt" || tmp.Dir(child) != topDir {
		t.Fatalf("Base/Dir results are incorrect")
	}

	if err := tmp.SetPath(topDir); err != nil {
		t.Fatalf("SetPath(directory) = %v", err)
	}
	if err := tmp.SetPath(child); err != nil {
		t.Fatalf("SetPath(child directory) = %v", err)
	}
	if err := tmp.SetPath(topFile); err == nil {
		t.Fatal("SetPath accepted a file")
	}
	if err := tmp.SetPath(tmp.root()); err != nil {
		t.Fatal(err)
	}
	if err := tmp.SetPath(tmp.root() + "/e/missing"); err == nil {
		t.Fatal("SetPath accepted a missing item")
	}

	var rootItems, childItems []vfs.VFSItem
	if err := tmp.ReadDir(context.Background(), tmp.root(), func(items []vfs.VFSItem) { rootItems = append(rootItems, items...) }); err != nil {
		t.Fatal(err)
	}
	if err := tmp.ReadDir(context.Background(), topDir, func(items []vfs.VFSItem) { childItems = append(childItems, items...) }); err != nil {
		t.Fatal(err)
	}
	if len(rootItems) != 2 || len(childItems) != 1 {
		t.Fatalf("ReadDir root=%d child=%d", len(rootItems), len(childItems))
	}
	if item, err := tmp.Stat(context.Background(), topFile); err != nil || item.Name != "file.txt" {
		t.Fatalf("Stat(file) = %#v, %v", item, err)
	}
	if _, err := tmp.Stat(context.Background(), tmp.root()+"/e/missing"); err == nil {
		t.Fatal("Stat accepted a missing item")
	}
	if !tmp.GetCapabilities().HasRandomAccess {
		t.Fatal("temporary panel lacks random access capability")
	}
	if _, err := tmp.Search(context.Background(), tmp.root(), "x"); err == nil || err != os.ErrInvalid {
		t.Fatalf("Search error = %v", err)
	}
	if _, err := tmp.Create(context.Background(), topFile); err == nil || err != os.ErrPermission {
		t.Fatalf("Create error = %v", err)
	}
	if err := tmp.MkDir(context.Background(), tmp.root()+"/n/new"); err != os.ErrPermission {
		t.Fatalf("MkDir error = %v", err)
	}
	if tmp.referenceSource(topFile) != fileRef.source || tmp.referenceSource("bad") != nil {
		t.Fatal("referenceSource result is incorrect")
	}
	if clone := tmp.Clone(); clone == nil || clone.GetPath() != tmp.root() || clone.ParentVFS() != nil {
		t.Fatal("Clone result is incorrect")
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	if !tmp.RemovePanelReferences([]string{topFile}) || len(store.references(0)) != 1 {
		t.Fatal("RemovePanelReferences did not remove only the requested reference")
	}

	if err := tmp.AddReferences(context.Background(), source, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	var renamedRef tempPanelReference
	for _, ref := range store.references(0) {
		if ref.path == file {
			renamedRef = ref
		}
	}
	oldPath := tmp.entryPath(renamedRef.Id)
	if err := tmp.Rename(context.Background(), oldPath, tmp.newNamePath("renamed.txt")); err != nil {
		t.Fatalf("Rename = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Remove(context.Background(), oldPath); err != nil {
		t.Fatalf("Remove renamed item = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); !os.IsNotExist(err) {
		t.Fatalf("renamed source still exists: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "lost.txt"), []byte("lost"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := tmp.AddReferences(context.Background(), source, []string{"lost.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "lost.txt")); err != nil {
		t.Fatal(err)
	}
	if err := tmp.ReadDir(context.Background(), tmp.root(), nil); err != nil {
		t.Fatal(err)
	}
	for _, ref := range store.references(0) {
		if ref.path == filepath.Join(root, "lost.txt") {
			t.Fatal("missing reference was not removed after ReadDir")
		}
	}

	for _, tc := range []struct {
		e    *vtinput.InputEvent
		slot int
		ok   bool
	}{
		{nil, 0, false},
		{&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_3}, 3, true},
		{&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_NUMPAD4}, 4, true},
		{&vtinput.InputEvent{Char: '7'}, 7, true},
		{&vtinput.InputEvent{Char: 'x'}, 0, false},
	} {
		if got, ok := tempPanelSlotKey(tc.e); got != tc.slot || ok != tc.ok {
			t.Fatalf("tempPanelSlotKey(%#v) = %d, %v", tc.e, got, ok)
		}
	}
}
