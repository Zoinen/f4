package editor

import (
	fmt "fmt"
	config "github.com/unxed/f4/internal/config"
	piecetable "github.com/unxed/f4/internal/piecetable"
	semantic "github.com/unxed/f4/internal/semantic"
	vtinput "github.com/unxed/vtinput"
	vtui "github.com/unxed/vtui"
	strings "strings"
	testing "testing"
)

func TestSemanticWindowProtocol_EditorKeepsThreeScreensAndStableOverlap(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for row := 0; row < 200; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "overlap.txt")
	editor.SetPosition(0, 0, 39, 8)
	editor.ScrollTopRow = 80

	first := editor.SemanticNode(nil)
	viewport := semantic.Int(first["viewportRows"])
	buffer := semantic.SemanticWindowBufferRows(viewport)
	if viewport != 8 || buffer != viewport {
		t.Fatalf("viewport=%d buffer=%d, want one full viewport of overscan", viewport, buffer)
	}
	if got, want := len(semantic.AppMapSlice(first["windowRows"])), 3*viewport; got != want {
		t.Fatalf("initial bounded rows=%d, want %d", got, want)
	}

	// With three screens buffered, QML requests the next window after half a
	// screen of travel. Even if the response arrives only at the old ListView's
	// lower bound, that live top-row anchor is still present in the new window.
	requestTop := editor.ScrollTopRow + viewport/2
	oldBoundaryTop := semantic.AppInt64(first["windowStart"]) + int64(2*viewport)
	if !editor.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(editor), "action": "editor.scroll",
		"visualRow": requestTop, "generation": uint64(1),
	}) {
		t.Fatal("editor window request was not handled")
	}
	second := editor.SemanticNode(nil)
	if overlap := assertSemanticOverlapStable(t, first, second, "rows"); overlap < 2*viewport {
		t.Fatalf("editor overlap=%d rows, want at least %d", overlap, 2*viewport)
	}
	if _, ok := semanticRowsByExtent(t, second, "rows")[oldBoundaryTop]; !ok {
		t.Fatalf("new window lost live old-boundary anchor row %d", oldBoundaryTop)
	}
}

func TestSemanticWindowProtocol_NativeViewportScrollsEditorAtFirstHiddenRow(t *testing.T) {
	vtui.SetDefaultPalette()
	var content strings.Builder
	for row := 0; row < 40; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "viewport.txt")
	defer editor.Close()
	editor.SetPosition(0, 0, 39, 10)
	editor.SetVisible(true)
	target := vtui.SemanticID(editor)

	if !editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "document.viewport", "rows": 8,
	}) {
		t.Fatal("native editor viewport was not handled")
	}
	if got := semantic.Int(editor.SemanticNode(nil)["viewportRows"]); got != 8 {
		t.Fatalf("semantic viewport rows=%d, want 8", got)
	}

	editor.ScrollTopRow = 0
	editor.CursorLine = 7
	editor.CursorPos = 0
	editor.EnsureCursorVisible()
	if editor.ScrollTopRow != 0 {
		t.Fatalf("last visible row scrolled early to %d", editor.ScrollTopRow)
	}
	editor.CursorLine = 8
	editor.EnsureCursorVisible()
	if editor.ScrollTopRow != 1 {
		t.Fatalf("first hidden row scrolled to %d, want 1", editor.ScrollTopRow)
	}
	cursor := editor.semanticCursorState(editor.semanticSurfaceWidth())
	if !cursor.visible || cursor.visualRow != 7 {
		t.Fatalf("cursor after immediate scroll=%#v, want visible row 7", cursor)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "document.viewport", "rows": 0,
	})
	if got := semantic.Int(editor.SemanticNode(nil)["viewportRows"]); got != 10 {
		t.Fatalf("cleared native viewport rows=%d, want terminal height 10", got)
	}
}

