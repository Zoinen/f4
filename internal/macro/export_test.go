package macro

import (
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRecordedMacroRoundTrip is the claim the export has to earn: a recording
// exported to Lua and loaded back replays exactly what was recorded. It is
// also what makes the two macro backends interchangeable for key sequences.
func TestRecordedMacroRoundTrip(t *testing.T) {
	events := []*vtinput.InputEvent{
		keymap.ParseFarKey("F5"),
		keymap.ParseFarKey("Enter"),
		keymap.ParseFarKey("Esc"),
	}
	source := RecordedMacroToLua("Shell", "CtrlA", "Copy and confirm", events)

	host := newFakeMacroHost()
	Engine := newTestMacroEngine(t, host, source)

	if Engine.Count() != 1 {
		t.Fatalf("Count = %d, want 1; exported source was:\n%s", Engine.Count(), source)
	}
	macro := Engine.Find("Shell", "CtrlA")
	if macro == nil {
		t.Fatalf("the exported macro did not bind its key; source was:\n%s", source)
	}
	if macro.Description != "Copy and confirm" {
		t.Errorf("description = %q, want it carried over", macro.Description)
	}

	fireMacro(t, Engine, "CtrlA")
	if got := strings.Join(host.injectedKeys(), " "); got != "F5 Enter Esc" {
		t.Fatalf("replayed %q, want \"F5 Enter Esc\"", got)
	}
}

func TestRecordedMacroDefaults(t *testing.T) {
	source := RecordedMacroToLua("", "CtrlB", "", nil)
	Engine := newTestMacroEngine(t, newFakeMacroHost(), source)

	if Engine.Find("Viewer", "CtrlB") == nil {
		t.Errorf("an export without an area did not become Common; source was:\n%s", source)
	}
	macro := Engine.Find("Shell", "CtrlB")
	if macro == nil || macro.Description == "" {
		t.Error("an export without a description did not get one")
	}
}

func TestRecordedMacroEscapesText(t *testing.T) {
	description := `say "hi" \ here`
	source := RecordedMacroToLua("Shell", "CtrlC", description, nil)

	Engine := newTestMacroEngine(t, newFakeMacroHost(), source)
	macro := Engine.Find("Shell", "CtrlC")
	if macro == nil {
		t.Fatalf("quoting broke the exported file:\n%s", source)
	}
	if macro.Description != description {
		t.Errorf("description = %q, want %q", macro.Description, description)
	}
}

func TestRecordedMacroWrapsLongSequences(t *testing.T) {
	var events []*vtinput.InputEvent
	for i := 0; i < 40; i++ {
		events = append(events, keymap.ParseFarKey("F5"))
	}
	source := RecordedMacroToLua("Shell", "CtrlD", "long one", events)

	if strings.Count(source, "Keys(") < 2 {
		t.Errorf("a long sequence was written as one unreadable line:\n%s", source)
	}

	host := newFakeMacroHost()
	Engine := newTestMacroEngine(t, host, source)
	fireMacro(t, Engine, "CtrlD")

	if got := host.injectedKeys(); len(got) != len(events) {
		t.Fatalf("replayed %d keys, want %d", len(got), len(events))
	}
}

func TestRecordedMacroSkipsUnusableEvents(t *testing.T) {
	events := []*vtinput.InputEvent{keymap.ParseFarKey("F5"), nil, keymap.ParseFarKey("Tab")}
	source := RecordedMacroToLua("Shell", "CtrlE", "with a hole", events)

	host := newFakeMacroHost()
	Engine := newTestMacroEngine(t, host, source)
	fireMacro(t, Engine, "CtrlE")

	if got := strings.Join(host.injectedKeys(), " "); got != "F5 Tab" {
		t.Fatalf("replayed %q, want \"F5 Tab\"", got)
	}
}

func TestSaveRecordedMacroTakesEffectImmediately(t *testing.T) {
	dir := t.TempDir()
	host := newFakeMacroHost()
	Engine := newTestMacroEngine(t, host, "")

	manager := NewMacroManager("")
	manager.Lua = Engine

	events := []*vtinput.InputEvent{keymap.ParseFarKey("F7"), keymap.ParseFarKey("Esc")}
	if err := manager.SaveRecordedMacro(dir, "Shell", "CtrlA", "make and cancel", events); err != nil {
		t.Fatalf("SaveRecordedMacro: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "shell_ctrla.lua")); err != nil {
		t.Fatalf("the macro file was not written: %v", err)
	}

	// No restart: the macro has to work in the session that recorded it.
	fireMacro(t, Engine, "CtrlA")
	if got := strings.Join(host.injectedKeys(), " "); got != "F7 Esc" {
		t.Fatalf("replayed %q, want \"F7 Esc\"", got)
	}
}

func TestSaveRecordedMacroWithoutARunningEngine(t *testing.T) {
	dir := t.TempDir()
	manager := NewMacroManager("")

	if err := manager.SaveRecordedMacro(dir, "Shell", "CtrlB", "", nil); err != nil {
		t.Fatalf("SaveRecordedMacro: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "shell_ctrlb.lua")); err != nil {
		t.Error("a user with no macros yet could not record their first one")
	}
}
func TestRecordedMacroFileName(t *testing.T) {
	if got := RecordedMacroFileName("Shell", "CtrlA"); got != "shell_ctrla.lua" {
		t.Errorf("RecordedMacroFileName = %q, want shell_ctrla.lua", got)
	}
	if got := RecordedMacroFileName("", "CtrlShiftF5"); got != "common_ctrlshiftf5.lua" {
		t.Errorf("RecordedMacroFileName = %q, want common_ctrlshiftf5.lua", got)
	}

	// Punctuation keys must still produce a name every filesystem accepts.
	for _, key := range []string{"Ctrl.", "Ctrl/", "Ctrl\\", "Ctrl:", "Ctrl*", "Ctrl?", "Ctrl\"", "Ctrl<"} {
		name := RecordedMacroFileName("Shell", key)
		if strings.ContainsAny(name, `/\:*?"<>|`) {
			t.Errorf("RecordedMacroFileName(%q) = %q, which a filesystem may reject", key, name)
		}
	}
}
