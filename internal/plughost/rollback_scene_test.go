package plughost

import (
	"github.com/unxed/f4/internal/semantic"
)

func rollbackCatalogScene(path string, catalog, metadata int64, loading bool, name string) map[string]any {
	scene := panelActivationFastPathScene(0, "Panels: "+path)
	panels, _ := semantic.SemanticScenePanelMaps(scene)
	for _, panel := range panels {
		panel["path"] = path
		panel["catalogRevision"] = catalog
		panel["metadataRevision"] = metadata
		panel["loading"] = loading
		panel["entries"] = []map[string]any{{"id": path + "/" + name, "name": name}}
	}
	return scene
}
