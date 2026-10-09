package stallwatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFrame_DumpsStacksWhenAFrameOverruns(t *testing.T) {
	dir := t.TempDir()
	logPath := Start(dir, 40*time.Millisecond)
	if logPath == "" {
		t.Fatal("Start did not report a log path")
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("the log file is not there the moment the watchdog arms: %v", err)
	}
	t.Cleanup(func() { enabled.Store(false) })

	if !Enabled() {
		t.Fatal("watchdog did not arm")
	}

	func() {
		defer Frame("test.slowFrame")()
		time.Sleep(200 * time.Millisecond)
	}()

	entries := dumpsIn(t, dir)
	if len(entries) != 1 {
		t.Fatalf("wrote %d dumps for one overrunning frame, want 1", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0]))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "test.slowFrame") {
		t.Errorf("the dump does not name the frame:\n%s", text)
	}
	// The stacks themselves are the point of the file.
	if !strings.Contains(text, "goroutine") {
		t.Errorf("the dump holds no goroutine stacks:\n%s", text)
	}

	// A frame inside the limit is not worth a file, and neither is the quiet
	// after it: the thread was not busy, so the user simply stopped.
	before := len(entries)
	func() {
		defer Frame("test.quickFrame")()
	}()
	time.Sleep(200 * time.Millisecond)
	if now := dumpsIn(t, dir); len(now) != before {
		t.Errorf("a frame inside the limit was dumped: %d files, was %d", len(now), before)
	}

	// The log names what it found, and exists whether or not it found
	// anything.
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "armed at") {
		t.Errorf("the log does not record arming:\n%s", log)
	}
	if !strings.Contains(string(log), "SLOW") {
		t.Errorf("the log does not record the slow frame:\n%s", log)
	}
}

// A freeze in input delivery runs no code at all: the UI thread is idle
// because nothing reaches it. It is caught as a gap that interrupts steady
// work, which is what tells it apart from the user pausing.
func TestFrame_DumpsAGapThatInterruptsSteadyWork(t *testing.T) {
	dir := t.TempDir()
	Start(dir, 60*time.Millisecond)
	t.Cleanup(func() { enabled.Store(false) })
	lastEnd.Store(0)
	tightRun.Store(0)

	// A busy stretch: units of work close enough together to count as one.
	for i := 0; i < busyRun+2; i++ {
		func() { defer Frame("test.busy")() }()
	}
	if got := tightRun.Load(); got < busyRun {
		t.Fatalf("the busy stretch was not recognised: %d units", got)
	}

	time.Sleep(300 * time.Millisecond)

	if got := dumpsIn(t, dir); len(got) == 0 {
		t.Fatal("a gap interrupting steady work was not dumped")
	}
	// Work resuming is what puts the gap in the log, with its length.
	func() { defer Frame("test.resumed")() }()
	log, err := os.ReadFile(filepath.Join(dir, "stall-watchdog.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "GAP") {
		t.Errorf("the log does not record the gap:\n%s", log)
	}

	// What resumes after the gap is the evidence that says where the gap came
	// from, so it is counted and reported too.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		func() { defer Frame("test.resumed")() }()
		body, err := os.ReadFile(filepath.Join(dir, "stall-watchdog.log"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "RESUMED") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the log never recorded what resumed after the gap")
}

func dumpsIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var dumps []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "stall-2") {
			dumps = append(dumps, e.Name())
		}
	}
	return dumps
}

func TestFrame_IsInertWhenDisarmed(t *testing.T) {
	enabled.Store(false)
	done := Frame("test.unarmed")
	if openedAt.Load() != 0 {
		t.Error("a disarmed watchdog still opened a frame")
	}
	done()
}

// A watchdog whose directory cannot be made must not claim to be armed, or
// the user waits for a freeze and collects nothing.
func TestStart_RefusesADirectoryItCannotWrite(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "in-the-way")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Start(filepath.Join(blocked, "crashes"), 50*time.Millisecond); got != "" {
		t.Errorf("Start reported %q for a directory it cannot make", got)
	}
	if Enabled() {
		t.Error("the watchdog armed on a directory it cannot write")
	}
	// The failed path must not become where dumps are written. Anything a
	// previous run left is inert: the watchdog is disarmed, so Frame does
	// nothing at all.
	if strings.Contains(dumpDir, "in-the-way") || strings.Contains(logPath, "in-the-way") {
		t.Errorf("Start pointed at the directory it could not make: dumpDir=%q logPath=%q", dumpDir, logPath)
	}
	if done := Frame("test.afterFailedStart"); done != nil {
		done()
	}
}

// A limit of zero is a run that did not ask for the watchdog.
func TestStart_ZeroLimitDoesNotArm(t *testing.T) {
	if got := Start(t.TempDir(), 0); got != "" {
		t.Errorf("Start(0) reported %q", got)
	}
	if Enabled() {
		t.Error("a zero limit armed the watchdog")
	}
}

