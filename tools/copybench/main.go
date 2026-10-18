// Command copybench measures how fast a set of files copies between two
// folders with 1, 2, 4 and 8 copies running at once, the numbers f4#1894
// needs to choose how many threads copying uses on which kind of disk.
//
// Until f4's multi-threaded copy engine exists it copies with a plain pool of
// goroutines, one file each, so the numbers show what the disks allow rather
// than what the engine makes of it.
//
//	go run ./tools/copybench -src /mnt/a -dst /mnt/b
package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/internal/sysinfo"
)

// fileSet is a set of files the benchmark copies.
type fileSet struct {
	name  string
	sizes []int64
}

func sets(scale float64) []fileSet {
	rng := rand.New(rand.NewSource(1894)) // #nosec G404 -- file sizes of a benchmark, the same on every run
	small := fileSet{name: "small (4-64 KB)"}
	for i := 0; i < int(2000*scale); i++ {
		small.sizes = append(small.sizes, int64(4+rng.Intn(61))<<10)
	}
	mixed := fileSet{name: "mixed"}
	for i := 0; i < int(400*scale); i++ {
		switch i % 10 {
		case 0:
			mixed.sizes = append(mixed.sizes, int64(8+rng.Intn(56))<<20)
		case 1, 2, 3:
			mixed.sizes = append(mixed.sizes, int64(256+rng.Intn(768))<<10)
		default:
			mixed.sizes = append(mixed.sizes, int64(4+rng.Intn(61))<<10)
		}
	}
	large := fileSet{name: "large"}
	for i := 0; i < 2; i++ {
		large.sizes = append(large.sizes, int64(1024*scale)<<20)
	}
	return []fileSet{small, mixed, large}
}

func main() {
	src := flag.String("src", "", "folder to create the files in (default: a new folder in the temp directory)")
	dst := flag.String("dst", "", "folder to copy them into (default: a new folder in the temp directory)")
	threads := flag.String("threads", "1,2,4,8", "numbers of copies running at once")
	scale := flag.Float64("scale", 1, "size of the sets: 1 is 2000 small files, 400 mixed and two of 1 GB")
	only := flag.String("set", "", "run only the set whose name starts with this")
	flush := flag.Bool("sync", true, "flush each copy to the disk before counting it done")
	flag.Parse()

	var counts []int
	for _, field := range strings.Split(*threads, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || n < 1 {
			fail(fmt.Errorf("bad -threads %q", *threads))
		}
		counts = append(counts, n)
	}
	srcRoot := makeRoot(*src, "copybench-src-")
	dstRoot := makeRoot(*dst, "copybench-dst-")
	defer func() { _ = os.RemoveAll(srcRoot) }()
	defer func() { _ = os.RemoveAll(dstRoot) }()

	fmt.Printf("source: %s (%s)\ndestination: %s (%s)\nengine: goroutine per file (before f4#1894 part 5)\n%s\n\n",
		srcRoot, describe(srcRoot), dstRoot, describe(dstRoot), cacheNote)
	fmt.Printf("%-16s %7s %8s %9s %8s %9s %9s\n", "set", "threads", "files", "MB", "seconds", "MB/s", "files/s")
	for _, set := range sets(*scale) {
		if *only != "" && !strings.HasPrefix(set.name, *only) {
			continue
		}
		files, total := create(filepath.Join(srcRoot, "set"), set)
		for _, n := range counts {
			out := filepath.Join(dstRoot, "set")
			if err := os.RemoveAll(out); err != nil {
				fail(err)
			}
			if err := os.MkdirAll(out, 0o700); err != nil {
				fail(err)
			}
			if err := dropCache(files); err != nil {
				fail(err)
			}
			start := time.Now()
			copyAll(files, out, n, *flush)
			sec := time.Since(start).Seconds()
			mb := float64(total) / (1 << 20)
			fmt.Printf("%-16s %7d %8d %9.1f %8.2f %9.1f %9.1f\n", set.name, n, len(files), mb, sec, mb/sec, float64(len(files))/sec)
		}
		if err := os.RemoveAll(filepath.Join(srcRoot, "set")); err != nil {
			fail(err)
		}
	}
}

func makeRoot(dir, pattern string) string {
	if dir != "" {
		root, err := os.MkdirTemp(dir, pattern)
		if err != nil {
			fail(err)
		}
		return root
	}
	root, err := os.MkdirTemp("", pattern)
	if err != nil {
		fail(err)
	}
	return root
}

func describe(path string) string {
	info, ok := sysinfo.Device(path)
	if !ok {
		return "unknown device"
	}
	if info.Name == "" {
		return info.Class.String()
	}
	return info.Class.String() + ", " + info.Name
}

// create writes the files of set into dir and returns their paths and size.
func create(dir string, set fileSet) ([]string, int64) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(err)
	}
	buf := make([]byte, 1<<20)
	rng := rand.New(rand.NewSource(int64(len(set.sizes)))) // #nosec G404 -- file contents of a benchmark, only meant not to compress
	rng.Read(buf)
	var files []string
	var total int64
	for i, size := range set.sizes {
		path := filepath.Join(dir, fmt.Sprintf("f%05d", i))
		f, err := os.Create(path)
		if err != nil {
			fail(err)
		}
		for left := size; left > 0; {
			n := min(left, int64(len(buf)))
			if _, err := f.Write(buf[:n]); err != nil {
				fail(err)
			}
			left -= n
		}
		if err := f.Close(); err != nil {
			fail(err)
		}
		files = append(files, path)
		total += size
	}
	return files, total
}

// copyAll copies files into dir, n at a time.
func copyAll(files []string, dir string, n int, flush bool) {
	work := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				if err := copyFile(path, filepath.Join(dir, filepath.Base(path)), flush); err != nil {
					fail(err)
				}
			}
		}()
	}
	for _, path := range files {
		work <- path
	}
	close(work)
	wg.Wait()
}

func copyFile(from, to string, flush bool) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if flush {
		if err := out.Sync(); err != nil {
			_ = out.Close()
			return err
		}
	}
	return out.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "copybench:", err)
	os.Exit(1)
}
