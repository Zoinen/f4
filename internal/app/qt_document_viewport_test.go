package app

import (
	"context"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withNativeDocumentNegotiation(t *testing.T) {
	t.Helper()
	presentation := config.App.GuiPresentation
	restore := semantic.BeginNativeDocumentViewport(true)
	config.App.GuiPresentation = config.GuiPresentationGUI
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(func() { restore(); config.App.GuiPresentation = presentation })
}

func reportDocumentGeometry(columns, rows int, revision uint64) {
	HandleSemanticAction(map[string]any{"target": "app", "action": "document.viewport",
		"scope": "standalone", "columns": columns, "rows": rows, "geometryRevision": revision})
}

func TestDocumentGeometryFirstEditorWindowAndRepeatedOpen(t *testing.T) {
	withNativeDocumentNegotiation(t)
	for _, columns := range []int{12, 100} {
		reportDocumentGeometry(columns, 17, uint64(columns))
		for attempt := 0; attempt < 2; attempt++ {
			editor := editor.NewEditorView(piecetable.New([]byte(strings.Repeat("x", 240))), nil, "wide.txt")
			editor.WordWrap = true
			semantic.SeedNativeDocumentViewport(editor)
			editor.SetPosition(0, 0, 39, 8)
			node := editor.SemanticNode(nil)
			rows := semantic.AppMapSlice(node["windowRows"])
			if semantic.Bool(node["layoutPending"]) || semantic.Int(node["viewportColumns"]) != columns ||
				semantic.Int(node["viewportRows"]) != 17 || len(rows) == 0 || semantic.AppInt64(rows[0]["endOffset"]) != int64(columns) {
				t.Fatalf("attempt %d: first window does not use %dx17: pending=%v columns=%v rows=%v fragments=%v",
					attempt, columns, node["layoutPending"], node["viewportColumns"], node["viewportRows"], len(rows))
			}
			editor.Close()
		}
	}
}

func TestDocumentGeometryFirstViewerWindowAndStartupPending(t *testing.T) {
	withNativeDocumentNegotiation(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 300)), 0600); err != nil {
		t.Fatal(err)
	}
	viewer, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(dir), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.WrapMode = true
	viewer.SetPosition(0, 0, 39, 8)
	pending := viewer.SemanticNode(nil)
	if !semantic.Bool(pending["layoutPending"]) || len(semantic.AppMapSlice(pending["windowRows"])) != 0 {
		t.Fatal("startup published console-width text before native measurement")
	}
	reportDocumentGeometry(95, 16, 1)
	semantic.SeedNativeDocumentViewport(viewer)
	node := viewer.SemanticNode(nil)
	rows := semantic.AppMapSlice(node["windowRows"])
	if semantic.Bool(node["layoutPending"]) || len(rows) == 0 || semantic.AppInt64(rows[0]["endOffset"]) != 95 || semantic.Int(node["viewportRows"]) != 16 {
		t.Fatalf("first viewer content has wrong geometry: pending=%v rows=%v", node["layoutPending"], rows)
	}
}
