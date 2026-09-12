package androidfs

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestSyncVFSPublicDevicePaths(t *testing.T) {
	var icon vfs.PanelIconProvider = newSyncVFS(nil, "serial", "Pixel 3", nil, nil)
	if got := icon.PanelIcon(); got != "android-logo" {
		t.Fatalf("Sync panel icon = %q", got)
	}
	client := &fakeSyncFS{
		entries: map[string]SyncEntry{
			"/sdcard/DCIM":      {Mode: remoteModeDir | 0755},
			"/sdcard/DCIM/file": {Mode: 0100644},
		},
	}
	fs := newSyncVFS(nil, "stable-serial", "Pixel 3", client, nil)
	if err := fs.SetPath("/sdcard/DCIM"); err != nil {
		t.Fatal(err)
	}
	const dir = "android://Pixel 3/sdcard/DCIM"
	if fs.PanelTitle(fs.GetPath()) != fs.GetPath() {
		t.Fatal("display path differs from public path")
	}
	if got := fs.GetPath(); got != dir {
		t.Errorf("GetPath = %q, want %q", got, dir)
	}
	if got := fs.Join(fs.GetPath(), "file"); got != dir+"/file" {
		t.Errorf("item path = %q", got)
	}
	if got, err := fs.Abs("file"); err != nil || got != dir+"/file" {
		t.Errorf("Abs = %q, %v", got, err)
	}
	if _, err := fs.Stat(context.Background(), dir+"/file"); err != nil {
		t.Errorf("public Stat: %v", err)
	}
	if _, err := fs.Abs("android://Other/sdcard/file"); err == nil {
		t.Error("accepted foreign device")
	}
	clone := fs.Clone()
	if clone.GetPath() != dir || clone.(interface{ SessionKey() any }).SessionKey() != fs.SessionKey() {
		t.Error("clone lost path or serial identity")
	}
	if !fs.IsAbs(dir) || fs.Dir(dir+"/file") != dir || fs.Base(dir+"/file") != "file" {
		t.Error("public path algebra failed")
	}
	var _ vfs.VFS = fs
}

func TestSyncVFSPublicPathsKeepTransportPaths(t *testing.T) {
	client := &fakeSyncFS{files: map[string][]byte{}}
	commands := []string{}
	fs := newSyncVFS(nil, "serial", "Pixel 3", client, func(_ context.Context, serial, command string) (shellResult, error) {
		if serial != "serial" {
			t.Fatalf("serial = %q", serial)
		}
		commands = append(commands, command)
		return shellResult{}, nil
	})
	ctx := context.Background()
	p := fs.Join(fs.GetPath(), "sdcard", "a'b #?.txt")
	w, err := fs.Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "body"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if client.sendPath != "/sdcard/a'b #?.txt" {
		t.Fatalf("SEND path = %q", client.sendPath)
	}
	if err := fs.Remove(ctx, p); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || strings.Contains(commands[0], "android://") {
		t.Fatalf("shell commands = %v", commands)
	}
	for _, p := range []string{"android://Other/sdcard/file", "android://Pixel 3/sdcard/%2e%2e/file", "android://Pixel 3/"} {
		if err := fs.Remove(ctx, p); err == nil {
			t.Errorf("accepted mutation %q", p)
		}
	}
	if len(commands) != 1 {
		t.Fatal("invalid mutation reached shell")
	}
}
