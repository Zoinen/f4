package vreactive

import "testing"

func TestNewStateMachine_InitialState(t *testing.T) {
	sm := NewStateMachine("idle")
	if sm.State.Get() != "idle" {
		t.Fatalf("expected initial state %q, got %q", "idle", sm.State.Get())
	}
}

func TestStateMachine_AddStateRunsSettersOnTransition(t *testing.T) {
	sm := NewStateMachine("idle")

	color := NewProperty("gray")
	label := NewProperty("Idle")

	sm.AddState("running", SetProp(color, "green"), SetProp(label, "Running"))
	sm.AddState("error", SetProp(color, "red"), SetProp(label, "Error"))

	// Setters for a state that has not been entered yet must not run.
	if color.Get() != "gray" || label.Get() != "Idle" {
		t.Fatalf("setters ran before transition: color=%q label=%q", color.Get(), label.Get())
	}

	sm.State.Set("running")
	if color.Get() != "green" {
		t.Errorf("expected color %q after entering running, got %q", "green", color.Get())
	}
	if label.Get() != "Running" {
		t.Errorf("expected label %q after entering running, got %q", "Running", label.Get())
	}

	sm.State.Set("error")
	if color.Get() != "red" {
		t.Errorf("expected color %q after entering error, got %q", "red", color.Get())
	}
	if label.Get() != "Error" {
		t.Errorf("expected label %q after entering error, got %q", "Error", label.Get())
	}
}

func TestStateMachine_TransitionToUnknownStateIsNoOp(t *testing.T) {
	sm := NewStateMachine("idle")
	color := NewProperty("gray")
	sm.AddState("running", SetProp(color, "green"))

	// "paused" has no registered setters: this must not panic and must not
	// touch any property, it should just update State itself.
	sm.State.Set("paused")

	if sm.State.Get() != "paused" {
		t.Fatalf("expected state %q, got %q", "paused", sm.State.Get())
	}
	if color.Get() != "gray" {
		t.Errorf("unexpected side effect on unrelated property: %q", color.Get())
	}
}

func TestStateMachine_MultipleSettersRunInOrder(t *testing.T) {
	sm := NewStateMachine("idle")
	var order []string

	sm.AddState("go",
		func() { order = append(order, "first") },
		func() { order = append(order, "second") },
		func() { order = append(order, "third") },
	)

	sm.State.Set("go")

	want := []string{"first", "second", "third"}
	if len(order) != len(want) {
		t.Fatalf("expected %d calls, got %d (%v)", len(want), len(order), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("call %d: expected %q, got %q", i, want[i], order[i])
		}
	}
}

func TestStateMachine_ReAddStateOverwritesRules(t *testing.T) {
	sm := NewStateMachine("idle")
	count := NewProperty(0)

	sm.AddState("on", SetProp(count, 1))
	sm.AddState("on", SetProp(count, 2)) // overwrite with a new setter list

	sm.State.Set("on")
	if count.Get() != 2 {
		t.Fatalf("expected overwritten setter to run (2), got %d", count.Get())
	}
}

func TestSetProp_ReturnsWorkingSetter(t *testing.T) {
	p := NewProperty(10)
	setter := SetProp(p, 42)
	if p.Get() != 10 {
		t.Fatalf("SetProp must not run eagerly")
	}
	setter()
	if p.Get() != 42 {
		t.Fatalf("expected 42 after running setter, got %d", p.Get())
	}
}
