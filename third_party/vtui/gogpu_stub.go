//go:build freebsd || dragonfly || openbsd || netbsd || illumos || solaris || plan9 || android || !(amd64 || arm64) || vtui_nogogpu

package vtui

import "fmt"

// GogpuRenderer является заглушкой для платформ, где gogpu бекенд отключен,
// и для сборок с тегом vtui_nogogpu. Это необходимо для успешной компиляции
// проверок типов в framemanager.go.
type GogpuRenderer struct{}

func (r *GogpuRenderer) Render(buf, shadow []CharInfo, width, height int, forceRedraw bool) {}
func (r *GogpuRenderer) SetCursor(x, y int, visible bool, shape CursorShape)                {}
func (r *GogpuRenderer) SetPalette(palette *[256]uint32)                                    {}
func (r *GogpuRenderer) SetWindowTitle(title string)                                        {}
func (r *GogpuRenderer) ResizeWindow(cols, rows int)                                        {}
func (r *GogpuRenderer) Flush()                                                             {}

// RunGogpuHost — заглушка функции запуска для платформ без gogpu backend и
// для сборок с тегом vtui_nogogpu.
func RunGogpuHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return fmt.Errorf("gogpu backend is not built into this binary")
}
