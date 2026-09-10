package panel

import (
	"context"
	"errors"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

type navigationURIProvider struct {
	scheme  string
	mu      sync.Mutex
	current vfs.VFS
	target  string
	result  vfs.VFS
}

func (p *navigationURIProvider) Scheme() string { return p.scheme }
func (p *navigationURIProvider) OpenURI(_ context.Context, current vfs.VFS, target string) (vfs.VFS, error) {
	p.mu.Lock()
	p.current = current
	p.target = target
	p.mu.Unlock()
	return p.result, nil
}

type navigationURIVFS struct {
	*vfs.NullVFS
	uri string
}

type blockingNavigationURIProvider struct {
	scheme  string
	started chan struct{}
	result  vfs.VFS
}

func (p *blockingNavigationURIProvider) Scheme() string { return p.scheme }
func (p *blockingNavigationURIProvider) OpenURI(ctx context.Context, _ vfs.VFS, _ string) (vfs.VFS, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type historyNavigationURIProvider struct {
	scheme string
	bad    string
	good   string
	result vfs.VFS
	mu     sync.Mutex
	calls  []string
}

func (p *historyNavigationURIProvider) Scheme() string { return p.scheme }
func (p *historyNavigationURIProvider) OpenURI(_ context.Context, _ vfs.VFS, target string) (vfs.VFS, error) {
	p.mu.Lock()
	p.calls = append(p.calls, target)
	p.mu.Unlock()
	if target == p.bad {
		return nil, errors.New("profile is unavailable")
	}
	if target == p.good {
		return p.result, nil
	}
	return nil, errors.New("unexpected history target")
}

func (v *navigationURIVFS) GetPath() string { return v.uri }
func (v *navigationURIVFS) ReadDir(ctx context.Context, _ string, onChunk func([]vfs.VFSItem)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if onChunk != nil {
		onChunk([]vfs.VFSItem{{Name: "saved-cursor.txt"}})
	}
	return nil
}
func (v *navigationURIVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	return vfs.VFSItem{Name: "core-uri-navigation-test", IsDir: true}, nil
}

func TestNavigateToPathRestoresRegisteredURIAsynchronously(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldSync := config.App.SyncPanelLoad
	config.App.SyncPanelLoad = false
	t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })

	target := "core-uri-navigation-test://profile/folder-id"
	mounted := &navigationURIVFS{NullVFS: vfs.NewNullVFS(0), uri: target}
	provider := &navigationURIProvider{scheme: "core-uri-navigation-test", result: mounted}
	if err := vfs.RegisterURIProvider(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vfs.UnregisterURIProvider(provider.scheme) })

	source := vfs.NewNullVFS(0)
	fsp := NewFileSystemPanel(0, 0, 40, 20, source)
	waitForLoad(t, fsp)
	t.Cleanup(func() {
		fsp.cancelProviderOpen()
		if fsp.CancelLoad != nil {
			fsp.CancelLoad()
		}
	})
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}
	fsp.PendingSelection = "saved-cursor.txt"

	if !pf.NavigateToPath(fsp, target) {
		t.Fatal("registered URI was rejected")
	}
	if fsp.ProviderOpenTask == nil {
		t.Fatal("URI provider was not opened through the asynchronous lifecycle")
	}
	waitForLoad(t, fsp)

	if fsp.Vfs != mounted || fsp.Vfs.GetPath() != target {
		t.Fatalf("panel VFS = %T %q", fsp.Vfs, fsp.Vfs.GetPath())
	}
	if got := fsp.GetRawSelectedName(); got != "saved-cursor.txt" {
		t.Fatalf("restored cursor = %q", got)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.current != source || provider.target != target {
		t.Fatalf("OpenURI current=%T target=%q", provider.current, provider.target)
	}
}

func TestNavigateToPathDoesNotFeedUnavailableURIToCurrentVFS(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	source := newQueuedNavigationVFS()
	t.Cleanup(func() {
		select {
		case source.releaseFirstRead <- struct{}{}:
		default:
		}
	})
	fsp := NewFileSystemPanel(0, 0, 40, 20, source)
	t.Cleanup(func() {
		if fsp.CancelLoad != nil {
			fsp.CancelLoad()
		}
	})
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}

	if pf.NavigateToPath(fsp, "unavailable-core-uri-test://profile/path") {
		t.Fatal("unregistered URI was accepted")
	}
	if got := source.checkedPathCalls.Load(); got != 0 {
		t.Fatalf("URI reached current VFS SetPath %d times", got)
	}
}

