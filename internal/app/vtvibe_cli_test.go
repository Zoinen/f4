package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/unxed/f4/internal/vtvibe"
)

// f4#1842, H9 item 12: f4 --ai answers without the UI.

func aiCLIServer(t *testing.T, answer string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(data))
		mu.Unlock()
		reply, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": answer}}}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

func TestAICLIPrintsTheAnswer(t *testing.T) {
	srv, bodies := aiCLIServer(t, "it adds a test")
	file := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(file, []byte("NOTES-CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	config := func() (vtvibe.Config, string, vtvibe.Provider) {
		return vtvibe.Config{BaseURL: srv.URL, Model: "from-settings", APIKey: "k"}, "TEST_KEY", vtvibe.Provider{}
	}
	var stdout, stderr bytes.Buffer
	code, handled := runAICLI([]string{"--ai", "review this", "--ai-file=" + file, "--ai-model", "other-model"},
		strings.NewReader("DIFF-CONTENT"), &stdout, &stderr, config)
	if !handled || code != 0 || stdout.String() != "it adds a test\n" || stderr.Len() != 0 {
		t.Fatalf("code %d, handled %v, stdout %q, stderr %q", code, handled, stdout.String(), stderr.String())
	}
	got := bodies()
	if len(got) != 1 {
		t.Fatalf("requests %d", len(got))
	}
	for _, want := range []string{"review this", "DIFF-CONTENT", "stdin.txt", "NOTES-CONTENT", "notes.md", `"model":"other-model"`} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the request lacks %q: %s", want, got[0])
		}
	}
}

func TestAICLIIsNotTakenWithoutAI(t *testing.T) {
	if _, handled := runAICLI([]string{"/tmp", "--tty"}, nil, io.Discard, io.Discard, nil); handled {
		t.Fatal("a normal start was taken for f4 --ai")
	}
}

func TestAICLIUsageAndNoKey(t *testing.T) {
	for _, args := range [][]string{{"--ai"}, {"--ai="}, {"--ai-model", "m"}} {
		var stderr bytes.Buffer
		if code, handled := runAICLI(args, nil, io.Discard, &stderr, nil); !handled || code != 2 || !strings.Contains(stderr.String(), "usage") {
			t.Errorf("%q: code %d, handled %v, stderr %q", args, code, handled, stderr.String())
		}
	}
	noKey := func() (vtvibe.Config, string, vtvibe.Provider) {
		return vtvibe.Config{BaseURL: "https://example.invalid/v1", Model: "m"}, "", vtvibe.Provider{}
	}
	var stderr bytes.Buffer
	if code, _ := runAICLI([]string{"--ai", "hi"}, nil, io.Discard, &stderr, noKey); code != 1 || !strings.Contains(stderr.String(), "no API key") {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
}
