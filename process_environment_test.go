package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type fakeProcessEnvironmentBackend struct {
	entries map[string]processEnvironmentEntry
	order   []string
	ops     []string
	calls   int
	fail    map[int]error
}

func newFakeProcessEnvironmentBackend(values ...string) *fakeProcessEnvironmentBackend {
	fake := &fakeProcessEnvironmentBackend{entries: make(map[string]processEnvironmentEntry)}
	for _, raw := range values {
		name, value, ok := splitProcessEnvironmentEntry(raw)
		if !ok {
			continue
		}
		key := processEnvironmentKey(name)
		fake.entries[key] = processEnvironmentEntry{name: name, value: value}
		fake.order = append(fake.order, key)
	}
	return fake
}

func (f *fakeProcessEnvironmentBackend) Environ() []string {
	result := make([]string, 0, len(f.entries))
	seen := make(map[string]bool, len(f.entries))
	for _, key := range f.order {
		if entry, ok := f.entries[key]; ok && !seen[key] {
			result = append(result, entry.name+"="+entry.value)
			seen[key] = true
		}
	}
	for key, entry := range f.entries {
		if !seen[key] {
			result = append(result, entry.name+"="+entry.value)
		}
	}
	return result
}

func (f *fakeProcessEnvironmentBackend) operationError() error {
	f.calls++
	if f.fail != nil {
		return f.fail[f.calls]
	}
	return nil
}

func (f *fakeProcessEnvironmentBackend) Setenv(name, value string) error {
	f.ops = append(f.ops, "set:"+name)
	if err := f.operationError(); err != nil {
		return err
	}
	key := processEnvironmentKey(name)
	if _, exists := f.entries[key]; !exists {
		f.order = append(f.order, key)
	}
	f.entries[key] = processEnvironmentEntry{name: name, value: value}
	return nil
}

func (f *fakeProcessEnvironmentBackend) Unsetenv(name string) error {
	f.ops = append(f.ops, "unset:"+name)
	if err := f.operationError(); err != nil {
		return err
	}
	delete(f.entries, processEnvironmentKey(name))
	return nil
}

func snapshotVariable(snapshot vfs.ProcessEnvironmentSnapshot, name string) (string, bool) {
	key := processEnvironmentKey(name)
	for _, variable := range snapshot.Variables {
		if processEnvironmentKey(variable.Name) == key {
			return variable.Value, true
		}
	}
	return "", false
}

func TestProcessEnvironmentSnapshotStableAndNoOpGeneration(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("B=2", "A=1")
	manager := newProcessEnvironmentManager(backend)
	snapshot, records := manager.snapshot()
	if snapshot.Generation != 0 || len(records) != 0 {
		t.Fatalf("initial snapshot = generation %d, records %v", snapshot.Generation, records)
	}
	if got := []string{snapshot.Variables[0].Name, snapshot.Variables[1].Name}; strings.Join(got, ",") != "A,B" {
		t.Fatalf("snapshot order = %v", got)
	}

	snapshot, records, err := manager.apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "1"}})
	if err != nil || snapshot.Generation != 0 || len(records) != 0 {
		t.Fatalf("no-op apply = generation %d, records %v, error %v", snapshot.Generation, records, err)
	}
	if strings.Join(backend.ops, ",") != "set:A" {
		t.Fatalf("backend operations = %v", backend.ops)
	}
}

func TestProcessEnvironmentApplyPreservesOrderAndReturnsActualSnapshot(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old", "B=old")
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	changes := []vfs.ProcessEnvironmentChange{
		{Name: "B", Unset: true},
		{Name: "A", Value: "new"},
		{Name: "C", Value: "three"},
	}
	snapshot, records, err := manager.apply(changes)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(backend.ops, ","); got != "unset:B,set:A,set:C" {
		t.Fatalf("backend operation order = %s", got)
	}
	if snapshot.Generation != 1 || len(records) != 1 || records[0].generation != 1 {
		t.Fatalf("apply generations = snapshot %d, records %#v", snapshot.Generation, records)
	}
	if got, ok := snapshotVariable(snapshot, "A"); !ok || got != "new" {
		t.Fatalf("A in actual snapshot = %q, %v", got, ok)
	}
	if _, ok := snapshotVariable(snapshot, "B"); ok {
		t.Fatal("B remained in actual snapshot")
	}
	if got := []string{records[0].changes[0].Name, records[0].changes[1].Name, records[0].changes[2].Name}; strings.Join(got, ",") != "B,A,C" {
		t.Fatalf("delivered change order = %v", got)
	}
}

func TestProcessEnvironmentValidatesWholeBatchBeforeMutation(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old")
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	_, _, err := manager.apply([]vfs.ProcessEnvironmentChange{
		{Name: "A", Value: "new"},
		{Name: "NOT-PORTABLE", Value: "bad"},
	})
	if err == nil || len(backend.ops) != 0 {
		t.Fatalf("invalid batch error = %v, operations = %v", err, backend.ops)
	}
	_, _, err = manager.apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "line\nbreak"}})
	if err == nil || len(backend.ops) != 0 {
		t.Fatalf("line-break validation error = %v, operations = %v", err, backend.ops)
	}
	// Value is deliberately ignored for Unset.
	_, _, err = manager.apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "line\nbreak", Unset: true}})
	if err != nil {
		t.Fatalf("unset rejected ignored value: %v", err)
	}
}

