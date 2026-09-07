package main

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type viewerEndContentFile struct {
	data []byte
	mu   sync.Mutex
	read []trackedReadRange
}

func (f *viewerEndContentFile) Size() int64 { return int64(len(f.data)) }
func (*viewerEndContentFile) Close() error  { return nil }
func (f *viewerEndContentFile) Read(ctx context.Context, p []byte) (int, error) {
	return f.ReadAt(ctx, p, 0)
}
func (f *viewerEndContentFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	f.mu.Lock()
	f.read = append(f.read, trackedReadRange{offset: off, length: n})
	f.mu.Unlock()
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *viewerEndContentFile) ranges() []trackedReadRange {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]trackedReadRange(nil), f.read...)
}

func runViewerEndAndWait(t *testing.T, vv *ViewerView) {
	t.Helper()
	if !vv.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END,
	}) {
		t.Fatal("End was not handled")
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for vv.Busy {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatal("End navigation timed out")
		}
	}
}

func referenceViewerEndOffset(t *testing.T, data []byte, width, height int, wrap bool) int64 {
	t.Helper()
	var offsets []int64
	var offset int64
	column := 0
	for offset < int64(len(data)) {
		offsets = append(offsets, offset)
		scan := scanViewerText(data[offset:], width, wrap, column, 0, false)
		if scan.lineLen <= 0 {
			t.Fatalf("reference layout made no progress at %d", offset)
		}
		offset += int64(scan.lineLen)
		column = scan.nextColumn
	}
	if len(offsets) <= height {
		return 0
	}
	return offsets[len(offsets)-height]
}

func viewerEndBoundaryFixture(kind string) (data []byte, tailStart int64) {
	const tailWindow = 192 * 1024
	const total = 3*tailWindow + 38
	data = bytes.Repeat([]byte{'p'}, total)
	tailStart = int64(len(data) - tailWindow)
	cut := int(tailStart)
	// The tail window begins inside a logical line. The first newline after the
	// cut is therefore the first source position from which layout is proven.
	data[cut-25] = '\n'
	for i := cut - 24; i <= cut+23; i++ {
		data[i] = 'x'
	}
	switch kind {
	case "tab":
		data[cut-3] = '\t'
	case "unicode":
		copy(data[cut-1:cut+2], []byte("界"))
	}
	data[cut+23] = '\n'
	anchor := cut + 24
	pattern := []byte("ab\t界z")
	if kind == "unicode" {
		pattern = []byte("界az")
	}
	for pos := anchor; pos < len(data); {
		n := copy(data[pos:], pattern)
		pos += n
	}
	return data, tailStart
}

func TestViewerEndTailWindowUsesOnlyProvenRowsAndCarriesLayoutState(t *testing.T) {
	oldTabSize := AppConfig.EditorTabSize
	AppConfig.EditorTabSize = 4
	t.Cleanup(func() { AppConfig.EditorTabSize = oldTabSize })

	tests := []struct {
		name          string
		kind          string
		wrap          bool
		trailingLF    bool
		width, height int
	}{
		{name: "wrapped_tab_state_across_chunks", kind: "tab", wrap: true, width: 13, height: 5},
		{name: "wrapped_utf8_boundary_and_chunks", kind: "unicode", wrap: true, width: 11, height: 4},
		{name: "unwrapped_chunk_starts_are_not_lines", kind: "unicode", wrap: false, width: 17, height: 1},
		{name: "unwrapped_terminal_lf_is_not_a_row", kind: "tab", wrap: false, trailingLF: true, width: 17, height: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			data, tailStart := viewerEndBoundaryFixture(tc.kind)
			if tc.trailingLF {
				data[len(data)-1] = '\n'
			}
			file := &viewerEndContentFile{data: data}
			ctx, cancel := context.WithCancel(context.Background())
			backend := &ViewerBackend{
				file: file, size: int64(len(data)), totalLines: -1, totalForSize: -1,
				ctx: ctx, cancelCtx: cancel,
			}
			vv := &ViewerView{
				backend: backend, WrapMode: tc.wrap,
				nativeViewportColumns: tc.width, nativeViewportRows: tc.height,
			}
			defer vv.Close()

			want := referenceViewerEndOffset(t, data, tc.width, tc.height, tc.wrap)
			runViewerEndAndWait(t, vv)
			if vv.TopOffset != want {
				t.Fatalf("End top offset=%d, want full-layout offset %d (tail starts at %d)",
					vv.TopOffset, want, tailStart)
			}
			if tc.wrap {
				window := vv.semanticWindow()
				if !window.ready || window.viewportRow >= len(window.rows) ||
					window.rows[window.viewportRow].Offset != want {
					t.Fatalf("accepted End did not publish from its retained wrap origin: %+v", window)
				}
			}

			ranges := file.ranges()
			if len(ranges) != 1 {
				t.Fatalf("anchored tail navigation made %d source reads, want one: %+v", len(ranges), ranges)
			}
			if ranges[0].length > 256*1024 || ranges[0].offset < int64(len(data))-256*1024 {
				t.Fatalf("anchored tail read escaped the bounded final cache window: %+v", ranges[0])
			}
		})
	}
}

