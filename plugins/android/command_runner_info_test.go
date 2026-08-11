package androidfs

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestSyncCommandRunnerInfo(t *testing.T) {
	info := (&SyncVFS{}).CommandRunnerInfo()
	if info.Dialect != vfs.CommandDialectPOSIX || info.MaxParallel != 4 {
		t.Fatalf("runner info = %+v", info)
	}
}