func TestSemanticEditorMouseUsesCanonicalEditorPointerHandling(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	editor := NewEditorView(piecetable.New([]byte("zero\nalpha beta\ngamma\n")), nil, "mouse.txt")
	defer editor.Close()
	editor.SetPosition(3, 4, 42, 12)
	target := vtui.SemanticID(editor)

	if !editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "press",
		"button": "left", "column": 2, "row": 1,
	}) {
		t.Fatal("semantic editor mouse press was not handled")
	}
	if editor.CursorLine != 1 || editor.CursorPos != 2 || !editor.SelActive {
		t.Fatalf("click cursor=(%d,%d) selection=%v, want (1,2) with anchor",
			editor.CursorLine, editor.CursorPos, editor.SelActive)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "move",
		"button": "left", "column": 3, "row": 2, "moved": true,
	})
	start, end := editor.GetSelectionRange()
	if editor.CursorLine != 2 || editor.CursorPos != 3 || start != 7 || end != 19 {
		t.Fatalf("drag cursor=(%d,%d) range=[%d:%d], want (2,3) [7:19]",
			editor.CursorLine, editor.CursorPos, start, end)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "press",
		"button": "left", "column": 8, "row": 1, "doubleClick": true,
	})
	start, end = editor.GetSelectionRange()
	if start != 11 || end != 15 {
		t.Fatalf("double-click range=[%d:%d], want beta [11:15]", start, end)
	}

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.mouse", "phase": "press",
		"button": "right", "column": 1, "row": 0,
	})
	if !editor.RectSelActive || editor.SelActive {
		t.Fatalf("right click rectangular=%v stream=%v, want rectangular only",
			editor.RectSelActive, editor.SelActive)
	}
}

func TestSemanticWindowProtocol_StaleGenerationsCannotMoveEditor(t *testing.T) {
	var content strings.Builder
	for row := 0; row < 100; row++ {
		fmt.Fprintf(&content, "row-%03d\n", row)
	}
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil, "generation.txt")
	editor.SetPosition(0, 0, 39, 8)
	target := vtui.SemanticID(editor)

	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.scroll", "visualRow": 40,
		"generation": uint64(41),
	})
	if editor.ScrollTopRow != 0 || editor.semanticWindowGeneration != 0 || !editor.semanticPendingScroll {
		t.Fatalf("unpublished request changed top=%d generation=%d pending=%v", editor.ScrollTopRow,
			editor.semanticWindowGeneration, editor.semanticPendingScroll)
	}
	editor.SemanticNode(nil)
	if editor.ScrollTopRow != 40 || editor.semanticWindowGeneration != 41 {
		t.Fatalf("fresh request produced top=%d generation=%d", editor.ScrollTopRow,
			editor.semanticWindowGeneration)
	}
	for _, generation := range []uint64{40, 41} {
		editor.HandleSemanticAction(map[string]any{
			"target": target, "action": "editor.scroll", "visualRow": 3,
			"generation": generation,
		})
		if editor.ScrollTopRow != 40 || editor.semanticWindowGeneration != 41 {
			t.Fatalf("stale generation %d moved editor to top=%d generation=%d",
				generation, editor.ScrollTopRow, editor.semanticWindowGeneration)
		}
	}
	editor.HandleSemanticAction(map[string]any{
		"target": target, "action": "editor.scroll", "visualRow": 1 << 20,
		"generation": uint64(42),
	})
	if editor.semanticWindowGeneration != 41 {
		t.Fatal("clamped request acknowledged before projection")
	}
	editor.SemanticNode(nil)
	if editor.semanticWindowGeneration != 42 {
		t.Fatalf("clamped no-op did not ACK exact generation 42: %d",
			editor.semanticWindowGeneration)
	}
}

