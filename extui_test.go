package main

import (
	"bytes"
	"net"
	"testing"
)

func TestExtUiProtocolRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := map[string]any{
		"type":   "frame",
		"width":  2,
		"height": 1,
		"full":   true,
		"cells":  [][3]uint64{{0, 'A', 0x010203}, {1, 'B', 0x040506}},
	}
	if err := extUiSendMessage(&buf, want); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	got, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if extUiString(got, "type") != "frame" {
		t.Fatalf("type mismatch: %q", extUiString(got, "type"))
	}
	if extUiInt(got, "width") != 2 || extUiInt(got, "height") != 1 {
		t.Fatalf("size mismatch: %dx%d", extUiInt(got, "width"), extUiInt(got, "height"))
	}
	if !extUiBool(got, "full") {
		t.Fatal("full flag was not preserved")
	}
}

func TestExtUi_HandshakeScaling(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	nonce := "testnonce"

	go func() {
		msg := map[string]any{
			"type":        "hello",
			"nonce":       nonce,
			"pixelWidth":  1000,
			"pixelHeight": 600,
			"cellWidth":   10,
			"cellHeight":  20,
		}
		extUiSendMessage(c, msg)
	}()

	hello, err := extUiReadMessage(s)
	if err != nil {
		t.Fatalf("failed to read hello: %v", err)
	}

	cols := 100
	rows := 30

	pixelW := extUiInt(hello, "pixelWidth")
	pixelH := extUiInt(hello, "pixelHeight")
	cellW := extUiInt(hello, "cellWidth")
	cellH := extUiInt(hello, "cellHeight")

	if pixelW > 0 && cellW > 0 {
		cols = pixelW / cellW
	}
	if pixelH > 0 && cellH > 0 {
		rows = pixelH / cellH
	}

	if cols != 100 {
		t.Errorf("expected cols 100 (1000/10), got %d", cols)
	}
	if rows != 30 {
		t.Errorf("expected rows 30 (600/20), got %d", rows)
	}
}
