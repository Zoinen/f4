package vtui

import (
	"bytes"
	"testing"
)

func TestQtRendererFlushFrame(t *testing.T) {
	var wire bytes.Buffer

	renderer := NewQtRendererWithSender(nil, &qtMessageSender{w: &wire})
	buf := []CharInfo{{Char: 'X', Attributes: SetRGBBoth(0, 0xFFFFFF, 0x000000)}}
	shadow := []CharInfo{{}}
	renderer.Render(buf, shadow, 1, 1, false)
	renderer.Flush()

	msg, err := qtReadMessage(&wire)
	if err != nil {
		t.Fatalf("read frame failed: %v", err)
	}
	if qtString(msg, "type") != "frame" {
		t.Fatalf("message type = %q, want frame", qtString(msg, "type"))
	}
	if qtInt(msg, "width") != 1 || qtInt(msg, "height") != 1 {
		t.Fatalf("unexpected frame size: %dx%d", qtInt(msg, "width"), qtInt(msg, "height"))
	}
}

func TestQtRendererSuppressesUnchangedFrame(t *testing.T) {
	var wire bytes.Buffer

	renderer := NewQtRendererWithSender(nil, &qtMessageSender{w: &wire})
	renderer.cursorDirty = false
	buf := []CharInfo{{Char: 'X', Attributes: 1}}
	shadow := []CharInfo{{Char: 'X', Attributes: 1}}
	renderer.Render(buf, shadow, 1, 1, false)
	renderer.Flush()

	if wire.Len() != 0 {
		t.Fatalf("expected no frame for unchanged buffer, got %d bytes", wire.Len())
	}
}
