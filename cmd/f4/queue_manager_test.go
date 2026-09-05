package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestQueueManager_Lifecycle(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	qm := GlobalQueueManager
	// Clear tasks
	qm.mu.Lock()
	qm.tasks = nil
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	executed := false
	completed := make(chan struct{})
	task := &QueueTask{
		Type:    "Test",
		Desc:    "Dummy",
		ResKeys: []string{"res1"},
		Run: func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error {
			executed = true
			return nil
		},
		OnComplete: func() { close(completed) },
	}

	qm.Enqueue(task)

	// Wait for worker to execute
	timeout := time.After(1 * time.Second)
	for {
		task.mu.Lock()
		state := task.State
		task.mu.Unlock()
		if state == "Done" {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Task did not complete")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitForQueueCompletion(t, completed)

	if !executed {
		t.Error("Task was not executed")
	}
}

func TestQueueManager_ConcurrencyLimit(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = nil
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	task1Started := make(chan bool)
	task1Finish := make(chan bool)

	task1 := &QueueTask{
		ResKeys: []string{"shared_res"},
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			task1Started <- true
			<-task1Finish
			return nil
		},
	}

	task2Started := false
	task2Completed := make(chan struct{})
	task2 := &QueueTask{
		ResKeys: []string{"shared_res"},
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			task2Started = true
			return nil
		},
		OnComplete: func() { close(task2Completed) },
	}

	qm.Enqueue(task1)
	qm.Enqueue(task2)

	<-task1Started

	time.Sleep(300 * time.Millisecond)

	task2.mu.Lock()
	state2 := task2.State
	task2.mu.Unlock()

	if state2 != "Queued" {
		t.Errorf("Task 2 should be Queued because resource is locked, but is %s", state2)
	}
	if task2Started {
		t.Error("Task 2 started concurrently on locked resource")
	}

	task1Finish <- true

	timeout := time.After(1 * time.Second)
	for {
		task2.mu.Lock()
		s2 := task2.State
		task2.mu.Unlock()
		if s2 == "Done" {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Task 2 did not complete after resource freed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitForQueueCompletion(t, task2Completed)

	if !task2Started {
		t.Error("Task 2 never started")
	}
}
func TestQueueManager_ConflictDetection(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()
	path := filepath.Join(tmp, "conflict.txt")
	if err := os.WriteFile(path, []byte("ver1"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmp)
	st, _ := v.Stat(context.Background(), path)

	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = nil
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	// 1. Ставим задачу в очередь с текущим состоянием файла
	completed := make(chan struct{})
	task := &QueueTask{
		Type:    "Copy",
		ResKeys: []string{getResourceKey(v)},
		Preconditions: []OpPrecondition{
			{Vfs: v, Path: path, MTime: st.MTime, Size: st.Size, IsDir: false},
		},
		Run:        func(ctx context.Context, r TaskReporter, a vtui.Frame) error { return nil },
		OnComplete: func() { close(completed) },
	}

	// Блокируем очередь, чтобы задача не запустилась мгновенно
	qm.mu.Lock()
	qm.activeKeys["local_disk"] = true // Предполагаем linux ресурс ключ
	if runtime.GOOS == "windows" {
		qm.activeKeys[filepath.VolumeName(tmp)] = true
	}
	qm.mu.Unlock()

	qm.Enqueue(task)

	// 2. Изменяем файл на диске
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(path, []byte("ver2-changed"), 0600); err != nil {
		t.Fatal(err)
	}

	// 3. Разблокируем очередь
	qm.mu.Lock()
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()
	qm.signalWorker()

	// Ждем обработки
	timeout := time.After(2 * time.Second)
	for {
		task.mu.Lock()
		state := task.State
		task.mu.Unlock()
		if state == "Error" || state == "Done" {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Task hung")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitForQueueCompletion(t, completed)

	task.mu.Lock()
	state, taskErr := task.State, task.ErrorMsg
	task.mu.Unlock()
	if state != "Error" || taskErr == nil {
		t.Errorf("Expected conflict error, got state %s", state)
	}
}

func TestQueueManager_ResourceIndependence(t *testing.T) {
	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = nil
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	start := make(chan bool, 2)
	completed := make(chan bool, 2)

	task1 := &QueueTask{
		ResKeys: []string{"disk_A"},
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			start <- true
			time.Sleep(200 * time.Millisecond)
			return nil
		},
		OnComplete: func() { completed <- true },
	}
	task2 := &QueueTask{
		ResKeys: []string{"disk_B"}, // Другой ресурс!
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			start <- true
			return nil
		},
		OnComplete: func() { completed <- true },
	}

	qm.Enqueue(task1)
	qm.Enqueue(task2)

	// Обе задачи должны запуститься почти одновременно, не дожидаясь друг друга
	count := 0
	timeout := time.After(1 * time.Second)
Loop:
	for count < 2 {
		select {
		case <-start:
			count++
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatalf("Only %d tasks started, expected 2 (independence check)", count)
			break Loop
		}
	}
	completionTimeout := time.NewTimer(time.Second)
	defer completionTimeout.Stop()
	for count = 0; count < 2; {
		select {
		case <-completed:
			count++
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-completionTimeout.C:
			t.Fatalf("Only %d tasks completed, expected 2", count)
		}
	}
}

func waitForQueueCompletion(t *testing.T, completed <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-completed:
			return
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timer.C:
			t.Fatal("queue completion callback did not run")
		}
	}
}

func TestQueueFrame_ClearDone(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()

	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = []*QueueTask{
		{ID: 1, State: "Done"},
		{ID: 2, State: "Running"},
		{ID: 3, State: "Error"},
	}
	qm.mu.Unlock()

	// Нажимаем "Clear Done"
	// В нашем коде btnClear это второй ребенок после таблицы и кнопки Cancel?
	// Нет, лучше найдем по тексту.
	var btnClear *vtui.Button
	for _, child := range qf.GetChildren() {
		if b, ok := child.(*vtui.Button); ok && strings.Contains(b.GetText(), "Clear") {
			btnClear = b
		}
	}

	btnClear.OnClick()

	qm.mu.Lock()
	count := len(qm.tasks)
	qm.mu.Unlock()

	if count != 1 {
		t.Errorf("Clear Done failed. Remaining tasks: %d, expected 1 (the Running one)", count)
	}
}
func TestQueueFrame_GetTitle(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()

	title := qf.GetTitle()
	if strings.TrimSpace(title) != "Operations Queue" {
		t.Errorf("QueueFrame title is missing or wrong: %q", title)
	}
}

func TestQueueFrameUsesDialogThemeColors(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	qf := NewQueueFrame()
	if qf.table.ColorTextIdx != vtui.ColDialogText ||
		qf.table.ColorSelectedTextIdx != vtui.ColDialogSelectedButton ||
		qf.table.ColorTitleIdx != vtui.ColDialogHighlightText ||
		qf.table.ColorBoxIdx != vtui.ColDialogBox {
		t.Fatalf("queue table does not use dialog palette: text=%d selected=%d title=%d box=%d",
			qf.table.ColorTextIdx, qf.table.ColorSelectedTextIdx, qf.table.ColorTitleIdx, qf.table.ColorBoxIdx)
	}

	def := vtui.SetRGBBoth(0, 0x112233, 0x445566)
	tests := []struct {
		state      string
		paletteIdx int
	}{
		{state: "Error", paletteIdx: vtui.ColWarnHighlightBoxTitle},
		{state: "Done", paletteIdx: vtui.ColDialogText},
		{state: "Running", paletteIdx: vtui.ColDialogHighlightText},
		{state: "Scanning", paletteIdx: vtui.ColDialogHighlightText},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := (queueRow{task: &QueueTask{State: tt.state}}).GetCellAttr(0, def)
			want := themedForeground(def, tt.paletteIdx)
			if got != want {
				t.Fatalf("%s attr = %#x, want themed attr %#x", tt.state, got, want)
			}
			if vtui.GetRGBBack(got) != vtui.GetRGBBack(def) {
				t.Fatalf("%s changed row background", tt.state)
			}
		})
	}
}

