package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestExtraCaretSelectionRange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		caret     extraCaret
		wantStart int
		wantEnd   int
	}{
		{name: "empty", caret: extraCaret{off: 4}, wantStart: 4, wantEnd: 4},
		{name: "forward", caret: extraCaret{off: 9, anchor: 3, hasSel: true}, wantStart: 3, wantEnd: 9},
		{name: "backward", caret: extraCaret{off: 3, anchor: 9, hasSel: true}, wantStart: 3, wantEnd: 9},
		{name: "same", caret: extraCaret{off: 5, anchor: 5, hasSel: true}, wantStart: 5, wantEnd: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := tc.caret.selRange()
			if start != tc.wantStart || end != tc.wantEnd {
				t.Fatalf("selRange = (%d, %d), want (%d, %d)", start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestHighlightDutyCoverageBatch21(t *testing.T) {
	if highlightDuty(false, true) != hlDutyIndexing {
		t.Fatal("indexing did not take priority")
	}
	if highlightDuty(true, false) != hlDutyVisible {
		t.Fatal("behind-viewport duty was not selected")
	}
	if highlightDuty(false, false) != hlDutyAhead {
		t.Fatal("ahead duty was not selected")
	}
}

func TestHighlightIdleGapClamps(t *testing.T) {
	if highlightIdleGap(time.Millisecond, 100) != 0 {
		t.Fatal("100% duty should not insert idle time")
	}
	if got := highlightIdleGap(time.Millisecond, 0); got != hlIdleMax {
		t.Fatalf("zero duty gap = %v, want %v", got, hlIdleMax)
	}
	if got := highlightIdleGap(time.Nanosecond, 99); got != hlIdleMin {
		t.Fatalf("small gap = %v, want minimum %v", got, hlIdleMin)
	}
	if got := highlightIdleGap(time.Hour, 1); got != hlIdleMax {
		t.Fatalf("large gap = %v, want maximum %v", got, hlIdleMax)
	}
}

func TestUsesStateChainNil(t *testing.T) {
	if usesStateChain(nil) {
		t.Fatal("nil highlighter cannot carry a state chain")
	}
}

func TestHexCharToByte(t *testing.T) {
	for _, tc := range []struct {
		input rune
		want  byte
	}{
		{input: '0', want: 0},
		{input: '9', want: 9},
		{input: 'a', want: 10},
		{input: 'f', want: 15},
		{input: 'A', want: 10},
		{input: 'F', want: 15},
		{input: 'x', want: 0},
	} {
		if got := hexCharToByte(tc.input); got != tc.want {
			t.Errorf("hexCharToByte(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestEditorVisualClusters(t *testing.T) {
	clusters := editorVisualClusters("a\u0301b")
	if len(clusters) != 2 {
		t.Fatalf("cluster count = %d, want 2: %#v", len(clusters), clusters)
	}
	if clusters[0].text != "a\u0301" || clusters[1].text != "b" {
		t.Fatalf("clusters = %#v", clusters)
	}
	if clusters[0].byteStart != 0 || clusters[0].byteEnd <= clusters[0].byteStart {
		t.Fatalf("first cluster byte range = (%d, %d)", clusters[0].byteStart, clusters[0].byteEnd)
	}
}

func TestEditorRenderClipCoverageBatch21(t *testing.T) {
	short := "short"
	if editorRenderClip(short, 20) != short || editorRenderClip(short, 0) != short {
		t.Fatal("short text was changed by clipping")
	}
	long := strings.Repeat("x", 200)
	clipped := editorRenderClip(long, 10)
	if len(clipped) < 10 || len(clipped) >= len(long) || !strings.HasPrefix(long, clipped) {
		t.Fatalf("clip length = %d, want a prefix shorter than %d", len(clipped), len(long))
	}
	oldBidi := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	if got := editorRenderClip("abc אבג", 2); got != "abc אבג" {
		t.Fatalf("RTL text was clipped: %q", got)
	}
	vtui.DefaultBidiMode = oldBidi
}

func TestEditorRenderColumns(t *testing.T) {
	if editorRenderColumns("") != 0 {
		t.Fatal("empty text has columns")
	}
	if editorRenderColumns("abc") != 3 {
		t.Fatalf("ASCII columns = %d, want 3", editorRenderColumns("abc"))
	}
	if editorRenderColumns("a\tb") != 3 {
		t.Fatalf("tab columns = %d, want 3", editorRenderColumns("a\tb"))
	}
}

func TestNextIndexPollClamps(t *testing.T) {
	if nextIndexPoll(indexPollMin/2) != indexPollMin {
		t.Fatal("nextIndexPoll did not clamp to minimum")
	}
	if nextIndexPoll(indexPollMax) != indexPollMax {
		t.Fatal("nextIndexPoll did not clamp to maximum")
	}
	if got := nextIndexPoll(indexPollMin); got <= indexPollMin || got > indexPollMax {
		t.Fatalf("normal poll interval = %v", got)
	}
}

func TestReplaceAllFold(t *testing.T) {
	if replaceAllFold("Hello HELLO", "hello", "x") != "x x" {
		t.Fatal("replaceAllFold did not replace case-insensitively")
	}
	if replaceAllFold("abc", "", "x") != "abc" {
		t.Fatal("replaceAllFold changed an empty pattern")
	}
	if replaceAllFold("abc", "z", "x") != "abc" {
		t.Fatal("replaceAllFold changed a string without matches")
	}
}
