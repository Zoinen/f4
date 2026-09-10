package plughost

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type extUiErrorWriter struct{}

func (extUiErrorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestExtUiMessageValidationAndIntegerConversions(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 0)
	if _, err := extUiReadMessage(bytes.NewReader(header[:])); err == nil {
		t.Fatal("zero-length message was accepted")
	}

	binary.BigEndian.PutUint32(header[:], extUiMaxMessageSize+1)
	if _, err := extUiReadMessage(bytes.NewReader(header[:])); err == nil {
		t.Fatal("oversized message was accepted")
	}

	binary.BigEndian.PutUint32(header[:], 1)
	if _, err := extUiReadMessage(bytes.NewReader(append(header[:], 0xff))); err == nil {
		t.Fatal("invalid msgpack payload was accepted")
	}

	binary.BigEndian.PutUint32(header[:], 2)
	if _, err := extUiReadMessage(bytes.NewReader(append(header[:], 0xc0))); err == nil {
		t.Fatal("truncated payload was accepted")
	}
	if err := extUiSendMessage(extUiErrorWriter{}, map[string]any{"type": "test"}); err == nil {
		t.Fatal("writer failure was swallowed")
	}

	tests := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{"int", int(-3), -3, true},
		{"int8", int8(4), 4, true},
		{"int16", int16(5), 5, true},
		{"int32", int32(6), 6, true},
		{"int64", int64(7), 7, true},
		{"uint", uint(8), 8, true},
		{"uint8", uint8(9), 9, true},
		{"uint16", uint16(10), 10, true},
		{"uint32", uint32(11), 11, true},
		{"uint64", uint64(12), 12, true},
		{"string", "12", 0, false},
		{"float", float64(12), 0, false},
		{"uint64-overflow", ^uint64(0), 0, false},
	}
	maxInt64 := int64(^uint64(0) >> 1)
	got, ok := extUiAnyIntOK(maxInt64)
	if (strconv.IntSize == 64 && (!ok || int64(got) != maxInt64)) || (strconv.IntSize == 32 && ok) {
		t.Fatalf("int64 max conversion = (%d, %t) on %d-bit platform", got, ok, strconv.IntSize)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := extUiAnyIntOK(test.value)
			if got != test.want || ok != test.ok {
				t.Fatalf("extUiAnyInt(%v) = (%d, %t), want (%d, %t)", test.value, got, ok, test.want, test.ok)
			}
		})
	}

	msg := map[string]any{"value": int64(42)}
	if ExtUiInt(msg, "value") != 42 || ExtUiInt(msg, "missing") != 0 {
		t.Fatal("extUiInt did not handle present and missing values")
	}
	if got, ok := extUiDimension(map[string]any{"v": 0}, "v"); ok || got != 0 {
		t.Fatalf("zero dimension = (%d, %t)", got, ok)
	}
	if got, ok := extUiDimension(map[string]any{"v": extUiMaxDimension + 1}, "v"); ok || got != extUiMaxDimension+1 {
		t.Fatalf("oversized dimension = (%d, %t)", got, ok)
	}
	if got, ok := extUiInt16(map[string]any{"v": 40000}, "v"); ok || got != 0 {
		t.Fatalf("out-of-range int16 = (%d, %t)", got, ok)
	}
	if got, ok := extUiUint16(map[string]any{"v": -1}, "v"); ok || got != 0 {
		t.Fatalf("negative uint16 = (%d, %t)", got, ok)
	}
	if got, ok := extUiUint32(map[string]any{"v": int64(-1)}, "v"); ok || got != 0 {
		t.Fatalf("negative uint32 = (%d, %t)", got, ok)
	}
	if got, ok := extUiRune(map[string]any{"v": 0x110000}, "v"); ok || got != 0 {
		t.Fatalf("invalid rune = (%d, %t)", got, ok)
	}
}

func TestExtUiRendererQueuesAndFlushesUpdates(t *testing.T) {
	var out bytes.Buffer
	renderer := NewExtUiRenderer(nil, &extUiMessageSender{w: &out})
	var palette [256]uint32
	palette[0] = 0x010203
	renderer.SetPalette(&palette)
	palette[0] = 0xffffff
	renderer.SetCursor(3, 2, true, vtui.CursorShapeBlock)
	renderer.Render([]vtui.CharInfo{{Char: 'A', Attributes: 7}}, nil, 1, 1, true)
	renderer.SetSemanticScene(map[string]any{"type": "scene", "schema": extui.Schema, "shell": map[string]any{"title": "main"}})
	renderer.SetWindowTitle("title")

	var messages []map[string]any
	for out.Len() > 0 {
		msg, err := extUiReadMessage(&out)
		if err != nil {
			t.Fatalf("read immediate/queued message: %v", err)
		}
		messages = append(messages, msg)
	}
	if len(messages) != 1 || extUiString(messages[0], "type") != "title" {
		t.Fatalf("before Flush got %v, want title message", messages)
	}

	renderer.Flush()
	for out.Len() > 0 {
		msg, err := extUiReadMessage(&out)
		if err != nil {
			t.Fatalf("read flushed message: %v", err)
		}
		messages = append(messages, msg)
	}
	if len(messages) != 6 {
		t.Fatalf("got %d messages, want title plus palette/chrome/shell/frame/cursor: %v", len(messages), messages)
	}
	for i, want := range []string{"title", "palette", "chrome_snapshot", "shell_snapshot", "frame", "cursor"} {
		if got := extUiString(messages[i], "type"); got != want {
			t.Errorf("message %d type = %q, want %q", i, got, want)
		}
	}
	if messages[1]["colors"] == nil || !ExtUiBool(messages[4], "full") {
		t.Fatal("palette or full frame update was not emitted")
	}
	if renderer.palette[0] != 0x010203 {
		t.Fatal("SetPalette retained an alias to its input")
	}

	renderer.Flush()
	if out.Len() != 0 {
		t.Fatal("Flush emitted duplicate messages")
	}

	var diff bytes.Buffer
	diffRenderer := NewExtUiRenderer(nil, &extUiMessageSender{w: &diff})
	cell := vtui.CharInfo{Char: 'A', Attributes: 1}
	diffRenderer.Flush()
	diff.Reset()
	diffRenderer.Render([]vtui.CharInfo{cell}, []vtui.CharInfo{cell}, 1, 1, false)
	diffRenderer.Flush()
	if diff.Len() != 0 {
		t.Fatal("unchanged frame was emitted")
	}
	diffRenderer.Render([]vtui.CharInfo{{Char: 'B', Attributes: 1}}, []vtui.CharInfo{cell}, 1, 1, false)
	diffRenderer.Flush()
	msg, err := extUiReadMessage(&diff)
	if err != nil || extUiString(msg, "type") != "frame" || ExtUiBool(msg, "full") {
		t.Fatalf("changed cell frame = %v, err %v", msg, err)
	}
}

