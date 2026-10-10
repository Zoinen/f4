package vtui

import "testing"

func TestPolicy_String(t *testing.T) {
	cases := []struct {
		p    Policy
		want string
	}{
		{PolicyFixed, "fixed"},
		{PolicyMinimum, "minimum"},
		{PolicyMaximum, "maximum"},
		{PolicyPreferred, "preferred"},
		{PolicyExpanding, "expanding"},
		{Policy(99), "preferred"}, // unknown value falls back to the default branch
	}
	for _, c := range cases {
		if got := c.p.String(); got != c.want {
			t.Errorf("Policy(%d).String() = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestParsePolicy(t *testing.T) {
	cases := []struct {
		in   string
		want Policy
	}{
		{"fixed", PolicyFixed},
		{"Fixed", PolicyFixed},
		{"  minimum  ", PolicyMinimum},
		{"MAXIMUM", PolicyMaximum},
		{"expanding", PolicyExpanding},
		{"preferred", PolicyPreferred},
		{"", PolicyPreferred},
		{"bogus", PolicyPreferred},
	}
	for _, c := range cases {
		if got := ParsePolicy(c.in); got != c.want {
			t.Errorf("ParsePolicy(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParsePolicy_RoundTripsWithString(t *testing.T) {
	for _, p := range []Policy{PolicyFixed, PolicyMinimum, PolicyMaximum, PolicyExpanding, PolicyPreferred} {
		if got := ParsePolicy(p.String()); got != p {
			t.Errorf("round trip: ParsePolicy(%q) = %v, want %v", p.String(), got, p)
		}
	}
}
