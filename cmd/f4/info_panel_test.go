package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type testPanelInfoVFS struct {
	*vfs.NullVFS

	mu        sync.Mutex
	cached    map[string]vfs.PanelInfoSnapshot
	fresh     map[string]bool
	lastReq   vfs.PanelInfoRequest
	refreshFn func(context.Context, vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error)
	publish   bool
}

func newTestPanelInfoVFS() *testPanelInfoVFS {
	return &testPanelInfoVFS{
		NullVFS: vfs.NewNullVFS(0),
		cached:  make(map[string]vfs.PanelInfoSnapshot),
		fresh:   make(map[string]bool),
		publish: true,
	}
}

func newStableInfoTestPanel(x, y, w, h int, filesystem vfs.VFS, entries []*fileEntry) *FileSystemPanel {
	fsp := NewFileSystemPanel(x, y, w, h, filesystem)
	// NewFileSystemPanel starts its real directory load immediately. Provider
	// tests install a synthetic row set, so cancel that constructor load first;
	// otherwise its queued completion can replace the rows while an async info
	// refresh test is draining the shared UI-task queue.
	if fsp.cancelLoad != nil {
		fsp.cancelLoad()
		fsp.cancelLoad = nil
	}
	if fsp.loadingTimer != nil {
		fsp.loadingTimer.Stop()
		fsp.loadingTimer = nil
	}
	fsp.isLoading = false
	fsp.entries = entries
	fsp.SetCursorIndex(0)
	fsp.Refresh()
	return fsp
}

func (v *testPanelInfoVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if req.SelectedName != "" {
		return req.SelectedName
	}
	return req.Path
}

func (v *testPanelInfoVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lastReq = req
	key := v.PanelInfoKey(req)
	return v.cached[key], v.fresh[key]
}

func (v *testPanelInfoVFS) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if v.refreshFn == nil {
		return vfs.PanelInfoSnapshot{}, nil
	}
	snapshot, err := v.refreshFn(ctx, req)
	if err == nil && v.publish {
		v.mu.Lock()
		v.cached[v.PanelInfoKey(req)] = snapshot
		v.fresh[v.PanelInfoKey(req)] = true
		v.mu.Unlock()
	}
	return snapshot, err
}

func testInfoSnapshot(model string, memory uint64) vfs.PanelInfoSnapshot {
	return vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID:       "device",
			TitleKey: "TestInfoPanel.MissingDeviceTitle",
			Title:    "Android device",
			Fields: []vfs.PanelInfoField{
				{ID: "model", LabelKey: "TestInfoPanel.MissingModel", Label: "Model", Value: model},
				{ID: "memory", LabelKey: "TestInfoPanel.MissingMemory", Label: "Device memory", Kind: vfs.PanelInfoBytes, Bytes: memory},
			},
		}},
	}
}

func infoPanelHasRow(ip *InfoPanel, label, value string) bool {
	for _, row := range ip.rows {
		if row.label == label && row.value == value {
			return true
		}
	}
	return false
}

func infoPanelHasSection(ip *InfoPanel, title string) bool {
	for _, row := range ip.rows {
		if row.label == "" && strings.Contains(row.text, title) {
			return true
		}
	}
	return false
}

func infoPanelHasUsageMeter(ip *InfoPanel, label string) bool {
	for i := 0; i+1 < len(ip.rows); i++ {
		first, second := ip.rows[i], ip.rows[i+1]
		if first.label == label && first.copyable && first.usageBarWidth > 0 &&
			second.label == label && !second.copyable {
			return true
		}
	}
	return false
}

func runInfoPanelUITasksUntil(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for !done() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatal("timed out waiting for information-panel refresh completion")
		}
	}
}

func drainInfoPanelUITasks(t *testing.T) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	quiet := time.NewTimer(100 * time.Millisecond)
	defer quiet.Stop()
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(100 * time.Millisecond)
		case <-quiet.C:
			return
		case <-deadline.C:
			t.Fatal("UI task queue did not become quiet")
		}
	}
}

