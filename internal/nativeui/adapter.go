// Package nativeui assembles frontend models from the interactive subsystems.
// Those subsystems own their geometry and content; the external-UI transport
// owns serialization, delivery and revision ordering.
package nativeui

import (
	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

type Adapter struct{}

func (Adapter) Options() plughost.ExternalUIOptions {
	return plughost.ExternalUIOptions{FontFamily: gui.EffectiveGuiFont(), WorktreeBranch: gui.CurrentWorktreeBranchName()}
}
func (Adapter) MenuState(ctx *vtui.SemanticContext) (map[string]any, bool) {
	return BuildAppMenuState(ctx)
}
func (Adapter) ProjectScene(ctx *vtui.SemanticContext) (plughost.SceneProjection, bool) {
	return BuildAppIncrementalScene(ctx)
}
func (Adapter) PrepareDrag(action map[string]any) map[string]any {
	return panel.PrepareSemanticDrag(action)
}
func (Adapter) CatalogRows(id, path string, revision int64, offset, limit int) (map[string]any, bool) {
	return panel.BuildLivePanelCatalogRows(id, path, revision, offset, limit)
}
func (Adapter) CatalogRowsRetryable(id, path string, revision int64) bool {
	return panel.LivePanelCatalogRowsRetryable(id, path, revision)
}
func (Adapter) LiveCatalogMetadata(id, path string, catalog, metadata int64, offset, limit int) (map[string]any, bool) {
	return panel.BuildLivePanelCatalogMetadataChunk(id, path, catalog, metadata, offset, limit)
}
func (Adapter) CatalogMetadata(id, path string, catalog, metadata int64, offset, limit int) (map[string]any, bool) {
	return panel.BuildPanelCatalogMetadataChunk(id, path, catalog, metadata, offset, limit)
}

func (s *appIncrementalScene) Snapshot() map[string]any { return s.Scene }
func (s *appIncrementalScene) BuildPatch(previous map[string]any) (extui.ScenePatch, func(), bool) {
	patch, acknowledgements, valid := BuildAppScenePatch(previous, s)
	return patch, func() {
		for _, acknowledgement := range acknowledgements {
			acknowledgement.Panel.AcknowledgeSemanticSelection(acknowledgement.Revision)
		}
	}, valid
}

var _ plughost.PresentationAdapter = Adapter{}
