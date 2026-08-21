package gitplugin

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestStatusVFSOrdersStagedSeparatorAndUnstagedRows(t *testing.T) {
	root := t.TempDir()
	plugin := testStatusPlugin(root, map[string]statusEntry{
		"both.txt": {
			Path: "both.txt", Index: gogit.Modified, Worktree: gogit.Modified, Class: statusBoth,
		},
		"added.txt": {
			Path: "added.txt", Index: gogit.Added, Worktree: gogit.Unmodified, Class: statusStaged,
		},
		"work.txt": {
			Path: "work.txt", Index: gogit.Unmodified, Worktree: gogit.Modified, Class: statusUnstaged,
		},
		"new.txt": {
			Path: "new.txt", Index: gogit.Untracked, Worktree: gogit.Untracked, Class: statusUntracked,
		},
	})
	view := newStatusVFS(plugin, Repository{Root: root}, vfs.NewOSVFS(root))
	t.Cleanup(func() { _ = view.Close() })

	var items []vfs.VFSItem
	if err := view.ReadDir(context.Background(), "/", func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if got, want := itemNames(items), []string{
		"0:added.txt", "0:both.txt", statusSeparatorName, "2:both.txt", "2:new.txt", "2:work.txt",
	}; !sameStrings(got, want) {
		t.Fatalf("row names = %q, want %q", got, want)
	}
	if got, want := items[1].PresentationName(), "both.txt"; got != want {
		t.Errorf("staged display name = %q, want %q", got, want)
	}
	if items[2].Kind != vfs.VFSItemSeparator || items[2].PresentationName() == items[2].Name {
		t.Errorf("separator item = %#v, want display-only separator", items[2])
	}
	if got, want := items[3].ExtendedAttributes["git.layer"], "unstaged"; got != want {
		t.Errorf("worktree layer = %q, want %q", got, want)
	}
	if got, want := items[4].ExtendedAttributes["git.indexStatus"], string(gogit.Untracked); got != want {
		t.Errorf("untracked index status = %q, want %q", got, want)
	}
}

func TestStatusVFSUsesUnderlyingFileOnlyForRegularRows(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "changed.txt"), []byte("current worktree bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	plugin := testStatusPlugin(root, map[string]statusEntry{
		"changed.txt": {
			Path: "changed.txt", Index: gogit.Unmodified, Worktree: gogit.Modified, Class: statusUnstaged,
		},
	})
	view := newStatusVFS(plugin, Repository{Root: root}, vfs.NewOSVFS(root))
	t.Cleanup(func() { _ = view.Close() })

	reader, err := view.Open(context.Background(), "/2:changed.txt")
	if err != nil {
		t.Fatalf("Open regular row: %v", err)
	}
	defer reader.Close()
	data := make([]byte, reader.Size())
	if _, err := reader.Read(context.Background(), data); err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if got, want := string(data), "current worktree bytes"; got != want {
		t.Errorf("opened bytes = %q, want %q", got, want)
	}
	if _, err := view.Open(context.Background(), "/"+statusSeparatorName); !os.IsNotExist(err) {
		t.Errorf("Open separator error = %v, want not exist", err)
	}
	if err := view.Remove(context.Background(), "/2:changed.txt"); !os.IsPermission(err) {
		t.Errorf("Remove status row error = %v, want permission", err)
	}
}

func TestStatusVFSFollowsOnlyItsLinkedSourcePanel(t *testing.T) {
	root := t.TempDir()
	source := vfs.NewOSVFS(root)
	plugin := testStatusPlugin(root, nil)
	view := newStatusVFS(plugin, Repository{Root: root}, source)
	t.Cleanup(func() { _ = view.Close() })

	other := vfs.NewOSVFS(t.TempDir())
	if changed := view.follow(nil, vfs.PanelSnapshot{VFS: other}, LookupResult{State: LookupRepository, Repository: Repository{Root: "other"}}); changed {
		t.Fatal("unrelated VFS unexpectedly changed linked status view")
	}
	if changed := view.follow(nil, vfs.PanelSnapshot{VFS: source}, LookupResult{State: LookupRepository, Repository: Repository{Root: "next"}}); !changed {
		t.Fatal("linked source navigation did not update status view")
	}
	if got, ok := view.repositorySnapshot(); !ok || got.Root != "next" {
		t.Fatalf("repository after follow = %#v, %t; want next, true", got, ok)
	}
	if changed := view.follow(nil, vfs.PanelSnapshot{VFS: source}, LookupResult{State: LookupRepository, Repository: Repository{Root: "next", Branch: Branch{Name: "trunk"}}}); !changed {
		t.Fatal("delayed branch update did not refresh the linked status view")
	}
}

func TestStatusVFSArrowKeysDoNotReadSelection(t *testing.T) {
	root := t.TempDir()
	view := newStatusVFS(testStatusPlugin(root, nil), Repository{Root: root}, vfs.NewOSVFS(root))
	t.Cleanup(func() { _ = view.Close() })
	app := &statusSelectionSpyApp{
		logControllerTestApp: logControllerTestApp{selected: []string{statusSeparatorName}},
	}

	for _, key := range []uint16{vtinput.VK_DOWN, vtinput.VK_UP} {
		if handled := view.ProcessPanelKey(app, &vtinput.InputEvent{
			Type:           vtinput.KeyEventType,
			KeyDown:        true,
			VirtualKeyCode: key,
		}); handled {
			t.Errorf("arrow key %d was unexpectedly handled", key)
		}
	}
	if app.selectionReads != 0 {
		t.Fatalf("arrow keys read the O(N) panel selection %d times, want 0", app.selectionReads)
	}

	if handled := view.ProcessPanelKey(app, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	}); !handled {
		t.Fatal("F3 was not handled")
	}
	if app.selectionReads != 1 {
		t.Fatalf("F3 selection reads = %d, want 1", app.selectionReads)
	}
}

