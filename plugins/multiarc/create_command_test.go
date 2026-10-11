package multiarc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type testRegistration struct{ unregistered int }

func (r *testRegistration) Unregister() { r.unregistered++ }

type testHotkey struct {
	vk      uint16
	mods    vtinput.ControlKeyState
	handler func(vfs.App)
}

// testHost is a vfs.HostAPI that records what a plugin registers.
type testHost struct {
	providers []vfs.VFSProvider
	hotkeys   []testHotkey
}

func (*testHost) GetVersion() string                           { return "test" }
func (*testHost) Log(string)                                   {}
func (*testHost) Message(string)                               {}
func (*testHost) RegisterHighlighter(vtui.HighlighterProvider) {}
func (h *testHost) RegisterVFSProvider(p vfs.VFSProvider)      { h.providers = append(h.providers, p) }
func (*testHost) RegisterURIProvider(vfs.URIProvider) error    { return nil }
func (*testHost) RegisterDrive(string, func() vfs.VFS)         {}
func (h *testHost) RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(vfs.App)) {
	h.hotkeys = append(h.hotkeys, testHotkey{vk: vk, mods: mods, handler: handler})
}
func (*testHost) RegisterPluginMenuItem(string, func(vfs.App)) {}
func (*testHost) RunAction(string) bool                        { return false }

// testContributionHost adds vfs.ContributionHost's command registration.
type testContributionHost struct {
	*testHost
	commands      []vfs.PluginCommand
	registrations []*testRegistration
	fail          error
}

func (h *testContributionHost) RegisterPluginCommand(c vfs.PluginCommand) (vfs.Registration, error) {
	if h.fail != nil {
		return nil, h.fail
	}
	r := &testRegistration{}
	h.commands = append(h.commands, c)
	h.registrations = append(h.registrations, r)
	return r, nil
}

func (*testContributionHost) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	panic("unexpected Quick View registration")
}

func (*testContributionHost) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	panic("unexpected command-prefix registration")
}

func (*testContributionHost) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	panic("unexpected macro registration")
}

// The lite build offers Add to archive exactly where the regular build
// does: same ID, label, localization keys, Files menu and Shift+F1.
func TestPluginRegistersAddToArchive(t *testing.T) {
	host := &testContributionHost{testHost: &testHost{}}
	p := &Plugin{}
	if err := p.Init(host); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(host.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(host.commands))
	}
	c := host.commands[0]
	if c.ID != "archive.add" || c.Location != vfs.PluginCommandPanel || c.Label != "Add to archive" ||
		c.LabelKey != "Archive.Command.Add" || c.DescriptionKey != "Archive.Command.Add.Desc" || c.Description == "" ||
		c.MenuPath != "Files" || c.Shortcut != "Shift+F1" || c.Run == nil ||
		!reflect.DeepEqual(c.SearchKeys, []string{"Attributes.Archive"}) {
		t.Errorf("command = %#v", c)
	}
	if len(host.providers) != 1 {
		t.Errorf("providers = %d, want the multiarc provider", len(host.providers))
	}
	if len(host.hotkeys) != 1 || host.hotkeys[0].vk != vtinput.VK_F1 || host.hotkeys[0].mods != vtinput.ShiftPressed || host.hotkeys[0].handler == nil {
		t.Errorf("hotkeys = %#v, want Shift+F1", host.hotkeys)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if host.registrations[0].unregistered != 1 {
		t.Errorf("command unregistered %d times, want once", host.registrations[0].unregistered)
	}
}

func TestPluginRegistrationFailureRegistersNothing(t *testing.T) {
	host := &testContributionHost{testHost: &testHost{}, fail: errors.New("injected")}
	if err := (&Plugin{}).Init(host); err == nil {
		t.Fatal("Init should fail when the command cannot be registered")
	}
	if len(host.providers) != 0 || len(host.hotkeys) != 0 {
		t.Fatalf("a failed Init left providers=%d hotkeys=%d", len(host.providers), len(host.hotkeys))
	}
}

// A host without rich contributions still gets the provider and Shift+F1.
func TestPluginWithoutContributionHost(t *testing.T) {
	host := &testHost{}
	p := &Plugin{}
	if err := p.Init(host); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	if len(host.providers) != 1 || len(host.hotkeys) != 1 {
		t.Fatalf("providers=%d hotkeys=%d, want 1 and 1", len(host.providers), len(host.hotkeys))
	}
}

