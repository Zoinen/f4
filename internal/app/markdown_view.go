package app

// F3 on a Markdown file shows it formatted rather than as its source (issue
// #1625). The rendering is vtui's Markdown viewer (vtui.NewMarkdownView), the
// help engine's scrolling, wrapping, link-following window fed from Markdown
// instead of a .hlf file, so f4 only adds what a file viewer needs on top:
// the whole workspace instead of a help-sized box, the file's name in the
// title, F4 to the ordinary text viewer for when the source is what the
// reader wanted after all, and, the other way, Shift+F3
// (actionSwitchViewerToMarkdown, below) from that text viewer back to the
// formatted view.
//
// It sits with the picture and video viewers behind "Open images, video and
// Markdown in their own viewers": switched off, a .md file opens as text, as
// before.

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/mdmath"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// markdownViewMaxSize is the largest file the formatted view takes. The whole
// document is parsed and wrapped in memory; past this a Markdown file is a
// data dump more than a document, and the text viewer, which pages through a
// file of any size, is the better tool for it.
const markdownViewMaxSize = 4 << 20

var errMarkdownTooLarge = errors.New("too large for the formatted Markdown view")

// isMarkdownFile says whether a name is one the formatted view is for.
func isMarkdownFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// markdownView is vtui's Markdown viewer as a file viewer of f4's.
type markdownView struct {
	*vtui.HelpView
	vfs  vfs.VFS
	path string

	// onSource opens the same file in the text viewer. F4 calls it after
	// closing this view.
	onSource func()
}

func newMarkdownView(v vfs.VFS, path string, source []byte) *markdownView {
	name := filepath.Base(path)
	if v != nil {
		name = v.Base(path)
	}
	text := strings.ReplaceAll(string(source), "\r\n", "\n")
	mv := &markdownView{HelpView: vtui.NewMarkdownView(name, mdmath.Prepare(text)), vfs: v, path: path}
	// A file viewer, not a help popup: it has a workspace screen of its own
	// and lets the reader switch away from it like any other viewer does.
	mv.Modal = false
	mv.SetTitle(" " + name + " ")
	return mv
}

// ResizeConsole takes the viewer's own place: the whole workspace above the
// key bar, rather than the centred box HelpView keeps for help.
func (mv *markdownView) ResizeConsole(w, h int) {
	top := 0
	if vtui.FrameManager != nil {
		top = vtui.FrameManager.WorkspaceTopInset()
	}
	mv.SetPosition(0, top, w-1, h-2)
}

func (mv *markdownView) ProcessKey(e *vtinput.InputEvent) bool {
	if e != nil && e.Type == vtinput.KeyEventType && e.KeyDown {
		const modifiers = vtinput.ShiftPressed | vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed |
			vtinput.LeftAltPressed | vtinput.RightAltPressed
		if e.ControlKeyState&modifiers == 0 {
			switch e.VirtualKeyCode {
			case vtinput.VK_F3, vtinput.VK_F10:
				mv.Close()
				return true
			case vtinput.VK_F4:
				// The text viewer's own F4 goes between text and hex; here
				// it goes from the formatted view to the text.
				mv.Close()
				if mv.onSource != nil {
					mv.onSource()
				}
				return true
			}
		}
	}
	return mv.HelpView.ProcessKey(e)
}

func (mv *markdownView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			"", "", i18n.Msg("KeyBar.ViewerF3"), mv.sourceLabel(),
			"", "", "", "", "", i18n.Msg("KeyBar.ViewerF10"),
		},
	}
}

// sourceLabel is F4's label: "Text" when there is a text viewer to go to, none
// for the editor's preview, where F4 only closes.
func (mv *markdownView) sourceLabel() string {
	if mv.onSource == nil {
		return ""
	}
	return i18n.Msg("Viewer.ModeText")
}

