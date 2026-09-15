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

func TestPersistentChild(t *testing.T) {
	if os.Getenv("F4_PERSISTENT_TEST_CHILD") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var r Request
		if decoder.Decode(&r) != nil {
			os.Exit(0)
		}
		if path := os.Getenv("F4_PERSISTENT_TEST_LOG"); path != "" {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				os.Exit(4)
			}
			_ = json.NewEncoder(f).Encode(r)
			_ = f.Close()
		}
		switch r.Operation {
		case "crash":
			os.Exit(3)
		case "malformed":
			fmt.Println("invalid")
			os.Exit(0)
		case "unknown":
			_ = encoder.Encode(Result{Outcome: "unknown"})
		case "wait":
			time.Sleep(time.Minute)
		default:
			_ = encoder.Encode(Result{Outcome: Selected, Action: fmt.Sprintf("%d:%s", os.Getpid(), r.Paths[0])})
		}
	}
}

func testPersistentClient(t *testing.T) (*helperClient, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "юникод & file.txt")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &helperClient{command: func(ctx context.Context) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPersistentChild$")
		cmd.Env = append(os.Environ(), "F4_PERSISTENT_TEST_CHILD=1")
		return cmd, nil
	}}
	t.Cleanup(c.close)
	return c, path
}

func TestPersistentHelperReusesProcessAndReapsOnClose(t *testing.T) {
	c, path := testPersistentClient(t)
	r := Request{Paths: []string{path}}
	first := c.run(context.Background(), r)
	for i := 0; i < 3; i++ {
		result := c.run(context.Background(), r)
		if result.Outcome != Selected || result.Action != first.Action {
			t.Fatalf("helper was replaced or selection changed: %+v / %+v", first, result)
		}
	}
	p := c.process
	c.close()
	select {
	case <-p.done:
	default:
		t.Fatal("helper not reaped")
	}
}

func TestPersistentHelperFailureIsNotReplayed(t *testing.T) {
	for _, mode := range []string{"crash", "malformed", "unknown", "wait"} {
		t.Run(mode, func(t *testing.T) {
			c, path := testPersistentClient(t)
			good := Request{Paths: []string{path}}
			first := c.run(context.Background(), good)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			result := c.run(ctx, Request{Paths: []string{path}, Operation: mode})
			want := Failed
			if mode == "wait" {
				want = Cancelled
			}
			if result.Outcome != want {
				t.Fatalf("%+v", result)
			}
			next := c.run(context.Background(), good)
			if next.Outcome != Selected || next.Action == first.Action {
				t.Fatalf("next explicit request did not get a fresh helper: %+v", next)
			}
		})
	}
}

func TestPersistentHelperSkipsObsoletePreparation(t *testing.T) {
	c, path := testPersistentClient(t)
	result := c.runWhen(context.Background(), Request{Paths: []string{path}}, func() bool { return false })
	if result.Outcome != Cancelled || c.process != nil {
		t.Fatalf("obsolete preparation started helper: %+v", result)
	}
}
