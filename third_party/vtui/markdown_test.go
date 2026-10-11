package vtui

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// mdVisible is a line of help markup as the reader sees it.
func mdVisible(line string) string {
	cells, _ := parseHelpCells(line)
	var b strings.Builder
	for _, c := range cells {
		b.WriteRune(c.r)
	}
	return b.String()
}

// mdBold is the bold part of a line of help markup, bold runs separated by '|'.
func mdBold(line string) string {
	cells, _ := parseHelpCells(line)
	var b strings.Builder
	prev := false
	for _, c := range cells {
		if c.bold {
			if !prev && b.Len() > 0 {
				b.WriteByte('|')
			}
			b.WriteRune(c.r)
		}
		prev = c.bold
	}
	return b.String()
}

func mdVisibleLines(topic *HelpTopic) []string {
	out := make([]string, len(topic.Lines))
	for i, line := range topic.Lines {
		out[i] = mdVisible(line)
	}
	return out
}

func mdCheckLines(t *testing.T, md string, want []string) *HelpTopic {
	t.Helper()
	topic := ParseMarkdownTopic("Doc", md)
	if got := mdVisibleLines(topic); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible lines of %q =\n%#v\nwant\n%#v\n(markup %#v)", md, got, want, topic.Lines)
	}
	return topic
}

func TestParseMarkdownTopic_Heading(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "# Title\nBody text.")
	if topic.Name != "Doc" {
		t.Fatalf("Name = %q, want %q", topic.Name, "Doc")
	}
	wantLines := []string{"#Title#", "Body text."}
	if !reflect.DeepEqual(topic.Lines, wantLines) {
		t.Fatalf("Lines = %#v, want %#v", topic.Lines, wantLines)
	}
}

func TestParseMarkdownTopic_HeadingInsertsBlankLineAfterContent(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "Intro.\n## Section")
	want := []string{"Intro.", "", "#Section#"}
	if !reflect.DeepEqual(topic.Lines, want) {
		t.Fatalf("Lines = %#v, want %#v", topic.Lines, want)
	}
}

func TestParseMarkdownTopic_HeadingClosingHashesStripped(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "### Title ###")
	if got, want := topic.Lines[0], "#Title#"; got != want {
		t.Errorf("Lines[0] = %q, want %q", got, want)
	}
}

func TestParseMarkdownTopic_SetextHeading(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "Title\n=====\n\nBody")
	want := []string{"#Title#", "", "Body"}
	if !reflect.DeepEqual(topic.Lines, want) {
		t.Fatalf("Lines = %#v, want %#v", topic.Lines, want)
	}
}

func TestParseMarkdownTopic_Bold(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "This is **important** and __also this__.")
	want := "This is #important# and #also this#."
	if topic.Lines[0] != want {
		t.Errorf("Lines[0] = %q, want %q", topic.Lines[0], want)
	}
}

func TestParseMarkdownTopic_Italic(t *testing.T) {
	topic := mdCheckLines(t, "An *italic* word and _this_ one.", []string{"An italic word and this one."})
	if got := mdBold(topic.Lines[0]); got != "italic|this" {
		t.Errorf("highlighted = %q, want %q", got, "italic|this")
	}
}

func TestParseMarkdownTopic_NestedEmphasisStaysOneSpan(t *testing.T) {
	// The help markup's '#' is a toggle, so a span inside a span must not
	// write a second one and switch the highlight off half way.
	topic := mdCheckLines(t, "**bold *and italic* still bold** plain", []string{"bold and italic still bold plain"})
	if got := mdBold(topic.Lines[0]); got != "bold and italic still bold" {
		t.Errorf("highlighted = %q", got)
	}
}

func TestParseMarkdownTopic_InlineCode(t *testing.T) {
	topic := mdCheckLines(t, "Run `go test #1 ~x @y` now.", []string{"Run go test #1 ~x @y now."})
	if got := mdBold(topic.Lines[0]); got != "go test #1 ~x @y" {
		t.Errorf("highlighted = %q", got)
	}
	if len(topic.Links) != 0 {
		t.Errorf("Links = %#v, want none", topic.Links)
	}
}

func TestParseMarkdownTopic_FencedCodeBlockIsLiteral(t *testing.T) {
	md := "Before:\n\n```sh\n# comment, not a heading\nls ~/src **not bold** [x](y) user@host\n\n  indented\n```\nAfter."
	topic := mdCheckLines(t, md, []string{
		"Before:",
		"",
		"    # comment, not a heading",
		"    ls ~/src **not bold** [x](y) user@host",
		"",
		"      indented",
		"",
		"After.",
	})
	for i, line := range topic.Lines {
		if got := mdBold(line); got != "" {
			t.Errorf("line %d %q has highlighted text %q", i, line, got)
		}
	}
	if len(topic.Links) != 0 {
		t.Errorf("Links = %#v, want none", topic.Links)
	}
}

