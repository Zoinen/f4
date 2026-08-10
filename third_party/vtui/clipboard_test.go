package vtui

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"
)

func TestClipboard_Truncation(t *testing.T) {
	// 1. Setup
	// Create a string larger than our 2MB global limit
	const maxGlobalClipboardSize = 2 * 1024 * 1024
	largeString := strings.Repeat("A", maxGlobalClipboardSize+100)

	// Redirect stdout to capture OSC 52 sequence
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// 2. Execute
	// We must read from the pipe in a separate goroutine to prevent deadlocking
	// when the 2MB string exceeds the OS pipe buffer (typically 64KB).
	var outputBuf bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		io.Copy(&outputBuf, r)
		close(readDone)
	}()

	SetClipboard(largeString)

	// Restore stdout and signal end of data by closing the writer
	w.Close()
	<-readDone
	os.Stdout = oldStdout
	output := outputBuf.String()

	// 3. Assertions
	// Check internal buffer
	if len(internalClipboard) != maxGlobalClipboardSize {
		t.Errorf("Internal clipboard was not truncated. Expected len %d, got %d", maxGlobalClipboardSize, len(internalClipboard))
	}

	// Check OSC 52 payload if it was used as a fallback
	if strings.HasPrefix(output, "\x1b]52;c;") {
		parts := strings.Split(output, ";")
		if len(parts) == 3 {
			b64 := strings.TrimSuffix(parts[2], "\x07")
			decoded, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatalf("Failed to decode base64 from OSC 52: %v", err)
			}
			if len(decoded) > maxGlobalClipboardSize {
				t.Errorf("OSC 52 payload was not truncated. Expected <= %d, got %d", maxGlobalClipboardSize, len(decoded))
			}
		}
	}
}
