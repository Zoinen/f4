//go:build extralite

package macro

import (
	"errors"
	"time"

	"github.com/unxed/vtinput"
)

// The extra-lite profile (unxed/f4#1671) carries no Lua interpreter, so there
// are no Lua macros: the engine below has the same surface as the real one
// (lua.go) and never starts. Recorded keyboard macros are not affected.

// ErrLuaMacrosUnavailable is what NewLuaMacroEngine returns in this build.
var ErrLuaMacrosUnavailable = errors.New("Lua macros are not available in the extra-lite build")

// LuaMacro is one macro declaration; there are none in this build.
type LuaMacro struct {
	Areas            []string
	Keys             []string
	EmptyCommandLine bool
	Description      string
	Source           string
}

// LuaMacroEngine is the placeholder of the Lua macro engine.
type LuaMacroEngine struct{}

// NewLuaMacroEngine reports ErrLuaMacrosUnavailable.
func NewLuaMacroEngine(MacroHost) (*LuaMacroEngine, error) {
	return nil, ErrLuaMacrosUnavailable
}

func (e *LuaMacroEngine) LoadDir(string) error                     { return ErrLuaMacrosUnavailable }
func (e *LuaMacroEngine) LoadString(_, _ string) error             { return ErrLuaMacrosUnavailable }
func (e *LuaMacroEngine) Count() int                               { return 0 }
func (e *LuaMacroEngine) Find(_, _ string) *LuaMacro               { return nil }
func (e *LuaMacroEngine) Bindings(string) []LuaMacroBinding        { return nil }
func (e *LuaMacroEngine) Remove(_, _ string) bool                  { return false }
func (e *LuaMacroEngine) Trigger(string, *vtinput.InputEvent) bool { return false }
func (e *LuaMacroEngine) TriggerWithCommandLine(string, *vtinput.InputEvent, string) bool {
	return false
}
func (e *LuaMacroEngine) Run(_, _ string) bool                    { return false }
func (e *LuaMacroEngine) RunExact(_, _ string) bool               { return false }
func (e *LuaMacroEngine) WaitIdle(time.Duration) bool             { return true }
func (e *LuaMacroEngine) Interrupted() bool                       { return false }
func (e *LuaMacroEngine) Close() error                            { return nil }
func (e *LuaMacroEngine) RaiseEvent(string) bool                  { return false }
func (e *LuaMacroEngine) RaiseEventNumbers(string, ...int) bool   { return false }
func (e *LuaMacroEngine) RunEvents(_ string, _ time.Duration) int { return 0 }

func (e *LuaMacroEngine) CommandLinePrefixes() []LuaCommandLineInfo { return nil }
func (e *LuaMacroEngine) RunCommandLine(_ int, _, _ string) bool    { return false }
func (e *LuaMacroEngine) MenuItems(_, _ string) []LuaMenuItemInfo   { return nil }
func (e *LuaMacroEngine) RunMenuItem(_ int, _, _ string) bool       { return false }
