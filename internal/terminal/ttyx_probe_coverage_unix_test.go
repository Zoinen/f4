//go:build !windows

package terminal

import (
	"os"
	"testing"
	"time"
)

func TestHostPixelsFromIoctlRejectsMissingAndNonTerminalFiles(t *testing.T) {
	if width, height, ok := HostPixelsFromIoctl(nil); ok || width != 0 || height != 0 {
		t.Fatalf("nil file result = (%d, %d, %v)", width, height, ok)
	}

	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close /dev/null: %v", err)
		}
	})
	if width, height, ok := HostPixelsFromIoctl(file); ok || width != 0 || height != 0 {
		t.Fatalf("non-terminal file result = (%d, %d, %v)", width, height, ok)
	}
}

func TestReadAnswerCompletesWithDeadline(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close pipe reader: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close pipe writer: %v", err)
		}
	})

	if _, err := writer.WriteString("\x1b[4;856;1319t"); err != nil {
		t.Fatal(err)
	}
	answer, ok := readAnswer(reader, time.Second, "\x1b[4;")
	if !ok || answer != "\x1b[4;856;1319t" {
		t.Fatalf("readAnswer() = (%q, %v)", answer, ok)
	}
}

func TestPollAnswerRejectsEndOfFile(t *testing.T) {
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close /dev/null: %v", err)
		}
	})

	answer, ok := pollAnswer(file, 10*time.Millisecond, "\x1b[4;")
	if ok || answer != "" {
		t.Fatalf("pollAnswer() = (%q, %v), want empty incomplete answer", answer, ok)
	}
}
