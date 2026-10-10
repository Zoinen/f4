package vtui

import "testing"

func TestPickCloseSymbol(t *testing.T) {
	cases := []struct {
		configured rune
		raster     bool
		want       rune
	}{
		{'×', false, '×'},
		{'×', true, 'x'},
		{'X', true, 'X'}, // an application's own symbol is not replaced
		{'x', false, 'x'},
	}
	for _, c := range cases {
		if got := pickCloseSymbol(c.configured, c.raster); got != c.want {
			t.Errorf("pickCloseSymbol(%q, %v) = %q, want %q", c.configured, c.raster, got, c.want)
		}
	}
}

func TestEffectiveCloseSymbolFollowsTheConfiguredOne(t *testing.T) {
	old := UIStrings.CloseSymbol
	defer func() { UIStrings.CloseSymbol = old }()
	UIStrings.CloseSymbol = 'Q'
	if got := EffectiveCloseSymbol(); got != 'Q' {
		t.Fatalf("EffectiveCloseSymbol() = %q, want Q", got)
	}
	// Whatever font the test host has, the default is either kept or turned
	// into a plain x, never something else.
	UIStrings.CloseSymbol = '×'
	if got := EffectiveCloseSymbol(); got != '×' && got != 'x' {
		t.Fatalf("EffectiveCloseSymbol() = %q", got)
	}
}
