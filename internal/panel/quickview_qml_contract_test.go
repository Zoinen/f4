package panel

import (
	bytes "bytes"
	context "context"
	base64 "encoding/base64"
	fmt "fmt"
	i18n "github.com/unxed/f4/internal/i18n"
	semantic "github.com/unxed/f4/internal/semantic"
	vfs "github.com/unxed/f4/vfs"
	vtinput "github.com/unxed/vtinput"
	vtui "github.com/unxed/vtui"
	png "image/png"
	os "os"
	filepath "path/filepath"
	strings "strings"
	testing "testing"
	time "time"
)

type unknownSemanticAltPanel struct {
	vtui.ScreenObject
	source  *FileSystemPanel
	focused bool
}

func (*unknownSemanticAltPanel) Show(*vtui.ScreenBuf) {}

func (*unknownSemanticAltPanel) ProcessKey(*vtinput.InputEvent) bool { return false }

func (*unknownSemanticAltPanel) ProcessMouse(*vtinput.InputEvent) bool { return false }

func (p *unknownSemanticAltPanel) SetFocus(focused bool) { p.focused = focused }

func (p *unknownSemanticAltPanel) IsFocused() bool { return p.focused }

func (*unknownSemanticAltPanel) GetSelectedName() string { return "" }

func (p *unknownSemanticAltPanel) Source() *FileSystemPanel { return p.source }

func (*unknownSemanticAltPanel) Kind() string { return "contract_unknown" }

func quickViewContractScreen(t *testing.T) {
	t.Helper()
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	vtui.SetDefaultPalette()
}

func quickViewContractFixture(t *testing.T, directory string, entries ...*FileEntry) (*QuickViewPanel, *FileSystemPanel) {
	t.Helper()
	quickViewContractScreen(t)
	panel := &FileSystemPanel{
		Vfs:     vfs.NewOSVFS(directory),
		Entries: entries,
	}
	panel.ScreenObject.SetPosition(0, 0, 39, 24)
	quick := NewQuickViewPanel(panel)
	quick.SetPosition(0, 0, 39, 24)
	t.Cleanup(quick.Close)
	return quick, panel
}

func quickViewContractRows(prefix string, count int) []byte {
	var content strings.Builder
	for row := 0; row < count; row++ {
		fmt.Fprintf(&content, "%s-%03d\n", prefix, row)
	}
	return []byte(content.String())
}

func quickViewContractRowText(rows []map[string]any) string {
	var text strings.Builder
	for _, row := range rows {
		text.WriteString(semantic.String(row["text"]))
		text.WriteByte('\n')
	}
	return text.String()
}

func TestQuickViewSemanticQMLContract_NativeKindDoesNotForceFallback(t *testing.T) {
	frame := &PanelsFrame{ShowPanels: true, ShowLeftPanel: true, ShowRightPanel: true}
	frame.AltPanels[0] = &QuickViewPanel{}
	if reason := frame.semanticGridFallbackReason(); reason != "" {
		t.Fatalf("native Quick View unexpectedly requested the raster fallback: %q", reason)
	}

	frame.AltPanels[0] = &unknownSemanticAltPanel{}
	if reason := frame.semanticGridFallbackReason(); !strings.Contains(reason, "contract_unknown") {
		t.Fatalf("unknown alternate panel fallback reason = %q", reason)
	}
}

