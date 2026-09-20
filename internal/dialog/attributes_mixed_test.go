package dialog

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
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

// A multiple selection must not be described, nor written back, as its first
// object: the header names the selection, owner and group that differ show as
// "(multiple values)", the time field is blank, and Set writes only what the
// user changed. Owner is edited here and must reach both objects; group and
// time are not and must stay each object's own.
func TestAttributesDialog_UnixMultipleSelectionWritesOnlyEditedFields(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	calls := map[string]vfs.VFSItem{}
	mockVFS := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			calls[path] = item
		},
	}
	firstTime := time.Date(2001, 2, 3, 4, 5, 6, 789, time.Local)
	secondTime := time.Date(2011, 12, 13, 14, 15, 16, 17, time.Local)
	targets := []AttributesTarget{
		{Path: "first.txt", Item: vfs.VFSItem{Name: "first.txt", UnixMode: 0644, Uid: 1001, Gid: 2001, MTime: firstTime}},
		{Path: "second.txt", Item: vfs.VFSItem{Name: "second.txt", UnixMode: 0644, Uid: 1002, Gid: 2002, MTime: secondTime}},
	}

	ShowAttributesUnixForTargets(nil, mockVFS, targets)
	dlg := fm.GetTopFrame().(vtui.Container)
	var texts []string
	var edits []*vtui.Edit
	var checks []*vtui.Checkbox
	var setButton *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		switch c := el.(type) {
		case *vtui.Text:
			texts = append(texts, c.GetText())
		case *vtui.Edit:
			edits = append(edits, c)
		case *vtui.Checkbox:
			checks = append(checks, c)
		case *vtui.Button:
			if strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSet")) {
				setButton = c
			}
		}
		return true
	})
	if len(edits) != 4 || len(checks) != 9 || setButton == nil {
		t.Fatalf("controls: %d edits, %d checkboxes, set button %v; want 4, 9, present", len(edits), len(checks), setButton != nil)
	}
	header := strings.Join(texts, "\n")
	if strings.Contains(header, "first.txt") {
		t.Errorf("header names the first object:\n%s", header)
	}
	for _, want := range []string{i18n.Msg("Attributes.SelectedObjects"), attributesSelectionSummary(targets)} {
		if !strings.Contains(header, want) {
			t.Errorf("header lacks %q:\n%s", want, header)
		}
	}
	// Edits in tree order: owner, group, octal, time.
	editOwner, editGroup, editOctal, editMTime := edits[0], edits[1], edits[2], edits[3]
	if got, want := editOwner.GetText(), i18n.Msg("Attributes.MultipleValues"); got != want {
		t.Errorf("owner field = %q, want %q", got, want)
	}
	if got, want := editGroup.GetText(), i18n.Msg("Attributes.MultipleValues"); got != want {
		t.Errorf("group field = %q, want %q", got, want)
	}
	if got := editMTime.GetText(); got != "" {
		t.Errorf("time field = %q, want blank for a multiple selection", got)
	}
	if got := editOctal.GetText(); got != "0644" {
		t.Errorf("octal field = %q, want 0644 for a mode both objects share", got)
	}

	editOwner.SetText("4242")
	// User: Execute, through the key so the Octal field follows.
	checks[2].ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if got := editOctal.GetText(); got != "0744" {
		t.Fatalf("octal field after checking User: Execute = %q, want 0744", got)
	}
	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	for _, target := range targets {
		got, ok := calls[target.Path]
		if !ok {
			t.Errorf("%s: SetAttributes not called", target.Path)
			continue
		}
		if got.Uid != 4242 {
			t.Errorf("%s: uid = %d, want the edited 4242", target.Path, got.Uid)
		}
		if got.Gid != target.Item.Gid {
			t.Errorf("%s: gid = %d, want its own %d", target.Path, got.Gid, target.Item.Gid)
		}
		if !got.MTime.Equal(target.Item.MTime) {
			t.Errorf("%s: mtime = %v, want its own %v", target.Path, got.MTime, target.Item.MTime)
		}
		if got.UnixMode != 0744 {
			t.Errorf("%s: mode = %04o, want 0744", target.Path, got.UnixMode)
		}
	}
}