// TestPanelsFrame_CtrlL_TogglesInfoPanel exercises far2l's Ctrl+L:
//   - first press installs an InfoPanel on the passive side, keeping
//     the file panel underneath alive and focus on the active side;
//   - second press removes it (toggle);
//   - Tab that lands on the alt slot keeps it open — the panel
//     visually becomes focused (as in far2l), but commands still
//     target the source file panel underneath.
func TestPanelsFrame_CtrlL_TogglesInfoPanel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	send := func(vk uint16, mods vtinput.ControlKeyState) {
		pressKey(pf, &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		})
	}

	// setupMockPanelsFrame sets activeIdx = 1 (right). Passive is left.
	if pf.altPanels[0] != nil || pf.altPanels[1] != nil {
		t.Fatal("precondition: no alt panels expected initially")
	}

	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] == nil {
		t.Fatal("Ctrl+L should install alt panel on passive (left) side")
	}
	if pf.altPanels[1] != nil {
		t.Error("active (right) side must not get an alt panel")
	}
	if _, ok := pf.altPanels[0].(*InfoPanel); !ok {
		t.Errorf("expected *InfoPanel, got %T", pf.altPanels[0])
	}
	if _, ok := pf.panels[0].(*FileSystemPanel); !ok {
		t.Error("file panel underneath must stay alive")
	}
	if pf.activeIdx != 1 {
		t.Errorf("Ctrl+L must not move active panel; got activeIdx=%d", pf.activeIdx)
	}

	// Source of the alt panel is the current active file panel.
	if src := pf.altPanels[0].Source(); src != pf.panels[1].(*FileSystemPanel) {
		t.Error("alt panel source should be the active file panel")
	}

	// Second press toggles it off.
	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] != nil {
		t.Error("second Ctrl+L should remove alt panel")
	}

	// Install again, then Tab to the alt side — Tab must keep the
	// alt panel visible AND flip its focused state so the frame
	// title recolors (matches far2l).
	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] == nil {
		t.Fatal("re-install: alt panel should be present again")
	}
	send(vtinput.VK_TAB, 0)
	if pf.activeIdx != 0 {
		t.Fatalf("Tab should switch active to left; got activeIdx=%d", pf.activeIdx)
	}
	if pf.altPanels[0] == nil {
		t.Error("Tab must NOT close the alt panel — it should stay visible")
	}
	// A render is required to propagate SetFocus into the alt panel;
	// call Show and then check the focus state was flipped.
	pf.Show(vtui.NewSilentScreenBuf())
	if !pf.altPanels[0].IsFocused() {
		t.Error("after Tab + render, alt panel should report focused=true")
	}

	// Ctrl+L while focus is ON the alt panel must close IT (matches
	// far2l), not open another one on the opposite side.
	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] != nil {
		t.Error("Ctrl+L on focused alt panel should close it")
	}
	if pf.altPanels[1] != nil {
		t.Error("Ctrl+L on focused alt must not spawn a second alt on the opposite side")
	}
}

// TestInfoPanel_ShowRenders verifies the panel renders without panic
// for a source panel with a couple of entries and clips to its width.
func TestInfoPanel_ShowRenders(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
	}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 19)
	// Should render without panic even on paths that fsSpace can't
	// resolve; fsSpace on tmp dirs works on unix, no crash otherwise.
	ip.Show(scr)

	if ip.Kind() != "info" {
		t.Errorf("Kind() = %q, want %q", ip.Kind(), "info")
	}
	if ip.Source() != fsp {
		t.Error("Source() should return the file panel we passed")
	}

	// SetFocus now tracks a visible focus marker (title recolour) —
	// used when Tab lands on the alt-panel slot.
	ip.SetFocus(true)
	if !ip.IsFocused() {
		t.Error("SetFocus(true) should be reflected by IsFocused")
	}
	ip.SetFocus(false)
	if ip.IsFocused() {
		t.Error("SetFocus(false) should clear the focus marker")
	}
}

func TestInfoPanel_AuthoritativeProviderReplacesLocalHostStats(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	if err := provider.SetPath("/sdcard"); err != nil {
		t.Fatal(err)
	}
	provider.cached["SM-G930F"] = testInfoSnapshot("SM-G930F", 4*1024*1024*1024)
	provider.fresh["SM-G930F"] = true

	fsp := newStableInfoTestPanel(0, 0, 50, 39, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "SM-G930F", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 39)

	oldBytes := AppConfig.InfoPanelBytes
	defer func() { AppConfig.InfoPanelBytes = oldBytes }()
	AppConfig.InfoPanelBytes = false
	ip.Show(scr)

	if !infoPanelHasSection(ip, "Android device") {
		t.Fatal("provider section was not rendered")
	}
	if !infoPanelHasRow(ip, "Model", "SM-G930F") {
		t.Fatal("provider model row was not rendered")
	}
	if !infoPanelHasRow(ip, "Device memory", formatBytesHuman(4*1024*1024*1024)) {
		t.Fatal("provider byte field did not use the information-panel formatter")
	}
	if infoPanelHasRow(ip, Msg("InfoPanel.Computer"), "") {
		t.Fatal("authoritative provider must replace, not augment, local computer data")
	}
	for _, row := range ip.rows {
		if row.label == Msg("InfoPanel.Computer") || row.label == Msg("InfoPanel.User") {
			t.Fatalf("local host row %q leaked into authoritative provider view", row.label)
		}
	}
	if infoPanelHasSection(ip, Msg("InfoPanel.MemoryTitle")) {
		t.Fatal("local memory section leaked into authoritative provider view")
	}
	if !infoPanelHasRow(ip, Msg("InfoPanel.CurrentDir"), provider.GetPath()) {
		t.Fatal("remote current directory should remain visible")
	}

	AppConfig.InfoPanelBytes = true
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Device memory", formatBytesCommas(4*1024*1024*1024)) {
		t.Fatal("B units mode did not apply to provider byte fields")
	}
}

