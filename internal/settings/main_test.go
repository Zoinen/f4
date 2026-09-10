package settings

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

type testHost struct{}

func (testHost) SaveSession() error  { return nil }
func (testHost) SessionPath() string { return filepath.Join(config.GetF4ConfigDir(), "session.ini") }
func (testHost) SaveGeometry() error { return nil }
func (testHost) ApplyRuntime(_ config.F4Config, changed []string) {
	for _, id := range changed {
		if id == "ColorStyle" || id == "EnforceColorCorrection" {
			_ = theme.ApplyColorStyle(config.App.ColorStyle)
		}
	}
}
func (testHost) CheckUpdates(context.Context) error                                    { return nil }
func (testHost) GuiBackends() []string                                                 { return []string{"win32", "gogpu", "ebiten", "x11", "wayland"} }
func (testHost) PluginPackage(bool, *panel.PanelsFrame, plughost.PlugRingItem, func()) {}
func InitLang() {
	i18n.InitLang(config.App.Language, config.App.FallbackLanguage, config.LocalLangDir())
}
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, func() {
		config.Executable = os.Executable
		config.CachedF4ConfigDir = testutil.ConfigDir()
		config.ConfigDirOnce.Do(func() {})
		Configure(testHost{})
		theme.SetDefaultF4Palette()
		InitLang()
	}, nil))
}
