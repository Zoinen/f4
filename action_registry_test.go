package main

import "testing"

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