func TestInfoPanel_ProviderTracksCurrentSelectionFromCache(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["first"] = testInfoSnapshot("Pixel", 1)
	provider.cached["second"] = testInfoSnapshot("Galaxy", 2)
	provider.fresh["first"] = true
	provider.fresh["second"] = true
	fsp := newStableInfoTestPanel(0, 0, 50, 34, provider, []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "first", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "second", IsDir: true}},
	})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)

	fsp.SetCursorIndex(0)
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "Pixel") {
		t.Fatal("first selected device was not rendered from cache")
	}
	fsp.SetCursorIndex(1)
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "Galaxy") {
		t.Fatal("selection change was not reflected immediately from cache")
	}
	if infoPanelHasRow(ip, "Model", "Pixel") {
		t.Fatal("old selected device remained visible after selection change")
	}
	provider.mu.Lock()
	lastReq := provider.lastReq
	provider.mu.Unlock()
	if lastReq.SelectedName != "second" {
		t.Fatalf("provider request SelectedName = %q, want second", lastReq.SelectedName)
	}
}

func TestInfoPanel_ProviderRefreshRunsInBackground(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["device"] = testInfoSnapshot("cached", 1)
	provider.fresh["device"] = false
	started := make(chan struct{})
	release := make(chan struct{})
	provider.refreshFn = func(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
		close(started)
		select {
		case <-release:
			return testInfoSnapshot("fresh", 2), nil
		case <-ctx.Done():
			return vfs.PanelInfoSnapshot{}, ctx.Err()
		}
	}

	fsp := newStableInfoTestPanel(0, 0, 50, 34, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)
	ip.Show(scr)

	if !infoPanelHasRow(ip, "Model", "cached") {
		t.Fatal("stale cache must render immediately while refresh is pending")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	if !infoPanelHasRow(ip, "Model", "cached") {
		t.Fatal("background refresh changed rows before it completed")
	}
	close(release)
	runInfoPanelUITasksUntil(t, func() bool { return ip.infoTask == nil })
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "fresh") {
		t.Fatal("completed background refresh was not rendered")
	}
}

func TestInfoPanel_IgnoresLateRefreshForPreviousSelection(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["first"] = testInfoSnapshot("first cached", 1)
	provider.fresh["first"] = false
	provider.cached["second"] = testInfoSnapshot("second cached", 2)
	provider.fresh["second"] = true
	started := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan struct{})
	provider.refreshFn = func(_ context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
		close(started)
		<-release // deliberately ignore cancellation to simulate a late transport reply
		close(returned)
		return testInfoSnapshot("late first", 3), nil
	}

	fsp := newStableInfoTestPanel(0, 0, 50, 34, provider, []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "first", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "second", IsDir: true}},
	})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)
	fsp.SetCursorIndex(0)
	ip.Show(scr)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first selection refresh did not start")
	}

	fsp.SetCursorIndex(1)
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "second cached") {
		t.Fatal("second selection cache did not replace first immediately")
	}
	close(release)
	<-returned
	drainInfoPanelUITasks(t)
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "second cached") {
		t.Fatal("late completion for previous selection overwrote current device")
	}
	if infoPanelHasRow(ip, "Model", "late first") {
		t.Fatal("late previous-selection data leaked into the current view")
	}
}

func TestInfoPanel_IgnoresLateRefreshFromReplacedVFSWithSameKey(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	oldProvider := newTestPanelInfoVFS()
	oldProvider.cached["device"] = testInfoSnapshot("old cached", 1)
	oldProvider.fresh["device"] = false
	started := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan struct{})
	oldProvider.refreshFn = func(_ context.Context, _ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
		close(started)
		<-release // simulate a transport that returns after cancellation
		close(returned)
		return testInfoSnapshot("late old VFS", 2), nil
	}
	newProvider := newTestPanelInfoVFS()
	newProvider.cached["device"] = testInfoSnapshot("new VFS", 3)
	newProvider.fresh["device"] = true

	fsp := newStableInfoTestPanel(0, 0, 50, 34, oldProvider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)
	ip.Show(scr)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("old VFS refresh did not start")
	}

	// A provider mount can replace fsp.vfs while keeping the same selected row
	// and cache key. Source identity, not the key alone, must invalidate it.
	fsp.vfs = newProvider
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "new VFS") {
		t.Fatal("replacement VFS cache was not rendered")
	}
	close(release)
	<-returned
	drainInfoPanelUITasks(t)
	ip.Show(scr)
	if !infoPanelHasRow(ip, "Model", "new VFS") || infoPanelHasRow(ip, "Model", "late old VFS") {
		t.Fatal("late result from replaced VFS overwrote the current source")
	}
}

func TestInfoPanel_KeepsRefreshResultWhenProviderDoesNotPublishCache(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["device"] = testInfoSnapshot("stale cache", 1)
	provider.fresh["device"] = false
	provider.publish = false
	provider.refreshFn = func(context.Context, vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
		return testInfoSnapshot("returned refresh", 2), nil
	}
	fsp := newStableInfoTestPanel(0, 0, 50, 34, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)
	ip.Show(scr)
	runInfoPanelUITasksUntil(t, func() bool { return ip.infoTask == nil })
	ip.Show(scr)

	if !infoPanelHasRow(ip, "Model", "returned refresh") {
		t.Fatal("stale CachedPanelInfo overwrote the provider's successful returned snapshot")
	}
	if infoPanelHasRow(ip, "Model", "stale cache") {
		t.Fatal("stale cache remained visible after a successful direct refresh")
	}
}

