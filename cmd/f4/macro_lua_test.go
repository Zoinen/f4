package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/vtinput"
	lua "github.com/yuin/gopher-lua"
)

// fakeMacroHost stands in for f4's UI so the engine can be tested without a
// terminal. That it is possible at all is the point of the MacroHost seam.
type fakeMacroHost struct {
	mu         sync.Mutex
	area       string
	panels     map[bool]MacroPanelInfo
	cmdLine    string
	width      int
	height     int
	title      string
	injected   []*vtinput.InputEvent
	messages   []string
	logs       []string
	pluginCall func(context.Context, string, []any) ([]any, error)
}

func newFakeMacroHost() *fakeMacroHost {
	return &fakeMacroHost{
		area:   "Shell",
		panels: map[bool]MacroPanelInfo{},
		width:  80,
		height: 25,
		title:  "f4-test",
	}
}

func (h *fakeMacroHost) CurrentArea() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.area
}

func (h *fakeMacroHost) Panel(active bool) MacroPanelInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.panels[active]
}

func (h *fakeMacroHost) CommandLine() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cmdLine
}

func (h *fakeMacroHost) ScreenSize() (int, int) { return h.width, h.height }
func (h *fakeMacroHost) Version() string        { return "f4-test" }
func (h *fakeMacroHost) WindowTitle() string    { return h.title }

func (h *fakeMacroHost) Message(title, text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, title+"|"+text)
}

func (h *fakeMacroHost) InjectKeys(keys []*vtinput.InputEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.injected = append(h.injected, keys...)
}

func (h *fakeMacroHost) Log(format string, args ...any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, format)
}

func (h *fakeMacroHost) RunAction(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, "action:"+name)
	return true
}

func (h *fakeMacroHost) CallPlugin(ctx context.Context, id string, args []any) ([]any, error) {
	if h.pluginCall == nil {
		return nil, errMacroCallProviderNotFound
	}
	return h.pluginCall(ctx, id, args)
}

func (h *fakeMacroHost) injectedKeys() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	names := make([]string, 0, len(h.injected))
	for _, event := range h.injected {
		names = append(names, EventToFarString(event))
	}
	return names
}

func newTestMacroEngine(t *testing.T, host MacroHost, source string) *LuaMacroEngine {
	t.Helper()
	engine, err := NewLuaMacroEngine(host)
	if err != nil {
		t.Fatalf("NewLuaMacroEngine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("close Lua macro engine: %v", err)
		}
	})

	if source != "" {
		if err := engine.LoadString("test", source); err != nil {
			t.Fatalf("LoadString: %v", err)
		}
	}
	return engine
}

// fireMacro triggers a key and waits for the macro to finish, since macro
// execution is asynchronous by design.
func fireMacro(t *testing.T, engine *LuaMacroEngine, key string) bool {
	t.Helper()
	consumed := engine.Trigger(engine.host.CurrentArea(), ParseFarKey(key))
	if !engine.waitIdle(5 * time.Second) {
		t.Fatal("macro did not finish in time")
	}
	return consumed
}

