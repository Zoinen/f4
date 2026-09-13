package vtui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/unxed/vtinput"
)

func TestQueuedPasteIsOneDispatchBetweenPhysicalKeys(t *testing.T) {
	fm := &frameManager{}
	screen := NewSilentScreenBuf()
	screen.AllocBuf(80, 24)
	fm.Init(screen)
	defer fm.Shutdown()
	frame := newMockFrame(0, 0, 79, 23, false)
	var delivered strings.Builder
	filtered := 0
	fm.EventFilter = func(*vtinput.InputEvent) bool { filtered++; return false }
	frame.onProcessKey = func(event *vtinput.InputEvent) bool {
		if event.Type == vtinput.KeyEventType {
			delivered.WriteRune(event.Char)
		}
		return true
	}
	fm.Push(frame)
	fm.EventChan = make(chan *vtinput.InputEvent, 3)
	fm.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '['}
	text := strings.Repeat("▶ Moving DSC00100.MP4 ━━ 100%\r\n", 200)
	if !fm.QueuePaste(fm.EventChan, text) {
		t.Fatal("paste not queued")
	}
	fm.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ']'}
	if len(fm.EventChan) != 3 {
		t.Fatal("paste expanded in input queue")
	}
	getSize := func() (int, int, error) { return 80, 24, nil }
	for len(fm.EventChan) > 0 {
		fm.stepWithSize(0, getSize)
	}
	if delivered.String() != "["+text+"]" || filtered != 3 {
		t.Fatalf("FIFO/dispatch mismatch: text length=%d, global dispatches=%d", delivered.Len(), filtered)
	}
	fm.pendingPastes.Range(func(_, _ any) bool { t.Error("consumed payload retained"); return false })
}

func TestQueuedPasteBlockedQueueReleasesPayload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fm := &frameManager{}
		if fm.QueuePaste(make(chan *vtinput.InputEvent), "clipboard") {
			t.Fatal("blocked paste accepted")
		}
		fm.pendingPastes.Range(func(_, _ any) bool { t.Error("failed payload retained"); return false })
	})
}

func TestPasteThroughAutocompleteCommitsOnce(t *testing.T) {
	edit := NewEdit(0, 0, 79, "")
	edit.History = []string{"example history"}
	menu := NewAutoCompleteMenu(edit)
	changes := 0
	edit.OnTextChange = func(string) { changes++ }
	dispatchFramePaste(menu, strings.Repeat("example ", 100))
	if changes != 1 || edit.GetText() != strings.Repeat("example ", 100) {
		t.Fatalf("autocomplete paste produced %d edits, text length=%d", changes, len(edit.GetText()))
	}
}

func BenchmarkQueuedClipboardPaste(b *testing.B) {
	fm := &frameManager{}
	screen := NewSilentScreenBuf()
	screen.AllocBuf(80, 24)
	fm.Init(screen)
	defer fm.Shutdown()
	edit := NewEdit(0, 0, 79, "")
	frame := newMockFrame(0, 0, 79, 23, false)
	frame.onProcessKey = edit.ProcessKey
	fm.Push(frame)
	queue := make(chan *vtinput.InputEvent, 1)
	text := "s), 4.0 GB total\n ▶ simulated Dummy destination\n Moving DSC00100.MP4 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100/100 100% • 500.0 MB/s • 0:00:08 ETA 0:00:00\nMoved 100 file(s). (simulated)\n 80 ARW file(s) to convert (10 parallel workers)"
	getSize := func() (int, int, error) { return 80, 24, nil }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edit.SetText("")
		fm.QueuePaste(queue, text)
		fm.consumeEvent(<-queue, false, getSize)
	}
}

func TestFrameManagerPasteRendersOnceAndPreservesFilterFIFO(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fm := &frameManager{}
		screen := NewSilentScreenBuf()
		screen.AllocBuf(80, 24)
		renderer := &redrawCoalescingTestRenderer{}
		screen.Renderer = renderer
		fm.Init(screen)
		defer fm.Shutdown()
		frame := newMockFrame(0, 0, 79, 23, false)
		var filtered, delivered strings.Builder
		fm.EventFilter = func(event *vtinput.InputEvent) bool {
			if event.Type == vtinput.KeyEventType {
				filtered.WriteRune(event.Char)
			}
			return false
		}
		frame.onProcessKey = func(event *vtinput.InputEvent) bool {
			if event.Type == vtinput.KeyEventType {
				delivered.WriteRune(event.Char)
			}
			return true
		}
		fm.Push(frame)
		getSize := func() (int, int, error) { return 80, 24, nil }
		fm.stepWithSize(0, getSize)
		baseline := renderer.scenes
		text := strings.Repeat("αβ paste\r\n", 600)
		fm.EventChan = make(chan *vtinput.InputEvent, len([]rune(text))+2)
		fm.EventChan <- &vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true}
		for _, char := range text {
			fm.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char}
		}
		fm.EventChan <- &vtinput.InputEvent{Type: vtinput.PasteEventType}
		for len(fm.EventChan) > 0 {
			fm.stepWithSize(0, getSize)
		}
		fm.stepWithSize(0, getSize)
		if filtered.String() != text || delivered.String() != text {
			t.Fatal("paste batching changed FIFO data or bypassed EventFilter")
		}
		if got := renderer.scenes - baseline; got != 1 {
			t.Fatalf("paste generated %d semantic renders, want one", got)
		}
		fm.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '!'}
		fm.stepWithSize(0, getSize)
		fm.stepWithSize(0, getSize)
		if delivered.String() != text+"!" || renderer.scenes != baseline+2 {
			t.Fatal("ordinary input after paste did not render normally in FIFO order")
		}
	})
}

func TestFrameManagerUnterminatedPasteDoesNotFreezeRendering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fm := &frameManager{}
		screen := NewSilentScreenBuf()
		screen.AllocBuf(80, 24)
		renderer := &redrawCoalescingTestRenderer{}
		screen.Renderer = renderer
		fm.Init(screen)
		defer fm.Shutdown()
		fm.Push(newMockFrame(0, 0, 79, 23, false))
		getSize := func() (int, int, error) { return 80, 24, nil }
		fm.stepWithSize(0, getSize)
		baseline := renderer.scenes
		fm.EventChan = make(chan *vtinput.InputEvent, 1)
		fm.EventChan <- &vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true}
		fm.stepWithSize(0, getSize)
		fm.stepWithSize(0, getSize)
		if renderer.scenes != baseline {
			t.Fatal("paste start was rendered before its payload")
		}
		started := time.Now()
		fm.stepWithSize(time.Second, getSize)
		if time.Since(started) > 250*time.Millisecond {
			t.Fatal("incomplete paste blocked the input loop for more than 250ms")
		}
		fm.stepWithSize(0, getSize)
		if renderer.scenes != baseline+1 {
			t.Fatal("missing paste end left rendering frozen")
		}
	})
}
