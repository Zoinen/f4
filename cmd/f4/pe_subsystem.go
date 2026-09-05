package main

import (
	"debug/pe"
	"strings"
	"sync"
)

// GUI-subsystem detection for the shell's children.
//
// cmd waits for a console program and returns to its prompt at once for a GUI
// one, so `notepad` typed at the prompt leaves cmd idle with notepad as its
// child. Treating every child as "busy" therefore kept f4's terminal busy for
// as long as notepad stayed open. Whether a program is a GUI program is
// written in its PE header, so it can be read from the image file without
// touching the process.

const (
	peSubsystemWindowsGUI = 2
	peSubsystemWindowsCUI = 3
)

var (
	peSubsystemMu    sync.Mutex
	peSubsystemCache = map[string]bool{}
)

// executableIsGUI reports whether the PE image at path was built for the
// Windows GUI subsystem. Results are cached per path: the shell's children
// are re-examined every few hundred milliseconds while one of them runs.
func executableIsGUI(path string) (bool, error) {
	key := strings.ToLower(path)
	peSubsystemMu.Lock()
	gui, ok := peSubsystemCache[key]
	peSubsystemMu.Unlock()
	if ok {
		return gui, nil
	}
	gui, err := readPESubsystemIsGUI(path)
	if err != nil {
		return false, err
	}
	peSubsystemMu.Lock()
	peSubsystemCache[key] = gui
	peSubsystemMu.Unlock()
	return gui, nil
}

func readPESubsystemIsGUI(path string) (bool, error) {
	f, err := pe.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return h.Subsystem == peSubsystemWindowsGUI, nil
	case *pe.OptionalHeader32:
		return h.Subsystem == peSubsystemWindowsGUI, nil
	}
	return false, nil
}
