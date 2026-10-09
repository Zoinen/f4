package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type findFileSceneRenderer struct {
	searchFirstActivationRenderer
	scene map[string]any
}

func (r *findFileSceneRenderer) SetSemanticScene(scene map[string]any) { r.scene = scene }
func (r *findFileSceneRenderer) SetSemanticSceneTransition(ctx *vtui.SemanticContext) bool {
	projection, ok := nativeui.BuildAppIncrementalScene(ctx)
	if !ok {
		return false
	}
	// A direct publication is already visible to Qt; no cell redraw is needed.
	r.scene = projection.Scene
	return true
}

func TestFindFileCompletionPublishesResultsWithoutInput(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	vtui.FrameManager.Push(vtui.NewDesktop())
	renderer := &findFileSceneRenderer{}
	screen.Renderer = renderer
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "match.txt"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	ExecuteFindFile(nil, vfs.NewOSVFS(dir), dir, "*.txt", "needle", FindFileOptions{})
	vtui.FrameManager.Redraw()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		vtui.FrameManager.Step(time.Millisecond)
		for _, dialog := range semantic.AppMapSlice(renderer.scene["dialogs"]) {
			if strings.TrimSpace(semantic.String(dialog["title"])) == strings.TrimSpace(i18n.Msg("FindFile.SearchResultsTitle")) {
				return
			}
		}
	}
	t.Fatalf("search completed with top frame %T, but published dialogs are %#v", vtui.FrameManager.GetTopFrame(), renderer.scene["dialogs"])
}