func TestQuickViewSemanticQMLContract_TextWindowIsBoundedToOverscan(t *testing.T) {
	directory := t.TempDir()
	content := quickViewContractRows("contract", 160)
	name := "many-lines.txt"
	if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
		t.Fatal(err)
	}
	quick, _ := quickViewContractFixture(t, directory,
		&FileEntry{VFSItem: vfs.VFSItem{Name: name, Size: int64(len(content))}})

	// Pick a middle viewport so the model has both the leading and trailing
	// full-screen buffers and therefore exercises the complete 3-screen window.
	_ = quick.semanticModel(1, 0, false) // establish the selected content first
	quick.ScrollY = 70
	model := quick.semanticModel(1, 0, false)
	if model.PreviewKind != "text" || model.ContentKey == "" {
		t.Fatalf("text model kind/key = %q/%q", model.PreviewKind, model.ContentKey)
	}
	if model.Surface.Kind != "quick_view" || model.Surface.ScrollUnit != "rows" ||
		model.Surface.ScrollAction != "quickView.scroll" ||
		model.Surface.DocumentKey != model.ContentKey {
		t.Fatalf("unexpected nested surface contract: %+v", model.Surface)
	}

	viewport := model.Surface.ViewportRows
	buffer := semantic.SemanticWindowBufferRows(viewport)
	wantWindowRows := viewport + 2*buffer
	if len(model.Surface.WindowRows) != wantWindowRows {
		t.Fatalf("window rows = %d, want one viewport + two buffers = %d (%d + 2*%d)",
			len(model.Surface.WindowRows), wantWindowRows, viewport, buffer)
	}
	if len(model.Surface.Rows) != viewport {
		t.Fatalf("canonical visible rows = %d, want %d", len(model.Surface.Rows), viewport)
	}
	if model.Surface.WindowStart != model.Surface.ViewportStart-int64(buffer) ||
		model.Surface.WindowEnd != model.Surface.ViewportStart+int64(viewport+buffer) {
		t.Fatalf("window [%d,%d) does not surround viewport start %d by %d rows",
			model.Surface.WindowStart, model.Surface.WindowEnd,
			model.Surface.ViewportStart, buffer)
	}
	if model.Surface.ContentExtent != 160 || !model.Surface.ContentExtentKnown {
		t.Fatalf("content extent = %d known=%t, want 160/true",
			model.Surface.ContentExtent, model.Surface.ContentExtentKnown)
	}
	if first := model.Surface.WindowRows[0]; int64(first.VisualRow) != model.Surface.WindowStart {
		t.Fatalf("first visual row = %d, windowStart = %d", first.VisualRow, model.Surface.WindowStart)
	}
}

