package viewer

import (
	"context"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectSearchMatchPreservesVisibleRows(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		wrap, hex  bool
		top, match int64
		length     int
		wantTop    int64
	}{
		{"forward", strings.Repeat("line\n", 40), false, false, 10, 20, 4, 10},
		{"backward", strings.Repeat("line\n", 40), false, false, 10, 10, 4, 10},
		{"below", strings.Repeat("line\n", 40), false, false, 10, 50, 4, 50},
		{"above", strings.Repeat("line\n", 40), false, false, 10, 0, 4, 0},
		{"wrapped", strings.Repeat("x", 400), true, false, 20, 55, 4, 20},
		{"wrapped-below", strings.Repeat("x", 400), true, false, 20, 115, 4, 110},
		{"hex", strings.Repeat("x", 400), false, true, 32, 63, 2, 32},
		{"hex-below", strings.Repeat("x", 400), false, true, 32, 120, 2, 112},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(testutil.SwapFrameManager(t))
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			path := filepath.Join(t.TempDir(), "match.txt")
			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(filepath.Dir(path)), path)
			if err != nil {
				t.Fatal(err)
			}
			defer vv.Close()
			vv.SetPosition(0, 0, 79, 24)
			vv.NativeViewportRows = 5
			vv.NativeViewportColumns = 10
			vv.WrapMode, vv.HexMode, vv.TopOffset = tc.wrap, tc.hex, tc.top
			vv.SelectSearchMatch(tc.match, tc.length)
			if vv.TopOffset != tc.wantTop {
				t.Fatalf("top=%d want %d", vv.TopOffset, tc.wantTop)
			}
			if !vv.LastSearchFound || vv.LastSearchOffset != tc.match || vv.LastSearchTopOffset != vv.TopOffset || vv.LastSearchMatchLen != int64(tc.length) {
				t.Fatal("match state was not updated")
			}
		})
	}
}
