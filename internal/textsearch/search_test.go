package textsearch

import "testing"

// TestFindMatchLiteral exercises the plain strings.Index/LastIndex path
// (regexp=false, wholeWord=false): forward and backward search, the "next"
// skip used by Find Next/Find Previous, the defensive clamp when startOff
// lands past the end of the buffer, and the byte-length adjustment
// case-insensitive folding needs when a match is a different width than
// the pattern (e.g. Kelvin sign U+212A folding to ASCII "k").
func TestFindMatchLiteral(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		pattern       string
		caseSensitive bool
		reverse       bool
		next          bool
		startOff      int
		wantOffset    int
		wantLen       int
	}{
		{
			name:          "forward from start finds first occurrence",
			data:          "abcabcabc",
			pattern:       "abc",
			caseSensitive: true,
			startOff:      0,
			wantOffset:    0,
			wantLen:       3,
		},
		{
			name:          "forward with next skips the match at startOff",
			data:          "abcabcabc",
			pattern:       "abc",
			caseSensitive: true,
			next:          true,
			startOff:      0,
			wantOffset:    3,
			wantLen:       3,
		},
		{
			name:          "forward pattern absent",
			data:          "abcabcabc",
			pattern:       "zzz",
			caseSensitive: true,
			startOff:      0,
			wantOffset:    -1,
			wantLen:       3,
		},
		{
			name:          "backward from end finds last occurrence",
			data:          "abcabcabc",
			pattern:       "abc",
			caseSensitive: true,
			reverse:       true,
			startOff:      9,
			wantOffset:    6,
			wantLen:       3,
		},
		{
			name:          "backward with next skips the match before startOff",
			data:          "abcabcabc",
			pattern:       "abc",
			caseSensitive: true,
			reverse:       true,
			next:          true,
			startOff:      9,
			wantOffset:    3,
			wantLen:       3,
		},
		{
			name:          "backward startOff past end of buffer clamps instead of missing",
			data:          "abcabcabc",
			pattern:       "abc",
			caseSensitive: true,
			reverse:       true,
			startOff:      100,
			wantOffset:    6,
			wantLen:       3,
		},
		{
			name:          "backward from offset zero has nothing before it",
			data:          "abcabcabc",
			pattern:       "abc",
			caseSensitive: true,
			reverse:       true,
			startOff:      0,
			wantOffset:    -1,
			wantLen:       3,
		},
		{
			name:          "case-insensitive fold widens the match past the pattern length",
			data:          "a\u212A", // 'a' + Kelvin sign K (3 bytes), folds to "ak"
			pattern:       "k",
			caseSensitive: false,
			startOff:      0,
			wantOffset:    1,
			wantLen:       3,
		},
		{
			name:          "case-insensitive backward finds the later occurrence",
			data:          "Hello World hello",
			pattern:       "hello",
			caseSensitive: false,
			reverse:       true,
			startOff:      len("Hello World hello"),
			wantOffset:    12,
			wantLen:       5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotOffset, gotLen, err := FindMatch([]byte(tc.data), tc.pattern, tc.caseSensitive, tc.reverse, false, false, tc.next, tc.startOff)
			if err != nil {
				t.Fatalf("FindMatch(%q, %q) unexpected error: %v", tc.data, tc.pattern, err)
			}
			if gotOffset != tc.wantOffset || gotLen != tc.wantLen {
				t.Errorf("FindMatch(%q, %q, cs=%v, rev=%v, next=%v, start=%d) = (%d, %d), want (%d, %d)",
					tc.data, tc.pattern, tc.caseSensitive, tc.reverse, tc.next, tc.startOff,
					gotOffset, gotLen, tc.wantOffset, tc.wantLen)
			}
		})
	}
}

// TestFindMatchRegex exercises the coregex path, taken whenever regexp or
// wholeWord is set: forward/backward search, the "next" skip in both
// directions and the startOff-past-end clamp on the backward branch mirror
// the literal-path cases above but go through FindIndex/FindAllIndex
// instead of strings.Index/LastIndex.
func TestFindMatchRegex(t *testing.T) {
	// "xxfooxxfooxx": "foo" occurs at byte offsets 2 and 7.
	const data = "xxfooxxfooxx"

	tests := []struct {
		name       string
		pattern    string
		reverse    bool
		next       bool
		startOff   int
		wantOffset int
		wantLen    int
	}{
		{
			name:       "forward from start finds first occurrence",
			pattern:    "foo",
			startOff:   0,
			wantOffset: 2,
			wantLen:    3,
		},
		{
			name:       "forward with next skips the match at startOff",
			pattern:    "foo",
			next:       true,
			startOff:   2,
			wantOffset: 7,
			wantLen:    3,
		},
		{
			name:       "forward startOff at end of buffer finds nothing",
			pattern:    "foo",
			startOff:   len(data),
			wantOffset: -1,
			wantLen:    3,
		},
		{
			name:       "backward from end finds last occurrence",
			pattern:    "foo",
			reverse:    true,
			startOff:   len(data),
			wantOffset: 7,
			wantLen:    3,
		},
		{
			name:       "backward with next skips the match before startOff",
			pattern:    "foo",
			reverse:    true,
			next:       true,
			startOff:   7,
			wantOffset: 2,
			wantLen:    3,
		},
		{
			name:       "backward startOff past end of buffer clamps instead of missing",
			pattern:    "foo",
			reverse:    true,
			startOff:   100,
			wantOffset: 7,
			wantLen:    3,
		},
		{
			name:       "backward from offset zero has nothing before it",
			pattern:    "foo",
			reverse:    true,
			startOff:   0,
			wantOffset: -1,
			wantLen:    3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotOffset, gotLen, err := FindMatch([]byte(data), tc.pattern, true, tc.reverse, true, false, tc.next, tc.startOff)
			if err != nil {
				t.Fatalf("FindMatch(%q) unexpected error: %v", tc.pattern, err)
			}
			if gotOffset != tc.wantOffset || gotLen != tc.wantLen {
				t.Errorf("FindMatch(%q, rev=%v, next=%v, start=%d) = (%d, %d), want (%d, %d)",
					tc.pattern, tc.reverse, tc.next, tc.startOff, gotOffset, gotLen, tc.wantOffset, tc.wantLen)
			}
		})
	}
}

