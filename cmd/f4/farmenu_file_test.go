package main

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

func parseFarMenuString(t *testing.T, s string) []UserMenuItem {
	t.Helper()
	items, err := ParseFarMenu(strings.NewReader(s))
	if err != nil {
		t.Fatalf("ParseFarMenu: %v", err)
	}
	return items
}

func TestParseFarMenu_Empty(t *testing.T) {
	for _, in := range []string{"", "\r\n", "\n\n\n", "   \r\n   \r\n"} {
		items := parseFarMenuString(t, in)
		if len(items) != 0 {
			t.Errorf("empty input %q yielded %v", in, items)
		}
	}
}

func TestParseFarMenu_OneItem_OneCommand(t *testing.T) {
	got := parseFarMenuString(t, "a:  Apple\r\n    eat\r\n")
	want := []UserMenuItem{{HotKey: "a", Label: "Apple", Commands: []string{"eat"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseFarMenu_MultipleItems(t *testing.T) {
	got := parseFarMenuString(t, ""+
		"a:  Apple\r\n"+
		"    cmd1\r\n"+
		"b:  Banana\r\n"+
		"    cmd2\r\n"+
		"    cmd3\r\n")
	if len(got) != 2 {
		t.Fatalf("want 2 items, got %d (%#v)", len(got), got)
	}
	if got[0].Label != "Apple" || got[1].Label != "Banana" {
		t.Errorf("labels: %q / %q", got[0].Label, got[1].Label)
	}
	if !reflect.DeepEqual(got[1].Commands, []string{"cmd2", "cmd3"}) {
		t.Errorf("multi commands lost: %#v", got[1].Commands)
	}
}

func TestParseFarMenu_BlankLinesIgnored(t *testing.T) {
	got := parseFarMenuString(t, ""+
		"\r\n"+
		"a:  Apple\r\n"+
		"\r\n"+
		"    cmd1\r\n"+
		"\r\n"+
		"    cmd2\r\n"+
		"\r\n"+
		"b:  Banana\r\n")
	if len(got) != 2 {
		t.Fatalf("want 2 items, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].Commands, []string{"cmd1", "cmd2"}) {
		t.Errorf("blanks broke command grouping: %#v", got[0].Commands)
	}
}

func TestParseFarMenu_Submenu(t *testing.T) {
	got := parseFarMenuString(t, ""+
		"m:  Menu\r\n"+
		"{\r\n"+
		"a:  Apple\r\n"+
		"    eat\r\n"+
		"b:  Banana\r\n"+
		"    peel\r\n"+
		"}\r\n")
	if len(got) != 1 || !got[0].IsSubmenu() {
		t.Fatalf("want submenu, got %#v", got)
	}
	sub := got[0].Submenu
	if len(sub) != 2 || sub[0].Label != "Apple" || sub[1].Label != "Banana" {
		t.Fatalf("submenu items wrong: %#v", sub)
	}
	if sub[0].Commands[0] != "eat" || sub[1].Commands[0] != "peel" {
		t.Errorf("commands lost: %#v", sub)
	}
}

func TestParseFarMenu_EmptySubmenu(t *testing.T) {
	got := parseFarMenuString(t, "m:  Menu\r\n{\r\n}\r\n")
	if len(got) != 1 || !got[0].IsSubmenu() {
		t.Fatalf("want submenu, got %#v", got)
	}
	if len(got[0].Submenu) != 0 {
		t.Errorf("expected empty submenu, got %d children", len(got[0].Submenu))
	}
}

func TestParseFarMenu_NestedSubmenus(t *testing.T) {
	got := parseFarMenuString(t, ""+
		"a:  A\r\n"+
		"{\r\n"+
		"b:  B\r\n"+
		"{\r\n"+
		"c:  C\r\n"+
		"    deep\r\n"+
		"}\r\n"+
		"}\r\n")
	if len(got) != 1 || !got[0].IsSubmenu() {
		t.Fatalf("top: %#v", got)
	}
	mid := got[0].Submenu
	if len(mid) != 1 || !mid[0].IsSubmenu() {
		t.Fatalf("mid: %#v", mid)
	}
	leaf := mid[0].Submenu
	if len(leaf) != 1 || leaf[0].Label != "C" || leaf[0].Commands[0] != "deep" {
		t.Fatalf("leaf: %#v", leaf)
	}
}

func TestParseFarMenu_Separator(t *testing.T) {
	got := parseFarMenuString(t, "--:  \r\n")
	if len(got) != 1 || !got[0].IsSeparator() || got[0].Label != "" {
		t.Fatalf("separator: %#v", got)
	}
}

func TestParseFarMenu_ColonInLabel(t *testing.T) {
	// far2l splits on the FIRST colon; everything after is the label.
	got := parseFarMenuString(t, "a:  foo: bar: baz\r\n    cmd\r\n")
	if len(got) != 1 || got[0].HotKey != "a" || got[0].Label != "foo: bar: baz" {
		t.Fatalf("first-colon split broken: %#v", got)
	}
}

func TestParseFarMenu_NoColonSkipped(t *testing.T) {
	got := parseFarMenuString(t, "this is junk\r\na:  Apple\r\n    cmd\r\n")
	if len(got) != 1 || got[0].Label != "Apple" {
		t.Fatalf("junk lines should be skipped: %#v", got)
	}
}

func TestParseFarMenu_TabIndentedCommand(t *testing.T) {
	got := parseFarMenuString(t, "a:  Apple\r\n\tcmd\r\n")
	if len(got) != 1 || len(got[0].Commands) != 1 || got[0].Commands[0] != "cmd" {
		t.Fatalf("tab indent not treated as command: %#v", got)
	}
}

func TestParseFarMenu_UTF8WithBOM(t *testing.T) {
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte("a:  Apple\r\n    cmd\r\n")...)
	got, err := ParseFarMenu(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Label != "Apple" {
		t.Fatalf("UTF-8 BOM not stripped: %#v", got)
	}
}

func TestParseFarMenu_UTF16LEWithBOM(t *testing.T) {
	// This is the format far2l actually writes.
	body := "a:  Яблоко\r\n    cmd\r\n"
	u16 := utf16.Encode([]rune(body))
	var buf bytes.Buffer
	if _, err := buf.Write([]byte{0xFF, 0xFE}); err != nil {
		t.Fatal(err)
	}
	for _, w := range u16 {
		_ = binary.Write(&buf, binary.LittleEndian, w)
	}
	got, err := ParseFarMenu(&buf)
	if err != nil {
		t.Fatalf("decode UTF-16LE: %v", err)
	}
	if len(got) != 1 || got[0].Label != "Яблоко" {
		t.Fatalf("UTF-16LE decoded wrong: %#v", got)
	}
}

func TestParseFarMenu_UTF16BEWithBOM(t *testing.T) {
	body := "a:  X\r\n"
	u16 := utf16.Encode([]rune(body))
	var buf bytes.Buffer
	if _, err := buf.Write([]byte{0xFE, 0xFF}); err != nil {
		t.Fatal(err)
	}
	for _, w := range u16 {
		_ = binary.Write(&buf, binary.BigEndian, w)
	}
	got, _ := ParseFarMenu(&buf)
	if len(got) != 1 || got[0].HotKey != "a" || got[0].Label != "X" {
		t.Fatalf("UTF-16BE decoded wrong: %#v", got)
	}
}

func TestParseFarMenu_UTF32LEWithBOM(t *testing.T) {
	// This is what far2l actually writes on Linux/macOS/BSD: SIGN_WIDE_LE
	// emitted as a single 4-byte wchar_t. Looks like FF FE 00 00 ... .
	body := "c:  code\r\n    code .\r\n"
	var buf bytes.Buffer
	if _, err := buf.Write([]byte{0xFF, 0xFE, 0x00, 0x00}); err != nil {
		t.Fatal(err)
	}
	for _, r := range body {
		_ = binary.Write(&buf, binary.LittleEndian, testUint32Rune(r))
	}
	got, err := ParseFarMenu(&buf)
	if err != nil {
		t.Fatalf("UTF-32LE decode: %v", err)
	}
	if len(got) != 1 || got[0].HotKey != "c" || got[0].Label != "code" ||
		len(got[0].Commands) != 1 || got[0].Commands[0] != "code ." {
		t.Fatalf("UTF-32LE parsed wrong: %#v", got)
	}
}

func TestParseFarMenu_UTF32LEWithCyrillic(t *testing.T) {
	body := "a:  Яблоко\r\n"
	var buf bytes.Buffer
	if _, err := buf.Write([]byte{0xFF, 0xFE, 0x00, 0x00}); err != nil {
		t.Fatal(err)
	}
	for _, r := range body {
		_ = binary.Write(&buf, binary.LittleEndian, testUint32Rune(r))
	}
	got, _ := ParseFarMenu(&buf)
	if len(got) != 1 || got[0].Label != "Яблоко" {
		t.Fatalf("UTF-32LE cyrillic mangled: %#v", got)
	}
}

func TestParseFarMenu_UTF32BEWithBOM(t *testing.T) {
	body := "a:  X\r\n"
	var buf bytes.Buffer
	if _, err := buf.Write([]byte{0x00, 0x00, 0xFE, 0xFF}); err != nil {
		t.Fatal(err)
	}
	for _, r := range body {
		_ = binary.Write(&buf, binary.BigEndian, testUint32Rune(r))
	}
	got, _ := ParseFarMenu(&buf)
	if len(got) != 1 || got[0].HotKey != "a" || got[0].Label != "X" {
		t.Fatalf("UTF-32BE decoded wrong: %#v", got)
	}
}

// Disambiguation check: UTF-32LE BOM is "FF FE 00 00" and UTF-16LE BOM
// is "FF FE", so the wider format must be detected first or else a real
// UTF-32 file gets misparsed as UTF-16 (every other char appears empty,
// which is exactly the symptom seen on far2l-authored FarMenu.ini on
// Linux).
func TestParseFarMenu_UTF32LE_NotMistakenForUTF16LE(t *testing.T) {
	body := "a:  Apple\r\n"
	var buf bytes.Buffer
	if _, err := buf.Write([]byte{0xFF, 0xFE, 0x00, 0x00}); err != nil {
		t.Fatal(err)
	}
	for _, r := range body {
		_ = binary.Write(&buf, binary.LittleEndian, testUint32Rune(r))
	}
	got, _ := ParseFarMenu(&buf)
	if len(got) != 1 || got[0].Label != "Apple" {
		t.Fatalf("UTF-32LE misparsed (likely as UTF-16LE): %#v", got)
	}
}

func TestParseFarMenu_LFOnlyLineEndings(t *testing.T) {
	got := parseFarMenuString(t, "a:  Apple\n    cmd\nb:  Banana\n")
	if len(got) != 2 || got[0].Commands[0] != "cmd" || got[1].Label != "Banana" {
		t.Fatalf("LF-only lines broken: %#v", got)
	}
}

func TestParseFarMenu_LoneCRLineEndings(t *testing.T) {
	got := parseFarMenuString(t, "a:  Apple\r    cmd\rb:  Banana\r")
	if len(got) != 2 || got[0].Commands[0] != "cmd" || got[1].Label != "Banana" {
		t.Fatalf("CR-only lines broken: %#v", got)
	}
}

func TestParseFarMenu_InvalidUTF8(t *testing.T) {
	if _, err := ParseFarMenu(bytes.NewReader([]byte{0x80, 0xFF, 0xC0})); err == nil {
		t.Errorf("expected error on garbage bytes with no BOM")
	}
}

func TestRenderFarMenuText_OneItem(t *testing.T) {
	got := renderFarMenuText([]UserMenuItem{
		{HotKey: "a", Label: "Apple", Commands: []string{"eat"}},
	})
	want := "a:  Apple\r\n    eat\r\n"
	if got != want {
		t.Fatalf("output mismatch:\nGOT  %q\nWANT %q", got, want)
	}
}

func TestRenderFarMenuText_Submenu(t *testing.T) {
	got := renderFarMenuText([]UserMenuItem{
		{HotKey: "m", Label: "Menu", Submenu: []UserMenuItem{
			{HotKey: "a", Label: "A", Commands: []string{"x"}},
		}},
	})
	want := "m:  Menu\r\n{\r\na:  A\r\n    x\r\n}\r\n"
	if got != want {
		t.Fatalf("submenu output:\nGOT  %q\nWANT %q", got, want)
	}
}

func TestRenderFarMenuText_Separator(t *testing.T) {
	got := renderFarMenuText([]UserMenuItem{
		{HotKey: "--", Label: ""},
	})
	if got != "--:  \r\n" {
		t.Fatalf("separator output: %q", got)
	}
}

func TestWriteFarMenu_PlatformEncodingHasBOM(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFarMenu(&buf, []UserMenuItem{
		{HotKey: "a", Label: "Apple", Commands: []string{"eat"}},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	b := buf.Bytes()
	switch runtime.GOOS {
	case "windows":
		if !bytes.HasPrefix(b, []byte{0xFF, 0xFE}) {
			t.Errorf("Windows build should write UTF-16LE BOM, got %x", b[:min(4, len(b))])
		}
		if bytes.HasPrefix(b, []byte{0xFF, 0xFE, 0x00, 0x00}) {
			t.Errorf("Windows build wrote UTF-32 BOM instead of UTF-16")
		}
	default:
		if !bytes.HasPrefix(b, []byte{0xFF, 0xFE, 0x00, 0x00}) {
			t.Errorf("Unix build should write UTF-32LE BOM, got %x", b[:min(8, len(b))])
		}
	}
	// Round-trip via the multi-encoding reader regardless of platform.
	out, err := ParseFarMenu(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if len(out) != 1 || out[0].Label != "Apple" {
		t.Fatalf("round-trip mangled: %#v", out)
	}
}

func TestEncodeUTF16LEWithBOM(t *testing.T) {
	got := encodeUTF16LEWithBOM("c:")
	want := []byte{0xFF, 0xFE, 'c', 0x00, ':', 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestEncodeUTF32LEWithBOM(t *testing.T) {
	got := encodeUTF32LEWithBOM("c:")
	want := []byte{
		0xFF, 0xFE, 0x00, 0x00,
		'c', 0x00, 0x00, 0x00,
		':', 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestEncode_CyrillicSurvivesBothEncodings(t *testing.T) {
	src := "Яблоко"
	for _, c := range []struct {
		name string
		fn   func(string) []byte
	}{
		{"UTF-16LE", encodeUTF16LEWithBOM},
		{"UTF-32LE", encodeUTF32LEWithBOM},
	} {
		decoded, err := decodeFarMenuBytes(c.fn(src))
		if err != nil {
			t.Errorf("%s decode: %v", c.name, err)
			continue
		}
		if decoded != src {
			t.Errorf("%s round-trip: got %q want %q", c.name, decoded, src)
		}
	}
}

func TestFarMenu_RoundTrip(t *testing.T) {
	in := []UserMenuItem{
		{HotKey: "c", Label: "VSCode", Commands: []string{"code ."}},
		{HotKey: "t", Label: "tmux", Submenu: []UserMenuItem{
			{HotKey: "l", Label: "list", Commands: []string{"tmux ls"}},
			{HotKey: "a", Label: "attach", Commands: []string{"tmux attach"}},
		}},
		{HotKey: "--", Label: ""},
		{HotKey: "x", Label: "exit", Commands: []string{"exit"}},
	}
	var buf bytes.Buffer
	if err := WriteFarMenu(&buf, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := ParseFarMenu(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip mismatch:\ngot  %#v\nwant %#v", out, in)
	}
}

func TestFarMenu_CrossFormatRoundTrip(t *testing.T) {
	// Same tree should survive: model -> FarMenu.ini -> model
	//                          -> main_menu.ini -> model
	// All three representations must reconstruct the same structure.
	tree := []UserMenuItem{
		{HotKey: "1", Label: "one", Commands: []string{"echo 1"}},
		{HotKey: "g", Label: "group", Submenu: []UserMenuItem{
			{HotKey: "a", Label: "alpha", Commands: []string{"echo a", "echo b"}},
		}},
		{HotKey: "--", Label: ""},
		{HotKey: "z", Label: "last", Commands: []string{"echo z"}},
	}

	var txt bytes.Buffer
	if err := WriteFarMenu(&txt, tree); err != nil {
		t.Fatalf("WriteFarMenu: %v", err)
	}
	viaText, err := ParseFarMenu(bytes.NewReader(txt.Bytes()))
	if err != nil {
		t.Fatalf("ParseFarMenu: %v", err)
	}
	if !reflect.DeepEqual(viaText, tree) {
		t.Fatalf("FarMenu.ini round-trip lost data:\ngot  %#v\nwant %#v", viaText, tree)
	}

	p := writeTemp(t, "")
	if err := SaveMainMenu(p, tree); err != nil {
		t.Fatalf("SaveMainMenu: %v", err)
	}
	viaINI, err := LoadMainMenu(p)
	if err != nil {
		t.Fatalf("LoadMainMenu: %v", err)
	}
	if !reflect.DeepEqual(viaINI, tree) {
		t.Fatalf("main_menu.ini round-trip lost data:\ngot  %#v\nwant %#v", viaINI, tree)
	}
}

func TestRenderFarMenuText_MatchesFar2lLayout(t *testing.T) {
	// Confirm the canonical text we render mirrors what far2l's
	// MenuRegToFile emits modulo encoding. Reference is pieced together
	// from usermenu.cpp:103-137. WriteFarMenu wraps this in the
	// platform's wide encoding; the layout itself must stay identical.
	items := []UserMenuItem{
		{HotKey: "c", Label: "VSCode", Commands: []string{"code ."}},
		{HotKey: "t", Label: "tmux", Submenu: []UserMenuItem{
			{HotKey: "l", Label: "list", Commands: []string{"tmux ls"}},
		}},
	}
	got := renderFarMenuText(items)
	want := "c:  VSCode\r\n" +
		"    code .\r\n" +
		"t:  tmux\r\n" +
		"{\r\n" +
		"l:  list\r\n" +
		"    tmux ls\r\n" +
		"}\r\n"
	if got != want {
		t.Fatalf("text-layout drift:\nGOT  %q\nWANT %q", got, want)
	}
}
