package main

import (
	"bytes"
	"testing"
)

func TestConsumerScrollbackSpillsCompleteLinesAndSurvivesResize(t *testing.T) {
	var source logicalLineStream
	source.Feed([]byte("a\r\neviction-boundary: " + string(bytes.Repeat([]byte{'E'}, 257)) + "\r\n" + "b\r\nc\r\nd\r\ne\r\n"))
	lines := source.Lines()
	model := newConsumerScrollback(3)
	for _, line := range lines {
		model.Append(line)
	}
	if len(model.spilled.pieces) != 3 {
		t.Fatalf("spilled pieces=%d, want 3", len(model.spilled.pieces))
	}
	if !bytes.Contains(model.spilled.Bytes(), []byte("eviction-boundary: "+string(bytes.Repeat([]byte{'E'}, 257)))) {
		t.Fatal("long line was not preserved as one spilled piece")
	}
	want := model.historyBytes()
	for _, width := range []int{1, 7, 80, 121, 3} {
		for _, offset := range []int{0, 1, 4, 0} {
			_ = model.visible(offset, 3, width)
			if !bytes.Equal(model.historyBytes(), want) {
				t.Fatalf("scroll/resize mutated history at width=%d offset=%d", width, offset)
			}
		}
	}
}

func TestConsumerScrollbackVisibleRowsRoundTrip(t *testing.T) {
	model := newConsumerScrollback(2)
	for _, text := range []string{"one", "two", "three", "long-" + string(bytes.Repeat([]byte{'x'}, 17)), "five"} {
		model.Append(logicalLine{Bytes: []byte(text), Terminator: []byte{'\r', '\n'}})
	}
	for _, width := range []int{1, 4, 9, 32} {
		rows := reflowLogicalLines(model.historyLines(), width)
		var joined []byte
		for _, row := range rows {
			joined = append(joined, row...)
		}
		var expected []byte
		for _, line := range model.historyLines() {
			expected = append(expected, line.Bytes...)
		}
		if !bytes.Equal(joined, expected) {
			t.Fatalf("width=%d changed visible text", width)
		}
	}
}