func TestViewerEndNewlineFreeTabAndUTF8LineCarriesAcrossReadSlices(t *testing.T) {
	oldTabSize := AppConfig.EditorTabSize
	AppConfig.EditorTabSize = 4
	t.Cleanup(func() { AppConfig.EditorTabSize = oldTabSize })

	const (
		tailWindow = 192 * 1024
		fileSize   = 3*tailWindow + 60
		width      = 13
		height     = 7
	)
	pattern := []byte("a\t界b")
	if fileSize%len(pattern) != 0 {
		t.Fatal("fixture must end on a complete UTF-8 pattern")
	}
	data := bytes.Repeat(pattern, fileSize/len(pattern))
	// 64 KiB ends between the UTF-8 bytes of 界 for this six-byte pattern.
	if !bytes.Equal(data[viewerEndReadSlice-2:viewerEndReadSlice+1], []byte("界")) {
		t.Fatal("fixture does not split a UTF-8 rune across a forward read slice")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &viewerEndContentFile{data: data}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{
		file: file, size: int64(len(data)), totalLines: -1, totalForSize: -1,
		ctx: ctx, cancelCtx: cancel,
	}
	vv := &ViewerView{
		backend: backend, WrapMode: true,
		nativeViewportColumns: width, nativeViewportRows: height,
	}
	defer vv.Close()

	want := referenceViewerEndOffset(t, data, width, height, true)
	runViewerEndAndWait(t, vv)
	if vv.TopOffset != want {
		t.Fatalf("End top offset=%d, want full-layout offset %d", vv.TopOffset, want)
	}
	readsBeforeWindow := len(file.ranges())
	window := vv.semanticWindow()
	if !window.ready || window.viewportRow >= len(window.rows) ||
		window.rows[window.viewportRow].Offset != want {
		t.Fatalf("accepted End did not reuse the retained tab/UTF-8 origin: %+v", window)
	}
	if reads := file.ranges(); len(reads) != readsBeforeWindow {
		t.Fatalf("publishing the accepted End reread the long logical line: before=%d after=%d", readsBeforeWindow, len(reads))
	}
	for _, read := range file.ranges() {
		if read.length > 256*1024 {
			t.Fatalf("End source read=%+v, want every I/O slice at most 256 KiB", read)
		}
	}
}

func TestViewerEndLongLineKeepsBOFWrapPhaseWithBoundedSlices(t *testing.T) {
	const (
		tailWindow = int64(192 * 1024)
		fileSize   = 3*tailWindow + 57
		width      = int64(120)
		height     = int64(40)
	)
	const originalReproducerSize = int64(392077017)
	if originalTop := ((originalReproducerSize-1)/width - (height - 1)) * width; originalTop != 392072280 {
		t.Fatalf("original long-line reference offset=%d, want 392072280", originalTop)
	}
	if phase := (fileSize - tailWindow) % width; phase == 0 {
		t.Fatal("fixture does not cut a visual row at the tail boundary")
	}

	for _, tc := range []struct {
		name string
		wrap bool
		want int64
	}{
		{
			name: "wrapped",
			wrap: true,
			want: ((fileSize-1)/width - (height - 1)) * width,
		},
		{name: "unwrapped", wrap: false, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			file := &tailTrackingFile{size: fileSize}
			ctx, cancel := context.WithCancel(context.Background())
			backend := &ViewerBackend{
				file: file, size: fileSize, totalLines: -1, totalForSize: -1,
				ctx: ctx, cancelCtx: cancel,
			}
			vv := &ViewerView{
				backend: backend, WrapMode: tc.wrap,
				nativeViewportColumns: int(width), nativeViewportRows: int(height),
			}
			defer vv.Close()

			runViewerEndAndWait(t, vv)
			if vv.TopOffset != tc.want {
				t.Fatalf("End top offset=%d, want %d", vv.TopOffset, tc.want)
			}
			if tc.wrap {
				readsBeforeWindow := len(file.ranges())
				window := vv.semanticWindow()
				if !window.ready || window.viewportRow >= len(window.rows) ||
					window.rows[window.viewportRow].Offset != tc.want {
					t.Fatalf("accepted End did not reuse the retained long-line origin: %+v", window)
				}
				if reads := file.ranges(); len(reads) != readsBeforeWindow {
					t.Fatalf("publishing End reread the newline-free line: before=%d after=%d", readsBeforeWindow, len(reads))
				}
			}
			for _, read := range file.ranges() {
				if read.length > 256*1024 {
					t.Fatalf("End source read=%+v, want every I/O slice at most 256 KiB", read)
				}
			}
			backend.mu.Lock()
			cached := len(backend.cacheData)
			backend.mu.Unlock()
			if cached > 256*1024 {
				t.Fatalf("End retained %d source bytes, want at most one backend window", cached)
			}
		})
	}
}

var _ vfs.ReadAtCloser = (*viewerEndContentFile)(nil)