func TestQuickViewSemanticQMLContract_ExportsHexDirectoryErrorAndImage(t *testing.T) {
	t.Run("hex", func(t *testing.T) {
		directory := t.TempDir()
		data := []byte{'A', 'B', 0, 'C', 0xff}
		name := "binary.bin"
		if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		quick, _ := quickViewContractFixture(t, directory,
			&FileEntry{VFSItem: vfs.VFSItem{Name: name, Size: int64(len(data))}})
		model := quick.semanticModel(1, 0, false)
		if model.PreviewKind != "hex" || model.Surface.Mode != "hex" {
			t.Fatalf("binary model kind/mode = %q/%q", model.PreviewKind, model.Surface.Mode)
		}
		var rendered strings.Builder
		for _, row := range model.Surface.WindowRows {
			rendered.WriteString(row.Text)
		}
		if !strings.Contains(rendered.String(), "00000000") || !strings.Contains(rendered.String(), "41 42 00 43 FF") {
			t.Fatalf("hex rows did not export the dump: %q", rendered.String())
		}
	})

	t.Run("directory", func(t *testing.T) {
		directory := t.TempDir()
		child := filepath.Join(directory, "folder")
		if err := os.Mkdir(child, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(child, "inside.txt"), []byte("payload"), 0o600); err != nil {
			t.Fatal(err)
		}
		quick, _ := quickViewContractFixture(t, directory,
			&FileEntry{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true}})
		initial := quick.semanticModel(1, 0, false)
		if initial.PreviewKind != "directory" || initial.Name != "folder" {
			t.Fatalf("directory model = kind %q name %q", initial.PreviewKind, initial.Name)
		}

		quick.scanMu.Lock()
		done := quick.scanDoneCh
		quick.scanMu.Unlock()
		if done == nil {
			t.Fatal("semantic directory preview did not start its bounded background scan")
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("directory preview scan did not finish")
		}

		model := quick.semanticModel(1, 0, false)
		if model.Loading || model.Error != "" || model.Surface.ContentExtent == 0 {
			t.Fatalf("completed directory model = loading=%t error=%q extent=%d",
				model.Loading, model.Error, model.Surface.ContentExtent)
		}
		var rendered strings.Builder
		for _, row := range model.Surface.WindowRows {
			rendered.WriteString(row.Text)
			rendered.WriteByte('\n')
		}
		if !strings.Contains(rendered.String(), "1") {
			t.Fatalf("directory statistics were not exported: %q", rendered.String())
		}
	})

	t.Run("error", func(t *testing.T) {
		directory := t.TempDir()
		quick, _ := quickViewContractFixture(t, directory,
			&FileEntry{VFSItem: vfs.VFSItem{Name: "missing.txt", Size: 19}})
		model := quick.semanticModel(1, 0, false)
		if model.PreviewKind != "error" || model.Error == "" {
			t.Fatalf("missing file model = kind %q error %q", model.PreviewKind, model.Error)
		}
		mapped := make([]map[string]any, 0, len(model.HeaderRows))
		for _, row := range model.HeaderRows {
			mapped = append(mapped, row.ToMap())
		}
		if !strings.Contains(quickViewContractRowText(mapped), i18n.Msg("QuickView.ReadError")) {
			t.Fatalf("error header rows = %#v", mapped)
		}
	})

	t.Run("image_data_url", func(t *testing.T) {
		directory := t.TempDir()
		name := "pixel.qoi"
		qoi := []byte{
			'q', 'o', 'i', 'f',
			0, 0, 0, 1,
			0, 0, 0, 1,
			4, 0,
			0xff, 0xff, 0x00, 0x00, 0xff,
		}
		if err := os.WriteFile(filepath.Join(directory, name), qoi, 0o600); err != nil {
			t.Fatal(err)
		}
		quick, _ := quickViewContractFixture(t, directory,
			&FileEntry{VFSItem: vfs.VFSItem{Name: name, Size: int64(len(qoi))}})
		model := quick.semanticModel(1, 0, false)
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		for model.Loading && quick.cacheReadErr == nil {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
				model = quick.semanticModel(1, 0, false)
			case <-time.After(time.Millisecond):
				model = quick.semanticModel(1, 0, false)
			case <-deadline.C:
				t.Fatal("timed out waiting for semantic image preview")
			}
		}
		if model.PreviewKind != "image" || model.Loading ||
			model.ImageWidth != 1 || model.ImageHeight != 1 {
			t.Fatalf("image model = kind=%q loading=%t dimensions=%dx%d",
				model.PreviewKind, model.Loading, model.ImageWidth, model.ImageHeight)
		}
		const prefix = "data:image/png;base64,"
		if !strings.HasPrefix(model.ImageSource, prefix) {
			t.Fatalf("image source is not a bounded PNG data URL: %.48q", model.ImageSource)
		}
		encoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(model.ImageSource, prefix))
		if err != nil {
			t.Fatalf("decode image data URL: %v", err)
		}
		configuration, err := png.DecodeConfig(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("decode semantic PNG: %v", err)
		}
		if configuration.Width != model.ImageWidth || configuration.Height != model.ImageHeight {
			t.Fatalf("PNG dimensions %dx%d differ from model %dx%d",
				configuration.Width, configuration.Height, model.ImageWidth, model.ImageHeight)
		}
	})
}

