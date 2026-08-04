package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestMacro_Far2lCompatibility(t *testing.T) {
	iniContent := `
[KeyMacros/Shell/CtrlW]
DisableOutput=0x1
Sequence=Up Up CtrlEnter Esc F5 Down ShiftF5 Esc Esc
`
	tmpFile := filepath.Join(t.TempDir(), "far2l_macros.ini")
	os.WriteFile(tmpFile, []byte(iniContent), 0644)

	mgr := NewMacroManager(tmpFile)

	shellMacros, ok := mgr.Macros["Shell"]
	if !ok {
		t.Fatal("Shell macros not loaded")
	}

	seq, ok := shellMacros["CtrlW"]
	if !ok {
		t.Fatal("CtrlW macro not found in Shell")
	}

	if len(seq) != 9 {
		t.Fatalf("Expected 9 keys, got %d", len(seq))
	}

	if seq[0].VirtualKeyCode != vtinput.VK_UP {
		t.Errorf("Expected first key VK_UP, got %d", seq[0].VirtualKeyCode)
	}
	if seq[2].VirtualKeyCode != vtinput.VK_RETURN || !normalizeMods(seq[2].ControlKeyState).Contains(vtinput.LeftCtrlPressed) {
		t.Errorf("Expected third key CtrlEnter, got %d", seq[2].VirtualKeyCode)
	}

	mgr.Save()

	savedData, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	savedStr := string(savedData)
	if !strings.Contains(savedStr, "[KeyMacros/Shell/CtrlW]") {
		t.Errorf("Saved config lost section name: %s", savedStr)
	}
	if !strings.Contains(savedStr, "Sequence=Up Up CtrlEnter Esc F5 Down ShiftF5 Esc Esc") {
		t.Errorf("Saved config lost sequence: %s", savedStr)
	}
}

type mockAreaFrame struct {
	vtui.BaseFrame
	typ   vtui.FrameType
	title string
}

func (m *mockAreaFrame) GetType() vtui.FrameType { return m.typ }
func (m *mockAreaFrame) GetTitle() string        { return m.title }

func TestMacro_GetCurrentArea(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	mgr := NewMacroManager("")

	// 1. Empty FrameManager -> "Common"
	if area := mgr.GetCurrentArea(); area != "Common" {
		t.Errorf("Expected 'Common' for empty FrameManager, got %q", area)
	}

	// 2. Dialog -> "Dialog"
	fDialog := &mockAreaFrame{typ: vtui.TypeDialog}
	vtui.FrameManager.Push(fDialog)
	if area := mgr.GetCurrentArea(); area != "Dialog" {
		t.Errorf("Expected 'Dialog', got %q", area)
	}
	vtui.FrameManager.Pop()

	// 3. Menu -> "Menu"
	fMenu := &mockAreaFrame{typ: vtui.TypeMenu}
	vtui.FrameManager.Push(fMenu)
	if area := mgr.GetCurrentArea(); area != "Menu" {
		t.Errorf("Expected 'Menu', got %q", area)
	}
	vtui.FrameManager.Pop()

	// 4. EditorView -> "Editor"
	fEditor := &mockAreaFrame{typ: vtui.TypeUser + 2}
	vtui.FrameManager.Push(fEditor)
	if area := mgr.GetCurrentArea(); area != "Editor" {
		t.Errorf("Expected 'Editor', got %q", area)
	}
	vtui.FrameManager.Pop()

	// 5. ViewerView -> "Viewer"
	fViewer := &mockAreaFrame{typ: vtui.TypeUser + 3}
	vtui.FrameManager.Push(fViewer)
	if area := mgr.GetCurrentArea(); area != "Viewer" {
		t.Errorf("Expected 'Viewer', got %q", area)
	}
	vtui.FrameManager.Pop()
}
func TestMacroRecordingAndPlayback(t *testing.T) {
	tmpFile := "test_macros.ini"
	defer os.Remove(tmpFile)

	mgr := NewMacroManager(tmpFile)

	// Trigger recording start (Ctrl+.)
	ctrlDot := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_OEM_PERIOD,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}

	if !mgr.Filter(ctrlDot) {
		t.Fatal("Ctrl+. should be filtered and start recording")
	}
	if !mgr.Recording {
		t.Fatal("Manager should be in recording state")
	}

	// Send a normal key 'A'
	keyA := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_A,
		Char:           'a',
	}
	mgr.Filter(keyA)

	if len(mgr.Buffer) != 1 {
		t.Fatalf("Expected 1 event in buffer, got %d", len(mgr.Buffer))
	}

	// Stop recording
	mgr.Filter(ctrlDot)
	if mgr.Recording {
		t.Fatal("Manager should stop recording")
	}

	// Simulate Assign Frame capturing Ctrl+F1
	ctrlF1 := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_F1,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}

	assignFrame := NewMacroAssignFrame(mgr)
	assignFrame.ProcessKey(ctrlF1)

	f1Key := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F1, ControlKeyState: vtinput.LeftCtrlPressed})
	if _, ok := mgr.Macros["Common"][f1Key]; !ok {
		t.Fatal("Macro should be saved with Ctrl+F1 key in Common area")
	}

	// Test reloading from file
	mgr2 := NewMacroManager(tmpFile)
	if _, ok := mgr2.Macros["Common"][f1Key]; !ok {
		t.Fatal("Macro was not correctly loaded from INI file")
	}
}

