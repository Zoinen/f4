package fileops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestRecursiveCopyRejectsSameLocalObject(t *testing.T) {
	for _, alias := range []string{"case", "hardlink"} {
		t.Run(alias, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "_1.MMM")
			target := filepath.Join(root, "_1.mmm")
			const payload = "the source must never be truncated"
			if err := os.WriteFile(source, []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			if alias == "hardlink" {
				target = filepath.Join(root, "alias.bin")
				if err := os.Link(source, target); err != nil {
					t.Skipf("hard links unavailable: %v", err)
				}
			} else if _, err := os.Stat(target); os.IsNotExist(err) {
				t.Skip("test volume is case-sensitive")
			}
			filesystem := vfs.NewOSVFS(root)
			err := recursiveCopy(
				t.Context(), filesystem, source, filesystem, target,
				&FileOpState{OverwriteAll: true}, 0,
			)
			t.Logf("[FIX:same-file] alias=%s error=%v", alias, err)
			if err == nil || !strings.Contains(err.Error(), "onto itself") {
				t.Errorf("copy to same object = %v, want self-copy rejection", err)
			}
			got, readErr := os.ReadFile(source)
			if readErr != nil || string(got) != payload {
				t.Fatalf("source corrupted: %q, %v", got, readErr)
			}
		})
	}
}

func TestOptimizedRenameChangesCase(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "_1.MMM")
	target := filepath.Join(root, "_1.mmm")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	filesystem := vfs.NewOSVFS(root)
	renamed, err := tryOptimizedRename(
		context.Background(), filesystem, filesystem, source, target,
	)
	if err != nil || !renamed {
		t.Fatalf("case rename = %v, %v, want success", renamed, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "_1.mmm" {
		t.Fatalf("case rename did not change stored spelling: %v, %v", entries, err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatalf("renamed contents = %q, %v", got, err)
	}
}

func TestSelfCopyDoesNotBlockNextQueuedCopy(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	source := filepath.Join(root, "_1.MMM")
	alias := filepath.Join(root, "alias.bin")
	target := filepath.Join(root, "_2.mmm")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	filesystem := vfs.NewOSVFS(root)
	queue := startPrivateQueueWorker(t)
	completed := make(chan struct{}, 2)
	tasks := make([]*QueueTask, 0, 2)
	for _, destination := range []string{alias, target} {
		task := &QueueTask{
			ResKeys: []string{"same-disk"},
			Run: func(ctx context.Context, _ TaskReporter, _ vtui.Frame) error {
				return recursiveCopy(
					ctx, filesystem, source, filesystem, destination, &FileOpState{}, 0,
				)
			},
			OnComplete: func() { completed <- struct{}{} },
		}
		tasks = append(tasks, task)
		queue.Enqueue(task)
	}
	deadline := time.After(3 * time.Second)
	for remaining := 2; remaining > 0; {
		select {
		case <-completed:
			remaining--
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			for _, task := range tasks {
				task.cancel()
			}
			t.Fatal("self-copy kept subsequent copy queued")
		}
	}
	for i, want := range []string{"Error", "Done"} {
		tasks[i].Mu.Lock()
		got := tasks[i].State
		tasks[i].Mu.Unlock()
		if got != want {
			t.Errorf("task %d state=%s, want %s", i, got, want)
		}
	}
	for _, path := range []string{source, target} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "keep" {
			t.Errorf("contents at %q = %q, %v", path, got, err)
		}
	}
}

func TestFoldOSPathCase(t *testing.T) {
	t.Run("case-insensitive filesystem folds both sides", func(t *testing.T) {
		src, dst := foldOSPathCase("/a/Foo", "/A/foo", true)
		if src != dst {
			t.Fatalf("folded paths differ: %q vs %q", src, dst)
		}
	})
	t.Run("case-sensitive filesystem keeps them apart", func(t *testing.T) {
		src, dst := foldOSPathCase("/a/Foo", "/a/foo", false)
		if src == dst {
			t.Fatalf("Foo and foo compared equal on a case-sensitive filesystem: %q", src)
		}
		if src != "/a/Foo" || dst != "/a/foo" {
			t.Fatalf("paths were changed: %q, %q", src, dst)
		}
	})
}
