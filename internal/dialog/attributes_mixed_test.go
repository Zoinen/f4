package dialog

import (
	"context"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"strings"
	"testing"
	"time"
)

func TestAttributesDialog_UnixMixedPermissionsPreserveOnSet(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	type attrCall struct {
		path string
		item vfs.VFSItem
	}
	var calls []attrCall
	mockVFS := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			calls = append(calls, attrCall{path: path, item: item})
		},
	}
	targets := []AttributesTarget{
		{Path: "first.txt", Item: vfs.VFSItem{Name: "first.txt", UnixMode: 0644, MTime: time.Now()}},
		{Path: "second.txt", Item: vfs.VFSItem{Name: "second.txt", UnixMode: 0600, MTime: time.Now()}},
	}

	ShowAttributesUnixForTargets(nil, mockVFS, targets)
	dlg := fm.GetTopFrame().(vtui.Container)
	var checks []*vtui.Checkbox
	var setButton *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		if c, ok := el.(*vtui.Checkbox); ok {
			checks = append(checks, c)
		}
		if b, ok := el.(*vtui.Button); ok && strings.Contains(b.GetText(), "Set") {
			setButton = b
		}
		return true
	})
	if len(checks) != 9 || setButton == nil {
		t.Fatalf("Unix mixed-permission dialog controls: got %d checkboxes and set button %v, want 9 and present", len(checks), setButton != nil)
	}
	// The fourth checkbox is Group: Read; it differs between 0644 and 0600.
	if !checks[3].ThreeState || checks[3].State != 2 {
		t.Fatalf("mixed Group: Read state = %d (three-state=%v), want 2 in three-state mode", checks[3].State, checks[3].ThreeState)
	}

	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	if len(calls) != 2 {
		t.Fatalf("SetAttributes called %d times, want 2", len(calls))
	}
	for _, call := range calls {
		want := uint32(0600)
		if call.path == "first.txt" {
			want = 0644
		}
		if call.item.UnixMode != want {
			t.Errorf("%s mode = %04o, want %04o", call.path, call.item.UnixMode, want)
		}
	}
}

func TestAttributesDialog_WindowsMixedFlagsPreserveOnSet(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	type attrCall struct {
		path string
		item vfs.VFSItem
	}
	var calls []attrCall
	mockVFS := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			calls = append(calls, attrCall{path: path, item: item})
		},
	}
	targets := []AttributesTarget{
		{Path: "first.txt", Item: vfs.VFSItem{Name: "first.txt", WinAttrs: 1, MTime: time.Now()}},
		{Path: "second.txt", Item: vfs.VFSItem{Name: "second.txt", WinAttrs: 0, MTime: time.Now()}},
	}

	ShowAttributesWindowsForTargets(nil, mockVFS, targets)
	dlg := fm.GetTopFrame().(vtui.Container)
	var chkRO, chkHidden *vtui.Checkbox
	var setButton *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		if c, ok := el.(*vtui.Checkbox); ok {
			switch {
			case strings.Contains(c.GetText(), "Read only"):
				chkRO = c
			case strings.Contains(c.GetText(), "Hidden"):
				chkHidden = c
			}
		}
		if b, ok := el.(*vtui.Button); ok && strings.Contains(b.GetText(), "Set") {
			setButton = b
		}
		return true
	})
	if chkRO == nil || chkHidden == nil || setButton == nil {
		t.Fatal("Windows mixed-flag dialog controls not found")
	}
	if !chkRO.ThreeState || chkRO.State != 2 {
		t.Fatalf("mixed Read only state = %d (three-state=%v), want 2 in three-state mode", chkRO.State, chkRO.ThreeState)
	}

	// Change an unambiguous flag, but leave the mixed Read only flag as '?'.
	chkHidden.State = 1
	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	if len(calls) != 2 {
		t.Fatalf("SetAttributes called %d times, want 2", len(calls))
	}
	for _, call := range calls {
		want := uint32(2)
		if call.path == "first.txt" {
			want = 3
		}
		if call.item.WinAttrs != want {
			t.Errorf("%s attributes = %#x, want %#x", call.path, call.item.WinAttrs, want)
		}
	}
}

// walkUI is a local helper to find elements in nested containers
func walkUI(el vtui.UIElement, fn func(vtui.UIElement) bool) bool {
	if !fn(el) {
		return false
	}
	if c, ok := el.(vtui.Container); ok {
		for _, child := range c.GetChildren() {
			if !walkUI(child, fn) {
				return false
			}
		}
	}
	return true
}

type mockMetadataVFS struct {
	vfs.VFS
	onSetAttr     func(vfs.VFSItem)
	onSetAttrPath func(string, vfs.VFSItem)
	statErr       error
	setAttrErr    error
	statToReturn  vfs.VFSItem
}

// runUITasksUntil pumps the frame manager's task channel until done reports
// true. A dialog's Set button posts its work as a UI task, and no manager is
// running one in a test.
func runUITasksUntil(t *testing.T, taskChan <-chan func(), done func() bool) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for !done() {
		select {
		case task := <-taskChan:
			task()
		case <-timeout:
			t.Fatal("timeout waiting for UI task")
		}
	}
}

func (m *mockMetadataVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	if m.statErr != nil {
		return vfs.VFSItem{}, m.statErr
	}
	if m.statToReturn.Name != "" {
		return m.statToReturn, nil
	}
	return m.VFS.Stat(ctx, path)
}

func (m *mockMetadataVFS) SetAttributes(ctx context.Context, path string, item vfs.VFSItem) error {
	if m.setAttrErr != nil {
		return m.setAttrErr
	}
	if m.onSetAttr != nil {
		m.onSetAttr(item)
	}
	if m.onSetAttrPath != nil {
		m.onSetAttrPath(path, item)
	}
	return nil
}