func TestInfoPanel_PassesRawParentSelectionToProvider(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached[".."] = testInfoSnapshot("parent", 1)
	provider.fresh[".."] = true
	fsp := newStableInfoTestPanel(0, 0, 50, 34, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)
	ip.Show(scr)

	provider.mu.Lock()
	lastReq := provider.lastReq
	provider.mu.Unlock()
	if lastReq.SelectedName != ".." {
		t.Fatalf("SelectedName = %q, want raw parent row", lastReq.SelectedName)
	}
}

func TestInfoPanel_PreservesCursorRowAcrossSnapshotExpansion(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 35)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	baseline := testInfoSnapshot("phone", 1)
	provider.cached["device"] = baseline
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 50, 34, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 34)
	ip.SetFocus(true)
	ip.Show(scr)
	for i := range ip.rows {
		if ip.rows[i].label == "Device memory" {
			ip.cursor = i
			break
		}
	}

	expanded := baseline
	expanded.Sections = append([]vfs.PanelInfoSection(nil), baseline.Sections...)
	expanded.Sections[0].Fields = append([]vfs.PanelInfoField{
		{ID: "android", Label: "Android", Value: "8.0"},
		{ID: "build", Label: "Build", Value: "R16NW"},
	}, baseline.Sections[0].Fields...)
	provider.mu.Lock()
	provider.cached["device"] = expanded
	provider.mu.Unlock()
	ip.Show(scr)
	if ip.cursor < 0 || ip.cursor >= len(ip.rows) || ip.rows[ip.cursor].label != "Device memory" {
		t.Fatalf("cursor moved to row %d (%q) after snapshot expansion", ip.cursor, func() string {
			if ip.cursor >= 0 && ip.cursor < len(ip.rows) {
				return ip.rows[ip.cursor].label
			}
			return ""
		}())
	}
}

func TestInfoPanel_CopyUsesFullProviderValueWhenDisplayIsTruncated(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 30)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	fullModel := strings.Repeat("long-model-", 8)
	provider := newTestPanelInfoVFS()
	provider.cached["device"] = testInfoSnapshot(fullModel, 1)
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 30, 29, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 29, 29)
	ip.SetFocus(true)
	ip.Show(scr)

	for i := range ip.rows {
		if ip.rows[i].label != "Model" {
			continue
		}
		if ip.rows[i].value != fullModel {
			t.Fatalf("row retained value %q, want full provider value", ip.rows[i].value)
		}
		if ip.rows[i].text == "" || strings.Contains(ip.rows[i].text, fullModel) {
			t.Fatal("test premise failed: long value was not visually truncated")
		}
		// copyCurrent copies infoRow.value verbatim (covered separately by
		// TestInfoPanel_CopyCopiesValue). Keeping this test off the live Windows
		// clipboard avoids interference from another foreground process.
		return
	}
	t.Fatal("provider Model row not found")
}

func TestInfoPanel_ShortPanelScrollsToLowerProviderRows(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(50, 12)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	fields := make([]vfs.PanelInfoField, 0, 16)
	for i := 0; i < 16; i++ {
		fields = append(fields, vfs.PanelInfoField{
			ID:    fmt.Sprintf("field_%02d", i),
			Label: fmt.Sprintf("Field %02d", i),
			Value: fmt.Sprintf("Value %02d", i),
		})
	}
	provider := newTestPanelInfoVFS()
	provider.cached["device"] = vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID:     "device",
			Title:  "Android device",
			Fields: fields,
		}},
	}
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 50, 12, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 11)
	ip.SetFocus(true)
	ip.Show(scr)

	visibleRows := ip.Y2 - ip.Y1 - 1
	if len(ip.rows) <= visibleRows {
		t.Fatalf("test premise failed: %d logical rows fit in %d-row viewport", len(ip.rows), visibleRows)
	}
	if !infoPanelHasRow(ip, "Field 15", "Value 15") {
		t.Fatal("lower provider row was clipped out of the logical row list")
	}

	ip.setCursorToLastCopyable()
	ip.Show(scr)
	if ip.scrollTop == 0 {
		t.Fatal("End did not scroll a short Info panel")
	}
	if ip.cursor < ip.scrollTop || ip.cursor >= ip.scrollTop+visibleRows {
		t.Fatalf("cursor %d is outside viewport [%d, %d)", ip.cursor, ip.scrollTop, ip.scrollTop+visibleRows)
	}
	got := ip.rows[ip.cursor]
	if got.label != Msg("InfoPanel.CurrentDir") || got.y < ip.Y1+1 || got.y > ip.Y2-1 {
		t.Fatalf("last row after scroll = label %q, screen y %d", got.label, got.y)
	}
}