func TestParseMarkdownTopic_IndentedCodeBlock(t *testing.T) {
	mdCheckLines(t, "Text\n\n    # not a heading\n    a ~ b\n\nMore", []string{
		"Text", "", "    # not a heading", "    a ~ b", "", "More",
	})
}

func TestParseMarkdownTopic_CodeBlockTabsExpanded(t *testing.T) {
	mdCheckLines(t, "```\na\tb\n\tc\n```", []string{"    a   b", "        c"})
}

func TestMarkdownView_CodeBlockHashDrawnAsText(t *testing.T) {
	// The regression of f4#1625 at the level the reader sees: the cells
	// HelpView draws, not only the markup.
	//
	// NewHelpView sizes itself to the global FrameManager's screen when there
	// is one, and wraps the topic to that width. A test that left a narrow
	// FrameManager behind used to cut this line short ("    # comment"), but
	// only when it ran first; this one wants the default, unwrapped view.
	savedFM := FrameManager
	FrameManager = nil
	t.Cleanup(func() { FrameManager = savedFM })
	view := NewMarkdownView("Doc", "```\n# comment ~x~y@ z\n```")
	line := view.current.Lines[0]
	scr := NewScreenBuf()
	scr.AllocBuf(40, 1)
	view.renderLine(scr, 0, 0, line, 40, 0)
	var b strings.Builder
	for x := 0; x < 40; x++ {
		c := scr.GetCell(x, 0)
		if c.Char == 0 {
			break
		}
		if c.Char > utf8.MaxRune {
			t.Fatalf("cell %d holds %#x, not a rune", x, c.Char)
			return
		}
		b.WriteRune(rune(c.Char))
	}
	if got, want := strings.TrimRight(b.String(), " "), "    # comment ~x~y@ z"; got != want {
		t.Fatalf("drawn %q, want %q", got, want)
	}
	if len(view.current.Links) != 0 {
		t.Errorf("Links = %#v, want none", view.current.Links)
	}
}

func TestParseMarkdownTopic_BulletList(t *testing.T) {
	mdCheckLines(t, "- one\n- two\n  - nested\n* three", []string{
		"• one", "• two", "  • nested", "• three",
	})
}

func TestParseMarkdownTopic_OrderedList(t *testing.T) {
	mdCheckLines(t, "3. third\n4. fourth\n\n1) a\n2) b", []string{
		"3. third", "4. fourth", "", "1) a", "2) b",
	})
}

func TestParseMarkdownTopic_LooseListKeepsBlankLines(t *testing.T) {
	mdCheckLines(t, "- a\n\n- b\n\n  second paragraph", []string{
		"• a", "", "• b", "", "  second paragraph",
	})
}

func TestParseMarkdownTopic_TaskList(t *testing.T) {
	mdCheckLines(t, "- [x] done\n- [ ] todo", []string{"• [x] done", "• [ ] todo"})
}

func TestParseMarkdownTopic_Link(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "See [the docs](https://example.com/docs) for more.")
	want := "See ~the docs~https://example.com/docs@ for more."
	if topic.Lines[0] != want {
		t.Fatalf("Lines[0] = %q, want %q", topic.Lines[0], want)
	}
	if len(topic.Links) != 1 {
		t.Fatalf("Links = %#v, want exactly one", topic.Links)
	}
	link := topic.Links[0]
	if link.Text != "the docs" || link.Target != "https://example.com/docs" {
		t.Errorf("link = %+v, want Text=%q Target=%q", link, "the docs", "https://example.com/docs")
	}
	if link.Line != 0 {
		t.Errorf("link.Line = %d, want 0", link.Line)
	}
}

func TestParseMarkdownTopic_LinkTextWithMarkupChars(t *testing.T) {
	topic := mdCheckLines(t, "[C# ~docs~ **here**](https://example.com/a@b)", []string{"C# ~docs~ here"})
	if len(topic.Links) != 1 {
		t.Fatalf("Links = %#v, want exactly one", topic.Links)
	}
	if l := topic.Links[0]; l.Text != "C# ~docs~ here" || l.Target != "https://example.com/a%40b" {
		t.Errorf("link = %+v", l)
	}
}