// MarkdownSearchTopic hands the topic the view shows (laid out for its
// current width) to the type-to-search of the help windows
// (dialog.HelpTopicForFrame): typing searches the formatted text.
func (mv *markdownView) MarkdownSearchTopic() *vtui.HelpTopic { return mv.CurrentTopic() }

// GetType keeps the view apart from help windows, which report TypeUser.
func (mv *markdownView) GetType() vtui.FrameType { return vtui.TypeUser + 20 }

func (mv *markdownView) GetTitle() string {
	return "View: " + filepath.Base(mv.path)
}

// readMarkdownSource reads the whole file for the formatted view, or refuses
// one too large for it.
func readMarkdownSource(ctx context.Context, v vfs.VFS, path string) ([]byte, error) {
	f, err := v.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	size := f.Size()
	if size < 0 || size > markdownViewMaxSize {
		return nil, errMarkdownTooLarge
	}
	buf := make([]byte, size)
	n, err := f.ReadAt(ctx, buf, 0)
	if err != nil && (!errors.Is(err, io.EOF) || int64(n) != size) {
		return nil, err
	}
	return buf[:n], nil
}

// tryOpenMarkdownViewer opens a Markdown file formatted. A file it cannot
// show that way (unreadable, too large) goes to the text viewer, which has
// its own, better answers for those.
func tryOpenMarkdownViewer(pf *panel.PanelsFrame, v vfs.VFS, path string) bool {
	if pf == nil || v == nil || !isMarkdownFile(path) {
		return false
	}
	var source []byte
	pf.RunProgressTaskAfter(openingProgressDelay, " Opening... ", "Preparing to open file...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		update("Opening file...", -1)
		ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(update))
		var err error
		source, err = readMarkdownSource(ctx, v, path)
		return err
	}, func(err error) {
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			vtui.DebugLog("MARKDOWN: %s opens as text: %v", path, err)
			openPlainViewer(pf, v, path, false)
			return
		}
		showMarkdownView(pf, v, path, source)
	})
	return true
}

func showMarkdownView(pf *panel.PanelsFrame, v vfs.VFS, path string, source []byte) {
	mv := newMarkdownView(v, path, source)
	mv.onSource = func() { openPlainViewer(pf, v, path, false) }
	mv.ResizeConsole(pf.LastW, pf.LastH)
	vtui.FrameManager.AddScreen(mv)
}

// actionSwitchViewerToMarkdown is the reverse of markdownView's own F4
// (onSource, above): from the plain text/hex viewer on a Markdown file, go
// back to the formatted view. Bound to Shift+F3 (Viewer.MarkdownFormatted) --
// plain F4 is already Viewer.HexMode there (f4#1625 step 2).
func actionSwitchViewerToMarkdown(vv *viewer.ViewerView) {
	if vv == nil || vv.Path == "" || vv.VFS == nil || !isMarkdownFile(vv.Path) {
		return
	}
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return
	}
	v, path := vv.VFS, vv.Path
	vv.Close()
	tryOpenMarkdownViewer(pf, v, path)
}

// actionEditorMarkdownPreview shows what the editor holds -- unsaved changes
// included -- formatted, in a window of its own over the editor (f4#1625
// step 3). It is a snapshot of the text at the moment of the call: close it
// (F3/F10/Esc) and the editor is as it was; press the key again to see later
// edits. Bound to Shift+F3 (Editor.MarkdownPreview).
func actionEditorMarkdownPreview(ev *editor.EditorView) {
	if ev == nil || ev.Pt == nil || !isMarkdownFile(ev.FilePath) {
		return
	}
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return
	}
	text := ev.GetText()
	if len(text) > markdownViewMaxSize {
		vtui.ShowMessage(" Markdown ", "The text is too large for the formatted Markdown view.", []string{"&Ok"})
		return
	}
	mv := newMarkdownView(ev.Vfs, ev.FilePath, []byte(text))
	mv.ResizeConsole(pf.LastW, pf.LastH)
	vtui.FrameManager.AddScreen(mv)
}