func TestInfoPanel_RendersUsageAsTwoLineMeter(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 20)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["device"] = vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID:    "status",
			Title: "Device status",
			Fields: []vfs.PanelInfoField{{
				ID: "memory", Label: "Memory", Kind: vfs.PanelInfoUsage,
				TotalBytes: 1000, AvailableBytes: 500,
			}},
		}},
	}
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 60, 20, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 59, 19)
	ip.SetFocus(true)

	oldBytes := AppConfig.InfoPanelBytes
	defer func() { AppConfig.InfoPanelBytes = oldBytes }()
	AppConfig.InfoPanelBytes = false
	ip.Show(scr)

	assertMeter := func(used, total string) {
		t.Helper()
		for i := 0; i+1 < len(ip.rows); i++ {
			first, second := ip.rows[i], ip.rows[i+1]
			if first.label != "Memory" || !first.copyable {
				continue
			}
			if second.label != "Memory" || second.copyable {
				t.Fatalf("usage continuation = %#v, want non-copyable Memory row", second)
			}
			if strings.Contains(first.text, "[") || strings.Contains(first.text, "]") || !strings.Contains(first.text, "50%") {
				t.Fatalf("meter line %q is not a bracketless 50%% progress bar", first.text)
			}
			if !strings.Contains(second.text, Msg("InfoPanel.UsedShort")) ||
				!strings.Contains(second.text, used) || !strings.Contains(second.text, total) {
				t.Fatalf("legend line %q does not contain used %q and total %q", second.text, used, total)
			}
			if !strings.Contains(first.value, used) || !strings.Contains(first.value, total) {
				t.Fatalf("copy value %q does not retain both usage values", first.value)
			}

			percentStart := strings.Index(first.text, "50%")
			filledAttr, unfilledAttr := panelInfoUsageAttrs(vtui.Palette[ColPanelCursor])
			for offset := 0; offset < len("50%"); offset++ {
				insideOffset := percentStart + offset - first.usageBarStart
				wantAttr := unfilledAttr
				if insideOffset >= 0 && insideOffset < first.usageBarFilled {
					wantAttr = filledAttr
				}
				cell := scr.GetCell(ip.X1+1+percentStart+offset, first.y)
				if cell.Attributes != wantAttr {
					t.Fatalf("percentage cell %q at bar offset %d: attr=%#x, want %#x",
						testRune(cell.Char), insideOffset, cell.Attributes, wantAttr)
				}
			}
			unfilledCell := scr.GetCell(ip.X1+1+first.usageBarStart+first.usageBarWidth-1, first.y)
			_, baseBackground := panelInfoAttrColors(vtui.Palette[ColPanelCursor])
			if got := vtui.GetRGBBack(unfilledCell.Attributes); got == baseBackground {
				t.Fatalf("unfilled bar background %#x is indistinguishable from panel background", got)
			}
			if got, filledBackground := vtui.GetRGBBack(unfilledCell.Attributes), vtui.GetRGBBack(filledAttr); got == filledBackground {
				t.Fatalf("unfilled bar background %#x is indistinguishable from filled background", got)
			}
			return
		}
		t.Fatal("two-line Memory meter not found")
	}

	assertMeter(formatBytes(500), formatBytes(1000))
	AppConfig.InfoPanelBytes = true
	ip.Show(scr)
	assertMeter(formatBytesCommas(500), formatBytesCommas(1000))
}

func TestInfoPanel_AlignsAllUsageMetersToNarrowestWidth(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(64, 24)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["device"] = vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{
			{ID: "short", Title: "Short labels", Fields: []vfs.PanelInfoField{
				{ID: "memory", Label: "RAM", Kind: vfs.PanelInfoUsage, TotalBytes: 1000, AvailableBytes: 250},
				{ID: "storage", Label: "Storage", Kind: vfs.PanelInfoUsage, TotalBytes: 2000, AvailableBytes: 1000},
			}},
			{ID: "long", Title: "Long labels", Fields: []vfs.PanelInfoField{
				{ID: "paging", Label: "Very long paging file", Kind: vfs.PanelInfoUsage, TotalBytes: 4000, AvailableBytes: 1000},
			}},
		},
	}
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 64, 24, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 63, 23)
	ip.Show(scr)

	innerW := ip.X2 - ip.X1 - 1
	width, start, count := 0, -1, 0
	for _, row := range ip.rows {
		if row.usageContinuation || row.usageMeterWidth <= 0 {
			continue
		}
		count++
		if width == 0 {
			width = row.usageMeterWidth
			start = row.usageBarStart
		}
		if row.usageMeterWidth != width || row.usageBarStart != start {
			t.Fatalf("meter %q = width %d start %d, want shared width %d start %d",
				row.label, row.usageMeterWidth, row.usageBarStart, width, start)
		}
		if row.usageBarStart+row.usageBarWidth != innerW {
			t.Fatalf("meter %q right edge = %d, want %d", row.label,
				row.usageBarStart+row.usageBarWidth, innerW)
		}
	}
	if count != 3 {
		t.Fatalf("aligned meter count = %d, want 3", count)
	}
	// The longest label determines the minimum natural width. A short label
	// must therefore leave padding before the exact same meter column.
	if row := func() infoRow {
		for _, candidate := range ip.rows {
			if candidate.label == "RAM" && !candidate.usageContinuation {
				return candidate
			}
		}
		return infoRow{}
	}(); !strings.Contains(row.text, "RAM ") || row.usageMeterWidth == 0 {
		t.Fatalf("short-label meter was not rebuilt into the shared column: %#v", row)
	}
}

