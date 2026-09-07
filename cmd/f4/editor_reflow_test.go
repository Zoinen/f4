package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type reflowMeasuredBuffer struct {
	data      []byte
	readBytes int
}

func (b *reflowMeasuredBuffer) Size() int { return len(b.data) }
func (b *reflowMeasuredBuffer) Read(offset, length int) ([]byte, error) {
	b.readBytes += length
	return b.data[offset:min(offset+length, len(b.data))], nil
}

func TestEditorReflowYieldsAndKeepsAnchorAcrossRepeatedResize(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	b := &reflowMeasuredBuffer{data: bytes.Repeat([]byte("a"), 8*1024*1024)}
	ev := NewEditorView(piecetable.NewWithBuffer(b), nil, "")
	defer ev.Close()
	ev.WordWrap, ev.nativeViewportColumns = true, 80
	ev.ensureEngineWidth()
	ev.ScrollTopRow = 90000
	ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	ev.nativeViewportColumns = 79
	b.readBytes = 0
	if ev.ensureEngineWidth() {
		t.Fatal("deep reflow did not yield")
	}
	if b.readBytes > editorMappingWorkBytes+4096 {
		t.Fatalf("unbounded reflow: %d bytes", b.readBytes)
	}
	if ev.ScrollTopRow != 90000 {
		t.Fatal("partial mapping replaced viewport")
	}
	// A second resize must retain the original source anchor, not reinterpret
	// the old row number in the unfinished intermediate layout.
	ev.nativeViewportColumns = 81
	for step := 0; step < 200; step++ {
		b.readBytes = 0
		ready := ev.ensureEngineWidth()
		if b.readBytes > editorMappingWorkBytes+4096 {
			t.Fatalf("slice %d read %d bytes", step, b.readBytes)
		}
		if ready {
			if ev.ScrollTopRow != 7200000/81 {
				t.Fatalf("lost anchor: row %d", ev.ScrollTopRow)
			}
			return
		}
	}
	t.Fatal("reflow did not finish")
}

func TestEditorReflowRetainsAnchorUntilAsyncBytesArrive(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	reader := &documentRangeReader{data: bytes.Repeat([]byte("a"), 128*1024), started: make(chan struct{}), release: make(chan struct{})}
	buf := NewAsyncBuffer(context.Background(), reader)
	buf.seedPrefix(reader.data[:32768])
	ev := NewEditorView(piecetable.NewWithBuffer(buf), nil, "")
	ev.asyncBuf = buf
	defer ev.Close()
	ev.WordWrap, ev.nativeViewportColumns = true, 80
	ev.ensureEngineWidth()
	ev.ScrollTopRow = 408 // byte 32640 is ready; its new fragment needs chunk 2.
	ev.engine.GetLogLineAtVisualRow(408)
	ev.nativeViewportColumns = 200
	ready := ev.ensureEngineWidth()
	close(reader.release)
	if ready || ev.ScrollTopRow != 408 {
		t.Fatal("reflow installed an incomplete anchor")
	}
	if _, err := buf.ReadContext(context.Background(), 32768, 32768); err != nil {
		t.Fatal(err)
	}
	if !ev.ensureEngineWidth() || ev.ScrollTopRow != 163 {
		t.Fatalf("ready reflow lost byte anchor: row %d", ev.ScrollTopRow)
	}
}

func TestViewerUnwrappedSeekYieldsAndResumes(t *testing.T) {
	viewer, file := constructionTestViewer(t, bytes.Repeat([]byte("a"), 8*1024*1024), 80, 24, false)
	file.profile = vfs.ReadAccessDirectLocal
	for step := 0; step < 150; step++ {
		before := len(file.reads)
		offset, ready := viewer.semanticResolveTextWindowOffset(7 * 1024 * 1024)
		// Each slice scans 64 KiB and can miss at most twice in the 256 KiB
		// source cache. The old unlimited adapter read 27.8 MB in one call.
		if len(file.reads)-before > 2 {
			t.Fatal("seek exceeded source-read budget")
		}
		if step == 0 && ready {
			t.Fatal("seek did not yield")
		}
		if ready {
			if offset != 0 {
				t.Fatalf("wrong line start: %d", offset)
			}
			return
		}
	}
	t.Fatal("seek did not resume to BOF")
}