func TestQueueManager_BackgroundWorkspace(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	// Начальное состояние: только 1 экран (Desktop)
	if len(fm.Screens) != 1 {
		t.Fatalf("Expected 1 screen initially, got %d", len(fm.Screens))
	}

	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = nil
	qm.mu.Unlock()

	// Добавляем задачу
	done := make(chan struct{})
	qm.Enqueue(&QueueTask{
		Type:       "Test",
		Run:        func(ctx context.Context, r TaskReporter, a vtui.Frame) error { return nil },
		OnComplete: func() { close(done) },
	})

	// Обрабатываем задачи UI (EnsureQueueWorkspace вызывается через PostTask)
	timer := time.NewTimer(1 * time.Second)
	t.Cleanup(func() { timer.Stop() })
	for len(fm.Screens) < 2 {
		select {
		case task := <-fm.TaskChan:
			task()
		case <-timer.C:
			t.Fatal("Queue workspace was not created in background")
		}
	}

	// Background workspaces are placed immediately to the right of their
	// source workspace, matching the tab bar's insertion order.
	qScreen := fm.Screens[1]
	found := false
	for _, f := range qScreen.Frames {
		if _, ok := f.(*QueueFrame); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("QueueFrame not found at index 0")
	}

	// The original workspace remains active and keeps its index.
	if fm.ActiveIdx != 0 {
		t.Errorf("Focus pointer tracking failed. ActiveIdx: %d, expected 0", fm.ActiveIdx)
	}

	for {
		select {
		case <-done:
			return
		case task := <-fm.TaskChan:
			task()
		case <-timer.C:
			t.Fatal("Background queue task did not complete")
		}
	}
}

