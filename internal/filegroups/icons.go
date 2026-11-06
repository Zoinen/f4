package filegroups

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/unxed/f4/vfs"
)

// IconName is the portable, case-independent filename of a group's artwork.
// Display names remain editable; renderers never derive policy from filenames.
func IconName(name string) string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(parts, "-")
}

func addGroupIcons(g Group, themes map[string]ThemeSet, icons map[string]IconReference) {
	if g.Fields["Icons.Enabled"] == "false" || g.Fields["Icons.Enabled"] == "0" {
		return
	}
	key := IconName(g.Label())
	for _, set := range themes {
		if !booleanValue(set.Fields["GroupIcons"], false) {
			continue
		}
		catalog := set.Catalog()
		if _, canonical := themes[catalog]; canonical && set.ID != catalog {
			continue
		}
		if _, explicit := icons[catalog]; explicit {
			continue
		}
		for _, asset := range strings.Split(set.Fields["CatalogAssets"], ",") {
			if key != "" && asset == key {
				icons[catalog] = IconReference{Kind: "asset", Value: key}
				break
			}
		}
	}
}

func (g compiledGroup) iconsEnabled() bool {
	return g.group.Fields["Enabled"] != "false" && g.group.Fields["Enabled"] != "0" &&
		g.group.Fields["Icons.Enabled"] != "false" && g.group.Fields["Icons.Enabled"] != "0" &&
		(g.group.Enabled("Icons") || len(g.icons) > 0)
}

// ParseIconReference accepts legacy URLs as well as typed references. Keeping
// interpretation here prevents frontends from treating a filename as policy.
func ParseIconReference(raw string) (IconReference, error) {
	if raw == "" || raw == "none" {
		return IconReference{Kind: "none"}, nil
	}
	kind, value, found := strings.Cut(raw, ":")
	if found && (kind == "asset" || kind == "native" || kind == "file") {
		if value == "" {
			return IconReference{}, fmt.Errorf("empty %s icon reference", kind)
		}
		return IconReference{Kind: kind, Value: value}, nil
	}
	// Existing highlight.ini icons include local paths and qrc/image URLs.
	return IconReference{Kind: "file", Value: raw}, nil
}

// IconChoices returns the first applicable choice for each theme independently.
// Masks and references are compiled once; no catalog or filesystem is read.
func (s *Snapshot) IconChoices(item *vfs.VFSItem, known bool, now time.Time) map[string]IconChoice {
	choices := map[string]IconChoice{}
	if s == nil {
		return choices
	}
	for order, g := range s.groups {
		if !g.iconsEnabled() || !g.predicate.Match(item, known, now) {
			continue
		}
		for theme, reference := range g.icons {
			if _, found := choices[theme]; found {
				continue
			}
			choices[theme] = IconChoice{Order: order, GroupID: g.group.ID, Reference: reference, OverrideNative: g.group.Fields["OverrideNativeIcon"] == "true" || g.group.Fields["OverrideNativeIcon"] == "1"}
		}
	}
	return choices
}
