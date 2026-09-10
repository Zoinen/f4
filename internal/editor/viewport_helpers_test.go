package editor

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
	"testing"
)

func withNativeDocumentNegotiation(t *testing.T) {
	previous := config.App.GuiPresentation
	restore := semantic.BeginNativeDocumentViewport(true)
	config.App.GuiPresentation = config.GuiPresentationGUI
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(func() { restore(); config.App.GuiPresentation = previous })
}
func reportDocumentGeometry(columns, rows int, revision uint64) {
	semantic.HandleStandaloneDocumentViewport(map[string]any{"target": "app", "action": "document.viewport", "scope": "standalone", "columns": columns, "rows": rows, "geometryRevision": revision})
}
