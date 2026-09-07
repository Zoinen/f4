package main

import (
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
