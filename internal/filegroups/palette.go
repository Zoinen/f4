package filegroups

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// HighlightConditionKeys are the shared predicate field names. Prefix one with
// Highlight. to override only highlighting, leaving classification unchanged.
var HighlightConditionKeys = []string{
	"Mask", "MaskSyntax", "Case", "IncludeAttributes", "ExcludeAttributes",
	"SizeAbove", "SizeBelow", "DateType", "DateAfter", "DateBefore", "DateRelative",
}

// HighlightFields identifies universal appearance overrides for editors and
// migrations. Classification conditions are deliberately excluded.
var HighlightFields = []string{
	"Highlight.Enabled", "NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor",
	"Mark", "ContinueProcessing", "UseDefaults",
	"Highlight.Mask", "Highlight.MaskSyntax", "Highlight.Case",
	"Highlight.IncludeAttributes", "Highlight.ExcludeAttributes",
	"Highlight.SizeAbove", "Highlight.SizeBelow", "Highlight.DateType",
	"Highlight.DateAfter", "Highlight.DateBefore", "Highlight.DateRelative",
}

// PalettePrefix encodes the exact UTF-8 style name without case folding.
func PalettePrefix(style string) string {
	return "Palette." + hex.EncodeToString([]byte(style)) + "."
}

// Highlighting returns an independent highlighting projection. A palette owns
// its whole predicate; absent palette conditions must not inherit classification
// restrictions. Explicit universal appearance fields, including empty clears,
// take precedence over the palette.
func (g Group) Highlighting(style string) Group {
	result := g.Clone()
	if result.Fields == nil {
		result.Fields = map[string]string{}
	}
	prefix := PalettePrefix(style)
	variant := false
	for key := range g.Fields {
		if strings.HasPrefix(key, prefix) {
			variant = true
			break
		}
	}
	if variant {
		for _, key := range HighlightConditionKeys {
			delete(result.Fields, key)
			delete(result.Fields, "Highlight."+key)
		}
		for key, value := range g.Fields {
			if strings.HasPrefix(key, prefix) {
				result.Fields[strings.TrimPrefix(key, prefix)] = value
			}
		}
		result.Fields["ColorStyle"] = style
		for _, key := range append([]string{"Name", "Enabled"}, HighlightFields...) {
			if value, ok := g.Fields[key]; ok {
				result.Fields[key] = value
			}
		}
		if _, universal := g.Fields["Highlight.Enabled"]; universal {
			result.Fields["ColorStyle"] = g.Fields["ColorStyle"]
		}
	}
	for _, key := range HighlightConditionKeys {
		if value, ok := result.Fields["Highlight."+key]; ok {
			result.Fields[key] = value
		}
	}
	return result
}

func paletteStyles(g Group) ([]string, error) {
	styles := []string{}
	seen := map[string]bool{}
	for key := range g.Fields {
		rest, ok := strings.CutPrefix(key, "Palette.")
		if !ok {
			continue
		}
		encoded, _, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		decoded, err := hex.DecodeString(encoded)
		if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) || encoded != hex.EncodeToString(decoded) {
			return nil, fmt.Errorf("invalid palette namespace %q", key)
		}
		style := string(decoded)
		if !seen[style] {
			styles = append(styles, style)
			seen[style] = true
		}
	}
	slices.Sort(styles)
	return styles, nil
}

func compileHighlight(g Group, style string) (*Predicate, error) {
	fields := g.Highlighting(style).Fields
	for _, key := range []string{"Enabled", "Highlight.Enabled", "ContinueProcessing", "UseDefaults", "DateRelative"} {
		switch fields[key] {
		case "", "true", "false", "0", "1":
		default:
			return nil, fmt.Errorf("invalid boolean %s", key)
		}
	}
	if raw := fields["Order"]; raw != "" {
		if _, err := strconv.Atoi(raw); err != nil {
			return nil, fmt.Errorf("invalid palette order %q", raw)
		}
	}
	return CompilePredicate(fields)
}

// HighlightPredicate returns a predicate compiled with the snapshot, including
// palette conditions and universal Highlight.* overrides. It never compiles.
func (s *Snapshot) HighlightPredicate(id, style string) *Predicate {
	if s != nil {
		for _, compiled := range s.groups {
			if compiled.group.ID == id {
				if s.colorTheme != "" {
					return compiled.highlight
				}
				if predicate := compiled.highlights[style]; predicate != nil {
					return predicate
				}
				return compiled.highlight
			}
		}
	}
	return nil
}

// HighlightGroups projects the selected palette in historical palette order.
// Universal groups keep their own order and precede the independent theme layer.
// Disabled groups remain visible so an explicitly disabled palette does not
// accidentally restore the legacy fallback theme rules.
func (s *Snapshot) HighlightGroups(style string) []Group {
	groups := []Group{}
	if s == nil {
		return groups
	}
	if s.colorTheme != "" {
		return s.colorHighlightGroups()
	}
	for _, compiled := range s.groups {
		group := compiled.group.Highlighting(style)
		if scope := group.Fields["ColorStyle"]; scope != "" && scope != style {
			continue
		}
		groups = append(groups, group)
	}
	slices.SortStableFunc(groups, func(a, b Group) int {
		aTheme, bTheme := a.Fields["ColorStyle"] != "", b.Fields["ColorStyle"] != ""
		if aTheme != bTheme {
			if aTheme {
				return 1
			}
			return -1
		}
		if !aTheme {
			return 0
		}
		aOrder, _ := strconv.Atoi(a.Fields[PalettePrefix(style)+"Order"])
		bOrder, _ := strconv.Atoi(b.Fields[PalettePrefix(style)+"Order"])
		return cmp.Compare(aOrder, bOrder)
	})
	return groups
}