type statusSelectionSpyApp struct {
	logControllerTestApp
	selectionReads int
}

func (app *statusSelectionSpyApp) GetSelectedNames() []string {
	app.selectionReads++
	return app.logControllerTestApp.GetSelectedNames()
}

func TestStatusVFSEditDiffRestoresBaseAndRollsBackInvalidRewrite(t *testing.T) {
	root := t.TempDir()
	repository, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "changed.txt")
	if err := os.WriteFile(file, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("changed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("base", &gogit.CommitOptions{Author: &object.Signature{
		Name: "f4 status test", Email: "status@example.invalid", When: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("current\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := readRepositoryStatus(context.Background(), Repository{Root: root})
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	plugin := testStatusPlugin(root, status.entries)
	plugin.statuses[root] = status
	view := newStatusVFS(plugin, Repository{Root: root}, vfs.NewOSVFS(root))
	t.Cleanup(func() { _ = view.Close() })
	app := &logControllerTestApp{selected: []string{"2:changed.txt"}}

	view.editDiff(app, app.selected)
	if got := len(app.editorRequests); got != 1 {
		t.Fatalf("editor requests = %d, want 1", got)
	}
	request := app.editorRequests[0]
	if request.OnSave == nil || !strings.Contains(string(request.Content), "-base\n+current") {
		t.Fatalf("editable patch = %q, want base-to-current unified diff", request.Content)
	}

	invalid := append(append([]byte(nil), request.Content...), []byte("unexpected patch trailer\n")...)
	if err := request.OnSave(context.Background(), invalid); err == nil {
		t.Fatal("invalid rewrite unexpectedly applied")
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != "current\n" {
		t.Fatalf("invalid rewrite left worktree at %q, %v; want current", contents, err)
	}

	edited := []byte(strings.Replace(string(request.Content), "+current", "+edited", 1))
	if err := request.OnSave(context.Background(), edited); err != nil {
		t.Fatalf("apply rewritten patch: %v", err)
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != "edited\n" {
		t.Fatalf("rewritten patch left worktree at %q, %v; want edited", contents, err)
	}
}

func TestEditableUnifiedPatchRejectsGitlinkAndBinaryDiffs(t *testing.T) {
	for name, patch := range map[string]string{
		"binary":  "diff --git a/file b/file\nGIT binary patch\n",
		"gitlink": "diff --git a/module b/module\nindex 1111111..2222222 160000\n--- a/module\n+++ b/module\n@@ -1 +1 @@\n-Subproject commit 111\n+Subproject commit 222\n",
	} {
		if editableUnifiedPatch(patch) {
			t.Errorf("%s patch was unexpectedly editable", name)
		}
	}
}

func TestStatusVFSF7OpensLazyLogInPassivePanel(t *testing.T) {
	root := t.TempDir()
	plugin := testStatusPlugin(root, nil)
	view := newStatusVFS(plugin, Repository{Root: root}, vfs.NewOSVFS(root))
	t.Cleanup(func() { _ = view.Close() })
	app := &logControllerTestApp{}

	view.openLog(app)
	logView, ok := app.openedVFS.(*LogVFS)
	if !ok || logView == nil {
		t.Fatalf("passive VFS = %T, want *LogVFS", app.openedVFS)
	}
	if got := logView.GetPath(); got != "/" {
		t.Errorf("initial log path = %q, want root", got)
	}
}

func TestStatusVFSCommitSigningNeverFallsBackToUnsigned(t *testing.T) {
	root := t.TempDir()
	repository, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := repository.Config()
	if err != nil {
		t.Fatal(err)
	}
	configuration.User.Name = "f4 status test"
	configuration.User.Email = "status@example.invalid"
	configuration.GPG.Format = "x509"
	if err := repository.SetConfig(configuration); err != nil {
		t.Fatal(err)
	}
	plugin := testStatusPlugin(root, nil)
	view := newStatusVFS(plugin, Repository{Root: root}, vfs.NewOSVFS(root))
	t.Cleanup(func() { _ = view.Close() })

	err = view.commit(context.Background(), "must not publish", true, &logControllerTestApp{})
	if !errors.Is(err, gogit.ErrX509SigningUnsupported) {
		t.Fatalf("signing error = %v, want explicit X.509 unsupported error", err)
	}
	if _, err := repository.Head(); !errors.Is(err, plumbing.ErrReferenceNotFound) {
		t.Fatalf("signing failure published a commit: HEAD error = %v, want reference not found", err)
	}
}

func testStatusPlugin(root string, entries map[string]statusEntry) *Plugin {
	return &Plugin{
		initialized: true,
		statuses: map[string]*repositoryStatus{
			root: {
				root:      root,
				entries:   entries,
				updatedAt: time.Now(),
			},
		},
		statusViews: make(map[*StatusVFS]struct{}),
	}
}

func itemNames(items []vfs.VFSItem) []string {
	names := make([]string, len(items))
	for index, item := range items {
		names[index] = item.Name
	}
	return names
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
