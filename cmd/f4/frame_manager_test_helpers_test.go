package main

import (
	"bytes"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
	"github.com/unxed/vtui/vreactive"
)

func closeFrameManagerScreens(screens []*vtui.AppScreen) {
	for _, screen := range screens {
		if screen == nil {
			continue
		}
		for _, frame := range screen.Frames {
			if frame != nil {
				frame.Close()
			}
		}
	}
}

func closeFrameManagerFrames(manager *vtui.FrameManagerType) {
	if manager != nil {
		closeFrameManagerScreens(manager.Screens)
	}
}

// setFrameManagerScreensForTest installs a screen fixture for the current
// manager. Its cleanup closes exactly the frames supplied by the fixture and
// then restores the manager's previous screen state.
func setFrameManagerScreensForTest(t *testing.T, screens []*vtui.AppScreen, activeIdx int) func() {
	t.Helper()
	manager := vtui.FrameManager
	oldScreens := manager.Screens
	oldActiveIdx := manager.ActiveIdx
	manager.Screens = screens
	manager.ActiveIdx = activeIdx
	return func() {
		closeFrameManagerScreens(screens)
		manager.Screens = oldScreens
		manager.ActiveIdx = oldActiveIdx
	}
}

// appendFrameManagerScreenForTest adds one owned screen fixture and removes it
// during cleanup after closing its frames.
func appendFrameManagerScreenForTest(t *testing.T, screen *vtui.AppScreen, activeIdx int) func() {
	t.Helper()
	manager := vtui.FrameManager
	oldScreens := manager.Screens
	oldActiveIdx := manager.ActiveIdx
	manager.Screens = append(manager.Screens, screen)
	manager.ActiveIdx = activeIdx
	return func() {
		closeFrameManagerScreens([]*vtui.AppScreen{screen})
		manager.Screens = oldScreens
		manager.ActiveIdx = oldActiveIdx
	}
}

func taskPumpGoroutineProfile() (int, string, error) {
	var stacks bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&stacks, 2); err != nil {
		return 0, "", err
	}
	profile := stacks.String()
	return strings.Count(profile, ".startTaskPump.func"), profile, nil
}

// waitForDirectoryLoads blocks until no directory-load worker is running
// anywhere in the process.
//
// The workers read vtui.FrameManager and AppConfig while they run, so a test
// that replaces either one has to know they are all finished first. Panels are
// created deep inside PanelsFrame.ResizeConsole as well as directly, so the
// caller usually has no panel to wait on and this asks the question globally
// instead.
func waitForDirectoryLoads(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		directoryLoadWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for the directory-load workers to stop")
	}
}

func pumpUntilToastActive(t *testing.T) {
	t.Helper()
	for vtui.FrameManager.GetActiveToast() == "" {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(time.Second):
			t.Fatal("toast did not start")
		}
	}
}

func waitForToastExpiry(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
			if vtui.FrameManager.GetActiveToast() == "" {
				return
			}
		case <-deadline:
			t.Fatal("toast did not expire")
		}
	}
}

// swapFrameManager replaces the global vtui.FrameManager with a fresh,
// independent instance and returns a function that restores the original
// pointer. In particular, the fresh manager has its own task queue: Init
// deliberately preserves an existing queue, which is useful in production
// but lets queued UI work escape from one test into the next.
func swapFrameManager(t *testing.T) func() {
	t.Helper()
	// Clipboard writes run asynchronously because they may wait for far2l IPC.
	// SetClipboard reads vtui.FrameManager, so finish those workers before
	// replacing the global manager.
	waitForAsyncClipboard()
	// A directory-load worker left running by an earlier test reads the
	// manager this is about to replace, which the race detector reports
	// against whichever test is unlucky enough to do the replacing. Joining
	// them first is what makes the swap safe.
	waitForDirectoryLoads(t)
	old := vtui.FrameManager
	oldUpdateQueue := vreactive.GlobalUpdateQueue
	oldAnimationManager := vreactive.GlobalAnimationManager
	fresh := vtui.NewFrameManager()
	vtui.FrameManager = fresh

	return func() {
		waitForAsyncClipboard()
		waitForDirectoryLoads(t)
		closeFrameManagerFrames(fresh)
		fresh.Shutdown()
		vtui.FrameManager = old
		vreactive.GlobalUpdateQueue = oldUpdateQueue
		vreactive.GlobalAnimationManager = oldAnimationManager
	}
}

func TestFrameManagerShutdownStopsTaskPumpAndUnblocksPostTask(t *testing.T) {
	before, _, err := taskPumpGoroutineProfile()
	if err != nil {
		t.Fatalf("capture initial goroutine profile: %v", err)
	}

	manager := vtui.NewFrameManager()
	manager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(manager.Shutdown)
	taskChan := manager.TaskChan
	if taskChan == nil {
		t.Fatal("Init did not create TaskChan")
	}
	started, profile, err := taskPumpGoroutineProfile()
	if err != nil {
		t.Fatalf("capture running goroutine profile: %v", err)
	}
	if started <= before {
		t.Fatalf("Init did not start a visible task-pump goroutine: before=%d after=%d\n%s", before, started, profile)
	}

	manager.PostTask(func() {})

	start := make(chan struct{})
	postReturned := make(chan struct{})
	shutdownReturned := make(chan struct{})
	go func() {
		<-start
		manager.PostTask(func() {})
		close(postReturned)
	}()
	go func() {
		<-start
		manager.Shutdown()
		close(shutdownReturned)
	}()
	close(start)

	for _, operation := range []struct {
		name string
		done <-chan struct{}
	}{
		{name: "PostTask", done: postReturned},
		{name: "Shutdown", done: shutdownReturned},
	} {
		select {
		case <-operation.done:
		case <-time.After(time.Second):
			t.Fatalf("concurrent %s did not return after manager shutdown", operation.name)
		}
	}

	if manager.TaskChan != nil {
		t.Fatal("Shutdown left TaskChan attached to the manager")
	}
	select {
	case <-taskChan:
		t.Fatal("the stopped task pump delivered a queued task")
	default:
	}

	deadline := time.Now().Add(time.Second)
	for {
		after, profile, profileErr := taskPumpGoroutineProfile()
		if profileErr != nil {
			t.Fatalf("capture final goroutine profile: %v", profileErr)
		}
		if after <= before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("FrameManager task pump did not exit: before=%d after=%d\n%s", before, after, profile)
		}
		time.Sleep(time.Millisecond)
	}
}
