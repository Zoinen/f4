package main

import (
	"io"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

func TestDummyPluginVFSAndCommands(t *testing.T) {
	plugin := &DummyPlugin{}
	drives, err := plugin.Init(nil)
	if err != nil || len(drives) != 1 || drives[0] != "Dummy RPC VFS" {
		t.Fatalf("Init() = %v, %v", drives, err)
	}

	attrs, _, err := plugin.Highlight("a1", nil, 7)
	if err != nil || len(attrs) != 2 || attrs[0] != 7 || attrs[1] == 7 {
		t.Fatalf("Highlight() = %v, %v", attrs, err)
	}
	if handled, err := plugin.ProcessKey("", vtinput.InputEvent{}); handled || err != nil {
		t.Fatalf("ProcessKey(no-op) = %v, %v", handled, err)
	}
	if handled, err := plugin.ProcessKey("", vtinput.InputEvent{
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F1,
	}); !handled || err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("ProcessKey(F1) = %v, %v", handled, err)
	}

	commands := plugin.PluginCommands()
	if len(commands) != 1 || commands[0].ID != "dummy-rpc.hello" || len(commands[0].ActiveDrives) != 1 {
		t.Fatalf("PluginCommands() = %#v", commands)
	}
	if err := plugin.RunPluginCommand("missing"); err == nil {
		t.Fatal("unknown plugin command unexpectedly succeeded")
	}
	if err := plugin.RunPluginCommand("dummy-rpc.hello"); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("initialized command without host = %v", err)
	}

	root, err := plugin.ReadDir("", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(root) != 6 {
		t.Fatalf("root entries = %d, want 6", len(root))
	}
	rootStat, err := plugin.Stat("", "/")
	if err != nil || !rootStat.IsDir {
		t.Fatalf("root stat = %+v, %v", rootStat, err)
	}
	fileStat, err := plugin.Stat("", "/file_1.txt")
	if err != nil || fileStat.IsDir || fileStat.Size == 0 {
		t.Fatalf("file stat = %+v, %v", fileStat, err)
	}
	if _, err := plugin.Stat("", "/missing"); err == nil {
		t.Fatal("missing stat unexpectedly succeeded")
	}

	fileID, size, err := plugin.Open("", "/file_1.txt")
	if err != nil || size != fileStat.Size {
		t.Fatalf("Open() = %d, %d, %v", fileID, size, err)
	}
	chunk, err := plugin.ReadAt(fileID, 4, 0)
	if err != nil || string(chunk) != "This" {
		t.Fatalf("ReadAt() = %q, %v", chunk, err)
	}
	if _, err := plugin.ReadAt(fileID, 4, fileStat.Size); err != io.EOF {
		t.Fatalf("ReadAt(EOF) = %v", err)
	}
	if _, err := plugin.ReadAt(999, 1, 0); err == nil {
		t.Fatal("invalid handle read unexpectedly succeeded")
	}
	if _, _, err := plugin.Open("", "/folder"); err == nil {
		t.Fatal("directory open unexpectedly succeeded")
	}

	createdID, err := plugin.Create("", "/created.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := plugin.Write(createdID, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	created, err := plugin.ReadAt(createdID, 10, 0)
	if err != nil || string(created) != "abc" {
		t.Fatalf("created file = %q, %v", created, err)
	}
	if err := plugin.Write(999, []byte("bad")); err == nil {
		t.Fatal("invalid handle write unexpectedly succeeded")
	}
	if err := plugin.CloseFile(createdID); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.ReadAt(createdID, 1, 0); err == nil {
		t.Fatal("closed handle read unexpectedly succeeded")
	}

	if err := plugin.MkDir("", "/newdir"); err != nil {
		t.Fatal(err)
	}
	if item, err := plugin.Stat("", "/newdir"); err != nil || !item.IsDir {
		t.Fatalf("new directory stat = %+v, %v", item, err)
	}
	if err := plugin.Rename("", "/created.txt", "/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Rename("", "/missing", "/still-missing"); err == nil {
		t.Fatal("missing rename unexpectedly succeeded")
	}
	if err := plugin.Remove("", "/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.Stat("", "/renamed.txt"); err == nil {
		t.Fatal("removed file unexpectedly exists")
	}
}
