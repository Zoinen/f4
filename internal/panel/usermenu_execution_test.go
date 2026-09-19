package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestUserMenuExecutesFromPanelFocus(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	for _, mode := range []config.PanelNavigationMode{config.NavigationClassic, config.NavigationSearchFirst} {
		for _, commands := range [][]string{{"echo F4_MENU_EXECUTION_TEST"}, {"cd .", "echo F4_MENU_EXECUTION_TEST"}} {
			config.App.NavigationMode = mode
			config.App.SearchCommandStayFocused = false
			pf := setupMockPanelsFrame(t)
			pf.ResizeConsole(80, 25)
			pf.SetCommandLineFocus(false)
			for _, p := range pf.Panels {
				p.(*FileSystemPanel).Entries = nil
			}
			backend := pf.Pty.(*mockPty)
			if !ExecuteMenuCommandsWithResult(pf, commands) {
				t.Fatal("menu action was not dispatched")
			}
			if !strings.Contains(backend.String(), "F4_MENU_EXECUTION_TEST") {
				t.Errorf("mode %v commands %v: command stayed in prompt %q instead of reaching PTY", mode, commands, pf.CmdLine.Edit.GetText())
			}
			if !pf.CmdLine.IsEmpty() {
				t.Errorf("submitted command remains in prompt: %q", pf.CmdLine.Edit.GetText())
			}
			pf.Close()
		}
	}
}
