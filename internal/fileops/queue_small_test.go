package fileops

import (
	"context"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func smallTestQueue(t *testing.T, bypass bool) *OpQueueManager {
	t.Helper()
	old := config.App.QueueSmallOpsBypass
	config.App.QueueSmallOpsBypass = bypass
	t.Cleanup(func() { config.App.QueueSmallOpsBypass = old })
	return &OpQueueManager{activeKeys: make(map[string]bool)}
}

func smallTestTask(state string, keys []string, paths []string, files ...OpPrecondition) *QueueTask {
	return &QueueTask{State: state, ResKeys: keys, Paths: paths, Preconditions: files}
}

func smallTestFile(path string, size int64) OpPrecondition {
	return OpPrecondition{Path: path, Size: size}
}

// A long copy holds the disk; a few small files elsewhere on it start at
// once instead of waiting for it.
func TestQueueSmallTaskStartsBesideALongOne(t *testing.T) {
	qm := smallTestQueue(t, true)
	long := smallTestTask("Running", []string{"disk:sda"}, []string{"/data/big.iso", "/backup/big.iso"}, smallTestFile("/data/big.iso", 100<<30))
	qm.activeKeys["disk:sda"] = true
	small := smallTestTask("Queued", []string{"disk:sda"}, []string{"/home/u/run.sh", "/backup/run.sh"}, smallTestFile("/home/u/run.sh", 4096))
	qm.tasks = []*QueueTask{long, small}

	if got := qm.nextRunnable(); got != small {
		t.Fatalf("nextRunnable = %v, want the small task", got)
	}
	if small.State != "Starting" || !small.bypass {
		t.Fatalf("small task: state %q bypass %v, want Starting started beside", small.State, small.bypass)
	}
	if !qm.activeKeys["disk:sda"] {
		t.Fatal("the long task's key was released by the small one starting")
	}
}

func TestQueueSmallTaskWaits(t *testing.T) {
	cases := []struct {
		name   string
		bypass bool
		small  *QueueTask
	}{
		{"its path lies under one of the running task", true,
			smallTestTask("Queued", []string{"disk:sda"}, []string{"/data/dir/a.txt", "/tmp/a.txt"}, smallTestFile("/data/dir/a.txt", 10))},
		{"it writes into what the running task copies", true,
			smallTestTask("Queued", []string{"disk:sda"}, []string{"/home/u/a.txt", "/backup/dir/a.txt"}, smallTestFile("/home/u/a.txt", 10))},
		{"it has a folder", true,
			smallTestTask("Queued", []string{"disk:sda"}, []string{"/home/u/src", "/tmp/src"}, OpPrecondition{Path: "/home/u/src", IsDir: true})},
		{"it is over 16 MB", true,
			smallTestTask("Queued", []string{"disk:sda"}, []string{"/home/u/a", "/tmp/a", "/home/u/b", "/tmp/b"}, smallTestFile("/home/u/a", 10<<20), smallTestFile("/home/u/b", 7<<20))},
		{"it does not say what it touches", true,
			smallTestTask("Queued", []string{"disk:sda"}, nil, smallTestFile("/home/u/a.txt", 10))},
		{"the setting is off", false,
			smallTestTask("Queued", []string{"disk:sda"}, []string{"/home/u/run.sh", "/tmp/run.sh"}, smallTestFile("/home/u/run.sh", 4096))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qm := smallTestQueue(t, tc.bypass)
			long := smallTestTask("Running", []string{"disk:sda"}, []string{"/data/dir", "/backup/dir"}, OpPrecondition{Path: "/data/dir", IsDir: true})
			qm.activeKeys["disk:sda"] = true
			qm.tasks = []*QueueTask{long, tc.small}
			if got := qm.nextRunnable(); got != nil {
				t.Fatalf("nextRunnable started %v, want nothing", got)
			}
			if tc.small.State != "Queued" {
				t.Fatalf("small task state %q, want Queued", tc.small.State)
			}
		})
	}
}

