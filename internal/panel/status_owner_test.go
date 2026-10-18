package panel

import (
	"os"
	"os/user"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4#1962 (fromgate): the focused-file status row shows who owns the file,
// "owner group" between the size (or the link target) and the date, as far2l
// does; a file system that does not report the owner keeps the old row.
func TestStatusRowShowsOwnerAndGroup_Issue1962(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.ShowPanelFileInfo = true

	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 25)
	vtui.FrameManager.Init(scr)

	uid, gid := os.Getuid(), os.Getgid()
	if uid < 0 {
		t.Skip("no Unix owners on this platform")
	}
	owner, group := strconv.Itoa(uid), strconv.Itoa(gid)
	if u, err := user.LookupId(owner); err == nil {
		owner = u.Username
	}
	if g, err := user.LookupGroupId(group); err == nil {
		group = g.Name
	}
	when := time.Date(2025, 11, 6, 12, 25, 0, 0, time.Local)

	fp := NewFileSystemPanel(0, 0, 90, 20, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fp)
	fp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "owned.txt", Size: 42, SizeKnown: true, MTime: when, Uid: uid, Gid: gid,
			KnownMetadata: vfs.MetadataExplicit | vfs.MetadataUID | vfs.MetadataGID | vfs.MetadataMTime}},
		{VFSItem: vfs.VFSItem{Name: "plain.txt", Size: 42, SizeKnown: true, MTime: when}},
	}
	fp.IsLoading = false
	if fp.LoadingTimer != nil {
		fp.LoadingTimer.Stop()
	}
	fp.Refresh()
	status := func(row int) string {
		fp.SetCursorIndex(row)
		fp.Show(scr)
		return strings.TrimSpace(testutil.ScreenRow(scr, fp.Y2-1, fp.X1+1, fp.X2-1))
	}

	want := owner + " " + group + "  06.11.25 12:25"
	if got := status(0); !strings.HasSuffix(got, want) || !strings.HasPrefix(got, "owned.txt") {
		t.Errorf("status row %q: want it to end with %q", got, want)
	}
	if got := status(1); !strings.HasSuffix(got, "42  06.11.25 12:25") || strings.Contains(got, owner+" "+group) {
		t.Errorf("status row without an owner %q: want size and date only", got)
	}
}
