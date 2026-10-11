//go:build windows

package netbrowse

import "testing"

// TestEnumerateTheRealNetwork asks the real WNet API for the top of the
// network. A CI runner may have no network provider at all, so an empty answer
// is fine; what must not happen is a failure that is not "no more items".
func TestEnumerateTheRealNetwork(t *testing.T) {
	entries, err := enumerateNetwork(nil)
	if err != nil {
		t.Fatalf("enumerateNetwork(nil): %v", err)
	}
	for _, e := range entries {
		if e.Remote == "" && e.Provider == "" {
			t.Errorf("an entry with no name: %+v", e)
		}
	}
	// One level down, for whatever container there is; errors there are
	// normal (no network), only a panic or a crash would be a bug.
	for _, e := range entries {
		if e.Container {
			_, _ = enumerateNetwork(&e)
			break
		}
	}
}
