package main

import "testing"

func TestShouldProbeGraphicsAsksOnlyWhenNothingElseAnswered(t *testing.T) {
	base := graphicsProbeInputs{protocolIsNone: true, ttyBackend: "ansi", windows: true}
	if !shouldProbeGraphics(base) {
		t.Fatal("a Windows VT session with no protocol chosen must ask the terminal")
	}

	// This is the case the whole probe exists for: Windows Terminal as the
	// default terminal, so no WT_SESSION, so no protocol, so ask.
	if got := shouldProbeGraphics(graphicsProbeInputs{
		protocolIsNone: true, ttyBackend: "ansi", windows: true}); !got {
		t.Fatal("the default-terminal handoff case must reach the probe")
	}

	for name, in := range map[string]graphicsProbeInputs{
		"a protocol is already chosen": {protocolIsNone: false, ttyBackend: "ansi", windows: true},
		"the user forced one":          {protocolIsNone: true, ttyBackend: "ansi", forcedProtocol: "sixel", windows: true},
		"the winapi backend":           {protocolIsNone: true, ttyBackend: "winapi", windows: true},
		"the win32 backend":            {protocolIsNone: true, ttyBackend: "win32", windows: true},
		"not Windows":                  {protocolIsNone: true, ttyBackend: "ansi", windows: false},
	} {
		if shouldProbeGraphics(in) {
			t.Errorf("%s: must not ask the terminal", name)
		}
	}
}
