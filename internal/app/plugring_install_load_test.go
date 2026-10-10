package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/vtui"
)

// TestPlugRingInstallLoadsPluginOffUIGoroutine is f4#1710. After a plugin is
// downloaded f4 starts it, and starting it asks the user's permission to run
// it: a dialog that only the UI goroutine can show. The load used to run ON the
// UI goroutine, so it waited for a dialog it was itself keeping from being
// shown, and f4 froze (Android plugin, first install).
//
// The load is replaced by one that asks for a UI task and waits for it, which
// is exactly the shape of that permission prompt. If the load runs on the
// goroutine pumping the UI queue (here the test's own), the task cannot run
// while the load waits, and the wait times out.
func TestPlugRingInstallLoadsPluginOffUIGoroutine(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpConfig)
	t.Setenv("APPDATA", tmpConfig)
	config.ResetConfigDirForTest()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\necho running\n"))
	}))
	defer ts.Close()

	previousLoad := loadPlugRingItem
	t.Cleanup(func() { loadPlugRingItem = previousLoad })
	uiAnswered := make(chan bool, 1)
	loaded := make(chan struct{})
	loadPlugRingItem = func(plughost.PlugRingItem) {
		defer close(loaded)
		ran := make(chan struct{})
		vtui.FrameManager.PostTask(func() { close(ran) })
		select {
		case <-ran:
			uiAnswered <- true
		case <-time.After(2 * time.Second):
			uiAnswered <- false
		}
	}

	item := plughost.PlugRingItem{
		ID:         "load-off-ui",
		Name:       "Load Off UI",
		Version:    "1.0.0",
		Author:     "Tester",
		URL:        ts.URL + "/plugin.sh",
		Entrypoint: "sh plugin.sh",
	}

	pf := panel.NewPanelsFrame()
	defer pf.Close()

	done := make(chan struct{})
	answered := map[*vtui.Window]bool{}
	answer := func() {
		top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		if !ok || top == nil || top.OnResult == nil || answered[top] {
			return
		}
		if strings.Contains(top.GetTitle(), "Installing Plugin") {
			return
		}
		answered[top] = true
		top.OnResult(0)
		top.SetExitCode(-1)
		vtui.FrameManager.Pop()
	}

	go actionInstallPlugRingItem(pf, nil, item, func() { close(done) })

	timeout := time.After(10 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			answer()
		case <-tick.C:
			answer()
		case <-done:
			break Loop
		case <-timeout:
			t.Fatal("timeout waiting for the installation to finish")
		}
	}

	select {
	case <-loaded:
	default:
		t.Fatal("the installed plugin was never loaded")
	}
	if ok := <-uiAnswered; !ok {
		t.Fatal("the plugin load blocked the UI goroutine: a task posted to it did not run while the load waited (f4#1710)")
	}
}
