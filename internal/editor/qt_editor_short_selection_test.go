package editor

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/vtinput"
	"os"
	"testing"
)

func TestShortEditorShiftSelectionKeepsViewport(t *testing.T) {
	data, err := os.ReadFile("../../run-f4-gogpu.sh")
	if err != nil {
		t.Fatal(err)
	}
	ev := projectionTestEditor(t, string(data))
	// The file-open path detects CPU bitness even for ordinary text files.
	ev.DisasmMode = viewer.DetectX86Mode(data)
	ev.NativeViewportColumns, ev.NativeViewportRows = 150, 45
	ev.NativeViewportRevision = 1
	ev.SetFocus(true)
	ev.SemanticNode(nil)
	for _, key := range []int{vtinput.VK_DOWN, vtinput.VK_UP} {
		for i := 0; i < 30; i++ {
			ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: uint16(key), ControlKeyState: vtinput.ShiftPressed})
			selected := ev.SemanticNode(nil)
			if !ev.SelActive || !semantic.Bool(selected["selection"]) {
				t.Fatalf("step %d key %d: text selection hidden with detected disassembler mode %d", i, key, ev.DisasmMode)
			}
			if ev.ScrollTopRow != 0 || semantic.Int(selected["viewportStart"]) != 0 {
				t.Fatalf("step %d key %d scrolled short document to %d", i, key, ev.ScrollTopRow)
			}
		}
	}
	for _, mode := range []string{"hex", "decode"} {
		ev.HexMode, ev.DecodeMode = mode == "hex", mode == "decode"
		if ev.semanticCursorState(ev.viewportWidth()).selection {
			t.Fatalf("text overlay enabled in %s mode", mode)
		}
	}
}
