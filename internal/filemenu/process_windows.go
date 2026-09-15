package filemenu

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/zzl/go-win32api/v2/win32"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func prepareProcessRequest(cmd *exec.Cmd, r Request) {
	if r.Operation == "" {
		// A reused helper no longer inherits the foreground process's permission
		// as a newly launched child would. Grant it for this explicit menu only.
		win32.AllowSetForegroundWindow(uint32(cmd.Process.Pid))
	}
}

var desktopClient = helperClient{command: func(ctx context.Context) (*exec.Cmd, error) { return helperCommand(ctx) }}

func runPlatform(ctx context.Context, r Request) Result {
	result := desktopClient.run(ctx, r)
	if result.Outcome == Invoked || result.Outcome == Failed {
		preparation.Lock()
		preparation.paths = nil
		preparation.Unlock()
	}
	return result
}

// Serve keeps all shell objects on one STA. The reader wakes its message loop;
// even while idle it must dispatch COM messages and property-sheet callbacks.
func Serve(in io.Reader, out io.Writer) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	threadID := win32.GetCurrentThreadId()
	var msg win32.MSG
	win32.PeekMessageW(&msg, 0, 0, 0, win32.PM_NOREMOVE)
	requests := make(chan Request, 1)
	go func() {
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			var r Request
			if json.Unmarshal(scanner.Bytes(), &r) != nil {
				os.Exit(2)
			}
			requests <- r
			win32.PostThreadMessageW(threadID, win32.WM_APP, 0, 0)
		}
		// EOF also covers abrupt parent termination while a native menu is modal.
		os.Exit(0)
	}()
	s, err := newShellSession()
	if err != nil {
		return 1
	}
	defer s.close()
	encoder := json.NewEncoder(out)
	for {
		pumpShellMessages()
		select {
		case r := <-requests:
			var result Result
			if err := validate(r); err != nil {
				result = Result{Outcome: Unavailable, Error: err.Error()}
			} else {
				result = s.run(r)
			}
			if encoder.Encode(result) != nil {
				return 1
			}
		default:
			win32.MsgWaitForMultipleObjects(0, nil, 0, 50, win32.QS_ALLINPUT)
		}
	}
}

var preparation struct {
	sync.Mutex
	enabled    bool
	paths      []string
	timer      *time.Timer
	generation uint64
	updated    time.Time
}

// EnablePreparation is called only by normal application startup, never tests
// or private helper modes. The first visible local selection warms the helper.
func EnablePreparation() {
	desktopClient.processMu.Lock()
	desktopClient.closed = false
	desktopClient.processMu.Unlock()
	preparation.Lock()
	preparation.enabled = true
	preparation.Unlock()
}

// Prepare coalesces navigation into one request after the selection settles.
// It never materializes a VFS file or launches an external file action.
func Prepare(paths []string) {
	preparation.Lock()
	defer preparation.Unlock()
	if !preparation.enabled || (slices.Equal(paths, preparation.paths) && time.Since(preparation.updated) < 5*time.Second) {
		return
	}
	preparation.paths = slices.Clone(paths)
	preparation.updated = time.Now()
	preparation.generation++
	generation := preparation.generation
	if preparation.timer != nil {
		preparation.timer.Stop()
	}
	if len(paths) == 0 {
		return
	}
	paths = slices.Clone(paths)
	preparation.timer = time.AfterFunc(150*time.Millisecond, func() {
		// Do not cancel an extension halfway through loading when the cursor moves.
		// Its result will be replaced by the newest selection, never displayed.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		desktopClient.runWhen(ctx, Request{Paths: paths, Operation: "prepare"}, func() bool {
			preparation.Lock()
			defer preparation.Unlock()
			return preparation.enabled && preparation.generation == generation && !active.Load()
		})
	})
}

func Close() {
	preparation.Lock()
	preparation.enabled = false
	preparation.paths = nil
	preparation.generation++
	if preparation.timer != nil {
		preparation.timer.Stop()
	}
	preparation.Unlock()
	desktopClient.shutdown()
}
