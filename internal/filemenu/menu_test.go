package filemenu

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFileMenuChild(t *testing.T) {
	mode := os.Getenv("F4_FILE_MENU_TEST_CHILD")
	if mode == "" {
		return
	}
	var request Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(2)
	}
	switch mode {
	case "malformed":
		fmt.Print("not-json")
	case "crash":
		os.Exit(3)
	case "wait":
		time.Sleep(time.Minute)
	case "unknown":
		_ = json.NewEncoder(os.Stdout).Encode(Result{Outcome: "unknown"})
	default:
		_ = json.NewEncoder(os.Stdout).Encode(Result{Outcome: Selected, Action: request.Paths[0]})
	}
	os.Exit(0)
}

func TestHelperTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "юникод & quotes ' file.txt")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	old := helperCommand
	t.Cleanup(func() { helperCommand = old })
	for _, tc := range []struct {
		mode string
		want Outcome
	}{{"echo", Selected}, {"malformed", Failed}, {"crash", Failed}, {"unknown", Failed}, {"wait", Cancelled}} {
		t.Run(tc.mode, func(t *testing.T) {
			helperCommand = func(ctx context.Context) (*exec.Cmd, error) {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileMenuChild$")
				cmd.Env = append(os.Environ(), "F4_FILE_MENU_TEST_CHILD="+tc.mode)
				return cmd, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			result := runOnce(ctx, Request{Paths: []string{path}})
			if result.Outcome != tc.want {
				t.Fatalf("result=%+v", result)
			}
			if tc.mode == "echo" && result.Action != path {
				t.Fatalf("path changed: %q", result.Action)
			}
			if active.Load() {
				t.Fatal("helper retained the active slot")
			}
		})
	}
}

func TestHelperRejectsInvalidTargetsAndOverlap(t *testing.T) {
	for _, r := range []Request{{}, {Paths: []string{"relative"}}, {Paths: []string{filepath.Join(t.TempDir(), "missing")}}} {
		if result := Run(context.Background(), r); result.Outcome != Unavailable {
			t.Fatalf("result=%+v", result)
		}
	}
	active.Store(true)
	t.Cleanup(func() { active.Store(false) })
	if result := Run(context.Background(), Request{}); result.Outcome != Cancelled {
		t.Fatalf("overlapping menu=%+v", result)
	}
}
