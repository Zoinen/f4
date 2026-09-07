package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type documentRangeReader struct {
	data         []byte
	profile      vfs.ReadAccessProfile
	shortRead    int
	started      chan struct{}
	release      chan struct{}
	ignoreCancel bool
	mu           sync.Mutex
	reads        []trackedReadRange
	active       int
	maxActive    int
	closes       atomic.Int32
}

func (f *documentRangeReader) Size() int64                              { return int64(len(f.data)) }
func (f *documentRangeReader) Close() error                             { f.closes.Add(1); return nil }
func (f *documentRangeReader) ReadAccessProfile() vfs.ReadAccessProfile { return f.profile }
func (f *documentRangeReader) Read(ctx context.Context, dst []byte) (int, error) {
	return f.ReadAt(ctx, dst, 0)
}
func (f *documentRangeReader) ReadAt(ctx context.Context, dst []byte, off int64) (int, error) {
	f.mu.Lock()
	first := len(f.reads) == 0
	f.reads = append(f.reads, trackedReadRange{offset: off, length: len(dst)})
	f.active++
	f.maxActive = max(f.maxActive, f.active)
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	if first && f.started != nil {
		close(f.started)
		if f.ignoreCancel {
			<-f.release
		} else {
			select {
			case <-f.release:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
	}
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	if f.shortRead > 0 {
		dst = dst[:min(len(dst), f.shortRead)]
	}
	n := copy(dst, f.data[off:])
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

func TestDocumentReadCompletesShortReadsWithoutPadding(t *testing.T) {
	file := &documentRangeReader{data: []byte("0123456789abcdef"), shortRead: 3}
	dst := make([]byte, len(file.data))
	n, err := readDocumentBytes(context.Background(), file, dst, 0)
	if err != nil || n != len(dst) || !bytes.Equal(dst, file.data) {
		t.Fatalf("read=%q n=%d err=%v", dst, n, err)
	}
	n, err = readDocumentBytes(context.Background(), file, make([]byte, 30), 0)
	if !errors.Is(err, io.EOF) || n != len(file.data) {
		t.Fatalf("short EOF n=%d err=%v", n, err)
	}
}

func TestPreparedEditorReusesProbeAndTransfersOwnership(t *testing.T) {
	oldDefault, oldDetect := AppConfig.EditorDefaultCodePage, AppConfig.EditorAutodetectCodePage
	defer func() { AppConfig.EditorDefaultCodePage = oldDefault; AppConfig.EditorAutodetectCodePage = oldDetect }()
	AppConfig.EditorDefaultCodePage = 65001
	AppConfig.EditorAutodetectCodePage = false
	file := &documentRangeReader{data: bytes.Repeat([]byte("line\n"), 20000)}
	prepared, err := prepareEditorDocument(context.Background(), nil, "large.txt", file)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.buf == nil {
		t.Fatal("large editor lost streaming source")
	}
	if len(file.reads) != 2 || file.reads[0].offset != 0 || file.reads[0].length != editorEncodingProbeSize || file.reads[1].offset != editorEncodingProbeSize {
		t.Fatalf("duplicate probe read: %+v", file.reads)
	}
	if file.closes.Load() != 0 {
		t.Fatal("prepared source closed before transfer")
	}
	prepared.close()
	if file.closes.Load() != 1 {
		t.Fatal("prepared source ownership not released")
	}
}

func TestPreparedLegacyEditorReadsOnlyProbeRemainder(t *testing.T) {
	oldDefault, oldDetect := AppConfig.EditorDefaultCodePage, AppConfig.EditorAutodetectCodePage
	defer func() { AppConfig.EditorDefaultCodePage = oldDefault; AppConfig.EditorAutodetectCodePage = oldDetect }()
	AppConfig.EditorDefaultCodePage = 1251
	AppConfig.EditorAutodetectCodePage = false
	file := &documentRangeReader{data: bytes.Repeat([]byte{0xc0, '\n'}, 20000)}
	prepared, err := prepareEditorDocument(context.Background(), nil, "legacy.txt", file)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.close()
	if len(file.reads) != 2 || file.reads[1].offset != editorEncodingProbeSize {
		t.Fatalf("legacy decode reread probe: %+v", file.reads)
	}
	if prepared.buf != nil || prepared.codepage != 1251 {
		t.Fatalf("legacy preparation = %+v", prepared)
	}
}

func TestEditorPreparationUsesOpeningEncodingSnapshot(t *testing.T) {
	oldDefault, oldDetect := AppConfig.EditorDefaultCodePage, AppConfig.EditorAutodetectCodePage
	defer func() { AppConfig.EditorDefaultCodePage, AppConfig.EditorAutodetectCodePage = oldDefault, oldDetect }()
	for _, test := range []struct {
		name           string
		initialDefault int
		initialDetect  bool
		laterDefault   int
		data           []byte
		wantCodepage   int
		wantText       string
	}{
		{"default_codepage", 1251, false, 1252, []byte{0xe9}, 1251, "й"},
		{"autodetect", 1251, true, 1251, []byte("€"), 65001, "€"},
	} {
		t.Run(test.name, func(t *testing.T) {
			AppConfig.EditorDefaultCodePage, AppConfig.EditorAutodetectCodePage = test.initialDefault, test.initialDetect
			encoding := snapshotEditorOpeningEncoding()
			file := &documentRangeReader{data: test.data, started: make(chan struct{}), release: make(chan struct{})}
			type preparationResult struct {
				prepared *preparedEditorDocument
				err      error
			}
			result := make(chan preparationResult, 1)
			go func() {
				prepared, err := prepareEditorDocumentWithEncoding(context.Background(), nil, "encoding.txt", file, encoding)
				result <- preparationResult{prepared, err}
			}()
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(file.release) }) }
			defer release()
			select {
			case <-file.started:
			case <-time.After(time.Second):
				t.Fatal("encoding probe did not start")
			}
			// Simulate another UI action changing defaults while this open is
			// blocked in provider I/O. Neither setting belongs to this worker.
			AppConfig.EditorDefaultCodePage, AppConfig.EditorAutodetectCodePage = test.laterDefault, false
			release()
			select {
			case got := <-result:
				if got.err != nil {
					t.Fatal(got.err)
				}
				defer got.prepared.close()
				data, err := got.prepared.pt.Bytes()
				if err != nil || got.prepared.codepage != test.wantCodepage || string(data) != test.wantText {
					t.Fatalf("open used changed defaults: codepage=%d text=%q err=%v", got.prepared.codepage, data, err)
				}
			case <-time.After(time.Second):
				t.Fatal("encoding preparation did not complete")
			}
		})
	}
}