func TestKeyNormalization(t *testing.T) {
	// Check that Left and Right Ctrl give same key string
	k1 := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_A, Char: 'a', ControlKeyState: vtinput.LeftCtrlPressed})
	k2 := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_A, Char: 'a', ControlKeyState: vtinput.RightCtrlPressed})
	if k1 != k2 {
		t.Errorf("Normalization failed: %s != %s", k1, k2)
	}

	// Check Ctrl+Shift combination
	k3 := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_B, Char: 'B', ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed})
	if k3 != "CtrlShiftB" {
		t.Errorf("Complex normalization failed: %s", k3)
	}
}

func TestMacroPlaybackLogic(t *testing.T) {
	mgr := NewMacroManager("unused.ini")

	// Create macro: print "hi" on F2 press
	f2Key := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F2})
	macroSeq := []*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'h', VirtualKeyCode: vtinput.VK_H},
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'i', VirtualKeyCode: vtinput.VK_I},
	}
	mgr.Macros["Common"] = make(map[string][]*vtinput.InputEvent)
	mgr.Macros["Common"][f2Key] = macroSeq

	// Simulate F2 press
	pressF2 := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F2,
	}

	// Hack for test: intercepting InjectEvents by replacing global FrameManager is not easy,
	// but we can check that Filter returned true (event consumed to be replaced by macro)
	if !mgr.Filter(pressF2) {
		t.Error("Filter should return true when triggering a macro")
	}
}
func TestMacro_FilterTriggerSwallowing_Order(t *testing.T) {
	mgr := NewMacroManager("unused.ini")
	mgr.Recording = true
	mgr.Buffer = make([]*vtinput.InputEvent, 0)

	// Trigger stop recording via Ctrl+.
	stopEvent := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		Char: '.', ControlKeyState: vtinput.LeftCtrlPressed,
	}

	res := mgr.Filter(stopEvent)

	if !res {
		t.Error("Filter should swallow the stop trigger even if recording is active")
	}
	if len(mgr.Buffer) != 0 {
		t.Errorf("Stop trigger should NOT be added to macro buffer, but buffer size is %d", len(mgr.Buffer))
	}
}
func TestMacro_TriggerSwallowing(t *testing.T) {
	mgr := NewMacroManager("unused.ini")

	// 1. Start recording via Ctrl+. (using Char for compatibility)
	startEvent := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		Char: '.', ControlKeyState: vtinput.LeftCtrlPressed,
	}
	mgr.Filter(startEvent)
	if !mgr.Recording {
		t.Fatal("Should be recording")
	}

	// 2. Type 'A'
	mgr.Filter(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'a', VirtualKeyCode: vtinput.VK_A})

	// 3. Stop recording via Ctrl+.
	stopEvent := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		Char: '.', ControlKeyState: vtinput.LeftCtrlPressed,
	}
	res := mgr.Filter(stopEvent)

	if !res {
		t.Error("Stop trigger should be consumed (return true)")
	}
	if mgr.Recording {
		t.Error("Should have stopped recording")
	}

	// 4. Verify buffer: should ONLY contain 'a', NOT the trigger dot
	if len(mgr.Buffer) != 1 || mgr.Buffer[0].Char != 'a' {
		t.Errorf("Macro buffer polluted or incomplete. Items: %d", len(mgr.Buffer))
	}
}

