package intchecker

import (
	"strings"
	"testing"
	"time"
)

func TestProgressSpeedTextBeforeAnyData(t *testing.T) {
	p := newProgressSpeed()
	text := p.text(0, 0)
	if !strings.Contains(text, "Remaining: ??:??:??") {
		t.Fatalf("text = %q, want an unknown ETA", text)
	}
	if strings.Contains(text, "/s") {
		t.Fatalf("text = %q, want no speed before any data was read", text)
	}
}

func TestProgressSpeedTextElapsedAndETA(t *testing.T) {
	p := newProgressSpeed()
	p.start = time.Now().Add(-10 * time.Second) // 10s in, 25% done
	text := p.text(25, 100)
	if !strings.Contains(text, "Time: 00:00:1") { // 10s, tolerate scheduling jitter to 11s
		t.Fatalf("text = %q, want elapsed around 10s", text)
	}
	// 25% in 10s means the remaining 75% should take about 30s more.
	if !strings.Contains(text, "Remaining: 00:00:2") && !strings.Contains(text, "Remaining: 00:00:3") {
		t.Fatalf("text = %q, want an ETA around 30s", text)
	}
}

func TestProgressSpeedTextReportsSpeedAfterASecond(t *testing.T) {
	p := newProgressSpeed()
	p.start = time.Now().Add(-2 * time.Second)
	p.lastSpeedUpdate = time.Now().Add(-2 * time.Second)
	// 10 MiB read over the last (simulated) 2 seconds: ~5 MB/s, safely above
	// the KB/MB rounding boundary regardless of scheduling jitter.
	text := p.text(10*1024*1024, 100*1024*1024)
	if !strings.Contains(text, "MB/s") {
		t.Fatalf("text = %q, want a MB/s speed", text)
	}
}

func TestProgressSpeedTextNoSpeedWithinFirstSecond(t *testing.T) {
	p := newProgressSpeed()
	// lastSpeedUpdate defaults to "now": less than a second has passed, so
	// the smoothed speed is not recomputed yet and stays blank.
	text := p.text(1024, 1024*1024)
	if strings.Contains(text, "/s") {
		t.Fatalf("text = %q, want no speed within the first second", text)
	}
}