func TestAsyncBufferReadContextSerializesAndCancelsWithoutPolling(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &documentRangeReader{data: make([]byte, 100000), started: make(chan struct{}), release: make(chan struct{})}
	buf := NewAsyncBuffer(context.Background(), file)
	defer buf.Close()
	if _, err := buf.Read(0, 1); err != piecetable.ErrLoading {
		t.Fatalf("read error=%v", err)
	}
	select {
	case <-file.started:
	case <-time.After(time.Second):
		t.Fatal("loader not started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buf.ReadContext(ctx, 40000, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting reader error=%v", err)
	}
	close(file.release)
	if _, err := buf.ReadContext(context.Background(), 0, 90000); err != nil {
		t.Fatal(err)
	}
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.maxActive != 1 || len(file.reads) != 3 {
		t.Fatalf("active=%d reads=%+v", file.maxActive, file.reads)
	}
}

func TestViewerDirectLocalRangeReadyInCallingTurn(t *testing.T) {
	file := &documentRangeReader{data: bytes.Repeat([]byte("abcdef"), 200000), profile: vfs.ReadAccessDirectLocal}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{file: file, size: file.Size(), ctx: ctx, cancelCtx: cancel}
	defer backend.Close()
	got, err := backend.ReadAt(500000, 120)
	if err != nil || !bytes.Equal(got, file.data[500000:500120]) {
		t.Fatalf("direct miss not ready: %v", err)
	}
	if backend.isFetching || len(file.reads) != 1 || file.reads[0].length > 256*1024 {
		t.Fatalf("unbounded/deferred local read: %+v", file.reads)
	}
}

