package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/luaplug"
	"github.com/unxed/vtinput"
	lua "github.com/yuin/gopher-lua"
)

// macroExitSentinel is what exit() raises. It ends the macro without being an
// error, the way Far's exit() does.
const macroExitSentinel = "f4macro:exit"

// macroCallTimeout bounds one macro. A macro is user code triggered by a key
// press, so it gets far less rope than a plugin.
const macroCallTimeout = 10 * time.Second

// MacroPanelInfo is the panel state a macro can see, gathered in one shot.
// Reading it costs a round trip to the UI goroutine, so it is fetched whole
// rather than field by field.
type MacroPanelInfo struct {
	Path      string
	Current   string
	ItemCount int
	SelCount  int
	CurPos    int
	TopPos    int
	IsFolder  bool
	Empty     bool
	Left      bool
	Visible   bool
	Root      bool
	Bof       bool
	Eof       bool
	Type      int
}

// MacroHost is everything the macro engine needs from f4. Keeping it an
// interface is what makes the engine testable without a terminal, and it is
// also the seam where the "must run on the UI goroutine" rule is enforced
// exactly once instead of in every API function.
type MacroHost interface {
	CurrentArea() string
	Panel(active bool) MacroPanelInfo
	CommandLine() string
	ScreenSize() (width, height int)
	Version() string
	Message(title, text string)
	InjectKeys(keys []*vtinput.InputEvent)
	Log(format string, args ...any)
	RunAction(name string) bool
	CallPlugin(context.Context, string, []any) ([]any, error)
}

// LuaMacro is one Macro{} declaration.
type LuaMacro struct {
	Areas       []string
	Keys        []string
	Description string
	Source      string

	action    *lua.LFunction
	condition *lua.LFunction
}

// LuaMacroEngine runs Far-compatible macros written in Lua.
type LuaMacroEngine struct {
	rt   *luaplug.Runtime
	host MacroHost

	mu     sync.Mutex
	byArea map[string]map[string][]*LuaMacro
	all    []*LuaMacro

	running atomic.Bool

	// The fields below belong to the interpreter's worker goroutine while a
	// macro is running, and are read by the caller once it has finished.
	pendingKeys []*vtinput.InputEvent
	invokedKey  string
}

// macroAreaAliases maps f4's own area names onto Far's. f4 reports Terminal
// when the panels are hidden; Far has no such area, and its Shell macros are
// what a user expects to fire there.
var macroAreaAliases = map[string]string{
	"terminal": "shell",
}

// NewLuaMacroEngine starts an engine with no macros loaded.
func NewLuaMacroEngine(host MacroHost) (*LuaMacroEngine, error) {
	engine := &LuaMacroEngine{
		host:   host,
		byArea: make(map[string]map[string][]*LuaMacro),
	}

	runtime, err := luaplug.New(luaplug.Options{
		Name:        "macros",
		CallTimeout: macroCallTimeout,
		Host: luaplug.HostFunc(func(method string, params any) (any, error) {
			if method == "Host.Log" {
				host.Log("MACRO: %v", params)
			}
			return nil, nil
		}),
	})
	if err != nil {
		return nil, err
	}
	engine.rt = runtime

	if err := runtime.Do(func(L *lua.LState) error {
		engine.installAPI(L)
		return nil
	}); err != nil {
		runtime.Close()
		return nil, err
	}
	return engine, nil
}

// LoadDir loads every .lua file under dir, the way Far reads its Macros
// directory. A file that fails to load is reported and skipped: one broken
// macro must not cost the user all the others.
func (e *LuaMacroEngine) LoadDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var failures int
	walkErr := filepath.Walk(dir, func(path string, entry os.FileInfo, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".lua") {
			return nil
		}
		if loadErr := e.rt.LoadFile(path); loadErr != nil {
			failures++
			e.host.Log("MACRO: failed to load %s: %v", path, loadErr)
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if failures > 0 {
		return fmt.Errorf("%d macro file(s) failed to load", failures)
	}
	return nil
}

// LoadString loads macros from a chunk, which is how tests and the eventual
// macro editor feed the engine.
func (e *LuaMacroEngine) LoadString(name, source string) error {
	return e.rt.LoadString(name, source)
}

// Count reports how many macros are registered.
func (e *LuaMacroEngine) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.all)
}

func (e *LuaMacroEngine) add(m *LuaMacro) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, area := range m.Areas {
		byKey := e.byArea[area]
		if byKey == nil {
			byKey = make(map[string][]*LuaMacro)
			e.byArea[area] = byKey
		}
		for _, key := range m.Keys {
			byKey[key] = append(byKey[key], m)
		}
	}
	e.all = append(e.all, m)
}