// TestFindMatchWholeWord checks that wholeWord alone (without regexp) still
// routes through the regex engine and rejects matches that are only a
// substring of a larger word.
func TestFindMatchWholeWord(t *testing.T) {
	data := "foofoo foo barfoo"

	gotOffset, gotLen, err := FindMatch([]byte(data), "foo", true, false, false, true, false, 0)
	if err != nil {
		t.Fatalf("FindMatch unexpected error: %v", err)
	}
	// "foofoo" (0) and "barfoo" (14) both contain "foo" without a word
	// boundary on at least one side; only the standalone "foo" at 7
	// qualifies.
	if gotOffset != 7 || gotLen != 3 {
		t.Errorf("FindMatch(wholeWord) = (%d, %d), want (7, 3)", gotOffset, gotLen)
	}
}

// TestFindMatchInvalidRegex checks that a broken pattern given with the
// regexp flag is reported as an error instead of panicking or silently
// returning "not found". wholeWord alone does not exercise this path:
// BuildSearchRegex quotes the pattern first whenever regexp is false, so
// unbalanced regex syntax in the input can never reach the compiler.
func TestFindMatchInvalidRegex(t *testing.T) {
	offset, length, err := FindMatch([]byte("abc"), "(unclosed", true, false, true, false, false, 0)
	if err == nil {
		t.Fatal("FindMatch with an unbalanced group did not return an error")
	}
	if offset != -1 || length != 0 {
		t.Errorf("FindMatch on error = (%d, %d), want (-1, 0)", offset, length)
	}
}

// TestBuildSearchRegex checks the three independent knobs BuildSearchRegex
// combines into one pattern: literal quoting of regex metacharacters,
// case-insensitivity via an (?i) prefix, and whole-word \b wrapping.
func TestBuildSearchRegex(t *testing.T) {
	t.Run("non-regex input is quoted literally", func(t *testing.T) {
		re, err := BuildSearchRegex("a.c", true, false, false)
		if err != nil {
			t.Fatalf("BuildSearchRegex returned error: %v", err)
		}
		if !re.MatchString("a.c") {
			t.Error(`expected literal "a.c" to match "a.c"`)
		}
		if re.MatchString("aXc") {
			t.Error(`expected literal "a.c" not to match "aXc" (dot must not act as wildcard)`)
		}
	})

	t.Run("regex input keeps metacharacters live", func(t *testing.T) {
		re, err := BuildSearchRegex("a.c", true, true, false)
		if err != nil {
			t.Fatalf("BuildSearchRegex returned error: %v", err)
		}
		if !re.MatchString("aXc") {
			t.Error(`expected regex "a.c" to match "aXc" (dot as wildcard)`)
		}
	})

	t.Run("case-insensitive adds an (?i) prefix", func(t *testing.T) {
		re, err := BuildSearchRegex("abc", false, true, false)
		if err != nil {
			t.Fatalf("BuildSearchRegex returned error: %v", err)
		}
		if !re.MatchString("ABC") {
			t.Error(`expected case-insensitive "abc" to match "ABC"`)
		}
	})

	t.Run("whole word rejects a partial match", func(t *testing.T) {
		re, err := BuildSearchRegex("cat", true, true, true)
		if err != nil {
			t.Fatalf("BuildSearchRegex returned error: %v", err)
		}
		if !re.MatchString("a cat sat") {
			t.Error(`expected whole-word "cat" to match "a cat sat"`)
		}
		if re.MatchString("category") {
			t.Error(`expected whole-word "cat" not to match inside "category"`)
		}
	})

	t.Run("invalid pattern surfaces a compile error", func(t *testing.T) {
		re, err := BuildSearchRegex("(unclosed", true, true, false)
		if err == nil {
			t.Fatal("expected an error for an unbalanced group")
		}
		if re != nil {
			t.Error("expected a nil regex alongside the error")
		}
	})
}

// TestBytesToString checks the unsafe []byte-to-string view: the explicit
// empty-input guard (unsafe.SliceData panics on a nil slice on some
// versions) and that non-empty input round-trips to the same content as
// the safe string() conversion.
func TestBytesToString(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "nil slice", data: nil},
		{name: "empty slice", data: []byte{}},
		{name: "ascii", data: []byte("hello, world")},
		{name: "multi-byte utf-8", data: []byte("日本語")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BytesToString(tc.data)
			want := string(tc.data)
			if got != want {
				t.Errorf("BytesToString(%v) = %q, want %q", tc.data, got, want)
			}
		})
	}
}
