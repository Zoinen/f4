package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestActionRegistry(t *testing.T) {
	called := false
	testAction := Action{
		Name:        "Test.Action",
		Label:       "Test Label",
		Description: "Test Description",
		Handler: func() bool {
			called = true
			return true
		},
	}

	RegisterAction(testAction)

	// Test GetAction
	a, ok := GetAction("test.action")
	if !ok {
		t.Fatal("Expected to find Test.Action")
	}
	if a.Label != "Test Label" || a.Description != "Test Description" {
		t.Errorf("Action fields mismatch. Got %+v", a)
	}

	// Test GetActions
	actions := GetActions()
	found := false
	for _, act := range actions {
		if act.Name == "Test.Action" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Action not found in GetActions() result")
	}

	// Test RunAction
	if !RunAction("Test.action") {
		t.Error("RunAction failed")
	}
	if !called {
		t.Error("Action handler was not executed")
	}

	// Test missing action
	if RunAction("Missing.Action") {
		t.Error("RunAction should return false for missing action")
	}
}

func TestAction_PanelToggleHidden(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	original := AppConfig.ShowHiddenFiles
	defer func() { AppConfig.ShowHiddenFiles = original }()

	if !RunAction("Panel.ToggleHidden") {
		t.Fatal("Panel.ToggleHidden did not run")
	}
	if AppConfig.ShowHiddenFiles == original {
		t.Errorf("Panel.ToggleHidden did not flip ShowHiddenFiles (was %v, still %v)", original, AppConfig.ShowHiddenFiles)
	}

	if !RunAction("Panel.ToggleHidden") {
		t.Fatal("Panel.ToggleHidden did not run on second call")
	}
	if AppConfig.ShowHiddenFiles != original {
		t.Errorf("Panel.ToggleHidden second call did not restore ShowHiddenFiles (want %v, got %v)", original, AppConfig.ShowHiddenFiles)
	}
}
