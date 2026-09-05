//go:build darwin || linux

package archive

import (
	"context"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func openArchiveTestDescriptors(t *testing.T, path string) []int {
	t.Helper()

	targetInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	targetStat := targetInfo.Sys().(*syscall.Stat_t)

	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatalf("read process file-descriptor limit: %v", err)
	}
	const maxDescriptorScan = 1 << 16
	if limit.Cur > maxDescriptorScan {
		limit.Cur = maxDescriptorScan
	}

	var matches []int
	for fd := 0; uint64(fd) < limit.Cur; fd++ {
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			continue
		}
		if stat.Dev == targetStat.Dev && stat.Ino == targetStat.Ino {
			matches = append(matches, fd)
		}
	}
	return matches
}

func TestMaterializeArchiveSourceClosesWriteHandleBeforePublishing(t *testing.T) {
	remote := &remoteArchiveFixtureVFS{
		uri:  "memory://archive/source.zip",
		name: "source.zip",
		data: []byte("archive source bytes"),
	}
	path, _, cleanup, err := materializeArchiveSource(context.Background(), remote, remote.uri, remote.name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("clean up materialized archive source: %v", err)
		}
	}()

	if descriptors := openArchiveTestDescriptors(t, path); len(descriptors) != 0 {
		t.Fatalf("published archive source still has open write descriptor(s): %v", descriptors)
	}
}
