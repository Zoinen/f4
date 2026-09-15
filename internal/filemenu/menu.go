// Package filemenu hosts desktop file menus without depending on application UI.
package filemenu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
)

const HelperFlag = "--file-menu-helper"

type Entry struct {
	ID       string
	Label    string
	Disabled bool
	Children []Entry
}

type Point struct {
	X, Y  int
	Valid bool
}
type Request struct {
	Paths       []string
	Entries     []Entry
	Position    Point
	Operation   string
	Application string
}

type Outcome string

const (
	Cancelled   Outcome = "cancelled"
	Invoked     Outcome = "invoked"
	Selected    Outcome = "selected"
	Unavailable Outcome = "unavailable"
	Failed      Outcome = "failed"
)

type Result struct {
	Outcome Outcome
	Action  string
	Error   string
	Entries []Entry
}

var active atomic.Bool

var helperCommand = func(ctx context.Context) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, exe, HelperFlag), nil
}

// Run uses a separate process so native event loops and extensions cannot own
// the application's UI thread. Unknown/partial replies are never retried.
func Run(ctx context.Context, request Request) Result {
	if !active.CompareAndSwap(false, true) {
		return Result{Outcome: Cancelled}
	}
	defer active.Store(false)
	return runPlatform(ctx, request)
}

func runOnce(ctx context.Context, request Request) Result {
	if err := validate(request); err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	cmd, err := helperCommand(ctx)
	if err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	configureProcess(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	if err = cmd.Start(); err != nil {
		_ = in.Close()
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	// Keep stdin open until the menu ends. EOF is the helper's parent-death
	// signal; this also covers an abrupt application exit.
	writeErr := json.NewEncoder(in).Encode(request)
	var result Result
	decodeErr := json.NewDecoder(io.LimitReader(out, 4<<20)).Decode(&result)
	_ = in.Close()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return Result{Outcome: Cancelled}
	}
	if err = errors.Join(writeErr, decodeErr, waitErr); err != nil {
		return Result{Outcome: Failed, Error: fmt.Sprintf("File menu helper: %v", err)}
	}
	switch result.Outcome {
	case Cancelled, Invoked, Selected, Unavailable, Failed:
		return result
	default:
		return Result{Outcome: Failed, Error: "Invalid file menu helper response"}
	}
}

func validate(r Request) error {
	if len(r.Paths) == 0 || len(r.Paths) > 4096 {
		return errors.New("Invalid file menu selection")
	}
	for _, p := range r.Paths {
		if !filepath.IsAbs(p) {
			return errors.New("File menu requires absolute paths")
		}
		if _, err := os.Lstat(p); err != nil {
			return err
		}
	}
	return nil
}

// Serve is entered before normal startup. It creates no panels, configuration
// or terminal sessions. Native implementations run on this goroutine.
func serveOnce(in io.Reader, out io.Writer) int {
	decoder := json.NewDecoder(in)
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return 2
	}
	if err := validate(request); err != nil {
		_ = json.NewEncoder(out).Encode(Result{Outcome: Unavailable, Error: err.Error()})
		return 0
	}
	go func() {
		_, _ = io.Copy(io.Discard, in)
		os.Exit(0)
	}()
	result := showNative(request)
	if err := json.NewEncoder(out).Encode(result); err != nil {
		return 1
	}
	return 0
}
