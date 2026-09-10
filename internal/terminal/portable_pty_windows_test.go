package terminal

import (
	vtui "github.com/unxed/vtui"
	testing "testing"
)

func TestSystemConPTYAvailableForPortableBuild(t *testing.T) {
	if vtui.IsWine() {
		t.Skip("ConPTY unavailable under Wine")
	}
	api, err := systemConPTY()
	if err != nil {
		t.Fatal(err)
	}
	if api.path != "system:kernel32.dll" {
		t.Fatalf("unexpected portable API: %s", api.path)
	}
}