func TestProcessEnvironmentRejectsUntransportableWindowsValueBeforeMutation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe live-update limit")
	}
	const name = "BOUNDARY"
	allowed := strings.Repeat("x", windowsCmdEnvironmentAssignmentLimit-len(name))
	allowedBackend := newFakeProcessEnvironmentBackend()
	allowedManager := newProcessEnvironmentManager(allowedBackend)
	allowedManager.snapshot()
	if _, _, err := allowedManager.apply([]vfs.ProcessEnvironmentChange{{Name: name, Value: allowed}}); err != nil {
		t.Fatalf("exact cmd.exe boundary rejected: %v", err)
	}
	if len(allowedBackend.ops) != 1 {
		t.Fatalf("exact boundary operations = %v", allowedBackend.ops)
	}

	backend := newFakeProcessEnvironmentBackend("A=old")
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	_, _, err := manager.apply([]vfs.ProcessEnvironmentChange{{Name: name, Value: allowed + "x"}})
	if err == nil {
		t.Fatal("expected cmd.exe length validation error")
	}
	if len(backend.ops) != 0 {
		t.Fatalf("overlong value mutated backend: %v", backend.ops)
	}
}

func TestProcessEnvironmentRuntimeFailurePreventsMutation(t *testing.T) {
	blockedRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("block mkdir"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := newFakeProcessEnvironmentBackend("A=old")
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	snapshot, records, err := applyProcessEnvironmentWithRuntime(
		manager,
		func() error {
			_, err := createProcessEnvironmentRuntimeSession(blockedRoot)
			return err
		},
		[]vfs.ProcessEnvironmentChange{{Name: "A", Value: "new"}},
	)
	if err == nil || !strings.Contains(err.Error(), "private environment runtime") {
		t.Fatalf("runtime initialization error = %v", err)
	}
	if len(backend.ops) != 0 || len(records) != 0 || snapshot.Generation != 0 {
		t.Fatalf("runtime failure mutated state: ops %v, records %#v, generation %d", backend.ops, records, snapshot.Generation)
	}
	if got, _ := snapshotVariable(snapshot, "A"); got != "old" {
		t.Fatalf("runtime failure snapshot A = %q", got)
	}
}

func TestProcessEnvironmentRollsBackInReverseOrder(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old-a", "B=old-b")
	backend.fail = map[int]error{3: errors.New("injected apply failure")}
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	snapshot, records, err := manager.apply([]vfs.ProcessEnvironmentChange{
		{Name: "A", Value: "new-a"},
		{Name: "B", Unset: true},
		{Name: "C", Value: "new-c"},
	})
	if err == nil {
		t.Fatal("expected apply error")
	}
	if got := strings.Join(backend.ops, ","); got != "set:A,unset:B,set:C,set:B,set:A" {
		t.Fatalf("operation/rollback order = %s", got)
	}
	if snapshot.Generation != 0 || len(records) != 0 {
		t.Fatalf("rolled-back generation = %d, records %#v", snapshot.Generation, records)
	}
	if got, _ := snapshotVariable(snapshot, "A"); got != "old-a" {
		t.Fatalf("A after rollback = %q", got)
	}
	if got, _ := snapshotVariable(snapshot, "B"); got != "old-b" {
		t.Fatalf("B after rollback = %q", got)
	}
}

func TestProcessEnvironmentReportsAndDistributesRollbackDrift(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old")
	backend.fail = map[int]error{
		2: errors.New("injected apply failure"),
		3: errors.New("injected rollback failure"),
	}
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	snapshot, records, err := manager.apply([]vfs.ProcessEnvironmentChange{
		{Name: "A", Value: "new"},
		{Name: "B", Value: "fail"},
	})
	if err == nil || !strings.Contains(err.Error(), "roll back") {
		t.Fatalf("rollback error = %v", err)
	}
	if snapshot.Generation != 1 || len(records) != 1 || len(records[0].changes) != 1 {
		t.Fatalf("rollback drift = snapshot %d, records %#v", snapshot.Generation, records)
	}
	if got, _ := snapshotVariable(snapshot, "A"); got != "new" {
		t.Fatalf("actual A after failed rollback = %q", got)
	}
}

func TestProcessEnvironmentExternalDriftIsNeverInApplyDelivery(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old", "PROMPT=original")
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	if err := backend.Setenv("PROMPT", "external"); err != nil {
		t.Fatal(err)
	}
	backend.ops = nil
	snapshot, records, err := manager.apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "new"}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 2 || len(records) != 1 || len(records[0].changes) != 1 || records[0].changes[0].Name != "A" {
		t.Fatalf("external drift leaked into delivery: snapshot %d, records %#v", snapshot.Generation, records)
	}
}

func TestProcessEnvironmentHistoryUsesManagedFallback(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend()
	manager := newProcessEnvironmentManager(backend)
	manager.snapshot()
	for i := 0; i < processEnvironmentHistoryLimit+2; i++ {
		name := "V" + string(rune('A'+i%26))
		value := string(rune('0' + i%10))
		if _, _, err := manager.apply([]vfs.ProcessEnvironmentChange{{Name: name, Value: value}}); err != nil {
			t.Fatal(err)
		}
	}
	generation, changes := manager.changesSince(0)
	if generation == 0 || len(changes) == 0 || manager.historyFloor == 0 {
		t.Fatalf("fallback generation %d, changes %d, floor %d", generation, len(changes), manager.historyFloor)
	}
}