func TestInfoPanel_LocalResourcesUseUsageMeters(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 60)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := newStableInfoTestPanel(0, 0, 50, 59, vfs.NewOSVFS(tmp), nil)
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 59)
	ip.Show(scr)

	if fs, ok := fsInfo(tmp); ok && fs.Total > 0 {
		if !infoPanelHasUsageMeter(ip, Msg("InfoPanel.Space")) {
			t.Fatal("local filesystem capacity was not rendered with the reusable usage meter")
		}
		if infoPanelHasRow(ip, Msg("InfoPanel.Total"), formatBytes(fs.Total)) ||
			infoPanelHasRow(ip, Msg("InfoPanel.Free"), formatBytes(fs.Free)) {
			t.Fatal("legacy filesystem total/free rows remained alongside the usage meter")
		}
	}

	if mem, ok := memInfo(); ok && mem.Total > 0 {
		if !infoPanelHasUsageMeter(ip, Msg("InfoPanel.Memory")) {
			t.Fatal("physical memory was not rendered with the reusable usage meter")
		}
		if mem.SwapTotal > 0 && !infoPanelHasUsageMeter(ip, Msg("InfoPanel.PagingFile")) {
			t.Fatal("paging space was not rendered with the reusable usage meter")
		}
	}
}

func TestInfoPanel_NarrowProviderRowsStayInsideFrame(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(20, 12)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["device"] = vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID:    "device",
			Title: "Android device with a deliberately long title",
			Fields: []vfs.PanelInfoField{{
				ID:    "long",
				Label: "A deliberately long provider label",
				Value: "value",
			}},
		}},
	}
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 10, 12, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 9, 11) // inner width is only eight cells
	ip.SetFocus(true)
	ip.Show(scr)

	innerW := ip.X2 - ip.X1 - 1
	for _, row := range ip.rows {
		if width := runewidth.StringWidth(row.text); width > innerW {
			t.Fatalf("row %q is %d cells wide, frame interior is %d", row.text, width, innerW)
		}
	}
}

func TestInfoPanel_MissingCachedCursorRowFallsBackNearby(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(50, 8)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	provider := newTestPanelInfoVFS()
	provider.cached["device"] = vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID:    "device",
			Title: "Android device",
			Fields: []vfs.PanelInfoField{
				{ID: "model", Label: "Model", Value: "SM-G930F"},
				{ID: "serial", Label: "Serial", Value: "serial"},
				{ID: "state", Label: "State", Value: "device"},
				{ID: "product", Label: "Product", Value: "herolte"},
			},
		}},
	}
	provider.fresh["device"] = true
	fsp := newStableInfoTestPanel(0, 0, 50, 7, provider,
		[]*fileEntry{{VFSItem: vfs.VFSItem{Name: "device", IsDir: true}}})
	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 49, 6)
	ip.SetFocus(true)
	ip.Show(scr)
	for i := range ip.rows {
		if ip.rows[i].label == "State" {
			ip.cursor = i
			break
		}
	}
	ip.scrollTop = 1
	ip.Show(scr)
	oldOffset := ip.cursor - ip.scrollTop

	provider.mu.Lock()
	provider.cached["device"] = vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID:    "device",
			Title: "Android device",
			Fields: []vfs.PanelInfoField{
				{ID: "model", Label: "Model", Value: "SM-G930F"},
				{ID: "serial", Label: "Serial", Value: "serial"},
				{ID: "android", Label: "Android", Value: "7.0"},
				{ID: "build", Label: "Build", Value: "NRD90M"},
				{ID: "abi", Label: "ABI", Value: "arm64-v8a"},
			},
		}},
	}
	provider.mu.Unlock()
	ip.Show(scr)

	if ip.cursor < 0 || ip.cursor >= len(ip.rows) {
		t.Fatalf("cursor became invalid after schema replacement: %d", ip.cursor)
	}
	if got := ip.rows[ip.cursor].label; got != "Android" {
		t.Fatalf("missing State row fell back to %q, want nearby Android row", got)
	}
	if got := ip.cursor - ip.scrollTop; got != oldOffset {
		t.Fatalf("cursor screen offset changed from %d to %d", oldOffset, got)
	}
}

// TestPanelsFrame_B_TogglesInfoPanelUnits verifies that `B` (plain,
// no modifiers) flips AppConfig.InfoPanelBytes while an info panel is
// visible, and falls through to fast-find otherwise.
func TestPanelsFrame_B_TogglesInfoPanelUnits(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	oldBytes := AppConfig.InfoPanelBytes
	defer func() { AppConfig.InfoPanelBytes = oldBytes }()
	AppConfig.InfoPanelBytes = false

	send := func(vk uint16) bool {
		return pressKey(pf, &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode: vk,
		})
	}

	// No info panel yet: `B` must NOT touch the config (fast-find
	// path is expected to consume it).
	send(vtinput.VK_B)
	if AppConfig.InfoPanelBytes {
		t.Errorf("without info panel: B must not flip units, got InfoPanelBytes=true")
	}

	// Install info panel on passive side, then `B` should flip units.
	pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_L,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if pf.altPanels[0] == nil {
		t.Fatal("Ctrl+L should install info panel")
	}
	send(vtinput.VK_B)
	if !AppConfig.InfoPanelBytes {
		t.Errorf("with info panel: B should flip units to bytes")
	}
	send(vtinput.VK_B)
	if AppConfig.InfoPanelBytes {
		t.Errorf("second B should flip back to human")
	}
}

