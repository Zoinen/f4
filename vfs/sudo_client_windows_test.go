//go:build windows

package vfs

import "testing"

func TestSudoClient_IsUnavailableOnWindows(t *testing.T) {
	client := &SudoClient{}
	if client.IsAvailable() {
		t.Fatal("Windows must not advertise the Unix sudo dispatcher as available")
	}
}
