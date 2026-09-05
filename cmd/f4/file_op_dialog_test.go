package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestFileOpProgressDialogBackgroundButton(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	d := NewFileOpProgressDialog(" Test ")

	// An operation that cannot be left running gets no such button, which is
	// most of them: closing the window of a copy has to stop the copy.
	if d.btnBackground != nil {
		t.Fatal("the background button appeared without being asked for")
	}

	called := 0
	d.EnableBackground(func() { called++ })
	if d.btnBackground == nil {
		t.Fatal("EnableBackground added no button")
	}
	d.btnBackground.OnClick()
	if called != 1 {
		t.Errorf("the button called back %d times, want 1", called)
	}

	// Asking twice replaces the callback rather than growing a second
	// button, since a dialog can be reconfigured as an operation goes on.
	first := d.btnBackground
	d.EnableBackground(func() { called += 10 })
	if d.btnBackground != first {
		t.Error("a second background button was added")
	}
	d.btnBackground.OnClick()
	if called != 11 {
		t.Errorf("the replaced callback did not take over: %d", called)
	}
	vtui.AssertLayout(t, d)
}

func TestFileOpProgressDialog_Layout(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	dlg := NewFileOpProgressDialog(" Test Dialog ")

	// Use vtui's layout validator to ensure no overlaps and proper margins
	vtui.AssertLayout(t, dlg)
}

func TestFileOpProgressDialog_VisibilityModes(t *testing.T) {
	vtui.SetDefaultPalette()
	dlg := NewFileOpProgressDialog(" Test ")

	// 1. Scan Mode
	dlg.UpdateScan("/usr/bin", 150, 10)

	if dlg.pbCurrent.IsVisible() || dlg.pbTotal.IsVisible() || dlg.lblSpeed.IsVisible() {
		t.Error("Progress bars and speed label should be hidden in scan mode")
	}

	if dlg.lblTotal.GetText() != "Found: 150 files, 10 folders" {
		t.Errorf("Scan text mismatch: %s", dlg.lblTotal.GetText())
	}

	// 2. Transfer Mode
	dlg.UpdateTransfer("Copying", "file.txt", 50, "Total: 100MB", 20, "5 MB/s")

	if !dlg.pbCurrent.IsVisible() || !dlg.pbTotal.IsVisible() || !dlg.lblSpeed.IsVisible() {
		t.Error("Progress bars and speed label should be visible in transfer mode")
	}

	if dlg.pbCurrent.Percent != 50 || dlg.pbTotal.Percent != 20 {
		t.Errorf("Percents not updated correctly: Curr=%d, Tot=%d", dlg.pbCurrent.Percent, dlg.pbTotal.Percent)
	}

	if dlg.lblCurrent.GetText() != "Copying: file.txt" {
		t.Errorf("Action text mismatch: %s", dlg.lblCurrent.GetText())
	}
}

func TestFileOpProgressDialog_LongPathTruncation(t *testing.T) {
	vtui.SetDefaultPalette()
	dlg := NewFileOpProgressDialog(" Truncation Test ")

	// Very long path should be truncated to fit 54 chars
	longPath := "/home/user/very/long/directory/structure/that/definitely/exceeds/the/dialog/width/limit/filename.txt"
	dlg.UpdateScan(longPath, 1, 1)

	text := dlg.lblCurrent.GetText()
	if len(text) > 54 {
		t.Errorf("UpdateScan failed to truncate long path: length %d", len(text))
	}
	if !strings.HasPrefix(text, "Scanning: ") {
		t.Error("UpdateScan lost the prefix")
	}
	if !strings.HasSuffix(text, "...") {
		t.Error("UpdateScan failed to add ellipsis")
	}
}
func TestFileOpProgressDialog_TransferFormatting(t *testing.T) {
	vtui.SetDefaultPalette()
	dlg := NewFileOpProgressDialog(" Formatting Test ")

	// Test the 3-column time/ETA/speed string
	// format: "%-16s %-21s %15s" -> 16+1+21+1+15 = 54 characters
	elapsed := "Time: 01:02:03"  // 14 chars
	eta := "Remaining: 00:00:59" // 19 chars
	speed := "10.5 MB/s"         // 9 chars

	// Simulating the format logic from file_ops.go
	timeSpeedText := fmt.Sprintf("%-16s %-21s %15s", elapsed, eta, speed)

	dlg.UpdateTransfer("Copying", "file.txt", 50, "Total: 100MB", 10, timeSpeedText)

	got := dlg.lblSpeed.GetText()
	if len(got) > 54 {
		t.Errorf("Transfer info string too long: %d chars, max 54", len(got))
	}

	// Check alignment (speed should be on the right)
	if !strings.HasSuffix(got, speed) {
		t.Errorf("Speed alignment failed. Expected suffix %q, got %q", speed, got)
	}
}