// TestFormatBytes_TogglesWithConfig verifies formatBytes routes to
// commas or human based on AppConfig.InfoPanelBytes.
func TestFormatBytes_TogglesWithConfig(t *testing.T) {
	old := AppConfig.InfoPanelBytes
	defer func() { AppConfig.InfoPanelBytes = old }()

	AppConfig.InfoPanelBytes = true
	if got := formatBytes(1024); got != formatBytesCommas(1024) {
		t.Errorf("bytes-mode: got %q, want commas form", got)
	}
	AppConfig.InfoPanelBytes = false
	if got := formatBytes(1024); got != formatBytesHuman(1024) {
		t.Errorf("human-mode: got %q, want human form", got)
	}
}

// TestShortUsername verifies the Windows machine/domain prefix is
// stripped so the info panel shows just the login name.
func TestShortUsername(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sogonov", "sogonov"},                 // unix
		{"INBOOK_X2_PLUS\\sogonov", "sogonov"}, // windows local
		{"MYDOMAIN\\alice.smith", "alice.smith"},
		{"forward/slash", "slash"}, // defensive: any known separator
		{"", ""},
		{"\\", ""},
	}
	for _, c := range cases {
		if got := shortUsername(c.in); got != c.want {
			t.Errorf("shortUsername(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestInfoPanel_CursorSkipsNonCopyable makes sure Up/Down land on
// copyable rows only — never on a section header or blank line.
func TestInfoPanel_CursorSkipsNonCopyable(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr) // populates rows and seeds cursor

	if ip.cursor < 0 || ip.cursor >= len(ip.rows) {
		t.Fatalf("expected cursor to be seeded on a row, got %d of %d", ip.cursor, len(ip.rows))
	}
	if !ip.rows[ip.cursor].copyable {
		t.Errorf("cursor landed on a non-copyable row (label=%q)", ip.rows[ip.cursor].label)
	}

	seen := 0
	for i := 0; i < 200; i++ {
		prev := ip.cursor
		ip.moveCursor(+1)
		if ip.cursor == prev {
			break
		}
		if !ip.rows[ip.cursor].copyable {
			t.Fatalf("Down landed on non-copyable row (label=%q)", ip.rows[ip.cursor].label)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("cursor didn't advance at all on repeated Down")
	}
	for i := 0; i < 200; i++ {
		prev := ip.cursor
		ip.moveCursor(-1)
		if ip.cursor == prev {
			break
		}
		if !ip.rows[ip.cursor].copyable {
			t.Fatalf("Up landed on non-copyable row (label=%q)", ip.rows[ip.cursor].label)
		}
	}
}

// TestInfoPanel_CopyCopiesValue verifies that 'C' while focused
// writes the current row's value (not the label) to vtui.SetClipboard.
func TestInfoPanel_CopyCopiesValue(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)

	// Force cursor onto a known row (first copyable) and copy.
	ip.setCursorToFirstCopyable()
	if ip.cursor < 0 {
		t.Fatal("no copyable row found on default info panel — layout regression?")
	}
	wantValue := ip.rows[ip.cursor].value
	if wantValue == "" {
		t.Fatal("first copyable row has empty value; test premise broken")
	}
	ip.copyCurrent()
	if got := vtui.GetClipboard(); got != wantValue {
		t.Errorf("SetClipboard got %q, want %q", got, wantValue)
	}
	pumpUntilToastActive(t)
	waitForToastExpiry(t, 3*time.Second)
}

// TestInfoPanel_ProcessKey_UnfocusedIgnoresC verifies the C copy
// hotkey only fires when the panel is focused — otherwise it must
// fall through so the file panel's fast-find still sees it.
func TestInfoPanel_ProcessKey_UnfocusedIgnoresC(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.Show(scr)
	// Not focused.
	handled := ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_C,
	})
	if handled {
		t.Error("unfocused info panel must not consume C")
	}
}

// TestInfoPanel_ShiftUpDownSelectsAndCCopiesLabelValue checks the
// multi-row selection UX: Shift+Down toggles the current row's
// selection and moves the cursor down; a subsequent C copies every
// selected row as "label: value" per line (in on-screen order),
// with a two-line minimum so the copy joiner is actually exercised.
func TestInfoPanel_ShiftUpDownSelectsAndCCopiesLabelValue(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)
	ip.setCursorToFirstCopyable()

	// First row: Shift+Down should toggle current selection then move.
	firstLabel := ip.rows[ip.cursor].label
	firstValue := ip.rows[ip.cursor].value
	ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.ShiftPressed,
	})
	// Second row (post-move): Shift+Down again, adds it too.
	secondLabel := ip.rows[ip.cursor].label
	secondValue := ip.rows[ip.cursor].value
	ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.ShiftPressed,
	})
	firstSection := ip.rows[0].section
	// After the second Shift+Down the cursor sits on the second
	// selectable row; recover its section from the row list.
	secondSection := ""
	for _, r := range ip.rows {
		if r.label == secondLabel && r.copyable {
			secondSection = r.section
			break
		}
	}
	if !ip.selection[rowKey(firstSection, firstLabel)] ||
		!ip.selection[rowKey(secondSection, secondLabel)] {
		t.Fatalf("expected both %q and %q to be selected; got selection=%v",
			firstLabel, secondLabel, ip.selection)
	}

	// C copies both rows as label: value per line, in on-screen order.
	ip.copyCurrent()
	want := firstLabel + ": " + firstValue + "\n" + secondLabel + ": " + secondValue
	if got := vtui.GetClipboard(); got != want {
		t.Errorf("clipboard = %q, want %q", got, want)
	}
	pumpUntilToastActive(t)
	waitForToastExpiry(t, 3*time.Second)

	// Selection persists across a rebuild — the highlight must
	// survive the next Show, which walks the row list from scratch.
	ip.Show(scr)
	for _, r := range ip.rows {
		if r.label == firstLabel || r.label == secondLabel {
			if !r.selected {
				t.Errorf("row %q lost its selected flag after rebuild", r.label)
			}
		}
	}
}

