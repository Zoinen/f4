package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unxed/f4/sdk/extui"
	"github.com/vmihailenco/msgpack/v5"
)

func main() {
	output := flag.String("output", "", "path of the generated MessagePack fixture")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-output is required")
		os.Exit(2)
	}

	fixtures := []extui.M{
		extui.Envelope{
			Sequence: 1,
			StreamID: "command-line",
			Revision: 1,
			Kind:     extui.KindSnapshot,
			Payload: extui.M{
				"type": "command_line_snapshot",
				"state": extui.M{
					"commandLine": extui.M{
						"visible":        true,
						"text":           "dir C:\\Windows\\WinSxS",
						"cursorPosition": 20,
					},
				},
			},
		}.ToMap(),
		extui.Envelope{
			Sequence: 2,
			StreamID: "panel/0",
			Revision: 7,
			Kind:     extui.KindSnapshot,
			Payload: extui.M{
				"type": "panel_catalog_snapshot",
				"state": extui.M{
					"side": 0,
					"panel": extui.M{
						"id":              "left-panel",
						"catalogId":       "winsxs",
						"catalogRevision": uint64(19),
						"totalCount":      30000,
						"current":         1,
						"entries": []extui.M{
							{"name": "..", "kind": "directory"},
							{"name": "amd64_fixture", "kind": "directory", "hidden": true},
						},
					},
				},
			},
		}.ToMap(),
		extui.Envelope{
			Sequence: 3,
			StreamID: "menus",
			Revision: 4,
			Kind:     extui.KindSnapshot,
			Payload: extui.M{
				"type": "menus_snapshot",
				"state": extui.M{
					"menus": []extui.M{{"id": "drives", "selected": 0}},
				},
			},
		}.ToMap(),
		extui.Envelope{
			Sequence:     4,
			StreamID:     "menus",
			Revision:     5,
			BaseRevision: extui.Revision(4),
			Kind:         extui.KindPatch,
			Payload: extui.M{
				"type":     "menu_state_patch",
				"selected": 1,
			},
		}.ToMap(),
	}

	encoded, err := msgpack.Marshal(fixtures)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode fixtures: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create fixture directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write fixture: %v\n", err)
		os.Exit(1)
	}
}
