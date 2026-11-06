package settings

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func fileGroupDraftThemes(d *f4settings.Draft) []filegroups.ThemeSet {
	result := []filegroups.ThemeSet{}
	for _, r := range d.Records["filegroups.themes"] {
		result = append(result, filegroups.ThemeSet{ID: r.ID, Fields: maps.Clone(r.Values)})
	}
	return result
}

type fileGroupPreview struct {
	item         vfs.VFSItem
	style        extui.HighlightStyleModel
	matched      bool
	contributors string
	attrs        [4]uint64
}

type fileGroupSampleData struct {
	item  vfs.VFSItem
	known bool
}

func fileGroupSample(g filegroups.Group) vfs.VFSItem {
	name := g.Fields["Preview.Name"]
	if name == "" {
		mask := strings.Split(g.Fields["Mask"], "|")[0]
		mask = strings.NewReplacer(";", ",").Replace(mask)
		name = strings.TrimSpace(strings.Split(mask, ",")[0])
		if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "[]") {
			name = "sample.txt"
		}
		name = strings.ReplaceAll(strings.ReplaceAll(name, "*", "sample"), "?", "x")
	}
	return vfs.VFSItem{Name: name, IsDir: strings.Contains(g.Fields["IncludeAttributes"], "Directory")}
}

func buildFileGroupPreviews(d *f4settings.Draft, themeID string, samples ...map[string]fileGroupSampleData) (*theme.FileHighlighter, map[string]fileGroupPreview, error) {
	snapshot, err := compileFileGroupDraft(d)
	if err != nil {
		return nil, nil, err
	}
	view := snapshot.ForIconTheme(themeID)
	highlighter := &theme.FileHighlighter{}
	if err := highlighter.LoadFileGroups(view); err != nil {
		return nil, nil, err
	}
	result := map[string]fileGroupPreview{}
	for _, g := range view.Groups() {
		item := fileGroupSample(g)
		known := false
		if len(samples) > 0 {
			if sample, ok := samples[0][g.ID]; ok {
				item = sample.item
				known = sample.known
			}
		}
		_, style := highlighter.SemanticStyle(&item, known)
		contributors := []string{}
		matched := false
		for _, feature := range []string{"Highlight", "Icons", "Sort", "Associations"} {
			if feature == "Highlight" {
				for _, match := range view.HighlightGroups(config.App.ColorStyle) {
					predicate := view.HighlightPredicate(match.ID, config.App.ColorStyle)
					if match.Enabled("Highlight") && predicate != nil && predicate.Match(&item, known, time.Now()) {
						contributors = append(contributors, feature+": "+match.Label())
						matched = matched || match.ID == g.ID
					}
				}
				continue
			}
			for _, match := range view.Matching(&item, known, feature, time.Now()) {
				contributors = append(contributors, feature+": "+match.Label())
				matched = matched || match.ID == g.ID
			}
		}
		preview := fileGroupPreview{item: item, style: style, matched: matched, contributors: strings.Join(contributors, "\n")}
		for state, index := range []int{theme.ColPanelText, theme.ColPanelSelectedText, theme.ColPanelCursor, theme.ColPanelSelectedCursor} {
			base := uint64(7)
			if index < len(vtui.Palette) {
				base = vtui.Palette[index]
			}
			patch := []extui.HighlightColorPatchModel{style.Normal, style.Selected, style.Cursor, style.SelectedCursor}[state]
			expression := []string{}
			if patch.Foreground != "" {
				expression = append(expression, "foreground:"+patch.Foreground)
			}
			if patch.Background != "" {
				expression = append(expression, "background:"+patch.Background)
			}
			attr := theme.ParseFarColor(strings.Join(expression, " | "), base)
			if config.App.EnforceColorCorrection && len(expression) > 0 {
				fg, bg := theme.GetColorRGBBoth(attr)
				attr = vtui.SetRGBFore(attr, theme.CorrectContrast(fg, bg))
			}
			preview.attrs[state] = attr
		}
		result[g.ID] = preview
	}
	return highlighter, result, nil
}

func fileGroupCondition(values map[string]string) string {
	parts := []string{}
	if mask := values["group.Mask"]; mask != "" {
		parts = append(parts, mask)
	}
	for _, key := range []string{"IncludeAttributes", "ExcludeAttributes", "SizeAbove", "SizeBelow", "DateAfter", "DateBefore"} {
		if v := values["group."+key]; v != "" && v != "0" {
			parts = append(parts, fmt.Sprintf("%s: %s", key, v))
		}
	}
	return strings.Join(parts, "; ")
}
