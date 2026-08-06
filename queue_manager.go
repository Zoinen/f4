package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type OpPrecondition struct {
	Vfs   vfs.VFS
	Path  string
	MTime time.Time
	Size  int64
	IsDir bool
}

type TaskReporter interface {
	UpdateScan(currentPath string, files, dirs int64)
	UpdateTransfer(action string, filename string, currentPct int, totalText string, totalPct int, speedText string)
	IsCancelled() bool
}

type DialogReporter struct {
	dlg *FileOpProgressDialog
}

func (r *DialogReporter) UpdateScan(currentPath string, files, dirs int64) {
	vtui.FrameManager.PostTask(func() {
		r.dlg.UpdateScan(currentPath, files, dirs)
		vtui.FrameManager.Redraw()
	})
}

func (r *DialogReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	vtui.FrameManager.PostTask(func() {
		r.dlg.UpdateTransfer(action, filename, currentPct, totalText, totalPct, speedText)
		vtui.FrameManager.Redraw()
	})
}

func (r *DialogReporter) IsCancelled() bool {
	return r.dlg.IsDone()
}

type DummyReporter struct{}

func (r *DummyReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (r *DummyReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
}
func (r *DummyReporter) IsCancelled() bool { return false }

type QueueTask struct {
	mu          sync.Mutex
	ID          int
	Type        string
	Desc        string
	State       string // Queued, Scanning, Running, Done, Error, Cancelled
	Progress    int
	TotalText   string
	Speed       string
	CurrentFile string
	ErrorMsg    error

	Preconditions []OpPrecondition
	ResKeys       []string

	Run        func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error
	OnComplete func()

	ctx    context.Context
	cancel context.CancelFunc
}

func (t *QueueTask) UpdateScan(currentPath string, files, dirs int64) {
	t.mu.Lock()
	if t.State == "Done" || t.State == "Error" || t.State == "Cancelled" {
		vtui.DebugLog("QUEUE_DEBUG: UpdateScan ignored for Task %d (State: %s)", t.ID, t.State)
		t.mu.Unlock()
		return
	}
	vtui.DebugLog("QUEUE_DEBUG: Task %d Scanning -> %s", t.ID, currentPath)
	t.State = "Scanning"
	t.CurrentFile = currentPath
	t.TotalText = fmt.Sprintf("Files: %d, Dirs: %d", files, dirs)
	t.mu.Unlock()

	GlobalQueueManager.RequestRefresh()
}
func (t *QueueTask) UpdateTransfer(action string, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	t.mu.Lock()
	if t.State == "Done" || t.State == "Error" || t.State == "Cancelled" {
		t.mu.Unlock()
		return
	}
	t.State = "Running"
	t.CurrentFile = filename
	t.Progress = totalPct
	t.TotalText = totalText

	// Extract clean speed from composite timeSpeedText string if applicable
	displaySpeed := speedText
	if len(speedText) >= 37 {
		displaySpeed = strings.TrimSpace(speedText[37:])
	}
	t.Speed = displaySpeed

	t.mu.Unlock()

	GlobalQueueManager.RequestRefresh()
}

func (t *QueueTask) IsCancelled() bool {
	if t.ctx != nil {
		return t.ctx.Err() != nil
	}
	return false
}

type OpQueueManager struct {
	mu             sync.Mutex
	tasks          []*QueueTask
	nextID         int
	activeKeys     map[string]bool
	frame          *QueueFrame
	refreshPending bool
}

var GlobalQueueManager *OpQueueManager

func init() {
	GlobalQueueManager = &OpQueueManager{
		activeKeys: make(map[string]bool),
	}
	go GlobalQueueManager.workerLoop()
}
func (qm *OpQueueManager) RequestRefresh() {
	qm.mu.Lock()
	if qm.refreshPending {
		qm.mu.Unlock()
		return
	}
	qm.refreshPending = true
	qm.mu.Unlock()

	vtui.FrameManager.PostTask(func() {
		qm.mu.Lock()
		qm.refreshPending = false
		qm.mu.Unlock()
		qm.RefreshUI()
	})
}

func getResourceKey(v vfs.VFS) string {
	if v == nil {
		return ""
	}
	if _, ok := v.(*vfs.OSVFS); ok {
		if runtime.GOOS == "windows" {
			return filepath.VolumeName(v.GetPath())
		}
		return "local_disk"
	}
	if parent := v.ParentVFS(); parent != nil {
		return getResourceKey(parent)
	}
	return fmt.Sprintf("%p", v)
}

func (qm *OpQueueManager) Enqueue(task *QueueTask) {
	qm.mu.Lock()
	qm.nextID++
	task.ID = qm.nextID
	task.State = "Queued"
	task.ctx, task.cancel = context.WithCancel(context.Background())
	qm.tasks = append(qm.tasks, task)
	qm.mu.Unlock()

	vtui.FrameManager.PostTask(func() {
		qm.EnsureQueueWorkspace()
		qm.RefreshUI()
	})

	go func(id int) {
		time.Sleep(500 * time.Millisecond)
		qm.mu.Lock()
		defer qm.mu.Unlock()
		for _, t := range qm.tasks {
			if t.ID == id && (t.State == "Queued" || t.State == "Starting" || t.State == "Scanning" || t.State == "Running") {
				vtui.ShowToast("Background operation started. Press Ctrl+Tab for Queue.", 4*time.Second)
				break
			}
		}
	}(task.ID)
}

func (qm *OpQueueManager) EnsureQueueWorkspace() {
	if vtui.FrameManager == nil || vtui.FrameManager.Screens == nil {
		return
	}
	for _, s := range vtui.FrameManager.Screens {
		for _, f := range s.Frames {
			if qf, ok := f.(*QueueFrame); ok {
				qm.frame = qf
				return
			}
		}
	}

	qm.frame = NewQueueFrame()
	vtui.FrameManager.AddScreenBackground(qm.frame)
}

func (qm *OpQueueManager) ActiveTasksCount() int {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	count := 0
	for _, t := range qm.tasks {
		t.mu.Lock()
		isActive := t.State == "Queued" || t.State == "Starting" || t.State == "Scanning" || t.State == "Running"
		t.mu.Unlock()
		if isActive {
			count++
		}
	}
	return count
}

func (qm *OpQueueManager) RefreshUI() {
	if qm.frame != nil {
		qm.frame.UpdateTasks(qm.tasks)
	}
}

func (qm *OpQueueManager) workerLoop() {
	for {
		time.Sleep(200 * time.Millisecond)

		qm.mu.Lock()
		var toRun *QueueTask
		for _, t := range qm.tasks {
			t.mu.Lock()
			isQueued := t.State == "Queued"
			t.mu.Unlock()

			if isQueued {
				canRun := true
				for _, rk := range t.ResKeys {
					if qm.activeKeys[rk] {
						canRun = false
						break
					}
				}
				if canRun {
					toRun = t
					for _, rk := range t.ResKeys {
						qm.activeKeys[rk] = true
					}
					t.mu.Lock()
					t.State = "Starting"
					t.mu.Unlock()
					break
				}
			}
		}
		qm.mu.Unlock()

		if toRun != nil {
			go qm.executeTask(toRun)
		}
	}
}

func (qm *OpQueueManager) executeTask(t *QueueTask) {
	vtui.DebugLog("QUEUE_DEBUG: Executing Task %d (%s)", t.ID, t.Type)
	if t.Run == nil {
		t.State = "Error"
		t.ErrorMsg = fmt.Errorf("internal error: task run function is nil")
		qm.mu.Lock()
		for _, rk := range t.ResKeys {
			qm.activeKeys[rk] = false
		}
		qm.mu.Unlock()
		return
	}
	conflict := false
	for _, pc := range t.Preconditions {
		st, err := pc.Vfs.Stat(context.Background(), pc.Path)
		if err != nil {
			conflict = true
			t.ErrorMsg = fmt.Errorf("conflict: missing %s", pc.Path)
			break
		}
		if st.MTime != pc.MTime || st.Size != pc.Size || st.IsDir != pc.IsDir {
			conflict = true
			t.ErrorMsg = fmt.Errorf("conflict: modified %s", pc.Path)
			break
		}
	}

	var err error
	if conflict {
		t.mu.Lock()
		t.State = "Error"
		t.mu.Unlock()
	} else {
		err = t.Run(t.ctx, t, qm.frame)
		t.mu.Lock()
		if err != nil {
			if err == context.Canceled {
				t.State = "Cancelled"
			} else {
				t.State = "Error"
				t.ErrorMsg = err
			}
		} else {
			t.State = "Done"
			t.Progress = 100
		}
		t.mu.Unlock()
	}

	qm.mu.Lock()
	for _, rk := range t.ResKeys {
		qm.activeKeys[rk] = false
	}
	qm.mu.Unlock()

	vtui.DebugLog("QUEUE_DEBUG: Task %d finalized with state %s. Posting OnComplete.", t.ID, t.State)

	vtui.FrameManager.PostTask(func() {
		if t.OnComplete != nil {
			t.OnComplete()
		}
		qm.RefreshUI()
	})
}

type QueueFrame struct {
	vtui.BaseWindow
	table *vtui.Table
	tasks []*QueueTask
}

type queueRow struct {
	task *QueueTask
}

func (r queueRow) GetCellText(col int) string {
	t := r.task
	t.mu.Lock()
	defer t.mu.Unlock()
	switch col {
	case 0:
		return fmt.Sprintf("%d", t.ID)
	case 1:
		return t.State
	case 2:
		return t.Type
	case 3:
		if t.State == "Running" || t.State == "Scanning" {
			return t.CurrentFile
		}
		return t.Desc
	case 4:
		pct := t.Progress
		bars := (pct * 10) / 100
		s := ""
		for i := 0; i < 10; i++ {
			if i < bars {
				s += "█"
			} else {
				s += "░"
			}
		}
		return fmt.Sprintf("%3d%% %s", pct, s)
	case 5:
		return t.Speed
	}
	return ""
}
func (r queueRow) GetCellAttr(col int, def uint64) uint64 {
	t := r.task
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State == "Error" {
		return themedForeground(def, vtui.ColWarnHighlightBoxTitle)
	}
	if t.State == "Done" {
		return themedForeground(def, vtui.ColDialogText)
	}
	if t.State == "Running" || t.State == "Scanning" {
		return themedForeground(def, vtui.ColDialogHighlightText)
	}
	if t.State == "Cancelled" {
		return vtui.DimColor(def)
	}
	return def
}

func NewQueueFrame() *QueueFrame {
	scrW, scrH := 80, 25
	if vtui.FrameManager != nil && vtui.FrameManager.GetScreenSize() > 0 {
		scrW = vtui.FrameManager.GetScreenSize()
		scrH = vtui.FrameManager.GetScreenHeight()
	}

	qf := &QueueFrame{
		BaseWindow: *vtui.NewBaseWindow(0, 0, scrW-1, scrH-1, " Operations Queue "),
	}
	qf.ShowClose = true
	qf.ShowZoom = true
	qf.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY)

	descW := scrW - 4 - 4 - 10 - 8 - 18 - 12 - 5 // Dynamic width calculation
	if descW < 10 {
		descW = 10
	}

	cols := []vtui.TableColumn{
		{Title: "ID", Width: 4},
		{Title: "State", Width: 10},
		{Title: "Type", Width: 8},
		{Title: "Description / Current File", Width: descW},
		{Title: "Progress", Width: 18},
		{Title: "Speed", Width: 12},
	}
	qf.table = vtui.NewTable(0, 0, scrW-4, scrH-6, cols)
	useDialogTableColors(qf.table)
	qf.table.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY)
	qf.table.ShowScrollBar = true

	btnCancel := vtui.NewButton(0, 0, "C&ancel Task")
	btnClear := vtui.NewButton(0, 0, "Clear &Done")

	qf.AddItem(qf.table)
	qf.AddItem(btnCancel)
	qf.AddItem(btnClear)

	vbox := vtui.NewVBoxLayout(qf.X1+2, qf.Y1+2, scrW-4, scrH-4)
	vbox.Add(qf.table, vtui.Margins{Bottom: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, 74, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClear, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() {
		idx := qf.table.SelectPos
		if idx >= 0 && idx < len(qf.tasks) {
			t := qf.tasks[idx]
			t.mu.Lock()
			state := t.State
			t.mu.Unlock()
			if state == "Queued" || state == "Running" || state == "Scanning" || state == "Starting" {
				vtui.ShowMessageOn(qf, " Confirm ", "Cancel task ID "+fmt.Sprintf("%d", t.ID)+"?", []string{"&Yes", "&No"}).OnResult = func(c int) {
					if c == 0 {
						if t.cancel != nil {
							t.cancel()
						}
						t.mu.Lock()
						t.State = "Cancelled"
						t.mu.Unlock()
						qf.UpdateTasks(qf.tasks)
					}
				}
			}
		}
	}

	btnClear.OnClick = func() {
		GlobalQueueManager.mu.Lock()
		var active []*QueueTask
		for _, t := range GlobalQueueManager.tasks {
			t.mu.Lock()
			isDone := t.State == "Done" || t.State == "Cancelled" || t.State == "Error"
			t.mu.Unlock()
			if !isDone {
				active = append(active, t)
			}
		}
		GlobalQueueManager.tasks = active
		GlobalQueueManager.mu.Unlock()
		GlobalQueueManager.RefreshUI()
	}

	return qf
}

func (qf *QueueFrame) UpdateTasks(tasks []*QueueTask) {
	qf.tasks = append([]*QueueTask(nil), tasks...)
	rows := make([]vtui.TableRow, len(qf.tasks))
	for i, t := range qf.tasks {
		rows[i] = queueRow{task: t}
	}
	qf.table.SetRows(rows)
	vtui.FrameManager.Redraw()
}

func (qf *QueueFrame) GetType() vtui.FrameType { return vtui.TypeUser }

func (qf *QueueFrame) ProcessKey(e *vtinput.InputEvent) bool {
	if e.KeyDown && (e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10) {
		active := false
		GlobalQueueManager.mu.Lock()
		for _, t := range GlobalQueueManager.tasks {
			t.mu.Lock()
			isActive := t.State == "Queued" || t.State == "Starting" || t.State == "Scanning" || t.State == "Running"
			t.mu.Unlock()
			if isActive {
				active = true
				break
			}
		}
		GlobalQueueManager.mu.Unlock()
		if active {
			vtui.ShowToast("Cannot close queue while operations are active. Use Ctrl+Tab to switch.", 3*time.Second)
			return true // Swallow ESC/F10
		}
	}

	// Let BaseWindow process focus cycling and button clicks
	if qf.BaseWindow.ProcessKey(e) {
		return true
	}

	// Pressing Enter on an Error task can show details
	if e.KeyDown && e.VirtualKeyCode == vtinput.VK_RETURN {
		idx := qf.table.SelectPos
		if idx >= 0 && idx < len(qf.tasks) {
			t := qf.tasks[idx]
			t.mu.Lock()
			isErr := t.State == "Error"
			errMsg := t.ErrorMsg
			t.mu.Unlock()

			if isErr && errMsg != nil {
				vtui.ShowMessageOn(qf, " Error Details ", errMsg.Error(), []string{"&Ok"})
			}
			return true
		}
	}

	return false
}
