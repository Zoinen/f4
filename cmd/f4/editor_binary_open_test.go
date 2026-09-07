package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type editorRenderTrackingBuffer struct {
	data    []byte
	mu      sync.Mutex
	maxRead int
}

func (b *editorRenderTrackingBuffer) Size() int { return len(b.data) }

func (b *editorRenderTrackingBuffer) Read(offset, length int) ([]byte, error) {
	b.mu.Lock()
	if length > b.maxRead {
		b.maxRead = length
	}
	b.mu.Unlock()
	return b.data[offset : offset+length], nil
}

func (b *editorRenderTrackingBuffer) reset() {
	b.mu.Lock()
	b.maxRead = 0
	b.mu.Unlock()
}

func (b *editorRenderTrackingBuffer) largestRead() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maxRead
}

// NUL headers are binary; cp1251 text (invalid UTF-8 but NUL-free) and
// UTF-16 text (NULs decoded away) are not.
func TestEditorHeaderIsBinary(t *testing.T) {
	if !editorHeaderIsBinary([]byte{0, 0, 0, 0x20, 'f', 't'}, 65001) {
		t.Error("NUL header must be binary")
	}
	cp1251 := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2} // "Привет"
	for _, cp := range []int{65001, 1251} {
		if editorHeaderIsBinary(cp1251, cp) {
			t.Errorf("cp1251 text must not be binary (cp=%d)", cp)
		}
	}
	if editorHeaderIsBinary([]byte{0xFF, 0xFE, 'h', 0, 'i', 0}, 1200) {
		t.Error("UTF-16 text must not be binary")
	}
}

// Hex/decode render by byte offset: no line scan may run, and a pending
// line-based restore is meaningless there.
func TestStartIndexingSkipsHexAndDecodeModes(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	pt := piecetable.New([]byte("line one\nline two\n"))
	for _, tc := range []struct {
		mode string
		hex  bool
		deco bool
	}{{"hex", true, false}, {"decode", false, true}} {
		ev := newEditorView(pt, nil, "", false, true)
		ev.HexMode, ev.DecodeMode = tc.hex, tc.deco
		ev.targetLine = 1
		ev.StartIndexing()
		if ev.targetLine != -1 {
			t.Errorf("%s mode must clear a pending target line, got %d", tc.mode, ev.targetLine)
		}
		if ev.indexing || ev.indexIsComplete() {
			t.Errorf("%s mode must not run the line-index scan", tc.mode)
		}
	}
}

func TestEditorHexRenderSkipsTextLayout(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	buffer := &editorRenderTrackingBuffer{data: make([]byte, 128*1024)}
	ev := NewEditorViewIndexedLater(piecetable.NewWithBuffer(buffer), nil, "sample.bin")
	defer ev.Close()
	ev.HexMode = true
	ev.SetPosition(0, 0, 80, 24)
	buffer.reset()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	ev.Show(scr)

	if got := buffer.largestRead(); got > 16 {
		t.Fatalf("hex render read %d bytes from the text buffer, want at most 16", got)
	}
}

func TestBinaryEditorSkipsColorer(t *testing.T) {
	oldHighlighter := AppConfig.EditorHighlighter
	AppConfig.EditorHighlighter = "Colorer"
	t.Cleanup(func() { AppConfig.EditorHighlighter = oldHighlighter })

	ev := newEditorView(piecetable.New([]byte{0, 1, 2, 3}), nil, "sample.bin", false, true)
	defer ev.Close()
	if !ev.binaryFile {
		t.Fatal("binary buffer was not detected")
	}
	if ev.highlighter != nil {
		t.Fatalf("binary editor initialized %T; Colorer must be skipped", ev.highlighter)
	}
}

func TestAwaitOffsetAsyncDoesNotReadOnUIThread(t *testing.T) {
	buffer := &editorRenderTrackingBuffer{data: bytes.Repeat([]byte("x"), 1024*1024)}
	ev := newEditorView(piecetable.NewWithBuffer(buffer), nil, "", false, true)
	defer ev.Close()

	buffer.reset()
	ev.indexing = true
	ev.indexStatus = IndexStatus{Phase: IndexScanning, Total: int64(len(buffer.data))}
	if !ev.awaitOffsetAsync(0) {
		t.Fatal("awaitOffsetAsync() did not start waiting for the index")
	}
	if got := buffer.largestRead(); got != 0 {
		t.Fatalf("awaitOffsetAsync() synchronously read %d bytes", got)
	}
}