func TestViewerLineSeekDoesNotAcknowledgeFailedDirectRead(t *testing.T) {
	failure := errors.New("failed local source range")
	file := &failingViewerFile{err: failure}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{file: file, size: file.Size(), owner: vfs.NewOSVFS(t.TempDir()), ctx: ctx, cancelCtx: cancel}
	defer backend.Close()
	if _, ready := backend.TryFindLineStart(12); ready {
		t.Fatal("failed line seek was acknowledged as ready")
	}
	if !errors.Is(backend.LastReadError(), failure) {
		t.Fatalf("source error=%v", backend.LastReadError())
	}
}

func TestViewerRangeSupersessionOnlyCommitsNewestRequest(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &documentRangeReader{data: bytes.Repeat([]byte("abcdef"), 600000), started: make(chan struct{}), release: make(chan struct{}), ignoreCancel: true}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{file: file, size: file.Size(), ctx: ctx, cancelCtx: cancel}
	defer backend.Close()
	if _, err := backend.ReadAt(0, 32); err != piecetable.ErrLoading {
		t.Fatal(err)
	}
	select {
	case <-file.started:
	case <-time.After(time.Second):
		t.Fatal("first range not started")
	}
	_, _ = backend.ReadAt(1000000, 32)
	_, _ = backend.ReadAt(2000000, 32)
	close(file.release)
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	got, err := backend.ReadContext(readCtx, 2000000, 32)
	if err != nil || !bytes.Equal(got, file.data[2000000:2000032]) {
		t.Fatalf("newest range not ready: %v", err)
	}
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.maxActive != 1 || len(file.reads) != 2 || file.reads[1].offset != 2000000-64*1024 {
		t.Fatalf("obsolete work queued or concurrent: active=%d reads=%+v", file.maxActive, file.reads)
	}
	if backend.cacheOff != 2000000-64*1024 {
		t.Fatalf("stale window committed at %d", backend.cacheOff)
	}
}

func TestPendingDocumentOpenCoalescesCancelsAndRejectsObsoleteCompletion(t *testing.T) {
	pf := &PanelsFrame{}
	defer cancelPendingDocumentOpen(pf)
	first := beginPendingDocumentOpen(pf, "viewer", nil, "one.txt")
	if first == nil || beginPendingDocumentOpen(pf, "viewer", nil, "one.txt") != nil {
		t.Fatal("duplicate open was not coalesced")
	}
	second := beginPendingDocumentOpen(pf, "editor", nil, "two.txt")
	if first.ctx.Err() != context.Canceled || second == nil {
		t.Fatal("superseded open not cancelled")
	}
	if finishPendingDocumentOpen(pf, first) || pendingDocumentOpens[pf] != second {
		t.Fatal("obsolete completion changed current open")
	}
	if !cancelPendingDocumentOpen(pf) || finishPendingDocumentOpen(pf, second) {
		t.Fatal("cancelled completion reopened document")
	}
	third := beginPendingDocumentOpen(pf, "viewer", nil, "one.txt")
	if !finishPendingDocumentOpen(pf, third) || pendingDocumentOpens[pf] != nil {
		t.Fatal("current completion not accepted exactly once")
	}
}

// This provider deliberately ignores cancellation while opening and reports
// progress after cancellation too. Its caller must own presentation lifetime;
// a remote provider cannot be relied upon to promptly return or stop callbacks.
type cancellationIgnoringDocumentVFS struct {
	*mockSlowVFS
	started chan struct{}
	release chan struct{}
	file    *documentRangeReader
}

func (v *cancellationIgnoringDocumentVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	update, _ := ctx.Value(vfs.ProgressKey).(vfs.ProgressCallback)
	if update != nil {
		update("Opening source", 0)
	}
	close(v.started)
	<-v.release
	if update != nil {
		update("Obsolete source completed", 50)
	}
	return v.file, nil
}

func TestPendingRemoteOpenCancellationOwnsProgressLifetime(t *testing.T) {
	for _, scenario := range []string{"escape_before_dialog", "cancel_visible_dialog", "supersede_before_dialog"} {
		t.Run(scenario, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			pf := &PanelsFrame{}
			file := &documentRangeReader{data: []byte("source content\n")}
			filesystem := &cancellationIgnoringDocumentVFS{
				mockSlowVFS: &mockSlowVFS{},
				started:     make(chan struct{}),
				release:     make(chan struct{}),
				file:        file,
			}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(filesystem.release) }) }
			completed, accepted := false, false // accessed only by UI callbacks
			pumpUntil := func(timeout time.Duration, condition func() bool) bool {
				timer := time.NewTimer(timeout)
				defer timer.Stop()
				for !condition() {
					select {
					case task := <-vtui.FrameManager.TaskChan:
						task()
					case <-timer.C:
						return condition()
					}
				}
				return true
			}
			liveProgress := func() bool {
				for _, screen := range vtui.FrameManager.Screens {
					for _, frame := range screen.Frames {
						if !frame.IsDone() && strings.Contains(frame.GetTitle(), "Opening") {
							return true
						}
					}
				}
				return false
			}
			op := beginPendingDocumentOpen(pf, "viewer", filesystem, "/mock/file.txt")
			var viewer *ViewerView
			runPendingDocumentOpen(pf, filesystem, op, "Preparing source", func(ctx context.Context) error {
				var err error
				viewer, err = NewViewerView(ctx, filesystem, "/mock/file.txt")
				return err
			}, func(err error) {
				accepted = finishPendingDocumentOpen(pf, op)
				if viewer != nil {
					_ = viewer.backend.Close()
				}
				completed = true
			})
			defer func() {
				cancelPendingDocumentOpen(pf)
				release()
				if !pumpUntil(time.Second, func() bool { return completed }) {
					t.Error("cancelled worker failed to finish during cleanup")
				}
			}()
			select {
			case <-filesystem.started:
			case <-time.After(time.Second):
				t.Fatal("remote opening did not start")
			}

			var replacement *pendingDocumentOpen
			switch scenario {
			case "escape_before_dialog":
				key := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
				if !pf.VetoActionKey(key) || !pf.ProcessKey(key) {
					t.Fatal("pending open did not own Escape")
				}
			case "cancel_visible_dialog":
				if !pumpUntil(time.Second, liveProgress) {
					t.Fatal("slow live open did not show its progress dialog")
				}
				cancelPendingDocumentOpen(pf)
				if !pumpUntil(time.Second, func() bool { return !liveProgress() }) {
					t.Fatal("cancelled dialog stayed visible waiting for an uncooperative provider")
				}
			case "supersede_before_dialog":
				replacement = beginPendingDocumentOpen(pf, "editor", filesystem, "/mock/other.txt")
			}
			if op.ctx.Err() != context.Canceled {
				t.Fatal("opening lifecycle was not cancelled")
			}
			// Drain past the real presentation deadline with Open still blocked.
			// This also checks progress queued immediately before cancellation.
			if pumpUntil(openingProgressDelay+50*time.Millisecond, liveProgress) {
				t.Fatal("obsolete opening resurrected a progress dialog")
			}
			if completed {
				t.Fatal("test provider did not stay blocked through cancellation")
			}
			release()
			if !pumpUntil(time.Second, func() bool { return completed }) {
				t.Fatal("late remote completion was not processed")
			}
			if accepted || liveProgress() || file.closes.Load() != 1 {
				t.Fatalf("obsolete completion: accepted=%v live dialog=%v source closes=%d", accepted, liveProgress(), file.closes.Load())
			}
			if pendingDocumentOpens[pf] != replacement {
				t.Fatal("obsolete completion changed its replacement lifecycle")
			}
		})
	}
}

