//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import "time"

// glyphKey используется для кэширования отрисованных символов
type glyphKey struct {
	r  rune
	fg uint32
	bg uint32
	w  int
}

// renderStats собирает статистику производительности графического вывода
type renderStats struct {
	frameCount int
	totalDraw  time.Duration
	totalFlush time.Duration
	totalRows  int
	dirtyRows  int
	glyphs     int
	putImages  int
	lastReport time.Time
}