func TestSemanticWindowProtocol_EditorEdgeNavigationFencesPendingScroll(t *testing.T) {
	var content strings.Builder
	for row := 0; row < 200; row++ {
		fmt.Fprintf(&content, "row-%03d alpha beta gamma\n", row)
	}
	for _, tc := range []struct {
		name  string
		key   uint16
		shift bool
		pos   int
	}{
		{name: "home", key: vtinput.VK_HOME, pos: 0},
		{name: "end", key: vtinput.VK_END, pos: len("row-045 alpha beta gamma")},
		{name: "shift-home", key: vtinput.VK_HOME, shift: true, pos: 0},
		{name: "shift-end", key: vtinput.VK_END, shift: true, pos: len("row-045 alpha beta gamma")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := NewEditorView(piecetable.New([]byte(content.String())), nil,
				"edge-navigation.txt")
			defer editor.Close()
			editor.Highlighter = nil
			editor.SetPosition(0, 0, 39, 8)
			editor.SetVisible(true)
			editor.ScrollTopRow = 40
			editor.CursorLine, editor.CursorPos = 45, 8
			target := vtui.SemanticID(editor)
			initial := editor.SemanticNode(nil)
			if !semantic.Bool(initial["cursorVisible"]) {
				t.Fatal("initial caret is not visible")
			}

			const pendingGeneration = uint64(11)
			if !editor.HandleSemanticAction(map[string]any{
				"target": target, "action": "editor.scroll", "visualRow": 120,
				"generation": pendingGeneration,
			}) || !editor.semanticPendingScroll {
				t.Fatal("test scroll destination was not pending")
			}
			state := vtinput.ShiftPressed
			if !tc.shift {
				state = 0
			}
			if !editor.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tc.key, ControlKeyState: state,
			}) {
				t.Fatal("edge key was not handled")
			}
			if editor.semanticPendingScroll ||
				editor.semanticWindowGeneration != pendingGeneration+1 ||
				editor.semanticWindowRequestGeneration != pendingGeneration+1 {
				t.Fatalf("edge fence state: pending=%v window=%d request=%d",
					editor.semanticPendingScroll, editor.semanticWindowGeneration,
					editor.semanticWindowRequestGeneration)
			}
			if editor.CursorLine != 45 || editor.CursorPos != tc.pos ||
				editor.SelActive != tc.shift {
				t.Fatalf("edge navigation result cursor=%d:%d selection=%v",
					editor.CursorLine, editor.CursorPos, editor.SelActive)
			}

			ack := editor.SemanticNode(nil)
			if semantic.Int64(ack["windowGeneration"]) != int64(pendingGeneration+1) ||
				!semantic.Bool(ack["cursorVisible"]) {
				t.Fatalf("successor ACK did not keep caret visible: generation=%v visible=%v",
					ack["windowGeneration"], ack["cursorVisible"])
			}
			if tc.shift {
				anchorRow, anchorColumn := editor.Engine.LogicalToVisual(editor.SelAnchorOffset)
				if !semantic.Bool(ack["selection"]) ||
					semantic.Int64(ack["selectionAnchorRow"]) != int64(anchorRow) ||
					semantic.Int(ack["selectionAnchorColumn"]) != anchorColumn ||
					semantic.String(ack["selectionBackground"]) !=
						semantic.SemanticAttrColor(vtui.Palette[vtui.ColDialogEditSelected], false) {
					t.Fatal("Shift edge ACK lost scalar selection geometry/style")
				}
			}

			// A delayed request/result in the generation canceled before the key
			// must not move either the viewport or selection endpoint.
			beforeTop, beforeLine, beforePos := editor.ScrollTopRow,
				editor.CursorLine, editor.CursorPos
			editor.HandleSemanticAction(map[string]any{
				"target": target, "action": "editor.scroll", "visualRow": 150,
				"generation": pendingGeneration,
			})
			late := editor.SemanticNode(nil)
			if editor.ScrollTopRow != beforeTop || editor.CursorLine != beforeLine ||
				editor.CursorPos != beforePos || editor.semanticPendingScroll ||
				semantic.Int64(late["windowGeneration"]) != int64(pendingGeneration+1) ||
				editor.SelActive != tc.shift {
				t.Fatalf("late scroll overrode edge result: top=%d cursor=%d:%d pending=%v generation=%v selection=%v",
					editor.ScrollTopRow, editor.CursorLine, editor.CursorPos,
					editor.semanticPendingScroll, late["windowGeneration"], editor.SelActive)
			}
		})
	}
}

