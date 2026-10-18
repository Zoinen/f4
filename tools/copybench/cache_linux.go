package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// dropCache takes the files out of the page cache, so the next copy reads
// them from the disk. Files the benchmark has just written, or copied a run
// before, are in memory, and reading them there measured the memory (nz256,
// f4#1894: 1843 MB/s from an SD card in a USB reader). The data is flushed
// first: the kernel keeps pages that are not written back yet.
func dropCache(files []string) error {
	for _, path := range files {
		f, err := os.Open(path) // #nosec G304 -- a file the benchmark created.
		if err != nil {
			return err
		}
		err = f.Sync()
		if err == nil {
			err = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

const cacheNote = "source cache: dropped before each run (posix_fadvise DONTNEED)"
