package macro

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

var MacroMgr *MacroManager

// MacroManager handles recording, playback and storage of simple keyboard macros.
type MacroManager struct {
	Macros    map[string]map[string][]*vtinput.InputEvent
	Recording bool
	Assigning bool
	Buffer    []*vtinput.InputEvent
	IniPath   string
	StartArea string
	// Lua is the Far-compatible macro engine, present only when the user has
	// macros. Recorded macros keep working either way: this is a second
	// backend, not a replacement.
	Lua *LuaMacroEngine
}

func NewMacroManager(iniPath string) *MacroManager {
	mgr := &MacroManager{
		Macros:  make(map[string]map[string][]*vtinput.InputEvent),
		IniPath: iniPath,
	}
	mgr.Load()
	return mgr
}

// ToggleRecording takes the area rather than deriving it: naming the current
// area means naming the view types, which is the application's knowledge.
func (m *MacroManager) ToggleRecording(area string) bool {
	if m == nil || vtui.FrameManager == nil {
		return false
	}
	if m.Recording {
		m.Recording = false
		vtui.FrameManager.PostTask(func() { m.showAssignDialog() })
	} else {
		m.Recording = true
		m.Buffer = make([]*vtinput.InputEvent, 0)
		m.StartArea = area
		vtui.DebugLog("MACRO: Started recording in area: %s", m.StartArea)
	}
	vtui.FrameManager.Redraw()
	return true
}

// LookupHotkey runs the configured action for e in the current area without
// touching macro-recording or macro-playback state. Frames use it as a
// fallback for synthesized key events that bypass FrameManager.EventFilter —
// notably mouse clicks on the F-key bar, which InjectEvents into the queue
// with is_injected=true and therefore never reach Filter above. Returns true
// when the key is bound (including to "None", which is a deliberate silence).
func (m *MacroManager) showAssignDialog() {
	m.Assigning = true
	frame := NewMacroAssignFrame(m)
	vtui.FrameManager.Push(frame)
}

func (m *MacroManager) Load() {
	vtui.DebugLog("MACRO: Loading macros from %s", m.IniPath)
	newMacros := make(map[string]map[string][]*vtinput.InputEvent)
	ini := ini.Load(m.IniPath)
	for sectionName, sec := range ini.Sections() {
		if strings.HasPrefix(sectionName, "KeyMacros/") {
			parts := strings.SplitN(sectionName, "/", 3)
			if len(parts) == 3 {
				area := parts[1]
				hotkey := parts[2]
				seqStr := sec["Sequence"]
				if seqStr == "" {
					continue
				}

				var events []*vtinput.InputEvent
				for _, keyStr := range strings.Fields(seqStr) {
					if strings.HasPrefix(keyStr, "callplugin") || strings.HasPrefix(keyStr, "eval") || strings.Contains(keyStr, "(") {
						continue // Skip far2l macro functions for now
					}
					events = append(events, keymap.ParseFarKey(keyStr))
				}

				if newMacros[area] == nil {
					newMacros[area] = make(map[string][]*vtinput.InputEvent)
				}
				newMacros[area][hotkey] = events
			}
		} else {
			targetArea := sectionName
			if sectionName == "Macros" {
				targetArea = "Common" // Migration from legacy format
			}
			if newMacros[targetArea] == nil {
				newMacros[targetArea] = make(map[string][]*vtinput.InputEvent)
			}
			for key, val := range sec {
				if strings.Contains(val, ":") && !strings.Contains(val, " ") {
					var events []*vtinput.InputEvent
					for _, p := range strings.Split(val, ",") {
						fields := strings.Split(p, ":")
						if len(fields) == 3 {
							charValue, charErr := strconv.ParseInt(fields[0], 10, 32)
							vkValue, vkErr := strconv.ParseUint(fields[1], 10, 16)
							modsValue, modsErr := strconv.ParseUint(fields[2], 10, 32)
							if charErr != nil || vkErr != nil || modsErr != nil {
								continue
							}
							// #nosec G115 -- ParseInt with bitSize 32 bounds the conversion; ValidRune rejects negative and surrogate values.
							char := rune(charValue)
							if char != 0 && !utf8.ValidRune(char) {
								continue
							}
							// #nosec G115 -- ParseUint with bitSize 16 bounds the virtual key code.
							vk := uint16(vkValue)
							// #nosec G115 -- ParseUint with bitSize 32 bounds the control-state bit field.
							mods := vtinput.ControlKeyState(uint32(modsValue))
							events = append(events, &vtinput.InputEvent{
								Type:            vtinput.KeyEventType,
								KeyDown:         true,
								Char:            char,
								VirtualKeyCode:  vk,
								ControlKeyState: mods,
							})
						}
					}
					cleanKey := strings.ReplaceAll(key, "+", "")
					newMacros[targetArea][cleanKey] = events
				}
			}
		}
	}
	m.Macros = newMacros
}

func (m *MacroManager) Save() {
	vtui.DebugLog("MACRO: Saving macros to %s", m.IniPath)

	var sb strings.Builder
	for area, areaMacros := range m.Macros {
		for hotkey, seq := range areaMacros {
			if len(seq) == 0 {
				continue
			}
			fmt.Fprintf(&sb, "[KeyMacros/%s/%s]\n", area, hotkey)
			sb.WriteString("DisableOutput=0x1\n")

			var parts []string
			for _, e := range seq {
				parts = append(parts, keymap.EventToFarString(e))
			}
			fmt.Fprintf(&sb, "Sequence=%s\n\n", strings.Join(parts, " "))
		}
	}

	if err := os.MkdirAll(filepath.Dir(m.IniPath), 0700); err != nil {
		vtui.DebugLog("MACRO: Failed to create macro directory: %v", err)
		return
	}
	err := os.WriteFile(m.IniPath, []byte(sb.String()), 0600)
	if err != nil {
		vtui.DebugLog("MACRO: Failed to save: %v", err)
		return
	}
	_ = os.Chmod(m.IniPath, 0600)
}