// TestInfoPanel_WrapRowContinuationInheritsSelection checks that
// when a value overflows the panel width and wrapRow spills it onto
// hanging continuation lines, selecting the row highlights ALL of
// its screen lines — not just the first. The line break is a
// display artifact, not a selection boundary. Realised via
// section+label tagging of continuation rows so ip.selection lights
// every line owned by the parent label.
func TestInfoPanel_WrapRowContinuationInheritsSelection(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	// Narrow panel so any real Flags-style row spills onto at least
	// two hanging lines.
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)

	// Directly select the Flags label if a wrapped row exists.
	// If it doesn't (fs has no flags string), simulate one by
	// invoking wrapRow via a synthesised call. Simpler: iterate
	// existing rows for two consecutive rows sharing (section,
	// label) — that's a wrap. If none exist skip the test.
	var parentIdx = -1
	for i := 0; i+1 < len(ip.rows); i++ {
		r, next := ip.rows[i], ip.rows[i+1]
		if r.copyable && !next.copyable && r.label != "" &&
			next.label == r.label && next.section == r.section {
			parentIdx = i
			break
		}
	}
	if parentIdx < 0 {
		t.Skip("no wrapped row present in this environment — nothing to verify")
	}
	parent := ip.rows[parentIdx]
	ip.selection[rowKey(parent.section, parent.label)] = true
	ip.Show(scr) // triggers the restore loop

	// Every row sharing (section, label) with the parent must now
	// carry selected=true, contiguous continuation and all.
	for _, r := range ip.rows {
		if r.section == parent.section && r.label == parent.label {
			if !r.selected {
				t.Errorf("row (section=%q label=%q, text=%q) should be selected",
					r.section, r.label, r.text)
			}
		}
	}
}

// TestInfoPanel_InsTogglesSelectionAndMoves verifies Ins behaves
// like the file-panel Ins: toggle current, advance one row.
func TestInfoPanel_InsTogglesSelectionAndMoves(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)
	ip.setCursorToFirstCopyable()

	startRow := ip.rows[ip.cursor]
	startCursor := ip.cursor
	ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_INSERT,
	})
	if !ip.selection[rowKey(startRow.section, startRow.label)] {
		t.Errorf("Ins should have selected %q", startRow.label)
	}
	if ip.cursor == startCursor {
		t.Error("Ins should have advanced the cursor by one copyable row")
	}
}

// TestInfoPanel_CPUSectionRespectsOption checks that the CPU/GPU
// section is opt-in — hidden when AppConfig.InfoPanelCPUGPU is off,
// present when it's on. Guards the maintainer's off-by-default ask.
func TestInfoPanel_CPUSectionRespectsOption(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 40)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 60, 40, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	old := AppConfig.InfoPanelCPUGPU
	defer func() { AppConfig.InfoPanelCPUGPU = old }()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 59, 39)

	hasLabelPrefix := func(prefix string) bool {
		for _, r := range ip.rows {
			if strings.HasPrefix(r.label, prefix) {
				return true
			}
		}
		return false
	}

	AppConfig.InfoPanelCPUGPU = false
	ip.Show(scr)
	if hasLabelPrefix("Model") || hasLabelPrefix("Cores") {
		t.Error("CPU rows must not render when InfoPanelCPUGPU is off")
	}

	AppConfig.InfoPanelCPUGPU = true
	ip.Show(scr)
	// Cores is always populated (runtime.NumCPU seeds LogicalCores
	// on every OS). Label depends on HT — plain "Cores / threads"
	// when different, or on any i18n change the prefix "Cores"
	// still matches.
	if !hasLabelPrefix("Cores") {
		t.Error("expected CPU 'Cores' row once InfoPanelCPUGPU is enabled")
	}
}

// TestFormatBytesCommas covers the raw-bytes-with-thousand-separator
// formatter used in the info panel. Matches far2l's InsertCommas.
func TestFormatBytesCommas(t *testing.T) {
	const nbsp = " "
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1" + nbsp + "000"},
		{1234567, "1" + nbsp + "234" + nbsp + "567"},
		{8191705088, "8" + nbsp + "191" + nbsp + "705" + nbsp + "088"},
	}
	for _, c := range cases {
		if got := formatBytesCommas(c.in); got != c.want {
			t.Errorf("formatBytesCommas(%d)=%q, want %q", c.in, got, c.want)
		}
	}
}
