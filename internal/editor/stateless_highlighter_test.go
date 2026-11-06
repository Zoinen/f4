package editor

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
	"testing"
)

type independentTestHighlighter struct{ stubHighlighter }

func (*independentTestHighlighter) StateIndependent() bool { return true }

func TestStatelessHighlighterSkipsStateWalk(t *testing.T) {
	var independent vtui.Highlighter = &independentTestHighlighter{}
	if usesStateChain(independent) {
		t.Fatal("stateless lexer scheduled unnecessary state-chain work")
	}
	if !usesStateChain(&stubHighlighter{}) {
		t.Fatal("legacy stateful lexer lost its state chain")
	}
	ev := NewEditorView(piecetable.New([]byte("first\nsecond\n")), nil, "")
	defer ev.Close()
	ev.Highlighter = independent
	ev.startHighlighting()
	if ev.highlighting || len(ev.lineStates) != 0 {
		t.Fatal("stateless lexer started an indexing walker that disables styled-row reuse")
	}
}
