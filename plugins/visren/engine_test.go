package visren

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func testItem(name string) *Item {
	item := NewItem(".", name, time.Date(2024, 3, 4, 5, 6, 7, 0, time.Local), false)
	item.Random = 123
	return item
}

func render(t *testing.T, source, nameMask, extMask string) string {
	t.Helper()
	e := Engine{Items: []*Item{testItem(source)}}
	p, err := e.Build(Options{NameMask: nameMask, ExtMask: extMask, CaseSensitive: true, WordDiv: "-. _&"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return p[0].Destination
}

func TestSplitName(t *testing.T) {
	tests := []struct{ in, base, ext string }{
		{"name.txt", "name", "txt"},
		{"archive.tar.gz", "archive.tar", "gz"},
		{".gitignore", "", "gitignore"},
		{"name.", "name.", ""},
		{"name", "name", ""},
	}
	for _, tt := range tests {
		base, ext := SplitName(tt.in)
		if base != tt.base || ext != tt.ext {
			t.Errorf("SplitName(%q)=(%q,%q), want (%q,%q)", tt.in, base, ext, tt.base, tt.ext)
		}
	}
}

func TestRanges(t *testing.T) {
	tests := map[string]string{
		"[N2]": "b", "[N2-5]": "bcde", "[N2,5]": "bcdef", "[N2-]": "bcdefghij",
		"[N-2]": "i", "[N2--5]": "bcdef", "[N-8-5]": "cde", "[N-8,5]": "cdefg",
		"[N-5-]": "fghij",
	}
	for mask, want := range tests {
		if got := render(t, "abcdefghij.txt", mask, ""); got != want {
			t.Errorf("%s => %q, want %q", mask, got, want)
		}
	}
}

func TestCountersAndEscapes(t *testing.T) {
	items := []*Item{testItem("a.txt"), testItem("b.txt"), testItem("c.txt")}
	e := Engine{Items: items}
	p, err := e.Build(Options{NameMask: "[[]x[]]-[C001+5]", ExtMask: "[E]", CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[x]-001.txt", "[x]-006.txt", "[x]-011.txt"}
	for i := range want {
		if p[i].Destination != want[i] {
			t.Errorf("row %d: %q, want %q", i, p[i].Destination, want[i])
		}
	}
}

func TestTransformOrderAndFlags(t *testing.T) {
	e := Engine{Items: []*Item{testItem("hello_WORLD.txt")}}
	p, err := e.Build(Options{NameMask: "[N][T]", ExtMask: "[E][U]", Search: "_", Replace: "-", CaseSensitive: true, WordDiv: "-. _&"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p[0].Destination; got != "Hello-World.TXT" {
		t.Fatalf("got %q", got)
	}
	if got := render(t, "Привет.txt", "[N][TL]", "[E]"); got != "Privet.txt" {
		t.Fatalf("translit got %q", got)
	}
	if got := render(t, "shh.txt", "[N][TR]", "[E]"); got != "щ.txt" {
		t.Fatalf("reverse translit got %q", got)
	}
	if got := render(t, "Ёж.txt", "[N][TL]", "[E]"); got != "JOzh.txt" {
		t.Fatalf("uppercase translit got %q", got)
	}
	if got := render(t, "ARTIST И FRIEND - BIG SONG.txt", "[N][M]", "[E]"); got != "Artist и Friend - Big song.txt" {
		t.Fatalf("music case got %q", got)
	}
}

func TestReplacement(t *testing.T) {
	item := testItem("File-123.txt")
	e := Engine{Items: []*Item{item}}
	plain, err := e.Build(Options{NameMask: "[N]", ExtMask: "[E]", Search: "file", Replace: "doc", CaseSensitive: false})
	if err != nil || plain[0].Destination != "doc-123.txt" {
		t.Fatalf("plain=%v err=%v", plain, err)
	}
	if want := []TextRange{{0, 3}}; !reflect.DeepEqual(plain[0].ReplacementMatches, want) {
		t.Fatalf("plain replacement ranges=%v, want %v", plain[0].ReplacementMatches, want)
	}
	regex, err := e.Build(Options{NameMask: "[N]", ExtMask: "[E]", Search: `/([A-Za-z]+)-(\d+)/`, Replace: "$2-$1", Regex: true, CaseSensitive: true})
	if err != nil || regex[0].Destination != "123-File.txt" {
		t.Fatalf("regex=%v err=%v", regex, err)
	}
	if want := []TextRange{{0, 8}}; !reflect.DeepEqual(regex[0].ReplacementMatches, want) {
		t.Fatalf("regex replacement ranges=%v, want %v", regex[0].ReplacementMatches, want)
	}
	bad, err := e.Build(Options{NameMask: "[N]", ExtMask: "[E]", Search: "/[/", Regex: true})
	if err == nil || bad[0].Destination != errorPreview {
		t.Fatalf("invalid regex was not diagnosed: %v, %v", bad, err)
	}
	slashItem := Engine{Items: []*Item{testItem("a-b.txt")}}
	slash, err := slashItem.Build(Options{NameMask: "[N]", ExtMask: "[E]", Search: `/a\-b/`, Replace: "ok", Regex: true, CaseSensitive: true})
	if err != nil || slash[0].Destination != "ok.txt" {
		t.Fatalf("escaped regex=%v err=%v", slash, err)
	}
}

func TestReplacementHighlightRanges(t *testing.T) {
	tests := []struct {
		name   string
		source string
		opts   Options
		want   string
		ranges []TextRange
	}{
		{
			name:   "multiple replacements",
			source: "banana.txt",
			opts:   Options{NameMask: "[N]", ExtMask: "[E]", Search: "a", Replace: "XYZ", CaseSensitive: true},
			want:   "bXYZnXYZnXYZ.txt",
			ranges: []TextRange{{1, 4}, {5, 8}, {9, 12}},
		},
		{
			name:   "empty replacement",
			source: "banana.txt",
			opts:   Options{NameMask: "[N]", ExtMask: "[E]", Search: "a", CaseSensitive: true},
			want:   "bnn.txt",
		},
		{
			name:   "separator inserted by replacement",
			source: "archive_txt",
			opts:   Options{NameMask: "[N]", Search: "_", Replace: ".", CaseSensitive: true},
			want:   "archive.txt",
			ranges: []TextRange{{7, 8}},
		},
		{
			name:   "transliteration changes replacement width",
			source: "жук.txt",
			opts:   Options{NameMask: "[N][TL]", ExtMask: "[E]", Search: "ж", Replace: "щ", CaseSensitive: true},
			want:   "shhuk.txt",
			ranges: []TextRange{{0, 3}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine := Engine{Items: []*Item{testItem(tc.source)}}
			preview, err := engine.Build(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := preview[0].Destination; got != tc.want {
				t.Fatalf("destination=%q, want %q", got, tc.want)
			}
			if got := preview[0].ReplacementMatches; !reflect.DeepEqual(got, tc.ranges) {
				t.Fatalf("replacement ranges=%v, want %v", got, tc.ranges)
			}
		})
	}
}

func TestSearchMatchRanges(t *testing.T) {
	tests := []struct {
		name  string
		opts  Options
		input string
		want  []TextRange
	}{
		{name: "plain all", opts: Options{Search: "an", CaseSensitive: true}, input: "banana", want: []TextRange{{1, 3}, {3, 5}}},
		{name: "plain folded", opts: Options{Search: "FILE"}, input: "File-file", want: []TextRange{{0, 4}, {5, 9}}},
		{name: "regex unicode", opts: Options{Search: `/ж./`, Regex: true, CaseSensitive: true}, input: "aжук", want: []TextRange{{1, 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, err := compileReplacement(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			got := program.matches(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("matches=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestStableRandomFollowsItem(t *testing.T) {
	a, b := testItem("a"), testItem("b")
	a.Random, b.Random = 10, 20
	e := Engine{Items: []*Item{a, b}}
	first, _ := e.Build(Options{NameMask: "[N]-[R]"})
	e.Items[0], e.Items[1] = e.Items[1], e.Items[0]
	second, _ := e.Build(Options{NameMask: "[N]-[R]"})
	if first[0].Destination != "a-10" || second[0].Destination != "b-20" {
		t.Fatalf("random did not follow item: %v / %v", first, second)
	}
}

func TestValidation(t *testing.T) {
	bad := []string{"", ".", "..", "a/b", "a\\b", "x?", "name.", "name ", "CON", "con.txt", "LPT9.log", "a\x01b"}
	for _, name := range bad {
		if err := ValidateFilename(name); err == nil {
			t.Errorf("ValidateFilename(%q) succeeded", name)
		}
	}
	for _, name := range []string{"normal.txt", ".gitignore", "Пример 1.txt"} {
		if err := ValidateFilename(name); err != nil {
			t.Errorf("ValidateFilename(%q): %v", name, err)
		}
	}
}

func TestMaskErrorsAllRows(t *testing.T) {
	e := Engine{Items: []*Item{testItem("a"), testItem("b")}}
	preview, err := e.Build(Options{NameMask: "[N0]"})
	if err == nil {
		t.Fatal("expected mask error")
	}
	for _, row := range preview {
		if row.Destination != errorPreview || row.Err == nil || !strings.Contains(row.Err.Error(), "range") {
			t.Fatalf("bad error row: %#v", row)
		}
	}
}