func TestQuickViewSemanticQMLContract_ContentIdentityGuardsScrollRequests(t *testing.T) {
	directory := t.TempDir()
	first := quickViewContractRows("first", 100)
	second := quickViewContractRows("second", 140)
	for name, content := range map[string][]byte{"first.txt": first, "second.txt": second} {
		if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	quick, panel := quickViewContractFixture(t, directory,
		&FileEntry{VFSItem: vfs.VFSItem{Name: "first.txt", Size: int64(len(first))}},
		&FileEntry{VFSItem: vfs.VFSItem{Name: "second.txt", Size: int64(len(second))}},
	)
	router := &PanelsFrame{}
	router.AltPanels[1] = quick

	firstModel := quick.semanticModel(1, 0, false)
	quick.ScrollY, quick.scrollX = 23, 7
	panel.CursorIdx = 1
	secondModel := quick.semanticModel(1, 0, false)
	if firstModel.ContentKey == secondModel.ContentKey || secondModel.ContentKey == "" {
		t.Fatalf("selection change did not replace content key: %q -> %q",
			firstModel.ContentKey, secondModel.ContentKey)
	}
	if secondModel.Name != "second.txt" || quick.ScrollY != 0 || quick.scrollX != 0 ||
		secondModel.Surface.ViewportStart != 0 {
		t.Fatalf("selection transition did not reset viewport: name=%q x=%d y=%d start=%d",
			secondModel.Name, quick.scrollX, quick.ScrollY, secondModel.Surface.ViewportStart)
	}

	initialGeneration := quick.semanticWindowGeneration
	if router.HandleSemanticAction(map[string]any{
		"target": "stale-target", "action": "quickView.scroll",
		"contentKey": secondModel.ContentKey, "visualRow": 30,
	}) {
		t.Fatal("scroll with a stale target was accepted")
	}
	if router.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(quick), "action": "quickView.scroll",
		"contentKey": firstModel.ContentKey, "visualRow": 30,
	}) {
		t.Fatal("scroll for previously selected content was accepted")
	}
	if quick.semanticWindowGeneration != initialGeneration || quick.ScrollY != 0 {
		t.Fatalf("rejected actions mutated viewport: generation=%d y=%d",
			quick.semanticWindowGeneration, quick.ScrollY)
	}

	request := map[string]any{
		"target": vtui.SemanticID(quick), "action": "quickView.scroll",
		"contentKey": secondModel.ContentKey, "visualRow": int64(1 << 30),
	}
	if !router.HandleSemanticAction(request) {
		t.Fatal("current target/contentKey scroll was rejected")
	}
	if quick.semanticWindowGeneration != initialGeneration+1 {
		t.Fatalf("generation after accepted scroll = %d, want %d",
			quick.semanticWindowGeneration, initialGeneration+1)
	}
	clamped := quick.semanticModel(1, 0, false)
	wantTop := clamped.Surface.ContentExtent - int64(clamped.Surface.ViewportRows)
	if clamped.Surface.ViewportStart != wantTop {
		t.Fatalf("large scroll clamped to %d, want %d",
			clamped.Surface.ViewportStart, wantTop)
	}

	// A repeated request at the already-clamped endpoint is still an ACK. The
	// native viewport uses this generation to release an in-flight request.
	generationAtEnd := quick.semanticWindowGeneration
	if !router.HandleSemanticAction(request) {
		t.Fatal("clamped no-op scroll was rejected")
	}
	if quick.semanticWindowGeneration != generationAtEnd+1 || int64(quick.ScrollY) != wantTop {
		t.Fatalf("no-op ACK = generation %d y %d, want %d/%d",
			quick.semanticWindowGeneration, quick.ScrollY, generationAtEnd+1, wantTop)
	}
}

