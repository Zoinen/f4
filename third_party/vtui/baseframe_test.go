package vtui

import "testing"

func TestBaseFrame_OnResult(t *testing.T) {
	bf := &BaseFrame{}
	result := -100
	bf.OnResult = func(code int) {
		result = code
	}
	bf.SetExitCode(42)
	if result != 42 {
		t.Errorf("OnResult callback failed, expected 42, got %d", result)
	}
}
func TestBaseFrame_SetBusy(t *testing.T) {
	bf := &BaseFrame{}
	if bf.IsBusy() {
		t.Fatal("expected IsBusy() to be false initially")
	}
	bf.SetBusy(true)
	if !bf.IsBusy() {
		t.Fatal("expected IsBusy() to be true after SetBusy(true)")
	}
}