// The set-id and sticky digit has no checkboxes. When it differs across the
// selection the Octal field shows it as '-', a checkbox change must carry
// that over rather than reset it to 0, and Set must keep each object's bits.
func TestAttributesDialog_UnixMixedSpecialDigitIsKept(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	calls := map[string]vfs.VFSItem{}
	mockVFS := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			calls[path] = item
		},
	}
	targets := []AttributesTarget{
		{Path: "suid", Item: vfs.VFSItem{Name: "suid", UnixMode: 04755}},
		{Path: "plain", Item: vfs.VFSItem{Name: "plain", UnixMode: 0755}},
	}

	ShowAttributesUnixForTargets(nil, mockVFS, targets)
	dlg := fm.GetTopFrame().(vtui.Container)
	var editOctal *vtui.Edit
	var checks []*vtui.Checkbox
	var setButton *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		switch c := el.(type) {
		case *vtui.Edit:
			if c.Validator != nil {
				editOctal = c
			}
		case *vtui.Checkbox:
			checks = append(checks, c)
		case *vtui.Button:
			if strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSet")) {
				setButton = c
			}
		}
		return true
	})
	if editOctal == nil || len(checks) != 9 || setButton == nil {
		t.Fatal("octal field, checkboxes or Set button not found")
	}
	if got := editOctal.GetText(); got != "-755" {
		t.Fatalf("octal field = %q, want -755", got)
	}
	// User: Execute off. A three-state box goes checked, '?', unchecked.
	space := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE}
	checks[2].ProcessKey(space)
	if got := editOctal.GetText(); got != "--55" {
		t.Fatalf("octal field with User: Execute at '?' = %q, want --55", got)
	}
	checks[2].ProcessKey(space)
	if got := editOctal.GetText(); got != "-655" {
		t.Fatalf("octal field after unchecking User: Execute = %q, want -655", got)
	}
	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	for path, want := range map[string]uint32{"suid": 04655, "plain": 0655} {
		if got := calls[path].UnixMode; got != want {
			t.Errorf("%s: mode = %04o, want %04o", path, got, want)
		}
	}
}

func TestParseOctalModeText(t *testing.T) {
	for _, tc := range []struct {
		text        string
		mode, mixed uint32
		ok          bool
	}{
		{"", 0, 0, true},
		{"755", 0755, 0, true},
		{"4755", 04755, 0, true},
		{"-7-5", 0705, 07070, true},
		{"--", 0, 077, true},
		{"8", 0, 0, false},
		{"07550", 0, 0, false},
		{"?755", 0, 0, false},
	} {
		mode, mixed, ok := parseOctalModeText(tc.text)
		if mode != tc.mode || mixed != tc.mixed || ok != tc.ok {
			t.Errorf("parseOctalModeText(%q) = %04o, %04o, %v; want %04o, %04o, %v", tc.text, mode, mixed, ok, tc.mode, tc.mixed, tc.ok)
		}
	}
}

// Typing '-' into a digit turns its checkboxes to '?', so each object keeps
// those bits, while the digits typed as numbers still apply to all.
func TestAttributesDialog_UnixTypedMixedDigitKeepsBits(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	calls := map[string]vfs.VFSItem{}
	mockVFS := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			calls[path] = item
		},
	}
	targets := []AttributesTarget{
		{Path: "a", Item: vfs.VFSItem{Name: "a", UnixMode: 0640}},
		{Path: "b", Item: vfs.VFSItem{Name: "b", UnixMode: 0604}},
	}

	ShowAttributesUnixForTargets(nil, mockVFS, targets)
	dlg := fm.GetTopFrame().(vtui.Container)
	var editOctal *vtui.Edit
	var checks []*vtui.Checkbox
	var setButton *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		switch c := el.(type) {
		case *vtui.Edit:
			if c.Validator != nil {
				editOctal = c
			}
		case *vtui.Checkbox:
			checks = append(checks, c)
		case *vtui.Button:
			if strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSet")) {
				setButton = c
			}
		}
		return true
	})
	if editOctal == nil || len(checks) != 9 || setButton == nil {
		t.Fatal("octal field, checkboxes or Set button not found")
	}
	if !editOctal.Validator.IsValidInput("07-5") || editOctal.Validator.IsValidInput("07?5") {
		t.Error("the multiple-selection Octal field must accept '-' and reject other non-digits")
	}
	editOctal.SetText("07-0")
	editOctal.OnTextChange("07-0")
	for i := 3; i < 6; i++ {
		if checks[i].State != 2 {
			t.Errorf("group checkbox %d state = %d after typing '-', want 2", i, checks[i].State)
		}
	}
	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	for path, want := range map[string]uint32{"a": 0740, "b": 0700} {
		if got := calls[path].UnixMode; got != want {
			t.Errorf("%s: mode = %04o, want %04o", path, got, want)
		}
	}
}