func TestProcessEnvironmentPrivatePayloadsDoNotPutValuesOnWire(t *testing.T) {
	changes := []vfs.ProcessEnvironmentChange{
		{Name: "SECRET", Value: "spaces & percent% bang! quote' euro-€"},
		{Name: "REMOVE_ME", Unset: true},
	}
	posixPayload, err := preparePOSIXProcessEnvironmentShellPayload(changes)
	if err != nil {
		t.Fatal(err)
	}
	defer posixPayload.cleanup()
	if strings.Contains(string(posixPayload.wire), changes[0].Value) || strings.Contains(string(posixPayload.wire), "euro") {
		t.Fatalf("POSIX wire contains value: %q", posixPayload.wire)
	}
	if !strings.Contains(string(posixPayload.wire), ".envman-runtime") {
		t.Fatalf("POSIX wire does not source private runtime: %q", posixPayload.wire)
	}

	chunks := windowsProcessEnvironmentValueChunks(changes[0].Value)
	paths := make([]string, len(chunks))
	var values []byte
	for i, chunk := range chunks {
		paths[i] = `C:\runtime\value-` + string(rune('A'+i))
		values = append(values, chunk...)
	}
	script := windowsProcessEnvironmentScript(changes, [][]string{paths, nil}, `C:\runtime\ok`, `C:\runtime\fail`, "F4_CP_TEST", "F4_STATUS_TEST", "F4_CHUNK_TEST")
	wire := windowsProcessEnvironmentCommand(`C:\runtime\script.cmd`, `C:\runtime\fail`)
	if strings.Contains(string(wire), changes[0].Value) || strings.Contains(string(script), changes[0].Value) ||
		strings.Contains(string(wire), "euro") || strings.Contains(string(script), "euro") {
		t.Fatalf("Windows command/script contains value: wire %q, script %q", wire, script)
	}
	if got, want := string(values), changes[0].Value; got != want {
		t.Fatalf("Windows UTF-8 value stream = %q, want %q", got, want)
	}
}

func TestPOSIXProcessEnvironmentPayloadHidesValuesWithVerboseTracing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell transport")
	}
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh in PATH")
	}
	const name = "F4_ENV_VERBOSE_SECRET"
	const value = " leading '$value' & punctuation trailing "
	payload, err := preparePOSIXProcessEnvironmentShellPayload([]vfs.ProcessEnvironmentChange{{Name: name, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	defer payload.cleanup()
	resultPath := filepath.Join(t.TempDir(), "result")
	command := "set -evx\n" + strings.ReplaceAll(string(payload.wire), "\r", "\n") +
		"set +vx\nprintf '%s' \"$" + name + "\" > " + posixProcessEnvironmentQuote(resultPath) + "\n"
	cmd := exec.Command(shellPath)
	cmd.Stdin = strings.NewReader(command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("traced POSIX transport failed: %v (output %q)", err, output)
	}
	if bytes.Contains(output, []byte(value)) {
		t.Fatalf("verbose/xtrace output leaked private value: %q", output)
	}
	if !bytes.Contains(output, []byte("\x1b]133;E;"+payload.token+";0\x07")) {
		t.Fatalf("traced POSIX transport did not acknowledge success: %q", output)
	}
	result, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != value {
		t.Fatalf("POSIX value round trip = %q, want %q", result, value)
	}

	t.Run("MissingScriptStillFailsWithErrexit", func(t *testing.T) {
		missing, err := preparePOSIXProcessEnvironmentShellPayload([]vfs.ProcessEnvironmentChange{{Name: name, Value: "missing-private-value"}})
		if err != nil {
			t.Fatal(err)
		}
		missing.cleanup()
		sentinel := filepath.Join(t.TempDir(), "continued")
		command := "set -evx\n" + strings.ReplaceAll(string(missing.wire), "\r", "\n") +
			"printf continued > " + posixProcessEnvironmentQuote(sentinel) + "\n"
		cmd := exec.Command(shellPath)
		cmd.Stdin = strings.NewReader(command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("missing script terminated errexit shell: %v (output %q)", err, output)
		}
		if !bytes.Contains(output, []byte("\x1b]133;E;"+missing.token+";1\x07")) {
			t.Fatalf("missing script did not emit failure acknowledgement: %q", output)
		}
		if _, err := os.Stat(sentinel); err != nil {
			t.Fatalf("shell did not continue after failure acknowledgement: %v", err)
		}
	})

	t.Run("UnreadableScriptStillFailsWithErrexit", func(t *testing.T) {
		unreadable, err := preparePOSIXProcessEnvironmentShellPayload([]vfs.ProcessEnvironmentChange{{Name: name, Value: "unreadable-private-value"}})
		if err != nil {
			t.Fatal(err)
		}
		defer unreadable.cleanup()
		scriptPath := unreadable.scriptPath
		if scriptPath == "" {
			t.Fatal("could not recover private script path from value-free wire")
		}
		if err := os.Chmod(scriptPath, 0); err != nil {
			t.Fatal(err)
		}
		readableCheck := exec.Command(shellPath, "-c", "test -r "+posixProcessEnvironmentQuote(scriptPath))
		if readableCheck.Run() == nil {
			t.Skip("current user can still read a mode-000 file")
		}
		command := "set -evx\n" + strings.ReplaceAll(string(unreadable.wire), "\r", "\n")
		cmd := exec.Command(shellPath)
		cmd.Stdin = strings.NewReader(command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("unreadable script terminated errexit shell: %v (output %q)", err, output)
		}
		if !bytes.Contains(output, []byte("\x1b]133;E;"+unreadable.token+";1\x07")) {
			t.Fatalf("unreadable script did not emit failure acknowledgement: %q", output)
		}
	})
}

type processEnvironmentPTY struct {
	mu     sync.Mutex
	busy   bool
	writes [][]byte
	err    error
}

func (p *processEnvironmentPTY) Read([]byte) (int, error) { return 0, io.EOF }
func (p *processEnvironmentPTY) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return 0, p.err
	}
	p.writes = append(p.writes, append([]byte(nil), data...))
	return len(data), nil
}
func (p *processEnvironmentPTY) Close() error                { return nil }
func (p *processEnvironmentPTY) SetSize(int, int)            {}
func (p *processEnvironmentPTY) Wait() error                 { return nil }
func (p *processEnvironmentPTY) Run(string, ...string) error { return nil }
func (p *processEnvironmentPTY) IsBusy() bool                { return p.busy }

