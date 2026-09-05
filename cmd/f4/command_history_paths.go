package main

import (
	"encoding/json"

	"github.com/unxed/vtui"
)

const commandHistoryPathsID = "cmdline.paths"

type commandHistoryPathRecord struct {
	Command string `json:"command"`
	Path    string `json:"path"`
}

func loadCommandHistoryPaths(commands []string) []string {
	paths := make([]string, len(commands))
	if vtui.GlobalHistoryProvider == nil || len(commands) == 0 {
		return paths
	}

	byCommand := make(map[string]string)
	for _, encoded := range vtui.GlobalHistoryProvider.LoadHistory(commandHistoryPathsID) {
		var record commandHistoryPathRecord
		if json.Unmarshal([]byte(encoded), &record) == nil && record.Command != "" {
			byCommand[record.Command] = record.Path
		}
	}
	for i, command := range commands {
		paths[i] = byCommand[command]
	}
	return paths
}

func saveCommandHistoryPaths(commands, paths []string) {
	if vtui.GlobalHistoryProvider == nil {
		return
	}

	encoded := make([]string, 0, len(commands))
	for i, command := range commands {
		if command == "" || i >= len(paths) || paths[i] == "" {
			continue
		}
		data, err := json.Marshal(commandHistoryPathRecord{Command: command, Path: paths[i]})
		if err == nil {
			encoded = append(encoded, string(data))
		}
	}
	vtui.GlobalHistoryProvider.SaveHistory(commandHistoryPathsID, encoded)
}

func rememberCommandHistoryPath(command, path string, commands []string) {
	if command == "" || path == "" || len(commands) == 0 {
		return
	}
	paths := loadCommandHistoryPaths(commands)
	for i, candidate := range commands {
		if candidate == command {
			paths[i] = path
			break
		}
	}
	saveCommandHistoryPaths(commands, paths)
}