// A freeze that repeats writes the same stacks; the point is to read them,
// not to collect them.
func TestDump_StopsAtMaxDumps(t *testing.T) {
	dir := t.TempDir()
	Start(dir, 30*time.Millisecond)
	t.Cleanup(func() { enabled.Store(false) })

	for i := 0; i < MaxDumps+5; i++ {
		func() {
			defer Frame("test.slow")()
			time.Sleep(60 * time.Millisecond)
		}()
	}
	if got := len(dumpsIn(t, dir)); got != MaxDumps {
		t.Errorf("wrote %d dumps, want the cap of %d", got, MaxDumps)
	}
}

// Start rewrites the directory and the log path while a watcher from an
// earlier Start may be dumping to them. Under -race this failed on main as a
// read in dump against a write in Start.
func TestStart_RestartingWhileAWatcherDumpsDoesNotRace(t *testing.T) {
	t.Cleanup(func() { enabled.Store(false) })
	for i := 0; i < 6; i++ {
		if Start(t.TempDir(), 16*time.Millisecond) == "" {
			t.Fatal("Start did not arm")
		}
		func() {
			defer Frame("test.restart")()
			time.Sleep(50 * time.Millisecond)
		}()
	}
}

// Start must not claim to be armed if the log file itself cannot be
// written, even when the directory that holds it exists and MkdirAll is
// happy with it: MkdirAll only checks the directory, not whether a file can
// be created inside it.
func TestStart_CannotCreateLogFileDisarms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner write bits do not block file creation the same way on Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	// Restore write access before TempDir's own cleanup tries to remove dir.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	t.Cleanup(func() { enabled.Store(false) })

	if got := Start(dir, 50*time.Millisecond); got != "" {
		t.Errorf("Start reported %q for a directory whose log file it cannot create", got)
	}
	if Enabled() {
		t.Error("the watchdog armed although its log file could not be created")
	}
	if _, err := os.Stat(filepath.Join(dir, "stall-watchdog.log")); err == nil {
		t.Error("the log file exists although Start reported failure")
	}
}

// Units do not nest: a Frame opened while another is still open must be
// ignored, so the outermost one measures what the user actually waited for.
func TestFrame_NestedCallIsIgnored(t *testing.T) {
	dir := t.TempDir()
	if Start(dir, 50*time.Millisecond) == "" {
		t.Fatal("Start did not arm")
	}
	t.Cleanup(func() { enabled.Store(false) })

	doneOuter := Frame("outer")
	openedAfterOuter := openedAt.Load()
	if openedAfterOuter == 0 {
		t.Fatal("the outer frame did not open")
	}

	doneInner := Frame("inner")
	if got := openedAt.Load(); got != openedAfterOuter {
		t.Errorf("a nested frame changed openedAt from %d to %d", openedAfterOuter, got)
	}

	// The nested call's closer must be inert: closing it must not clear the
	// outer frame's state.
	doneInner()
	if got := openedAt.Load(); got != openedAfterOuter {
		t.Errorf("closing the nested frame cleared openedAt: got %d, want %d", got, openedAfterOuter)
	}

	doneOuter()
	if got := openedAt.Load(); got != 0 {
		t.Errorf("closing the outer frame left openedAt = %d, want 0", got)
	}
}

// watchTick's own guard must stop the loop the moment the watchdog is
// disarmed or its limit drops to zero -- in practice this never happens to
// a running watcher (enabled and threshold only ever move one way after
// Start), so the only way to exercise the guard is to drive watchTick
// directly with state it would otherwise never see.
func TestWatchTick_StopsWhenDisarmed(t *testing.T) {
	enabled.Store(false)
	threshold.Store(int64(time.Second))
	if watchTick() {
		t.Error("watchTick kept looping although the watchdog is disarmed")
	}
}

func TestWatchTick_StopsWhenLimitIsZero(t *testing.T) {
	enabled.Store(true)
	t.Cleanup(func() { enabled.Store(false) })
	threshold.Store(0)
	if watchTick() {
		t.Error("watchTick kept looping with a zero limit")
	}
}

// A gap can only be reported once something has actually finished: without
// that, there is nothing to measure the quiet stretch from.
func TestWatchTick_SkipsGapCheckWhenNothingHasFinishedYet(t *testing.T) {
	enabled.Store(true)
	t.Cleanup(func() { enabled.Store(false) })
	threshold.Store(int64(4 * time.Millisecond))
	openedAt.Store(0)
	reported.Store(false)
	gapReported.Store(false)
	tightRun.Store(busyRun)
	lastEnd.Store(0)

	if !watchTick() {
		t.Fatal("watchTick stopped although the watchdog is armed with a positive limit")
	}
	if gapReported.Load() {
		t.Error("watchTick reported a gap although nothing had finished yet (lastEnd == 0)")
	}
}

// logLocked is only ever called while holding mu, by callers (logf, dump)
// that already guard against no watchdog having been armed -- but the
// guard lives in logLocked itself, so it must be safe on its own.
func TestLogLocked_NoopWhenNoLogPath(t *testing.T) {
	mu.Lock()
	saved := logPath
	logPath = ""
	logLocked("must be inert, nothing to write to: %d", 1)
	logPath = saved
	mu.Unlock()
}