func TestQueueFrame_InputLock(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()

	qm := GlobalQueueManager
	task := &QueueTask{ID: 1, State: "Running"}
	qm.mu.Lock()
	// Имитируем активную задачу
	qm.tasks = []*QueueTask{task}
	qm.mu.Unlock()

	// Попытка нажать Esc
	ev := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	handled := qf.ProcessKey(ev)

	if !handled {
		t.Error("QueueFrame should swallow ESC when tasks are active")
	}

	// Попытка нажать F10
	evF10 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F10}
	handledF10 := qf.ProcessKey(evF10)
	if !handledF10 {
		t.Error("QueueFrame should swallow F10 when tasks are active")
	}

	// Ctrl+W is a global workspace-close fallback and must be swallowed too.
	evCtrlW := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_W,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	if !qf.ProcessKey(evCtrlW) {
		t.Error("QueueFrame should swallow Ctrl+W when tasks are active")
	}

	// Завершаем задачи
	task.mu.Lock()
	task.State = "Done"
	task.mu.Unlock()

	// Теперь Esc не должен поглощаться самим фреймом (вернет false или обработает BaseWindow)
	if qf.ProcessKey(ev) {
		// BaseWindow вернет true и закроет окно. Это корректно.
		if !qf.IsDone() {
			t.Error("QueueFrame did not close on ESC after tasks finished")
		}
	}
}

type mockVFSWithParent struct {
	vfs.VFS
	parent vfs.VFS
}

func (m *mockVFSWithParent) ParentVFS() vfs.VFS {
	return m.parent
}

func (m *mockVFSWithParent) GetPath() string {
	return "mock_archive_inner_path"
}

func TestQueueManager_ArchiveResourceKey(t *testing.T) {
	// Create parent OSVFS pointing to a local temp directory
	parent := vfs.NewOSVFS(t.TempDir())
	expectedKey := getResourceKey(parent)

	// Create mock nested VFS mimicking an active ArchiveVFS
	child := &mockVFSWithParent{parent: parent}

	// Verify that the child's resource key matches the parent's (physical disk locking)
	key := getResourceKey(child)
	if key != expectedKey {
		t.Errorf("Expected resource key %q, got %q (failed to inherit ParentVFS disk lock)", expectedKey, key)
	}
}