// addTestApp is the panel side of Add to archive. The flow hops across
// goroutines the way it must in f4, so every step it reaches is reported
// on events, and a test waits for the one that ends it.
type addTestApp struct {
	active   vfs.VFS
	selected []string
	answer   string // what the user types into the name prompt
	reply    int    // the button every message box answers with

	mu         sync.Mutex
	suggestion string
	messages   []string
	pending    string
	refreshes  int
	events     chan string
	backlog    []string // events waitFor read past; test goroutine only
}

func newAddTestApp(active vfs.VFS, selected []string, answer string) *addTestApp {
	return &addTestApp{active: active, selected: selected, answer: answer, events: make(chan string, 16)}
}

func (a *addTestApp) GetActivePanelVFS() vfs.VFS  { return a.active }
func (a *addTestApp) GetPassivePanelVFS() vfs.VFS { return nil }
func (a *addTestApp) GetSelectedNames() []string  { return a.selected }
func (a *addTestApp) GetSelectedName() string     { return "" }
func (a *addTestApp) RefreshAll() {
	a.mu.Lock()
	a.refreshes++
	a.mu.Unlock()
}
func (a *addTestApp) SetPendingSelection(name string) {
	a.mu.Lock()
	a.pending = name
	a.mu.Unlock()
}
func (a *addTestApp) RunProgressTask(_, _ string, _ bool, worker func(context.Context, func(string, int)) error, onComplete func(error)) {
	err := worker(context.Background(), func(string, int) {})
	onComplete(err)
	if err != nil {
		a.events <- "complete: " + err.Error()
		return
	}
	a.events <- "complete"
}
func (a *addTestApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
	panic("Add to archive uses RunProgressTask")
}
func (a *addTestApp) Message(title, msg string, _ []string) int {
	a.mu.Lock()
	a.messages = append(a.messages, title+"|"+msg)
	a.mu.Unlock()
	a.events <- "message: " + msg
	return a.reply
}
func (a *addTestApp) InputBox(_, _, defaultText string, callback func(string)) {
	a.mu.Lock()
	a.suggestion = defaultText
	a.mu.Unlock()
	callback(a.answer)
	if a.answer == "" {
		a.events <- "prompt dismissed"
	}
}
func (a *addTestApp) Menu(string, []string, func(int)) { panic("Add to archive asks no menu") }

// waitFor returns the first event starting with prefix. Events that arrive
// meanwhile are kept for later calls: the error message onComplete posts
// on a goroutine of its own may overtake the "complete" event, or not.
func (a *addTestApp) waitFor(t *testing.T, prefix string) string {
	t.Helper()
	for i, e := range a.backlog {
		if strings.HasPrefix(e, prefix) {
			a.backlog = append(a.backlog[:i], a.backlog[i+1:]...)
			return e
		}
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case e := <-a.events:
			if strings.HasPrefix(e, prefix) {
				return e
			}
			a.backlog = append(a.backlog, e)
		case <-deadline:
			t.Fatalf("timed out waiting for %q; saw %q", prefix, a.backlog)
			return ""
		}
	}
}

func addFlowFixture(t *testing.T) (string, *vfs.OSVFS) {
	t.Helper()
	root := t.TempDir()
	panelDir := filepath.Join(root, "project")
	writeTree(t, panelDir, map[string]string{"a.txt": "a", "dir/b.txt": "b"})
	return panelDir, vfs.NewOSVFS(panelDir)
}

func TestAddToArchiveCreatesTheNamedArchive(t *testing.T) {
	f := &fakeArchiver{tools: toolSet("tar", "gzip"), tarVersion: gnuTarVersion, writeArchives: true}
	f.install(t)
	panelDir, osvfs := addFlowFixture(t)
	app := newAddTestApp(osvfs, []string{"..", "a.txt", "dir"}, "backup.tar")

	actionAddArchive(app)
	app.waitFor(t, "complete")

	if app.suggestion != "project.tar.gz" {
		t.Errorf("suggested name = %q, want the folder's name with the best format the tools make", app.suggestion)
	}
	want := []string{"tar -c --force-local -f <work>" + string(filepath.Separator) + "backup.tar -- a.txt dir"}
	if got := placeholders(f.commands(), panelDir); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	if got := readArchive(t, filepath.Join(panelDir, "backup.tar")); got != "|c:a.txt,dir" {
		t.Errorf("archive = %q", got)
	}
	if app.pending != "backup.tar" || app.refreshes != 1 {
		t.Errorf("pending=%q refreshes=%d, want the cursor on backup.tar after one refresh", app.pending, app.refreshes)
	}
}

