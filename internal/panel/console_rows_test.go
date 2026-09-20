package panel

import "testing"

// TestUnpaintedTerminalRows_Issue249 pins which rows the panels frame has to
// fill itself. With the panels hidden the layout keeps the keybar row out of
// the PTY, but neither the keybar nor the command line is drawn while an
// alternate-screen program or a running command owns the terminal, and the
// row left over then showed vtui's blue desktop background under the output.
func TestUnpaintedTerminalRows_Issue249(t *testing.T) {
	cases := []struct {
		name      string
		altScreen bool
		busy      bool
		termY2    int
		screenH   int
		wantFirst int
		wantLast  int
		wantEmpty bool
	}{
		{
			name:      "busy command leaves the reserved keybar row",
			busy:      true,
			termY2:    28,
			screenH:   30,
			wantFirst: 29,
			wantLast:  29,
		},
		{
			name:      "idle shell draws keybar and command line itself",
			termY2:    28,
			screenH:   30,
			wantEmpty: true,
		},
		{
			name:      "alt screen program already owns the last row",
			altScreen: true,
			termY2:    29,
			screenH:   30,
			wantEmpty: true,
		},
		{
			name:      "alt screen program short of the last row",
			altScreen: true,
			termY2:    27,
			screenH:   30,
			wantFirst: 28,
			wantLast:  29,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, last := unpaintedTerminalRows(tc.altScreen, tc.busy, tc.termY2, tc.screenH)
			if tc.wantEmpty {
				if first <= last {
					t.Fatalf("rows %d..%d, want an empty range", first, last)
				}
				return
			}
			if first != tc.wantFirst || last != tc.wantLast {
				t.Fatalf("rows %d..%d, want %d..%d", first, last, tc.wantFirst, tc.wantLast)
			}
		})
	}
}