func TestExtUiRendererClosesAfterSendFailure(t *testing.T) {
	renderer := NewExtUiRenderer(nil, &extUiMessageSender{w: extUiErrorWriter{}})
	renderer.SetCursor(1, 1, true, vtui.CursorShapeUnderline)
	renderer.Flush()
	if !renderer.closed {
		t.Fatal("renderer did not close after send failure")
	}
	renderer.SetCursor(2, 2, true, vtui.CursorShapeBlock)
	renderer.Flush()
}

func TestExtUiHostHandleMessageEvents(t *testing.T) {
	reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 16)}
	host := &ExtUiHost{reader: reader, cols: 10, rows: 5}
	host.handleMessage(map[string]any{"type": "resize", "cols": 12, "rows": 7})
	host.handleMessage(map[string]any{"type": "key", "vk": 65, "char": 'A', "mods": 3, "down": true})
	host.handleMessage(map[string]any{"type": "text", "mods": 4, "text": "xy"})
	host.handleMessage(map[string]any{"type": "mouse", "x": -2, "y": 3, "button": 1, "flags": 2, "mods": 4, "down": true})
	host.handleMessage(map[string]any{"type": "wheel", "x": 1, "y": -1, "dir": -2, "mods": 8})
	host.handleMessage(map[string]any{"type": "paste", "text": "z"})
	host.handleMessage(map[string]any{"type": "resize", "cols": 0, "rows": 7})
	host.handleMessage(map[string]any{"type": "unknown"})

	if host.cols != 12 || host.rows != 7 {
		t.Fatalf("invalid resize changed size to %dx%d", host.cols, host.rows)
	}
	var events []*vtinput.InputEvent
	for len(reader.EventChan) > 0 {
		events = append(events, <-reader.EventChan)
	}
	if len(events) != 9 {
		t.Fatalf("got %d events, want resize, key, 2 text, mouse, wheel, paste start, char, end", len(events))
	}
	if events[0].Type != vtinput.ResizeEventType || events[0].InputSource != "extui" {
		t.Errorf("resize event = %+v", events[0])
	}
	if events[1].Type != vtinput.KeyEventType || events[1].VirtualKeyCode != 65 || events[1].Char != 'A' || !events[1].KeyDown {
		t.Errorf("key event = %+v", events[1])
	}
	if events[2].Char != 'x' || events[3].Char != 'y' || events[2].InputSource != "extui_text" {
		t.Errorf("text events = %+v, %+v", events[2], events[3])
	}
	if events[4].MouseX != -2 || events[4].MouseY != 3 || events[4].ButtonState != 1 {
		t.Errorf("mouse event = %+v", events[4])
	}
	if events[5].WheelDirection != -2 {
		t.Errorf("wheel event = %+v", events[5])
	}
	if events[6].Type != vtinput.PasteEventType || !events[6].PasteStart || events[8].PasteStart {
		t.Errorf("paste events = %+v, %+v, %+v", events[6], events[7], events[8])
	}
}

func TestFindExtUiPathValidationAndOverride(t *testing.T) {
	if extUiFileExists("") {
		t.Fatal("empty executable path exists")
	}
	dir := t.TempDir()
	if extUiFileExists(dir) {
		t.Fatal("directory was accepted as executable")
	}
	file := filepath.Join(dir, "host")
	if err := os.WriteFile(file, []byte("stub"), 0600); err != nil {
		t.Fatal(err)
	}
	if !extUiFileExists(file) {
		t.Fatal("regular executable candidate was rejected")
	}
	t.Setenv("F4_EXT_UI_PATH", file)
	got, err := findExtUiPath("qt")
	if err != nil || got != file {
		t.Fatalf("findExtUiPath override = %q, %v", got, err)
	}

	t.Setenv("F4_EXT_UI_PATH", "")
	for _, backend := range []string{"ext:", "ext:../escape", "ext:a/b", "ext:a\\b", "ext:a..b"} {
		if _, err := findExtUiPath(backend); err == nil {
			t.Errorf("findExtUiPath(%q) accepted unsafe name", backend)
		}
	}
}

func TestRunExternalUIReportsMissingExecutable(t *testing.T) {
	if err := RunExternalUI(1, 1, filepath.Join(t.TempDir(), "missing-host"), nil); err == nil {
		t.Fatal("missing external UI executable did not return an error")
	}
	if err := RunExternalUIWithMapping("ext:"); err == nil {
		t.Fatal("invalid external UI mapping did not return an error")
	}
}
