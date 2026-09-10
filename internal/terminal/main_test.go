package terminal

import (
	"bytes"
	"errors"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
	"image/png"
	"os"
	"testing"
)

// testApplication answers the few questions the terminal asks upward, with the
// smallest thing that is still true. The session server's methods are never
// reached: no test in this package attaches a client.
type testApplication struct{}

func (testApplication) InitCore() *vtui.ScreenBuf     { return vtui.NewSilentScreenBuf() }
func (testApplication) InstallImageOverlay()          {}
func (testApplication) OpenEditFile()                 {}
func (testApplication) ClientAttached(_, _, _ string) {}
func (testApplication) ClientDetached()               {}
func (testApplication) VersionInfo() string           { return "f4-test" }
func (testApplication) EditFilePath() string          { return "" }
func (testApplication) StartupDirs() (string, string) { return "", "" }

// DecodeImage handles PNG only. The application routes several formats here;
// the kitty protocol tests transmit PNG, and a decoder that silently accepts
// more than the test sends would hide a format the terminal cannot show.
func (testApplication) DecodeImage(data []byte) (*vtui.ImageSurface, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	surf := vtui.NewImageSurfaceFromImage(img)
	if surf == nil {
		return nil, errors.New("unsupported image geometry")
	}
	return surf, nil
}

func TestMain(m *testing.M) {
	App = testApplication{}
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