func TestQueueManagerCancelRunningWaitsForRunToUnwind(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	allowReturn := make(chan struct{})
	finished := make(chan struct{})
	task := &QueueTask{
		ID:      1,
		State:   "Starting",
		ResKeys: []string{"resource"},
		ctx:     ctx,
		cancel:  cancel,
		Run: func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error {
			close(started)
			<-ctx.Done()
			<-allowReturn
			return fmt.Errorf("run cleanup: %w", ctx.Err())
		},
	}
	qm := &OpQueueManager{
		tasks:      []*QueueTask{task},
		activeKeys: map[string]bool{"resource": true},
	}

	go func() {
		qm.executeTask(task)
		close(finished)
	}()
	<-started

	if !qm.Cancel(task.ID) {
		t.Fatal("Cancel did not find the running task")
	}
	task.mu.Lock()
	state := task.State
	task.mu.Unlock()
	if state != "Cancelling" {
		t.Fatalf("state immediately after Cancel = %q, want Cancelling", state)
	}
	if got := qm.ActiveTasksCount(); got != 1 {
		t.Fatalf("ActiveTasksCount while Run is unwinding = %d, want 1", got)
	}

	// Late worker progress must not resurrect a task after cancellation starts.
	task.UpdateScan("late scan", 1, 0)
	task.UpdateTransfer("apply", "late file", 50, "1/2", 50, "")
	task.mu.Lock()
	state = task.State
	task.mu.Unlock()
	if state != "Cancelling" {
		t.Fatalf("late progress changed state to %q, want Cancelling", state)
	}

	close(allowReturn)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("executeTask did not finish after Run unwound")
	}

	task.mu.Lock()
	state = task.State
	err := task.ErrorMsg
	task.mu.Unlock()
	if state != "Cancelled" {
		t.Fatalf("final state = %q, want Cancelled", state)
	}
	if err != nil {
		t.Fatalf("cancelled task retained error %v", err)
	}
	if got := qm.ActiveTasksCount(); got != 0 {
		t.Fatalf("ActiveTasksCount after unwind = %d, want 0", got)
	}
	qm.mu.Lock()
	resourceActive := qm.activeKeys["resource"]
	qm.mu.Unlock()
	if resourceActive {
		t.Fatal("resource key remained active after cancellation completed")
	}
}

