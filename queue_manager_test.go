package main

import (
	"context"
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
	task := &QueueTask{
		Type:    "Test",
		Desc:    "Dummy",
		ResKeys: []string{"res1"},
		Run: func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error {
			executed = true
			return nil
		},
	}

	qm.Enqueue(task)

	// Wait for worker to execute
	timeout := time.After(1 * time.Second)
	for {
		qm.mu.Lock()
		if len(qm.tasks) == 0 {
			qm.mu.Unlock()
			continue
		}
		state := qm.tasks[0].State
		qm.mu.Unlock()
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
	task2 := &QueueTask{
		ResKeys: []string{"shared_res"},
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			task2Started = true
			return nil
		},
	}

	qm.Enqueue(task1)
	qm.Enqueue(task2)

	<-task1Started

	time.Sleep(300 * time.Millisecond)

	qm.mu.Lock()
	state2 := task2.State
	qm.mu.Unlock()

	if state2 != "Queued" {
		t.Errorf("Task 2 should be Queued because resource is locked, but is %s", state2)
	}
	if task2Started {
		t.Error("Task 2 started concurrently on locked resource")
	}

	task1Finish <- true

	timeout := time.After(1 * time.Second)
	for {
		qm.mu.Lock()
		s2 := task2.State
		qm.mu.Unlock()
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

	if !task2Started {
		t.Error("Task 2 never started")
	}
}
func TestQueueManager_ConflictDetection(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := t.TempDir()
	path := filepath.Join(tmp, "conflict.txt")
	os.WriteFile(path, []byte("ver1"), 0644)

	v := vfs.NewOSVFS(tmp)
	st, _ := v.Stat(context.Background(), path)

	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = nil
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	// 1. Ставим задачу в очередь с текущим состоянием файла
	task := &QueueTask{
		Type: "Copy",
		Preconditions: []OpPrecondition{
			{Vfs: v, Path: path, MTime: st.MTime, Size: st.Size, IsDir: false},
		},
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error { return nil },
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
	os.WriteFile(path, []byte("ver2-changed"), 0644)

	// 3. Разблокируем очередь
	qm.mu.Lock()
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	// Ждем обработки
	timeout := time.After(2 * time.Second)
	for {
		qm.mu.Lock()
		state := task.State
		qm.mu.Unlock()
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

	if task.State != "Error" || task.ErrorMsg == nil {
		t.Errorf("Expected conflict error, got state %s", task.State)
	}
}

func TestQueueManager_ResourceIndependence(t *testing.T) {
	qm := GlobalQueueManager
	qm.mu.Lock()
	qm.tasks = nil
	qm.activeKeys = make(map[string]bool)
	qm.mu.Unlock()

	start := make(chan bool, 2)

	task1 := &QueueTask{
		ResKeys: []string{"disk_A"},
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			start <- true
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}
	task2 := &QueueTask{
		ResKeys: []string{"disk_B"}, // Другой ресурс!
		Run: func(ctx context.Context, r TaskReporter, a vtui.Frame) error {
			start <- true
			return nil
		},
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
	qm.Enqueue(&QueueTask{
		Type: "Test",
		Run:  func(ctx context.Context, r TaskReporter, a vtui.Frame) error { return nil },
	})

	// Обрабатываем задачи UI (EnsureQueueWorkspace вызывается через PostTask)
	timeout := time.After(1 * time.Second)
	for len(fm.Screens) < 2 {
		select {
		case task := <-fm.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Queue workspace was not created in background")
		}
	}

	// Проверяем, что новый экран (вставленный в начало, индекс 0) содержит QueueFrame
	qScreen := fm.Screens[0]
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

	// Проверяем, что фокус остался на исходном экране.
	// Так как мы вставили в начало, индекс активного экрана должен был сдвинуться на 1.
	if fm.ActiveIdx != 1 {
		t.Errorf("Focus pointer tracking failed. ActiveIdx: %d, expected 1", fm.ActiveIdx)
	}
}

func TestQueueFrame_InputLock(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()

	qm := GlobalQueueManager
	qm.mu.Lock()
	// Имитируем активную задачу
	qm.tasks = []*QueueTask{{ID: 1, State: "Running"}}
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

	// Завершаем задачи
	qm.mu.Lock()
	qm.tasks[0].State = "Done"
	qm.mu.Unlock()

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
