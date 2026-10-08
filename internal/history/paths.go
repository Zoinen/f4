package history

import (
	"encoding/json"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const CommandHistoryPathsID = "cmdline.paths"

// normalizeHistoryPath canonicalizes legacy escaped NetFox addresses without
// decoding literal percent signs twice or treating # and ? as URL delimiters.
func normalizeHistoryPath(raw string) string {
	scheme, ok := vfs.URIScheme(raw)
	if !ok || !strings.EqualFold(scheme, "net") {
		return raw
	}
	scheme, device, remote, err := vfs.ParseDevicePath(raw)
	if err != nil {
		return raw
	}
	result := (vfs.DevicePath{Scheme: scheme, Device: device}).Public(remote)
	if !strings.Contains(raw[strings.Index(raw, "://")+3:], "/") {
		result = strings.TrimSuffix(result, "/")
	}
	return result
}

func normalizeHistoryEntry(id, raw string) string {
	if id == "folders" {
		return normalizeHistoryPath(raw)
	}
	if id == ViewerEditorHistoryID {
		var record ViewerEditorRecord
		if json.Unmarshal([]byte(raw), &record) != nil {
			return raw
		}
		path, display := normalizeHistoryPath(record.Path), normalizeHistoryPath(record.Display)
		if path != record.Path || display != record.Display {
			record.Path, record.Display = path, display
			if encoded, err := json.Marshal(record); err == nil {
				return string(encoded)
			}
		}
	}
	return raw
}

func normalizeHistoryRecord(id string, record HistoryRecord) HistoryRecord {
	if id == "folders" {
		record.Name = normalizeHistoryPath(record.Name)
	}
	record.Dir = normalizeHistoryPath(record.Dir)
	record.Extra = normalizeHistoryPath(record.Extra)
	return record
}

type commandHistoryPathRecord struct {
	Command string `json:"command"`
	Path    string `json:"path"`
}

func LoadCommandHistoryPaths(commands []string) []string {
	paths := make([]string, len(commands))
	if vtui.GlobalHistoryProvider == nil || len(commands) == 0 {
		return paths
	}

	byCommand := make(map[string]string)
	for _, encoded := range vtui.GlobalHistoryProvider.LoadHistory(CommandHistoryPathsID) {
		var record commandHistoryPathRecord
		if json.Unmarshal([]byte(encoded), &record) == nil && record.Command != "" {
			byCommand[record.Command] = normalizeHistoryPath(record.Path)
		}
	}
	for i, command := range commands {
		paths[i] = byCommand[command]
	}
	return paths
}

func SaveCommandHistoryPaths(commands, paths []string) {
	if vtui.GlobalHistoryProvider == nil {
		return
	}

	encoded := make([]string, 0, len(commands))
	for i, command := range commands {
		if command == "" || i >= len(paths) || paths[i] == "" {
			continue
		}
		data, err := json.Marshal(commandHistoryPathRecord{Command: command, Path: normalizeHistoryPath(paths[i])})
		if err == nil {
			encoded = append(encoded, string(data))
		}
	}
	vtui.GlobalHistoryProvider.SaveHistory(CommandHistoryPathsID, encoded)
}

func RememberCommandHistoryPath(command, path string, commands []string) {
	if command == "" || path == "" || len(commands) == 0 {
		return
	}
	paths := LoadCommandHistoryPaths(commands)
	for i, candidate := range commands {
		if candidate == command {
			paths[i] = path
			break
		}
	}
	SaveCommandHistoryPaths(commands, paths)
}
