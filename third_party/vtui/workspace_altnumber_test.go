package vtui

import "testing"

func newScreenFM(numbers []int, active int) *frameManager {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Screens = nil
	for _, n := range numbers {
		fm.Screens = append(fm.Screens, &AppScreen{Number: n})
	}
	fm.ActiveIdx = active
	return fm
}

// The reported bug: with a single workspace, Alt+1 was swallowed for good.
// Workspace 1 is always there and always active, so the switch reported
// success while doing nothing, and quick search never saw the key. Alt+2 and
// the rest worked because no such workspace exists and the key fell through.
func TestSwitchScreenNumber_AlreadyActiveDoesNotConsume(t *testing.T) {
	fm := newScreenFM([]int{1}, 0)

	if fm.switchScreenNumber(1) {
		t.Error("Alt+1 on the only workspace must fall through, not be consumed")
	}
}

// The feature itself must keep working: from another workspace, Alt+N still
// switches and still consumes the key.
func TestSwitchScreenNumber_SwitchesAndConsumes(t *testing.T) {
	fm := newScreenFM([]int{1, 2, 3}, 1) // sitting on workspace 2

	if !fm.switchScreenNumber(3) {
		t.Fatal("switching to another workspace should consume the key")
	}
	if fm.ActiveIdx != 2 {
		t.Errorf("ActiveIdx = %d after switching to workspace 3, want 2", fm.ActiveIdx)
	}
}

// Same rule with several workspaces: pressing the number you are already on
// is a no-op, so the key belongs to the frame below.
func TestSwitchScreenNumber_ActiveAmongManyDoesNotConsume(t *testing.T) {
	fm := newScreenFM([]int{1, 2, 3}, 1)

	if fm.switchScreenNumber(2) {
		t.Error("Alt+2 while already on workspace 2 must fall through")
	}
	if fm.ActiveIdx != 1 {
		t.Errorf("ActiveIdx changed to %d, want it left at 1", fm.ActiveIdx)
	}
}

// A number nobody owns was always meant to fall through.
func TestSwitchScreenNumber_MissingNumberFallsThrough(t *testing.T) {
	fm := newScreenFM([]int{1, 2}, 0)

	for _, n := range []int{3, 7, 9} {
		if fm.switchScreenNumber(n) {
			t.Errorf("Alt+%d matches no workspace and must fall through", n)
		}
	}
}

// Workspace numbers are stable and need not match their position, so the
// lookup must go by number rather than by index.
func TestSwitchScreenNumber_MatchesByNumberNotIndex(t *testing.T) {
	fm := newScreenFM([]int{5, 1, 9}, 0) // sitting on workspace 5, at index 0

	if !fm.switchScreenNumber(1) {
		t.Fatal("workspace 1 exists at index 1 and should be reachable")
	}
	if fm.ActiveIdx != 1 {
		t.Errorf("ActiveIdx = %d, want 1", fm.ActiveIdx)
	}

	// Index 0 holds workspace 5, so Alt+1 must not be read as "first screen".
	fm.ActiveIdx = 0
	if fm.switchScreenNumber(5) {
		t.Error("already on workspace 5, so Alt+5 must fall through")
	}
}

func TestSwitchScreenNumber_NoScreens(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.Screens = nil

	if fm.switchScreenNumber(1) {
		t.Error("with no workspaces at all, nothing can be switched to")
	}
}
