package vtui

import "testing"

func TestLogAndRepanicKeepsThePanicGoing(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("LogAndRepanic swallowed the panic; the caller would carry on with broken state")
		}
		if s, ok := r.(string); !ok || s != "boom" {
			t.Errorf("LogAndRepanic re-panicked with %v, want the original value", r)
		}
	}()

	func() {
		defer LogAndRepanic("test callback")
		panic("boom")
	}()

	t.Fatal("the panic never reached the caller")
}

func TestLogAndRepanicIsQuietWithoutAPanic(t *testing.T) {
	done := false
	func() {
		defer LogAndRepanic("test callback")
		done = true
	}()
	if !done {
		t.Fatal("the guarded function did not run")
	}
}
