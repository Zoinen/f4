package dirwatch

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func waitCount(t *testing.T, n *atomic.Int32, want int32, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if n.Load() >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return n.Load() >= want
}

// TestDebounceCoalescesABurst: fifty events in a row make one call, and a
// later event makes another.
func TestDebounceCoalescesABurst(t *testing.T) {
	events := make(chan struct{}, 100)
	stop := make(chan struct{})
	done := make(chan struct{})
	var calls atomic.Int32
	go func() {
		defer close(done)
		debounce(events, stop, 30*time.Millisecond, time.Second, func() { calls.Add(1) })
	}()
	for i := 0; i < 50; i++ {
		events <- struct{}{}
	}
	if !waitCount(t, &calls, 1, 2*time.Second) {
		t.Fatal("the burst produced no call")
	}
	time.Sleep(100 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls after one burst = %d, want 1", got)
	}
	events <- struct{}{}
	if !waitCount(t, &calls, 2, 2*time.Second) {
		t.Fatal("a later event produced no call")
	}
	close(stop)
	<-done
}

// TestDebounceMaxWaitCapsAContinuousBurst: events keep coming faster than the
// quiet period, yet the callback still runs, at the MaxWait pace.
func TestDebounceMaxWaitCapsAContinuousBurst(t *testing.T) {
	events := make(chan struct{}, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	var calls atomic.Int32
	go func() {
		defer close(done)
		debounce(events, stop, 200*time.Millisecond, 80*time.Millisecond, func() { calls.Add(1) })
	}()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		<-tick.C
		select {
		case events <- struct{}{}:
		default:
		}
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("calls during a continuous burst = %d, want at least 2", got)
	}
	close(stop)
	<-done
}

func TestDebounceStopsWithAPendingEvent(t *testing.T) {
	events := make(chan struct{}, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	var calls atomic.Int32
	go func() {
		defer close(done)
		debounce(events, stop, time.Hour, time.Hour, func() { calls.Add(1) })
	}()
	events <- struct{}{}
	time.Sleep(20 * time.Millisecond)
	close(stop)
	<-done
	if calls.Load() != 0 {
		t.Fatal("a call was made after stop")
	}
}

func watchAndTouch(t *testing.T, opts Options, wantNative bool) {
	t.Helper()
	dir := t.TempDir()
	var calls atomic.Int32
	w, err := Watch(dir, opts, func() { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if wantNative && !w.Native() {
		t.Skip("no OS notifications available here")
	}
	if !wantNative && w.Native() {
		t.Fatal("ForcePolling still used the native watcher")
	}

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitCount(t, &calls, 1, 5*time.Second) {
		t.Fatal("creating a file was not reported")
	}
	before := calls.Load()
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if !waitCount(t, &calls, before+1, 5*time.Second) {
		t.Fatal("removing a file was not reported")
	}
}

func TestWatchPollingReportsChanges(t *testing.T) {
	watchAndTouch(t, Options{ForcePolling: true, PollInterval: 20 * time.Millisecond, Quiet: 10 * time.Millisecond}, false)
}

func TestWatchNativeReportsChanges(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "windows", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly":
	default:
		t.Skip("no native watcher on this OS")
	}
	watchAndTouch(t, Options{Quiet: 10 * time.Millisecond}, true)
}

func TestWatchQuietWithoutChanges(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	w, err := Watch(dir, Options{PollInterval: 10 * time.Millisecond, Quiet: 10 * time.Millisecond}, func() { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	w.Close()
	w.Close() // safe twice
	if got := calls.Load(); got != 0 {
		t.Fatalf("calls without any change = %d, want 0", got)
	}
}

func TestWatchRejectsBadArguments(t *testing.T) {
	dir := t.TempDir()
	if _, err := Watch(dir, Options{}, nil); err == nil {
		t.Error("nil callback accepted")
	}
	if _, err := Watch(filepath.Join(dir, "missing"), Options{}, func() {}); err == nil {
		t.Error("missing directory accepted")
	}
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Watch(file, Options{}, func() {}); err == nil {
		t.Error("a file accepted as a directory")
	}
}

func TestOptionsDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.Quiet != defaultQuiet || o.MaxWait != defaultMaxWait || o.PollInterval != defaultPoll {
		t.Errorf("defaults = %+v", o)
	}
	o = Options{Quiet: 2 * time.Second, MaxWait: time.Second}.withDefaults()
	if o.MaxWait != o.Quiet {
		t.Errorf("MaxWait %v below Quiet %v was not raised", o.MaxWait, o.Quiet)
	}
}

func TestSnapshotSeesSizeAndVanishing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s1 := snapshot(dir)
	if err := os.WriteFile(f, []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	if snapshot(dir) == s1 {
		t.Error("a size change left the snapshot as it was")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	gone := snapshot(dir)
	if gone == s1 || gone != snapshot(dir) {
		t.Error("a vanished directory must give one stable, different snapshot")
	}
}
