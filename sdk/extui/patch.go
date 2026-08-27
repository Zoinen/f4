package extui

// MapPatch updates a bounded semantic object. Missing keys are unchanged;
// Clear removes optional keys explicitly, so nil never has ambiguous meaning
// on the MessagePack wire.
type MapPatch struct {
	Set   M
	Clear []string
}

func (p MapPatch) ToMap() M {
	out := M{}
	if len(p.Set) > 0 {
		out["set"] = p.Set
	}
	if len(p.Clear) > 0 {
		out["clear"] = append([]string(nil), p.Clear...)
	}
	return out
}

// PanelPatch is the only scene-patch location allowed to carry a file catalog.
// State updates deliberately carry a row-free panel map.
type PanelPatch struct {
	Op                  string
	Side                int
	PanelID             string
	CatalogRevision     int64
	BaseCatalogRevision int64
	Panel               M
	State               M
	BaseSelection       int64
	SelectionRevision   int64
	SelectionChanges    []M
	SelectedEntryIDs    []string
}

func (p PanelPatch) ToMap() M {
	out := M{"op": p.Op, "side": p.Side}
	if p.PanelID != "" {
		out["panelId"] = p.PanelID
	}
	if p.CatalogRevision != 0 || p.Op == "catalog_replace" {
		out["catalogRevision"] = p.CatalogRevision
	}
	if p.BaseCatalogRevision != 0 || p.Op == "catalog_replace" {
		out["baseCatalogRevision"] = p.BaseCatalogRevision
	}
	if p.Panel != nil {
		out["panel"] = p.Panel
	}
	if p.State != nil {
		out["state"] = p.State
	}
	if p.Op == "selection_delta" || p.Op == "selection_replace" {
		out["baseSelectionRevision"] = p.BaseSelection
		out["selectionRevision"] = p.SelectionRevision
	}
	if len(p.SelectionChanges) > 0 {
		out["changes"] = append([]M(nil), p.SelectionChanges...)
	}
	if p.SelectedEntryIDs != nil {
		out["selectedEntryIds"] = append([]string(nil), p.SelectedEntryIDs...)
	}
	return out
}

type ShellPatch struct {
	MapPatch
	Panels []PanelPatch
}

func (p ShellPatch) ToMap() M {
	out := p.MapPatch.ToMap()
	if len(p.Panels) > 0 {
		panels := make([]M, 0, len(p.Panels))
		for _, panel := range p.Panels {
			panels = append(panels, panel.ToMap())
		}
		out["panels"] = panels
	}
	return out
}

// ScenePatch advances one exact app-scene revision. Revision gaps are protocol
// errors; the Qt host never guesses or silently requests a full replacement.
type ScenePatch struct {
	BaseRevision uint64
	Revision     uint64
	Root         *MapPatch
	Shell        *ShellPatch
}

func (p ScenePatch) ToMap() M {
	out := M{
		"type":         "scene_patch",
		"schema":       Schema,
		"version":      SceneVersion,
		"baseRevision": p.BaseRevision,
		"revision":     p.Revision,
	}
	if p.Root != nil {
		out["root"] = p.Root.ToMap()
	}
	if p.Shell != nil {
		out["shell"] = p.Shell.ToMap()
	}
	return out
}
