package fileops

import (
	"errors"
	"reflect"
	"testing"

	"github.com/unxed/vtui"
)

// TestQueueRowGetCellText covers every column queueRow.GetCellText renders,
// including the CurrentFile/Desc split for the description column and the
// unknown-column fallback the table never actually asks for in practice.
func TestQueueRowGetCellText(t *testing.T) {
	tests := []struct {
		name string
		task *QueueTask
		col  int
		want string
	}{
		{"id column", &QueueTask{ID: 42}, 0, "42"},
		{"state column", &QueueTask{State: "Running"}, 1, "Running"},
		{"type column", &QueueTask{Type: "Copy"}, 2, "Copy"},
		{"desc shown while idle", &QueueTask{State: "Queued", Desc: "copy foo", CurrentFile: "foo.txt"}, 3, "copy foo"},
		{"current file shown while running", &QueueTask{State: "Running", Desc: "copy foo", CurrentFile: "foo.txt"}, 3, "foo.txt"},
		{"current file shown while scanning", &QueueTask{State: "Scanning", Desc: "copy foo", CurrentFile: "bar.txt"}, 3, "bar.txt"},
		{"current file shown while cancelling", &QueueTask{State: "Cancelling", Desc: "copy foo", CurrentFile: "baz.txt"}, 3, "baz.txt"},
		{"desc shown once done", &QueueTask{State: "Done", Desc: "copy foo", CurrentFile: "foo.txt"}, 3, "copy foo"},
		{"progress bar empty", &QueueTask{Progress: 0}, 4, "  0% ░░░░░░░░░░"},
		{"progress bar partial", &QueueTask{Progress: 45}, 4, " 45% ████░░░░░░"},
		{"progress bar full", &QueueTask{Progress: 100}, 4, "100% ██████████"},
		{"speed column", &QueueTask{Speed: "1.2 MB/s"}, 5, "1.2 MB/s"},
		{"unknown column falls back to empty", &QueueTask{ID: 1, State: "Running"}, 6, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (queueRow{task: tt.task}).GetCellText(tt.col)
			if got != tt.want {
				t.Fatalf("GetCellText(%d) = %q, want %q", tt.col, got, tt.want)
			}
		})
	}
}

// TestQueueRowGetCellAttrRemainingStates fills in the branches
// TestQueueFrameUsesDialogThemeColors leaves out: the dimmed Cancelled/
// Cancelling states and the plain default a Queued/unrecognized state gets.
func TestQueueRowGetCellAttrRemainingStates(t *testing.T) {
	def := vtui.SetRGBBoth(0, 0x112233, 0x445566)
	tests := []struct {
		state string
		want  uint64
	}{
		{"Cancelled", vtui.DimColor(def)},
		{"Cancelling", vtui.DimColor(def)},
		{"Queued", def},
		{"", def},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := (queueRow{task: &QueueTask{State: tt.state}}).GetCellAttr(0, def)
			if got != tt.want {
				t.Fatalf("%s attr = %#x, want %#x", tt.state, got, tt.want)
			}
		})
	}
}

