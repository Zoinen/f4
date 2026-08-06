package main

import (
	"testing"
)

func TestFar2lDarkStyle(t *testing.T) {
	err := ApplyColorStyle("Far2l Dark")
	if err != nil {
		t.Fatalf("Failed to apply Far2l Dark style: %v", err)
	}
}
