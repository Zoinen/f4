package editor

import (
	"bytes"
	"testing"
)

func TestReplaceAllFoldEmptyOldCoverageBatch27(t *testing.T) {
	input := "unchanged"
	if got := replaceAllFold(input, "", "new"); got != input {
		t.Fatalf("replaceAllFold(empty old) = %q, want %q", got, input)
	}
}

func TestReplaceAllFoldASCIIInsensitiveCoverageBatch27(t *testing.T) {
	if got := replaceAllFold("One TWO one", "one", "x"); got != "x TWO x" {
		t.Fatalf("replaceAllFold ASCII = %q, want %q", got, "x TWO x")
	}
}

func TestReplaceAllFoldUnicodeSimpleFoldCoverageBatch27(t *testing.T) {
	if got := replaceAllFold("k K K", "k", "x"); got != "x x x" {
		t.Fatalf("replaceAllFold Unicode = %q, want %q", got, "x x x")
	}
}

func TestReplaceAllFoldPreservesUnmatchedTextCoverageBatch27(t *testing.T) {
	if got := replaceAllFold("prefix abc suffix abc", "abc", "replacement"); got != "prefix replacement suffix replacement" {
		t.Fatalf("replaceAllFold unmatched text = %q", got)
	}
}

func TestParseHexPatternWildcardsCoverageBatch27(t *testing.T) {
	got, err := parseHexPatternToRegex("A ? 0f ??")
	if err != nil {
		t.Fatal(err)
	}
	if got != `(?s)\x0a.\x0f.` {
		t.Fatalf("parseHexPatternToRegex = %q, want %q", got, `(?s)\x0a.\x0f.`)
	}
}

func TestParseHexPatternRejectsInvalidTokenCoverageBatch27(t *testing.T) {
	if _, err := parseHexPatternToRegex("aa nope"); err == nil {
		t.Fatal("parseHexPatternToRegex accepted an invalid token")
	}
}

func TestParseHexReplacementTokensCoverageBatch27(t *testing.T) {
	got, err := parseHexReplacement("A 0f ff")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0x0a, 0x0f, 0xff}) {
		t.Fatalf("parseHexReplacement = %x, want 0a0fff", got)
	}
}

func TestParseHexReplacementRejectsInvalidTokenCoverageBatch27(t *testing.T) {
	if _, err := parseHexReplacement("gg"); err == nil {
		t.Fatal("parseHexReplacement accepted an invalid token")
	}
}

func TestTrailingLineTerminatorFormsCoverageBatch27(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want []byte
	}{
		{name: "lf", data: []byte("line\n"), want: []byte("\n")},
		{name: "crlf", data: []byte("line\r\n"), want: []byte("\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := trailingLineTerminator(tc.data); !bytes.Equal(got, tc.want) {
				t.Fatalf("trailingLineTerminator(%q) = %q, want %q", tc.data, got, tc.want)
			}
		})
	}
}

func TestTrailingLineTerminatorRejectsMissingTerminatorCoverageBatch27(t *testing.T) {
	for _, data := range [][]byte{nil, {}, []byte("line"), []byte("line\r")} {
		if got := trailingLineTerminator(data); got != nil {
			t.Fatalf("trailingLineTerminator(%q) = %q, want nil", data, got)
		}
	}
}
