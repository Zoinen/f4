package editor

import (
	"github.com/unxed/vtui"
)

type editorSurfaceRecorder struct {
	vtui.SurfaceRenderer
	state map[string]any
}

func (r *editorSurfaceRecorder) QueueSurfaceState(_ string, state map[string]any) bool {
	r.state = state
	return true
}
func (r *editorSurfaceRecorder) takePatch() (map[string]any, error) {
	state := r.state
	r.state = nil
	return map[string]any{"type": "scene_patch", "surface": map[string]any{"set": state}}, nil
}
