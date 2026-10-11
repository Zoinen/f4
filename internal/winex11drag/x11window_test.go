package winex11drag

import "testing"

func TestWineX11WindowFromProp(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  uintptr
		want uintptr
		ok   bool
	}{
		{name: "absent property (real Windows, or a native build)", raw: 0, want: 0, ok: false},
		{name: "a real Xlib Window ID", raw: 0x4200007, want: 0x4200007, ok: true},
		{name: "the smallest non-zero Window ID", raw: 1, want: 1, ok: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := wineX11WindowFromProp(tt.raw)
			if got != tt.want || ok != tt.ok {
				t.Errorf("wineX11WindowFromProp(%#x) = (%#x, %v), want (%#x, %v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}
