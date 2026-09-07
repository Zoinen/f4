package mediainfo

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRenderTextPreservesSmallOutput(t *testing.T) {
	report := Report{
		General: General{FileName: "sample.flac", Format: "FLAC"},
		Tags:    []Field{{Name: "Artist", Value: "Example"}},
	}
	want := "General\n" +
		"File name : sample.flac\n" +
		"Format    : FLAC\n\n" +
		"Metadata\n" +
		"Artist    : Example"
	if got := RenderText(report, RenderOptions{}); got != want {
		t.Fatalf("RenderText() changed small output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderTextAlignsLabelColumnAcrossSections(t *testing.T) {
	report := Report{
		General: General{Format: "Matroska"},
		Streams: []Stream{{Kind: StreamAudio, Format: "Opus", Audio: &Audio{CompressionMode: "Lossy"}}},
		Tags:    []Field{{Name: "Artist", Value: "Example"}},
	}

	got := RenderText(report, RenderOptions{})
	for _, want := range []string{
		"Format           : Matroska",
		"Format           : Opus",
		"Compression mode : Lossy",
		"Artist           : Example",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered report does not use one label column; missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTextAlignsWideLabelsByTerminalCellWidth(t *testing.T) {
	report := Report{
		General: General{Format: "Matroska"},
		Tags:    []Field{{Name: "映像情報", Value: "Example"}},
	}

	got := RenderText(report, RenderOptions{})
	if !strings.Contains(got, "Format   : Matroska") ||
		!strings.Contains(got, "映像情報 : Example") {
		t.Fatalf("rendered report does not align a wide label by terminal cells:\n%s", got)
	}
}

func TestRenderTextGlobalWidthRespectsCompactAndTechnicalFields(t *testing.T) {
	report := Report{
		General: General{Format: "Matroska", CodecID: "matroska"},
		Streams: []Stream{{Kind: StreamAudio, Format: "Opus", Audio: &Audio{CompressionMode: "Lossy"}}},
	}

	compact := RenderText(report, RenderOptions{Compact: true})
	if !strings.Contains(compact, "Format : Matroska") ||
		strings.Contains(compact, "Codec ID") || strings.Contains(compact, "Compression mode") {
		t.Fatalf("compact report used fields excluded by its selection:\n%s", compact)
	}

	technical := RenderText(report, RenderOptions{Compact: true, Technical: true})
	if !strings.Contains(technical, "Format   : Matroska") ||
		!strings.Contains(technical, "Codec ID : matroska") || strings.Contains(technical, "Compression mode") {
		t.Fatalf("technical compact report used the wrong global label width:\n%s", technical)
	}
}

func TestRenderTextCapsOversizedFieldBeforeWritingIt(t *testing.T) {
	report := Report{
		General: General{Format: "FLAC"},
		Tags: []Field{{
			Name:  "Comment",
			Value: strings.Repeat("x", maxRenderedTextBytes*2),
		}},
	}
	got := RenderText(report, RenderOptions{})
	if len(got) > maxRenderedTextBytes {
		t.Fatalf("rendered %d bytes, limit is %d", len(got), maxRenderedTextBytes)
	}
	if !strings.HasSuffix(got, "\n\n"+renderTextTruncationNotice) {
		t.Fatalf("missing deterministic truncation notice: %q", got[len(got)-min(len(got), 128):])
	}
	if strings.Contains(got, "Comment") {
		t.Fatal("oversized section was partially written")
	}
}

func TestRenderTextLocalizesTruncationNotice(t *testing.T) {
	report := Report{General: General{FileName: strings.Repeat("x", maxRenderedTextBytes*2)}}
	got := RenderText(report, RenderOptions{Language: "ru"})
	if !strings.HasSuffix(got, renderTextTruncationNoticeRU) {
		t.Fatalf("Russian truncation notice is not localized: %q", got[len(got)-min(len(got), 128):])
	}
	if strings.HasSuffix(got, renderTextTruncationNotice) {
		t.Fatal("Russian report ended with the English truncation notice")
	}
}

func TestRenderTextNormalizesUntrustedInlineControls(t *testing.T) {
	report := Report{
		General: General{Format: "FLAC\nforged", FileName: "sample\tname.flac"},
		Tags: []Field{{
			Name:  "Com\x1bment\rname",
			Value: "first\r\nsecond\tthird\x00fourth\u2028fifth",
		}},
	}
	got := RenderText(report, RenderOptions{})
	if strings.ContainsAny(got, "\r\t\x00\x1b") || strings.ContainsRune(got, '\u2028') {
		t.Fatalf("rendered report retained an inline control: %q", got)
	}
	if !strings.Contains(got, "Format        : FLAC forged") ||
		!strings.Contains(got, "File name     : sample name.flac") ||
		!strings.Contains(got, "Com ment name : first second third fourth fifth") {
		t.Fatalf("rendered report did not normalize fields onto one line:\n%s", got)
	}
	if gotLines := strings.Count(got, "\n") + 1; gotLines != 6 {
		t.Fatalf("rendered report has %d lines after injected newlines, want 6:\n%s", gotLines, got)
	}
}

func TestSplitRenderedTextLinesCapsRowAmplification(t *testing.T) {
	text := strings.Repeat("line\n", maxRenderedTextLines+100)
	lines, truncated := splitRenderedTextLines(text, renderTextTruncationNotice)
	if !truncated {
		t.Fatal("oversized line collection was not marked truncated")
	}
	if len(lines) != maxRenderedTextLines {
		t.Fatalf("split retained %d lines, want %d", len(lines), maxRenderedTextLines)
	}
	if lines[len(lines)-1] != renderTextTruncationNotice {
		t.Fatalf("last line = %q, want truncation notice", lines[len(lines)-1])
	}

	exact := strings.TrimSuffix(strings.Repeat("line\n", maxRenderedTextLines), "\n")
	lines, truncated = splitRenderedTextLines(exact, renderTextTruncationNotice)
	if truncated || len(lines) != maxRenderedTextLines || lines[len(lines)-1] != "line" {
		t.Fatalf("exact-limit split = len %d truncated %v last %q", len(lines), truncated, lines[len(lines)-1])
	}
}

func TestCanonicalSectionsDoNotMultiplyTargetedTagsAcrossDuplicateStreamIDs(t *testing.T) {
	const streamCount = 256
	tagCount := DefaultOptions(ModeDetailed).MaxTags
	report := Report{Streams: make([]Stream, streamCount), Tags: make([]Field, tagCount)}
	for index := range report.Streams {
		report.Streams[index] = Stream{Index: index, ID: "duplicate", Kind: StreamVideo, Format: "AVC"}
	}
	for index := range report.Tags {
		report.Tags[index] = Field{Target: "duplicate", Name: "TargetTag", Value: fmt.Sprintf("value-%04d", index)}
	}

	sections := CanonicalSections(report)
	attached := 0
	sectionsWithTags := 0
	for _, section := range sections[1:] {
		sectionTags := 0
		for _, field := range section.Fields {
			if field.Name == "TargetTag" {
				sectionTags++
			}
		}
		if sectionTags > 0 {
			sectionsWithTags++
		}
		attached += sectionTags
	}
	if attached != tagCount || sectionsWithTags != 1 {
		t.Fatalf("targeted tags attached %d times across %d sections, want %d tags on one section", attached, sectionsWithTags, tagCount)
	}

	pairs := macroTechnicalPairs(report)
	pairTags := 0
	for _, pair := range pairs {
		if pair.key == "TargetTag" {
			pairTags++
		}
	}
	if pairTags != tagCount {
		t.Fatalf("macro pairs contain %d targeted tags, want %d", pairTags, tagCount)
	}

	got, err := ExecuteTemplate(report, "Video;[%TargetTag%|]")
	if err != nil {
		t.Fatal(err)
	}
	if got != "value-0000|" {
		t.Fatalf("Inform output = %q, want first duplicate target only", got)
	}
}

func TestRenderTextCollectionLimitIsDeterministic(t *testing.T) {
	tags := make([]Field, maxRenderedCollectionItems+1)
	for index := range tags {
		tags[index] = Field{Name: "Tag", Value: "value"}
	}
	report := Report{General: General{Format: "Matroska"}, Tags: tags}
	first := RenderText(report, RenderOptions{})
	second := RenderText(report, RenderOptions{})
	if first != second {
		t.Fatal("collection truncation is not deterministic")
	}
	if len(first) > maxRenderedTextBytes {
		t.Fatalf("rendered %d bytes, limit is %d", len(first), maxRenderedTextBytes)
	}
	if !strings.HasSuffix(first, "\n\n"+renderTextTruncationNotice) {
		t.Fatal("collection limit did not append truncation notice")
	}
}

func TestReportTextWriterNeverGrowsPastContentCap(t *testing.T) {
	writer := newReportTextWriter(128, renderTextTruncationNotice)
	renderTextSection(writer, "Metadata", [][2]string{{"Comment", strings.Repeat("x", 256)}}, "")
	if !writer.truncated {
		t.Fatal("oversized section did not mark the writer truncated")
	}
	if writer.out.b.Len() > writer.contentCap {
		t.Fatalf("content buffer grew to %d bytes, cap is %d", writer.out.b.Len(), writer.contentCap)
	}
	first := writer.String()
	if second := writer.String(); second != first {
		t.Fatal("finalizing a truncated writer is not idempotent")
	}
	if first != renderTextTruncationNotice {
		t.Fatalf("truncated empty report = %q", first)
	}
}

func TestCappedTemplateWriterLatchesOverflow(t *testing.T) {
	writer := newCappedTemplateWriter(4)
	if err := writer.WriteString("1234"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteByte('5'); !errors.Is(err, errTemplateOutputTooLarge) {
		t.Fatalf("overflow error = %v", err)
	}
	writer.limit = 8 // Raising the ceiling must not resume a failed stream.
	if err := writer.WriteString("6"); !errors.Is(err, errTemplateOutputTooLarge) {
		t.Fatalf("write after overflow = %v", err)
	}
	if got := writer.String(); got != "1234" {
		t.Fatalf("writer resumed after overflow: %q", got)
	}
}
