//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !arm

package vtui

import (
	"os"
	"runtime"
	"testing"
)

func TestLoadGogpuFontUsesMacOSMonaco(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS system font test")
	}
	if _, err := os.Stat("/System/Library/Fonts/Monaco.ttf"); err != nil {
		t.Skip("Monaco system font is not installed")
	}
	face, cellWidth, cellHeight := loadGogpuFont("Monaco", 16)
	if face == nil {
		t.Fatal("Go/GPU failed to load Monaco.ttf")
	}
	if cellWidth <= 0 || cellHeight <= 0 {
		t.Fatalf("invalid Monaco cell size: %dx%d", cellWidth, cellHeight)
	}
}
