package cloudfox

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestManagerVFSRootAndCapabilityMethods(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	if !m.IsAtRoot() || m.GetPath() != managerVisualRoot() || m.GetTitle() != DriveName {
		t.Fatalf("manager root identity is inconsistent: path=%q title=%q", m.GetPath(), m.GetTitle())
	}
	if m.PanelTitle("ignored") != defaultManagerStrings().Title {
		t.Fatalf("PanelTitle = %q", m.PanelTitle("ignored"))
	}
	if m.ParentVFS() != nil {
		t.Fatal("manager must not have a parent VFS")
	}
	if m.GetCapabilities() != (vfs.VFSCapabilities{}) {
		t.Fatalf("manager capabilities = %#v", m.GetCapabilities())
	}
}

func TestManagerVFSPathConversions(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	for _, p := range []string{ManagerRoot, DriveName + ":/saved", managerVisualRoot() + "saved"} {
		if !m.IsAbs(p) {
			t.Errorf("IsAbs(%q) = false", p)
		}
	}
	if got := m.Join(ManagerRoot, "saved", "nested"); got != managerVisualRoot()+"saved"+string(os.PathSeparator)+"nested" {
		t.Fatalf("Join = %q", got)
	}
	if got, err := m.Abs("saved"); err != nil || got != managerVisualRoot()+"saved" {
		t.Fatalf("Abs relative = %q, %v", got, err)
	}
	if got := m.Base(DriveName + ":/saved/nested"); got != "nested" {
		t.Fatalf("Base = %q", got)
	}
	if got := m.Dir("ignored"); got != managerVisualRoot() {
		t.Fatalf("Dir = %q", got)
	}
}

func TestManagerVFSSetPathAcceptsOnlyRoot(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	for _, p := range []string{"", ".", "/", ManagerRoot, managerVisualRoot()} {
		if err := m.SetPath(p); err != nil {
			t.Errorf("SetPath(%q) = %v", p, err)
		}
	}
	if err := m.SetPath("saved"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SetPath(non-root) = %v, want not-exist", err)
	}
}

func TestManagerVFSReadDirWithoutRepository(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	if err := m.ReadDir(context.Background(), managerVisualRoot(), nil); err == nil {
		t.Fatal("ReadDir without repository must fail")
	}
}

func TestManagerVFSStatRootAndAddConnection(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	for _, p := range []string{"", ".", "/", ManagerRoot, managerVisualRoot()} {
		item, err := m.Stat(context.Background(), p)
		if err != nil || !item.IsDir || item.Name != DriveName {
			t.Errorf("Stat(%q) = %#v, %v", p, item, err)
		}
	}
	item, err := m.Stat(context.Background(), managerVisualRoot()+m.strings.AddConnection)
	if err != nil || !item.IsExecutable || !item.NoExtension || item.Name != m.strings.AddConnection {
		t.Fatalf("Stat(add row) = %#v, %v", item, err)
	}
}

func TestManagerVFSReadOnlyMethods(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	ctx := context.Background()
	if err := m.MkDir(ctx, "x"); !errors.Is(err, ErrManagerReadOnly) {
		t.Errorf("MkDir = %v", err)
	}
	if err := m.SetAttributes(ctx, "x", vfs.VFSItem{}); !errors.Is(err, ErrManagerReadOnly) {
		t.Errorf("SetAttributes = %v", err)
	}
	if _, err := m.Search(ctx, "", ""); !errors.Is(err, ErrManagerReadOnly) {
		t.Errorf("Search = %v", err)
	}
	if _, err := m.Open(ctx, "x"); !errors.Is(err, ErrManagerReadOnly) {
		t.Errorf("Open = %v", err)
	}
	if _, err := m.Create(ctx, "x"); !errors.Is(err, ErrManagerReadOnly) {
		t.Errorf("Create = %v", err)
	}
}

func TestManagerVFSCloneStartsWithEmptyRows(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	m.rows["saved"] = Connection{Name: "saved"}
	clone, ok := m.Clone().(*ManagerVFS)
	if !ok {
		t.Fatal("Clone did not return ManagerVFS")
	}
	if len(clone.rows) != 0 {
		t.Fatalf("clone rows = %d, want empty", len(clone.rows))
	}
}

func TestManagerVFSHandleActionsWithoutEditor(t *testing.T) {
	m := NewManagerVFS(nil, nil)
	if m.HandlePanelAction(nil, vfs.PanelActionCreate, nil) {
		t.Fatal("create without editor must not be handled")
	}
	if m.HandlePanelAction(nil, vfs.PanelActionActivate, []string{m.strings.AddConnection}) {
		t.Fatal("activate without editor must not be handled")
	}
	if m.HandlePanelAction(nil, vfs.PanelActionEdit, []string{"saved"}) {
		t.Fatal("edit without editor must not be handled")
	}
	if m.HandlePanelAction(nil, vfs.PanelActionActivate, nil) {
		t.Fatal("activate without a path must not be handled")
	}
	if m.HandlePanelAction(nil, vfs.PanelActionDelete, nil) != true {
		t.Fatal("delete with no paths must be handled as a no-op")
	}
}

func TestManagerVFSHandleCreateAndActivate(t *testing.T) {
	var calls int
	editor := ProfileEditorFunc(func(vfs.App, *ManagerVFS, *Connection) { calls++ })
	m := NewManagerVFS(nil, editor)
	if !m.HandlePanelAction(nil, vfs.PanelActionCreate, nil) {
		t.Fatal("create action was not handled")
	}
	if !m.HandlePanelAction(nil, vfs.PanelActionActivate, []string{m.strings.AddConnection}) {
		t.Fatal("activate add-row action was not handled")
	}
	if calls != 2 {
		t.Fatalf("editor calls = %d, want 2", calls)
	}
}

func TestManagerVFSHandleEditAndClose(t *testing.T) {
	var calls int
	editor := ProfileEditorFunc(func(vfs.App, *ManagerVFS, *Connection) { calls++ })
	m := NewManagerVFS(nil, editor)
	m.rows["saved"] = Connection{ID: "id", Name: "saved"}
	if m.HandlePanelAction(nil, vfs.PanelActionEdit, nil) {
		t.Fatal("edit with multiple/zero paths must not be handled")
	}
	if !m.HandlePanelAction(nil, vfs.PanelActionEdit, []string{m.strings.AddConnection}) {
		t.Fatal("edit of add row must be recognised")
	}
	if calls != 0 {
		t.Fatalf("editor calls after reserved edit = %d, want 0", calls)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if len(m.rows) != 0 {
		t.Fatal("Close must clear cached rows")
	}
}
