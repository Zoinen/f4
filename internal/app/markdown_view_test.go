package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestIsMarkdownFile(t *testing.T) {
	for name, want := range map[string]bool{
		"README.md":      true,
		"notes.MD":       true,
		"doc.markdown":   true,
		"a.mdown":        true,
		"b.mkd":          true,
		"main.go":        false,
		"md":             false,
		"archive.md.zip": false,
	} {
		if got := isMarkdownFile(name); got != want {
			t.Errorf("isMarkdownFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestReadMarkdownSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)
	got, err := readMarkdownSource(context.Background(), v, path)
	if err != nil || string(got) != "# Title\n" {
		t.Fatalf("readMarkdownSource = %q, %v", got, err)
	}

	big := filepath.Join(dir, "big.md")
	if err := os.WriteFile(big, make([]byte, markdownViewMaxSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMarkdownSource(context.Background(), v, big); !errors.Is(err, errMarkdownTooLarge) {
		t.Fatalf("large file: err = %v, want errMarkdownTooLarge", err)
	}
}

func TestMarkdownViewRendersFormattedAndF4GoesToSource(t *testing.T) {
	vtui.SetDefaultPalette()
	screen := vtui.NewSilentScreenBuf()
	vtui.FrameManager.Init(screen)
	screen.AllocBuf(80, 20)

	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	source := "# f4 notes\r\n\r\nThis is **important** text.\r\n\r\nSee [the docs](https://example.com/docs).\r\n"
	mv := newMarkdownView(vfs.NewOSVFS(dir), path, []byte(source))
	sourceOpened := 0
	mv.onSource = func() { sourceOpened++ }
	mv.ResizeConsole(80, 20)
	mv.Show(screen)

	var dump bytes.Buffer
	screen.Dump(&dump)
	text := dump.String()
	t.Logf("formatted view:\n%s", text[:strings.Index(text, "--- CELL METADATA")])
	for _, want := range []string{" README.md ", "f4 notes", "This is important text.", "See the docs."} {
		if !strings.Contains(text, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	for _, markup := range []string{"# f4", "**", "](", "\r"} {
		if strings.Contains(text, markup) {
			t.Errorf("screen still shows Markdown markup %q", markup)
		}
	}
	if x1, y1, x2, y2 := mv.GetPosition(); x1 != 0 || x2 != 79 || y2 != 18 || y1 != vtui.FrameManager.WorkspaceTopInset() {
		t.Errorf("position = %d,%d-%d,%d, want the whole workspace above the key bar", x1, y1, x2, y2)
	}
	if mv.IsModal() {
		t.Error("the Markdown view is modal; a file viewer must not be")
	}
	labels := mv.GetKeyLabels()
	if labels.Normal[3] == "" || labels.Normal[2] == "" || labels.Normal[9] == "" {
		t.Errorf("key bar = %q, want F3, F4 and F10 labelled", labels.Normal)
	}

	mv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4})
	if sourceOpened != 1 || !mv.IsDone() {
		t.Fatalf("F4: source opened %d time(s), done = %v; want the text viewer once and this view closed", sourceOpened, mv.IsDone())
	}
}

func TestMarkdownViewF3Closes(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	mv := newMarkdownView(nil, "/tmp/x.md", []byte("text"))
	mv.onSource = func() { t.Fatal("F3 opened the source") }
	mv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3})
	if !mv.IsDone() {
		t.Fatal("F3 did not close the Markdown view")
	}
}

// TestActionSwitchViewerToMarkdownAndBack is the reverse leg of
// TestMarkdownViewRendersFormattedAndF4GoesToSource: from the plain text
// viewer on a Markdown file, Shift+F3 (Viewer.MarkdownFormatted) goes back to
// the formatted view, closing the text viewer behind it (f4#1625 step 2).
func TestActionSwitchViewerToMarkdownAndBack(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("# Title\n\nBody text.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	// Open the plain text/hex viewer directly, bypassing the "open as
	// formatted" path -- the situation Shift+F3 exists for.
	openPlainViewer(pf, v, path, false)

	var vv *viewer.ViewerView
	timeout := time.After(2 * time.Second)
	for vv == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("timeout waiting for the text viewer to open")
		}
		if top, ok := vtui.FrameManager.GetTopFrame().(*viewer.ViewerView); ok {
			vv = top
		}
	}

	if !RunAction("Viewer.MarkdownFormatted") {
		t.Fatal("Viewer.MarkdownFormatted failed to run")
	}

	var mv *markdownView
	timeout = time.After(2 * time.Second)
	for mv == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("timeout waiting for the formatted view to reopen")
		}
		if top, ok := vtui.FrameManager.GetTopFrame().(*markdownView); ok {
			mv = top
		}
	}
	defer mv.Close()

	if !vv.IsDone() {
		t.Error("the text viewer is still open after switching back to the formatted view")
	}
}

// TestActionSwitchViewerToMarkdownDisabledForOrdinaryFiles keeps Shift+F3 a
// no-op (dimmed, per f4#1356's Enabled convention) outside a Markdown file:
// there is no formatted view to switch to.
func TestActionSwitchViewerToMarkdownDisabledForOrdinaryFiles(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("plain text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	openPlainViewer(pf, v, path, false)

	var vv *viewer.ViewerView
	timeout := time.After(2 * time.Second)
	for vv == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("timeout waiting for the text viewer to open")
		}
		if top, ok := vtui.FrameManager.GetTopFrame().(*viewer.ViewerView); ok {
			vv = top
		}
	}
	defer vv.Close()

	if RunAction("Viewer.MarkdownFormatted") {
		t.Fatal("Viewer.MarkdownFormatted ran for a non-Markdown file, want it disabled")
	}
	if _, ok := vtui.FrameManager.GetTopFrame().(*viewer.ViewerView); !ok {
		t.Fatal("the text viewer is no longer the top frame after the disabled action")
	}
}
