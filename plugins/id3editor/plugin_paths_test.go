package id3editor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	v2 "github.com/unxed/id3-go/v2"
)

type id3AppStub struct {
	activeVFS    vfs.VFS
	selected     []string
	messages     []string
	refreshCalls int
}

func (a *id3AppStub) GetActivePanelVFS() vfs.VFS { return a.activeVFS }
func (*id3AppStub) GetPassivePanelVFS() vfs.VFS  { return nil }
func (a *id3AppStub) GetSelectedNames() []string { return a.selected }
func (*id3AppStub) GetSelectedName() string      { return "" }
func (a *id3AppStub) RefreshAll()                { a.refreshCalls++ }
func (*id3AppStub) SetPendingSelection(string)   {}
func (*id3AppStub) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (*id3AppStub) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (a *id3AppStub) Message(_ string, msg string, _ []string) int {
	a.messages = append(a.messages, msg)
	return 0
}
func (*id3AppStub) InputBox(string, string, string, func(string)) {}
func (*id3AppStub) Menu(string, []string, func(int))              {}

func TestPluginCloseClearsLegacyRegistrationState(t *testing.T) {
	host := &id3HostMock{}
	plugin := &ID3EditorPlugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if plugin.api != nil || plugin.registration != nil {
		t.Fatal("Close retained legacy plugin state")
	}
}

func TestShowEditorDialogReportsOpenError(t *testing.T) {
	app := &id3AppStub{}
	plugin := &ID3EditorPlugin{}
	plugin.showEditorDialog(app, filepath.Join(t.TempDir(), "missing.mp3"))
	if len(app.messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(app.messages))
	}
}

func TestSetCommentSupportsID3v2Versions(t *testing.T) {
	for _, version := range []byte{2, 3} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			tag := v2.NewTag(version)
			setComment(tag, "updated comment")
			comments := tag.Comments()
			if len(comments) != 1 || !strings.HasSuffix(comments[0], "updated comment") {
				t.Fatalf("comments = %#v, want one updated comment", comments)
			}
		})
	}
}