// macroGlobals reads globals back out of the interpreter, which is how these
// tests observe what an action did.
func macroGlobals(t *testing.T, engine *LuaMacroEngine, names ...string) map[string]lua.LValue {
	t.Helper()
	values := make(map[string]lua.LValue, len(names))
	err := engine.rt.Do(func(L *lua.LState) error {
		for _, name := range names {
			values[name] = L.GetGlobal(name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading globals: %v", err)
	}
	return values
}

func TestMacroRegistration(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { area = "Shell Editor"; key = "CtrlA CtrlB"; description = "two by two";
			action = function() end }
	`)

	if engine.Count() != 1 {
		t.Fatalf("Count = %d, want 1", engine.Count())
	}
	for _, area := range []string{"Shell", "shell", "Editor"} {
		for _, key := range []string{"CtrlA", "ctrla", "CtrlB"} {
			if engine.Find(area, key) == nil {
				t.Errorf("Find(%q, %q) found nothing", area, key)
			}
		}
	}
	if engine.Find("Viewer", "CtrlA") != nil {
		t.Error("a Shell macro leaked into the Viewer area")
	}
	if engine.Find("Shell", "CtrlC") != nil {
		t.Error("an unbound key resolved to a macro")
	}
}

func TestMacroCommonFallbackAndTerminalAlias(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { key = "CtrlG"; action = function() end }
		Macro { area = "Shell"; key = "CtrlH"; action = function() end }
	`)

	if engine.Find("Viewer", "CtrlG") == nil {
		t.Error("a macro without an area did not fall back to common")
	}
	if engine.Find("Terminal", "CtrlH") == nil {
		t.Error("the Terminal area did not resolve to Far's shell")
	}
}

func TestMacroLastRegistrationWins(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { area = "Shell"; key = "CtrlJ"; description = "first"; action = function() Keys("F1") end }
		Macro { area = "Shell"; key = "CtrlJ"; description = "second"; action = function() Keys("F2") end }
	`)

	macro := engine.Find("Shell", "CtrlJ")
	if macro == nil || macro.Description != "second" {
		t.Fatalf("Find returned %v, want the second registration", macro)
	}
}

func TestMacroRejectsIncompleteDeclarations(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), "")

	if err := engine.LoadString("bad", `Macro { key = "CtrlK" }`); err == nil {
		t.Error("a macro without an action was accepted")
	}
	if err := engine.LoadString("bad", `Macro { action = function() end }`); err == nil {
		t.Error("a macro without a key was accepted")
	}
	if engine.Count() != 0 {
		t.Errorf("Count = %d, want 0", engine.Count())
	}
}

func TestMacroKeysAreInjected(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlL"; action = function()
			Keys("F5 Enter")
			Keys("Esc")
		end }
	`)

	if !fireMacro(t, engine, "CtrlL") {
		t.Fatal("the trigger key was not consumed")
	}
	got := strings.Join(host.injectedKeys(), " ")
	if got != "F5 Enter Esc" {
		t.Fatalf("injected %q, want \"F5 Enter Esc\"", got)
	}
}

func TestMacroUnboundKeyIsNotConsumed(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { area = "Shell"; key = "CtrlM"; action = function() end }
	`)

	if engine.Trigger("Shell", ParseFarKey("CtrlN")) {
		t.Fatal("an unbound key was consumed")
	}
}

func TestMacroConditionDeclinesAndReplaysTheKey(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		ran = false
		Macro { area = "Shell"; key = "CtrlO";
			condition = function() return false end;
			action = function() ran = true; Keys("F9") end }
	`)

	if !fireMacro(t, engine, "CtrlO") {
		t.Fatal("the trigger key was not consumed")
	}
	if macroGlobals(t, engine, "ran")["ran"] == lua.LTrue {
		t.Error("the action ran even though the condition declined")
	}

	got := host.injectedKeys()
	if len(got) != 1 || got[0] != "CtrlO" {
		t.Fatalf("injected %v, want the original key replayed", got)
	}
}

func TestMacroConditionAccepts(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlP";
			condition = function(key) return key == "CtrlP" end;
			action = function() Keys("Tab") end }
	`)

	fireMacro(t, engine, "CtrlP")
	if got := host.injectedKeys(); len(got) != 1 || got[0] != "Tab" {
		t.Fatalf("injected %v, want [Tab]", got)
	}
}

func TestMacroAKeyAndExit(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlQ"; action = function()
			invoked = akey()
			Keys("F3")
			exit()
			Keys("F4")
		end }
	`)

	fireMacro(t, engine, "CtrlQ")

	if got := lua.LVAsString(macroGlobals(t, engine, "invoked")["invoked"]); got != "CtrlQ" {
		t.Errorf("akey() returned %q, want CtrlQ", got)
	}
	if got := host.injectedKeys(); len(got) != 1 || got[0] != "F3" {
		t.Fatalf("injected %v, want only the keys queued before exit", got)
	}
}