func TestQuickViewSemanticQMLContract_ToggleReplacementPreservesUnderlyingPanelAndCancels(t *testing.T) {
	quickViewContractScreen(t)
	frame := setupMockPanelsFrame(t)
	t.Cleanup(frame.Close)
	frame.ResizeConsole(80, 25)

	underlying := frame.Panels[0].(*FileSystemPanel)
	// Keep the requested TopPos valid across a resize; an undersized catalog
	// is legitimately clamped to zero by FileSystemPanel itself.
	underlying.Entries = make([]*FileEntry, 100)
	for index := range underlying.Entries {
		underlying.Entries[index] = &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("left-%02d", index)}}
	}
	underlying.Refresh()
	underlying.SetCursorIndex(15)
	underlying.Table.TopPos = 10
	wantCursor, wantTop := underlying.CursorIdx, underlying.Table.TopPos

	directory := t.TempDir()
	name := "pending.qv-contract-replacement"
	if err := os.WriteFile(filepath.Join(directory, name), []byte("provider payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := frame.Panels[1].(*FileSystemPanel)
	if source.CancelLoad != nil {
		source.CancelLoad()
		source.CancelLoad = nil
	}
	source.Vfs = vfs.NewOSVFS(directory)
	source.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: name, Size: 16}}}
	source.CursorIdx = 0
	source.IsLoading = false

	started := make(chan struct{})
	cancelled := make(chan struct{})
	registerQuickViewTestProvider(t, &quickViewTestProvider{
		name:     "qml-contract-replacement-cancel",
		priority: 20_000,
		match: func(request vfs.QuickViewRequest) bool {
			return strings.HasSuffix(request.Path, ".qv-contract-replacement")
		},
		preview: func(ctx context.Context, _ vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return vfs.QuickViewResult{}, ctx.Err()
		},
	})

	frame.ToggleAltPanel("quick_view", func(source *FileSystemPanel) AltPanel {
		return NewQuickViewPanel(source)
	})
	quick, ok := frame.AltPanels[0].(*QuickViewPanel)
	if !ok || quick.Source() != source {
		t.Fatalf("Ctrl+Q replacement = %T source=%p, want QuickView/%p",
			frame.AltPanels[0], quick.Source(), source)
	}
	loading := quick.semanticModel(0, 1, false)
	if loading.PreviewKind != "loading" || !loading.Loading {
		t.Fatalf("provider did not enter semantic loading state: %+v", loading)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Quick View provider did not start")
	}

	// Ctrl+L is the real replacement path: it must Close the QuickView before
	// installing InfoPanel in the same passive slot.
	frame.ToggleAltPanel("info", func(source *FileSystemPanel) AltPanel {
		return NewInfoPanel(source)
	})
	if _, ok := frame.AltPanels[0].(*InfoPanel); !ok {
		t.Fatalf("Quick View replacement = %T, want InfoPanel", frame.AltPanels[0])
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("replacing Quick View did not cancel its provider")
	}

	assertUnderlying := func(stage string) {
		t.Helper()
		if frame.Panels[0] != underlying {
			t.Fatalf("%s replaced underlying FileSystemPanel: %p != %p", stage, frame.Panels[0], underlying)
		}
		if underlying.CursorIdx != wantCursor || underlying.Table.TopPos != wantTop {
			t.Fatalf("%s changed underlying state: cursor=%d top=%d",
				stage, underlying.CursorIdx, underlying.Table.TopPos)
		}
	}
	assertUnderlying("replacement")

	frame.ToggleAltPanel("quick_view", func(source *FileSystemPanel) AltPanel {
		return NewQuickViewPanel(source)
	})
	if _, ok := frame.AltPanels[0].(*QuickViewPanel); !ok {
		t.Fatalf("reopening Quick View replaced passive slot with %T", frame.AltPanels[0])
	}
	frame.ToggleAltPanel("quick_view", func(source *FileSystemPanel) AltPanel {
		return NewQuickViewPanel(source)
	})
	if frame.AltPanels[0] != nil {
		t.Fatalf("second Ctrl+Q left alternate panel %T installed", frame.AltPanels[0])
	}
	assertUnderlying("toggle off")
	if frame.ActiveIdx != 1 {
		t.Fatalf("alternate panel lifecycle changed active side to %d", frame.ActiveIdx)
	}
}
