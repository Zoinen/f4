package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaySession_Verification(t *testing.T) {
	recordContent := `{"time":0.001,"dir":"down","msg":{"op":"hello","seq":1,"version":1}}
{"time":0.003,"dir":"up","msg":{"op":"welcome","replyTo":1,"version":1}}
{"time":0.005,"dir":"down","msg":{"op":"mount","frameId":"testDlg","tree":{"type":"Dialog","id":"testDlg","props":{"title":"Test"}}}}
{"time":0.010,"dir":"down","msg":{"op":"quit"}}
`
	r := strings.NewReader(recordContent)
	err := replaySession(r, true, 0.0)
	if err != nil {
		t.Fatalf("Replay session failed: %v", err)
	}
}

// TestReplaySession_Golden replays every recorded session in testdata/sessions
// and checks the kernel's replies against the recording. A protocol change that
// alters a reply (op name, code, message) fails here and needs the recording
// regenerated on purpose.
func TestReplaySession_Golden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "sessions", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no golden sessions in testdata/sessions")
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := replaySession(bytes.NewReader(data), true, 0.0); err != nil {
				t.Fatalf("replay %s: %v", p, err)
			}
		})
	}
}

// TestReplaySession_DetectsMismatch makes sure verification is not vacuous: a
// recording whose expected reply is wrong must fail.
func TestReplaySession_DetectsMismatch(t *testing.T) {
	rec := `{"time":0.001,"dir":"down","msg":{"op":"frobnicate","seq":2}}
{"time":0.002,"dir":"up","msg":{"op":"error","replyTo":2,"code":"HANDLER_ERROR","message":"something else"}}
{"time":0.003,"dir":"down","msg":{"op":"quit"}}
`
	if err := replaySession(strings.NewReader(rec), true, 0.0); err == nil {
		t.Fatal("replay accepted a wrong recorded reply")
	}
}
