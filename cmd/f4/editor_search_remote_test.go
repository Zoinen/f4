package main

import (
	"context"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// searchableVFS is a file system that answers searches itself, the way FISH+
// does with a remote grep: fixed strings, case folded, and capped — so what it
// reports is a superset of some searches and short of others.
type searchableVFS struct {
	vfs.VFS
	content string
	// truncateAt drops everything past this many matches, standing in for the
	// host-side limit.
	truncateAt int
	calls      int
}

func (s *searchableVFS) GetCapabilities() vfs.VFSCapabilities {
	caps := s.VFS.GetCapabilities()
	caps.HasSearch = true
	return caps
}

func (s *searchableVFS) Search(ctx context.Context, path, pattern string) (chan int64, error) {
	s.calls++
	lowerContent := strings.ToLower(s.content)
	lowerPattern := strings.ToLower(pattern)

	var offsets []int64
	for i := 0; ; {
		idx := strings.Index(lowerContent[i:], lowerPattern)
		if idx < 0 {
			break
		}
		offsets = append(offsets, int64(i+idx))
		i += idx + 1
	}
	if s.truncateAt > 0 && len(offsets) > s.truncateAt {
		offsets = offsets[:s.truncateAt]
	}

	out := make(chan int64, len(offsets))
	for _, off := range offsets {
		out <- off
	}
	close(out)
	return out, nil
}

func newSearchableEditor(t *testing.T, content string) (*EditorView, *searchableVFS) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	fsys := &searchableVFS{VFS: vfs.NewOSVFS(t.TempDir()), content: content}
	pt := piecetable.New([]byte(content))
	ev := newEditorView(pt, fsys, "remote.txt", false, true)
	ev.Codepage = 65001
	return ev, fsys
}

func TestSearchDelegation_UsesTheFileSystemAndConfirmsLocally(t *testing.T) {
	content := "alpha beta\nGAMMA gamma\ndelta\n"
	ev, fsys := newSearchableEditor(t, content)

	d, ok := ev.searchDelegation(false, false)
	if !ok {
		t.Fatal("an unedited UTF-8 buffer on a searchable file system should delegate")
	}

	// Case-insensitive: the host's own semantics, so the first hit stands.
	off, mLen, handled := ev.searchViaFileSystem(context.Background(), d, "gamma", false, false, false, 0)
	if !handled {
		t.Fatal("search was not handled by the file system")
	}
	if want := strings.Index(content, "GAMMA"); off != want {
		t.Errorf("offset = %d, want %d", off, want)
	}
	if mLen != len("gamma") {
		t.Errorf("length = %d, want %d", mLen, len("gamma"))
	}

	// Case-sensitive: the host still reports both, and the folded one has to
	// be rejected here rather than jumped to.
	off, _, handled = ev.searchViaFileSystem(context.Background(), d, "gamma", true, false, false, 0)
	if !handled {
		t.Fatal("case-sensitive search was not handled")
	}
	if want := strings.LastIndex(content, "gamma"); off != want {
		t.Errorf("case-sensitive offset = %d, want %d", off, want)
	}
	if got := content[off : off+len("gamma")]; got != "gamma" {
		t.Errorf("case-sensitive search landed on %q, want the lowercase run", got)
	}
	if fsys.calls == 0 {
		t.Error("the file system was never asked")
	}
}

func TestSearchDelegation_DirectionAndNext(t *testing.T) {
	content := "one match here, another match there, a third match at the end\n"
	ev, _ := newSearchableEditor(t, content)
	d, _ := ev.searchDelegation(false, false)

	first := strings.Index(content, "match")
	second := strings.Index(content[first+1:], "match") + first + 1
	third := strings.LastIndex(content, "match")

	off, _, ok := ev.searchViaFileSystem(context.Background(), d, "match", false, false, false, 0)
	if !ok || off != first {
		t.Errorf("forward from 0 = %d (handled=%v), want %d", off, ok, first)
	}
	off, _, ok = ev.searchViaFileSystem(context.Background(), d, "match", false, false, true, first)
	if !ok || off != second {
		t.Errorf("forward next from %d = %d (handled=%v), want %d", first, off, ok, second)
	}
	off, _, ok = ev.searchViaFileSystem(context.Background(), d, "match", false, true, true, third)
	if !ok || off != second {
		t.Errorf("backward next from %d = %d (handled=%v), want %d", third, off, ok, second)
	}
}

// TestSearchDelegation_FallsBackWhenTheAnswerCouldBeShort is the one that keeps
// a capped host-side search honest: a list that stops early can only miss
// matches, so finding nothing usable in it has to mean "scan the buffer", never
// "not in the file".
func TestSearchDelegation_FallsBackWhenTheAnswerCouldBeShort(t *testing.T) {
	content := strings.Repeat("needle\n", 50)
	ev, fsys := newSearchableEditor(t, content)
	fsys.truncateAt = 3 // the host reports only the first three
	d, _ := ev.searchDelegation(false, false)

	// Searching past the reported matches must not answer "not found".
	after := strings.Index(content, "needle") + 10*len("needle\n")
	if _, _, ok := ev.searchViaFileSystem(context.Background(), d, "needle", false, false, false, after); ok {
		t.Error("a truncated list was treated as the whole truth")
	}

	// And the local scan, which is what the caller falls back to, does find it.
	off, _, err := findMatch([]byte(content), "needle", false, false, false, false, false, after)
	if err != nil || off < after {
		t.Errorf("local scan from %d found %d (err=%v)", after, off, err)
	}
}

func TestSearchDelegation_DeclinesWhatItCannotExpress(t *testing.T) {
	content := "some text with a word in it\n"

	t.Run("regular expression", func(t *testing.T) {
		ev, _ := newSearchableEditor(t, content)
		if _, ok := ev.searchDelegation(true, false); ok {
			t.Error("delegated a regex search to a fixed-string grep")
		}
	})

	t.Run("whole word", func(t *testing.T) {
		ev, _ := newSearchableEditor(t, content)
		if _, ok := ev.searchDelegation(false, true); ok {
			t.Error("delegated a whole-word search")
		}
	})

	t.Run("edited buffer", func(t *testing.T) {
		ev, _ := newSearchableEditor(t, content)
		ev.pt.Insert(0, []byte("typed "))
		if _, ok := ev.searchDelegation(false, false); ok {
			t.Error("delegated while the buffer no longer matches the file")
		}
	})

	t.Run("decoded codepage", func(t *testing.T) {
		ev, _ := newSearchableEditor(t, content)
		ev.Codepage = 1251
		if _, ok := ev.searchDelegation(false, false); ok {
			t.Error("delegated for a buffer whose offsets are not the file's")
		}
	})

	t.Run("file system without the capability", func(t *testing.T) {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
		ev := newEditorView(piecetable.New([]byte(content)), vfs.NewOSVFS(t.TempDir()), "local.txt", false, true)
		ev.Codepage = 65001
		if _, ok := ev.searchDelegation(false, false); ok {
			t.Error("delegated to a file system that does not search")
		}
	})
}
