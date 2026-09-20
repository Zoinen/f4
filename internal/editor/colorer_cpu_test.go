package editor

import "testing"

func TestColorerRuntimeCheckForCPU(t *testing.T) {
	for _, tc := range []struct {
		name              string
		compilerSupported bool
		hasPOPCNT         bool
		want              error
	}{
		{"compiler with POPCNT", true, true, nil},
		{"compiler without POPCNT", true, false, errColorerUnsupportedCPU},
		{"interpreter with POPCNT", false, true, nil},
		{"interpreter without POPCNT", false, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := colorerRuntimeCheckForCPU(tc.compilerSupported, tc.hasPOPCNT); err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
