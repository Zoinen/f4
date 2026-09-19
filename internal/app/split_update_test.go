package app

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

type splitUpdateRenderer struct {
	searchFirstActivationRenderer
	renders, exports, skipped int
}

func (r *splitUpdateRenderer) Render([]vtui.CharInfo, []vtui.CharInfo, int, int, bool) { r.renders++ }
func (r *splitUpdateRenderer) SetSemanticScene(map[string]any)                         { r.exports++ }
func (r *splitUpdateRenderer) BeginSemanticSceneUpdate()                               {}
func (r *splitUpdateRenderer) EndSemanticSceneUpdate()                                 {}
func (r *splitUpdateRenderer) EndSemanticSceneUpdateUnchanged() bool                   { r.skipped++; return true }

func TestSplitUpdatePostedActionSkipsRedraw(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.AutoSaveSettings = false
	pf, _, _ := newSearchFirstTestFrame(t)
	pf.MenuBar = vtui.NewMenuBar(nil)
	pf.KeyBar = vtui.NewKeyBar()
	vtui.FrameManager.Screen().AllocBuf(80, 25)
	vtui.FrameManager.Push(pf)
	renderer := &splitUpdateRenderer{}
	vtui.FrameManager.Screen().Renderer = renderer
	for len(vtui.FrameManager.RedrawChan) > 0 {
		<-vtui.FrameManager.RedrawChan
	}
	vtui.FrameManager.Step(0)
	renderer.renders, renderer.exports, renderer.skipped = 0, 0, 0
	done := false
	vtui.FrameManager.PostPriorityTask(func() {
		if !HandleSemanticAction(map[string]any{"action": "panel.setSplit",
			"target": vtui.SemanticID(pf), "ratioMillionths": 600000}) {
			t.Error("split update rejected")
		}
		done = true
	})
	deadline := time.Now().Add(time.Second)
	for !done && time.Now().Before(deadline) {
		vtui.FrameManager.Step(time.Millisecond)
	}
	if !done || pf.WidthDecrement != -8 {
		t.Fatal("Go did not receive the split")
	}
	if renderer.renders != 0 || renderer.exports != 0 || renderer.skipped == 0 {
		t.Fatalf("split caused rendering: renders=%d exports=%d skips=%d", renderer.renders, renderer.exports, renderer.skipped)
	}
	if len(vtui.FrameManager.RedrawChan) != 0 {
		t.Fatal("split queued a later redraw")
	}
}
