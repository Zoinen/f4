package settings

import (
	"maps"
	"slices"
	"strings"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/sdk/f4settings"
)

const fileGroupCommands = "filegroups.commands"
const fileGroupActionPrefix = "filegroups.action."

func fileGroupCommandConfigured(values map[string]string, slot string) bool {
	field := "command." + slot
	return values[field] != "" || values[field+".Enabled"] == "true"
}

// The editor works with one record per action. The storage format can still
// read legacy records that contain commands for several actions.
func fileGroupActionCollections(draft *f4settings.Draft, groupID string) []f4settings.Collection {
	labels := [...]string{"Open (Enter)", "Alternate open", "View (F3)", "Alternate view", "Edit (F4)", "Alternate edit"}
	collections := make([]f4settings.Collection, 0, len(labels))
	usedOriginalID := make(map[string]bool)
	for index, slot := range filegroups.CommandNames {
		id := fileGroupActionPrefix + slot
		field := "command." + slot
		enabled := recordField(field+".Enabled", "Enabled", "", f4settings.Boolean)
		enabled.Default = "true"
		collections = append(collections, recordCollection(
			id, "filegroup-Associations", labels[index],
			"Commands for this action and file group.",
			"command.Name",
			[]f4settings.Field{
				recordField("command.Name", "Command name", "Shown in the command chooser.", f4settings.String),
				enabled,
				recordField(field, "Command", "Supports Far command substitutions and multiple lines.", f4settings.Multiline),
			},
		))
		var rows []f4settings.Record
		for _, record := range draft.Records[fileGroupCommands] {
			if record.Values["command.Group"] != groupID || !fileGroupCommandConfigured(record.Values, slot) {
				continue
			}
			values := maps.Clone(record.Values)
			for _, other := range filegroups.CommandNames {
				if other != slot {
					delete(values, "command."+other)
					delete(values, "command."+other+".Enabled")
				}
			}
			rowID := record.ID
			if usedOriginalID[rowID] {
				rowID = filegroups.NewID()
			}
			usedOriginalID[record.ID] = true
			rows = append(rows, f4settings.Record{ID: rowID, Values: values})
		}
		draft.Records[id] = rows
		draft.BaselineRecords[id] = f4settings.NewDraft(nil, map[string][]f4settings.Record{id: rows}).Records[id]
	}
	return collections
}

// Rebuild the persisted view from the original records on every preview so
// undoing every action edit leaves the original records and IDs intact.
func syncFileGroupActionCommands(draft *f4settings.Draft, groupID string) bool {
	original := draft.BaselineRecords[fileGroupCommands]
	changed := slices.ContainsFunc(filegroups.CommandNames[:], func(slot string) bool {
		return draft.Dirty(fileGroupActionPrefix + slot)
	})
	if !changed {
		draft.Records[fileGroupCommands] = original
		return false
	}

	rows := make([]f4settings.Record, 0, len(original))
	for _, record := range original {
		configured := slices.ContainsFunc(filegroups.CommandNames[:], func(slot string) bool {
			return fileGroupCommandConfigured(record.Values, slot)
		})
		if record.Values["command.Group"] != groupID || !configured {
			rows = append(rows, record)
		}
	}
	for _, slot := range filegroups.CommandNames {
		for _, record := range draft.Records[fileGroupActionPrefix+slot] {
			values := maps.Clone(record.Values)
			values["command.Group"] = groupID
			rows = append(rows, f4settings.Record{ID: record.ID, Values: values})
		}
	}
	draft.Records[fileGroupCommands] = rows
	return true
}

func isFileGroupActionCollection(id string) bool {
	return strings.HasPrefix(id, fileGroupActionPrefix)
}