func TestQueueSmallTaskCountsFiles(t *testing.T) {
	var files []OpPrecondition
	for i := 0; i <= queueSmallMaxFiles; i++ {
		files = append(files, smallTestFile("/home/u/f", 1))
	}
	if smallQueueTask(smallTestTask("Queued", nil, []string{"/home/u"}, files[:queueSmallMaxFiles]...)) != true {
		t.Fatalf("%d files: not small", queueSmallMaxFiles)
	}
	if smallQueueTask(smallTestTask("Queued", nil, []string{"/home/u"}, files...)) {
		t.Fatalf("%d files: small", len(files))
	}
}

// A task that takes the disk once it is free still waits for a small task
// started beside the long one when their paths meet.
func TestQueueTaskWaitsForASmallOneOnItsPaths(t *testing.T) {
	qm := smallTestQueue(t, true)
	small := smallTestTask("Running", []string{"disk:sda"}, []string{"/home/u/a.txt", "/backup/a.txt"}, smallTestFile("/home/u/a.txt", 10))
	small.bypass = true
	next := smallTestTask("Queued", []string{"disk:sda"}, []string{"/backup", "/mnt/backup"}, OpPrecondition{Path: "/backup", IsDir: true})
	qm.tasks = []*QueueTask{small, next}
	if got := qm.nextRunnable(); got != nil {
		t.Fatalf("nextRunnable started %v beside the small task on its paths", got)
	}
	small.State = "Done"
	if got := qm.nextRunnable(); got != next || !qm.activeKeys["disk:sda"] {
		t.Fatalf("nextRunnable = %v, keys %v; want the task with its key", got, qm.activeKeys)
	}
}

// The small task holds no key, so finishing it must leave the long task's
// key taken.
func TestQueueSmallTaskKeepsTheKeyOfTheLongOne(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	qm := smallTestQueue(t, true)
	qm.activeKeys["disk:sda"] = true
	small := smallTestTask("Starting", []string{"disk:sda"}, []string{"/home/u/a.txt"})
	small.bypass = true
	small.Run = func(context.Context, TaskReporter, vtui.Frame) error { return nil }
	qm.tasks = []*QueueTask{small}
	qm.executeTask(small)
	if small.State != "Done" {
		t.Fatalf("small task state %q, want Done", small.State)
	}
	if !qm.activeKeys["disk:sda"] {
		t.Fatal("the small task released the key it never took")
	}
}

func TestQueuePathWithin(t *testing.T) {
	for _, tc := range []struct {
		p, dir string
		want   bool
	}{
		{"/a/b", "/a", true},
		{"/a", "/a/", true},
		{"/ab", "/a", false},
		{`C:\Data\x`, `c:\data`, true},
		{"/anything", "/", true},
	} {
		if got := queuePathWithin(tc.p, tc.dir); got != tc.want {
			t.Errorf("queuePathWithin(%q, %q) = %v, want %v", tc.p, tc.dir, got, tc.want)
		}
	}
}

func TestTransferQueuePaths(t *testing.T) {
	fs := vfs.NewOSVFS("/")
	j := fs.Join
	got := transferQueuePaths(fs, "/src", []string{"a.txt", "b.txt"}, fs, "/dst", true, "")
	want := []string{j("/src", "a.txt"), j("/dst", "a.txt"), j("/src", "b.txt"), j("/dst", "b.txt")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("into a folder: %q, want %q", got, want)
	}
	if got := transferQueuePaths(fs, "/src", []string{"a.txt"}, fs, "/dst/c.txt", false, ""); !reflect.DeepEqual(got, []string{j("/src", "a.txt"), "/dst/c.txt"}) {
		t.Fatalf("to a name: %q", got)
	}
	if got := transferQueuePaths(fs, "/src", []string{"a.txt"}, fs, "/dst", true, "*.bak"); !reflect.DeepEqual(got, []string{j("/src", "a.txt"), "/dst"}) {
		t.Fatalf("under a mask: %q", got)
	}
}
