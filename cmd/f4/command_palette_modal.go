package main

import "github.com/unxed/vtui"

func commandPaletteModalFrameSupported(frame vtui.Frame) bool {
	switch frame.(type) {
	case commandPaletteHelpFrame, *GrabberFrame, *ArkanoidFrame:
		return true
	default:
		return false
	}
}