func TestEditorEscapeCancelsIndexing(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := newEditorView(piecetable.New([]byte("text")), nil, "", false, true)
	defer ev.Close()

	cancelled := false
	ev.indexing = true
	ev.indexStatus = IndexStatus{Phase: IndexScanning, Total: 4, Scanned: 1}
	ev.indexCancel = func() { cancelled = true }
	ev.targetOffset = 123
	ev.targetLine = -1

	escape := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}
	if !ev.VetoActionKey(escape) {
		t.Fatal("Escape was not reserved for the editor while indexing")
	}
	handled := ev.ProcessKey(escape)
	if !handled {
		t.Fatal("Escape was not handled while indexing")
	}
	if !cancelled || ev.indexing || ev.indexCancel != nil {
		t.Errorf("indexing was not cancelled: cancelled=%v indexing=%v cancel=%v", cancelled, ev.indexing, ev.indexCancel != nil)
	}
	if ev.targetOffset != -1 || ev.targetLine != -1 {
		t.Errorf("pending target was not cleared: offset=%d line=%d", ev.targetOffset, ev.targetLine)
	}
	if ev.IndexState().Phase != IndexIdle {
		t.Errorf("index phase = %v, want idle", ev.IndexState().Phase)
	}
}

func TestEditorEscapeCancelsColorer(t *testing.T) {
	ch := &ColorerHighlighter{}
	ev := &EditorView{
		highlighter:     ch,
		colorerIndexing: true,
		colorerCancel:   ch.Cancel,
	}
	e := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}

	if !ev.VetoActionKey(e) {
		t.Fatal("Escape was not reserved for the editor while Colorer is working")
	}
	ev.cancelColorer()
	if !ch.disabled || ev.colorerIndexing || ev.colorerCancel != nil {
		t.Fatalf("Colorer was not cancelled: disabled=%v indexing=%v cancel=%v", ch.disabled, ev.colorerIndexing, ev.colorerCancel != nil)
	}
}

// A binary file opened for editing goes straight into hex on the lazy chunked
// path (codepage 65001) without a background scan.
func TestShowEditorBinaryOpensInHex(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.bin")
	if err := os.WriteFile(path, []byte{0, 0, 0, 0x20, 'f', 't', 'y', 'p', 0x0A, 'x'}, 0600); err != nil {
		t.Fatal(err)
	}

	localVFS := vfs.NewOSVFS(dir)
	_ = localVFS.SetPath(dir)
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	for _, panel := range pf.panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			if fsp.cancelLoad != nil {
				fsp.cancelLoad()
			}
			fsp.stopLoadingAnimation()
		}
	}
	left := NewFileSystemPanel(0, 0, 40, 20, localVFS)
	right := NewFileSystemPanel(40, 0, 40, 20, localVFS.Clone())
	pf.panels[0] = left
	pf.panels[1] = right
	waitForLoad(t, left)
	waitForLoad(t, right)
	pf.ResizeConsole(120, 60)
	vtui.FrameManager.Push(pf)

	f, err := localVFS.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	showEditor(pf, localVFS, path, f)

	ev, _ := findOpenedEditor(localVFS, path)
	if ev == nil {
		t.Fatal("editor was not opened")
	}
	t.Cleanup(ev.Close)
	if !ev.HexMode || ev.Codepage != 65001 {
		t.Errorf("binary file must open in hex with codepage 65001, got hex=%v cp=%d", ev.HexMode, ev.Codepage)
	}
	if !ev.binaryFile {
		t.Error("binary file must disable syntax parsing without disabling text editing")
	}
	if ev.indexing || ev.indexIsComplete() {
		t.Error("binary file in hex mode must not run the line-index scan")
	}
}
