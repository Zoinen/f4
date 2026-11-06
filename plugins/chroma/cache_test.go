package chroma

import (
	"fmt"
	syntax "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"testing"
)

type countingLexer struct {
	syntax.Lexer
	calls int
}

func (l *countingLexer) Tokenise(options *syntax.TokeniseOptions, text string) (syntax.Iterator, error) {
	l.calls++
	return l.Lexer.Tokenise(options, text)
}

func TestLineTokenCacheReusesLexerAndOwnsAttributes(t *testing.T) {
	applyStyle(t, "Modern")
	lexer := &countingLexer{Lexer: lexers.Get("python")}
	h := &ChromaHighlighter{lexer: lexer}
	first, _ := h.Highlight("x = 1\n", nil, 0)
	if len(first) == 0 {
		t.Fatal("missing highlighting")
	}
	expected := first[0]
	first[0] = 123
	next, _ := h.Highlight("x = 1\n", nil, 0)
	if lexer.calls != 1 || next[0] != expected {
		t.Fatal("cached line reparsed or shared mutable attributes")
	}
	for i := 0; i < 2100; i++ {
		h.Highlight(fmt.Sprintf("value_%d = %d\n", i, i), nil, 0)
	}
	if h.order.Len() > 2048 || h.cacheBytes > 4*1024*1024 {
		t.Fatal("cache exceeded budget")
	}
	h.Highlight("x = 1\n", nil, 0)
	if lexer.calls != 2102 {
		t.Fatal("evicted line was not parsed again")
	}
}

func TestCachedLineTokensUseCurrentTheme(t *testing.T) {
	applyStyle(t, "Modern")
	lexer := &countingLexer{Lexer: lexers.Get("python")}
	h := &ChromaHighlighter{lexer: lexer}
	before, _ := h.Highlight("def example():\n", nil, 0)
	applyStyle(t, "Radiola")
	after, _ := h.Highlight("def example():\n", nil, 0)
	if lexer.calls != 1 {
		t.Fatal("theme change reparsed cached tokens")
	}
	same := true
	for i := range before {
		if before[i] != after[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("cached tokens retained old theme attributes")
	}
}