func TestOpeningProgressDialogFollowsRuntimeTheme(t *testing.T) {
	indices := []int{vtui.ColDialogText, vtui.ColDialogEdit, vtui.ColDialogBox,
		vtui.ColDialogBoxTitle, vtui.ColDialogButton, vtui.ColDialogSelectedButton,
		vtui.ColDialogHighlightButton, vtui.ColDialogHighlightSelectedButton}
	original := make(map[int]uint64, len(indices))
	for _, index := range indices {
		original[index] = vtui.Palette[index]
	}
	defer func() {
		for index, attr := range original {
			vtui.Palette[index] = attr
		}
	}()
	applyTheme := func(variant int) {
		for i, index := range indices {
			vtui.Palette[index] = vtui.SetRGBBoth(0, uint32(0x101010+variant*0x202020+i*0x010101), uint32(0x909090+variant*0x101010+i*0x010101))
		}
	}
	applyTheme(0)
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	pf := &PanelsFrame{}
	release := make(chan struct{})
	completed := false
	// Exercise the unchanged generic/immediate caller contract as well as the
	// actual opening dialog construction; no document lifetime is supplied.
	pf.runProgressTaskAfter(0, " Opening... ", "Opening source", false, func(ctx context.Context, update func(string, int)) error {
		<-release
		return nil
	}, func(error) { completed = true })
	defer func() {
		close(release)
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for !completed {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			case <-timer.C:
				t.Error("progress worker failed to complete during cleanup")
				return
			}
		}
	}()
	var dialog *vtui.Window
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for dialog == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			for _, appScreen := range vtui.FrameManager.Screens {
				for _, frame := range appScreen.Frames {
					if frame.GetTitle() == " Opening... " {
						dialog, _ = frame.(*vtui.Window)
					}
				}
			}
		case <-timer.C:
			t.Fatal("immediate progress dialog was not shown")
		}
	}
	var labels []*vtui.Text
	var progress *vtui.ProgressBar
	var button *vtui.Button
	for _, child := range dialog.GetChildren() {
		switch control := child.(type) {
		case *vtui.Text:
			labels = append(labels, control)
		case *vtui.ProgressBar:
			progress = control
		case *vtui.Button:
			button = control
		}
	}
	if len(labels) != 2 || progress == nil || button == nil {
		t.Fatal("progress dialog lost its expected controls")
	}
	progress.SetPercent(50)
	button.SetText("&Cancel")
	checkAttr := func(name string, x, y, paletteIndex int) {
		t.Helper()
		if got, want := screen.GetCell(x, y).Attributes, vtui.Palette[paletteIndex]; got != want {
			t.Fatalf("%s at %d,%d: attr=%#x, want palette[%d]=%#x", name, x, y, got, paletteIndex, want)
		}
	}
	for variant := 0; variant < 2; variant++ {
		applyTheme(variant)
		for _, focused := range []bool{false, true} {
			button.SetFocus(focused)
			dialog.Show(screen)
			for _, label := range labels {
				checkAttr("label", label.X1, label.Y1, vtui.ColDialogText)
			}
			checkAttr("progress filled", progress.X1, progress.Y1, vtui.ColDialogEdit)
			checkAttr("progress empty", progress.X2, progress.Y1, vtui.ColDialogText)
			checkAttr("border", dialog.X1, dialog.Y1+1, vtui.ColDialogBox)
			buttonIndex, hotkeyIndex := vtui.ColDialogButton, vtui.ColDialogHighlightButton
			if focused {
				buttonIndex, hotkeyIndex = vtui.ColDialogSelectedButton, vtui.ColDialogHighlightSelectedButton
			}
			checkAttr("cancel button", button.X1, button.Y1, buttonIndex)
			checkAttr("cancel hotkey", button.X1+2, button.Y1, hotkeyIndex)
		}
	}
}