// MacroAssignFrame is a modal frame that captures a key combination to assign a macro.
type MacroAssignFrame struct {
	*vtui.Window
	Mgr *MacroManager
}

func NewMacroAssignFrame(m *MacroManager) *MacroAssignFrame {
	width, height := 42, 7
	base := vtui.NewCenteredDialog(width, height, i18n.Msg("Macro.AssignTitle"))
	f := &MacroAssignFrame{
		Window: base,
		Mgr:    m,
	}

	prompt := vtui.NewText(0, 0, i18n.Msg("Macro.AssignPrompt"), vtui.Palette[vtui.ColDialogText])
	f.AddItem(prompt)

	cancelPrompt := vtui.NewText(0, 0, i18n.Msg("Macro.AssignCancel"), vtui.Palette[vtui.ColDialogText])
	f.AddItem(cancelPrompt)

	vbox := vtui.NewVBoxLayout(f.X1+2, f.Y1+2, width-4, height-4)
	vbox.Add(prompt, vtui.Margins{}, vtui.AlignCenter)
	vbox.Add(cancelPrompt, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Apply()

	return f
}

func (f *MacroAssignFrame) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return f.Window.ProcessKey(e)
	}

	if !e.KeyDown {
		return false
	}

	if e.VirtualKeyCode == vtinput.VK_ESCAPE {
		f.Mgr.Buffer = nil
		f.SetExitCode(-1)
		vtui.FrameManager.Redraw()
		return true
	}

	// Only ignore "pure" modifiers without any other key.
	// Everything else (including Esc and Alt-combos) can be a macro.
	switch e.VirtualKeyCode {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return false
	}

	key := keymap.EventToFarString(e)
	if f.Mgr.Macros == nil {
		f.Mgr.Macros = make(map[string]map[string][]*vtinput.InputEvent)
	}

	area := f.Mgr.StartArea
	if area == "" {
		area = "Common"
	}
	if f.Mgr.Macros[area] == nil {
		f.Mgr.Macros[area] = make(map[string][]*vtinput.InputEvent)
	}

	keyDesc := key
	var msg string

	removedIni := false
	if f.Mgr.Macros[area] != nil {
		if _, exists := f.Mgr.Macros[area][key]; exists {
			delete(f.Mgr.Macros[area], key)
			removedIni = true
		}
	}
	if !removedIni && f.Mgr.Macros["Common"] != nil {
		if _, exists := f.Mgr.Macros["Common"][key]; exists {
			delete(f.Mgr.Macros["Common"], key)
			removedIni = true
			area = "Common"
		}
	}

	removedLua := false
	if f.Mgr.Lua != nil && f.Mgr.Lua.Remove(area, key) {
		scriptDir := filepath.Join(config.GetF4ConfigDir(), "Macros", "scripts")
		scriptPath := filepath.Join(scriptDir, RecordedMacroFileName(area, key))
		// Best effort: the macro is already gone from the engine, and a script
		// left on disk is reloaded into nothing.
		_ = os.Remove(scriptPath)
		removedLua = true
	}

	if len(f.Mgr.Buffer) == 0 {
		if removedIni || removedLua {
			msg = fmt.Sprintf("Macro removed from key:\n%s\nArea: %s", keyDesc, area)
		} else {
			msg = fmt.Sprintf("No macro found for key:\n%s", keyDesc)
		}
		if removedIni {
			f.Mgr.Save()
		}
	} else {
		if removedIni {
			f.Mgr.Save()
		}
		if config.App.MacroRecordFormat == 1 {
			scriptDir := filepath.Join(config.GetF4ConfigDir(), "Macros", "scripts")
			err := f.Mgr.SaveRecordedMacro(scriptDir, area, key, "", f.Mgr.Buffer)
			if err != nil {
				msg = fmt.Sprintf("Failed to save Lua macro:\n%v", err)
			} else {
				msg = fmt.Sprintf("Lua macro assigned to key:\n%s\nArea: %s", keyDesc, area)
			}
		} else {
			if f.Mgr.Macros[area] == nil {
				f.Mgr.Macros[area] = make(map[string][]*vtinput.InputEvent)
			}
			f.Mgr.Macros[area][key] = f.Mgr.Buffer
			f.Mgr.Save()
			msg = fmt.Sprintf("Macro assigned to key:\n%s\nArea: %s", keyDesc, area)
		}
	}

	f.Mgr.Buffer = nil
	f.SetExitCode(0)

	vtui.FrameManager.PostTask(func() {
		vtui.ShowMessage(" Macro ", msg, []string{"&Ok"})
	})

	vtui.FrameManager.Redraw()
	return true
}

func (f *MacroAssignFrame) ProcessMouse(e *vtinput.InputEvent) bool {
	return true // Block clicks from falling through
}
func (f *MacroAssignFrame) GetType() vtui.FrameType { return vtui.TypeDialog }
func (f *MacroAssignFrame) IsModal() bool           { return true }
func (f *MacroAssignFrame) GetTitle() string        { return "Macro Assign" }

func (f *MacroAssignFrame) SetExitCode(code int) {
	f.Mgr.Assigning = false
	f.Window.SetExitCode(code)
}