func TestAttributesDialog_WindowsMultipleSelectionDescribesSelection(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	calls := map[string]vfs.VFSItem{}
	mockVFS := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			calls[path] = item
		},
	}
	firstTime := time.Date(2001, 2, 3, 4, 5, 6, 789, time.Local)
	secondTime := time.Date(2011, 12, 13, 14, 15, 16, 17, time.Local)
	targets := []AttributesTarget{
		{Path: "dir", Item: vfs.VFSItem{Name: "dir", IsDir: true, WinAttrs: 0x10, UnixMode: 0755, MTime: firstTime}},
		{Path: "packed.txt", Item: vfs.VFSItem{Name: "packed.txt", WinAttrs: 0x800, UnixMode: 0600, MTime: secondTime}},
	}

	ShowAttributesWindowsWithPropertiesForTargets(nil, mockVFS, targets, func(string) error { return nil })
	dlg := fm.GetTopFrame().(vtui.Container)
	var texts []string
	var editMTime *vtui.Edit
	var chkHidden *vtui.Checkbox
	var setButton, securityButton *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		switch c := el.(type) {
		case *vtui.Text:
			texts = append(texts, c.GetText())
		case *vtui.Edit:
			editMTime = c
		case *vtui.Checkbox:
			if strings.Contains(c.GetText(), i18n.Msg("Attributes.Hidden")) {
				chkHidden = c
			}
		case *vtui.Button:
			switch {
			case strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSet")):
				setButton = c
			case strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSecurity")):
				securityButton = c
			}
		}
		return true
	})
	if editMTime == nil || chkHidden == nil || setButton == nil || securityButton == nil {
		t.Fatal("Windows multiple-selection dialog controls not found")
	}
	all := strings.Join(texts, "\n")
	if !strings.Contains(all, attributesSelectionSummary(targets)) {
		t.Errorf("dialog lacks the selection summary %q:\n%s", attributesSelectionSummary(targets), all)
	}
	for _, want := range []string{"Compressed (?)", "Directory (?)"} {
		if !strings.Contains(all, want) {
			t.Errorf("advanced flags lack %q:\n%s", want, all)
		}
	}
	if got := editMTime.GetText(); got != "" {
		t.Errorf("last write field = %q, want blank for a multiple selection", got)
	}
	if !securityButton.IsDisabled() {
		t.Error("Security must be disabled: it would open the first object's properties only")
	}

	chkHidden.State = 1
	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	posixSemantics := runtime.GOOS != "windows" || hostmode.Posix()
	for _, target := range targets {
		got, ok := calls[target.Path]
		if !ok {
			t.Errorf("%s: SetAttributes not called", target.Path)
			continue
		}
		if !got.MTime.Equal(target.Item.MTime) {
			t.Errorf("%s: mtime = %v, want its own %v", target.Path, got.MTime, target.Item.MTime)
		}
		if want := target.Item.WinAttrs | 2; got.WinAttrs != want {
			t.Errorf("%s: attributes = %#x, want %#x", target.Path, got.WinAttrs, want)
		}
		wantMode := target.Item.UnixMode
		if !posixSemantics {
			wantMode = 0666
		}
		if got.UnixMode != wantMode {
			t.Errorf("%s: mode = %04o, want %04o", target.Path, got.UnixMode, wantMode)
		}
	}
}

func TestAttributesDialog_MultipleSelectionLayout(t *testing.T) {
	targets := []AttributesTarget{
		{Path: "first.txt", Item: vfs.VFSItem{Name: "first.txt", UnixMode: 0644, Uid: 1, Gid: 1, MTime: time.Now()}},
		{Path: "dir", Item: vfs.VFSItem{Name: "dir", IsDir: true, UnixMode: 0755, Uid: 2, Gid: 2, WinAttrs: 0x10, MTime: time.Now()}},
		{Path: "link", Item: vfs.VFSItem{Name: "link", IsSymlink: true, UnixMode: 0777, Uid: 3, Gid: 3, MTime: time.Now()}},
	}
	for _, show := range []struct {
		name string
		open func(vfs.VFS)
	}{
		{"unix", func(v vfs.VFS) { ShowAttributesUnixForTargets(nil, v, targets) }},
		{"windows", func(v vfs.VFS) {
			ShowAttributesWindowsWithPropertiesForTargets(nil, v, targets, func(string) error { return nil })
		}},
	} {
		t.Run(show.name, func(t *testing.T) {
			scr := vtui.NewSilentScreenBuf()
			scr.AllocBuf(80, 25)
			vtui.FrameManager.Init(scr)
			show.open(vfs.NewOSVFS(t.TempDir()))
			top := vtui.FrameManager.GetTopFrame()
			dlg, ok := top.(vtui.Container)
			if !ok {
				t.Fatal("attributes dialog not found")
			}
			vtui.AssertLayout(t, dlg)
			top.SetExitCode(-1)
			vtui.FrameManager.Pop()
		})
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