func TestParseMarkdownTopic_ReferenceLinkImageAndAutolinks(t *testing.T) {
	md := "A [ref][r], ![logo](img.png), <https://a.example>, https://b.example and me@example.com now\n\n[r]: https://ref.example"
	topic := mdCheckLines(t, md, []string{"A ref, logo, https://a.example, https://b.example and me@example.com now"})
	var targets []string
	for _, l := range topic.Links {
		targets = append(targets, l.Target)
	}
	want := []string{"https://ref.example", "img.png", "https://a.example", "https://b.example", "mailto:me%40example.com"}
	if !reflect.DeepEqual(targets, want) {
		t.Errorf("targets = %#v, want %#v", targets, want)
	}
}

func TestParseMarkdownTopic_LinkInHeadingEndsBold(t *testing.T) {
	topic := mdCheckLines(t, "## See [docs](https://d.example) here", []string{"See docs here"})
	if got := mdBold(topic.Lines[0]); got != "See | here" {
		t.Errorf("highlighted = %q", got)
	}
	if len(topic.Links) != 1 || topic.Links[0].Target != "https://d.example" {
		t.Errorf("Links = %#v", topic.Links)
	}
}

func TestParseMarkdownTopic_MultipleLinesTrackLineIndex(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "# Title\n\n[one](https://a.example)\n\n[two](https://b.example)")
	var targets []string
	var lines []int
	for _, l := range topic.Links {
		targets = append(targets, l.Target)
		lines = append(lines, l.Line)
	}
	if len(targets) != 2 || targets[0] != "https://a.example" || targets[1] != "https://b.example" {
		t.Fatalf("targets = %#v", targets)
	}
	if lines[0] == lines[1] {
		t.Errorf("both links reported the same line index %d", lines[0])
	}
}

func TestParseMarkdownTopic_SoftAndHardLineBreaks(t *testing.T) {
	mdCheckLines(t, "one\ntwo\\\nthree  \nfour", []string{"one two", "three", "four"})
	topic := mdCheckLines(t, "**a\\\nb**", []string{"a", "b"})
	for i, line := range topic.Lines {
		if mdBold(line) != mdVisible(line) {
			t.Errorf("line %d %q is not bold all through", i, line)
		}
	}
}

func TestParseMarkdownTopic_Blockquote(t *testing.T) {
	mdCheckLines(t, "> quoted **text**\n> more\n>\n> - item", []string{
		"│ quoted text more", "│", "│ • item",
	})
}

func TestParseMarkdownTopic_ThematicBreak(t *testing.T) {
	rule := strings.Repeat("─", markdownRuleWidth)
	mdCheckLines(t, "a\n\n---\n\nb\n\n***", []string{"a", "", rule, "", "b", "", rule})
}

func TestParseMarkdownTopic_Table(t *testing.T) {
	topic := mdCheckLines(t, "| a | b |\n|---|--:|\n| x | 10 |", []string{
		"a │  b",
		"──┼───",
		"x │ 10",
	})
	if got := mdBold(topic.Lines[0]); got != "a|b" {
		t.Errorf("header highlighted = %q, want %q", got, "a|b")
	}
}

func TestParseMarkdownTopic_LiteralMarkupCharsInText(t *testing.T) {
	// '#', '~', '@' and a leading '^' mean something to the help markup; in
	// Markdown text they are just characters (f4#1625).
	md := "C# and #hashtag, ~/src and ~a~b@ c, user @alice\n\n^ not centered"
	topic := mdCheckLines(t, md, []string{
		"C# and #hashtag, ~/src and ~a~b@ c, user @alice",
		"",
		"^ not centered",
	})
	if len(topic.Links) != 0 {
		t.Errorf("Links = %#v, want none", topic.Links)
	}
	if got := mdBold(topic.Lines[0]); got != "" {
		t.Errorf("highlighted = %q, want nothing", got)
	}
	if strings.HasPrefix(topic.Lines[2], "^") {
		t.Errorf("line %q would be drawn centered", topic.Lines[2])
	}
}

func TestParseMarkdownTopic_LiteralMarkupCharsInHeading(t *testing.T) {
	topic := mdCheckLines(t, "# C# and ~tilde~ in a heading", []string{"C# and ~tilde~ in a heading"})
	if got := mdBold(topic.Lines[0]); got != "C# and ~tilde~ in a heading" {
		t.Errorf("highlighted = %q, want the whole heading", got)
	}
}