func (p *processEnvironmentPTY) snapshotWrites() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([][]byte, len(p.writes))
	for i := range p.writes {
		result[i] = append([]byte(nil), p.writes[i]...)
	}
	return result
}

func TestProcessEnvironmentBroadcastReachesEveryLocalWorkspace(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	firstPTY := &processEnvironmentPTY{}
	secondPTY := &processEnvironmentPTY{}
	first := &PanelsFrame{pty: firstPTY, termView: NewTerminalView(80, 24)}
	second := &PanelsFrame{pty: secondPTY, termView: NewTerminalView(80, 24)}
	defer first.closeProcessEnvironmentShell()
	defer second.closeProcessEnvironmentShell()
	vtui.FrameManager.Screens = []*vtui.AppScreen{
		{Number: 1, Frames: []vtui.Frame{first}},
		{Number: 2, Frames: []vtui.Frame{second}},
	}

	broadcastProcessEnvironmentGenerations([]processEnvironmentGeneration{{
		generation: 12,
		changes:    []vfs.ProcessEnvironmentChange{{Name: "BROADCAST_PRIVATE", Value: "workspace-value"}},
	}})
	deadline := time.After(time.Second)
	for len(firstPTY.snapshotWrites()) == 0 || len(secondPTY.snapshotWrites()) == 0 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("environment broadcast did not reach every local workspace")
		}
	}

	for index, frame := range []*PanelsFrame{first, second} {
		frame.processEnvironmentMu.Lock()
		inFlight := frame.processEnvironmentInFlight
		frame.processEnvironmentMu.Unlock()
		if inFlight == nil || inFlight.generation != 12 || len(inFlight.changes) != 1 || inFlight.changes[0].Value != "workspace-value" {
			t.Fatalf("workspace %d in-flight update = %#v", index+1, inFlight)
		}
		frame.processEnvironmentShellOutput([]byte("\x1b]133;E;" + inFlight.token + ";0\a"))
	}
}

func TestPanelsFrameEnvironmentNewShellInheritanceAndSpawnCatchUp(t *testing.T) {
	oldManager := globalProcessEnvironment
	backend := newFakeProcessEnvironmentBackend("BASELINE=initial")
	manager := newProcessEnvironmentManager(backend)
	globalProcessEnvironment = manager
	defer func() { globalProcessEnvironment = oldManager }()

	initial, _ := manager.snapshot()
	if initial.Generation != 0 {
		t.Fatalf("initial generation = %d, want 0", initial.Generation)
	}
	applied, _, err := manager.apply([]vfs.ProcessEnvironmentChange{{Name: "DURING_SPAWN", Value: "private"}})
	if err != nil {
		t.Fatal(err)
	}

	upToDatePTY := &processEnvironmentPTY{}
	upToDate := &PanelsFrame{pty: upToDatePTY, termView: NewTerminalView(80, 24)}
	defer upToDate.closeProcessEnvironmentShell()
	upToDate.localShellStarted(applied.Generation)
	if writes := upToDatePTY.snapshotWrites(); len(writes) != 0 {
		t.Fatalf("new shell that inherited generation %d received redundant writes: %q", applied.Generation, writes)
	}

	catchUpPTY := &processEnvironmentPTY{}
	catchUp := &PanelsFrame{pty: catchUpPTY, termView: NewTerminalView(80, 24)}
	defer catchUp.closeProcessEnvironmentShell()
	catchUp.localShellStarted(initial.Generation)
	if writes := catchUpPTY.snapshotWrites(); len(writes) != 1 {
		t.Fatalf("shell created across generation change received %d catch-up writes, want 1", len(writes))
	}
	catchUp.processEnvironmentMu.Lock()
	inFlight := catchUp.processEnvironmentInFlight
	catchUp.processEnvironmentMu.Unlock()
	if inFlight == nil || inFlight.generation != applied.Generation || len(inFlight.changes) != 1 || inFlight.changes[0].Name != "DURING_SPAWN" {
		t.Fatalf("spawn catch-up update = %#v", inFlight)
	}
	catchUp.processEnvironmentShellOutput([]byte("\x1b]133;E;" + inFlight.token + ";0\a"))
}