func TestMacroSeesArea(t *testing.T) {
	host := newFakeMacroHost()
	host.area = "Editor"
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Editor"; key = "CtrlR"; action = function()
			current = Area.Current
			in_editor = Area.Editor
			in_shell = Area.Shell
		end }
	`)

	fireMacro(t, engine, "CtrlR")

	values := macroGlobals(t, engine, "current", "in_editor", "in_shell")
	if got := lua.LVAsString(values["current"]); got != "Editor" {
		t.Errorf("Area.Current = %q, want Editor", got)
	}
	if values["in_editor"] != lua.LTrue {
		t.Error("Area.Editor was not true in the Editor area")
	}
	if values["in_shell"] != lua.LFalse {
		t.Error("Area.Shell was true in the Editor area")
	}
}

func TestMacroSeesPanelsAndCommandLine(t *testing.T) {
	host := newFakeMacroHost()
	host.panels[true] = MacroPanelInfo{
		Path: "/home/user", Current: "notes.txt", ItemCount: 12,
		SelCount: 3, CurPos: 4, Left: true, Visible: true,
	}
	host.panels[false] = MacroPanelInfo{Path: "/tmp", Current: "core", ItemCount: 1}
	host.cmdLine = "grep -r foo"

	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlS"; action = function()
			apath = APanel.Path
			acur = APanel.Current
			asel = APanel.SelCount
			aleft = APanel.Left
			ppath = PPanel.Path
			cmd = CmdLine.Value
			cmdempty = CmdLine.Empty
			unknown = APanel.NoSuchField
		end }
	`)

	fireMacro(t, engine, "CtrlS")

	values := macroGlobals(t, engine,
		"apath", "acur", "asel", "aleft", "ppath", "cmd", "cmdempty", "unknown")

	if got := lua.LVAsString(values["apath"]); got != "/home/user" {
		t.Errorf("APanel.Path = %q", got)
	}
	if got := lua.LVAsString(values["acur"]); got != "notes.txt" {
		t.Errorf("APanel.Current = %q", got)
	}
	if got := float64(lua.LVAsNumber(values["asel"])); got != 3 {
		t.Errorf("APanel.SelCount = %v, want 3", got)
	}
	if values["aleft"] != lua.LTrue {
		t.Error("APanel.Left was not true")
	}
	if got := lua.LVAsString(values["ppath"]); got != "/tmp" {
		t.Errorf("PPanel.Path = %q, want the passive panel", got)
	}
	if got := lua.LVAsString(values["cmd"]); got != "grep -r foo" {
		t.Errorf("CmdLine.Value = %q", got)
	}
	if values["cmdempty"] != lua.LFalse {
		t.Error("CmdLine.Empty was true for a non-empty command line")
	}
	if values["unknown"] != lua.LNil {
		t.Error("an unknown panel field did not read as nil")
	}
}

func TestMacroPanelsAreReadAtAccessTime(t *testing.T) {
	host := newFakeMacroHost()
	host.panels[true] = MacroPanelInfo{Current: "before.txt"}

	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlY"; action = function() seen = APanel.Current end }
	`)

	fireMacro(t, engine, "CtrlY")
	if got := lua.LVAsString(macroGlobals(t, engine, "seen")["seen"]); got != "before.txt" {
		t.Fatalf("APanel.Current = %q, want before.txt", got)
	}

	// Far's panel tables are live: a second run must see the new state, not a
	// snapshot taken when the table was built.
	host.mu.Lock()
	host.panels[true] = MacroPanelInfo{Current: "after.txt"}
	host.mu.Unlock()

	fireMacro(t, engine, "CtrlY")
	if got := lua.LVAsString(macroGlobals(t, engine, "seen")["seen"]); got != "after.txt" {
		t.Fatalf("APanel.Current = %q, want after.txt", got)
	}
}

func TestMacroMsgBox(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlT"; action = function()
			msgbox("body", "Title")
		end }
	`)

	fireMacro(t, engine, "CtrlT")

	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.messages) != 1 || host.messages[0] != "Title|body" {
		t.Fatalf("messages = %v", host.messages)
	}
}

func TestMacroStringHelpers(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), "")

	cases := []struct {
		expression string
		want       string
	}{
		{`mf.substr("Hello world", 6)`, "world"},
		{`mf.substr("Hello world", 0, 5)`, "Hello"},
		{`mf.substr("Hello", 10)`, ""},
		{`tostring(mf.index("Hello", "llo"))`, "2"},
		{`tostring(mf.index("Hello", "zzz"))`, "-1"},
		{`tostring(mf.rindex("abcabc", "b"))`, "4"},
		{`mf.lcase("ABC")`, "abc"},
		{`mf.ucase("abc")`, "ABC"},
		{`mf.trim("  x  ")`, "x"},
		{`mf.replace("a-b-c", "-", "+")`, "a+b+c"},
		{`mf.iif(1 == 1, "yes", "no")`, "yes"},
		{`mf.iif(false, "yes", "no")`, "no"},
		{`tostring(mf.len("abcd"))`, "4"},
		{`mf.chr(65)`, "A"},
		{`tostring(mf.asc("A"))`, "65"},
		{`tostring(bit.band(12, 10))`, "8"},
		{`tostring(bit.bor(12, 10))`, "14"},
		{`tostring(bit.bxor(12, 10))`, "6"},
		{`tostring(bit.lshift(1, 4))`, "16"},
		{`tostring(bit.rshift(16, 4))`, "1"},
	}

	for _, tc := range cases {
		var got string
		err := engine.rt.Do(func(L *lua.LState) error {
			if err := L.DoString("__result = " + tc.expression); err != nil {
				return err
			}
			got = lua.LVAsString(L.GetGlobal("__result"))
			return nil
		})
		if err != nil {
			t.Errorf("%s: %v", tc.expression, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expression, got, tc.want)
		}
	}
}

