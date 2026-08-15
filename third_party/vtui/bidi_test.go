package vtui

import "testing"

func TestBidi_ASCII_Guard(t *testing.T) {
	s := "Hello World!"
	vis := VisualString(s)
	if vis != s {
		t.Errorf("ASCII guard failed: expected %q, got %q", s, vis)
	}
}

func TestBidi_Hebrew_Simple(t *testing.T) {
	s := "שלום"
	vis, offsets := VisualStringWithMap(s)
	expected := "םולש"
	if vis != expected {
		t.Errorf("Hebrew reversal failed: expected %q, got %q", expected, vis)
	}
	if len(offsets) != 4 || offsets[0] != 6 {
		t.Errorf("RTL offset mapping failed: %v", offsets)
	}
}

func TestBidi_Hebrew_With_Latin(t *testing.T) {
	s := "שלום hello"
	vis := VisualString(s)
	expected := " םולשhello"
	if vis != expected {
		t.Errorf("Mixed LTR/RTL failed: expected %q, got %q", expected, vis)
	}
}

func TestBidi_Parentheses_Mirroring(t *testing.T) {
	s := "(שלום)"
	vis := VisualString(s)
	expected := "(םולש)"
	if vis != expected {
		t.Errorf("Bracket mirroring failed: expected %q, got %q", expected, vis)
	}
}

func TestBidi_Width_Invariant(t *testing.T) {
	tests := []string{
		"שלום",
		"שלום hello",
		"hello שלום",
		"(שלום)",
		"abc مرحبا def",
	}
	for _, tc := range tests {
		wLogical := StringWidth(tc)
		vis := VisualString(tc)
		wVisual := StringWidth(vis)
		if wLogical != wVisual {
			t.Errorf("Width invariant violated for %q: logical width %d != visual width %d (visual: %q)", tc, wLogical, wVisual, vis)
		}
	}
}