func TestMacro_AssignRobustness(t *testing.T) {
	// Clean manager for testing
	mgr := &MacroManager{
		Macros:    make(map[string]map[string][]*vtinput.InputEvent),
		StartArea: "Common",
	}
	mgr.Buffer = []*vtinput.InputEvent{{Char: 'x', KeyDown: true}}
	f := NewMacroAssignFrame(mgr)

	// 1. Standalone modifiers should be ignored (dialog stays open)
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SHIFT})
	if f.Done {
		t.Error("Assign dialog should ignore standalone Shift")
	}

	// 2. Esc SHOULD now cancel the dialog without assignment
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if !f.Done {
		t.Error("Assign dialog should close after pressing Esc")
	}

	escKey := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_ESCAPE})
	if _, ok := mgr.Macros["Common"][escKey]; ok {
		t.Error("Esc should cancel, not assign a macro")
	}

	// 3. Test Alt+X assignment
	f.Done = false
	mgr.Buffer = []*vtinput.InputEvent{{Char: 'y', KeyDown: true}}
	f.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_X,
		Char:            'x',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	altXKey := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_X, Char: 'x', ControlKeyState: vtinput.LeftAltPressed})
	if _, ok := mgr.Macros["Common"][altXKey]; !ok {
		t.Error("Macro failed to assign to Alt+X")
	}
}

func TestMacro_KeyUpConsumption(t *testing.T) {
	mgr := NewMacroManager("unused.ini")

	// Start recording
	ctrlDot := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_OEM_PERIOD, ControlKeyState: vtinput.LeftCtrlPressed,
	}
	mgr.Filter(ctrlDot)

	// Release trigger (KeyUp)
	ctrlDotUp := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: false,
		VirtualKeyCode: vtinput.VK_OEM_PERIOD, ControlKeyState: vtinput.LeftCtrlPressed,
	}

	if !mgr.Filter(ctrlDotUp) {
		t.Error("KeyUp for Ctrl+. should be consumed by the filter")
	}

	// Normal key release during recording should NOT be added to buffer
	keyAUp := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: false,
		VirtualKeyCode: vtinput.VK_A, Char: 'a',
	}
	mgr.Filter(keyAUp)
	if len(mgr.Buffer) != 0 {
		t.Errorf("KeyUp should not be recorded in macro buffer, got length %d", len(mgr.Buffer))
	}
}

func TestMacro_CancelEsc(t *testing.T) {
	tmpPath := filepath.Join(os.TempDir(), "esc.ini")
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	mgr := NewMacroManager(tmpPath)
	mgr.Recording = true
	mgr.Buffer = []*vtinput.InputEvent{{Char: 'h', KeyDown: true}}

	assign := NewMacroAssignFrame(mgr)
	escEvent := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}

	assign.ProcessKey(escEvent)

	key := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_ESCAPE})
	if _, ok := mgr.Macros["Common"][key]; ok {
		t.Error("Esc should cancel, not assign a macro")
	}
	if !assign.Done {
		t.Error("Assign frame should be Done after cancellation")
	}
}