func TestSemanticWindowProtocol_EditorArrowAndWheelNavigationFencePendingScroll(t *testing.T) {
	var content strings.Builder
	for row := 0; row < 200; row++ {
		fmt.Fprintf(&content, "row-%03d alpha beta gamma\n", row)
	}

	for _, tc := range []struct {
		name string
		key  uint16
		want int
	}{
		{name: "up", key: vtinput.VK_UP, want: 44},
		{name: "down", key: vtinput.VK_DOWN, want: 46},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := NewEditorView(piecetable.New([]byte(content.String())), nil,
				"arrow-navigation.txt")
			defer editor.Close()
			editor.Highlighter = nil
			editor.SetPosition(0, 0, 39, 8)
			editor.SetVisible(true)
			editor.ScrollTopRow = 40
			editor.CursorLine, editor.CursorPos = 45, 8
			target := vtui.SemanticID(editor)
			if !editor.HandleSemanticAction(map[string]any{
				"target": target, "action": "editor.scroll", "visualRow": 120,
				"generation": uint64(11),
			}) || !editor.semanticPendingScroll {
				t.Fatal("test scroll destination was not pending")
			}
			if !editor.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tc.key,
			}) {
				t.Fatalf("%s was not handled", tc.name)
			}
			if editor.semanticPendingScroll ||
				editor.semanticWindowGeneration != 12 ||
				editor.semanticWindowRequestGeneration != 12 {
				t.Fatalf("%s fence state: pending=%v window=%d request=%d",
					tc.name, editor.semanticPendingScroll,
					editor.semanticWindowGeneration,
					editor.semanticWindowRequestGeneration)
			}
			if editor.CursorLine != tc.want {
				t.Fatalf("%s moved cursor to line %d, want %d",
					tc.name, editor.CursorLine, tc.want)
			}
			state := editor.SemanticNode(nil)
			if semantic.Int64(state["windowGeneration"]) != 12 {
				t.Fatalf("%s scene generation=%v, want 12",
					tc.name, state["windowGeneration"])
			}
		})
	}

	originalConfig := config.App
	t.Cleanup(func() { config.App = originalConfig })
	config.App.WheelEditorDown = 1
	editor := NewEditorView(piecetable.New([]byte(content.String())), nil,
		"wheel-navigation.txt")
	defer editor.Close()
	editor.Highlighter = nil
	editor.SetPosition(0, 0, 39, 8)
	editor.SetVisible(true)
	editor.ScrollTopRow = 40
	editor.CursorLine, editor.CursorPos = 45, 8
	if !editor.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(editor), "action": "editor.scroll",
		"visualRow": 120, "generation": uint64(21),
	}) {
		t.Fatal("native wheel destination was not handled")
	}
	if !editor.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, WheelDirection: -1,
	}) {
		t.Fatal("wheel event was not handled")
	}
	if editor.semanticPendingScroll || editor.semanticWindowGeneration != 22 {
		t.Fatalf("wheel fence state: pending=%v window=%d",
			editor.semanticPendingScroll, editor.semanticWindowGeneration)
	}
}

func TestSemanticWindowProtocol_ContentKeyIgnoresCursorAndStreamSelection(t *testing.T) {
	previous := config.App.EditorMarkOccurrences
	config.App.EditorMarkOccurrences = false
	t.Cleanup(func() { config.App.EditorMarkOccurrences = previous })
	vtui.SetDefaultPalette()
	editor := NewEditorView(piecetable.New([]byte("alpha\nbeta\ngamma\ndelta\n")), nil,
		"content-key.txt")
	editor.SetPosition(0, 0, 39, 8)
	editor.CursorLine = 1
	first := semantic.String(editor.SemanticNode(nil)["windowContentKey"])
	if first == "" {
		t.Fatal("editor did not export a window content key")
	}

	// Cursor geometry is exported separately and must not force QML to inspect
	// or replace an otherwise byte-for-byte identical row model.
	editor.CursorLine = 2
	second := semantic.String(editor.SemanticNode(nil)["windowContentKey"])
	if second != first {
		t.Fatalf("cursor-only scene changed row content key: %q -> %q", first, second)
	}

	// Regular stream selection is a scalar overlay. Moving its endpoint must
	// leave the complete base-row fingerprint unchanged.
	editor.SelActive = true
	editor.SelAnchorOffset = editor.Li.GetLineOffset(1)
	editor.CursorLine = 1
	editor.CursorPos = 3
	selected := semantic.String(editor.SemanticNode(nil)["windowContentKey"])
	if selected != first {
		t.Fatalf("selection overlay changed row content key: %q -> %q", first, selected)
	}
}
