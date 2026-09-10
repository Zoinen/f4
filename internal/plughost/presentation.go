package plughost

import (
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// SceneProjection is an immutable projection plus an acknowledgement owned by
// the presenting subsystem. The transport commits it only after accepting the
// matching scene revision; it never retains a live panel or editor pointer.
type SceneProjection interface {
	Snapshot() map[string]any
	BuildPatch(previous map[string]any) (extui.ScenePatch, func(), bool)
}

type ExternalUIOptions struct{ FontFamily, WorktreeBranch string }

// PresentationAdapter is supplied by the composition root. Protocol handling
// remains independent of the interactive packages and their mutable state.
type PresentationAdapter interface {
	Options() ExternalUIOptions
	MenuState(*vtui.SemanticContext) (map[string]any, bool)
	ProjectScene(*vtui.SemanticContext) (SceneProjection, bool)
	PrepareDrag(map[string]any) map[string]any
	CatalogRows(string, string, int64, int, int) (map[string]any, bool)
	CatalogRowsRetryable(string, string, int64) bool
	LiveCatalogMetadata(string, string, int64, int64, int, int) (map[string]any, bool)
	CatalogMetadata(string, string, int64, int64, int, int) (map[string]any, bool)
}

var Presentation PresentationAdapter = emptyPresentation{}

type emptyPresentation struct{}

func (emptyPresentation) Options() ExternalUIOptions                             { return ExternalUIOptions{} }
func (emptyPresentation) MenuState(*vtui.SemanticContext) (map[string]any, bool) { return nil, false }
func (emptyPresentation) ProjectScene(*vtui.SemanticContext) (SceneProjection, bool) {
	return nil, false
}
func (emptyPresentation) PrepareDrag(map[string]any) map[string]any { return nil }
func (emptyPresentation) CatalogRows(string, string, int64, int, int) (map[string]any, bool) {
	return nil, false
}
func (emptyPresentation) CatalogRowsRetryable(string, string, int64) bool { return false }
func (emptyPresentation) LiveCatalogMetadata(string, string, int64, int64, int, int) (map[string]any, bool) {
	return nil, false
}
func (emptyPresentation) CatalogMetadata(string, string, int64, int64, int, int) (map[string]any, bool) {
	return nil, false
}
