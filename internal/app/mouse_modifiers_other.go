//go:build !darwin

package app

import "github.com/unxed/vtinput"

type darwinMouseModifierTracker struct{}

func (*darwinMouseModifierTracker) normalize(*vtinput.InputEvent, string) {}
