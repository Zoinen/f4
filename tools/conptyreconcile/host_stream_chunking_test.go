package main

import "testing"

func TestVerifyHostStreamChunkingIncludesEscapeAndUTF8Boundaries(t *testing.T) {
	data := []byte("prefix-漢\x1b[8;40;121tline\r\nnext-👩‍💻\r\n")
	checks, err := verifyHostStreamChunking(data, 115)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 6 {
		t.Fatalf("got %d chunking checks, want 6", len(checks))
	}
	for _, check := range checks {
		if check.Status != "passed" {
			t.Fatalf("%s status=%s detail=%s", check.Mode, check.Status, check.Detail)
		}
	}
}
