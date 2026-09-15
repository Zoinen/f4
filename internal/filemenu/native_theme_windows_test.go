package filemenu

import (
	"reflect"
	"testing"

	"github.com/zzl/go-win32api/v2/win32"
)

func TestMenuThemeFollowsSystemAndHighContrast(t *testing.T) {
	var dark, contrast bool
	var modes []uintptr
	var allowed []bool
	flushes := 0
	m := &menuTheme{
		prefer:     func(mode uintptr) { modes = append(modes, mode) },
		allow:      func(_ win32.HWND, value bool) { allowed = append(allowed, value) },
		refresh:    func() {},
		shouldDark: func() bool { return dark },
		contrast:   func() bool { return contrast },
		flush:      func() { flushes++ },
	}
	if !m.update(10) {
		t.Fatal("initial theme was not applied")
	}
	if m.update(10) {
		t.Fatal("unchanged theme invalidated the prepared menu")
	}
	dark = true
	if !m.update(10) || !m.dark {
		t.Fatal("dark theme was not applied")
	}
	contrast = true
	if !m.update(10) || m.dark {
		t.Fatal("dark mode overrode high contrast")
	}
	contrast = false
	if !m.update(10) || !m.dark {
		t.Fatal("leaving high contrast did not restore system dark mode")
	}
	dark = false
	if !m.update(10) || m.dark {
		t.Fatal("light theme was not restored")
	}
	if !reflect.DeepEqual(modes, []uintptr{1, 0, 1}) || !reflect.DeepEqual(allowed, []bool{false, true, false, true, false}) || flushes != 5 {
		t.Fatalf("modes=%v allowed=%v flushes=%d", modes, allowed, flushes)
	}
}

func TestMenuThemeUnsupportedAPIsFallBack(t *testing.T) {
	if (&menuTheme{}).update(10) {
		t.Fatal("missing API changed the theme")
	}
	for _, tc := range []struct {
		major, build uint32
		want         bool
	}{{6, 7601, false}, {10, 17763, false}, {10, 18362, true}, {10, 26100, true}} {
		if got := supportsDarkMenus(tc.major, tc.build); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}
