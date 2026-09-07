package main

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestScanEditorWrapSafety(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		line   int
		unsafe bool
	}{
		{name: "short line", data: []byte("short\ntext"), unsafe: false},
		{name: "long first line", data: []byte(strings.Repeat("x", maxWordWrapLineBytes+1)), unsafe: true},
		{name: "long later line", data: []byte("first\n" + strings.Repeat("x", maxWordWrapLineBytes+1)), unsafe: true},
		{name: "binary marker", data: []byte("text\x00after"), unsafe: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, unsafe := scanEditorWrapSafety(test.data, test.line)
			if unsafe != test.unsafe {
				t.Fatalf("unsafe = %v, want %v", unsafe, test.unsafe)
			}
		})
	}

	// A line may cross the 256 KiB indexing chunks or any smaller VFS window.
	lineLen, unsafe := scanEditorWrapSafety([]byte(strings.Repeat("x", maxWordWrapLineBytes)), 0)
	if unsafe {
		t.Fatal("exactly-threshold prefix must still be safe")
	}
	_, unsafe = scanEditorWrapSafety([]byte("x\n"), lineLen)
	if !unsafe {
		t.Fatal("line crossing a chunk boundary must be detected")
	}

	if !editorWrapIntervalUnsafe(0, maxWordWrapLineBytes+2) {
		t.Fatal("remote line offset must detect a line longer than the limit")
	}
	if editorWrapIntervalUnsafe(0, maxWordWrapLineBytes+1) {
		t.Fatal("remote line offset must accept a line exactly at the limit")
	}
}

func TestEditorView_UnsafeWordWrapCannotBeReenabled(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	ev := NewEditorView(piecetable.New([]byte("text")), nil, "")
	defer ev.Close()
	ev.WordWrap = true
	ev.ScrollLeft = 7

	ev.disableUnsafeWordWrap()
	if ev.WordWrap {
		t.Fatal("unsafe content must turn word wrap off")
	}
	if ev.ScrollLeft != 0 {
		t.Fatalf("horizontal scroll = %d, want 0 after disabling wrap", ev.ScrollLeft)
	}

	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})
	if ev.WordWrap {
		t.Fatal("F3 must not re-enable word wrap for unsafe content")
	}
}

func TestEditorView_StartIndexingFindsUnsafeLaterLine(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content := "first\n" + strings.Repeat("x", maxWordWrapLineBytes+1)
	ev := NewEditorView(piecetable.New([]byte(content)), nil, "")
	defer ev.Close()
	ev.WordWrap = true

	ev.StartIndexing()

	timeout := time.After(2 * time.Second)
	for !ev.wordWrapSuppressed {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("unsafe later line was not detected")
		}
	}
	if ev.WordWrap {
		t.Fatal("later unsafe line must disable word wrap")
	}
}
