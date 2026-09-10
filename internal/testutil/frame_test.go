package testutil

import (
	"github.com/unxed/vtui"
	"testing"
	"time"
)

// The one test of the harness itself: SwapFrameManager's restore closes the
// fresh manager, and a manager that leaks its task pump would leave every
// later test running against a queue nobody drains.
func TestFrameManagerShutdownStopsTaskPumpAndUnblocksPostTask(t *testing.T) {
	before, _, err := TaskPumpGoroutineProfile()
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
	started, profile, err := TaskPumpGoroutineProfile()
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
		after, profile, profileErr := TaskPumpGoroutineProfile()
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