func TestMacro_Clear(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "clear_macros.ini")
	mgr := NewMacroManager(tmpFile)

	mgr.StartArea = "Common"

	// 1. Assign a macro first
	key := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F3})
	mgr.Macros["Common"] = make(map[string][]*vtinput.InputEvent)
	mgr.Macros["Common"][key] = []*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'x'},
	}
	mgr.Save()

	// 2. Simulate empty recording and assigning to F3 (to clear it)
	mgr.Buffer = nil // Empty recording

	assignFrame := NewMacroAssignFrame(mgr)
	assignFrame.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})

	// 3. Verify it is deleted from active map
	if _, ok := mgr.Macros["Common"][key]; ok {
		t.Error("Macro should be completely deleted from map when assigned an empty buffer")
	}

	// 4. Verify it is deleted from saved file
	mgr2 := NewMacroManager(tmpFile)
	if _, ok := mgr2.Macros["Common"][key]; ok {
		t.Error("Cleared macro should not persist in the saved INI file")
	}
}

func TestMacro_CharTrigger(t *testing.T) {
	mgr := NewMacroManager("unused.ini")

	// Test trigger using Char instead of VK (for terminals that map dot differently)
	event := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		Char: '.', VirtualKeyCode: 0, ControlKeyState: vtinput.LeftCtrlPressed,
	}

	if !mgr.Filter(event) {
		t.Error("Macro recording should start via Char '.' detection")
	}
	if !mgr.Recording {
		t.Error("Manager failed to enter recording state via Char trigger")
	}
}
func TestMacro_AssignFrame_Structure(t *testing.T) {
	mgr := &MacroManager{
		Macros:    make(map[string]map[string][]*vtinput.InputEvent),
		StartArea: "Common",
	}
	f := NewMacroAssignFrame(mgr)

	// Check that it's a proper window with a child (the prompt text)
	if len(f.GetChildren()) == 0 {
		t.Error("MacroAssignFrame should have at least one child (prompt)")
	}

	// Validate Layout
	vtui.AssertLayout(t, f)

	// Verify focus logic: it should NOT allow Tab to cycle away
	// because any key (including Tab) must be captured as a macro.
	tabEvent := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_TAB,
	}

	mgr.Buffer = []*vtinput.InputEvent{{Char: 't', KeyDown: true}}
	handled := f.ProcessKey(tabEvent)

	if !handled {
		t.Error("MacroAssignFrame should handle (capture) Tab key")
	}
	if !f.IsDone() {
		t.Error("MacroAssignFrame should close after capturing a key")
	}

	// Verify that macro was assigned to Tab
	tabKey := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_TAB})
	if _, ok := mgr.Macros["Common"][tabKey]; !ok {
		t.Error("Macro failed to assign to Tab key")
	}
}

func TestMacroKeyStrDistinguishesEnhancedKeys(t *testing.T) {
	// Standard Delete has the EnhancedKey modifier in modern protocols
	delKeyStr := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_DELETE, ControlKeyState: vtinput.EnhancedKey})

	// Numpad Delete (NumDel) does not have the EnhancedKey modifier
	numDelKeyStr := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_DELETE, ControlKeyState: 0})

	if delKeyStr == numDelKeyStr {
		t.Errorf("Expected different KeyStr representations for standard Del (%q) and NumDel (%q), but they are identical", delKeyStr, numDelKeyStr)
	}
}

func TestMacroIgnoresStandaloneModifiers(t *testing.T) {
	mgr := NewMacroManager("")
	mgr.Recording = true
	mgr.Buffer = nil

	// Simulate pressing Ctrl, then Shift, then a letter 'A', then releasing them
	events := []*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_CONTROL},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SHIFT},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A, Char: 'A'},
		{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_A, Char: 'A'},
		{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_SHIFT},
		{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_CONTROL},
	}

	for _, ev := range events {
		mgr.Filter(ev)
	}

	if len(mgr.Buffer) != 1 {
		t.Errorf("Expected exactly 1 event in macro buffer, but got %d", len(mgr.Buffer))
	} else if mgr.Buffer[0].VirtualKeyCode != vtinput.VK_A {
		t.Errorf("Expected recorded event to be VK_A, but got %v", vtinput.VKString(mgr.Buffer[0].VirtualKeyCode))
	}
}