// TestQueueHasActiveTasks covers the guard branches QueueHasActiveTasks takes
// before it ever gets to compare a state: no manager at all, an empty queue,
// a nil entry mixed into the slice, and a queue made only of terminal tasks.
func TestQueueHasActiveTasks(t *testing.T) {
	original := GlobalQueueManager
	t.Cleanup(func() { GlobalQueueManager = original })

	t.Run("nil manager", func(t *testing.T) {
		GlobalQueueManager = nil
		if QueueHasActiveTasks() {
			t.Fatal("a nil manager must report no active tasks")
		}
	})

	tests := []struct {
		name  string
		tasks []*QueueTask
		want  bool
	}{
		{"no tasks", nil, false},
		{"only terminal tasks", []*QueueTask{{State: "Done"}, {State: "Error"}, {State: "Cancelled"}}, false},
		{"nil entry is skipped, not active", []*QueueTask{nil, {State: "Done"}}, false},
		{"a queued task counts as active", []*QueueTask{{State: "Done"}, {State: "Queued"}}, true},
		{"a running task counts as active", []*QueueTask{{State: "Running"}}, true},
		{"a cancelling task still counts as active", []*QueueTask{{State: "Cancelling"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GlobalQueueManager = NewQueueManagerWithTasks(tt.tasks...)
			if got := QueueHasActiveTasks(); got != tt.want {
				t.Fatalf("QueueHasActiveTasks() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestQueueTaskStatus checks that Status hands back the three fields it
// guards under the task's own lock, unmodified.
func TestQueueTaskStatus(t *testing.T) {
	wantErr := errors.New("boom")
	task := &QueueTask{ID: 7, State: "Error", ErrorMsg: wantErr}

	state, id, err := task.Status()
	if state != "Error" || id != 7 || err != wantErr {
		t.Fatalf("Status() = (%q, %d, %v), want (%q, %d, %v)", state, id, err, "Error", 7, wantErr)
	}
}

// TestQueueFrameSelectedTaskAndIndex covers the cursor-in-bounds and
// cursor-out-of-bounds paths of SelectedTask/SelectedIndex, which the rest of
// the package only exercises indirectly through key handling.
func TestQueueFrameSelectedTaskAndIndex(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()

	task0 := &QueueTask{ID: 1}
	task1 := &QueueTask{ID: 2}
	qf.UpdateTasks([]*QueueTask{task0, task1})

	tests := []struct {
		name      string
		selectPos int
		wantTask  *QueueTask
		wantIndex int
	}{
		{"first row selected", 0, task0, 0},
		{"second row selected", 1, task1, 1},
		{"negative position is out of bounds", -1, nil, -1},
		{"position past the end is out of bounds", 2, nil, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qf.Table.SelectPos = tt.selectPos
			if got := qf.SelectedTask(); got != tt.wantTask {
				t.Fatalf("SelectedTask() = %v, want %v", got, tt.wantTask)
			}
			if got := qf.SelectedIndex(); got != tt.wantIndex {
				t.Fatalf("SelectedIndex() = %d, want %d", got, tt.wantIndex)
			}
		})
	}
}

// TestOpQueueManagerClearFinished covers the removal, the report of whether
// anything went, and that active tasks survive the sweep untouched.
func TestOpQueueManagerClearFinished(t *testing.T) {
	tests := []struct {
		name             string
		tasks            []*QueueTask
		wantRemoved      bool
		wantRemainingIDs []int
	}{
		{
			name:        "no tasks",
			wantRemoved: false,
		},
		{
			name: "nothing finished yet",
			tasks: []*QueueTask{
				{ID: 1, State: "Running"},
				{ID: 2, State: "Queued"},
			},
			wantRemoved:      false,
			wantRemainingIDs: []int{1, 2},
		},
		{
			name: "finished tasks removed, active tasks kept",
			tasks: []*QueueTask{
				{ID: 1, State: "Done"},
				{ID: 2, State: "Running"},
				{ID: 3, State: "Error"},
				{ID: 4, State: "Cancelled"},
			},
			wantRemoved:      true,
			wantRemainingIDs: []int{2},
		},
		{
			name: "every task finished",
			tasks: []*QueueTask{
				{ID: 1, State: "Done"},
				{ID: 2, State: "Error"},
			},
			wantRemoved: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qm := NewQueueManagerWithTasks(tt.tasks...)

			if got := qm.ClearFinished(); got != tt.wantRemoved {
				t.Fatalf("ClearFinished() = %v, want %v", got, tt.wantRemoved)
			}

			qm.Mu.Lock()
			var gotIDs []int
			for _, task := range qm.tasks {
				gotIDs = append(gotIDs, task.ID)
			}
			qm.Mu.Unlock()
			if !reflect.DeepEqual(gotIDs, tt.wantRemainingIDs) {
				t.Fatalf("remaining task IDs = %v, want %v", gotIDs, tt.wantRemainingIDs)
			}
		})
	}
}

// TestQueueFrameHandleCommand covers both branches of HandleCommand: the root
// claiming the command through HandleWorkspaceFork, and the fallback to
// BaseWindow when the root declines it.
func TestQueueFrameHandleCommand(t *testing.T) {
	original := HandleWorkspaceFork
	t.Cleanup(func() { HandleWorkspaceFork = original })

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qf := NewQueueFrame()

	t.Run("workspace fork claims the command", func(t *testing.T) {
		called := false
		var gotCmd int
		var gotArgs any
		HandleWorkspaceFork = func(cmd int, args any) bool {
			called = true
			gotCmd, gotArgs = cmd, args
			return true
		}
		if !qf.HandleCommand(99, "payload") {
			t.Fatal("HandleCommand should report true when the workspace fork handled it")
		}
		if !called || gotCmd != 99 || gotArgs != "payload" {
			t.Fatalf("HandleWorkspaceFork called=%v with (%v, %v), want (99, payload)", called, gotCmd, gotArgs)
		}
	})

	t.Run("BaseWindow handles it when the workspace fork declines", func(t *testing.T) {
		HandleWorkspaceFork = func(cmd int, args any) bool { return false }
		want := qf.BaseWindow.HandleCommand(99, "payload")
		got := qf.HandleCommand(99, "payload")
		if got != want {
			t.Fatalf("HandleCommand() = %v, want the same as BaseWindow.HandleCommand() = %v", got, want)
		}
	})
}