func TestQueueManagerCancelQueuedAndCancelAll(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	queuedCtx, queuedCancel := context.WithCancel(context.Background())
	runningCtx, runningCancel := context.WithCancel(context.Background())
	cancellingCtx, cancellingCancel := context.WithCancel(context.Background())
	queuedComplete := make(chan struct{})
	queued := &QueueTask{
		ID: 1, State: "Queued", ctx: queuedCtx, cancel: queuedCancel,
		OnComplete: func() { close(queuedComplete) },
	}
	running := &QueueTask{ID: 2, State: "Running", ctx: runningCtx, cancel: runningCancel}
	cancelling := &QueueTask{ID: 3, State: "Cancelling", ctx: cancellingCtx, cancel: cancellingCancel}
	done := &QueueTask{ID: 4, State: "Done"}
	qm := &OpQueueManager{
		tasks:      []*QueueTask{queued, running, cancelling, done},
		activeKeys: make(map[string]bool),
	}

	qm.CancelAll()
	deadline := time.Now().Add(time.Second)
	for {
		queued.mu.Lock()
		queuedDone := queued.State == "Cancelled"
		queued.mu.Unlock()
		if queuedDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued cancellation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
pumpQueuedCompletion:
	for {
		select {
		case <-queuedComplete:
			break pumpQueuedCompletion
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(time.Second):
			t.Fatal("queued cancellation completion callback did not run")
		}
	}

	for _, tt := range []struct {
		task      *QueueTask
		wantState string
		wantErr   error
	}{
		{queued, "Cancelled", context.Canceled},
		{running, "Cancelling", context.Canceled},
		{cancelling, "Cancelling", context.Canceled},
		{done, "Done", nil},
	} {
		tt.task.mu.Lock()
		state := tt.task.State
		tt.task.mu.Unlock()
		if state != tt.wantState {
			t.Errorf("task %d state = %q, want %q", tt.task.ID, state, tt.wantState)
		}
		if !errorsAreSame(tt.task.ctx, tt.wantErr) {
			t.Errorf("task %d context error = %v, want %v", tt.task.ID, contextError(tt.task.ctx), tt.wantErr)
		}
	}
}

func TestQueueManagerCancelQueuedCompletesExactlyOnce(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ctx, cancel := context.WithCancel(context.Background())
	completeCalls := 0
	task := &QueueTask{
		ID:     1,
		State:  "Queued",
		ctx:    ctx,
		cancel: cancel,
		OnComplete: func() {
			completeCalls++
		},
	}
	qm := &OpQueueManager{
		tasks:      []*QueueTask{task},
		activeKeys: make(map[string]bool),
	}

	if !qm.Cancel(task.ID) {
		t.Fatal("Cancel did not find the queued task")
	}
	if qm.Cancel(task.ID) {
		t.Fatal("a second Cancel treated the terminal task as cancellable")
	}
	// Simulate a competing/future finalization path. The task completion guard
	// must still allow only the callback already posted by Cancel.
	qm.postTaskCompletion(task)

	quiet := time.NewTimer(100 * time.Millisecond)
	defer quiet.Stop()
	for {
		select {
		case uiTask := <-vtui.FrameManager.TaskChan:
			uiTask()
		case <-quiet.C:
			if completeCalls != 1 {
				t.Fatalf("OnComplete called %d times, want exactly once", completeCalls)
			}
			if ctx.Err() != context.Canceled {
				t.Fatalf("queued task context = %v, want context.Canceled", ctx.Err())
			}
			return
		}
	}
}

func TestQueueManagerCancelQueuedStaysActiveUntilFinalizeReturns(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ctx, cancel := context.WithCancel(context.Background())
	finalizeStarted := make(chan struct{})
	allowFinalize := make(chan struct{})
	task := &QueueTask{
		ID: 1, State: "Queued", ctx: ctx, cancel: cancel,
		Finalize: func() {
			close(finalizeStarted)
			<-allowFinalize
		},
	}
	qm := &OpQueueManager{tasks: []*QueueTask{task}, activeKeys: make(map[string]bool)}
	if !qm.Cancel(task.ID) {
		t.Fatal("Cancel did not report the queued task")
	}
	<-finalizeStarted

	if got := qm.ActiveTasksCount(); got != 1 {
		t.Fatalf("ActiveTasksCount during Finalize = %d, want 1", got)
	}
	task.mu.Lock()
	state := task.State
	task.mu.Unlock()
	if state != "Cancelling" {
		t.Fatalf("state during Finalize = %q, want Cancelling", state)
	}

	close(allowFinalize)
	deadline := time.Now().Add(time.Second)
	for {
		task.mu.Lock()
		state = task.State
		task.mu.Unlock()
		if state == "Cancelled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued task did not finish cancellation")
		}
		time.Sleep(time.Millisecond)
	}
	if state != "Cancelled" || qm.ActiveTasksCount() != 0 {
		t.Fatalf("after Finalize state=%q active=%d, want Cancelled/0", state, qm.ActiveTasksCount())
	}
}

func TestQueueManagerWrappedCancellationIsCancelled(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := &QueueTask{
		ID:     1,
		State:  "Starting",
		ctx:    ctx,
		cancel: cancel,
		Run: func(context.Context, TaskReporter, vtui.Frame) error {
			return fmt.Errorf("wrapped: %w", context.Canceled)
		},
	}
	qm := &OpQueueManager{tasks: []*QueueTask{task}, activeKeys: make(map[string]bool)}

	qm.executeTask(task)

	task.mu.Lock()
	state := task.State
	task.mu.Unlock()
	if state != "Cancelled" {
		t.Fatalf("wrapped context cancellation produced state %q, want Cancelled", state)
	}
}

