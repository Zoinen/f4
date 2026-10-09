package macro

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// farStrings renders a loaded macro sequence the way a human reads a
// hotkeys.ini Sequence=, which is what the assertions below compare against.
func farStrings(events []*vtinput.InputEvent) string {
	names := make([]string, len(events))
	for i, e := range events {
		names[i] = keymap.EventToFarString(e)
	}
	return strings.Join(names, " ")
}

func writeMacrosIni(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "macros.ini")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ini: %v", err)
	}
	return path
}

func TestMacroManagerLoadParsesKeyMacrosSections(t *testing.T) {
	path := writeMacrosIni(t, `
[KeyMacros/Shell/F5]
DisableOutput=0x1
Sequence=F5 Enter

[KeyMacros/Shell/F6]
DisableOutput=0x1
Sequence=callplugin(id) F7 eval(x) foo(bar) F8
`)
	mgr := NewMacroManager(path)

	events, ok := mgr.Macros["Shell"]["F5"]
	if !ok {
		t.Fatal("Shell/F5 was not loaded")
	}
	if got := farStrings(events); got != "F5 Enter" {
		t.Errorf("Shell/F5 sequence = %q, want %q", got, "F5 Enter")
	}

	events, ok = mgr.Macros["Shell"]["F6"]
	if !ok {
		t.Fatal("Shell/F6 was not loaded")
	}
	if got := farStrings(events); got != "F7 F8" {
		t.Errorf("Shell/F6 sequence = %q, want far2l callplugin/eval/paren tokens skipped and F7 F8 kept", got)
	}
}

func TestMacroManagerLoadSkipsEmptySequence(t *testing.T) {
	path := writeMacrosIni(t, "[KeyMacros/Shell/F9]\nSequence=\n")
	mgr := NewMacroManager(path)
	if _, ok := mgr.Macros["Shell"]["F9"]; ok {
		t.Error("a KeyMacros section with an empty Sequence was loaded as a macro")
	}
}

func TestMacroManagerLoadIgnoresMalformedKeyMacrosSectionName(t *testing.T) {
	// "KeyMacros/OnlyArea" has the right prefix but is missing the hotkey
	// component, so SplitN yields only two parts and the section must be
	// dropped rather than misfiled.
	path := writeMacrosIni(t, "[KeyMacros/OnlyArea]\nSequence=F1\n")
	mgr := NewMacroManager(path)
	if len(mgr.Macros) != 0 {
		t.Errorf("Macros = %v, want nothing loaded from a malformed KeyMacros section name", mgr.Macros)
	}
}

func TestMacroManagerLoadLegacyAreaSection(t *testing.T) {
	path := writeMacrosIni(t, `
[Shell]
Ctrl+A=72:0:0,101:0:0
NotAMacro=plain value
HasColonButSpaced=72:0:0, with spaces
NoColon=nocolonhere
BadFieldCount=1:2
ParseErrorThenOK=x:0:0,72:0:0
InvalidRune=55296:0:0,72:0:0
`)
	mgr := NewMacroManager(path)
	shell := mgr.Macros["Shell"]

	// "+" is stripped from the key, and each colon-delimited char:vk:mods
	// triple becomes one event.
	events, ok := shell["CtrlA"]
	if !ok {
		t.Fatal("Ctrl+A was not loaded as CtrlA")
	}
	if got := farStrings(events); got != "H E" {
		t.Errorf("CtrlA sequence = %q, want %q", got, "H E")
	}

	// Values without both a colon and no spaces are not macros at all.
	if _, ok := shell["NotAMacro"]; ok {
		t.Error("a value without a colon was treated as a macro")
	}
	if _, ok := shell["HasColonButSpaced"]; ok {
		t.Error("a value containing a space was treated as a macro")
	}
	if _, ok := shell["NoColon"]; ok {
		t.Error("a value without a colon was treated as a macro")
	}

	// A triple with the wrong field count is skipped, but the key itself is
	// still registered, empty.
	if events, ok := shell["BadFieldCount"]; !ok {
		t.Error("BadFieldCount was not registered at all")
	} else if len(events) != 0 {
		t.Errorf("BadFieldCount events = %v, want none", events)
	}

	// One malformed group in a comma list does not poison the rest of it.
	if got := farStrings(shell["ParseErrorThenOK"]); got != "H" {
		t.Errorf("ParseErrorThenOK sequence = %q, want %q", got, "H")
	}

	// A surrogate code point is not a valid rune and must be skipped, again
	// without losing the other group in the same list.
	if got := farStrings(shell["InvalidRune"]); got != "H" {
		t.Errorf("InvalidRune sequence = %q, want %q", got, "H")
	}
}

func TestMacroManagerLoadMigratesLegacyMacrosSectionToCommon(t *testing.T) {
	path := writeMacrosIni(t, "[Macros]\nCtrlZ=90:0:0\n")
	mgr := NewMacroManager(path)

	if _, ok := mgr.Macros["Common"]["CtrlZ"]; !ok {
		t.Error("the legacy [Macros] section was not migrated into the Common area")
	}
	if _, ok := mgr.Macros["Macros"]; ok {
		t.Error("the legacy section name leaked into the loaded macros as its own area")
	}
}

func TestMacroManagerSaveWritesAndReloadsMacros(t *testing.T) {
	// The ini file lives under a directory that does not exist yet, so Save
	// must create it.
	path := filepath.Join(t.TempDir(), "nested", "macros.ini")
	mgr := NewMacroManager(path)
	mgr.Macros["Shell"] = map[string][]*vtinput.InputEvent{
		"F5": {keymap.ParseFarKey("F5"), keymap.ParseFarKey("Enter")},
		"F6": {}, // an empty sequence must not be written out at all
	}

	mgr.Save()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Save did not create the ini file: %v", err)
	}
	// Windows has no Unix permission bits: os.Stat reports 0666 for any
	// writable file whatever mode it was created with, so the 0600 check
	// only means something elsewhere.
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("ini file mode = %v, want 0600", perm)
	}

	reloaded := NewMacroManager(path)
	events, ok := reloaded.Macros["Shell"]["F5"]
	if !ok {
		t.Fatal("the saved macro was not present after reload")
	}
	if got := farStrings(events); got != "F5 Enter" {
		t.Errorf("reloaded sequence = %q, want %q", got, "F5 Enter")
	}
	if _, ok := reloaded.Macros["Shell"]["F6"]; ok {
		t.Error("a macro with an empty key sequence was written to disk")
	}
}

func TestMacroManagerToggleRecordingNilReceiver(t *testing.T) {
	var mgr *MacroManager
	if mgr.ToggleRecording("Shell") {
		t.Error("ToggleRecording on a nil *MacroManager reported success")
	}
}

func TestMacroManagerToggleRecordingWithoutFrameManager(t *testing.T) {
	saved := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = saved })

	mgr := &MacroManager{}
	if mgr.ToggleRecording("Shell") {
		t.Error("ToggleRecording reported success with no FrameManager installed")
	}
	if mgr.Recording {
		t.Error("ToggleRecording flipped Recording despite bailing out early")
	}
}
