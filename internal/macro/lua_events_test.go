//go:build !extralite

package macro

import (
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestMacroRaiseEventRunsTheDeclaredActions(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), `
		__n = 0
		Event { group = "FolderChanged"; action = function(group) __n = __n + 1; __group = group end }
		Event { group = "folderchanged"; action = function() __n = __n + 10 end }
		Event { group = "ExitFAR"; action = function() __n = __n + 100 end }
	`)
	if !Engine.RaiseEvent("FolderChanged") {
		t.Fatal("RaiseEvent started nothing for a declared group")
	}
	if !Engine.WaitIdle(5 * time.Second) {
		t.Fatal("the event actions never finished")
	}
	values := macroGlobals(t, Engine, "__n", "__group")
	if lua.LVAsNumber(values["__n"]) != 11 || lua.LVAsString(values["__group"]) != "FolderChanged" {
		t.Fatalf("n=%v group=%v, want 11 and FolderChanged", values["__n"], values["__group"])
	}
	if Engine.RaiseEvent("Nothing") {
		t.Error("RaiseEvent started something for a group nobody declared")
	}
}

func TestMacroRaiseEventSkipsWhileAMacroRuns(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Event { group = "FolderChanged"; action = function() __ran = true end }
	`)
	Engine.running.Store(true)
	if Engine.RaiseEvent("FolderChanged") {
		t.Error("RaiseEvent ran while a macro was running")
	}
	Engine.running.Store(false)
}

func TestMacroRaiseEventIsSafeWithoutAnEngine(t *testing.T) {
	if (*LuaMacroEngine)(nil).RaiseEvent("FolderChanged") {
		t.Error("a nil engine raised an event")
	}
	if (*MacroManager)(nil).RaiseEvent("FolderChanged") || (&MacroManager{}).RaiseEvent("FolderChanged") {
		t.Error("a manager without Lua macros raised an event")
	}
}

func TestMacroRaiseEventNumbersPassesThem(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Event { group = "EditorEvent"; action = function(id, event, param) __id, __event, __param = id, event, param end }
	`)
	if !Engine.RaiseEventNumbers("EditorEvent", 7, 3, 0) {
		t.Fatal("RaiseEventNumbers started nothing for a declared group")
	}
	if !Engine.WaitIdle(5 * time.Second) {
		t.Fatal("the event action never finished")
	}
	values := macroGlobals(t, Engine, "__id", "__event", "__param")
	if lua.LVAsNumber(values["__id"]) != 7 || lua.LVAsNumber(values["__event"]) != 3 || lua.LVAsNumber(values["__param"]) != 0 {
		t.Fatalf("action got %v %v %v, want the numbers 7 3 0", values["__id"], values["__event"], values["__param"])
	}
	if values["__event"].Type() != lua.LTNumber {
		t.Errorf("event arrived as %v, want a number", values["__event"].Type())
	}
	manager := &MacroManager{Lua: Engine}
	if !manager.RaiseEditorEvent(1, 0) {
		t.Error("the manager did not raise the editor event")
	}
	Engine.WaitIdle(5 * time.Second)
	if (*MacroManager)(nil).RaiseEditorEvent(1, 0) || (&MacroManager{}).RaiseEditorEvent(1, 0) || (*LuaMacroEngine)(nil).RaiseEventNumbers("EditorEvent", 1) {
		t.Error("an event was raised without an engine")
	}
}

func TestMacroRaiseViewerEvent(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Event { group = "ViewerEvent"; action = function(id, event) __vid, __vevent = id, event end }
	`)
	manager := &MacroManager{Lua: Engine}
	if !manager.RaiseViewerEvent(4, 1) {
		t.Fatal("the manager did not raise the viewer event")
	}
	if !Engine.WaitIdle(5 * time.Second) {
		t.Fatal("the event action never finished")
	}
	values := macroGlobals(t, Engine, "__vid", "__vevent")
	if lua.LVAsNumber(values["__vid"]) != 4 || lua.LVAsNumber(values["__vevent"]) != 1 {
		t.Fatalf("action got %v %v, want 4 1", values["__vid"], values["__vevent"])
	}
	if (*MacroManager)(nil).RaiseViewerEvent(1, 0) || (&MacroManager{}).RaiseViewerEvent(1, 0) {
		t.Error("a viewer event was raised without an engine")
	}
}