func TestQueueFrameEnterOpensTaskDetails(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()
	called := false
	var gotAnchor vtui.Frame
	task := &QueueTask{
		ID:    1,
		State: "Running",
		OpenDetails: func(anchor vtui.Frame) {
			called = true
			gotAnchor = anchor
		},
	}
	qf.UpdateTasks([]*QueueTask{task})
	qf.table.SelectPos = 0

	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}
	if !qf.ProcessKey(e) {
		t.Fatal("Enter on a task with details was not handled")
	}
	if !called {
		t.Fatal("Enter did not invoke QueueTask.OpenDetails")
	}
	if gotAnchor != qf {
		t.Fatalf("OpenDetails anchor = %T, want QueueFrame", gotAnchor)
	}
}

func TestCancelOperationsForShutdownCancelsQueueAndBackgroundJobs(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	originalQueue := GlobalQueueManager
	originalJobs := GlobalBackgroundJobs
	defer func() {
		GlobalQueueManager = originalQueue
		GlobalBackgroundJobs = originalJobs
	}()

	ctx, cancel := context.WithCancel(context.Background())
	queued := &QueueTask{ID: 1, State: "Queued", ctx: ctx, cancel: cancel}
	GlobalQueueManager = &OpQueueManager{
		tasks:      []*QueueTask{queued},
		activeKeys: make(map[string]bool),
	}
	GlobalBackgroundJobs = NewBackgroundJobRegistry()
	backgroundCancelled := false
	var background *BackgroundJob
	background = GlobalBackgroundJobs.Start("test job", func() {
		backgroundCancelled = true
		background.Finish()
	})

	cancelOperationsForShutdown()

	queued.mu.Lock()
	state := queued.State
	queued.mu.Unlock()
	if state != "Cancelled" || ctx.Err() != context.Canceled {
		t.Fatalf("queued task after shutdown cancellation: state=%q context=%v", state, ctx.Err())
	}
	if !backgroundCancelled {
		t.Fatal("background-job registry was not cancelled during shutdown")
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func errorsAreSame(ctx context.Context, want error) bool {
	return contextError(ctx) == want
}

// TestQueueCancelFinalizesOnTheFrameManagerItStartedWith is the regression for
// a race the CI race job reported against TestSimpleInline_CommandExecution.
//
// The blamed test only swapped the process-wide frame manager in. The write
// raced a read from a goroutine an earlier test had left running:
// TestCancelOperationsForShutdownCancelsQueueAndBackgroundJobs cancels a queued
// task, Cancel finishes such a task off-thread, and that finalizer read
// vtui.FrameManager to post the completion callback. Shutdown deliberately does
// not wait for it, so the goroutine outlives the test that started it.
//
// Cancel now reads the frame manager where it starts the finalizer, so the
// callback lands on the one the task was cancelled against no matter what the
// global holds by then.
func TestQueueCancelFinalizesOnTheFrameManagerItStartedWith(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	starting := vtui.FrameManager

	ctx, cancel := context.WithCancel(context.Background())
	completed := false
	task := &QueueTask{
		ID:         1,
		State:      "Queued",
		ctx:        ctx,
		cancel:     cancel,
		OnComplete: func() { completed = true },
	}
	qm := &OpQueueManager{
		tasks:      []*QueueTask{task},
		activeKeys: make(map[string]bool),
	}

	if !qm.Cancel(task.ID) {
		t.Fatal("Cancel did not find the queued task")
	}

	// What the next test does while the finalizer is still in flight.
	replacement := vtui.NewFrameManager()
	vtui.FrameManager = replacement
	t.Cleanup(func() {
		closeFrameManagerFrames(replacement)
		replacement.Shutdown()
		vtui.FrameManager = starting
	})

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !completed {
		select {
		case uiTask := <-starting.TaskChan:
			uiTask()
		case <-deadline.C:
			t.Fatal("the cancelled task never completed on the frame manager it started with")
		}
	}

	select {
	case <-replacement.TaskChan:
		t.Fatal("the finalizer posted to the frame manager that replaced it")
	default:
	}
}