func TestPendingURIMountIsPersistedAndPlainEscapeCancelsIt(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldSync := config.App.SyncPanelLoad
	config.App.SyncPanelLoad = false
	t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })

	target := "core-uri-blocking-test://profile/folder-id"
	provider := &blockingNavigationURIProvider{
		scheme:  "core-uri-blocking-test",
		started: make(chan struct{}),
		result:  &navigationURIVFS{NullVFS: vfs.NewNullVFS(0), uri: target},
	}
	if err := vfs.RegisterURIProvider(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vfs.UnregisterURIProvider(provider.scheme) })

	source := &navigationURIVFS{NullVFS: vfs.NewNullVFS(0), uri: "/source"}
	fsp := NewFileSystemPanel(0, 0, 40, 20, source)
	waitForLoad(t, fsp)
	pf := &PanelsFrame{
		Panels:           [2]Panel{fsp, nil},
		ActiveIdx:        0,
		ShowPanels:       true,
		FolderHistoryPos: [2]int{-1, -1},
	}
	if !pf.NavigateToPath(fsp, target) {
		t.Fatal("registered URI was rejected")
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for URI provider")
	}
	left, _ := pf.GetPaths()
	if left != target {
		t.Fatalf("persisted pending path = %q, want %q", left, target)
	}
	escape := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	if !pf.VetoActionKey(escape) {
		t.Fatal("plain Escape was not reserved for the pending mount")
	}
	if !fsp.ProcessKey(escape) {
		t.Fatal("plain Escape did not cancel the pending mount")
	}
	waitForLoad(t, fsp)
	if fsp.ProviderOpenTask != nil || fsp.Vfs != source || fsp.Vfs.GetPath() != "/source" {
		t.Fatalf("panel was not restored after cancel: task=%v vfs=%T path=%q", fsp.ProviderOpenTask != nil, fsp.Vfs, fsp.Vfs.GetPath())
	}
	if got := fsp.GetRawSelectedName(); got != "saved-cursor.txt" {
		t.Fatalf("selection after cancel = %q", got)
	}
}

func TestFolderHistoryBackUsesPendingVisualTargetInsteadOfSourceVFS(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldSync := config.App.SyncPanelLoad
	config.App.SyncPanelLoad = false
	t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source")
	olderPath := filepath.Join(root, "older")
	for _, path := range []string{sourcePath, olderPath} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	target := "core-uri-pending-history-test://profile/folder-id"
	provider := &blockingNavigationURIProvider{
		scheme:  "core-uri-pending-history-test",
		started: make(chan struct{}),
	}
	if err := vfs.RegisterURIProvider(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vfs.UnregisterURIProvider(provider.scheme) })

	folders := history.NewProviderAtPath(filepath.Join(root, "history.json"))
	folders.SaveHistory("folders", []string{target, sourcePath, olderPath})
	oldHistory := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = folders
	t.Cleanup(func() { vtui.GlobalHistoryProvider = oldHistory })

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(sourcePath))
	waitForLoad(t, fsp)
	pf := &PanelsFrame{
		Panels:           [2]Panel{fsp, nil},
		ActiveIdx:        0,
		ShowPanels:       true,
		FolderHistoryPos: [2]int{-1, -1},
	}
	t.Cleanup(func() {
		fsp.cancelProviderOpen()
		if fsp.CancelLoad != nil {
			fsp.CancelLoad()
		}
	})

	if !pf.NavigateToPath(fsp, target) {
		t.Fatal("pending history target was rejected")
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pending history mount")
	}
	if got := fsp.PersistentPath(); got != target {
		t.Fatalf("persistent pending path = %q, want %q", got, target)
	}
	altLeft := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_LEFT,
		ControlKeyState: vtinput.LeftAltPressed,
	}
	if pf.VetoActionKey(altLeft) {
		t.Fatal("pending provider mount vetoed Alt+Left history action")
	}

	if !pf.MoveFolderHistory(fsp, -1) {
		t.Fatal("Alt+Left history move was not performed")
	}
	waitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); !fileops.SameFolderHistoryPath(got, sourcePath) {
		t.Fatalf("Alt+Left skipped source folder: got %q, want %q", got, sourcePath)
	}
	if got := pf.FolderHistoryPos[0]; got != 1 {
		t.Fatalf("history position = %d, want source index 1", got)
	}
	if got := folders.LoadHistory("folders"); !reflect.DeepEqual(got, []string{target, sourcePath, olderPath}) {
		t.Fatalf("Alt+Left reordered history: %#v", got)
	}
}

func TestFolderHistoryCommitsPositionOnlyAfterSuccessfulURIMount(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldSync := config.App.SyncPanelLoad
	config.App.SyncPanelLoad = false
	t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })

	bad := "core-uri-history-test://missing/folder"
	good := "core-uri-history-test://available/folder"
	mounted := &navigationURIVFS{NullVFS: vfs.NewNullVFS(0), uri: good}
	provider := &historyNavigationURIProvider{scheme: "core-uri-history-test", bad: bad, good: good, result: mounted}
	if err := vfs.RegisterURIProvider(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vfs.UnregisterURIProvider(provider.scheme) })

	source := &navigationURIVFS{NullVFS: vfs.NewNullVFS(0), uri: "/source"}
	fsp := NewFileSystemPanel(0, 0, 40, 20, source)
	waitForLoad(t, fsp)
	pf := &PanelsFrame{
		Panels:           [2]Panel{fsp, nil},
		ActiveIdx:        0,
		FolderHistoryPos: [2]int{-1, -1},
	}
	if !pf.NavigateAvailableFolderHistory(fsp, []string{bad, good}, 0, 1) {
		t.Fatal("history navigation was not started")
	}
	if got := pf.FolderHistoryPos[0]; got != -1 {
		t.Fatalf("history position committed before mount completion: %d", got)
	}
	waitForLoad(t, fsp)
	if fsp.Vfs != mounted || pf.FolderHistoryPos[0] != 1 {
		t.Fatalf("history fallback result: vfs=%T pos=%d", fsp.Vfs, pf.FolderHistoryPos[0])
	}
	provider.mu.Lock()
	calls := append([]string(nil), provider.calls...)
	provider.mu.Unlock()
	if len(calls) != 2 || calls[0] != bad || calls[1] != good {
		t.Fatalf("history provider calls = %#v", calls)
	}
}
