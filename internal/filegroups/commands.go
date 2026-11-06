package filegroups

import (
	"maps"
	"strconv"
)

func commandValues(command CommandSet) map[string]string {
	values := maps.Clone(command.Extra)
	if values == nil {
		values = map[string]string{}
	}
	values["Name"] = command.Name
	for slot, name := range CommandNames {
		values[name] = command.Commands[slot]
		values[name+".Enabled"] = strconv.FormatBool(command.Enabled[slot])
	}
	return values
}

func commandFromValues(id string, values map[string]string) CommandSet {
	command := CommandSet{ID: id, Name: values["Name"], Extra: maps.Clone(values)}
	for slot, name := range CommandNames {
		command.Commands[slot] = values[name]
		command.Enabled[slot] = values[name+".Enabled"] == "true" || values[name+".Enabled"] == "1"
		if values["Disabled"] == "true" {
			command.Enabled[slot] = false
		}
	}
	return command
}

func mergeCommands(defaults, overrides []CommandSet) []CommandSet {
	if overrides == nil {
		return defaults
	}
	byID := map[string]CommandSet{}
	for _, command := range defaults {
		byID[command.ID] = command
	}
	result := []CommandSet{}
	for _, patch := range overrides {
		if patch.Overrides != nil {
			values := commandValues(byID[patch.ID])
			maps.Copy(values, patch.Overrides)
			result = append(result, commandFromValues(patch.ID, values))
		} else {
			result = append(result, patch)
		}
		delete(byID, patch.ID)
	}
	for _, command := range defaults {
		if _, exists := byID[command.ID]; exists {
			result = append(result, command)
		}
	}
	return result
}

func commandDifferences(defaults, commands []CommandSet) []CommandSet {
	base := map[string]CommandSet{}
	for _, command := range defaults {
		base[command.ID] = command
	}
	result := []CommandSet{}
	for _, command := range commands {
		previous, exists := base[command.ID]
		if !exists {
			command.Overrides = nil
			result = append(result, command)
			continue
		}
		before, after := commandValues(previous), commandValues(command)
		patch := map[string]string{}
		for key, value := range after {
			if old, found := before[key]; !found || old != value {
				patch[key] = value
			}
		}
		command.Overrides = patch
		result = append(result, command)
		delete(base, command.ID)
	}
	for _, command := range defaults {
		if _, removed := base[command.ID]; removed {
			result = append(result, CommandSet{ID: command.ID, Overrides: map[string]string{"Disabled": "true"}})
		}
	}
	return result
}