// Find returns the macro bound to a key in an area, falling back to common.
// The most recently registered wins, so a user file loaded later can shadow an
// earlier one.
func (e *LuaMacroEngine) Find(area, key string) *LuaMacro {
	e.mu.Lock()
	defer e.mu.Unlock()

	area = strings.ToLower(area)
	if alias, ok := macroAreaAliases[area]; ok {
		area = alias
	}
	key = strings.ToLower(key)

	if list := e.byArea[area][key]; len(list) > 0 {
		return list[len(list)-1]
	}
	if area != "common" {
		if list := e.byArea["common"][key]; len(list) > 0 {
			return list[len(list)-1]
		}
	}
	return nil
}

// Remove drops a macro from the engine.
func (e *LuaMacroEngine) Remove(area, key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	area = strings.ToLower(area)
	if alias, ok := macroAreaAliases[area]; ok {
		area = alias
	}
	key = strings.ToLower(key)

	if e.byArea[area] == nil {
		return false
	}
	list := e.byArea[area][key]
	if len(list) == 0 {
		return false
	}

	macroToRemove := list[len(list)-1]
	e.byArea[area][key] = list[:len(list)-1]

	for i, m := range e.all {
		if m == macroToRemove {
			e.all = append(e.all[:i], e.all[i+1:]...)
			break
		}
	}
	return true
}

// Trigger consumes a key if a macro claims it and starts that macro.
//
// The macro runs on its own goroutine, never on the caller's. The caller is
// the input loop running on the UI goroutine, and a macro that asks for panel
// state or shows a message needs that goroutine to be free to answer.
func (e *LuaMacroEngine) Trigger(area string, event *vtinput.InputEvent) bool {
	if e == nil || event == nil {
		return false
	}
	key := EventToFarString(event)
	macro := e.Find(area, key)
	if macro == nil {
		return false
	}
	if !e.running.CompareAndSwap(false, true) {
		// Far does not nest macro execution either.
		return true
	}

	original := *event
	go func() {
		defer e.running.Store(false)
		e.execute(macro, key, &original)
	}()
	return true
}

// execute runs one macro to completion and flushes whatever keys it queued.
func (e *LuaMacroEngine) execute(macro *LuaMacro, key string, original *vtinput.InputEvent) {
	skipped := false

	err := e.rt.Do(func(L *lua.LState) error {
		e.invokedKey = key
		e.pendingKeys = nil

		if macro.condition != nil {
			L.Push(macro.condition)
			L.Push(lua.LString(key))
			if err := L.PCall(1, 1, nil); err != nil {
				return err
			}
			passed := lua.LVAsBool(L.Get(-1))
			L.Pop(1)
			if !passed {
				skipped = true
				return nil
			}
		}

		L.Push(macro.action)
		if err := L.PCall(0, 0, nil); err != nil {
			if strings.Contains(err.Error(), macroExitSentinel) {
				return nil
			}
			return err
		}
		return nil
	})

	keys := e.pendingKeys
	e.pendingKeys = nil

	if err != nil {
		e.host.Log("MACRO: %s (%s): %v", macro.Description, macro.Source, err)
	}

	if skipped {
		// The key was already consumed on the caller's side, so put it back
		// rather than swallowing a keystroke the macro declined to handle.
		if original != nil {
			e.host.InjectKeys([]*vtinput.InputEvent{original})
		}
		return
	}
	if len(keys) > 0 {
		e.host.InjectKeys(keys)
	}
}

// waitIdle blocks until no macro is running. Tests use it; macro execution is
// asynchronous by design.
func (e *LuaMacroEngine) waitIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !e.running.Load() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return !e.running.Load()
}

// Close releases the interpreter.
func (e *LuaMacroEngine) Close() error {
	if e == nil || e.rt == nil {
		return nil
	}
	return e.rt.Close()
}

// parseMacroKeys turns a Far key sequence such as "F5 Enter" into events.
func parseMacroKeys(spec string) []*vtinput.InputEvent {
	var events []*vtinput.InputEvent
	for _, token := range strings.Fields(spec) {
		if event := ParseFarKey(token); event != nil {
			events = append(events, event)
		}
	}
	return events
}

// splitMacroList splits the space separated lists Far uses for area and key.
func splitMacroList(value string) []string {
	fields := strings.Fields(strings.ToLower(value))
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}
