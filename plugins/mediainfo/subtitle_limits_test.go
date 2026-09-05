package mediainfo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type subtitleParser func(string, *Text, *probe) error

func subtitleProbe(t *testing.T, ctx context.Context, maxElements, maxValueBytes int) *probe {
	t.Helper()
	opts := DefaultOptions(ModeFast)
	opts.MaxElements = maxElements
	opts.MaxValueBytes = maxValueBytes
	p, err := newProbe(ctx, Source{Name: "subtitle", Reader: memorySource{}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSubtitleParsersStopAtElementLimit(t *testing.T) {
	const records = 4096
	timing := "00:00:01,000 --> 00:00:02,000\n"
	tests := []struct {
		name  string
		parse subtitleParser
		text  string
	}{
		{name: "SRT", parse: parseSRT, text: strings.Repeat(timing, records)},
		{name: "WebVTT", parse: parseVTT, text: "WEBVTT\n" + strings.Repeat(timing, records)},
		{name: "ASS", parse: parseASS, text: strings.Repeat("Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,x\n", records)},
		{name: "MicroDVD", parse: parseMicroDVD, text: strings.Repeat("{25}{50}x\n", records)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const maxElements = 32
			p := subtitleProbe(t, context.Background(), maxElements, 1024)
			text := &Text{}
			err := tt.parse(tt.text, text, p)
			if !errors.Is(err, ErrLimit) {
				t.Fatalf("error = %v, want ErrLimit", err)
			}
			if p.elements != maxElements+1 {
				t.Fatalf("elements = %d, want %d", p.elements, maxElements+1)
			}
			if text.CueCount == 0 || text.CueCount > maxElements {
				t.Fatalf("cue count = %d", text.CueCount)
			}
		})
	}
}

func TestSubtitleParsersHonorCancellation(t *testing.T) {
	tests := []struct {
		name  string
		parse subtitleParser
		text  string
	}{
		{name: "SRT", parse: parseSRT, text: "00:00:01,000 --> 00:00:02,000\n"},
		{name: "WebVTT", parse: parseVTT, text: "WEBVTT\n00:01.000 --> 00:02.000\n"},
		{name: "ASS", parse: parseASS, text: "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,x\n"},
		{name: "MicroDVD", parse: parseMicroDVD, text: "{25}{50}x\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			p := subtitleProbe(t, ctx, 100, 1024)
			err := tt.parse(tt.text, &Text{}, p)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if p.elements != 0 {
				t.Fatalf("elements processed after cancellation = %d", p.elements)
			}
		})
	}
}

func TestSubtitleParsersPropagateOversizedLine(t *testing.T) {
	oversized := strings.Repeat("x", 256)
	tests := []struct {
		name  string
		parse subtitleParser
		text  string
	}{
		{name: "SRT", parse: parseSRT, text: oversized},
		{name: "WebVTT", parse: parseVTT, text: "WEBVTT\n" + oversized},
		{name: "ASS", parse: parseASS, text: oversized},
		{name: "MicroDVD", parse: parseMicroDVD, text: oversized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := subtitleProbe(t, context.Background(), 100, 64)
			err := tt.parse(tt.text, &Text{}, p)
			if err == nil || !strings.Contains(err.Error(), "token too long") {
				t.Fatalf("error = %v, want Scanner token-too-long error", err)
			}
		})
	}
}

func TestAnalyzeASSPropagatesScannerError(t *testing.T) {
	data := []byte("[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,," + strings.Repeat("x", 256))
	opts := DefaultOptions(ModeFast)
	opts.MaxValueBytes = 64
	_, err := Analyze(context.Background(), Source{Name: "oversized.ass", Size: int64(len(data)), Reader: memorySource(data)}, opts)
	if err == nil || !strings.Contains(err.Error(), "ASS parse error") || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("error = %v, want propagated ASS scanner error", err)
	}
}

func TestTTMLPreflightAllowsBoundedDocument(t *testing.T) {
	p := subtitleProbe(t, context.Background(), 100, 1024)
	text := &Text{}
	err := parseTTML(`<tt xml:lang="en"><body><div><p begin="1s" end="2s">cue</p></div></body></tt>`, text, p)
	if err != nil {
		t.Fatal(err)
	}
	if text.CueCount != 1 || text.FirstCue != time.Second || text.LastCue != 2*time.Second {
		t.Fatalf("unexpected TTML timing: %#v", text)
	}
	if len(p.report.Tags) != 1 || p.report.Tags[0].Name != "Language" || p.report.Tags[0].Value != "en" {
		t.Fatalf("unexpected TTML language tags: %#v", p.report.Tags)
	}
}

func TestTTMLPreflightRejectsAttributeAmplification(t *testing.T) {
	p := subtitleProbe(t, context.Background(), 10000, maxTTMLMarkupBytes)
	document := "<tt" + strings.Repeat(` a=""`, maxTTMLAttributes+1) + `><body><p begin="0s" end="1s"/></body></tt>`
	err := parseTTML(document, &Text{}, p)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("error = %v, want ErrLimit", err)
	}
	if !p.report.Truncated {
		t.Fatal("attribute limit must mark report truncated")
	}
}

func TestTTMLPreflightRejectsOversizedMarkupToken(t *testing.T) {
	p := subtitleProbe(t, context.Background(), 10000, maxTTMLMarkupBytes)
	document := "<tt " + strings.Repeat("x", maxTTMLMarkupBytes) + `><body><p begin="0s" end="1s"/></body></tt>`
	err := parseTTML(document, &Text{}, p)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("error = %v, want ErrLimit", err)
	}
	if !p.report.Truncated {
		t.Fatal("markup limit must mark report truncated")
	}
}

func TestTTMLPreflightChargesAttributesToElementBudget(t *testing.T) {
	p := subtitleProbe(t, context.Background(), 4, 1024)
	document := `<tt a="" b="" c="" d="" e=""><body><p begin="0s" end="1s"/></body></tt>`
	err := parseTTML(document, &Text{}, p)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("error = %v, want ErrLimit", err)
	}
	if p.elements != 5 {
		t.Fatalf("elements = %d, want 5", p.elements)
	}
}