func TestMacroClearRecordingIsEmpty(t *testing.T) {
	mgr := NewMacroManager("")

	// Start recording
	startEvent := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_OEM_PERIOD,
		Char:            '.',
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	mgr.Filter(startEvent)

	if !mgr.Recording {
		t.Error("Expected MacroManager to be in Recording state")
	}

	// Pressing Ctrl key before pressing '.' to stop recording
	ctrlDown := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_CONTROL,
	}
	mgr.Filter(ctrlDown)

	// Pressing '.' to stop recording
	stopEvent := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_OEM_PERIOD,
		Char:            '.',
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	mgr.Filter(stopEvent)

	if mgr.Recording {
		t.Error("Expected MacroManager to stop Recording")
	}

	if len(mgr.Buffer) != 0 {
		t.Errorf("Expected macro buffer to be empty for immediate stop recording, but got %d items", len(mgr.Buffer))
	}
}

func TestMacroClearResetsExisting(t *testing.T) {
	mgr := NewMacroManager("")
	clearKeyStr := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_CLEAR})
	mgr.Macros = map[string]map[string][]*vtinput.InputEvent{
		"Common": {
			clearKeyStr: {{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_CLEAR}},
		},
	}
	mgr.StartArea = "Common"

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	// 1. Начинаем запись макроса
	startEvent := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_OEM_PERIOD,
		Char:            '.',
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	mgr.Filter(startEvent)

	if !mgr.Recording {
		t.Error("Expected MacroManager to be in Recording state")
	}

	// 2. Останавливаем запись (буфер пуст)
	stopEvent := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_OEM_PERIOD,
		Char:            '.',
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	mgr.Filter(stopEvent)

	if mgr.Recording {
		t.Error("Expected MacroManager to stop Recording")
	}

	// Выполняем все накопившиеся асинхронные задачи, пока не включится нужный режим
	timeout := time.After(1 * time.Second)
WaitLoop:
	for !mgr.Assigning {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for Assigning state")
			break WaitLoop
		}
	}

	if !mgr.Assigning {
		t.Error("Expected MacroManager to be in Assigning state")
	}

	// Пытаемся нажать 'Clear' (VK_CLEAR = 0x0C)
	clearEvent := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_CLEAR,
	}

	// Фильтр НЕ должен поглотить событие воспроизведением старого макроса,
	// так как активен режим назначения (Assigning == true)
	consumed := mgr.Filter(clearEvent)
	if consumed {
		t.Error("Expected Filter to not consume VK_CLEAR while Assigning is active")
	}

	// Симулируем обработку нажатия диалогом
	frame := NewMacroAssignFrame(mgr)
	frame.ProcessKey(clearEvent)

	// После обработки флаг назначения должен сброситься, а макрос удалиться
	if mgr.Assigning {
		t.Error("Expected Assigning state to be cleared after key processing")
	}

	if _, exists := mgr.Macros["Common"][clearKeyStr]; exists {
		t.Error("Expected macro to be deleted")
	}
}
func TestMacro_ReassignAndCleanup(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "reassign_macros.ini")
	mgr := NewMacroManager(tmpFile)
	mgr.StartArea = "Common"

	key := EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_F3})
	mgr.Macros["Common"] = map[string][]*vtinput.InputEvent{
		key: {
			&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3},
		},
	}
	mgr.Save()

	host := newFakeMacroHost()
	engine, _ := NewLuaMacroEngine(host)
	mgr.Lua = engine

	scriptDir := filepath.Join(GetF4ConfigDir(), "Macros", "scripts")
	os.MkdirAll(scriptDir, 0755)
	scriptPath := filepath.Join(scriptDir, RecordedMacroFileName("Common", key))
	os.WriteFile(scriptPath, []byte(""), 0644)

	_ = engine.LoadString("test", `Macro { area = "Common"; key = "F3"; action = function() end }`)

	mgr.Buffer = nil
	assignFrame := NewMacroAssignFrame(mgr)
	assignFrame.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})

	if _, ok := mgr.Macros["Common"][key]; ok {
		t.Error("Macro should be deleted from INI")
	}
	if engine.Find("Common", "F3") != nil {
		t.Error("Macro should be deleted from Lua Engine")
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("Lua script file should be deleted from disk")
	}
}