func TestMacroFarTitleReportsCurrentWindowTitle(t *testing.T) {
	host := newFakeMacroHost()
	host.title = "f4 | Panels | Linux ARM64"
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlT"; action = function()
			__title = Far.Title
		end }
	`)

	if !fireMacro(t, engine, "CtrlT") {
		t.Fatal("title macro trigger was not consumed")
	}
	if got := lua.LVAsString(macroGlobals(t, engine, "__title")["__title"]); got != host.title {
		t.Fatalf("Far.Title = %q, want %q", got, host.title)
	}
}

func TestMacroUnsupportedDeclarationsDoNotAbortAFile(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		Event { group = "ExitFAR"; action = function() end }
		MenuItem { description = "something" }
		Macro { area = "Shell"; key = "CtrlU"; action = function() Keys("F7") end }
	`)

	if engine.Count() != 1 {
		t.Fatalf("Count = %d, want 1: an unsupported declaration cost the file its macros", engine.Count())
	}
	fireMacro(t, engine, "CtrlU")
	if got := host.injectedKeys(); len(got) != 1 || got[0] != "F7" {
		t.Fatalf("injected %v, want [F7]", got)
	}
}

func TestMacroFailingActionIsContained(t *testing.T) {
	host := newFakeMacroHost()
	engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlV"; action = function()
			error("boom")
		end }
	`)

	fireMacro(t, engine, "CtrlV")

	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.logs) == 0 {
		t.Fatal("a failing macro was not reported")
	}
}

func TestMacroLoadDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "user")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write(filepath.Join(dir, "a.lua"),
		`Macro { area = "Shell"; key = "CtrlW"; action = function() end }`)
	write(filepath.Join(nested, "b.lua"),
		`Macro { area = "Shell"; key = "CtrlX"; action = function() end }`)
	write(filepath.Join(dir, "notes.txt"), "ignored")
	write(filepath.Join(dir, "broken.lua"), "this is not lua")

	engine := newTestMacroEngine(t, newFakeMacroHost(), "")

	if err := engine.LoadDir(dir); err == nil {
		t.Error("LoadDir did not report the broken file")
	}
	if engine.Count() != 2 {
		t.Fatalf("Count = %d, want 2: a broken file cost the other macros", engine.Count())
	}
	if engine.Find("Shell", "CtrlX") == nil {
		t.Error("macros in a subdirectory were not loaded")
	}
}

func TestMacroLoadDirIgnoresMissingDirectory(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), "")
	if err := engine.LoadDir(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Fatalf("LoadDir on a missing directory returned %v", err)
	}
}

func TestMacro_Remove(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { area = "Shell"; key = "CtrlZ"; action = function() Keys("F5") end }
	`)

	if engine.Count() != 1 {
		t.Fatalf("Count = %d, want 1", engine.Count())
	}
	if engine.Find("Shell", "CtrlZ") == nil {
		t.Fatal("Expected to find CtrlZ macro")
	}

	if !engine.Remove("Shell", "CtrlZ") {
		t.Fatal("Expected Remove to return true")
	}
	if engine.Count() != 0 {
		t.Errorf("Count = %d, want 0 after Remove", engine.Count())
	}
	if engine.Find("Shell", "CtrlZ") != nil {
		t.Error("Expected Find to return nil after Remove")
	}

	if engine.Remove("Shell", "CtrlZ") {
		t.Error("Expected second Remove to return false")
	}
}

func TestMacroExplicitRunReportsBusy(t *testing.T) {
	engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Macro { area = "Shell"; key = "CtrlX"; description = "Busy test";
			action = function() end }
	`)
	engine.running.Store(true)
	defer engine.running.Store(false)
	if engine.Run("Shell", "CtrlX") {
		t.Fatal("Run reported success while the macro engine was busy")
	}
	if engine.RunExact("Shell", "CtrlX") {
		t.Fatal("RunExact reported success while the macro engine was busy")
	}
}