func TestPanelsFrameEnvironmentBusyDefersCoalescesAndAcknowledges(t *testing.T) {
	pty := &processEnvironmentPTY{busy: true}
	pf := &PanelsFrame{pty: pty, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(1, []vfs.ProcessEnvironmentChange{{Name: "SECRET", Value: "first"}}, true)
	pf.queueProcessEnvironment(2, []vfs.ProcessEnvironmentChange{{Name: "SECRET", Value: "second"}}, true)
	if writes := pty.snapshotWrites(); len(writes) != 0 {
		t.Fatalf("busy shell writes = %q", writes)
	}
	pty.busy = false
	pf.flushProcessEnvironment()
	writes := pty.snapshotWrites()
	if len(writes) != 1 || strings.Contains(string(writes[0]), "first") || strings.Contains(string(writes[0]), "second") {
		t.Fatalf("private coalesced write = %q", writes)
	}
	pf.processEnvironmentMu.Lock()
	inFlight := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if inFlight == nil || len(inFlight.changes) != 1 || inFlight.changes[0].Value != "second" {
		t.Fatalf("in-flight update = %#v", inFlight)
	}
	pf.processEnvironmentShellOutput([]byte("split\x1b]133;E;" + inFlight.token[:5]))
	pf.processEnvironmentShellOutput([]byte(inFlight.token[5:] + ";0\aafter"))
	pf.processEnvironmentMu.Lock()
	delivered := pf.processEnvironmentGeneration
	stillInFlight := pf.processEnvironmentInFlight != nil
	pf.processEnvironmentMu.Unlock()
	if delivered != 2 || stillInFlight {
		t.Fatalf("acknowledged generation = %d, in-flight %v", delivered, stillInFlight)
	}
	pf.closeProcessEnvironmentShell()
}

func runProcessEnvironmentUIInline(t *testing.T) {
	t.Helper()
	previous := processEnvironmentRunOnUI
	previousWithDecision := processEnvironmentRunOnUIWithRedrawDecision
	processEnvironmentRunOnUI = func(task func()) { task() }
	processEnvironmentRunOnUIWithRedrawDecision = func(task func() bool) { task() }
	t.Cleanup(func() {
		processEnvironmentRunOnUI = previous
		processEnvironmentRunOnUIWithRedrawDecision = previousWithDecision
	})
}

func TestPanelsFrameEnvironmentIdleMarkerOwnsNoRedrawWhenCaughtUp(t *testing.T) {
	previous := processEnvironmentRunOnUIWithRedrawDecision
	var queued func() bool
	processEnvironmentRunOnUIWithRedrawDecision = func(task func() bool) {
		if queued != nil {
			t.Fatal("idle marker queued more than one environment catch-up task")
		}
		queued = task
	}
	t.Cleanup(func() { processEnvironmentRunOnUIWithRedrawDecision = previous })

	pf := &PanelsFrame{
		pty:                          &processEnvironmentPTY{},
		termView:                     NewTerminalView(80, 24),
		processEnvironmentGeneration: globalProcessEnvironment.currentGeneration(),
	}
	defer pf.closeProcessEnvironmentShell()
	pf.processEnvironmentShellOutput([]byte("\x1b]133;D\x1b\\"))
	if queued == nil {
		t.Fatal("idle marker did not queue environment catch-up")
	}
	if queued() {
		t.Fatal("already-caught-up idle marker claimed a redraw")
	}
}

func TestPanelsFrameTerminalBusyTaskReportsOnlyVisibleChanges(t *testing.T) {
	pf := &PanelsFrame{}
	if pf.applyTerminalBusyChange(false, false) {
		t.Fatal("repeated idle marker reported a visible change")
	}
	if !pf.applyTerminalBusyChange(true, false) || !pf.executing {
		t.Fatal("busy marker did not report and apply executing state")
	}
	if pf.applyTerminalBusyChange(true, false) {
		t.Fatal("repeated busy marker reported a visible change")
	}
	if !pf.applyTerminalBusyChange(false, false) || pf.executing {
		t.Fatal("idle transition did not report and clear executing state")
	}
	if pf.applyTerminalBusyChange(false, false) {
		t.Fatal("second idle marker reported a visible change")
	}
}

func TestPanelsFrameEnvironmentGatesLocalInputAndLeavesRemoteUntouched(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	local := &processEnvironmentPTY{}
	remote := &processEnvironmentPTY{}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(1, []vfs.ProcessEnvironmentChange{{Name: "LOCAL_ONLY", Value: "private"}}, true)
	pf.processEnvironmentMu.Lock()
	inFlight := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if inFlight == nil {
		t.Fatal("local update was not sent")
	}
	if n, err := pf.writePTY(local, []byte("user-command\r")); err != nil || n != len("user-command\r") {
		t.Fatalf("gate local input = %d, %v", n, err)
	}
	if len(local.snapshotWrites()) != 1 {
		t.Fatal("user input reached local PTY before acknowledgement")
	}
	if _, err := pf.writePTY(remote, []byte("remote-input")); err != nil {
		t.Fatal(err)
	}
	if writes := remote.snapshotWrites(); len(writes) != 1 || string(writes[0]) != "remote-input" {
		t.Fatalf("remote passthrough writes = %q", writes)
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;E;" + inFlight.token + ";0\a"))
	writes := local.snapshotWrites()
	if len(writes) != 2 || string(writes[1]) != "user-command\r" {
		t.Fatalf("released local writes = %q", writes)
	}
	pf.closeProcessEnvironmentShell()
}

func TestPanelsFrameEnvironmentPrepareFailureGatesInputUntilRetry(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	originalPreparer := processEnvironmentPayloadPreparer
	prepareFailed := true
	processEnvironmentPayloadPreparer = func(changes []vfs.ProcessEnvironmentChange) (processEnvironmentShellPayload, error) {
		if prepareFailed {
			return processEnvironmentShellPayload{}, errors.New("injected private payload failure")
		}
		return originalPreparer(changes)
	}
	defer func() { processEnvironmentPayloadPreparer = originalPreparer }()

	local := &processEnvironmentPTY{}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(3, []vfs.ProcessEnvironmentChange{{Name: "PREPARE_RETRY", Value: "private"}}, true)
	if n, err := pf.writePTY(local, []byte("held-after-prepare-failure\r")); err != nil || n != len("held-after-prepare-failure\r") {
		t.Fatalf("gated write = %d, %v", n, err)
	}
	if writes := local.snapshotWrites(); len(writes) != 0 {
		t.Fatalf("prepare failure released input: %q", writes)
	}
	pf.processEnvironmentMu.Lock()
	deferred := string(pf.deferredProcessEnvironmentInput)
	pending := len(pf.pendingProcessEnvironment)
	failed := pf.processEnvironmentDeliveryFailed
	pf.processEnvironmentMu.Unlock()
	if deferred != "held-after-prepare-failure\r" || pending != 1 || !failed {
		t.Fatalf("prepare failure state = deferred %q, pending %d, failed %v", deferred, pending, failed)
	}

	prepareFailed = false
	pf.flushProcessEnvironment()
	pf.processEnvironmentMu.Lock()
	inFlight := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if inFlight == nil {
		t.Fatal("successful payload retry was not sent")
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;E;" + inFlight.token + ";0\a"))
	if writes := local.snapshotWrites(); len(writes) != 2 || string(writes[1]) != "held-after-prepare-failure\r" {
		t.Fatalf("prepare retry writes = %q", writes)
	}
	pf.closeProcessEnvironmentShell()
}

func TestPanelsFrameEnvironmentWriteFailureGatesInputUntilRetry(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	local := &processEnvironmentPTY{err: errors.New("injected PTY write failure")}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(6, []vfs.ProcessEnvironmentChange{{Name: "WRITE_RETRY", Value: "private"}}, true)
	if n, err := pf.writePTY(local, []byte("held-after-write-failure\r")); err != nil || n != len("held-after-write-failure\r") {
		t.Fatalf("gated write = %d, %v", n, err)
	}
	if writes := local.snapshotWrites(); len(writes) != 0 {
		t.Fatalf("write failure released input: %q", writes)
	}
	local.mu.Lock()
	local.err = nil
	local.mu.Unlock()
	pf.flushProcessEnvironment()
	pf.processEnvironmentMu.Lock()
	inFlight := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if inFlight == nil {
		t.Fatal("successful write retry was not sent")
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;E;" + inFlight.token + ";0\a"))
	if writes := local.snapshotWrites(); len(writes) != 2 || string(writes[1]) != "held-after-write-failure\r" {
		t.Fatalf("write retry writes = %q", writes)
	}
	pf.closeProcessEnvironmentShell()
}

func TestPanelsFrameEnvironmentSerializesParserReplies(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	local := &processEnvironmentPTY{}
	remote := &processEnvironmentPTY{}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(8, []vfs.ProcessEnvironmentChange{{Name: "SERIALIZE_REPLY", Value: "private"}}, true)
	pf.processEnvironmentMu.Lock()
	inFlight := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if inFlight == nil {
		t.Fatal("private update was not sent")
	}
	wrapped := &processEnvironmentSerializedPTY{owner: pf, backend: local}
	if n, err := wrapped.Write([]byte("terminal-reply")); err != nil || n != len("terminal-reply") {
		t.Fatalf("serialized reply = %d, %v", n, err)
	}
	if writes := local.snapshotWrites(); len(writes) != 1 {
		t.Fatalf("reply interleaved private wire: %q", writes)
	}
	if _, err := pf.writePTYAuxiliary(remote, []byte("remote-reply")); err != nil {
		t.Fatal(err)
	}
	if writes := remote.snapshotWrites(); len(writes) != 1 || string(writes[0]) != "remote-reply" {
		t.Fatalf("remote auxiliary write = %q", writes)
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;E;" + inFlight.token + ";0\a"))
	if writes := local.snapshotWrites(); len(writes) != 2 || string(writes[1]) != "terminal-reply" {
		t.Fatalf("released parser reply = %q", writes)
	}
	pf.closeProcessEnvironmentShell()
}

func TestPanelsFrameEnvironmentFailureKeepsInputUntilSuccessfulRetry(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	local := &processEnvironmentPTY{}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(4, []vfs.ProcessEnvironmentChange{{Name: "RETRY_ME", Value: "private"}}, true)
	pf.processEnvironmentMu.Lock()
	first := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if first == nil {
		t.Fatal("initial private update was not sent")
	}
	if _, err := pf.writePTY(local, []byte("held-command\r")); err != nil {
		t.Fatal(err)
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;E;" + first.token + ";1\a"))
	if writes := local.snapshotWrites(); len(writes) != 1 {
		t.Fatalf("failed update released input: %q", writes)
	}
	pf.processEnvironmentMu.Lock()
	deferred := string(pf.deferredProcessEnvironmentInput)
	pending := len(pf.pendingProcessEnvironment)
	pf.processEnvironmentMu.Unlock()
	if deferred != "held-command\r" || pending != 1 {
		t.Fatalf("failure state = deferred %q, pending %d", deferred, pending)
	}

	pf.flushProcessEnvironment()
	pf.processEnvironmentMu.Lock()
	second := pf.processEnvironmentInFlight
	pf.processEnvironmentMu.Unlock()
	if second == nil || second.token == first.token {
		t.Fatalf("retry in flight = %#v", second)
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;E;" + second.token + ";0\a"))
	writes := local.snapshotWrites()
	if len(writes) != 3 || string(writes[2]) != "held-command\r" {
		t.Fatalf("successful retry writes = %q", writes)
	}
	pf.processEnvironmentMu.Lock()
	generation := pf.processEnvironmentGeneration
	pf.processEnvironmentMu.Unlock()
	if generation != 4 {
		t.Fatalf("successful retry generation = %d", generation)
	}
	pf.closeProcessEnvironmentShell()
}

func TestPanelsFrameEnvironmentAcknowledgementTimeoutKeepsDeferredInput(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	oldTimeout := processEnvironmentAcknowledgementTimeout
	processEnvironmentAcknowledgementTimeout = 15 * time.Millisecond
	defer func() { processEnvironmentAcknowledgementTimeout = oldTimeout }()

	local := &processEnvironmentPTY{}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(5, []vfs.ProcessEnvironmentChange{{Name: "TIMEOUT_ME", Value: "private"}}, true)
	if _, err := pf.writePTY(local, []byte("held-after-timeout\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		pf.processEnvironmentMu.Lock()
		inFlight := pf.processEnvironmentInFlight != nil
		pf.processEnvironmentMu.Unlock()
		if !inFlight || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	pf.processEnvironmentMu.Lock()
	inFlight := pf.processEnvironmentInFlight
	deferred := string(pf.deferredProcessEnvironmentInput)
	pending := len(pf.pendingProcessEnvironment)
	generation := pf.processEnvironmentGeneration
	pf.processEnvironmentMu.Unlock()
	if inFlight != nil || deferred != "held-after-timeout\r" || pending != 1 || generation != 0 {
		t.Fatalf("timeout state = in-flight %#v, deferred %q, pending %d, generation %d", inFlight, deferred, pending, generation)
	}
	if writes := local.snapshotWrites(); len(writes) != 1 {
		t.Fatalf("timeout released held input: %q", writes)
	}
	pf.closeProcessEnvironmentShell()
}

func TestPanelsFrameExplicitBusyNeverExpiresFromBackendIdle(t *testing.T) {
	runProcessEnvironmentUIInline(t)
	local := &processEnvironmentPTY{busy: false}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.noteLocalShellBusy(true)
	// This exceeds the removed grace-period implementation. An interactive
	// shell builtin such as `read VAR` has no child process, so IsBusy remains
	// false no matter how long the explicit OSC busy state lasts.
	time.Sleep(1200 * time.Millisecond)
	pf.queueProcessEnvironment(7, []vfs.ProcessEnvironmentChange{{Name: "WAIT_FOR_PROMPT", Value: "private"}}, true)
	if writes := local.snapshotWrites(); len(writes) != 0 {
		t.Fatalf("explicitly busy shell received an update: %q", writes)
	}
	pf.processEnvironmentShellOutput([]byte("\x1b]133;D\x1b\\"))
	if writes := local.snapshotWrites(); len(writes) != 1 {
		t.Fatalf("prompt completion did not flush deferred update: %q", writes)
	}
	pf.closeProcessEnvironmentShell()
}

func TestProcessEnvironmentRuntimeDirectoryIsPrivate(t *testing.T) {
	if err := initializeProcessEnvironmentRuntime(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(processEnvironmentRuntimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() == false {
		t.Fatalf("runtime path is not a directory: %s", processEnvironmentRuntimeDir)
	}
	if runtime.GOOS != "windows" {
		if runtimePerm := info.Mode().Perm(); runtimePerm&0o077 != 0 {
			t.Fatalf("runtime permissions = %o, want no group/other access", runtimePerm)
		}
	}
}

func TestProcessEnvironmentRuntimeSessionsDoNotSweepEachOther(t *testing.T) {
	root := t.TempDir()
	first, err := createProcessEnvironmentRuntimeSession(root)
	if err != nil {
		t.Fatal(err)
	}
	ownedFile := filepath.Join(first, "in-flight")
	if err := os.WriteFile(ownedFile, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := createProcessEnvironmentRuntimeSession(root)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("runtime sessions collided: %s", first)
	}
	if _, err := os.Stat(ownedFile); err != nil {
		t.Fatalf("second session removed first session's file: %v", err)
	}
}

func TestProcessEnvironmentRuntimeSweepsOnlyConfirmedDeadSessions(t *testing.T) {
	root := t.TempDir()
	const deadPID = 2147483647
	if alive, known := processEnvironmentProcessState(deadPID); !known || alive {
		t.Skip("platform cannot positively identify the test PID as dead")
	}
	deadDir := filepath.Join(root, "2147483647-0123456789ABCDEF0123456789ABCDEF")
	liveDir := filepath.Join(root, strconv.Itoa(os.Getpid())+"-ABCDEF0123456789ABCDEF0123456789")
	for _, dir := range []string{deadDir, liveDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "private"), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sweepStaleProcessEnvironmentRuntimeSessions(root)
	if _, err := os.Stat(deadDir); !os.IsNotExist(err) {
		t.Fatalf("confirmed-dead session remained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(liveDir, "private")); err != nil {
		t.Fatalf("live session was swept: %v", err)
	}
}

func TestPanelsFrameCloseCleansItsOwnedRuntimeFiles(t *testing.T) {
	if err := initializeProcessEnvironmentRuntime(); err != nil {
		t.Fatal(err)
	}
	local := &processEnvironmentPTY{}
	pf := &PanelsFrame{pty: local, termView: NewTerminalView(80, 24)}
	pf.queueProcessEnvironment(9, []vfs.ProcessEnvironmentChange{{Name: "CLEAN_ON_CLOSE", Value: "private"}}, true)
	entries, err := os.ReadDir(processEnvironmentRuntimeDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("private files before close = %d, %v", len(entries), err)
	}
	pf.closeProcessEnvironmentShell()
	entries, err = os.ReadDir(processEnvironmentRuntimeDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("owned private files remained after close: %d, %v", len(entries), err)
	}
}

func TestProcessEnvironmentShutdownCleansExactSession(t *testing.T) {
	if err := initializeProcessEnvironmentRuntime(); err != nil {
		t.Fatal(err)
	}
	dir := processEnvironmentRuntimeDir
	if err := os.WriteFile(filepath.Join(dir, "orphaned-private-file"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	shutdownProcessEnvironmentRuntime()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("shutdown session remained: %v", err)
	}
	// sync.Once intentionally retains this process's unique session name; a
	// later workspace can safely recreate the same owned directory.
	if err := initializeProcessEnvironmentRuntime(); err != nil {
		t.Fatalf("recreate runtime after shutdown: %v", err)
	}
}

func TestWindowsProcessEnvironmentScriptGuardsMarkerFiles(t *testing.T) {
	changes := []vfs.ProcessEnvironmentChange{{Name: "GUARDED", Value: "private"}}
	script := string(windowsProcessEnvironmentScript(
		changes,
		[][]string{{`C:\runtime\chunk`}},
		`C:\runtime\success`,
		`C:\runtime\failure`,
		"F4_CP_GUARD",
		"F4_STATUS_GUARD",
		"F4_CHUNK_GUARD",
	))
	for _, path := range []string{`C:\runtime\chunk`, `C:\runtime\success`, `C:\runtime\failure`} {
		if !strings.Contains(script, `if not exist "`+path+`"`) {
			t.Fatalf("batch script does not guard %s", path)
		}
	}
	if !strings.Contains(script, `type "C:\runtime\success" || (type "C:\runtime\failure" & exit /b 1)`) {
		t.Fatal("successful marker read has no failure-marker fallback")
	}
	if strings.Contains(script, changes[0].Value) {
		t.Fatal("private value leaked into batch script")
	}
}

func TestWindowsProcessEnvironmentScriptRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe transport")
	}
	const variableName = "F4_ENV_ROUNDTRIP"
	middleLength := windowsCmdEnvironmentAssignmentLimit - len(variableName) - windowsEnvironmentUTF16Length("  €Ж  ")
	pattern := `!%&^"Az09`
	middle := strings.Repeat(pattern, (middleLength+len(pattern)-1)/len(pattern))[:middleLength]
	value := "  €Ж" + middle + "  "
	if got := len(variableName) + windowsEnvironmentUTF16Length(value); got != windowsCmdEnvironmentAssignmentLimit {
		t.Fatalf("boundary fixture length = %d", got)
	}
	for _, delayed := range []string{"OFF", "ON"} {
		t.Run("DelayedExpansion"+delayed, func(t *testing.T) {
			payload, err := prepareWindowsProcessEnvironmentShellPayload([]vfs.ProcessEnvironmentChange{{Name: variableName, Value: value}})
			if err != nil {
				t.Fatal(err)
			}
			defer payload.cleanup()
			cmd := exec.Command("cmd.exe", "/D", "/Q", "/V:"+delayed)
			pipeWire := strings.ReplaceAll(string(payload.wire), "\r", "\r\n")
			cmd.Stdin = strings.NewReader(pipeWire + "@chcp 65001 >nul\r\n@set " + variableName + "\r\n@exit\r\n")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("cmd transport failed: %v (output bytes %d)", err, len(output))
			}
			if !bytes.Contains(output, []byte(variableName+"="+value)) {
				t.Fatalf("cmd /V:%s did not preserve the %d-byte value (output bytes %d)", delayed, len(value), len(output))
			}
			if !bytes.Contains(output, []byte("\x1b]133;E;"+payload.token+";0\x07")) {
				t.Fatalf("cmd /V:%s did not emit success acknowledgement", delayed)
			}
		})
	}
}

func TestWindowsProcessEnvironmentControlCharacterRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe transport")
	}
	controls := []struct {
		name  string
		value string
	}{
		{name: "Tab", value: "before\tafter"},
		{name: "Escape", value: "before\x1bafter"},
		{name: "CtrlZ", value: "before\x1aafter"},
	}
	for _, delayed := range []string{"OFF", "ON"} {
		for _, control := range controls {
			t.Run(delayed+control.name, func(t *testing.T) {
				payload, err := prepareWindowsProcessEnvironmentShellPayload([]vfs.ProcessEnvironmentChange{{Name: "F4_ENV_CONTROL", Value: control.value}})
				if err != nil {
					t.Fatal(err)
				}
				defer payload.cleanup()
				cmd := exec.Command("cmd.exe", "/D", "/Q", "/V:"+delayed)
				pipeWire := strings.ReplaceAll(string(payload.wire), "\r", "\r\n")
				cmd.Stdin = strings.NewReader(pipeWire + "@chcp 65001 >nul\r\n@set F4_ENV_CONTROL\r\n@exit\r\n")
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("cmd transport failed: %v", err)
				}
				if !bytes.Contains(output, []byte("F4_ENV_CONTROL="+control.value)) {
					t.Fatalf("cmd /V:%s did not preserve %s (output bytes %d)", delayed, control.name, len(output))
				}
				if !bytes.Contains(output, []byte("\x1b]133;E;"+payload.token+";0\x07")) {
					t.Fatalf("cmd /V:%s did not acknowledge %s", delayed, control.name)
				}
			})
		}
	}
}