func TestAddToArchiveSuggestsTheSingleItemsName(t *testing.T) {
	f := &fakeArchiver{tools: toolSet("zip")}
	f.install(t)
	_, osvfs := addFlowFixture(t)
	app := newAddTestApp(osvfs, []string{"a.txt"}, "")
	actionAddArchive(app)
	app.waitFor(t, "prompt dismissed")
	if app.suggestion != "a.txt.zip" {
		t.Fatalf("suggested name = %q, want a.txt.zip", app.suggestion)
	}
	if len(f.commands()) != 0 {
		t.Fatalf("a dismissed prompt still ran %q", f.commands())
	}
}

func TestAddToArchiveAsksBeforeOverwriting(t *testing.T) {
	f := &fakeArchiver{tools: toolSet("zip"), writeArchives: true}
	f.install(t)
	panelDir, osvfs := addFlowFixture(t)
	existing := filepath.Join(panelDir, "old.zip")
	if err := os.WriteFile(existing, []byte("OLD"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := newAddTestApp(osvfs, []string{"a.txt"}, "old.zip")
	app.reply = 1 // No
	actionAddArchive(app)
	app.waitFor(t, "message: The target archive already exists")
	if got := readArchive(t, existing); got != "OLD" || len(f.commands()) != 0 {
		t.Fatalf("answering No still archived: archive=%q commands=%q", got, f.commands())
	}

	app = newAddTestApp(osvfs, []string{"a.txt"}, "old.zip")
	app.reply = 0 // Yes
	actionAddArchive(app)
	app.waitFor(t, "message: The target archive already exists")
	app.waitFor(t, "complete")
	if got := readArchive(t, existing); got != "|zip:a.txt" {
		t.Fatalf("archive = %q, want it rebuilt from scratch", got)
	}
}

func TestAddToArchiveReportsAnUnknownFormatUpFront(t *testing.T) {
	f := &fakeArchiver{tools: toolSet("zip")}
	f.install(t)
	_, osvfs := addFlowFixture(t)
	app := newAddTestApp(osvfs, []string{"a.txt"}, "a.rar")
	actionAddArchive(app)
	e := app.waitFor(t, "message: ")
	if !strings.Contains(e, "end the name with one of .zip") {
		t.Fatalf("message = %q, want the formats that can be made", e)
	}
}

func TestAddToArchiveReportsAFailedTool(t *testing.T) {
	f := &fakeArchiver{tools: toolSet("zip"), fail: map[string]error{"zip -q": errors.New("exit status 18")}}
	f.install(t)
	_, osvfs := addFlowFixture(t)
	app := newAddTestApp(osvfs, []string{"a.txt"}, "x.zip")
	actionAddArchive(app)
	app.waitFor(t, "complete: ")
	e := app.waitFor(t, "message: Archiving failed")
	if !strings.Contains(e, "zip says no") {
		t.Fatalf("message = %q, want what zip said", e)
	}
}

func TestAddToArchiveNeedsTheLocalDisk(t *testing.T) {
	app := newAddTestApp(newTestVFS(t, fakeBackend{}), []string{"a.txt"}, "x.zip")
	actionAddArchive(app)
	e := app.waitFor(t, "message: ")
	if !strings.Contains(e, "local disk") {
		t.Fatalf("message = %q", e)
	}
}

func TestAddToArchiveIgnoresTheParentRow(t *testing.T) {
	_, osvfs := addFlowFixture(t)
	app := newAddTestApp(osvfs, []string{".."}, "x.zip")
	actionAddArchive(app)
	select {
	case e := <-app.events:
		t.Fatalf("\"..\" alone started the flow: %q", e)
	case <-time.After(50 * time.Millisecond):
	}
}
