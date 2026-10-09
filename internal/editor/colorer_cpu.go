package editor

import "errors"

// wazero's amd64 compiler emits POPCNT for the scalar popcnt instructions in
// Colorer WASM. Callers fall back to Chroma when a session cannot be created.
// Hosts where wazero automatically uses its interpreter need no such guard.
var errColorerUnsupportedCPU = errors.New("Colorer's WASM compiler requires CPU POPCNT support")

func colorerRuntimeCheckForCPU(compilerSupported, hasPOPCNT bool) error {
	if !compilerSupported || hasPOPCNT {
		return nil
	}
	return errColorerUnsupportedCPU
}
