package filegroups

import (
	"fmt"
	"strconv"
	"strings"
)

// ImportLegacy leaves the source documents untouched. Highlight and sort
// predicates retain comma-only legacy glob syntax and case-insensitive matching.
func ImportLegacy(highlight, associations *Document) []Group {
	result := []Group{}
	for _, section := range legacySections(highlight, "highlight_") {
		fields := highlight.Section(section)
		fields["MaskSyntax"] = "legacy-glob"
		fields["Case"] = "insensitive"
		fields["Highlight.Enabled"] = "true"
		canonicalColors(fields)
		if _, ok := fields["Group"]; ok {
			fields["Sort.Enabled"] = "true"
			fields["Sort.Number"] = fields["Group"]
		}
		if fields["Icon"] != "" {
			fields["Icons.Enabled"] = "true"
			fields["Icon.*"] = fields["Icon"]
		}
		result = append(result, Group{ID: "legacy." + strings.ToLower(section), Fields: fields})
	}
	for i, section := range legacySections(highlight, "sortgroup_") {
		fields := highlight.Section(section)
		fields["MaskSyntax"] = "legacy-glob"
		fields["Case"] = "insensitive"
		fields["Sort.Enabled"] = "true"
		fields["Sort.Number"] = strconv.Itoa(i)
		if raw, ok := fields["Group"]; ok {
			fields["Sort.Number"] = raw
		}
		result = append(result, Group{ID: "legacy." + strings.ToLower(section), Fields: fields})
	}
	if associations == nil {
		return result
	}
	for i := 0; ; i++ {
		fields := associations.Section(fmt.Sprintf("Associations/Type%d", i))
		if fields == nil || fields["Mask"] == "" {
			break
		}
		state, _ := strconv.ParseUint(fields["State"], 10, 64)
		command := CommandSet{ID: "imported", Name: fields["Description"], Extra: fields}
		for slot, key := range CommandNames {
			command.Commands[slot] = fields[key]
			command.Enabled[slot] = state&(1<<slot) != 0
		}
		result = append(result, Group{ID: fmt.Sprintf("legacy.association.%d", i), Fields: map[string]string{
			"Name": fields["Description"], "Mask": fields["Mask"], "MaskSyntax": "far", "Case": "platform", "ExcludeAttributes": "Directory", "Associations.Enabled": "true",
		}, Commands: []CommandSet{command}})
	}
	return result
}

func legacySections(d *Document, prefix string) []string {
	if d == nil {
		return nil
	}
	names := []string{}
	for _, section := range d.SectionNames() {
		if strings.HasPrefix(strings.ToLower(section), prefix) {
			names = append(names, section)
		}
	}
	// Match the legacy numeric-section ordering, not lexicographical order.
	sortNumericSections(names, prefix)
	return names
}

func canonicalColors(fields map[string]string) {
	aliases := map[string][]string{
		"NormalColor": {"NormalFileName"}, "SelectedColor": {"SelectedFileName"},
		"CursorColor":         {"NormalColorUnderCursor", "FileNameUnderCursor"},
		"SelectedCursorColor": {"SelectedColorUnderCursor", "FileNameSelectedUnderCursor"}, "Mark": {"MarkChar"},
	}
	for key, names := range aliases {
		if fields[key] != "" {
			continue
		}
		for _, name := range names {
			if fields[name] != "" {
				fields[key] = fields[name]
				break
			}
		}
	}
}
