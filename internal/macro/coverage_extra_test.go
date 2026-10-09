package macro

import (
	"reflect"
	"testing"
	"time"

	"github.com/unxed/f4/internal/keymap"
)

func TestMacroParsingHelpers(t *testing.T) {
	if got, want := splitMacroList(" Shell shell  EDITOR shell "), []string{"shell", "editor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("splitMacroList = %#v, want %#v", got, want)
	}
	if containsFold([]string{"Shell", "Editor"}, "viewer") {
		t.Fatal("containsFold found an absent value")
	}
	if !containsFold([]string{"Shell", "Editor"}, "EDITOR") {
		t.Fatal("containsFold missed a case-insensitive value")
	}

	events := parseMacroKeys("CtrlA not-a-far-key F5")
	if len(events) != 3 {
		t.Fatalf("parseMacroKeys returned %d events, want 3", len(events))
	}
	if got := keymap.EventToFarString(events[0]); got != "CtrlA" {
		t.Errorf("first parsed key = %q, want CtrlA", got)
	}
	if got := keymap.EventToFarString(events[2]); got != "F5" {
		t.Errorf("last parsed key = %q, want F5", got)
	}
}

func TestMacroBindingsShadowCommonAndRunExact(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { key = "CtrlA"; action = function() __ran = "common" end }
		Macro { area = "Shell"; key = "CtrlA"; action = function() __ran = "shell" end }
	`)

	bindings := engine.Bindings("Shell")
	if len(bindings) != 1 || bindings[0].Area != "shell" || bindings[0].Key != "ctrla" {
		t.Fatalf("Shell bindings = %#v, want only the shell-specific binding", bindings)
	}
	bindings = engine.Bindings("Viewer")
	if len(bindings) != 1 || bindings[0].Area != "common" || bindings[0].Key != "ctrla" {
		t.Fatalf("Viewer bindings = %#v, want the common fallback", bindings)
	}
	if engine.RunExact("Viewer", "CtrlA") {
		t.Fatal("RunExact used Common fallback")
	}
	if !engine.RunExact("Shell", "CtrlA") || !engine.WaitIdle(5*time.Second) {
		t.Fatal("RunExact did not run the area-specific binding")
	}
	if got := macroGlobals(t, engine, "__ran")["__ran"]; got.String() != "shell" {
		t.Fatalf("RunExact set __ran = %v, want shell", got)
	}
	if (*LuaMacroEngine)(nil).Bindings("Shell") != nil || (*LuaMacroEngine)(nil).RunExact("Shell", "CtrlA") {
		t.Fatal("nil engine exposed bindings or ran a macro")
	}
}
