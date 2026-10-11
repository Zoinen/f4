package panel

import (
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestClipboardImageNumbering(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"screenshot001.png", "screenshot9.png", "screenshot099.png", "screenshot900.jpg", "screenshot8x.png", "screenshot100.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "screenshot999.png"), 0700); err != nil {
		t.Fatal(err)
	}
	naming := clipboardImageName{before: "screenshot", after: ".png", width: 3}
	sequence, err := naming.next(context.Background(), vfs.NewOSVFS(dir), dir)
	if err != nil || naming.filename(sequence) != "screenshot1000.png" {
		t.Fatalf("sequence = %v, %v", sequence, err)
	}
	if got := naming.filename(new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)); !strings.Contains(got, "1000000000000000000000000000000") {
		t.Fatal(got)
	}
}

type clipboardCaseVFS struct {
	vfs.VFS
	insensitive bool
}

func (p *clipboardCaseVFS) ReadDir(ctx context.Context, dir string, chunk func([]vfs.VFSItem)) error {
	chunk([]vfs.VFSItem{{Name: "SCREENSHOT007.PNG", IsHidden: true}, {Name: "screenshot003.png"}})
	return nil
}

func (p *clipboardCaseVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	name := p.Base(path)
	for _, existing := range []string{"SCREENSHOT007.PNG", "screenshot003.png"} {
		if name == existing || p.insensitive && strings.EqualFold(name, existing) {
			return vfs.VFSItem{Name: existing}, nil
		}
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func TestClipboardImageNumberingUsesDestinationCaseSemantics(t *testing.T) {
	for _, insensitive := range []bool{false, true} {
		fs := &clipboardCaseVFS{VFS: vfs.NewOSVFS(t.TempDir()), insensitive: insensitive}
		naming := clipboardImageName{before: "screenshot", after: ".png", width: 3, foldCase: !insensitive}
		sequence, err := naming.next(context.Background(), fs, fs.GetPath())
		want := "screenshot004.png"
		if insensitive {
			want = "screenshot008.png"
		}
		if err != nil || naming.filename(sequence) != want {
			t.Fatalf("insensitive=%v: %v %v", insensitive, sequence, err)
		}
	}
}

type clipboardSaveProbe struct {
	vfs.VFS
	collision            bool
	collisionHigh        bool
	failWrite, failClose bool
	cancel               context.CancelFunc
	readonly             bool
}

func (p *clipboardSaveProbe) GetCapabilities() vfs.VFSCapabilities {
	caps := p.VFS.GetCapabilities()
	if p.readonly {
		caps.HasWrite = false
	}
	return caps
}
func (p *clipboardSaveProbe) Rename(ctx context.Context, old, new string) error {
	if p.collision {
		p.collision = false
		if p.collisionHigh {
			if err := os.WriteFile(p.Join(p.GetPath(), "screenshot099.png"), []byte("reserved"), 0600); err != nil {
				return err
			}
		}
		if err := os.WriteFile(new, []byte("existing"), 0600); err != nil {
			return err
		}
	}
	return p.VFS.Rename(ctx, old, new)
}
func (p *clipboardSaveProbe) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	w, err := p.VFS.Create(ctx, path)
	if err != nil {
		return nil, err
	}
	return &clipboardFaultWriter{WriteCloser: w, probe: p}, nil
}

type clipboardFaultWriter struct {
	io.WriteCloser
	probe *clipboardSaveProbe
}

func (w *clipboardFaultWriter) Write(data []byte) (int, error) {
	if w.probe.failWrite {
		return 0, io.ErrShortWrite
	}
	n, err := w.WriteCloser.Write(data)
	if w.probe.cancel != nil {
		w.probe.cancel()
	}
	return n, err
}
func (w *clipboardFaultWriter) Close() error {
	err := w.WriteCloser.Close()
	if w.probe.failClose {
		return errors.Join(err, io.ErrClosedPipe)
	}
	return err
}

func TestClipboardImageSafePublication(t *testing.T) {
	for _, scenario := range []string{"success", "collision", "collision-high", "write", "close", "cancel", "readonly"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			probe := &clipboardSaveProbe{VFS: vfs.NewOSVFS(dir), collision: strings.HasPrefix(scenario, "collision"), collisionHigh: scenario == "collision-high", failWrite: scenario == "write", failClose: scenario == "close", readonly: scenario == "readonly"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancel" {
				probe.cancel = cancel
			}
			name, err := saveClipboardImage(ctx, probe, dir, image.NewNRGBA(image.Rect(0, 0, 2, 2)), config.DefaultConfig(), clipboardImageName{before: "screenshot", after: ".png", width: 3})
			success := scenario == "success" || strings.HasPrefix(scenario, "collision")
			if success && err != nil {
				t.Fatal(err)
			}
			if scenario == "readonly" && (err == nil || !strings.Contains(err.Error(), i18n.Msg("ClipboardImage.ReadOnly"))) {
				t.Fatal("read-only destination was not explained")
			}
			if !success && (err == nil || name != "") {
				t.Fatalf("name = %q, err = %v", name, err)
			}
			if success {
				want := "screenshot001.png"
				if scenario == "collision" {
					want = "screenshot002.png"
				}
				if scenario == "collision-high" {
					want = "screenshot100.png"
				}
				if name != want {
					t.Fatalf("name = %q", name)
				}
				f, err := os.Open(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				_, err = png.Decode(f)
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".f4-") {
					t.Fatalf("staging file leaked: %s", entry.Name())
				}
			}
			if scenario == "collision" {
				data, err := os.ReadFile(filepath.Join(dir, "screenshot001.png"))
				if err != nil || string(data) != "existing" {
					t.Fatal("existing image overwritten")
				}
			}
		})
	}
}

func TestClipboardImageRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"../1.png", "a/b.png", "a\\b.png", "a:1.png", "CON.png", "x\n1.png", "", strings.Repeat("x", 256)} {
		if err := ValidateClipboardImageName(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestClipboardPasteChoiceAndTextFallback(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	old := readPanelClipboard
	defer func() { terminal.WaitForAsyncClipboard(); readPanelClipboard = old }()
	readPanelClipboard = func(context.Context) (terminal.ClipboardContents, error) {
		return terminal.ClipboardContents{Text: "paste me"}, nil
	}
	pf.CmdLine.Edit.SetText("keep ")
	pf.CmdLine.Edit.HistoryPos = 0
	if !ActionPasteClipboard(pf) {
		t.Fatal("paste action not handled")
	}
	terminal.WaitForAsyncClipboard()
	testutil.DrainUITasks()
	if got := pf.CmdLine.Edit.GetText(); got != "keep paste me" {
		t.Fatal(got)
	}
	if pf.CmdLine.Edit.HistoryPos != -1 {
		t.Fatal("paste did not exit history browsing")
	}
	for _, choice := range []string{"ClipboardImage.SaveImage", "ClipboardImage.PasteText", "vtui.Cancel"} {
		saved, pasted := false, false
		showClipboardPasteChoice(pf, "first\n"+strings.Repeat("long ", 100), func() { saved = true }, func() { pasted = true })
		dlg := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		preview := false
		for _, child := range dlg.GetChildren() {
			if list, ok := child.(*vtui.ListBox); ok {
				preview = len(list.Items) > 2
			}
			if button, ok := child.(*vtui.Button); ok && button.GetCaption() == strings.ReplaceAll(i18n.Msg(choice), "&", "") {
				button.OnClick()
			}
		}
		if !preview || saved != (choice == "ClipboardImage.SaveImage") || pasted != (choice == "ClipboardImage.PasteText") {
			t.Fatalf("choice %s: preview=%v save=%v text=%v", choice, preview, saved, pasted)
		}
		vtui.FrameManager.RemoveFrame(dlg)
	}
}

func TestClipboardPasteCompletionKeepsDestinationAndMarks(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pnl := pf.GetActivePanel()
	dir := t.TempDir()
	pnl.Vfs = vfs.NewOSVFS(dir)
	for _, name := range []string{"keep.txt", "screenshot001.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	pnl.ReadDirectory()
	waitForDirectoryLoads(t)
	testutil.DrainUITasks()
	pnl.ReplaceMarkedNames([]string{"keep.txt"})
	capture := captureApplyCommandPanel(pnl, nil)
	finishClipboardImagePaste(pf, capture, "screenshot001.png")
	waitForDirectoryLoads(t)
	testutil.DrainUITasks()
	if pnl.GetRawSelectedName() != "screenshot001.png" {
		t.Fatal("pasted image not focused")
	}
	if got := pnl.GetMarkedNames(); len(got) != 1 || got[0] != "keep.txt" {
		t.Fatalf("marks changed: %v", got)
	}
	other := t.TempDir()
	_ = pnl.Vfs.SetPath(other)
	finishClipboardImagePaste(pf, capture, "screenshot002.png")
	if pnl.PendingSelection == "screenshot002.png" {
		t.Fatal("selection applied to new directory")
	}
}

func TestClipboardImagePasteCapturesDirectoryAndSettingsBeforeReading(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	originalDir, nextDir := t.TempDir(), t.TempDir()
	pnl := pf.GetActivePanel()
	pnl.Vfs = vfs.NewOSVFS(originalDir)
	before, reader := config.App, readPanelClipboard
	defer func() { terminal.WaitForAsyncClipboard(); config.App = before; readPanelClipboard = reader }()
	config.App = config.DefaultConfig()
	reading, resume := make(chan struct{}), make(chan struct{})
	readPanelClipboard = func(context.Context) (terminal.ClipboardContents, error) {
		close(reading)
		<-resume
		return terminal.ClipboardContents{Image: image.NewNRGBA(image.Rect(0, 0, 2, 2))}, nil
	}
	if !ActionPasteClipboard(pf) {
		t.Fatal("paste not handled")
	}
	<-reading
	if err := pnl.Vfs.SetPath(nextDir); err != nil {
		t.Fatal(err)
	}
	config.App.ClipboardImagePrefix = "changed"
	close(resume)
	terminal.WaitForAsyncClipboard()
	deadline := time.After(5 * time.Second)
	for vtui.FrameManager.GetActiveToast() == "" {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("paste did not finish")
		}
	}
	if _, err := os.Stat(filepath.Join(originalDir, "screenshot001.png")); err != nil {
		t.Fatal("captured destination/settings not used:", err)
	}
	entries, err := os.ReadDir(nextDir)
	if err != nil || len(entries) != 0 {
		t.Fatal("paste wrote into newly navigated directory")
	}
	if pnl.PendingSelection != "" {
		t.Fatal("paste focused a file in the new directory")
	}
}

type clipboardHiddenVFS struct{ vfs.VFS }

func (p *clipboardHiddenVFS) ReadDir(ctx context.Context, dir string, chunk func([]vfs.VFSItem)) error {
	return p.VFS.ReadDir(ctx, dir, func(items []vfs.VFSItem) {
		for i := range items {
			if items[i].Name == "screenshot001.png" {
				items[i].IsHidden = true
			}
		}
		chunk(items)
	})
}

func TestClipboardPasteRevealsHiddenImageAndClearsFilter(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	before := config.App
	defer func() { config.App = before }()
	config.App.ShowHiddenFiles = false
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pnl := pf.GetActivePanel()
	dir := t.TempDir()
	pnl.Vfs = &clipboardHiddenVFS{VFS: vfs.NewOSVFS(dir)}
	for _, name := range []string{"keep.txt", "screenshot001.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	pnl.ReadDirectory()
	waitForDirectoryLoads(t)
	testutil.DrainUITasks()
	pnl.ReplaceMarkedNames([]string{"keep.txt"})
	pnl.FastFindStr = "*keep"
	pnl.ToggleAutoFilter()
	capture := captureApplyCommandPanel(pnl, nil)
	finishClipboardImagePaste(pf, capture, "screenshot001.png")
	waitForDirectoryLoads(t)
	testutil.DrainUITasks()
	if pnl.AutoFilterActive() || pnl.GetRawSelectedName() != "screenshot001.png" {
		t.Fatal("pasted image was hidden by display filters")
	}
	if got := pnl.GetMarkedNames(); len(got) != 1 || got[0] != "keep.txt" {
		t.Fatalf("marks changed: %v", got)
	}
	if config.App.ShowHiddenFiles {
		t.Fatal("global hidden-file preference changed")
	}
	if got := pnl.clipboardImageRevealFor(pnl.Vfs, t.TempDir()); got != "" {
		t.Fatal("reveal escaped destination directory")
	}
}

func TestClipboardChoiceDefaultsToImageAndCancelReleasesSnapshot(t *testing.T) {
	defer swapFrameManager(t)()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	saved, canceled := false, false
	showClipboardPasteChoice(pf, "preview", func() { saved = true }, func() { t.Fatal("default pasted text") }, func() { canceled = true })
	dlg := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if !saved || canceled {
		t.Fatal("Enter did not save image")
	}
	vtui.FrameManager.RemoveFrame(dlg)
	showApplyCommandPrompts(pf, []cmdline.ApplyCommandResolvedPrompt{{Title: "Name", Initial: "photo"}}, func(cmdline.ApplyCommandPromptValues) { t.Fatal("canceled prompt accepted") }, func() { canceled = true })
	prompt := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	prompt.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if !canceled {
		t.Fatal("canceled prompt did not release snapshot")
	}
	vtui.FrameManager.RemoveFrame(prompt)
}
