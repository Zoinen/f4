package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/unxed/vtui"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: vtui-lint <file.vui> ...")
		os.Exit(1)
	}

	hasErrors := false
	for _, path := range os.Args[1:] {
		if err := lintFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			hasErrors = true
		} else {
			fmt.Printf("%s: OK\n", path)
		}
	}

	if hasErrors {
		os.Exit(1)
	}
}

func lintFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid JSON syntax: %w", err)
	}

	if _, ok := raw["vuiVersion"]; !ok {
		return fmt.Errorf("missing required 'vuiVersion' field")
	}
	if _, ok := raw["root"]; !ok {
		return fmt.Errorf("missing required 'root' node")
	}

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dlg, err := vtui.LoadDialogFile(path)
	if err != nil {
		return fmt.Errorf("failed to load dialog: %w", err)
	}

	errs := vtui.ValidateLayout(dlg)
	if len(errs) > 0 {
		return fmt.Errorf("layout validation failed: %v", errs[0])
	}

	return nil
}
