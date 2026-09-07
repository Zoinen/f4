package main

import "testing"

// A fake ini: only what overlayEnabled asks for, so the test says nothing
// about how a file is parsed.
func fakeIni(values map[string]string) func(string, string, string) string {
	return func(section, key, def string) string {
		if v, ok := values[section+"."+key]; ok {
			return v
		}
		return def
	}
}

func TestTheOverlayIsOnByDefault(t *testing.T) {
	if !overlayEnabled(fakeIni(nil)) {
		t.Error("a configuration file that says nothing turned the overlay off")
	}
}

func TestTheNewNameWorks(t *testing.T) {
	if overlayEnabled(fakeIni(map[string]string{"Images.Overlay": "0"})) {
		t.Error("Overlay=0 left the overlay on")
	}
	if !overlayEnabled(fakeIni(map[string]string{"Images.Overlay": "1"})) {
		t.Error("Overlay=1 turned the overlay off")
	}
}

// The whole point of keeping the old name: somebody who switched the overlay
// off long ago must not have it come back on under them after an update.
func TestTheOldNameStillCounts(t *testing.T) {
	if overlayEnabled(fakeIni(map[string]string{"Images.X11Overlay": "0"})) {
		t.Error("an existing X11Overlay=0 was ignored")
	}
}

// A file carrying both is a file that has been edited since the rename, so the
// name it was edited with is the one that means anything.
func TestTheNewNameWinsOverTheOld(t *testing.T) {
	if !overlayEnabled(fakeIni(map[string]string{"Images.Overlay": "1", "Images.X11Overlay": "0"})) {
		t.Error("the old name overruled the new one")
	}
	if overlayEnabled(fakeIni(map[string]string{"Images.Overlay": "0", "Images.X11Overlay": "1"})) {
		t.Error("the old name overruled the new one")
	}
}
