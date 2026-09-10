package semantic

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// NativeDocumentGeometry is negotiated presentation state, independent of the
// backing document. Revisions fence delayed geometry from an older Qt layout.
type NativeDocumentGeometry struct {
	Columns, Rows int
	Revision      uint64
}

// NativeDocument owns the reflow and scroll state affected by a viewport change.
// The protocol never reaches into an editor's or viewer's private fields.
type NativeDocument interface {
	ApplyNativeViewport(NativeDocumentGeometry)
}

var nativeDocumentViewport struct {
	enabled  bool
	geometry NativeDocumentGeometry
}

// BeginNativeDocumentViewport scopes negotiation to one frontend connection.
func BeginNativeDocumentViewport(enabled bool) func() {
	previous := nativeDocumentViewport
	nativeDocumentViewport.enabled = enabled
	nativeDocumentViewport.geometry = NativeDocumentGeometry{}
	return func() { nativeDocumentViewport = previous }
}

func ApplyNativeDocumentViewport(document any, geometry NativeDocumentGeometry) {
	if view, ok := document.(NativeDocument); ok {
		view.ApplyNativeViewport(geometry)
	}
}

func SeedNativeDocumentViewport(document any) {
	if nativeDocumentViewport.enabled && config.App.GuiPresentation != config.GuiPresentationText && nativeDocumentViewport.geometry.Revision != 0 {
		ApplyNativeDocumentViewport(document, nativeDocumentViewport.geometry)
	}
}

func NativeDocumentLayoutPending(revision uint64) bool {
	return nativeDocumentViewport.enabled && config.App.GuiPresentation != config.GuiPresentationText && revision == 0
}

func HandleStandaloneDocumentViewport(action map[string]any) bool {
	if String(action["action"]) != "document.viewport" || String(action["scope"]) != "standalone" {
		return false
	}
	if !nativeDocumentViewport.enabled {
		return true
	}
	columns, rows := Int(action["columns"]), Int(action["rows"])
	revision := Int64(action["geometryRevision"])
	if columns <= 0 || columns > 16384 || rows <= 0 || rows > 4096 || revision <= 0 || uint64(revision) <= nativeDocumentViewport.geometry.Revision {
		return true
	}
	g := NativeDocumentGeometry{Columns: columns, Rows: rows, Revision: uint64(revision)}
	nativeDocumentViewport.geometry = g
	if vtui.FrameManager != nil && config.App.GuiPresentation != config.GuiPresentationText {
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				ApplyNativeDocumentViewport(frame, g)
			}
		}
	}
	return true
}

// NativeDocumentCellPaintOwned suppresses only the hidden compatibility surface.
// Offscreen styling and unsupported/modal stacks continue to paint normally.
func NativeDocumentCellPaintOwned(scr *vtui.ScreenBuf) bool {
	if !nativeDocumentViewport.enabled || config.App.GuiPresentation == config.GuiPresentationText || vtui.FrameManager == nil || scr != vtui.FrameManager.Screen() {
		return false
	}
	renderer, ok := scr.Renderer.(interface{ NativeSemanticSurfaceActive() bool })
	if !ok || !renderer.NativeSemanticSurfaceActive() {
		return false
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if _, ok := frame.(NativeDocument); !ok && frame.GetType() != vtui.TypeDesktop {
			return false
		}
	}
	return true
}

func TargetedDocumentGeometry(action map[string]any, columns int) NativeDocumentGeometry {
	if raw, exists := action["columns"]; exists {
		columns = max(0, min(16384, Int(raw)))
	}
	return NativeDocumentGeometry{
		Columns: columns, Rows: max(0, min(4096, Int(action["rows"]))),
		Revision: uint64(max(int64(0), Int64(action["geometryRevision"]))),
	}
}

func SemanticDocumentLayoutMatches(action map[string]any, key string, revision uint64) bool {
	if incoming := String(action["documentKey"]); incoming != "" && incoming != key {
		return false
	}
	if raw, exists := action["layoutRevision"]; exists {
		return Int64(raw) == int64(revision)
	}
	return true
}
