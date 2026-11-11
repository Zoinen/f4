package theme

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
)

// Clone freezes matching inputs for deferred metadata work. Compiled predicates
// and file-group snapshots are immutable and shared; mutable lookup caches and
// their mutexes belong to each clone. Include attribute overrides, not just the
// ordinary colour cascade, so UseDefaults survives deferred projection too.
func (fh *FileHighlighter) Clone() *FileHighlighter {
	if fh == nil {
		return nil
	}
	return &FileHighlighter{
		fileGroups: fh.fileGroups,
		UserRules:  slices.Clone(fh.UserRules), ThemeRules: slices.Clone(fh.ThemeRules),
		baseThemeRules: slices.Clone(fh.baseThemeRules), activeColorStyle: fh.activeColorStyle,
		overrides: slices.Clone(fh.overrides), Rules: slices.Clone(fh.Rules),
		Revision: fh.Revision, hasVolatileRules: fh.hasVolatileRules,
	}
}

// SemanticFileStyle uses only available header metadata. It performs no stat or
// file read while a document is painting; deferred rules stay deferred.
func SemanticFileStyle(name string) *extui.HighlightStyleModel {
	if GlobalFileHighlighter == nil || GlobalFileHighlighter.fileGroups == nil {
		return nil
	}
	_, style := GlobalFileHighlighter.SemanticStyle(&vfs.VFSItem{Name: name}, false)
	return &style
}

// LoadFileGroups projects only the highlighting facet. Icon-only groups cannot
// terminate the colour cascade. Colour-style rules retain their normal layer.
func (fh *FileHighlighter) LoadFileGroups(snapshot *filegroups.Snapshot) error {
	rules := []HighlightRule{}
	themeRules := []HighlightRule{}
	activeStyle := fh.activeColorStyle
	if activeStyle == "" {
		activeStyle = config.App.ColorStyle
	}
	for _, group := range snapshot.HighlightGroups(activeStyle) {
		style := group.Fields["ColorStyle"]
		if style != "" && style != activeStyle {
			continue
		}
		if !group.Enabled("Highlight") {
			continue
		}
		condition := snapshot.HighlightPredicate(group.ID, activeStyle)
		file := ini.New()
		file.Data["Highlight_0"] = fileGroupHighlightFields(group.Fields)
		parsed := ParseHighlightRules(file)
		if len(parsed) == 0 {
			continue
		}
		rule := parsed[0]
		rule.SourceRevision = snapshot.Revision()
		rule.RuleID = group.ID
		rule.ColorizeFileName = group.Fields["Highlight.ColorizeFileName"] == "true" || group.Fields["Highlight.ColorizeFileName"] == "1"
		rule.Condition = condition
		// The compiled predicate owns mask/case semantics, including Far masks.
		rule.Masks = nil
		rule.maskIndex = nil
		if style != "" {
			themeRules = append(themeRules, rule)
		} else {
			rules = append(rules, rule)
		}
	}
	fh.UserRules = rules
	// Once file groups own appearance, an empty list is intentional. Falling
	// back to legacy style rules would silently resurrect deleted groups.
	fh.ThemeRules = themeRules
	fh.fileGroups = snapshot
	fh.CombineRules()
	return nil
}

// The legacy renderer accepts 0/1 booleans and alias colour keys. The domain
// uses true/false and canonical fields; stale imported aliases must not undo an
// explicit clear of a canonical field in the editor.
func fileGroupHighlightFields(fields map[string]string) map[string]string {
	result := maps.Clone(fields)
	for _, key := range []string{"ContinueProcessing", "UseDefaults", "DateRelative"} {
		if result[key] == "true" {
			result[key] = "1"
		}
		if result[key] == "false" {
			result[key] = "0"
		}
	}
	for _, key := range []string{"NormalFileName", "SelectedFileName", "NormalColorUnderCursor", "FileNameUnderCursor", "SelectedColorUnderCursor", "FileNameSelectedUnderCursor", "MarkChar"} {
		delete(result, key)
	}
	return result
}

// FileGroupDefaults returns the complete canonical factory set, with INI-backed
// palette variants merged into shared classification groups. Classification
// order stays independent of historical palette order; fallbacks remain last.
func FileGroupDefaults() []filegroups.Group {
	return colorStyleFileGroups(AvailableColorStyles())
}

func colorStyleFileGroups(styles []ColorStyle) []filegroups.Group {
	groups := []filegroups.Group{}
	byID := map[string]int{}
	factory, err := filegroups.FactoryGroups()
	if err != nil {
		// Embedded factory data is validated by its package tests. Preserve
		// palette contributions even if that independent source is unavailable.
		factory = nil
	}
	for _, group := range factory {
		byID[group.ID] = len(groups)
		groups = append(groups, group.Clone())
	}
	for _, style := range styles {
		if style.ini == nil {
			continue
		}
		for order, section := range parseRuleSectionsAt(style.ini, "highlight_", "") {
			fields := style.ini.Sections()[section.Section]
			copy := filegroups.OverlayFields(fields, map[string]string{
				"Highlight.Enabled": "true", "MaskSyntax": "legacy-glob", "Case": "insensitive",
			})
			copy["NormalColor"] = section.Rule.NormalStr
			copy["SelectedColor"] = section.Rule.SelectedStr
			copy["CursorColor"] = section.Rule.CursorStr
			copy["SelectedCursorColor"] = section.Rule.SelectedCursorStr
			copy["Mark"] = section.Rule.Mark
			legacyID := fmt.Sprintf("style.%x.%s", sha256.Sum256([]byte(style.Name)), section.Section)
			if declared, present := fields["LegacyID"]; present {
				// Reorganized shipped palettes declare their old identity;
				// new rules explicitly opt out instead of stealing a row ID.
				legacyID = strings.TrimSpace(declared)
			}
			id := strings.TrimSpace(fields["FileGroup"])
			if id == "" {
				// Unknown installed styles do not acquire shared identities just
				// because a label happens to resemble a built-in classification.
				id = fmt.Sprintf("palette.%x.%s", sha256.Sum256([]byte(style.Name)), section.Section)
			}
			index, exists := byID[id]
			if !exists {
				index = len(groups)
				byID[id] = index
				base := map[string]string{"Factory": "true", "Name": fields["Name"]}
				for _, key := range filegroups.HighlightConditionKeys {
					if value, ok := copy[key]; ok {
						base[key] = value
					}
				}
				groups = append(groups, filegroups.Group{ID: id, Fields: base})
			}
			prefix := filegroups.PalettePrefix(style.Name)
			for key, value := range copy {
				groups[index].Fields[prefix+key] = value
			}
			groups[index].Fields[prefix+"LegacyID"] = legacyID
			groups[index].Fields[prefix+"Order"] = strconv.Itoa(order)
		}
	}
	ordered := make([]filegroups.Group, 0, len(groups))
	fallback := func(id string) bool {
		return id == "type.parent-folder" || id == "type.folders" || id == "type.files"
	}
	for _, group := range groups {
		if !fallback(group.ID) {
			ordered = append(ordered, group)
		}
	}
	for _, group := range groups {
		if fallback(group.ID) {
			ordered = append(ordered, group)
		}
	}
	return ordered
}
