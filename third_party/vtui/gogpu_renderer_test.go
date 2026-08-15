//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris

package vtui

import (
	"testing"
)

func TestGogpuRenderer_CursorDirtyOnStateChange(t *testing.T) {
	r := NewGogpuRenderer(nil, nil, 8, 16)
	r.dirty = false

	// Смена позиции курсора должна взводить флаг dirty для обхода раннего выхода
	r.SetCursor(5, 5, true, CursorShapeUnderline)
	if !r.dirty {
		t.Error("GogpuRenderer: expected dirty to be true after cursor position change")
	}
	r.dirty = false

	// Смена формы курсора (Ins/Ovr) также должна помечать буфер грязным
	r.SetCursor(5, 5, true, CursorShapeBlock)
	if !r.dirty {
		t.Error("GogpuRenderer: expected dirty to be true after cursor shape change")
	}
}
func TestGogpuRenderer_Flush(t *testing.T) {
	host := &GogpuHost{}
	host.resizePending = true

	r := NewGogpuRenderer(host, nil, 8, 16)
	r.dirty = false

	r.Flush()

	// After Flush:
	// 1. host.resizePending should be false
	if host.resizePending {
		t.Error("GogpuRenderer.Flush: expected host.resizePending to be false")
	}

	// 2. r.dirty should be true (because of forceDirty/resizePending)
	r.mu.Lock()
	dirty := r.dirty
	r.mu.Unlock()
	if !dirty {
		t.Error("GogpuRenderer.Flush: expected r.dirty to be true because of resizePending")
	}
}

func TestGogpuRenderer_CursorShapeState(t *testing.T) {
	host := &GogpuHost{}
	r := NewGogpuRenderer(host, nil, 8, 16)

	r.SetCursor(1, 2, true, CursorShapeBlock)
	if r.cursorShape != CursorShapeBlock {
		t.Errorf("Expected CursorShapeBlock, got %v", r.cursorShape)
	}

	r.SetCursor(1, 2, true, CursorShapeUnderline)
	if r.cursorShape != CursorShapeUnderline {
		t.Errorf("Expected CursorShapeUnderline, got %v", r.cursorShape)
	}
}
