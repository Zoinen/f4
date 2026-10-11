package visren

import (
	"reflect"
	"strings"
	"testing"
)

// TestRenderItemExtensionMaskError exercises the error path taken when the
// name mask expands fine but the extension mask does not (masks.go wraps it
// as "extension mask: ...", distinct from the already-covered "name mask"
// wrapping).
func TestRenderItemExtensionMaskError(t *testing.T) {
	e := Engine{Items: []*Item{testItem("name.txt")}}
	_, err := e.Build(Options{NameMask: "[N]", ExtMask: "[Q]", CaseSensitive: true})
	if err == nil || !strings.Contains(err.Error(), "extension mask") {
		t.Fatalf("expected extension mask error, got %v", err)
	}
}

// TestRenderItemExtensionMatchOffsets covers the branch of renderItem that
// offsets replacement ranges landing inside the (non-empty) extension after
// the name/extension split.
func TestRenderItemExtensionMatchOffsets(t *testing.T) {
	e := Engine{Items: []*Item{testItem("cat.aaa")}}
	p, err := e.Build(Options{NameMask: "[N]", ExtMask: "[E]", Search: "a", Replace: "A", CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := p[0].Destination; got != "cAt.AAA" {
		t.Fatalf("destination = %q", got)
	}
	want := []TextRange{{1, 2}, {4, 5}, {5, 6}, {6, 7}}
	if !reflect.DeepEqual(p[0].ReplacementMatches, want) {
		t.Fatalf("replacement ranges = %v, want %v", p[0].ReplacementMatches, want)
	}
}

func TestExpandTokenLiteralBrackets(t *testing.T) {
	item := testItem("a.txt")
	flags := &maskFlags{}
	if got, err := expandToken("[", "base", "ext", item, 0, flags); err != nil || got != "[" {
		t.Fatalf(`expandToken("[") = %q, %v`, got, err)
	}
	if got, err := expandToken("]", "base", "ext", item, 0, flags); err != nil || got != "]" {
		t.Fatalf(`expandToken("]") = %q, %v`, got, err)
	}
}

func TestExpandTokenFirstFlag(t *testing.T) {
	item := testItem("a.txt")
	flags := &maskFlags{}
	got, err := expandToken("F", "base", "ext", item, 0, flags)
	if err != nil || got != "" || !flags.first {
		t.Fatalf("expandToken(F) = %q, %v, flags=%+v", got, err, flags)
	}
}

// TestExpandTokenMetadataFields covers the metadata-backed single-letter
// tokens, including the safeMetadata sanitisation of values that contain
// characters forbidden in filenames.
func TestExpandTokenMetadataFields(t *testing.T) {
	item := testItem("song.mp3")
	item.metaOnce.Do(func() {}) // pin the cached metadata below, skip real I/O
	item.meta = Metadata{
		Track: "5", Title: "Bad<Title", Artist: "Artist Name", Album: "Al/bum",
		Year: "2024", Genre: "Rock", CameraMake: "Can*non", CameraModel: "EOS 5D",
		ImageDate: "2024:01:01", Width: 800, Height: 600, Version: "v1.2",
	}
	flags := &maskFlags{}
	cases := []struct{ token, want string }{
		{"#", "5"},
		{"t", ""}, // sanitised: contains '<'
		{"a", "Artist Name"},
		{"l", ""}, // sanitised: contains '/'
		{"y", "2024"},
		{"g", "Rock"},
		{"c", ""}, // sanitised: contains '*'
		{"m", "EOS 5D"},
		{"d", "2024:01:01"}, // ImageDate is not sanitised
		{"r", "800x600"},
		{"V", "v1.2"},
	}
	for _, tc := range cases {
		got, err := expandToken(tc.token, "base", "ext", item, 0, flags)
		if err != nil || got != tc.want {
			t.Errorf("expandToken(%q) = %q, %v; want %q", tc.token, got, err, tc.want)
		}
	}
}

func TestExpandTokenImageDimensionsMissing(t *testing.T) {
	item := testItem("photo.jpg")
	item.metaOnce.Do(func() {})
	item.meta = Metadata{}
	got, err := expandToken("r", "base", "ext", item, 0, &maskFlags{})
	if err != nil || got != "" {
		t.Fatalf("expandToken(r) with no dimensions = %q, %v", got, err)
	}
}

func TestExpandCounterEdgeCases(t *testing.T) {
	if _, err := expandCounter("+5", 0); err == nil {
		t.Fatal("empty initial digits was accepted")
	}
	if _, err := expandCounter("1234567890", 0); err == nil {
		t.Fatal("overlong initial digits was accepted")
	}
	if _, err := expandCounter("5+", 0); err == nil {
		t.Fatal("empty step was accepted")
	}
	if _, err := expandCounter("1a+2", 0); err == nil {
		t.Fatal("non-digit initial was accepted")
	}
	if _, err := expandCounter("1+z", 0); err == nil {
		t.Fatal("non-numeric step was accepted")
	}
	if got, err := expandCounter("5+-1", 10); err != nil || got != "-5" {
		t.Fatalf("expandCounter(5+-1, 10) = %q, %v", got, err)
	}
}

// TestApplyRangeClampsOutOfBoundsIndices exercises the clamping of a range
// whose resolved start is negative and whose resolved end runs past the end
// of the source, both of which applyRange must clip instead of failing.
func TestApplyRangeClampsOutOfBoundsIndices(t *testing.T) {
	got, err := applyRange([]rune("abc"), "-5,10")
	if err != nil || got != "abc" {
		t.Fatalf("applyRange(-5,10) = %q, %v", got, err)
	}
}

func TestParseRangeInvalidDashForms(t *testing.T) {
	if _, _, err := parseRange("2-x", 5); err == nil {
		t.Fatal("dash range with invalid end was accepted")
	}
	if _, _, err := parseRange("x-", 5); err == nil {
		t.Fatal("trailing-dash range with invalid start was accepted")
	}
}
