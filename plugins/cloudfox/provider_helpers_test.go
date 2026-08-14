package cloudfox

import "testing"

func TestStrongETagAcceptsOnlyOneRFCEntityTag(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: `"version-one"`, want: `"version-one"`},
		{value: ` "comma,inside" `, want: `"comma,inside"`},
		{value: "\"obs-\x80\"", want: "\"obs-\x80\""},
		{value: `W/"weak"`},
		{value: `w/"weak"`},
		{value: `*`},
		{value: `unquoted`},
		{value: `"one", "two"`},
		{value: "\"control\x7f\""},
		{value: "\"space inside\""},
		{value: `"embedded"quote"`},
	} {
		if got := strongETag(test.value); got != test.want {
			t.Errorf("strongETag(%q)=%q, want %q", test.value, got, test.want)
		}
	}
}

func TestCacheETagAcceptsValidStrongAndWeakValidatorsOnly(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: `"strong"`, want: `"strong"`},
		{value: ` W/"weak" `, want: `W/"weak"`},
		{value: `*`},
		{value: `garbage`},
		{value: `w/"lowercase-invalid"`},
		{value: `W/ "space-invalid"`},
		{value: `W/"one", "two"`},
	} {
		if got := cacheETag(test.value); got != test.want {
			t.Errorf("cacheETag(%q)=%q, want %q", test.value, got, test.want)
		}
	}
}

func TestWeakETagComparisonUsesOpaqueTag(t *testing.T) {
	t.Parallel()
	if !weakETagEqual(`W/"same"`, `"same"`) {
		t.Fatal("weak and strong forms of the same opaque tag did not match")
	}
	for _, pair := range [][2]string{{`"one"`, `"two"`}, {`*`, `*`}, {`garbage`, `garbage`}} {
		if weakETagEqual(pair[0], pair[1]) {
			t.Fatalf("weakETagEqual(%q,%q)=true", pair[0], pair[1])
		}
	}
}
