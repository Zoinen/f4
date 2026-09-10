package editor

import (
	"bytes"
	"context"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
	"testing"
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
	ev.WordWrap, ev.NativeViewportColumns = true, 80
	ev.EnsureEngineWidth()
	ev.ScrollTopRow = 90000
	ev.Engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	ev.NativeViewportColumns = 79
	b.readBytes = 0
	if ev.EnsureEngineWidth() {
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
	ev.NativeViewportColumns = 81
	for step := 0; step < 200; step++ {
		b.readBytes = 0
		ready := ev.EnsureEngineWidth()
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
	buf.SeedPrefix(reader.data[:32768])
	ev := NewEditorView(piecetable.NewWithBuffer(buf), nil, "")
	ev.AsyncBuf = buf
	defer ev.Close()
	ev.WordWrap, ev.NativeViewportColumns = true, 80
	ev.EnsureEngineWidth()
	ev.ScrollTopRow = 408 // byte 32640 is ready; its new fragment needs chunk 2.
	ev.Engine.GetLogLineAtVisualRow(408)
	ev.NativeViewportColumns = 200
	ready := ev.EnsureEngineWidth()
	close(reader.release)
	if ready || ev.ScrollTopRow != 408 {
		t.Fatal("reflow installed an incomplete anchor")
	}
	if _, err := buf.ReadContext(context.Background(), 32768, 32768); err != nil {
		t.Fatal(err)
	}
	if !ev.EnsureEngineWidth() || ev.ScrollTopRow != 163 {
		t.Fatalf("ready reflow lost byte anchor: row %d", ev.ScrollTopRow)
	}
}