func TestParseMarkdownTopic_EscapesAndEntities(t *testing.T) {
	topic := mdCheckLines(t, "\\*not em\\* \\# &amp; &#35; &copy;", []string{"*not em* # & # ©"})
	if got := mdBold(topic.Lines[0]); got != "" {
		t.Errorf("highlighted = %q, want nothing", got)
	}
}

func TestParseMarkdownTopic_HTML(t *testing.T) {
	mdCheckLines(t, "<!-- hidden -->\n\n<div>\n# raw\n</div>\n\ntext <b>x</b>", []string{
		"<div>", "# raw", "</div>", "", "text <b>x</b>",
	})
}

func TestParseMarkdownTopic_Empty(t *testing.T) {
	if got := ParseMarkdownTopic("Doc", "").Lines; !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("Lines = %#v, want one empty line", got)
	}
}

func TestParseMarkdownTopic_EscapedCharsSurviveWrapping(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "`#aaaa ~bbbb ^cccc`")
	wrapped, _ := wrapHelpTopic(topic, 6)
	want := []string{"#aaaa", "~bbbb", "^cccc"}
	if got := mdVisibleLines(wrapped); !reflect.DeepEqual(got, want) {
		t.Fatalf("wrapped = %#v, want %#v (markup %#v)", got, want, wrapped.Lines)
	}
	for i, line := range wrapped.Lines {
		if mdBold(line) != want[i] {
			t.Errorf("row %d %q: highlighted %q, want all of it", i, line, mdBold(line))
		}
	}
	if len(wrapped.Links) != 0 {
		t.Errorf("Links = %#v, want none", wrapped.Links)
	}
}

func TestNewMarkdownView_ShowsParsedContent(t *testing.T) {
	view := NewMarkdownView("Readme", "# Hello\n\nSee [more](https://example.com).")
	if view == nil {
		t.Fatal("NewMarkdownView returned nil")
	}
	if view.current == nil {
		t.Fatal("view has no current topic")
	}
	if view.current.Name != "Readme" {
		t.Errorf("current topic Name = %q, want %q", view.current.Name, "Readme")
	}
	if len(view.current.Links) != 1 || view.current.Links[0].Target != "https://example.com" {
		t.Errorf("current topic Links = %#v", view.current.Links)
	}
}

func TestParseMarkdownTopicMap(t *testing.T) {
	md := "# Title\n\nFirst paragraph.\n\n```\ncode a\ncode b\n```\n\n- one\n- two\n\nLast."
	topic, src := ParseMarkdownTopicMap("Doc", md)
	if len(src) != len(topic.Lines) {
		t.Fatalf("map has %d entries for %d lines", len(src), len(topic.Lines))
	}
	plain := ParseMarkdownTopic("Doc", md)
	if len(plain.Lines) != len(topic.Lines) {
		t.Fatal("ParseMarkdownTopic and ParseMarkdownTopicMap disagree")
	}
	for i := 1; i < len(src); i++ {
		if src[i] < src[i-1] {
			t.Fatalf("map goes back at %d: %v", i, src)
		}
	}
	find := func(sub string) int {
		for i, l := range topic.Lines {
			if strings.Contains(l, sub) {
				return i
			}
		}
		t.Fatalf("no topic line with %q in %q", sub, topic.Lines)
		return -1
	}
	want := map[string]int{"Title": 0, "First": 2, "code a": 5, "code b": 6, "one": 9, "two": 10, "Last.": 12}
	for sub, line := range want {
		if got := src[find(sub)]; got != line {
			t.Errorf("%q comes from line %d, want %d (map %v)", sub, got, line, src)
		}
	}
	if _, src := ParseMarkdownTopicMap("Empty", ""); len(src) != 1 || src[0] != 0 {
		t.Errorf("empty document map = %v", src)
	}
}

// A Markdown view built while a FrameManager with a screen exists sizes itself
// to that screen, centred, the way every HelpView does.
func TestNewMarkdownView_SizesItselfToTheFrameManagerScreen(t *testing.T) {
	savedFM := FrameManager
	t.Cleanup(func() { FrameManager = savedFM })
	scr := NewScreenBuf()
	scr.AllocBuf(100, 30)
	fm := NewFrameManager()
	fm.Init(scr)
	FrameManager = fm

	view := NewMarkdownView("Doc", "text")
	if view.X1 != 12 || view.Y1 != 2 || view.X2 != 87 || view.Y2 != 27 {
		t.Errorf("view at (%d,%d)-(%d,%d), want (12,2)-(87,27)", view.X1, view.Y1, view.X2, view.Y2)
	}
}
